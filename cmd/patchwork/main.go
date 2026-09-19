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
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/eventsource"
	"github.com/patchwork-toolkit/patchwork/internal/gazetteer"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
	"github.com/patchwork-toolkit/patchwork/web"
)

func main() {
	configPath := flag.String("config", "patchwork.yaml", "path to config file")
	healthcheck := flag.Bool("healthcheck", false, "probe the running instance's health endpoint and exit 0 (healthy) or 1")
	verifyAttestation := flag.String("verify-attestation", "", "check an admin attestation blob (docs/adr/087) and exit 0 if it stands")
	attestationKey := flag.String("attestation-key", "", "PEM public key file to check -verify-attestation against, instead of fetching it from the claimed domain")
	repairGovernance := flag.Bool("repair-governance", false, "rebuild governance repos from the database, print a summary, and exit (run with the server stopped)")
	flag.Parse()

	// Probe mode: no database, no server. Used by the image's HEALTHCHECK.
	if *healthcheck {
		runHealthcheck(*configPath)
		return
	}

	// Verifier mode: no config, no database, no server. This is the side of
	// the exchange that does *not* run a quilt — a host or a directory that
	// happens to have the binary.
	if *verifyAttestation != "" {
		runVerifyAttestation(*verifyAttestation, *attestationKey)
		return
	}

	// Repair mode: database and repos, no server (docs/adr/084).
	if *repairGovernance {
		runGovernanceRepair(*configPath)
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
	}

	// The instance service actor relays remote-patch Follows for all local
	// users (docs/adr/024) — ensure it exists and its ap_id matches the
	// configured domain.
	//
	// Outside the federation gate, unlike the AP-ID backfills above. Its
	// keypair is also what signs an admin's proof of the role (docs/adr/087),
	// and that has to work on a quilt that never federates: a key minted only
	// when federation is on is a proof that disappears the day an instance
	// turns federation off. Minting it costs one RSA keypair, once, on a
	// database that has never had one.
	if err := ap.EnsureInstanceActor(db, ap.GetDomain()); err != nil {
		log.Printf("warning: failed to ensure instance actor: %v", err)
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

	// Move migration 062's contact cards into the item shape docs/adr/083
	// gives them. Safe to clear the legacy columns as it reads them only
	// because no handler reads them any more: the Me endpoints, the Members
	// room and the profile all serve contact_items. Fatal rather than
	// warn-and-continue — a half-converted card is a phone number in two
	// places with two different audiences, and the conversion is one
	// transaction, so failing here leaves the old shape intact.
	if n, err := handler.BackfillContactItems(db); err != nil {
		log.Fatalf("contact items backfill: %v", err)
	} else if n > 0 {
		log.Printf("contact: converted %d card fields into items", n)
	}

	// Create the repos that are absent, from the canonical DB rows — a patch
	// whose repo creation failed at runtime, and every patch on an instance
	// restored from a database backup alone, which carries no repos at all
	// (docs/adr/084). Strictly create-missing: a repo that is already there is
	// never written into on a boot. Repairing one that exists but has drifted
	// is `patchwork -repair-governance`, an operator's decision.
	if n, err := handler.BackfillNodeGovernanceRepos(db); err != nil {
		log.Fatalf("governance backfill: %v", err)
	} else if n > 0 {
		log.Printf("governance: rebuilt repos for %d patches from the database", n)
	}

	// Fill the governance_config cache for nodes created while CreateNode
	// forked rules without syncing them — the rules in force become readable
	// from the DB, which is what makes admin-decides patches actually behave
	// as admin-decides (docs/adr/041). Warn-and-continue: a partial backfill
	// leaves the affected nodes on the voting defaults they already had.
	if n, err := handler.BackfillGovernanceConfig(db); err != nil {
		log.Printf("warning: governance config backfill: %v", err)
	} else if n > 0 {
		log.Printf("governance: synced rules for %d nodes", n)
	}

	// One-time pass for unclaimed patches created before migration 031:
	// derive verification domains from admin-supplied websites (docs/adr/030).
	handler.BackfillVerificationDomains(db)

	// Decode HTML entities in imported event text ("Lanc Workshop &amp; Tool
	// Library"). The reader fix only reaches listings a sync still finds; a
	// dropped or past listing keeps what it was imported with. Warn-and-
	// continue: encoded text reads badly, it doesn't break anything.
	if n, err := eventsource.HealEncodedEntities(db); err != nil {
		log.Printf("warning: entity heal: %v", err)
	} else if n > 0 {
		log.Printf("events: decoded HTML entities in %d stored fields", n)
	}

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

	// Bring every patch's lining to the current shipped text (docs/adr/037):
	// create missing linings, auto-update stale ones. Diverged linings are
	// never touched. Runs after the repo backfill (so git mirrors land) and
	// after SetNotifier (so lining.updated notifications aren't dropped).
	if created, updatedLinings, err := handler.AutoUpdateLinings(db); err != nil {
		log.Fatalf("lining auto-update: %v", err)
	} else if created > 0 || updatedLinings > 0 {
		log.Printf("lining: created %d, auto-updated %d to v%d", created, updatedLinings, governance.CurrentLiningVersion())
	}
	// Close the follower access to members-only charters that shipped on by
	// default (docs/adr/116), in the rules file as well as the row, and tell
	// each patch's admins. Same placement and same reasons as the lining pass
	// above: after the repo backfill so the git write lands, after SetNotifier
	// so the notice is not dropped. Idempotent, so it costs one query per boot
	// once it has run.
	if closed, err := handler.CloseFollowerChartersDefault(db); err != nil {
		log.Fatalf("follower charters default: %v", err)
	} else if closed > 0 {
		log.Printf("follower charters: closed the shipped default on %d patch(es)", closed)
	}

	reminderCtx, reminderCancel := context.WithCancel(context.Background())
	defer reminderCancel()
	notifications.StartReminderWorker(reminderCtx, notifier)

	// Votes move on a calendar, not on a person: an election's nominations
	// close and voting opens, voting ends and the council is seated
	// (docs/adr/051); an ordinary proposal's window closes and it resolves
	// or lapses (docs/adr/097). Hourly is plenty — the windows are days long.
	sweepCtx, sweepCancel := context.WithCancel(context.Background())
	defer sweepCancel()
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		sweep := func() {
			handler.SweepElections(db)
			handler.SweepProposals(db)
			// After the windows that closed have closed: the people a still-
			// open vote is waiting on, told once each (docs/adr/093).
			handler.SweepVoteNotices(db)
		}
		sweep()
		for {
			select {
			case <-sweepCtx.Done():
				return
			case <-ticker.C:
				sweep()
			}
		}
	}()

	// Tell the feed parsers what a zoneless time in a calendar means,
	// before anything reads one (docs/adr/065). Set here rather than
	// read from config inside the parsers so the sync worker and the
	// handlers' "sync now" agree by construction.
	// The bottom rung of the chain an event's zone resolves through:
	// event → patch → instance → UTC (docs/adr/045). Recorded before
	// anything serves a request or syncs a feed. The admin's override
	// lives in instance_settings and is read per request, so it takes
	// effect without a restart; this is only the configured default.
	settings.SetTimezoneDefault(cfg.Timezone())
	log.Printf("config: this quilt keeps time in %s", settings.EffectiveTimezone(db))

	// Start the event source worker: hourly re-sync of every attached
	// calendar feed (docs/adr/031).
	sourceCtx, sourceCancel := context.WithCancel(context.Background())
	defer sourceCancel()
	eventsource.StartWorker(sourceCtx, db, notifier)

	// Usage counts: in memory, written as daily totals once a minute
	// (docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md).
	// The switch is read per page load, so the admin's change needs no
	// restart.
	usage := middleware.NewUsageCounter(db, func() bool { return settings.UsageStatsEnabled(db) })
	usageCtx, usageCancel := context.WithCancel(context.Background())
	defer usageCancel()
	usage.Start(usageCtx)

	// First-run bootstrap notice: until an account exists there is no admin,
	// so tell the operator how to claim the instance.
	if auth.NoUsersExist(db) {
		// The first account is claimed with a token, not raced for
		// (docs/adr/070). A configured token comes from a provisioning
		// layer that already handed it to the operator; otherwise generate
		// one and print it, since this log is already the bootstrap channel
		// — it is where magic links go without SMTP, so the operator is the
		// one reader guaranteed to have it.
		token := cfg.Instance.BootstrapToken
		generated := token == ""
		if generated {
			var err error
			if token, err = auth.GenerateBootstrapToken(); err != nil {
				log.Fatalf("first run: could not generate a bootstrap token: %v", err)
			}
		}
		auth.SetBootstrapToken(token)

		log.Println("first run: no accounts exist yet — the first account created will become the instance admin")
		log.Printf("first run: sign in at https://%s/login (without SMTP, the magic link prints to this log)", cfg.Instance.Domain)
		if generated {
			log.Printf("first run: bootstrap token %s — the signup form asks for it; it dies with the first account", token)
			log.Println("first run: set instance.bootstrap_token or PATCHWORK_BOOTSTRAP_TOKEN to choose your own (it changes on every restart until then)")
		} else {
			log.Println("first run: bootstrap token read from configuration — the signup form asks for it")
		}
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

	// The gazetteer is optional infrastructure (docs/adr/082): a place index
	// built offline and copied in. Absent, the server runs exactly as it did
	// before it existed and every address is placed by hand.
	//
	// How loudly a failure is reported depends on whether the admin named the
	// file. An explicit path that will not open is a mistake worth a warning;
	// the conventional path beside the database is a convention, and its
	// absence is the normal state of most instances.
	gazPath, gazExplicit := cfg.GazetteerPath()
	gaz, err := gazetteer.Open(gazPath)
	switch {
	case err == nil:
		log.Printf("gazetteer: %d places from %s", gaz.Count(), gazPath)
		defer gaz.Close()
	case gazExplicit:
		log.Printf("warning: gazetteer %s could not be opened, address suggestions are off: %v", gazPath, err)
		gaz = nil
	default:
		gaz = nil
	}

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

	// Build router. The registrations live in routes.go, as a table a test
	// can walk: authorization is decided inside the handlers, so enumerating
	// the routes is the only way to ask every one of them whether it refuses
	// a caller who holds nothing (routes_auth_test.go).
	//
	// Visitor counting wraps the SPA alone, so it sees a page being loaded
	// and never an API call or an asset. It counts nothing until the admin
	// turns it on (docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md).
	mux, _ := buildRoutes(serverDeps{
		db:       db,
		cfg:      cfg,
		wa:       wa,
		gaz:      gaz,
		notifier: notifier,
		usage:    usage,
		spa:      usage.Wrap(seoWrapped),
	})

	// Middleware stack: BlockAICrawlers → Compress → CORS → CSRF → routes.
	// BlockAICrawlers is outermost so matching crawlers are rejected before
	// any other work; federation, preview, and search agents pass through.
	// Compress sits above everything that writes a body — the SPA bundle and
	// the JSON alike — and below the crawler gate, which writes none.
	var root http.Handler = mux
	root = middleware.CSRF(root)
	root = middleware.CORS(cfg, root)
	root = middleware.Compress(root)
	root = middleware.BlockAICrawlers(root)

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

	// Everything under /assets/ carries a content hash in its name, so the
	// bytes behind a given URL never change: a new build is a new name. That
	// makes them cacheable for as long as a browser cares to, which is the
	// difference between a returning visitor revalidating two megabytes and
	// fetching nothing at all. index.html is deliberately not in here — it is
	// the file that names the current hashes.
	if strings.HasPrefix(path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	http.FileServer(h.fs).ServeHTTP(w, r)
}
