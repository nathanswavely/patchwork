package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

func main() {
	dbPath := flag.String("db", "data/patchwork.db", "path to SQLite database")
	force := flag.Bool("force", false, "wipe existing seed data and re-seed")
	flag.Parse()

	profile := artsProfile()

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		log.Fatalf("migrations fs: %v", err)
	}

	db, err := database.Open(*dbPath, migrations)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	// Set up AP domain and governance data dir
	ap.SetDomain("localhost")
	dataDir := filepath.Dir(*dbPath)
	governance.SetDataDir(dataDir)

	// Initialize instance governance repo
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		log.Printf("warning: governance init: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE email = 'admin@localhost'").Scan(&count)
	if err != nil {
		log.Fatalf("check existing: %v", err)
	}

	// The seed is for empty dev databases only (docs/adr/009 — it must never
	// bootstrap a real instance). A database with users but no seed marker
	// (admin@localhost) holds real data: refuse, even with -force. The
	// system sentinel user (created by migration 015) doesn't count.
	var totalUsers int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE id != '00000000-0000-0000-0000-000000000000'").Scan(&totalUsers); err != nil {
		log.Fatalf("check existing: %v", err)
	}
	if totalUsers > 0 && count == 0 {
		log.Fatalf("refusing to seed: %s contains %d users and no seed marker (admin@localhost). The seed is demo fiction for empty dev databases, never for a live instance (docs/adr/009).", *dbPath, totalUsers)
	}

	if count > 0 && !*force {
		fmt.Println("Seed data already exists. Use -force to wipe and re-seed.")
		return
	}

	if *force {
		log.Println("Wiping existing data...")
		wipe(db, dataDir)
	}

	log.Println("Seeding database...")
	s := newSeeder(db, dataDir, profile)
	s.run()

	// Create dev sessions for all test users so E2E tests can authenticate as different roles.
	devUsers := []struct{ email, token string }{
		{"admin@localhost", "dev-admin-token"},
		{"organizer@localhost", "dev-organizer-token"},
		{"active@localhost", "dev-active-token"},
		{"lurker@localhost", "dev-lurker-token"},
		{"new@localhost", "dev-new-token"},
		{"joiner@localhost", "dev-joiner-token"},
	}
	expiresAt := time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	for _, u := range devUsers {
		var userID string
		if err := db.QueryRow("SELECT id FROM users WHERE email = ?", u.email).Scan(&userID); err != nil {
			log.Printf("warning: could not find %s for dev session: %v", u.email, err)
			continue
		}
		// The cookie value stays the fixed dev token; only its hash is stored.
		tokenHash := auth.HashToken(u.token)
		db.Exec("DELETE FROM sessions WHERE token = ?", tokenHash)
		if _, err := db.Exec(
			"INSERT INTO sessions (id, user_id, token, expires_at, ip_address) VALUES (?, ?, ?, ?, ?)",
			auth.NewUUIDv7(), userID, tokenHash, expiresAt, "127.0.0.1",
		); err != nil {
			log.Printf("warning: could not create dev session for %s: %v", u.email, err)
		}
	}

	fmt.Printf(`
Seed complete:
  Users:         %d
  Nodes:         %d
  Events:        %d
  Memberships:   %d
  Proposals:     %d (%d open, %d approved, %d rejected, %d withdrawn)
  Comments:      %d
  Reactions:     %d
  Revisions:     %d
  Gov Docs:      %d
  Reports:       %d
  Notifications: %d
  AP Followers:  %d
  Gov Configs:   set on all %d nodes

  Dev admin session token: dev-admin-token
  To use in browser console:
    document.cookie = "patchwork_session=dev-admin-token; path=/; max-age=31536000"
`,
		s.stats.users,
		s.stats.nodes,
		s.stats.events,
		s.stats.memberships,
		s.stats.proposals, s.stats.openProposals, s.stats.approvedProposals, s.stats.rejectedProposals, s.stats.withdrawnProposals,
		s.stats.comments,
		s.stats.reactions,
		s.stats.revisions,
		s.stats.govDocs,
		s.stats.reports,
		s.stats.notifications,
		s.stats.apFollowers,
		s.stats.nodes,
	)
}

func wipe(db *database.DB, dataDir string) {
	tables := []string{
		"claim_requests",
		"ap_followers", "ap_outbox_queue",
		"comment_reactions", "proposal_comments", "proposal_revisions",
		"notifications", "votes", "content_reports", "audit_log",
		"governance_docs", "proposals", "events", "memberships",
		"node_tags", "tags", "nodes",
		"sessions", "credentials", "magic_links", "invite_links",
		"users",
	}
	for _, t := range tables {
		if _, err := db.Exec("DELETE FROM " + t); err != nil {
			log.Printf("warning: wipe %s: %v", t, err)
		}
	}

	// Clean governance repos
	os.RemoveAll(filepath.Join(dataDir, "governance"))
}

type stats struct {
	users, nodes                                int
	events, memberships                         int
	proposals, openProposals, approvedProposals int
	rejectedProposals, withdrawnProposals       int
	comments, reactions, revisions              int
	govDocs, reports, notifications             int
	apFollowers                                 int
}

type seeder struct {
	db      *database.DB
	dataDir string
	stats   stats
	rng     *rand.Rand
	now     time.Time
	profile profileData

	userIDs []string
	// nodeID -> slug mapping
	nodeIDs   []string
	nodeSlugs map[string]string
	// nodeID -> list of member userIDs (active)
	nodeMembers map[string][]string
}

func newSeeder(db *database.DB, dataDir string, profile profileData) *seeder {
	return &seeder{
		db:          db,
		dataDir:     dataDir,
		rng:         rand.New(rand.NewSource(42)),
		now:         time.Now().UTC(),
		profile:     profile,
		nodeSlugs:   make(map[string]string),
		nodeMembers: make(map[string][]string),
	}
}

func (s *seeder) run() {
	s.seedUsers()
	s.seedTags()
	s.seedNodes()
	s.seedMemberships()
	s.seedDevPersonas()
	s.seedJoinFlowPersona()
	s.seedUnclaimedPatches()
	s.seedExtraMemberships()
	s.seedEvents()
	s.seedProposals()
	s.seedComments()
	s.seedRevisions()
	s.seedGovernanceDocs()
	s.seedReports()
	s.seedNotifications()
	s.seedAuditLog()
	s.seedFederationData()
}

func (s *seeder) ts(daysAgo int) string {
	t := s.now.AddDate(0, 0, -daysAgo)
	h := s.rng.Intn(14) + 8
	m := s.rng.Intn(60)
	t = time.Date(t.Year(), t.Month(), t.Day(), h, m, 0, 0, time.UTC)
	return t.Format("2006-01-02T15:04:05.000Z")
}

func (s *seeder) futureTS(daysFromNow int) string {
	t := s.now.AddDate(0, 0, daysFromNow)
	h := s.rng.Intn(10) + 10
	m := s.rng.Intn(60)
	t = time.Date(t.Year(), t.Month(), t.Day(), h, m, 0, 0, time.UTC)
	return t.Format("2006-01-02T15:04:05.000Z")
}

func (s *seeder) pick(items []string) string {
	return items[s.rng.Intn(len(items))]
}

// ---------------------------------------------------------------------------
// Seed profiles — pluggable data tables. The seeding machinery below (all
// the seedXxx methods) is profile-agnostic; only these data tables differ
// between "arts" (default) and "music". Dev tokens (dev-admin-token etc.)
// are created in main() from fixed emails and are identical across profiles.
// ---------------------------------------------------------------------------

// profileData holds every hand-authored data table a profile contributes.
// Generic/mechanical seed steps (dev personas, comment threads, revision
// notes, audit log, federation followers) stay shared and are not part of
// this struct.
type profileData struct {
	users              []userDef
	tags               []string
	nodes              []nodeDef
	events             []eventDef
	proposals          []proposalDef
	govDocs            []govDocDef
	notifications      []notifDef
	unclaimed          []unclaimedDef
	pendingSubmissions []unclaimedDef
	// extraMemberships wires curated membership overlap that the generic
	// random join pass can't express — e.g. invite-only bands where every
	// member must be an admin, or specific musicians following specific
	// venues so the affinity threads tell a coherent story.
	extraMemberships []extraMembershipDef
}

// extraMembershipDef assigns a specific user (by index into profileData.users)
// a specific role on a specific node (by slug), inserted after the generic
// random membership pass and after unclaimed patches exist.
type extraMembershipDef struct {
	userIdx  int
	nodeSlug string
	role     string // "admin", "member", or "follower"
}

type govDocDef struct {
	nodeSlug string
	title    string
	body     string
}

type notifDef struct {
	userIdx int
	nType   string
	title   string
	body    string
	link    string
	read    bool
}

type unclaimedDef struct {
	name             string
	desc             string
	website          string
	links            []nodeLink
	tags             []string
	lat              float64
	lng              float64
	address          string
	submissionSource string // defaults to "admin" if empty (matches admin-created unclaimed patches)
	submitterIdx     *int   // index into profileData.users; set for submission_source "community"
}

// ---------------------------------------------------------------------------
// Users — 30 realistic Lancaster community members
// ---------------------------------------------------------------------------

type userDef struct {
	email       string
	username    string
	displayName string
	bio         string
	role        string
	daysAgo     int
}

func (s *seeder) seedUsers() {
	users := s.profile.users

	for _, u := range users {
		id := auth.NewUUIDv7()
		createdAt := s.ts(u.daysAgo)
		apID := ap.UserAPID(ap.GetDomain(), id)
		_, err := s.db.Exec(`INSERT INTO users (id, email, username, display_name, bio, role, created_at, updated_at, ap_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, u.email, u.username, u.displayName, u.bio, u.role, createdAt, createdAt, apID)
		if err != nil {
			log.Fatalf("seed user %s: %v", u.username, err)
		}
		s.userIDs = append(s.userIDs, id)
	}
	s.stats.users = len(users)
}

// ---------------------------------------------------------------------------
// Tags
// ---------------------------------------------------------------------------

// seedTagMotifs pairs each seeded tag with its motif, mirroring what the
// 026 migration backfilled for pre-existing vocabularies. The seeder runs
// after migrations on a fresh database, so it must set motifs itself.
var seedTagMotifs = map[string]string{
	"visual-arts": "palette",
	"gallery":     "images",
	"music":       "musicNotes",
	"venue":       "buildings",
	"theater":     "maskHappy",
	"dance":       "sneaker",
	"film":        "filmSlate",
	"literary":    "bookOpen",
	"food":        "forkKnife",
	"craft":       "scissors",
	"community":   "usersThree",
	"education":   "gradCap",
	"sports":      "soccerBall",
	"tech":        "code",
	"wellness":    "heartbeat",
	"ceramics":    "paintBrush",
	"printmaking": "stamp",
	"radio":       "radio",
}

func (s *seeder) seedTags() {
	tags := s.profile.tags
	for _, name := range tags {
		id := auth.NewUUIDv7()
		var motif interface{}
		if m := seedTagMotifs[name]; m != "" {
			motif = m
		}
		_, err := s.db.Exec("INSERT INTO tags (id, name, motif) VALUES (?, ?, ?)", id, name, motif)
		if err != nil {
			log.Fatalf("seed tag %s: %v", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Nodes — 25 patches representing Lancaster's grassroots ecosystem
// ---------------------------------------------------------------------------

type nodeDef struct {
	name             string
	slug             string
	description      string
	ownerIdx         int
	tags             []string
	lat, lng         float64
	membershipPolicy string
	address          string
	palette          string
	block            string
	// draftBlock is raw JSON for a drafted block (docs/adr/029); when set
	// it replaces palette/block entirely and bundle supplies the fabrics.
	draftBlock string
	bundle     []string
	website    string
	links            []nodeLink
	followerPerms    *followerPerms
}

type followerPerms struct {
	Events    bool `json:"events"`
	Proposals bool `json:"proposals"`
	Charters  bool `json:"charters"`
	Members   bool `json:"members"`
}

type nodeLink struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

func (s *seeder) seedNodes() {
	nodes := s.profile.nodes

	// Available palettes for auto-assignment.
	palettes := []string{
		"adolescents", "pinkRazors", "greatestSongs", "allroysRevenge",
		"anthem", "allTheShoes", "bottlesToTheGround", "liberalAnimation",
	}

	slugToID := make(map[string]string)
	for i, n := range nodes {
		id := auth.NewUUIDv7()
		slugToID[n.slug] = id

		// Use explicit palette if set, otherwise cycle. Block stays unset
		// (hash-assigned) unless the node def pins one.
		palette := n.palette
		if palette == "" {
			palette = palettes[i%len(palettes)]
		}
		appearance := map[string]interface{}{"palette": palette}
		if n.block != "" {
			appearance["block"] = n.block
		}
		if n.draftBlock != "" {
			// A drafted block (docs/adr/029) replaces the curated pick.
			appearance = map[string]interface{}{"block": json.RawMessage(n.draftBlock)}
			if len(n.bundle) > 0 {
				appearance["bundle"] = n.bundle
			}
		}
		appearanceJSON, _ := json.Marshal(appearance)

		apID := ap.NodeAPID(ap.GetDomain(), id)

		// Assign governance config based on membership policy
		var gcJSON string
		switch n.membershipPolicy {
		case "open":
			gcJSON = `{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,"amendment_threshold":"majority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":0}`
		case "approval_required":
			gcJSON = `{"decision_method":"majority","quorum_percent":25,"default_vote_duration_hours":168,"amendment_threshold":"supermajority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":7}`
		case "invite_only":
			gcJSON = `{"decision_method":"consensus","quorum_percent":50,"default_vote_duration_hours":336,"amendment_threshold":"consensus","amendment_auto_apply":false,"succession_policy":"longest_tenure","min_voting_tenure_days":30}`
		}

		createdAt := s.ts(s.rng.Intn(90) + 90)

		linksJSON := "[]"
		if len(n.links) > 0 {
			b, _ := json.Marshal(n.links)
			linksJSON = string(b)
		}

		var fpJSON *string
		if n.followerPerms != nil {
			b, _ := json.Marshal(n.followerPerms)
			s := string(b)
			fpJSON = &s
		}

		_, err := s.db.Exec(`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, visibility, membership_policy, appearance, created_at, updated_at, status, ap_id, governance_config, website, links, follower_permissions)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'public', ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?)`,
			id, s.userIDs[n.ownerIdx], n.name, n.slug, n.description,
			n.lat, n.lng, n.address, n.membershipPolicy, string(appearanceJSON), createdAt, createdAt, apID, gcJSON, n.website, linksJSON, fpJSON)
		if err != nil {
			log.Fatalf("seed node %s: %v", n.slug, err)
		}

		// Fork governance repo for this node — map membership policy to template.
		templateName := "casual"
		switch n.membershipPolicy {
		case "invite_only":
			templateName = "minimal"
		case "approval_required":
			templateName = "collaborative"
		}
		if err := governance.ForkForNode(s.dataDir, id, templateName); err != nil {
			log.Printf("warning: governance fork for %s: %v", n.slug, err)
		}

		s.db.Exec("UPDATE nodes SET governance_setup_complete = TRUE WHERE id = ?", id)

		s.nodeIDs = append(s.nodeIDs, id)
		s.nodeSlugs[id] = n.slug

		for pos, tagName := range n.tags {
			var tagID string
			err := s.db.QueryRow("SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID)
			if err != nil {
				log.Fatalf("find tag %s: %v", tagName, err)
			}
			// Position is the priority order: the profile lists each patch's
			// defining tag first, and that tag derives the motif.
			s.db.Exec("INSERT INTO node_tags (node_id, tag_id, position) VALUES (?, ?, ?)", id, tagID, pos)
		}
	}
	s.stats.nodes = len(nodes)
}

// ---------------------------------------------------------------------------
// Memberships — rich overlap creates meaningful affinity threads
// ---------------------------------------------------------------------------

func (s *seeder) seedMemberships() {
	// Node owners get admin membership.
	type ownerInfo struct {
		nodeID  string
		ownerID string
	}
	rows, err := s.db.Query("SELECT id, owner_id FROM nodes")
	if err != nil {
		log.Fatalf("query nodes for memberships: %v", err)
	}
	var owners []ownerInfo
	for rows.Next() {
		var o ownerInfo
		rows.Scan(&o.nodeID, &o.ownerID)
		owners = append(owners, o)
	}
	rows.Close()

	inserted := make(map[string]bool)

	for _, o := range owners {
		key := o.ownerID + ":" + o.nodeID
		if inserted[key] {
			continue
		}
		id := auth.NewUUIDv7()
		_, err := s.db.Exec(`INSERT INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, 'admin', 'active', ?)`,
			id, o.ownerID, o.nodeID, s.ts(s.rng.Intn(30)+150))
		if err != nil {
			log.Fatalf("seed owner membership: %v", err)
		}
		inserted[key] = true
		s.nodeMembers[o.nodeID] = append(s.nodeMembers[o.nodeID], o.ownerID)
		s.stats.memberships++
	}

	policyMap := make(map[string]string)
	prows, _ := s.db.Query("SELECT id, membership_policy FROM nodes")
	for prows.Next() {
		var nid, pol string
		prows.Scan(&nid, &pol)
		policyMap[nid] = pol
	}
	prows.Close()

	// Invite-only nodes (bands, per CLAUDE.md's governance shape — "invite-only,
	// all members are admins") don't take random self-joins. Their membership
	// is curated explicitly via profileData.extraMemberships instead, so 2-6
	// member bands stay plausible instead of ballooning from the random pool.
	var joinableNodeIDs []string
	for _, nodeID := range s.nodeIDs {
		if policyMap[nodeID] != "invite_only" {
			joinableNodeIDs = append(joinableNodeIDs, nodeID)
		}
	}

	// Each user joins 4-8 nodes (more overlap = richer inferred threads).
	roles := []string{"member", "member", "member", "admin", "follower", "follower"}

	for _, userID := range s.userIDs {
		numNodes := 4 + s.rng.Intn(5)
		perm := s.rng.Perm(len(joinableNodeIDs))
		for i := 0; i < numNodes && i < len(perm); i++ {
			nodeID := joinableNodeIDs[perm[i]]
			key := userID + ":" + nodeID
			if inserted[key] {
				continue
			}

			role := roles[s.rng.Intn(len(roles))]
			status := "active"
			if policyMap[nodeID] == "approval_required" && s.rng.Intn(5) == 0 {
				status = "pending"
			}

			id := auth.NewUUIDv7()
			_, err := s.db.Exec(`INSERT INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, ?, ?, ?)`,
				id, userID, nodeID, role, status, s.ts(s.rng.Intn(120)+10))
			if err != nil {
				log.Fatalf("seed membership: %v", err)
			}
			inserted[key] = true
			if status == "active" {
				s.nodeMembers[nodeID] = append(s.nodeMembers[nodeID], userID)
			}
			s.stats.memberships++
		}
	}
}

// ---------------------------------------------------------------------------
// Dev Personas — specific test accounts with controlled membership patterns
// ---------------------------------------------------------------------------

func (s *seeder) seedDevPersonas() {
	type persona struct {
		email       string
		username    string
		displayName string
		bio         string
		role        string // site role
		// Membership pattern: "admin:3" means admin of 3 random nodes,
		// "member:5" means member of 5, "follower:8" means follower of 8
		adminOf    int
		memberOf   int
		followerOf int
	}

	personas := []persona{
		{
			email: "new@localhost", username: "new-user", displayName: "New User",
			bio:  "Just signed up. Haven't joined anything yet.",
			role: "member",
		},
		{
			email: "lurker@localhost", username: "lurker", displayName: "Quiet Observer",
			bio:        "I follow everything but never join.",
			role:       "member",
			followerOf: 12,
		},
		{
			email: "active@localhost", username: "active-member", displayName: "Active Member",
			bio:        "I'm involved in a bunch of stuff.",
			role:       "member",
			adminOf:    1,
			memberOf:   5,
			followerOf: 3,
		},
		{
			email: "organizer@localhost", username: "organizer", displayName: "Super Organizer",
			bio:        "I run half the patches in this town.",
			role:       "member",
			adminOf:    6,
			memberOf:   4,
			followerOf: 2,
		},
		{
			email: "admin@localhost", username: "patchwork-admin", displayName: "Patchwork Admin",
			bio:        "Platform administrator for the Lancaster Patchwork instance.",
			role:       "admin",
			adminOf:    3,
			memberOf:   5,
			followerOf: 4,
		},
	}

	// Dev personas join like any other self-serve user — they shouldn't land
	// on invite-only bands (self-join is blocked there; membership is
	// curated admin-only, per CLAUDE.md's band governance shape).
	policyMap := make(map[string]string)
	prows, _ := s.db.Query("SELECT id, membership_policy FROM nodes")
	for prows.Next() {
		var nid, pol string
		prows.Scan(&nid, &pol)
		policyMap[nid] = pol
	}
	prows.Close()
	var joinableNodeIDs []string
	for _, nodeID := range s.nodeIDs {
		if policyMap[nodeID] != "invite_only" {
			joinableNodeIDs = append(joinableNodeIDs, nodeID)
		}
	}

	// admin@localhost already exists from seedUsers — skip creating, just set memberships
	for _, p := range personas {
		var userID string
		err := s.db.QueryRow("SELECT id FROM users WHERE email = ?", p.email).Scan(&userID)

		if err != nil {
			// User doesn't exist — create
			userID = auth.NewUUIDv7()
			createdAt := s.ts(30)
			personaAPID := ap.UserAPID(ap.GetDomain(), userID)
			_, err := s.db.Exec(`INSERT INTO users (id, email, username, display_name, bio, role, created_at, updated_at, ap_id)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				userID, p.email, p.username, p.displayName, p.bio, p.role, createdAt, createdAt, personaAPID)
			if err != nil {
				log.Printf("skip persona %s: %v", p.username, err)
				continue
			}
			s.userIDs = append(s.userIDs, userID)
			s.stats.users++
		}

		// Clear existing memberships for this persona (so re-seed is clean)
		s.db.Exec("DELETE FROM memberships WHERE user_id = ?", userID)

		// Assign memberships
		perm := s.rng.Perm(len(joinableNodeIDs))
		idx := 0

		assign := func(count int, role string) {
			for i := 0; i < count && idx < len(perm); i++ {
				nodeID := joinableNodeIDs[perm[idx]]
				idx++
				id := auth.NewUUIDv7()
				s.db.Exec(`INSERT INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, ?, 'active', ?)`,
					id, userID, nodeID, role, s.ts(s.rng.Intn(60)+5))
				s.stats.memberships++
			}
		}

		assign(p.adminOf, "admin")
		assign(p.memberOf, "member")
		assign(p.followerOf, "follower")

		// Guarantee the dev admin is an admin of code-and-coffee so E2E tests
		// can deterministically reach its patch-admin settings page, and a
		// member of lancaster-arts-district — the governance, proposal-voting,
		// and double-join specs all assert member-only UI/behavior there as
		// the admin. Before this guarantee that membership fell out of rng
		// luck and broke whenever the dataset changed shape.
		//
		// Fixed timestamps (NOT s.ts()): this block runs before
		// seedProposals, so consuming seeder rng would shift every
		// downstream draw and break specs that pin vote tallies. These
		// memberships also never enter s.nodeMembers, so the admin is never
		// a seeded voter — proposal-voting.spec.js owns the admin's votes.
		if p.email == "admin@localhost" {
			guarantees := []struct{ slug, role string }{
				{"code-and-coffee", "admin"},
				{"lancaster-arts-district", "member"},
			}
			for _, g := range guarantees {
				var nodeID string
				if err := s.db.QueryRow("SELECT id FROM nodes WHERE slug = ?", g.slug).Scan(&nodeID); err != nil {
					continue
				}
				s.db.Exec("DELETE FROM memberships WHERE user_id = ? AND node_id = ?", userID, nodeID)
				joinedAt := s.now.AddDate(0, 0, -45).Format("2006-01-02T15:04:05.000Z")
				s.db.Exec(`INSERT INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, ?, 'active', ?)`,
					auth.NewUUIDv7(), userID, nodeID, g.role, joinedAt)
				s.stats.memberships++
			}
		}
	}
}

// seedJoinFlowPersona creates the account that join-follow.spec.js owns for
// join/leave/follow round trips. Unlike the rng-assigned personas above, its
// memberships are deterministic: exactly one membership in a quiet patch (so
// the zero-membership /welcome redirect never fires) and guaranteed
// non-membership in lancaster-writers-guild, the patch that spec joins and
// leaves. No other spec may assert on this user or on writers-guild membership.
func (s *seeder) seedJoinFlowPersona() {
	// Plain timestamps, NOT s.ts(): that would consume rng draws and shift
	// every random decision after this point (vote tallies, memberships, …)
	// that other specs pin their assertions to.
	userID := auth.NewUUIDv7()
	createdAt := s.now.AddDate(0, 0, -30).Format("2006-01-02T15:04:05.000Z")
	joinedAt := s.now.AddDate(0, 0, -20).Format("2006-01-02T15:04:05.000Z")
	apID := ap.UserAPID(ap.GetDomain(), userID)
	_, err := s.db.Exec(`INSERT INTO users (id, email, username, display_name, bio, role, created_at, updated_at, ap_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, "joiner@localhost", "joiner", "Join Flow Tester",
		"E2E account for membership round trips.", "member", createdAt, createdAt, apID)
	if err != nil {
		log.Printf("skip joiner persona: %v", err)
		return
	}
	s.stats.users++

	var homeNodeID string
	if err := s.db.QueryRow("SELECT id FROM nodes WHERE slug = 'yoga-in-the-park'").Scan(&homeNodeID); err != nil {
		log.Printf("warning: joiner home patch: %v", err)
		return
	}
	s.db.Exec(`INSERT INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, 'member', 'active', ?)`,
		auth.NewUUIDv7(), userID, homeNodeID, joinedAt)
	s.stats.memberships++
}

// ---------------------------------------------------------------------------
// Events — 40+ events spanning past, present, and future
// ---------------------------------------------------------------------------

type eventDef struct {
	title       string
	description string
	nodeSlug    string
	location    string
	daysOffset  int
	durationH   int
}

func (s *seeder) seedEvents() {
	events := s.profile.events

	type nodeGeo struct {
		id  string
		lat float64
		lng float64
	}
	slugToGeo := make(map[string]nodeGeo)
	rows, _ := s.db.Query("SELECT id, slug, latitude, longitude FROM nodes")
	for rows.Next() {
		var g nodeGeo
		var slug string
		var lat, lng sql.NullFloat64
		rows.Scan(&g.id, &slug, &lat, &lng)
		if lat.Valid {
			g.lat = lat.Float64
		}
		if lng.Valid {
			g.lng = lng.Float64
		}
		slugToGeo[slug] = g
	}
	rows.Close()

	for _, e := range events {
		geo := slugToGeo[e.nodeSlug]
		if geo.id == "" {
			log.Printf("warning: event node %s not found", e.nodeSlug)
			continue
		}

		members := s.nodeMembers[geo.id]
		if len(members) == 0 {
			members = s.userIDs[:1]
		}
		creatorID := members[s.rng.Intn(len(members))]

		id := auth.NewUUIDv7()
		var startsAt, endsAt string
		if e.daysOffset < 0 {
			startsAt = s.ts(-e.daysOffset)
		} else {
			startsAt = s.futureTS(e.daysOffset)
		}
		st, _ := time.Parse("2006-01-02T15:04:05.000Z", startsAt)
		et := st.Add(time.Duration(e.durationH) * time.Hour)
		endsAt = et.Format("2006-01-02T15:04:05.000Z")

		lat := geo.lat + (s.rng.Float64()-0.5)*0.001
		lng := geo.lng + (s.rng.Float64()-0.5)*0.001

		createdAt := s.ts(-e.daysOffset + 14)
		apID := ap.EventAPID(ap.GetDomain(), id)

		_, err := s.db.Exec(`INSERT INTO events (id, node_id, created_by, title, description, location, latitude, longitude, starts_at, ends_at, visibility, created_at, updated_at, ap_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'public', ?, ?, ?)`,
			id, geo.id, creatorID, e.title, e.description, e.location, lat, lng, startsAt, endsAt, createdAt, createdAt, apID)
		if err != nil {
			log.Fatalf("seed event %s: %v", e.title, err)
		}
		s.stats.events++
	}
}

// ---------------------------------------------------------------------------
// Proposals
// ---------------------------------------------------------------------------

type proposalDef struct {
	title        string
	body         string
	nodeSlug     string
	status       string
	proposalType string
	daysAgo      int
	durationH    int
}

func (s *seeder) seedProposals() {
	proposals := s.profile.proposals

	slugToID := make(map[string]string)
	rows, _ := s.db.Query("SELECT id, slug FROM nodes")
	for rows.Next() {
		var id, slug string
		rows.Scan(&id, &slug)
		slugToID[slug] = id
	}
	rows.Close()

	for _, p := range proposals {
		nodeID := slugToID[p.nodeSlug]
		if nodeID == "" {
			log.Printf("warning: proposal node %s not found", p.nodeSlug)
			continue
		}

		members := s.nodeMembers[nodeID]
		if len(members) == 0 {
			continue
		}
		authorID := members[s.rng.Intn(len(members))]

		id := auth.NewUUIDv7()
		createdAt := s.ts(p.daysAgo)
		apID := ap.ProposalAPID(ap.GetDomain(), id)

		var votingEndsAt *string
		if p.status == "open" {
			v := s.futureTS(p.durationH/24 - p.daysAgo)
			votingEndsAt = &v
		} else if p.status != "withdrawn" {
			v := s.ts(p.daysAgo - p.durationH/24)
			votingEndsAt = &v
		}

		// For amendment proposals, set up target doc and proposed branch
		var targetDoc, proposedBranch, proposedBody *string
		if p.proposalType == "amendment" {
			td := "community-standards.md"
			targetDoc = &td
			branchName := fmt.Sprintf("amendment-%s", id[:8])
			proposedBranch = &branchName
			pb := p.body
			proposedBody = &pb
		}

		_, err := s.db.Exec(`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at, ap_id, target_doc, proposed_branch, proposed_body)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, nodeID, authorID, p.title, p.body, p.status, p.proposalType, p.durationH, votingEndsAt, createdAt, createdAt, apID, targetDoc, proposedBranch, proposedBody)
		if err != nil {
			log.Fatalf("seed proposal %s: %v", p.title, err)
		}

		// Create git branch for amendment proposals
		if targetDoc != nil && proposedBranch != nil && proposedBody != nil {
			_, branchErr := governance.CreateBranch(s.dataDir, nodeID, *proposedBranch, *targetDoc, *proposedBody, "Patchwork Seed", "seed@patchwork.local", "Proposed: "+p.title)
			if branchErr != nil {
				log.Printf("warning: create amendment branch for %s: %v", p.title, branchErr)
			}
		}

		numVotes := 3 + s.rng.Intn(6)
		if numVotes > len(members) {
			numVotes = len(members)
		}
		voterPerm := s.rng.Perm(len(members))
		for v := 0; v < numVotes; v++ {
			voterID := members[voterPerm[v]]
			voteID := auth.NewUUIDv7()

			var value string
			switch p.status {
			case "approved":
				if s.rng.Intn(5) == 0 {
					value = "reject"
				} else {
					value = "approve"
				}
			case "rejected":
				if s.rng.Intn(5) == 0 {
					value = "approve"
				} else {
					value = "reject"
				}
			default:
				values := []string{"approve", "approve", "reject", "abstain"}
				value = values[s.rng.Intn(len(values))]
			}

			voteAt := s.ts(p.daysAgo - s.rng.Intn(3))
			s.db.Exec(`INSERT INTO votes (id, proposal_id, user_id, value, created_at) VALUES (?, ?, ?, ?, ?)`,
				voteID, id, voterID, value, voteAt)
		}

		s.stats.proposals++
		switch p.status {
		case "open":
			s.stats.openProposals++
		case "approved":
			s.stats.approvedProposals++
		case "rejected":
			s.stats.rejectedProposals++
		case "withdrawn":
			s.stats.withdrawnProposals++
		}
	}
}

// ---------------------------------------------------------------------------
// Comments & Reactions — threaded discussion on proposals
// ---------------------------------------------------------------------------

func (s *seeder) seedComments() {
	// Gather proposals with their node members for realistic authorship
	type proposalInfo struct {
		id      string
		nodeID  string
		title   string
		status  string
		daysAgo int
	}

	rows, err := s.db.Query(`SELECT p.id, p.node_id, p.title, p.status,
		CAST(julianday('now') - julianday(p.created_at) AS INTEGER)
		FROM proposals p ORDER BY p.created_at DESC`)
	if err != nil {
		log.Printf("warning: query proposals for comments: %v", err)
		return
	}
	var proposals []proposalInfo
	for rows.Next() {
		var p proposalInfo
		rows.Scan(&p.id, &p.nodeID, &p.title, &p.status, &p.daysAgo)
		proposals = append(proposals, p)
	}
	rows.Close()

	if len(proposals) == 0 {
		return
	}

	// Comment threads per proposal — keyed by proposal title substring for matching
	type commentDef struct {
		body    string
		emoji   string // reaction to add (empty = none)
		replies []struct {
			body  string
			emoji string
		}
	}

	// Generic governance comments that work for any proposal
	genericThreads := [][]commentDef{
		{
			{body: "I support this — it addresses something we've been talking about for months.", emoji: "👍"},
			{body: "Could we add a trial period so we can evaluate after 3 months?", emoji: "🤔", replies: []struct {
				body  string
				emoji string
			}{
				{body: "Good idea. A 90-day review clause would make this easier to approve.", emoji: "👍"},
			}},
			{body: "Has anyone looked at how other patches handle this? Would be good to learn from their experience.", emoji: ""},
		},
		{
			{body: "This is a great step forward. Count me in as a volunteer if it passes.", emoji: "👍"},
			{body: "I have concerns about the budget implications. Can we get a cost breakdown?", emoji: "🤔", replies: []struct {
				body  string
				emoji string
			}{
				{body: "I can put together numbers by next week. The initial estimate was in the proposal body.", emoji: ""},
				{body: "Thanks — that would help a lot. Hard to vote without knowing the full picture.", emoji: "👍"},
			}},
		},
		{
			{body: "The language in the proposal is clear and well-thought-out. Nice work.", emoji: "👍"},
			{body: "I think we should also consider accessibility. Not everyone can participate in the same way.", emoji: "🤔"},
			{body: "Fully support this. Let's move forward.", emoji: "👍"},
		},
		{
			{body: "This feels premature. Can we table it until we have more member input?", emoji: "🤔", replies: []struct {
				body  string
				emoji string
			}{
				{body: "We've discussed this at three meetings already. I think we have enough input.", emoji: ""},
			}},
			{body: "I'm a yes on this. The status quo isn't working.", emoji: "👍"},
		},
		{
			{body: "Love the intent but the enforcement section feels vague. Can we define specific consequences?", emoji: "🤔", replies: []struct {
				body  string
				emoji string
			}{
				{body: "Agreed — maybe a tiered system: warning, suspension, removal?", emoji: "👍"},
			}},
			{body: "Overdue change. The clearer our guidelines, the safer our spaces.", emoji: "👍"},
			{body: "Could we also address online behavior? Our Discord has had some issues.", emoji: "", replies: []struct {
				body  string
				emoji string
			}{
				{body: "Good point. Maybe that's a separate amendment though?", emoji: "🤔"},
			}},
		},
		{
			{body: "Strong yes from me. This aligns with our values.", emoji: "👍"},
			{body: "What's the timeline for implementation if this passes?", emoji: "🤔", replies: []struct {
				body  string
				emoji string
			}{
				{body: "I'd suggest 30 days from approval to give everyone time to adjust.", emoji: ""},
			}},
		},
		{
			{body: "I appreciate the author putting this together. It's a lot of work to draft a proposal.", emoji: "👍"},
			{body: "Minor suggestion: can we add a review date so this doesn't just sit forever?", emoji: ""},
		},
		{
			{body: "Not sure this is the right approach but I'm open to being convinced.", emoji: "🤔", replies: []struct {
				body  string
				emoji string
			}{
				{body: "What alternative would you prefer? Happy to discuss.", emoji: ""},
				{body: "I had similar reservations but the more I think about it, the more it makes sense.", emoji: "👍"},
			}},
		},
	}

	// Seed comments on first 8 proposals (or fewer if we have fewer)
	numToSeed := 8
	if numToSeed > len(proposals) {
		numToSeed = len(proposals)
	}

	for i := 0; i < numToSeed; i++ {
		p := proposals[i]
		threads := genericThreads[i%len(genericThreads)]

		members := s.nodeMembers[p.nodeID]
		if len(members) < 2 {
			continue
		}

		memberIdx := 0
		pickMember := func() string {
			uid := members[memberIdx%len(members)]
			memberIdx++
			return uid
		}

		for _, thread := range threads {
			commentID := auth.NewUUIDv7()
			authorID := pickMember()
			commentAge := p.daysAgo - 1 - s.rng.Intn(3)
			if commentAge < 0 {
				commentAge = 0
			}
			createdAt := s.ts(commentAge)

			_, err := s.db.Exec(`INSERT INTO proposal_comments (id, proposal_id, parent_id, author_id, body, created_at, updated_at)
				VALUES (?, ?, NULL, ?, ?, ?, ?)`,
				commentID, p.id, authorID, thread.body, createdAt, createdAt)
			if err != nil {
				log.Printf("warning: seed comment: %v", err)
				continue
			}
			s.stats.comments++

			// Add reaction to this comment
			if thread.emoji != "" {
				reactorID := pickMember()
				reactionID := auth.NewUUIDv7()
				_, err := s.db.Exec(`INSERT INTO comment_reactions (id, comment_id, user_id, emoji, created_at)
					VALUES (?, ?, ?, ?, ?)`,
					reactionID, commentID, reactorID, thread.emoji, createdAt)
				if err != nil {
					log.Printf("warning: seed reaction: %v", err)
				} else {
					s.stats.reactions++
				}
			}

			// Add replies
			for _, reply := range thread.replies {
				replyID := auth.NewUUIDv7()
				replyAuthor := pickMember()
				replyAge := commentAge - 1
				if replyAge < 0 {
					replyAge = 0
				}
				replyAt := s.ts(replyAge)

				_, err := s.db.Exec(`INSERT INTO proposal_comments (id, proposal_id, parent_id, author_id, body, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?)`,
					replyID, p.id, commentID, replyAuthor, reply.body, replyAt, replyAt)
				if err != nil {
					log.Printf("warning: seed reply: %v", err)
					continue
				}
				s.stats.comments++

				if reply.emoji != "" {
					reactorID := pickMember()
					reactionID := auth.NewUUIDv7()
					_, err := s.db.Exec(`INSERT INTO comment_reactions (id, comment_id, user_id, emoji, created_at)
						VALUES (?, ?, ?, ?, ?)`,
						reactionID, replyID, reactorID, reply.emoji, replyAt)
					if err != nil {
						log.Printf("warning: seed reply reaction: %v", err)
					} else {
						s.stats.reactions++
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Revisions — versioned edits showing proposal evolution
// ---------------------------------------------------------------------------

func (s *seeder) seedRevisions() {
	// Pick 3 proposals that have been around long enough to have revisions
	type proposalInfo struct {
		id       string
		nodeID   string
		title    string
		body     string
		authorID string
	}

	rows, err := s.db.Query(`SELECT p.id, p.node_id, p.title, p.body, p.author_id
		FROM proposals p
		WHERE p.status IN ('approved', 'open')
		ORDER BY p.created_at ASC LIMIT 3`)
	if err != nil {
		log.Printf("warning: query proposals for revisions: %v", err)
		return
	}
	var proposals []proposalInfo
	for rows.Next() {
		var p proposalInfo
		rows.Scan(&p.id, &p.nodeID, &p.title, &p.body, &p.authorID)
		proposals = append(proposals, p)
	}
	rows.Close()

	type revisionDef struct {
		titleSuffix string
		bodySuffix  string
		changeNote  string
	}

	// Revision patterns — each proposal gets 1-2 revisions
	revisionSets := [][]revisionDef{
		{
			{titleSuffix: "", bodySuffix: " (Draft: initial wording before community feedback.)", changeNote: "Initial draft submitted for discussion"},
			{titleSuffix: "", bodySuffix: "", changeNote: "Updated quorum value based on community feedback"},
		},
		{
			{titleSuffix: "", bodySuffix: " Preliminary version without cost estimates.", changeNote: "Added cost breakdown and timeline after member request"},
		},
		{
			{titleSuffix: " (v1)", bodySuffix: " Original scope was broader.", changeNote: "Narrowed scope to focus on immediate needs"},
			{titleSuffix: "", bodySuffix: "", changeNote: "Incorporated accessibility suggestions from discussion"},
		},
	}

	for i, p := range proposals {
		revisions := revisionSets[i%len(revisionSets)]

		for j, rev := range revisions {
			revID := auth.NewUUIDv7()
			revNum := j + 1
			revTitle := p.title + rev.titleSuffix
			revBody := p.body + rev.bodySuffix
			createdAt := s.ts(30 - j*3) // revisions spaced a few days apart

			_, err := s.db.Exec(`INSERT INTO proposal_revisions (id, proposal_id, title, body, proposed_body, revision_number, author_id, change_note, created_at)
				VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?)`,
				revID, p.id, revTitle, revBody, revNum, p.authorID, rev.changeNote, createdAt)
			if err != nil {
				log.Printf("warning: seed revision: %v", err)
				continue
			}
			s.stats.revisions++
		}
	}
}

// ---------------------------------------------------------------------------
// Governance Docs
// ---------------------------------------------------------------------------

func (s *seeder) seedGovernanceDocs() {
	docs := s.profile.govDocs

	slugToID := make(map[string]string)
	rows, _ := s.db.Query("SELECT id, slug FROM nodes")
	for rows.Next() {
		var id, slug string
		rows.Scan(&id, &slug)
		slugToID[slug] = id
	}
	rows.Close()

	for _, d := range docs {
		nodeID := slugToID[d.nodeSlug]
		if nodeID == "" {
			continue
		}

		members := s.nodeMembers[nodeID]
		if len(members) == 0 {
			continue
		}
		creatorID := members[0]

		id := auth.NewUUIDv7()
		createdAt := s.ts(s.rng.Intn(60) + 60)
		_, err := s.db.Exec(`INSERT INTO governance_docs (id, node_id, title, body, version, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, 1, ?, ?, ?)`,
			id, nodeID, d.title, d.body, creatorID, createdAt, createdAt)
		if err != nil {
			log.Fatalf("seed governance doc %s: %v", d.title, err)
		}

		// Also write the governance doc to the node's git repo
		filename := slug(d.title) + ".md"
		_, gitErr := governance.DirectEdit(s.dataDir, nodeID, filename, d.body, "Patchwork Admin", "admin@patchwork.local", "Initial governance doc: "+d.title)
		if gitErr != nil {
			log.Printf("warning: governance git write for %s/%s: %v", d.nodeSlug, d.title, gitErr)
		}

		s.stats.govDocs++
	}
}

// ---------------------------------------------------------------------------
// Content Reports
// ---------------------------------------------------------------------------

func (s *seeder) seedReports() {
	type reportDef struct {
		entityType     string
		reason         string
		details        string
		status         string
		resolutionNote string
	}

	reports := []reportDef{
		{"event", "spam", "This event appears to be promoting a commercial product rather than a community activity.", "pending", ""},
		{"node", "inappropriate", "Description contains language that may be unwelcoming to certain community members.", "pending", ""},
		{"event", "misinformation", "Event location is incorrect. This venue closed last month.", "dismissed", "Reporter confirmed venue reopened under new management."},
		{"event", "harassment", "Event organizer made unwelcoming comments to attendees at the last session.", "resolved", "Organizer has been contacted and agreed to community guidelines refresher."},
	}

	var eventIDs []string
	erows, _ := s.db.Query("SELECT id FROM events LIMIT 10")
	for erows.Next() {
		var id string
		erows.Scan(&id)
		eventIDs = append(eventIDs, id)
	}
	erows.Close()

	adminID := s.userIDs[0]

	for i, r := range reports {
		id := auth.NewUUIDv7()
		reporterID := s.userIDs[s.rng.Intn(len(s.userIDs))]

		var entityID string
		if r.entityType == "event" && len(eventIDs) > 0 {
			entityID = eventIDs[i%len(eventIDs)]
		} else {
			entityID = s.nodeIDs[s.rng.Intn(len(s.nodeIDs))]
		}

		var reviewedBy *string
		if r.status != "pending" {
			reviewedBy = &adminID
		}

		createdAt := s.ts(s.rng.Intn(20) + 5)
		_, err := s.db.Exec(`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details, status, reviewed_by, resolution_note, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, reporterID, r.entityType, entityID, r.reason, r.details, r.status, reviewedBy, r.resolutionNote, createdAt, createdAt)
		if err != nil {
			log.Fatalf("seed report: %v", err)
		}
		s.stats.reports++
	}
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

func (s *seeder) seedNotifications() {
	notifs := s.profile.notifications

	for _, n := range notifs {
		id := auth.NewUUIDv7()
		userID := s.userIDs[n.userIdx]
		createdAt := s.ts(s.rng.Intn(30) + 1)

		var readAt *string
		if n.read {
			r := s.ts(s.rng.Intn(5))
			readAt = &r
		}

		_, err := s.db.Exec(`INSERT INTO notifications (id, user_id, type, title, body, link, read_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, userID, n.nType, n.title, n.body, n.link, readAt, createdAt)
		if err != nil {
			log.Fatalf("seed notification: %v", err)
		}
		s.stats.notifications++
	}

	// admin@localhost (index 0) — seed a fixed inbox so the bell panel shows a
	// list + "View all" link, letting E2E assert it unconditionally. Added after
	// the rng-driven loop with FIXED timestamps (NOT s.ts()): consuming seeder
	// rng here would shift downstream draws (seedUnclaimedPatches' random
	// follower picks) and break the new-user zero-membership invariant.
	adminNotifs := []struct {
		nType, title, body, link string
		read                     bool
	}{
		{"new_event", "New event: First Friday Gallery Walk", "A new event has been posted in the First Friday Collective.", "/events", false},
		{"proposal_created", "New proposal: Anti-harassment policy", "A new proposal has been created for the Lancaster Arts District.", "/nodes/lancaster-arts-district/proposals", false},
		{"new_member", "New member joined First Friday", "David Park has joined the First Friday Collective.", "/nodes/first-friday-collective/members", true},
	}
	for i, n := range adminNotifs {
		createdAt := s.now.AddDate(0, 0, -(i + 1)).Format("2006-01-02T15:04:05.000Z")
		var readAt *string
		if n.read {
			r := s.now.Format("2006-01-02T15:04:05.000Z")
			readAt = &r
		}
		_, err := s.db.Exec(`INSERT INTO notifications (id, user_id, type, title, body, link, read_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			auth.NewUUIDv7(), s.userIDs[0], n.nType, n.title, n.body, n.link, readAt, createdAt)
		if err != nil {
			log.Fatalf("seed admin notification: %v", err)
		}
		s.stats.notifications++
	}
}

// ---------------------------------------------------------------------------
// Audit Log
// ---------------------------------------------------------------------------

func (s *seeder) seedAuditLog() {
	type auditDef struct {
		action     string
		entityType string
		desc       string
	}

	actions := []auditDef{
		{"create", "user", "user_registered"},
		{"create", "node", "node_created"},
		{"create", "node", "node_created"},
		{"create", "event", "event_created"},
		{"create", "event", "event_created"},
		{"create", "membership", "membership_joined"},
		{"create", "membership", "membership_joined"},
		{"update", "membership", "membership_approved"},
		{"create", "proposal", "proposal_created"},
		{"update", "proposal", "proposal_approved"},
		{"create", "governance_doc", "governance_doc_created"},
		{"create", "report", "report_submitted"},
		{"update", "report", "report_reviewed"},
	}

	for _, a := range actions {
		id := auth.NewUUIDv7()
		userID := s.pick(s.userIDs)

		var entityID string
		switch a.entityType {
		case "node":
			entityID = s.pick(s.nodeIDs)
		default:
			entityID = auth.NewUUIDv7()
		}

		metadata := fmt.Sprintf(`{"action_detail":"%s"}`, a.desc)
		createdAt := s.ts(s.rng.Intn(120) + 1)

		s.db.Exec(`INSERT INTO audit_log (id, user_id, action, entity_type, entity_id, metadata, ip_address, created_at)
			VALUES (?, ?, ?, ?, ?, ?, '127.0.0.1', ?)`,
			id, userID, a.action, a.entityType, entityID, metadata, createdAt)
	}
}

// ---------------------------------------------------------------------------
// Federation Data — simulated cross-instance AP followers
// ---------------------------------------------------------------------------

func (s *seeder) seedFederationData() {
	remoteInstances := []struct{ domain, userSlug string }{
		{"philly-music.example.com", "philly-dj"},
		{"nyc-arts.example.com", "brooklyn-painter"},
		{"dc-community.example.com", "dc-organizer"},
	}

	for i, remote := range remoteInstances {
		if i >= len(s.nodeIDs) {
			break
		}
		id := auth.NewUUIDv7()
		remoteActorID := fmt.Sprintf("https://%s/ap/users/%s", remote.domain, remote.userSlug)
		remoteInbox := fmt.Sprintf("https://%s/ap/users/%s/inbox", remote.domain, remote.userSlug)
		createdAt := s.ts(s.rng.Intn(30) + 10)
		_, err := s.db.Exec(`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted, created_at) VALUES (?, 'node', ?, ?, ?, 1, ?)`,
			id, s.nodeIDs[i], remoteActorID, remoteInbox, createdAt)
		if err != nil {
			log.Printf("warning: seed ap follower: %v", err)
			continue
		}
		s.stats.apFollowers++
	}
}

func (s *seeder) seedUnclaimedPatches() {
	// Insert the system user for unclaimed patches.
	s.db.Exec(`INSERT OR IGNORE INTO users (id, username, display_name, role, bio, avatar_url, created_at, updated_at)
		VALUES ('00000000-0000-0000-0000-000000000000', '_system', 'Community', 'member', '', '', ?, ?)`,
		s.ts(365), s.ts(365))

	unclaimed := s.profile.unclaimed

	// Available palettes for unclaimed patches.
	unclaimedPalettes := []string{
		"adolescents", "pinkRazors", "greatestSongs", "allroysRevenge",
		"anthem", "allTheShoes", "bottlesToTheGround", "liberalAnimation",
	}

	for i, u := range unclaimed {
		id := auth.NewUUIDv7()
		nodeSlug := slug(u.name)

		linksJSON := "[]"
		if len(u.links) > 0 {
			b, _ := json.Marshal(u.links)
			linksJSON = string(b)
		}

		appearanceJSON := fmt.Sprintf(`{"palette":%q}`, unclaimedPalettes[i%len(unclaimedPalettes)])
		apID := fmt.Sprintf("https://%s/ap/nodes/%s", "patchwork.local", id)
		createdAt := s.ts(s.rng.Intn(60) + 30)

		submissionSource := u.submissionSource
		if submissionSource == "" {
			submissionSource = "admin"
		}
		var submittedBy interface{}
		if u.submitterIdx != nil && *u.submitterIdx < len(s.userIDs) {
			submittedBy = s.userIDs[*u.submitterIdx]
		}

		_, err := s.db.Exec(
			`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, website, links, visibility, membership_policy, appearance, status, submitted_by, submission_source, ap_id, created_at, updated_at)
			 VALUES (?, '00000000-0000-0000-0000-000000000000', ?, ?, ?, ?, ?, ?, ?, ?, 'public', 'open', ?, 'unclaimed', ?, ?, ?, ?, ?)`,
			id, u.name, nodeSlug, u.desc, u.lat, u.lng, u.address, u.website, linksJSON, appearanceJSON, submittedBy, submissionSource, apID, createdAt, createdAt,
		)
		if err != nil {
			log.Printf("warning: seed unclaimed %s: %v", u.name, err)
			continue
		}

		// Assign tags.
		for _, tagName := range u.tags {
			var tagID string
			if s.db.QueryRow("SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID) == nil {
				s.db.Exec("INSERT OR IGNORE INTO node_tags (node_id, tag_id) VALUES (?, ?)", id, tagID)
			}
		}

		// Add some followers from existing users to test affinity.
		followerCount := s.rng.Intn(5) + 2
		for j := 0; j < followerCount && j < len(s.userIDs); j++ {
			userIdx := s.rng.Intn(len(s.userIDs))
			memID := auth.NewUUIDv7()
			s.db.Exec(
				`INSERT OR IGNORE INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, 'follower', 'active', ?)`,
				memID, s.userIDs[userIdx], id, s.ts(s.rng.Intn(30)+5),
			)
		}

		s.stats.nodes++
	}

	// Also create pending_review submissions for admin queue testing.
	pendingSubmissions := s.profile.pendingSubmissions

	for _, u := range pendingSubmissions {
		id := auth.NewUUIDv7()
		nodeSlug := slug(u.name)
		createdAt := s.ts(s.rng.Intn(5) + 1)

		// Use a random existing user as submitter.
		submitterIdx := s.rng.Intn(len(s.userIDs))

		s.db.Exec(
			`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, visibility, membership_policy, status, submitted_by, submission_source, created_at, updated_at)
			 VALUES (?, '00000000-0000-0000-0000-000000000000', ?, ?, ?, ?, ?, '', 'public', 'open', 'pending_review', ?, 'community', ?, ?)`,
			id, u.name, nodeSlug, u.desc, u.lat, u.lng, s.userIDs[submitterIdx], createdAt, createdAt,
		)

		for _, tagName := range u.tags {
			var tagID string
			if s.db.QueryRow("SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID) == nil {
				s.db.Exec("INSERT OR IGNORE INTO node_tags (node_id, tag_id) VALUES (?, ?)", id, tagID)
			}
		}
	}

	log.Printf("  seeded %d unclaimed patches + %d pending submissions", len(unclaimed), len(pendingSubmissions))
}

// ---------------------------------------------------------------------------
// Extra Memberships — curated overlap a profile can't express through the
// generic random join pass (e.g. "every member of this band is an admin",
// or "these musicians specifically follow these venues"). No-op when a
// profile doesn't set profileData.extraMemberships (e.g. arts).
// ---------------------------------------------------------------------------

func (s *seeder) seedExtraMemberships() {
	if len(s.profile.extraMemberships) == 0 {
		return
	}

	slugToID := make(map[string]string)
	rows, _ := s.db.Query("SELECT id, slug FROM nodes")
	for rows.Next() {
		var id, slug string
		rows.Scan(&id, &slug)
		slugToID[slug] = id
	}
	rows.Close()

	for _, em := range s.profile.extraMemberships {
		if em.userIdx >= len(s.userIDs) {
			continue
		}
		nodeID := slugToID[em.nodeSlug]
		if nodeID == "" {
			log.Printf("warning: extra membership node %s not found", em.nodeSlug)
			continue
		}
		userID := s.userIDs[em.userIdx]

		var existing string
		if err := s.db.QueryRow("SELECT id FROM memberships WHERE user_id = ? AND node_id = ?", userID, nodeID).Scan(&existing); err == nil {
			// Already joined via the random pass or another curated row — leave it.
			continue
		}

		id := auth.NewUUIDv7()
		_, err := s.db.Exec(`INSERT INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, ?, 'active', ?)`,
			id, userID, nodeID, em.role, s.ts(s.rng.Intn(120)+10))
		if err != nil {
			log.Printf("warning: seed extra membership: %v", err)
			continue
		}
		s.nodeMembers[nodeID] = append(s.nodeMembers[nodeID], userID)
		s.stats.memberships++
	}
}

func slug(name string) string {
	s := strings.ToLower(name)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
