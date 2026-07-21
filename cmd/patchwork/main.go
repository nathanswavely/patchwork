package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/web"
)

func main() {
	configPath := flag.String("config", "patchwork.yaml", "path to config file")
	healthcheck := flag.Bool("healthcheck", false, "probe the running instance's health endpoint and exit 0 (healthy) or 1")
	flag.Parse()

	// Probe mode: no database, no server. Used by the image's HEALTHCHECK.
	if *healthcheck {
		runHealthcheck(*configPath)
		return
	}

	// Load config.
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("config: loaded instance %q", cfg.Instance.Name)

	for _, w := range cfg.Warnings() {
		log.Printf("warning: %s", w)
	}

	// Session lifetimes (docs/adr/017). Load already validated these, so the
	// error here is unreachable in practice.
	sessionMax, sessionIdle, err := cfg.Session.Durations()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	auth.ConfigureSessions(sessionMax, sessionIdle)
	log.Printf("config: sessions expire after %s, or %s idle", sessionMax, sessionIdle)

	if !cfg.SMTP.Configured() {
		log.Println("warning: SMTP not configured — magic links will print to terminal")
	}

	// Refuse to run a containerized instance whose database would land in
	// the ephemeral layer — that data silently vanishes on the next
	// `docker compose up --force-recreate`.
	if err := database.CheckDurability(cfg.Database.Path); err != nil {
		log.Fatalf("database: %v", err)
	}

	// Open database.
	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		log.Fatalf("migrations fs: %v", err)
	}

	db, err := database.Open(cfg.Database.Path, migrations)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	// Set up ActivityPub domain. AP IDs are permanent once minted, so the
	// backfills only run when federation is enabled (i.e. the domain is meant
	// to be real). Turning federation on later backfills on that boot.
	ap.SetDomain(cfg.Instance.Domain)
	if cfg.Federation.Enabled {
		if err := ap.PopulateAPIds(db, ap.GetDomain()); err != nil {
			log.Printf("warning: failed to populate AP IDs: %v", err)
		}

		// Heal locally-generated ap_ids stamped under a previous domain (e.g. a
		// dev database seeded as "localhost") — stale ones break outbound
		// delivery signing, which looks actors up by the stored ap_id.
		if n, err := ap.BackfillAPIDs(db, ap.GetDomain()); err != nil {
			log.Printf("warning: failed to heal AP IDs: %v", err)
		} else if n > 0 {
			log.Printf("federation: rewrote %d AP IDs to domain %s", n, ap.GetDomain())
		}

		// Backfill signing keypairs for entities that predate federation (e.g.
		// seeded data). Without a key they can't sign activities or serve a
		// publicKey in their AP actor document.
		if nu, nn, err := ap.BackfillKeypairs(db); err != nil {
			log.Printf("warning: failed to backfill keypairs: %v", err)
		} else if nu > 0 || nn > 0 {
			log.Printf("federation: backfilled keypairs for %d users and %d nodes", nu, nn)
		}

		// The instance service actor relays remote-patch Follows for all
		// local users (docs/adr/024) — ensure it exists and its ap_id
		// matches the configured domain.
		if err := ap.EnsureInstanceActor(db, ap.GetDomain()); err != nil {
			log.Printf("warning: failed to ensure instance actor: %v", err)
		}
	}

	// Initialize instance governance repo. Repo creation is pure go-git, so
	// it must work in any runtime (including the gitless distroless image) —
	// a failure here means something is genuinely wrong (disk, permissions)
	// and continuing would silently degrade amendment history, so refuse to
	// start rather than warn-and-degrade.
	dataDir := filepath.Dir(cfg.Database.Path)
	governance.SetDataDir(dataDir)
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		log.Fatalf("governance init: %v", err)
	}

	// Heal nodes whose repo creation failed at runtime (e.g. instances that
	// ran a pre-pure-go-git build in a container without a git binary).
	if n, err := handler.BackfillNodeGovernanceRepos(db); err != nil {
		log.Fatalf("governance backfill: %v", err)
	} else if n > 0 {
		log.Printf("governance: backfilled repos for %d nodes", n)
	}

	// One-time pass for unclaimed patches created before migration 031:
	// derive verification domains from admin-supplied websites (docs/adr/030).
	handler.BackfillVerificationDomains(db)

	// Start AP delivery worker (background goroutine) — only when the
	// instance actually federates.
	if cfg.Federation.Enabled {
		deliveryCtx, deliveryCancel := context.WithCancel(context.Background())
		defer deliveryCancel()
		ap.StartDeliveryWorker(deliveryCtx, db)
	}

	// Initialize notifier and start reminder worker.
	notifier := notifications.NewNotifier(db)
	if cfg.SMTP.Configured() {
		notifier.Channels = append(notifier.Channels, &notifications.EmailChannel{
			SMTP:         &cfg.SMTP,
			Domain:       cfg.Instance.Domain,
			InstanceName: cfg.Instance.Name,
		})
		log.Println("notifications: email channel enabled")
	}
	handler.SetNotifier(notifier)
	reminderCtx, reminderCancel := context.WithCancel(context.Background())
	defer reminderCancel()
	notifications.StartReminderWorker(reminderCtx, notifier)

	// First-run bootstrap notice: until an account exists there is no admin,
	// so tell the operator how to claim the instance.
	if auth.NoUsersExist(db) {
		log.Println("first run: no accounts exist yet — the first account created will become the instance admin")
		log.Printf("first run: sign in at https://%s/login (without SMTP, the magic link prints to this log)", cfg.Instance.Domain)
	}

	// Configure which peers may set X-Forwarded-For. Everything else has the
	// header ignored, so client IPs used for rate limiting, session rows, and
	// the audit log cannot be forged.
	if err := middleware.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		log.Fatalf("config: server.trusted_proxies: %v", err)
	}

	// Initialize WebAuthn service.
	wa, err := auth.NewWebAuthnService(db, cfg)
	if err != nil {
		log.Fatalf("webauthn: %v", err)
	}

	// Build router.
	mux := http.NewServeMux()

	// Public API routes.
	mux.HandleFunc("GET /api/v1/health", handler.Health(db, cfg))
	mux.HandleFunc("GET /api/v1/instance", handler.Instance(db, cfg))
	mux.HandleFunc("GET /api/v1/instance/icon", handler.InstanceIcon(db, cfg))

	// The Label (docs/adr/023) — public read: its most important reader
	// has no account yet. Steward self-listing is the person's own switch.
	mux.HandleFunc("GET /api/v1/label", handler.GetLabel(db))

	// Legal documents (docs/adr/028) — public read, defaults ship in the
	// binary so this never 404s on a fresh deployment.
	mux.HandleFunc("GET /api/v1/legal/{doc}", handler.LegalDoc(db, cfg))
	mux.HandleFunc("GET /api/v1/users/me/steward", middleware.AuthRequired(db, handler.GetMyStewardListing(db)))
	mux.HandleFunc("PATCH /api/v1/users/me/steward", middleware.AuthRequired(db, handler.UpdateMyStewardListing(db)))
	mux.HandleFunc("DELETE /api/v1/users/me/steward", middleware.AuthRequired(db, handler.DeleteMyStewardListing(db)))

	// Auth routes — public. Everything unauthenticated here is rate limited
	// per client IP and instance-wide: each request converts into retained
	// server memory (most sharply the WebAuthn login challenge), and the host
	// has no other throttle in front of it. Magic link routes keep their own
	// per-email and per-IP limits inside the handlers.
	rl := middleware.UnauthedAuthRateLimit
	mux.HandleFunc("POST /api/v1/auth/invite", rl(handler.RedeemInviteLink(db)))
	mux.HandleFunc("GET /api/v1/auth/invite/{token}/validate", rl(handler.ValidateInviteLink(db)))
	mux.HandleFunc("POST /api/v1/auth/magic-link", handler.RequestMagicLink(db, cfg))
	mux.HandleFunc("GET /api/v1/auth/verify/{token}", handler.VerifyMagicLink(db))
	// Alias for magic links mailed before the link builder was fixed: they
	// point at /auth/verify/{token}, which the SPA has no route for and would
	// swallow into the home page. Keep it working.
	mux.HandleFunc("GET /auth/verify/{token}", handler.VerifyMagicLink(db))
	mux.HandleFunc("GET /api/v1/auth/signup/{token}/validate", rl(handler.ValidateSignupToken(db)))
	mux.HandleFunc("POST /api/v1/auth/signup", rl(handler.CompleteSignup(db)))
	mux.HandleFunc("POST /api/v1/auth/webauthn/login/begin", rl(handler.WebAuthnLoginBegin(wa)))
	mux.HandleFunc("POST /api/v1/auth/webauthn/login/finish", rl(handler.WebAuthnLoginFinish(db, wa)))
	mux.HandleFunc("POST /api/v1/auth/recovery", rl(handler.RedeemRecoveryCode(db)))

	// Auth routes — require session.
	mux.HandleFunc("GET /api/v1/auth/me", middleware.AuthRequired(db, handler.Me(db)))
	mux.HandleFunc("PATCH /api/v1/auth/me", middleware.AuthRequired(db, handler.UpdateMe(db)))
	mux.HandleFunc("POST /api/v1/auth/logout", middleware.AuthRequired(db, handler.Logout(db)))
	mux.HandleFunc("GET /api/v1/auth/credentials", middleware.AuthRequired(db, handler.ListCredentials(db)))
	mux.HandleFunc("GET /api/v1/auth/recovery-codes", middleware.AuthRequired(db, handler.RecoveryCodeStatus(db)))
	mux.HandleFunc("POST /api/v1/auth/recovery-codes", middleware.AuthRequired(db, handler.GenerateRecoveryCodes(db)))
	mux.HandleFunc("PATCH /api/v1/auth/credentials/{id}", middleware.AuthRequired(db, handler.RenameCredential(db)))
	mux.HandleFunc("DELETE /api/v1/auth/credentials/{id}", middleware.AuthRequired(db, handler.DeleteCredential(db)))
	// Step-up: a fresh assertion from an already-signed-in person, opening a
	// short window for the three irreversible instance actions.
	mux.HandleFunc("GET /api/v1/auth/step-up", middleware.AuthRequired(db, handler.StepUpStatus(db)))
	mux.HandleFunc("POST /api/v1/auth/step-up/begin", middleware.AuthRequired(db, handler.StepUpBegin(db, wa)))
	mux.HandleFunc("POST /api/v1/auth/step-up/finish", middleware.AuthRequired(db, handler.StepUpFinish(db, wa)))

	mux.HandleFunc("POST /api/v1/auth/webauthn/register/begin", middleware.AuthRequired(db, handler.WebAuthnRegisterBegin(db, wa)))
	mux.HandleFunc("POST /api/v1/auth/webauthn/register/finish", middleware.AuthRequired(db, handler.WebAuthnRegisterFinish(db, wa)))

	// Auth routes — admin only.
	mux.HandleFunc("POST /api/v1/auth/invite-link", middleware.AdminRequired(db, handler.GenerateInviteLink(db, cfg)))

	// Node routes — public.
	// AuthOptional so ?scope=my can resolve the caller; anonymous reads are unaffected.
	mux.HandleFunc("GET /api/v1/nodes", middleware.AuthOptional(db, handler.ListNodes(db)))
	mux.HandleFunc("GET /api/v1/nodes/{slug}", middleware.AuthOptional(db, handler.GetNode(db)))
	mux.HandleFunc("GET /api/v1/nodes/{slug}/members", middleware.AuthOptional(db, handler.ListMembers(db)))
	mux.HandleFunc("GET /api/v1/nodes/{slug}/proposals", handler.ListProposals(db))

	// User profiles — public (docs/adr/006).
	mux.HandleFunc("GET /api/v1/users/{username}", handler.GetUserProfile(db))

	// Node routes — auth required.
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	mux.HandleFunc("PATCH /api/v1/nodes/{slug}", middleware.AuthRequired(db, middleware.RequireNodeRole(db, "admin")(handler.UpdateNode(db))))
	mux.HandleFunc("DELETE /api/v1/nodes/{slug}", middleware.AuthRequired(db, middleware.RequireNodeRole(db, "admin")(handler.DeleteNode(db))))

	// Membership routes — auth required.
	mux.HandleFunc("POST /api/v1/nodes/{slug}/join", middleware.AuthRequired(db, handler.JoinNode(db)))
	mux.HandleFunc("POST /api/v1/nodes/{slug}/leave", middleware.AuthRequired(db, handler.LeaveNode(db)))
	mux.HandleFunc("PATCH /api/v1/users/me/memberships/{nodeId}", middleware.AuthRequired(db, handler.UpdateMyMembershipVisibility(db)))

	// Cross-quilt following (docs/adr/024): remote follows and personal
	// connected quilts live on the follower's home instance.
	mux.HandleFunc("GET /api/v1/users/me/remote-follows", middleware.AuthRequired(db, handler.ListRemoteFollows(db)))
	mux.HandleFunc("POST /api/v1/users/me/remote-follows", middleware.AuthRequired(db, handler.CreateRemoteFollow(db, cfg)))
	mux.HandleFunc("PATCH /api/v1/users/me/remote-follows/{id}", middleware.AuthRequired(db, handler.UpdateRemoteFollow(db)))
	mux.HandleFunc("DELETE /api/v1/users/me/remote-follows/{id}", middleware.AuthRequired(db, handler.DeleteRemoteFollow(db, cfg)))
	mux.HandleFunc("GET /api/v1/users/me/quilts", middleware.AuthRequired(db, handler.ListUserQuilts(db)))
	mux.HandleFunc("POST /api/v1/users/me/quilts", middleware.AuthRequired(db, handler.AddUserQuilt(db)))
	mux.HandleFunc("DELETE /api/v1/users/me/quilts/{id}", middleware.AuthRequired(db, handler.DeleteUserQuilt(db)))
	mux.HandleFunc("GET /api/v1/me/nodes", middleware.AuthRequired(db, handler.ListMyMemberships(db)))
	mux.HandleFunc("PATCH /api/v1/nodes/{slug}/members/{userId}", middleware.AuthRequired(db, handler.UpdateMember(db)))

	// Proposal routes — public.
	mux.HandleFunc("GET /api/v1/proposals/{id}", handler.GetProposal(db))

	// Proposal routes — auth required.
	mux.HandleFunc("POST /api/v1/nodes/{slug}/proposals", middleware.AuthRequired(db, handler.CreateProposal(db)))
	mux.HandleFunc("PATCH /api/v1/proposals/{id}", middleware.AuthRequired(db, handler.UpdateProposal(db)))
	mux.HandleFunc("DELETE /api/v1/proposals/{id}", middleware.AuthRequired(db, handler.WithdrawProposal(db)))
	mux.HandleFunc("POST /api/v1/proposals/{id}/vote", middleware.AuthRequired(db, handler.VoteOnProposal(db)))
	mux.HandleFunc("POST /api/v1/proposals/{id}/apply", middleware.AuthRequired(db, handler.ApplyProposal(db)))

	// Governance routes — public.
	mux.HandleFunc("GET /api/v1/nodes/{slug}/governance", handler.ListGovernanceDocs(db))
	mux.HandleFunc("GET /api/v1/governance/{id}/versions", handler.GetGovernanceVersions(db))
	mux.HandleFunc("GET /api/v1/governance/{id}/diff", handler.GetGovernanceDiff(db))
	mux.HandleFunc("GET /api/v1/governance/{id}", handler.GetGovernanceDoc(db))

	// Governance routes — auth required.
	mux.HandleFunc("POST /api/v1/nodes/{slug}/governance", middleware.AuthRequired(db, handler.CreateGovernanceDoc(db)))
	mux.HandleFunc("PUT /api/v1/governance/{id}", middleware.AuthRequired(db, handler.UpdateGovernanceDoc(db)))
	mux.HandleFunc("GET /api/v1/nodes/{slug}/governance/rules", handler.GetGovernanceRules(db))

	// Comments.
	mux.HandleFunc("GET /api/v1/proposals/{id}/comments", handler.ListComments(db))
	mux.HandleFunc("POST /api/v1/proposals/{id}/comments", middleware.AuthRequired(db, handler.CreateComment(db)))
	mux.HandleFunc("PATCH /api/v1/comments/{id}", middleware.AuthRequired(db, handler.UpdateComment(db)))
	mux.HandleFunc("DELETE /api/v1/comments/{id}", middleware.AuthRequired(db, handler.DeleteComment(db)))
	mux.HandleFunc("POST /api/v1/comments/{id}/reactions", middleware.AuthRequired(db, handler.AddReaction(db)))
	mux.HandleFunc("DELETE /api/v1/comments/{id}/reactions/{emoji}", middleware.AuthRequired(db, handler.RemoveReaction(db)))

	// Revisions.
	mux.HandleFunc("GET /api/v1/proposals/{id}/revisions", handler.ListRevisions(db))
	mux.HandleFunc("POST /api/v1/proposals/{id}/revisions", middleware.AuthRequired(db, handler.CreateRevision(db)))

	// Event routes — public. GetEvent is AuthOptional because a pending
	// submission is visible only to its submitter and reviewers.
	mux.HandleFunc("GET /api/v1/events", handler.ListEvents(db))
	mux.HandleFunc("GET /api/v1/events/{id}", middleware.AuthOptional(db, handler.GetEvent(db)))

	// Event routes — auth required. CreateEvent decides direct-post vs
	// pending_review per docs/adr/026.
	mux.HandleFunc("POST /api/v1/events", middleware.AuthRequired(db, handler.CreateEvent(db, cfg)))
	mux.HandleFunc("PATCH /api/v1/events/{id}", middleware.AuthRequired(db, handler.UpdateEvent(db)))
	mux.HandleFunc("DELETE /api/v1/events/{id}", middleware.AuthRequired(db, handler.DeleteEvent(db)))
	mux.HandleFunc("PATCH /api/v1/events/{id}/review", middleware.AuthRequired(db, handler.ReviewEventSubmission(db)))
	mux.HandleFunc("GET /api/v1/nodes/{slug}/event-submissions", middleware.AuthRequired(db, handler.ListNodeEventSubmissions(db)))

	// Tree route — public, optionally personalized with scope=my.
	mux.HandleFunc("GET /api/v1/nodes/tree", middleware.AuthOptional(db, handler.NodeTree(db)))

	// Tag routes — public.
	mux.HandleFunc("GET /api/v1/tags", handler.ListTags(db))

	// Report routes — auth required.
	mux.HandleFunc("POST /api/v1/reports", middleware.AuthRequired(db, handler.CreateReport(db)))

	// Notification routes — auth required.
	mux.HandleFunc("GET /api/v1/notifications", middleware.AuthRequired(db, handler.ListNotifications(db)))
	mux.HandleFunc("GET /api/v1/notifications/count", middleware.AuthRequired(db, handler.NotificationCount(db)))
	mux.HandleFunc("PATCH /api/v1/notifications/{id}/read", middleware.AuthRequired(db, handler.MarkNotificationRead(db)))
	mux.HandleFunc("POST /api/v1/notifications/read-all", middleware.AuthRequired(db, handler.MarkAllNotificationsRead(db)))
	mux.HandleFunc("GET /api/v1/notifications/preferences", middleware.AuthRequired(db, handler.GetNotificationPreferences(db, notifier)))
	mux.HandleFunc("PUT /api/v1/notifications/preferences", middleware.AuthRequired(db, handler.UpdateNotificationPreferences(db)))

	// Patch notification config — admin required on the patch.
	mux.HandleFunc("GET /api/v1/nodes/{slug}/notification-config", middleware.AuthRequired(db, middleware.RequireNodeRole(db, "admin")(handler.GetPatchNotifConfig(db))))
	mux.HandleFunc("PUT /api/v1/nodes/{slug}/notification-config", middleware.AuthRequired(db, middleware.RequireNodeRole(db, "admin")(handler.UpdatePatchNotifConfig(db))))

	// Activity feed — auth required.
	mux.HandleFunc("GET /api/v1/activity", middleware.AuthRequired(db, handler.UserActivityFeed(db)))

	// AP preview — admin only.
	mux.HandleFunc("GET /api/v1/nodes/{slug}/ap-preview", middleware.AdminRequired(db, handler.APPreview(db, cfg)))

	// Admin routes.
	// Export and wipe carry a step-up gate (docs/adr/017): export moves every
	// member's email address, wipe erases the instance including its audit
	// log. A month-old cookie is not sufficient proof of presence for either.
	mux.HandleFunc("GET /api/v1/admin/export", middleware.AdminRequired(db, middleware.SudoRequired(db, handler.AdminExport(db, cfg))))
	mux.HandleFunc("POST /api/v1/admin/tags", middleware.AdminRequired(db, handler.CreateTag(db)))
	mux.HandleFunc("PATCH /api/v1/admin/tags/{id}", middleware.AdminRequired(db, handler.UpdateTag(db)))
	mux.HandleFunc("DELETE /api/v1/admin/tags/{id}", middleware.AdminRequired(db, handler.DeleteTag(db)))
	mux.HandleFunc("GET /api/v1/admin/reports", middleware.AdminRequired(db, handler.ListReports(db)))
	mux.HandleFunc("PATCH /api/v1/admin/reports/{id}", middleware.AdminRequired(db, handler.UpdateReport(db)))
	mux.HandleFunc("GET /api/v1/admin/users", middleware.AdminRequired(db, handler.ListUsers(db)))
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}", middleware.AdminRequired(db, handler.UpdateUser(db)))
	mux.HandleFunc("GET /api/v1/admin/audit-log", middleware.AdminRequired(db, handler.AuditLog(db)))
	mux.HandleFunc("GET /api/v1/admin/stats", middleware.AdminRequired(db, handler.AdminStats(db)))

	// Quilt settings (docs/adr/014): community identity + danger zone.
	// Neighbor quilts: the instance's public adjacency list (docs/adr/024).
	mux.HandleFunc("GET /api/v1/admin/neighbor-quilts", middleware.AdminRequired(db, handler.AdminListNeighborQuilts(db)))
	mux.HandleFunc("POST /api/v1/admin/neighbor-quilts", middleware.AdminRequired(db, handler.AdminAddNeighborQuilt(db)))
	mux.HandleFunc("DELETE /api/v1/admin/neighbor-quilts/{id}", middleware.AdminRequired(db, handler.AdminDeleteNeighborQuilt(db)))

	mux.HandleFunc("GET /api/v1/admin/settings", middleware.AdminRequired(db, handler.AdminGetSettings(db, cfg)))
	mux.HandleFunc("PATCH /api/v1/admin/settings", middleware.AdminRequired(db, handler.AdminUpdateSettings(db, cfg)))
	mux.HandleFunc("PUT /api/v1/admin/settings/icon", middleware.AdminRequired(db, handler.AdminUploadIcon(db)))
	mux.HandleFunc("DELETE /api/v1/admin/settings/icon", middleware.AdminRequired(db, handler.AdminDeleteIcon(db)))
	mux.HandleFunc("GET /api/v1/admin/legal", middleware.AdminRequired(db, handler.AdminGetLegal(db, cfg)))
	mux.HandleFunc("PUT /api/v1/admin/legal/{doc}", middleware.AdminRequired(db, handler.AdminUpdateLegal(db)))
	mux.HandleFunc("DELETE /api/v1/admin/legal/{doc}", middleware.AdminRequired(db, handler.AdminResetLegal(db)))
	mux.HandleFunc("GET /api/v1/admin/label", middleware.AdminRequired(db, handler.AdminGetLabel(db)))
	mux.HandleFunc("PATCH /api/v1/admin/label", middleware.AdminRequired(db, handler.AdminUpdateLabel(db)))
	mux.HandleFunc("PUT /api/v1/admin/label/costs", middleware.AdminRequired(db, handler.AdminPutLabelCosts(db)))
	mux.HandleFunc("POST /api/v1/admin/label/stewards", middleware.AdminRequired(db, handler.AdminAddLabelSteward(db)))
	mux.HandleFunc("DELETE /api/v1/admin/label/stewards/{id}", middleware.AdminRequired(db, handler.AdminRemoveLabelSteward(db)))
	mux.HandleFunc("POST /api/v1/admin/wipe", middleware.AdminRequired(db, middleware.SudoRequired(db, handler.AdminWipe(db, cfg))))

	// Unclaimed patches: community submissions + admin management.
	mux.HandleFunc("POST /api/v1/submissions", middleware.AuthRequired(db, handler.SubmitPatch(db, cfg)))
	mux.HandleFunc("POST /api/v1/admin/unclaimed", middleware.AdminRequired(db, handler.CreateUnclaimedPatch(db)))
	mux.HandleFunc("POST /api/v1/admin/unclaimed/bulk", middleware.AdminRequired(db, handler.BulkCreateUnclaimed(db)))
	mux.HandleFunc("GET /api/v1/admin/submissions", middleware.AdminRequired(db, handler.ListSubmissions(db)))
	mux.HandleFunc("PATCH /api/v1/admin/submissions/{id}", middleware.AdminRequired(db, handler.ReviewSubmission(db)))
	mux.HandleFunc("GET /api/v1/admin/event-submissions", middleware.AdminRequired(db, handler.ListAdminEventSubmissions(db)))
	mux.HandleFunc("POST /api/v1/nodes/{slug}/claim", middleware.AuthRequired(db, handler.RequestClaim(db, cfg)))
	mux.HandleFunc("GET /api/v1/nodes/{slug}/claims/mine", middleware.AuthRequired(db, handler.MyClaim(db, cfg)))
	mux.HandleFunc("POST /api/v1/claims/{id}/verify", middleware.AuthRequired(db, handler.VerifyClaim(db)))
	mux.HandleFunc("POST /api/v1/claims/{id}/withdraw", middleware.AuthRequired(db, handler.WithdrawClaim(db)))
	mux.HandleFunc("POST /api/v1/claims/{id}/resend-email", middleware.AuthRequired(db, handler.ResendClaimEmail(db, cfg)))
	// Email-claim link landing: no auth — possessing the token is the proof
	// (docs/adr/030). GET is read-only; completion requires the POST.
	mux.HandleFunc("GET /api/v1/claims/verify-email", handler.EmailClaimInfo(db))
	mux.HandleFunc("POST /api/v1/claims/verify-email", handler.CompleteEmailClaim(db))
	mux.HandleFunc("GET /api/v1/admin/claims", middleware.AdminRequired(db, handler.ListClaims(db)))
	mux.HandleFunc("PATCH /api/v1/admin/claims/{id}", middleware.AdminRequired(db, handler.ReviewClaim(db)))
	mux.HandleFunc("POST /api/v1/admin/nodes/{slug}/assign", middleware.AdminRequired(db, handler.AdminAssignOwner(db)))
	mux.HandleFunc("PATCH /api/v1/admin/nodes/{slug}/verification-domain", middleware.AdminRequired(db, handler.AdminSetVerificationDomain(db)))

	// Governance templates + overview.
	mux.HandleFunc("GET /api/v1/templates/{id}", handler.GetTemplate())
	mux.HandleFunc("GET /api/v1/nodes/{slug}/governance/overview", middleware.AuthOptional(db, handler.GovernanceOverview(db)))

	// Federation surface — honor the federation.enabled config toggle.
	// Keypair/ap_id backfill above stays unconditional so enabling later
	// is seamless.
	if cfg.Federation.Enabled {
		// ActivityPub endpoints.
		mux.HandleFunc("GET /ap/users/{id}", handler.APUser(db))
		mux.HandleFunc("GET /ap/users/{id}/outbox", handler.APUserOutbox(db))
		mux.HandleFunc("GET /ap/users/{id}/followers", handler.APUserFollowers(db))
		mux.HandleFunc("GET /ap/nodes/{id}", handler.APNode(db))
		mux.HandleFunc("GET /ap/nodes/{id}/outbox", handler.APNodeOutbox(db))
		mux.HandleFunc("GET /ap/nodes/{id}/followers", handler.APNodeFollowers(db))
		mux.HandleFunc("GET /ap/events/{id}", handler.APEvent(db))
		mux.HandleFunc("GET /ap/proposals/{id}", handler.APProposal(db))
		mux.HandleFunc("GET /ap/governance/{id}", handler.APGovernanceDoc(db))

		// AP Inbox endpoints (receive activities from remote instances).
		mux.HandleFunc("POST /ap/users/{id}/inbox", handler.APUserInbox(db))
		mux.HandleFunc("POST /ap/nodes/{id}/inbox", handler.APNodeInbox(db))

		// Instance service actor (docs/adr/024): relays cross-quilt
		// follows; its inbox receives Accepts and followed patches'
		// broadcasts.
		mux.HandleFunc("GET /ap/instance", handler.APInstanceActor(db, cfg))
		mux.HandleFunc("POST /ap/instance/inbox", handler.APInstanceInbox(db))

		// WebFinger.
		mux.HandleFunc("GET /.well-known/webfinger", handler.WebFinger(db))

		// Git smart HTTP for governance repos (federation transport).
		// Uses a wrapper that only handles /governance.git/ paths, passing through otherwise.
		gitHandler := governance.GitHTTPHandler(func(slug string) string {
			return handler.NodeIDFromSlug(db, slug)
		})
		mux.HandleFunc("GET /api/v1/nodes/{slug}/governance.git/info/refs", gitHandler.ServeHTTP)
		mux.HandleFunc("POST /api/v1/nodes/{slug}/governance.git/git-upload-pack", gitHandler.ServeHTTP)
	} else {
		log.Println("federation: disabled (federation.enabled=false) — AP, WebFinger, and git transport not mounted")
	}

	// Legacy /pins/{id} URLs (the retired UI word for events — docs/adr/027)
	// were federated into other instances' timelines; redirect them forever.
	mux.HandleFunc("GET /pins/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/events/"+r.PathValue("id"), http.StatusMovedPermanently)
	})

	// SPA: serve web/dist/ for everything else.
	dist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		log.Fatalf("dist fs: %v", err)
	}

	// Read index.html from embedded FS for SEO tag injection.
	spaHTML, err := fs.ReadFile(web.DistFS, "dist/index.html")
	if err != nil {
		log.Fatalf("read index.html: %v", err)
	}

	spa := spaHandler{fs: http.FS(dist)}
	seoWrapped := middleware.SEO(db, cfg, spaHTML)(spa)
	mux.Handle("/", seoWrapped)

	// Middleware stack: CORS → CSRF → routes.
	var root http.Handler = mux
	root = middleware.CSRF(root)
	root = middleware.CORS(cfg, root)

	// Start server.
	addr := ":" + cfg.Server.Port
	log.Printf("server: listening on %s", addr)

	if !cfg.SMTP.Configured() {
		// Only advertise seed-data test accounts when they actually exist
		// (i.e. this is a seeded dev database, not a fresh production deploy).
		var seeded bool
		db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email = 'admin@localhost')`).Scan(&seeded)

		fmt.Println("\n\033[1;33m╔══════════════════════════════════════════════════╗")
		fmt.Println("║          🧵 Patchwork Login                       ║")
		fmt.Println("╠══════════════════════════════════════════════════╣")
		fmt.Println("║                                                  ║")
		fmt.Println("║  Enter any email on the login page.              ║")
		fmt.Println("║  The magic link will print here in the terminal. ║")
		fmt.Println("║                                                  ║")
		if seeded {
			fmt.Println("║  Test accounts (enter email on login page):      ║")
			fmt.Println("║                                                  ║")
			fmt.Println("║  admin@localhost     — Site admin, many patches  ║")
			fmt.Println("║  organizer@localhost — Runs 6 patches            ║")
			fmt.Println("║  active@localhost    — Member of many patches    ║")
			fmt.Println("║  lurker@localhost    — Follows lots, joins none  ║")
			fmt.Println("║  new@localhost       — Brand new, no memberships ║")
			fmt.Println("║                                                  ║")
		}
		fmt.Println("╚══════════════════════════════════════════════════╝\033[0m")
	}

	if err := http.ListenAndServe(addr, root); err != nil {
		log.Fatalf("server: %v", err)
		os.Exit(1)
	}
}

// spaHandler serves static files and falls back to index.html for client-side routing.
type spaHandler struct {
	fs http.FileSystem
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if strings.HasPrefix(path, "/api/") {
		http.NotFound(w, r)
		return
	}

	// Try to serve the file directly.
	f, err := h.fs.Open(path)
	if err != nil {
		// Fall back to index.html for SPA routing.
		r.URL.Path = "/"
		http.FileServer(h.fs).ServeHTTP(w, r)
		return
	}
	f.Close()

	http.FileServer(h.fs).ServeHTTP(w, r)
}
