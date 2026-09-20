package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/gazetteer"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// The route table lives here rather than inline in main() so that a test can
// walk it. Authorization in Patchwork is decided inside the handlers, because
// the per-template and per-venue rules do not fit a role gate at the router;
// docs/adr/092's who-decides matrix is why. So the only way to know that a new
// mutating route refuses the people it should is to enumerate the routes and
// ask each one. routes_auth_test.go does exactly that.
//
// Nothing here decides anything: buildRoutes is the registration block main()
// used to hold, moved as it stood, and the recorder below only writes down
// what it registers.

// route is one registered pattern: the method it answers and the path it
// matches, split out of the ServeMux pattern string.
type route struct {
	Method  string
	Pattern string
}

// routeTable is a ServeMux that remembers what was registered on it.
type routeTable struct {
	mux    *http.ServeMux
	routes []route
}

func (t *routeTable) record(pattern string) {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		// A pattern with no method answers every method (the SPA's "/").
		t.routes = append(t.routes, route{Method: "", Pattern: pattern})
		return
	}
	t.routes = append(t.routes, route{Method: method, Pattern: path})
}

func (t *routeTable) handleFunc(pattern string, h http.HandlerFunc) {
	t.record(pattern)
	t.mux.HandleFunc(pattern, h)
}

func (t *routeTable) handle(pattern string, h http.Handler) {
	t.record(pattern)
	t.mux.Handle(pattern, h)
}

// serverDeps is everything the route table needs from main's startup: the
// values the handlers close over, plus the already-wrapped SPA handler that
// catches everything the API does not.
type serverDeps struct {
	db       *database.DB
	cfg      *config.Config
	wa       *auth.WebAuthnService
	gaz      *gazetteer.Gazetteer
	notifier *notifications.Notifier
	usage    *middleware.UsageCounter
	spa      http.Handler
}

// buildRoutes registers every route and hands back the mux plus the list of
// what went onto it. The outer middleware stack (the crawler gate,
// compression, CORS, CSRF) is applied by the caller.
func buildRoutes(d serverDeps) (*http.ServeMux, []route) {
	t := &routeTable{mux: http.NewServeMux()}

	// Public API routes.
	t.handleFunc("GET /api/v1/health", handler.Health(d.db, d.cfg))
	t.handleFunc("GET /api/v1/instance", handler.Instance(d.db, d.cfg))
	t.handleFunc("GET /api/v1/instance/icon", handler.InstanceIcon(d.db, d.cfg))
	t.handleFunc("GET /api/v1/instance/lining", handler.GetInstanceLining(d.db))

	// The public half of admin attestation (docs/adr/087). Deliberately here
	// rather than under the federation gate with /ap/instance: a verifier
	// checking an admin's proof has no account and does not care whether this
	// quilt federates.
	t.handleFunc("GET /api/v1/instance/attestation-key", handler.AttestationKey(d.db, d.cfg))

	// The platform association files for the native apps this quilt vouches
	// for (docs/adr/2026-09-20-an-instance-vouches-for-an-app.md). Here for
	// the same reason the attestation key is: an app store's fetcher has no
	// account and does not care whether this instance federates, so neither
	// file may sit behind the federation gate.
	//
	// Both are exact patterns, so ServeMux matches them ahead of the SPA's
	// "/" catch-all — the specific pattern wins, the way
	// /.well-known/webfinger already does. Each answers 404 until an admin
	// has listed an app of that platform: an instance that asked for none of
	// this publishes none of it.
	t.handleFunc("GET "+handler.AppleAssociationPath, handler.AppleAppSiteAssociation(d.db))
	t.handleFunc("GET "+handler.AndroidAssociationPath, handler.AssetLinks(d.db))

	// Suggesting a placement from an address. Authenticated and throttled;
	// answers "no suggestion" rather than an error when there is no index or
	// no match, because both are ordinary.
	t.handleFunc("GET /api/v1/gazetteer/suggest", middleware.AuthRequired(d.db, handler.SuggestPlace(d.gaz)))

	// The Label (docs/adr/023) — public read: its most important reader
	// has no account yet. Steward self-listing is the person's own switch.
	t.handleFunc("GET /api/v1/label", handler.GetLabel(d.db, d.cfg))

	// Legal documents (docs/adr/028) — public read, defaults ship in the
	// binary so this never 404s on a fresh deployment.
	t.handleFunc("GET /api/v1/legal/{doc}", handler.LegalDoc(d.db, d.cfg))
	t.handleFunc("GET /api/v1/users/me/steward", middleware.AuthRequired(d.db, handler.GetMyStewardListing(d.db)))
	t.handleFunc("PATCH /api/v1/users/me/steward", middleware.AuthRequired(d.db, handler.UpdateMyStewardListing(d.db)))
	t.handleFunc("DELETE /api/v1/users/me/steward", middleware.AuthRequired(d.db, handler.DeleteMyStewardListing(d.db)))

	// Auth routes — public. Everything unauthenticated here is rate limited
	// per client IP and instance-wide: each request converts into retained
	// server memory (most sharply the WebAuthn login challenge), and the host
	// has no other throttle in front of it. Magic link routes keep their own
	// per-email and per-IP limits inside the handlers.
	rl := middleware.UnauthedAuthRateLimit
	t.handleFunc("POST /api/v1/auth/invite", rl(handler.RedeemInviteLink(d.db, d.cfg)))
	t.handleFunc("GET /api/v1/auth/invite/{token}/validate", rl(handler.ValidateInviteLink(d.db)))
	t.handleFunc("POST /api/v1/auth/magic-link", handler.RequestMagicLink(d.db, d.cfg))
	// The code carried by the same email, for a client that cannot receive
	// the link in its own session. Its own per-email limiter lives inside
	// the handler, the way the request route's does, on top of this shared
	// unauthed budget.
	t.handleFunc("POST /api/v1/auth/magic-link/verify", rl(handler.VerifyMagicCode(d.db)))
	t.handleFunc("GET /api/v1/auth/verify/{token}", handler.VerifyMagicLink(d.db))
	// Alias for magic links mailed before the link builder was fixed: they
	// point at /auth/verify/{token}, which the SPA has no route for and would
	// swallow into the home page. Keep it working.
	t.handleFunc("GET /auth/verify/{token}", handler.VerifyMagicLink(d.db))
	t.handleFunc("GET /api/v1/auth/signup/{token}/validate", rl(handler.ValidateSignupToken(d.db)))
	t.handleFunc("POST /api/v1/auth/signup", rl(handler.CompleteSignup(d.db)))
	t.handleFunc("POST /api/v1/auth/webauthn/login/begin", rl(handler.WebAuthnLoginBegin(d.wa)))
	t.handleFunc("POST /api/v1/auth/webauthn/login/finish", rl(handler.WebAuthnLoginFinish(d.db, d.wa)))
	t.handleFunc("POST /api/v1/auth/recovery", rl(handler.RedeemRecoveryCode(d.db)))

	// Auth routes — require session.
	t.handleFunc("GET /api/v1/auth/me", middleware.AuthRequired(d.db, handler.Me(d.db)))
	t.handleFunc("PATCH /api/v1/auth/me", middleware.AuthRequired(d.db, handler.UpdateMe(d.db)))
	t.handleFunc("POST /api/v1/auth/logout", middleware.AuthRequired(d.db, handler.Logout(d.db)))
	t.handleFunc("GET /api/v1/auth/credentials", middleware.AuthRequired(d.db, handler.ListCredentials(d.db)))
	t.handleFunc("GET /api/v1/auth/recovery-codes", middleware.AuthRequired(d.db, handler.RecoveryCodeStatus(d.db)))
	t.handleFunc("POST /api/v1/auth/recovery-codes", middleware.AuthRequired(d.db, handler.GenerateRecoveryCodes(d.db)))
	t.handleFunc("PATCH /api/v1/auth/credentials/{id}", middleware.AuthRequired(d.db, handler.RenameCredential(d.db)))
	t.handleFunc("DELETE /api/v1/auth/credentials/{id}", middleware.AuthRequired(d.db, handler.DeleteCredential(d.db)))
	// Session manager: a person sees and revokes only their own sessions
	// (issue #3, follow-up to docs/adr/017).
	t.handleFunc("GET /api/v1/auth/sessions", middleware.AuthRequired(d.db, handler.ListSessions(d.db)))
	t.handleFunc("POST /api/v1/auth/sessions/revoke-others", middleware.AuthRequired(d.db, handler.RevokeOtherSessions(d.db)))
	t.handleFunc("DELETE /api/v1/auth/sessions/{id}", middleware.AuthRequired(d.db, handler.RevokeSession(d.db)))
	// Step-up: a fresh assertion from an already-signed-in person, opening a
	// short window for the three irreversible instance actions.
	t.handleFunc("GET /api/v1/auth/step-up", middleware.AuthRequired(d.db, handler.StepUpStatus(d.db)))
	t.handleFunc("POST /api/v1/auth/step-up/begin", middleware.AuthRequired(d.db, handler.StepUpBegin(d.db, d.wa)))
	t.handleFunc("POST /api/v1/auth/step-up/finish", middleware.AuthRequired(d.db, handler.StepUpFinish(d.db, d.wa)))
	// The way through for a person whose device makes no passkey
	// (docs/adr/099). Burns a recovery code that predates this session.
	t.handleFunc("POST /api/v1/auth/step-up/recovery", middleware.AuthRequired(d.db, handler.StepUpRecovery(d.db)))

	t.handleFunc("POST /api/v1/auth/webauthn/register/begin", middleware.AuthRequired(d.db, handler.WebAuthnRegisterBegin(d.db, d.wa)))
	t.handleFunc("POST /api/v1/auth/webauthn/register/finish", middleware.AuthRequired(d.db, handler.WebAuthnRegisterFinish(d.db, d.wa)))

	// Auth routes — admin only.
	t.handleFunc("POST /api/v1/auth/invite-link", middleware.AdminRequired(d.db, handler.GenerateInviteLink(d.db, d.cfg)))

	// Node routes — public.
	// AuthOptional so ?scope=my can resolve the caller; anonymous reads are unaffected.
	t.handleFunc("GET /api/v1/nodes", middleware.AuthOptional(d.db, handler.ListNodes(d.db)))
	t.handleFunc("GET /api/v1/nodes/{slug}", middleware.AuthOptional(d.db, handler.GetNode(d.db)))
	t.handleFunc("GET /api/v1/nodes/{slug}/members", middleware.AuthOptional(d.db, handler.ListMembers(d.db)))
	t.handleFunc("GET /api/v1/nodes/{slug}/proposals", middleware.AuthOptional(d.db, handler.ListProposals(d.db)))

	// User profiles — public (docs/adr/006). AuthOptional because the shared
	// contact items on a profile depend on the caller (docs/adr/083); every
	// other field on the page is the same for everybody, anonymous included.
	t.handleFunc("GET /api/v1/users/{username}", middleware.AuthOptional(d.db, handler.GetUserProfile(d.db)))

	// Node routes — auth required.
	t.handleFunc("POST /api/v1/nodes", middleware.AuthRequired(d.db, handler.CreateNode(d.db)))
	t.handleFunc("PATCH /api/v1/nodes/{slug}", middleware.AuthRequired(d.db, middleware.RequireNodeRole(d.db, "admin")(handler.UpdateNode(d.db))))
	t.handleFunc("DELETE /api/v1/nodes/{slug}", middleware.AuthRequired(d.db, middleware.RequireNodeRole(d.db, "admin")(handler.DeleteNode(d.db))))

	// Membership routes — auth required.
	// Outbound calendar feeds (docs/adr/031): every public patch is
	// subscribable; the personal feed's URL secret is its credential.
	t.handleFunc("GET /api/v1/nodes/{slug}/events.ics", handler.NodeICSFeed(d.db, d.cfg))
	t.handleFunc("GET /api/v1/nodes/{slug}/events.rss", handler.NodeRSSFeed(d.db, d.cfg))
	t.handleFunc("GET /api/v1/feeds/{secret}/events.ics", rl(handler.PersonalICSFeed(d.db, d.cfg)))
	t.handleFunc("GET /api/v1/users/me/feed-secret", middleware.AuthRequired(d.db, handler.FeedSecretStatus(d.db)))
	t.handleFunc("POST /api/v1/users/me/feed-secret", middleware.AuthRequired(d.db, handler.GenerateFeedSecret(d.db, d.cfg)))
	t.handleFunc("DELETE /api/v1/users/me/feed-secret", middleware.AuthRequired(d.db, handler.DeleteFeedSecret(d.db)))

	// Event sources (docs/adr/031): owner-attached calendar feeds.
	t.handleFunc("GET /api/v1/nodes/{slug}/event-sources", middleware.AuthRequired(d.db, handler.ListEventSources(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/event-sources", middleware.AuthRequired(d.db, handler.CreateEventSource(d.db)))
	t.handleFunc("PATCH /api/v1/nodes/{slug}/event-sources/{id}", middleware.AuthRequired(d.db, handler.UpdateEventSource(d.db)))
	t.handleFunc("DELETE /api/v1/nodes/{slug}/event-sources/{id}", middleware.AuthRequired(d.db, handler.DeleteEventSource(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/event-sources/{id}/sync", middleware.AuthRequired(d.db, handler.SyncEventSource(d.db)))
	t.handleFunc("POST /api/v1/events/{id}/detach", middleware.AuthRequired(d.db, handler.DetachEvent(d.db)))

	// Aggregators and the crosswalk (docs/adr/056). The node-scoped
	// routes are the door for a patch's own admins; mapping an active
	// patch is deliberately not an instance-admin power.
	t.handleFunc("GET /api/v1/nodes/{slug}/aggregator-names", middleware.AuthRequired(d.db, handler.ListAggregatorNames(d.db)))
	t.handleFunc("GET /api/v1/nodes/{slug}/crosswalk", middleware.AuthRequired(d.db, handler.ListCrosswalk(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/crosswalk", middleware.AuthRequired(d.db, handler.CreateCrosswalkEntry(d.db)))
	t.handleFunc("DELETE /api/v1/nodes/{slug}/crosswalk/{id}", middleware.AuthRequired(d.db, handler.DeleteCrosswalkEntry(d.db)))
	t.handleFunc("GET /api/v1/nodes/{slug}/aggregator-holds", middleware.AuthRequired(d.db, handler.ListAggregatorHolds(d.db)))
	t.handleFunc("POST /api/v1/aggregator-holds/{id}/decide", middleware.AuthRequired(d.db, handler.DecideAggregatorHold(d.db)))
	// Programs and their offers (docs/adr/063). Node-scoped because
	// standing is over the credited patch and nothing else — the venue
	// whose event it is has no say and needs none.
	t.handleFunc("GET /api/v1/nodes/{slug}/programs", middleware.AuthRequired(d.db, handler.ListPrograms(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/programs", middleware.AuthRequired(d.db, handler.CreateProgram(d.db)))
	t.handleFunc("DELETE /api/v1/nodes/{slug}/programs/{id}", middleware.AuthRequired(d.db, handler.DeleteProgram(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/offers/dismiss", middleware.AuthRequired(d.db, handler.DismissOffer(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/events/bulk", middleware.AuthRequired(d.db, handler.BulkCreateEvents(d.db)))

	t.handleFunc("POST /api/v1/nodes/{slug}/join", middleware.AuthRequired(d.db, handler.JoinNode(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/leave", middleware.AuthRequired(d.db, handler.LeaveNode(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/withdraw", middleware.AuthRequired(d.db, handler.WithdrawMembershipRequest(d.db)))
	// Membership invitations (docs/adr/098): an admin asks by username, the
	// person answers. Accept and decline are the invitee's own row only.
	t.handleFunc("POST /api/v1/nodes/{slug}/invitations", middleware.AuthRequired(d.db, handler.InviteMember(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/invitations/accept", middleware.AuthRequired(d.db, handler.AcceptInvitation(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/invitations/decline", middleware.AuthRequired(d.db, handler.DeclineInvitation(d.db)))
	t.handleFunc("DELETE /api/v1/nodes/{slug}/invitations/{userId}", middleware.AuthRequired(d.db, handler.RescindInvitation(d.db)))
	t.handleFunc("GET /api/v1/users/me/invitations", middleware.AuthRequired(d.db, handler.ListMyInvitations(d.db)))
	// Maintainer succession (docs/adr/051). Naming a successor decides who
	// inherits the patch, so it is step-up gated like the other power moves.
	t.handleFunc("PUT /api/v1/nodes/{slug}/successor", middleware.AuthRequired(d.db, middleware.SudoRequired(d.db, handler.SetSuccessor(d.db))))
	t.handleFunc("DELETE /api/v1/nodes/{slug}/successor", middleware.AuthRequired(d.db, handler.ClearSuccessor(d.db)))
	// Elections (docs/adr/051). Nominating is a member act; the ballot is the
	// set of candidates one person approves, so it is a PUT of the whole set
	// rather than an append.
	t.handleFunc("POST /api/v1/proposals/{id}/candidates", middleware.AuthRequired(d.db, handler.AddCandidate(d.db)))
	// Your own candidacy, and the route says so (docs/adr/107). Standing was
	// irreversible, which is what made a mis-click on a public ballot a thing
	// somebody had to write a comment to undo.
	t.handleFunc("DELETE /api/v1/proposals/{id}/candidates/me", middleware.AuthRequired(d.db, handler.WithdrawCandidacy(d.db)))
	// The council's size is its seats, added and removed explicitly by an
	// admin of the patch (docs/adr/100). Neither act seats or unseats anybody
	// — removal only ever touches an empty chair — so neither is step-up
	// gated the way a power transfer is.
	t.handleFunc("POST /api/v1/nodes/{slug}/seats", middleware.AuthRequired(d.db, handler.AddSeat(d.db)))
	t.handleFunc("DELETE /api/v1/nodes/{slug}/seats/{id}", middleware.AuthRequired(d.db, handler.RemoveSeat(d.db)))
	// A seat's term end is the patch's election calendar (docs/adr/051 put the
	// clock on the seat). An admin may bring a chair's date forward; pushing a
	// held one back would hand out a term nobody voted for, and is refused.
	t.handleFunc("PATCH /api/v1/nodes/{slug}/seats/{id}", middleware.AuthRequired(d.db, handler.SetSeatTerm(d.db)))
	t.handleFunc("PUT /api/v1/proposals/{id}/ballot", middleware.AuthRequired(d.db, handler.CastElectionBallot(d.db)))
	// Attestations (docs/adr/052, docs/adr/053) — decisions a community made
	// somewhere Patchwork was not. Public to read: the whole value is that the
	// people who were in the room can check it. Recording one moves who runs
	// the patch or what its charter says, so both are step-up gated like every
	// other power move.
	t.handleFunc("GET /api/v1/nodes/{slug}/attestations", middleware.AuthOptional(d.db, handler.ListAttestations(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/attestations", middleware.AuthRequired(d.db, middleware.SudoRequired(d.db, handler.CreateAttestation(d.db))))
	t.handleFunc("PATCH /api/v1/nodes/{slug}/attestation-names/{id}", middleware.AuthRequired(d.db, middleware.SudoRequired(d.db, handler.LinkAttestationName(d.db))))
	t.handleFunc("GET /api/v1/nodes/{slug}/amendment-attestations", middleware.AuthOptional(d.db, handler.ListAmendmentAttestations(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/amendment-attestations", middleware.AuthRequired(d.db, middleware.SudoRequired(d.db, handler.CreateAmendmentAttestation(d.db))))
	t.handleFunc("PATCH /api/v1/users/me/memberships/{nodeId}", middleware.AuthRequired(d.db, handler.UpdateMyMembership(d.db)))

	// Self-serve account deletion (docs/adr/086). Step-up gated for the same
	// reason the wipe is: a valid cookie is proof of identity, and this is
	// irreversible enough to need proof of presence too (docs/adr/017).
	t.handleFunc("DELETE /api/v1/users/me", middleware.AuthRequired(d.db, middleware.SudoRequired(d.db, handler.DeleteMyAccount(d.db, d.cfg))))

	// The contact card (docs/adr/083). The items live on the account and are
	// only ever edited by their owner; sharing them is patch-first, below,
	// because that is where the intent forms. The one item-first write is
	// unshare-everywhere, which can only reduce exposure.
	t.handleFunc("GET /api/v1/users/me/contact-items", middleware.AuthRequired(d.db, handler.ListMyContactItems(d.db)))
	t.handleFunc("POST /api/v1/users/me/contact-items", middleware.AuthRequired(d.db, handler.CreateMyContactItem(d.db)))
	t.handleFunc("PATCH /api/v1/users/me/contact-items/{id}", middleware.AuthRequired(d.db, handler.UpdateMyContactItem(d.db)))
	t.handleFunc("DELETE /api/v1/users/me/contact-items/{id}", middleware.AuthRequired(d.db, handler.DeleteMyContactItem(d.db)))
	t.handleFunc("DELETE /api/v1/users/me/contact-items/{id}/shares", middleware.AuthRequired(d.db, handler.UnshareMyContactItemEverywhere(d.db)))
	t.handleFunc("GET /api/v1/nodes/{slug}/contact-shares", middleware.AuthRequired(d.db, handler.GetMyContactSharesForNode(d.db)))
	t.handleFunc("PUT /api/v1/nodes/{slug}/contact-shares", middleware.AuthRequired(d.db, handler.PutMyContactSharesForNode(d.db)))

	// The noticeboard — members-only, the check in every handler (docs/adr/081).
	t.handleFunc("GET /api/v1/nodes/{slug}/notices", middleware.AuthRequired(d.db, handler.ListNotices(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/notices", middleware.AuthRequired(d.db, handler.CreateNotice(d.db)))
	t.handleFunc("GET /api/v1/notices/{id}", middleware.AuthRequired(d.db, handler.GetNotice(d.db)))
	t.handleFunc("PATCH /api/v1/notices/{id}", middleware.AuthRequired(d.db, handler.UpdateNotice(d.db)))
	t.handleFunc("DELETE /api/v1/notices/{id}", middleware.AuthRequired(d.db, handler.DeleteNotice(d.db)))
	t.handleFunc("GET /api/v1/notices/{id}/replies", middleware.AuthRequired(d.db, handler.ListReplies(d.db)))
	t.handleFunc("POST /api/v1/notices/{id}/replies", middleware.AuthRequired(d.db, handler.CreateReply(d.db)))
	t.handleFunc("PATCH /api/v1/replies/{id}", middleware.AuthRequired(d.db, handler.UpdateReply(d.db)))
	t.handleFunc("DELETE /api/v1/replies/{id}", middleware.AuthRequired(d.db, handler.DeleteReply(d.db)))
	// The patch's own report queue for its noticeboard (docs/adr/081, tool 3).
	t.handleFunc("GET /api/v1/nodes/{slug}/reports", middleware.AuthRequired(d.db, middleware.RequireNodeRole(d.db, "admin")(handler.ListPatchReports(d.db))))
	t.handleFunc("PATCH /api/v1/nodes/{slug}/reports/{id}", middleware.AuthRequired(d.db, middleware.RequireNodeRole(d.db, "admin")(handler.UpdatePatchReport(d.db))))

	// Cross-quilt following (docs/adr/024): remote follows and personal
	// connected quilts live on the follower's home instance.
	t.handleFunc("GET /api/v1/users/me/remote-follows", middleware.AuthRequired(d.db, handler.ListRemoteFollows(d.db)))
	t.handleFunc("POST /api/v1/users/me/remote-follows", middleware.AuthRequired(d.db, handler.CreateRemoteFollow(d.db, d.cfg)))
	t.handleFunc("PATCH /api/v1/users/me/remote-follows/{id}", middleware.AuthRequired(d.db, handler.UpdateRemoteFollow(d.db)))
	t.handleFunc("DELETE /api/v1/users/me/remote-follows/{id}", middleware.AuthRequired(d.db, handler.DeleteRemoteFollow(d.db, d.cfg)))
	t.handleFunc("GET /api/v1/users/me/quilts", middleware.AuthRequired(d.db, handler.ListUserQuilts(d.db)))
	t.handleFunc("POST /api/v1/users/me/quilts", middleware.AuthRequired(d.db, handler.AddUserQuilt(d.db)))
	t.handleFunc("DELETE /api/v1/users/me/quilts/{id}", middleware.AuthRequired(d.db, handler.DeleteUserQuilt(d.db)))
	t.handleFunc("GET /api/v1/me/nodes", middleware.AuthRequired(d.db, handler.ListMyMemberships(d.db)))
	// Personal export (docs/adr/012, affordance 1): everything about the
	// person asking, with no admin involved. Rate-limited inside the handler.
	t.handleFunc("GET /api/v1/users/me/export", middleware.AuthRequired(d.db, handler.PersonalExport(d.db, d.cfg)))
	// Member seamrip (docs/adr/012, affordance 2; docs/adr/089): the quilt
	// as this member can already see it, in the import format, so a fork
	// needs nobody's permission. Rate-limited inside the handler, tighter
	// than the personal export.
	t.handleFunc("GET /api/v1/users/me/seamrip", middleware.AuthRequired(d.db, handler.MemberSeamrip(d.db, d.cfg)))
	// The trust request
	// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
	// decision 7). One open ask per person, made from the event form and
	// answered from Admin → Users. There is deliberately no navigation entry
	// into these: nobody should go looking for a rank.
	t.handleFunc("GET /api/v1/users/me/trust-request", middleware.AuthRequired(d.db, handler.GetMyTrustRequest(d.db)))
	t.handleFunc("POST /api/v1/users/me/trust-request", middleware.AuthRequired(d.db, handler.CreateTrustRequest(d.db)))
	t.handleFunc("PATCH /api/v1/nodes/{slug}/members/{userId}", middleware.AuthRequired(d.db, handler.UpdateMember(d.db)))

	// Proposal routes — public, but amendment text follows the target
	// charter's visibility, so the optional session is read (docs/adr/036).
	t.handleFunc("GET /api/v1/proposals/{id}", middleware.AuthOptional(d.db, handler.GetProposal(d.db)))

	// Proposal routes — auth required.
	t.handleFunc("POST /api/v1/nodes/{slug}/proposals", middleware.AuthRequired(d.db, handler.CreateProposal(d.db)))
	t.handleFunc("PATCH /api/v1/proposals/{id}", middleware.AuthRequired(d.db, handler.UpdateProposal(d.db)))
	t.handleFunc("DELETE /api/v1/proposals/{id}", middleware.AuthRequired(d.db, handler.WithdrawProposal(d.db)))
	t.handleFunc("POST /api/v1/proposals/{id}/vote", middleware.AuthRequired(d.db, handler.VoteOnProposal(d.db)))
	t.handleFunc("POST /api/v1/proposals/{id}/apply", middleware.AuthRequired(d.db, handler.ApplyProposal(d.db)))
	// The maintainer's verbs on an admin-decides patch (docs/adr/092).
	t.handleFunc("POST /api/v1/proposals/{id}/decide", middleware.AuthRequired(d.db, handler.DecideProposal(d.db)))
	t.handleFunc("POST /api/v1/proposals/{id}/open-vote", middleware.AuthRequired(d.db, handler.OpenAdvisoryVote(d.db)))

	// Governance reads — public docs for everyone, members-only docs for
	// viewers the patch has admitted, so each needs the optional session
	// (docs/adr/036).
	t.handleFunc("GET /api/v1/nodes/{slug}/governance", middleware.AuthOptional(d.db, handler.ListGovernanceDocs(d.db)))
	t.handleFunc("GET /api/v1/governance/{id}/versions", middleware.AuthOptional(d.db, handler.GetGovernanceVersions(d.db)))
	t.handleFunc("GET /api/v1/governance/{id}/diff", middleware.AuthOptional(d.db, handler.GetGovernanceDiff(d.db)))
	t.handleFunc("GET /api/v1/governance/{id}", middleware.AuthOptional(d.db, handler.GetGovernanceDoc(d.db)))

	// Governance routes — auth required.
	t.handleFunc("POST /api/v1/nodes/{slug}/governance", middleware.AuthRequired(d.db, handler.CreateGovernanceDoc(d.db)))
	t.handleFunc("PUT /api/v1/governance/{id}", middleware.AuthRequired(d.db, handler.UpdateGovernanceDoc(d.db)))
	t.handleFunc("GET /api/v1/nodes/{slug}/governance/rules", handler.GetGovernanceRules(d.db))
	// Beside the rules rather than part of them: the editor spreads what the
	// rules endpoint sends back into its submission, so facts about the patch
	// cannot travel in that payload (docs/adr/104).
	t.handleFunc("GET /api/v1/nodes/{slug}/governance/electorate", middleware.AuthRequired(d.db, handler.GovernanceElectorate(d.db)))

	// Comments.
	t.handleFunc("GET /api/v1/proposals/{id}/comments", middleware.AuthOptional(d.db, handler.ListComments(d.db)))
	t.handleFunc("POST /api/v1/proposals/{id}/comments", middleware.AuthRequired(d.db, handler.CreateComment(d.db)))
	t.handleFunc("PATCH /api/v1/comments/{id}", middleware.AuthRequired(d.db, handler.UpdateComment(d.db)))
	t.handleFunc("DELETE /api/v1/comments/{id}", middleware.AuthRequired(d.db, handler.DeleteComment(d.db)))
	t.handleFunc("POST /api/v1/comments/{id}/reactions", middleware.AuthRequired(d.db, handler.AddReaction(d.db)))
	t.handleFunc("DELETE /api/v1/comments/{id}/reactions/{emoji}", middleware.AuthRequired(d.db, handler.RemoveReaction(d.db)))

	// Revisions.
	t.handleFunc("GET /api/v1/proposals/{id}/revisions", middleware.AuthOptional(d.db, handler.ListRevisions(d.db)))
	t.handleFunc("POST /api/v1/proposals/{id}/revisions", middleware.AuthRequired(d.db, handler.CreateRevision(d.db)))

	// Event routes — public. GetEvent is AuthOptional because a pending
	// submission is visible only to its submitter and reviewers.
	t.handleFunc("GET /api/v1/events", middleware.AuthOptional(d.db, handler.ListEvents(d.db)))
	t.handleFunc("GET /api/v1/events/{id}", middleware.AuthOptional(d.db, handler.GetEvent(d.db)))
	// One event as a downloadable calendar file (docs/adr/093). Auth is
	// optional because a public event is anonymous, and a members-only
	// one has to see who is asking. The wildcard needs its own segment:
	// a pattern cannot mix a wildcard and a literal in one segment.
	t.handleFunc("GET /api/v1/events/{id}/event.ics", middleware.AuthOptional(d.db, handler.EventICS(d.db, d.cfg)))

	// Event routes — auth required. CreateEvent decides direct-post vs
	// pending_review per docs/adr/026.
	t.handleFunc("POST /api/v1/events", middleware.AuthRequired(d.db, handler.CreateEvent(d.db, d.cfg)))
	t.handleFunc("PATCH /api/v1/events/{id}", middleware.AuthRequired(d.db, handler.UpdateEvent(d.db)))
	t.handleFunc("DELETE /api/v1/events/{id}", middleware.AuthRequired(d.db, handler.DeleteEvent(d.db)))
	t.handleFunc("PATCH /api/v1/events/{id}/review", middleware.AuthRequired(d.db, handler.ReviewEventSubmission(d.db)))
	// Event links (docs/adr/032): one owner, two consents.
	t.handleFunc("POST /api/v1/events/{id}/links", middleware.AuthRequired(d.db, handler.CreateEventLink(d.db, d.cfg)))
	t.handleFunc("POST /api/v1/events/{id}/links/{nodeId}/confirm", middleware.AuthRequired(d.db, handler.ConfirmEventLink(d.db)))
	t.handleFunc("DELETE /api/v1/events/{id}/links/{nodeId}", middleware.AuthRequired(d.db, handler.RemoveEventLink(d.db)))
	t.handleFunc("DELETE /api/v1/events/{id}/mentions/{mentionId}", middleware.AuthRequired(d.db, handler.RemoveEventMention(d.db)))
	t.handleFunc("GET /api/v1/nodes/{slug}/event-submissions", middleware.AuthRequired(d.db, handler.ListNodeEventSubmissions(d.db)))

	// Tree route — public, optionally personalized with scope=my.
	t.handleFunc("GET /api/v1/nodes/tree", middleware.AuthOptional(d.db, handler.NodeTree(d.db)))

	// Tag routes — public.
	t.handleFunc("GET /api/v1/tags", handler.ListTags(d.db))

	// Report routes — auth required.
	t.handleFunc("POST /api/v1/reports", middleware.AuthRequired(d.db, handler.CreateReport(d.db)))

	// Notification routes — auth required.
	t.handleFunc("GET /api/v1/notifications", middleware.AuthRequired(d.db, handler.ListNotifications(d.db)))
	t.handleFunc("GET /api/v1/notifications/count", middleware.AuthRequired(d.db, handler.NotificationCount(d.db)))
	t.handleFunc("PATCH /api/v1/notifications/{id}/read", middleware.AuthRequired(d.db, handler.MarkNotificationRead(d.db)))
	t.handleFunc("POST /api/v1/notifications/read-all", middleware.AuthRequired(d.db, handler.MarkAllNotificationsRead(d.db)))
	t.handleFunc("DELETE /api/v1/notifications/{id}", middleware.AuthRequired(d.db, handler.DeleteNotification(d.db)))
	t.handleFunc("DELETE /api/v1/notifications", middleware.AuthRequired(d.db, handler.ClearNotifications(d.db)))
	t.handleFunc("GET /api/v1/notifications/preferences", middleware.AuthRequired(d.db, handler.GetNotificationPreferences(d.db, d.notifier)))
	t.handleFunc("PUT /api/v1/notifications/preferences", middleware.AuthRequired(d.db, handler.UpdateNotificationPreferences(d.db)))

	// Patch notification config — admin required on the patch.
	t.handleFunc("GET /api/v1/nodes/{slug}/notification-config", middleware.AuthRequired(d.db, middleware.RequireNodeRole(d.db, "admin")(handler.GetPatchNotifConfig(d.db))))
	t.handleFunc("PUT /api/v1/nodes/{slug}/notification-config", middleware.AuthRequired(d.db, middleware.RequireNodeRole(d.db, "admin")(handler.UpdatePatchNotifConfig(d.db))))

	// Activity feed — auth required.
	t.handleFunc("GET /api/v1/activity", middleware.AuthRequired(d.db, handler.UserActivityFeed(d.db)))

	// AP preview — admin only.
	t.handleFunc("GET /api/v1/nodes/{slug}/ap-preview", middleware.AdminRequired(d.db, handler.APPreview(d.db, d.cfg)))

	// Admin routes.
	// Export and wipe carry a step-up gate (docs/adr/017): export moves every
	// member's email address, wipe erases the instance including its audit
	// log. A month-old cookie is not sufficient proof of presence for either.
	t.handleFunc("GET /api/v1/admin/export", middleware.AdminRequired(d.db, middleware.SudoRequired(d.db, handler.AdminExport(d.db, d.cfg))))
	t.handleFunc("POST /api/v1/admin/tags", middleware.AdminRequired(d.db, handler.CreateTag(d.db)))
	t.handleFunc("PATCH /api/v1/admin/tags/{id}", middleware.AdminRequired(d.db, handler.UpdateTag(d.db)))
	t.handleFunc("DELETE /api/v1/admin/tags/{id}", middleware.AdminRequired(d.db, handler.DeleteTag(d.db)))
	// The suggested-tag review queue (docs/adr/114). Approve and reject live
	// here rather than on PATCH /admin/tags/{id}, which only sets a motif:
	// approving can rename, and renaming can merge two rows and re-point
	// every attachment.
	t.handleFunc("GET /api/v1/admin/tag-suggestions", middleware.AdminRequired(d.db, handler.ListTagSuggestions(d.db)))
	t.handleFunc("PATCH /api/v1/admin/tag-suggestions/{id}", middleware.AdminRequired(d.db, handler.DecideTagSuggestion(d.db)))
	t.handleFunc("DELETE /api/v1/nodes/{slug}/suggested-tags/{name}", middleware.AuthRequired(d.db, handler.WithdrawSuggestedTag(d.db)))
	t.handleFunc("GET /api/v1/admin/reports", middleware.AdminRequired(d.db, handler.ListReports(d.db)))
	t.handleFunc("PATCH /api/v1/admin/reports/{id}", middleware.AdminRequired(d.db, handler.UpdateReport(d.db)))
	t.handleFunc("GET /api/v1/admin/users", middleware.AdminRequired(d.db, handler.ListUsers(d.db)))
	t.handleFunc("PATCH /api/v1/admin/users/{id}", middleware.AdminRequired(d.db, handler.UpdateUser(d.db)))
	// The trust queue and the per-patch grant, beside the quilt-wide toggle
	// the PATCH above carries — the two scopes of one grant, answered on one
	// screen (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-
	// its-calendar.md, decisions 2 and 7).
	t.handleFunc("GET /api/v1/admin/trust-requests", middleware.AdminRequired(d.db, handler.ListTrustRequests(d.db)))
	t.handleFunc("PATCH /api/v1/admin/trust-requests/{id}", middleware.AdminRequired(d.db, handler.DecideTrustRequest(d.db)))
	t.handleFunc("POST /api/v1/admin/users/{id}/trusted-patches", middleware.AdminRequired(d.db, handler.GrantTrustedPatch(d.db)))
	t.handleFunc("DELETE /api/v1/admin/users/{id}/trusted-patches/{nodeId}", middleware.AdminRequired(d.db, handler.RevokeTrustedPatch(d.db)))
	// Setting an address points an account at a mailbox, and whoever holds
	// that mailbox can magic-link into it — the same shape as promotion, so
	// the same step-up gate (docs/adr/017), and its own route rather than a
	// field on the PATCH above (docs/adr/072).
	t.handleFunc("PUT /api/v1/admin/users/{id}/email", middleware.AdminRequired(d.db, middleware.SudoRequired(d.db, handler.SetUserEmail(d.db, d.cfg))))
	t.handleFunc("GET /api/v1/admin/audit-log", middleware.AdminRequired(d.db, handler.AuditLog(d.db)))
	// Signing an outsider's nonce as this quilt (docs/adr/087). Step-up
	// gated like the other actions that hand something outward: the blob
	// leaves the building and stands on its own for fifteen minutes.
	t.handleFunc("POST /api/v1/admin/attestation", middleware.AdminRequired(d.db, middleware.SudoRequired(d.db, handler.IssueAttestation(d.db, d.cfg))))
	// Archived patches: list + the only way back from archived (docs/adr/034).
	t.handleFunc("GET /api/v1/admin/nodes", middleware.AdminRequired(d.db, handler.AdminListNodes(d.db)))
	t.handleFunc("POST /api/v1/admin/nodes/{id}/restore", middleware.AdminRequired(d.db, handler.AdminRestoreNode(d.db)))
	// Overview (CONTEXT.md, "Overview"): what waits on the instance admin and
	// what is unattended. Never a size or growth figure.
	t.handleFunc("GET /api/v1/admin/overview", middleware.AdminRequired(d.db, handler.AdminOverview(d.db, d.cfg)))

	// Quilt settings (docs/adr/014): community identity + danger zone.
	// Native apps: the apps this domain vouches for
	// (docs/adr/2026-09-20-an-instance-vouches-for-an-app.md). Adding one is
	// step-up gated, because publishing an identifier lets that app ask a
	// phone for this domain's passkeys — the same class of act as setting an
	// account's email address (docs/adr/072). Removing one is not: taking
	// trust back is the safe direction.
	t.handleFunc("GET /api/v1/admin/native-apps", middleware.AdminRequired(d.db, handler.AdminListNativeApps(d.db, d.cfg)))
	t.handleFunc("POST /api/v1/admin/native-apps", middleware.AdminRequired(d.db, middleware.SudoRequired(d.db, handler.AdminAddNativeApp(d.db, d.wa))))
	t.handleFunc("DELETE /api/v1/admin/native-apps/{id}", middleware.AdminRequired(d.db, handler.AdminDeleteNativeApp(d.db, d.wa)))

	// Neighbor quilts: the instance's public adjacency list (docs/adr/024).
	t.handleFunc("GET /api/v1/admin/neighbor-quilts", middleware.AdminRequired(d.db, handler.AdminListNeighborQuilts(d.db)))
	t.handleFunc("POST /api/v1/admin/neighbor-quilts", middleware.AdminRequired(d.db, handler.AdminAddNeighborQuilt(d.db)))
	t.handleFunc("DELETE /api/v1/admin/neighbor-quilts/{id}", middleware.AdminRequired(d.db, handler.AdminDeleteNeighborQuilt(d.db)))

	t.handleFunc("GET /api/v1/admin/aggregators", middleware.AdminRequired(d.db, handler.AdminListAggregators(d.db)))
	t.handleFunc("POST /api/v1/admin/aggregators", middleware.AdminRequired(d.db, handler.AdminCreateAggregator(d.db)))
	t.handleFunc("PATCH /api/v1/admin/aggregators/{id}", middleware.AdminRequired(d.db, handler.AdminUpdateAggregator(d.db)))
	t.handleFunc("DELETE /api/v1/admin/aggregators/{id}", middleware.AdminRequired(d.db, handler.AdminDeleteAggregator(d.db)))
	t.handleFunc("POST /api/v1/admin/aggregators/{id}/sync", middleware.AdminRequired(d.db, handler.AdminSyncAggregator(d.db)))
	t.handleFunc("GET /api/v1/admin/aggregator-names", middleware.AdminRequired(d.db, handler.AdminListUnroutedNames(d.db)))
	t.handleFunc("POST /api/v1/admin/aggregator-names/ignore", middleware.AdminRequired(d.db, handler.AdminIgnoreName(d.db, true)))
	t.handleFunc("POST /api/v1/admin/aggregator-names/unignore", middleware.AdminRequired(d.db, handler.AdminIgnoreName(d.db, false)))
	t.handleFunc("GET /api/v1/admin/aggregator-listings", middleware.AdminRequired(d.db, handler.AdminListNameListings(d.db)))
	t.handleFunc("GET /api/v1/admin/programs", middleware.AdminRequired(d.db, handler.AdminListPrograms(d.db)))

	t.handleFunc("GET /api/v1/admin/settings", middleware.AdminRequired(d.db, handler.AdminGetSettings(d.db, d.cfg)))
	t.handleFunc("PATCH /api/v1/admin/settings", middleware.AdminRequired(d.db, handler.AdminUpdateSettings(d.db, d.cfg)))
	// Usage counts (docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md).
	t.handleFunc("GET /api/v1/admin/usage", middleware.AdminRequired(d.db, handler.AdminUsage(d.db)))
	t.handleFunc("DELETE /api/v1/admin/usage", middleware.AdminRequired(d.db, handler.AdminClearUsage(d.db, d.usage)))
	t.handleFunc("GET /api/v1/admin/legal", middleware.AdminRequired(d.db, handler.AdminGetLegal(d.db, d.cfg)))
	t.handleFunc("PUT /api/v1/admin/legal/{doc}", middleware.AdminRequired(d.db, handler.AdminUpdateLegal(d.db)))
	t.handleFunc("DELETE /api/v1/admin/legal/{doc}", middleware.AdminRequired(d.db, handler.AdminResetLegal(d.db)))
	t.handleFunc("GET /api/v1/admin/label", middleware.AdminRequired(d.db, handler.AdminGetLabel(d.db)))
	t.handleFunc("PATCH /api/v1/admin/label", middleware.AdminRequired(d.db, handler.AdminUpdateLabel(d.db)))
	t.handleFunc("PUT /api/v1/admin/label/costs", middleware.AdminRequired(d.db, handler.AdminPutLabelCosts(d.db)))
	t.handleFunc("POST /api/v1/admin/label/stewards", middleware.AdminRequired(d.db, handler.AdminAddLabelSteward(d.db)))
	t.handleFunc("DELETE /api/v1/admin/label/stewards/{id}", middleware.AdminRequired(d.db, handler.AdminRemoveLabelSteward(d.db)))
	t.handleFunc("POST /api/v1/admin/wipe", middleware.AdminRequired(d.db, middleware.SudoRequired(d.db, handler.AdminWipe(d.db, d.cfg))))

	// Unclaimed patches: community submissions + admin management.
	t.handleFunc("POST /api/v1/submissions", middleware.AuthRequired(d.db, handler.SubmitPatch(d.db, d.cfg)))
	t.handleFunc("POST /api/v1/admin/unclaimed", middleware.AdminRequired(d.db, handler.CreateUnclaimedPatch(d.db)))
	t.handleFunc("POST /api/v1/admin/unclaimed/bulk", middleware.AdminRequired(d.db, handler.BulkCreateUnclaimed(d.db)))
	t.handleFunc("GET /api/v1/admin/submissions", middleware.AdminRequired(d.db, handler.ListSubmissions(d.db)))
	t.handleFunc("PATCH /api/v1/admin/submissions/{id}", middleware.AdminRequired(d.db, handler.ReviewSubmission(d.db)))
	t.handleFunc("GET /api/v1/admin/event-submissions", middleware.AdminRequired(d.db, handler.ListAdminEventSubmissions(d.db)))
	t.handleFunc("POST /api/v1/nodes/{slug}/claim", middleware.AuthRequired(d.db, handler.RequestClaim(d.db, d.cfg)))
	t.handleFunc("GET /api/v1/nodes/{slug}/claims/mine", middleware.AuthRequired(d.db, handler.MyClaim(d.db, d.cfg)))
	// Every open claim the caller holds, across patches — My Patches is built
	// from memberships and an approved claimant has none yet (docs/adr/039).
	t.handleFunc("GET /api/v1/users/me/claims", middleware.AuthRequired(d.db, handler.MyClaims(d.db)))
	t.handleFunc("POST /api/v1/claims/{id}/verify", middleware.AuthRequired(d.db, handler.VerifyClaim(d.db)))
	t.handleFunc("POST /api/v1/claims/{id}/withdraw", middleware.AuthRequired(d.db, handler.WithdrawClaim(d.db)))
	t.handleFunc("POST /api/v1/claims/{id}/resend-email", middleware.AuthRequired(d.db, handler.ResendClaimEmail(d.db, d.cfg)))
	t.handleFunc("POST /api/v1/claims/{id}/setup", middleware.AuthRequired(d.db, handler.SetupClaim(d.db)))
	// Email-claim link landing: no auth — possessing the token is the proof
	// (docs/adr/030). GET is read-only; completion requires the POST.
	t.handleFunc("GET /api/v1/claims/verify-email", handler.EmailClaimInfo(d.db))
	t.handleFunc("POST /api/v1/claims/verify-email", handler.CompleteEmailClaim(d.db))
	t.handleFunc("GET /api/v1/admin/claims", middleware.AdminRequired(d.db, handler.ListClaims(d.db)))
	t.handleFunc("PATCH /api/v1/admin/claims/{id}", middleware.AdminRequired(d.db, handler.ReviewClaim(d.db)))
	t.handleFunc("POST /api/v1/admin/nodes/{slug}/assign", middleware.AdminRequired(d.db, handler.AdminAssignOwner(d.db)))
	t.handleFunc("PATCH /api/v1/admin/nodes/{slug}/verification-domain", middleware.AdminRequired(d.db, handler.AdminSetVerificationDomain(d.db)))

	// Governance templates + overview.
	t.handleFunc("GET /api/v1/templates/{id}", handler.GetTemplate())
	t.handleFunc("GET /api/v1/nodes/{slug}/governance/overview", middleware.AuthOptional(d.db, handler.GovernanceOverview(d.db)))
	// What the patch has decided, in order (docs/adr/055). Assembled from
	// proposals and attestations rather than stored, so it needs no auth of
	// its own beyond what those already carry.
	t.handleFunc("GET /api/v1/nodes/{slug}/governance/record", middleware.AuthOptional(d.db, handler.GovernanceRecord(d.db)))

	// Federation surface — honor the federation.enabled config toggle.
	// Keypair/ap_id backfill above stays unconditional so enabling later
	// is seamless.
	if d.cfg.Federation.Enabled {
		// ActivityPub endpoints.
		t.handleFunc("GET /ap/users/{id}", handler.APUser(d.db))
		t.handleFunc("GET /ap/users/{id}/outbox", handler.APUserOutbox(d.db))
		t.handleFunc("GET /ap/users/{id}/followers", handler.APUserFollowers(d.db))
		t.handleFunc("GET /ap/nodes/{id}", handler.APNode(d.db))
		t.handleFunc("GET /ap/nodes/{id}/outbox", handler.APNodeOutbox(d.db))
		t.handleFunc("GET /ap/nodes/{id}/followers", handler.APNodeFollowers(d.db))
		t.handleFunc("GET /ap/events/{id}", handler.APEvent(d.db))
		t.handleFunc("GET /ap/proposals/{id}", handler.APProposal(d.db))
		t.handleFunc("GET /ap/governance/{id}", handler.APGovernanceDoc(d.db))

		// AP Inbox endpoints (receive activities from remote instances).
		t.handleFunc("POST /ap/users/{id}/inbox", handler.APUserInbox(d.db))
		t.handleFunc("POST /ap/nodes/{id}/inbox", handler.APNodeInbox(d.db))

		// Instance service actor (docs/adr/024): relays cross-quilt
		// follows; its inbox receives Accepts and followed patches'
		// broadcasts.
		t.handleFunc("GET /ap/instance", handler.APInstanceActor(d.db, d.cfg))
		t.handleFunc("POST /ap/instance/inbox", handler.APInstanceInbox(d.db))

		// WebFinger.
		t.handleFunc("GET /.well-known/webfinger", handler.WebFinger(d.db))

		// Git smart HTTP for governance repos (federation transport).
		// Uses a wrapper that only handles /governance.git/ paths, passing through otherwise.
		//
		// AuthOptional, and not anonymous: a clone takes the whole repo,
		// members-only charters included, so the transport carries the
		// whole-shelf gate (docs/adr/110) and needs to know who is asking.
		gitHandler := governance.GitHTTPHandler(handler.GovernanceRepoNodeID(d.db))
		t.handleFunc("GET /api/v1/nodes/{slug}/governance.git/info/refs", middleware.AuthOptional(d.db, gitHandler.ServeHTTP))
		t.handleFunc("POST /api/v1/nodes/{slug}/governance.git/git-upload-pack", middleware.AuthOptional(d.db, gitHandler.ServeHTTP))
	} else {
		log.Println("federation: disabled (federation.enabled=false) — AP, WebFinger, and git transport not mounted")
	}

	// robots.txt: opt out of AI training/scraping crawlers by name. Paired
	// with the BlockAICrawlers middleware in main's stack for the ones that
	// ignore it.
	t.handleFunc("GET /robots.txt", middleware.RobotsTxt())

	// Legacy /pins/{id} URLs (the retired UI word for events — docs/adr/027)
	// were federated into other instances' timelines; redirect them forever.
	t.handleFunc("GET /pins/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/events/"+r.PathValue("id"), http.StatusMovedPermanently)
	})

	// SPA: serve web/dist/ for everything else.
	t.handle("/", d.spa)

	return t.mux, t.routes
}
