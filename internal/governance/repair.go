package governance

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// The repair identity. A repair commit is a statement about the database, not
// about a person, and it must never be mistaken for one: it wears its own
// author name so that git log, the versions API and the history view can all
// tell a reconstructed commit from a real one (docs/adr/084).
const (
	RepairAuthorName  = "Patchwork repair"
	RepairAuthorEmail = "repair@patchwork.local"

	// RebuildMessage is the message on the single commit that stands in for a
	// repo's whole lost history. Its exact text is load-bearing: it is what
	// separates "rebuilt" (everything before is gone) from "restored" (the
	// history is intact, one file was behind).
	RebuildMessage = "Rebuilt from database; original history unavailable"
)

// RepairKind classifies a commit written by the repair pass. It is empty for
// every ordinary commit.
const (
	RepairRebuilt  = "rebuilt"
	RepairRestored = "restored"
)

// restoreMessage is the message on a commit that brings one file in an
// existing repo back in line with its canonical database row.
func restoreMessage(what string) string {
	return "Restored " + what + " from database; the repo copy was missing or stale"
}

// repairKind reads the repair marker off a commit. Detection is anchored on
// the author identity, not the message, so a member who happens to write
// "Rebuilt from database" in an amendment is not mislabelled.
func repairKind(c *object.Commit) string {
	if c.Author.Name != RepairAuthorName {
		return ""
	}
	if strings.TrimSpace(c.Message) == RebuildMessage {
		return RepairRebuilt
	}
	return RepairRestored
}

// RepoStatus is what a repair pass did to one repo.
type RepoStatus string

const (
	RepoUnchanged RepoStatus = "unchanged"
	RepoCreated   RepoStatus = "created"
	RepoUpdated   RepoStatus = "updated"
)

// RepoReport records the outcome for a single node's repo.
type RepoReport struct {
	NodeID string
	Slug   string
	Status RepoStatus
	// Files written by this pass, in the order they were committed. Empty
	// for an unchanged repo.
	Files []string
}

// Report is the whole pass, for printing.
type Report struct {
	InstanceRepo RepoStatus
	Repos        []RepoReport
}

// Counts summarizes the node repos: how many were looked at, and how each
// group came out.
func (r *Report) Counts() (checked, created, updated, unchanged int) {
	for _, rr := range r.Repos {
		checked++
		switch rr.Status {
		case RepoCreated:
			created++
		case RepoUpdated:
			updated++
		default:
			unchanged++
		}
	}
	return
}

// RepairOptions selects how far a pass reaches.
type RepairOptions struct {
	// RestoreExisting lets the pass write inside repos that already exist —
	// adding a document the database has and the repo lacks, and bringing a
	// stale file up to its canonical row. Left false, the pass only creates
	// repos that are absent and never touches one that is present. Startup
	// runs the safe half; the operator command runs the whole thing
	// (docs/adr/084).
	RestoreExisting bool
}

// Repair walks every live patch and reconciles its bare governance repo with
// the database, which is the canonical store (docs/adr/011). It is
// idempotent: a repo that already matches its rows is left byte-for-byte
// alone, HEAD included. It never deletes a ref, a commit, or a file — a repo
// that carries documents the database has never heard of keeps them.
//
// Scoped to active patches, because an unclaimed patch carries no governance
// repo at all until its claim's setup completes (docs/adr/039).
func Repair(db *database.DB, dataDir string, opts RepairOptions) (*Report, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("governance data dir not set")
	}

	rep := &Report{}

	// The instance repo holds no database-backed documents: its whole content
	// is the shipped baseline and the bundled templates (defaultInstanceFiles),
	// and nothing edits it at runtime. So "repairing" it means the same thing
	// as first boot does — create it if it is gone.
	if _, err := os.Stat(InstanceRepoPath(dataDir)); err == nil {
		rep.InstanceRepo = RepoUnchanged
	} else {
		if err := InitInstanceRepo(dataDir); err != nil {
			return rep, fmt.Errorf("instance repo: %w", err)
		}
		rep.InstanceRepo = RepoCreated
	}

	nodes, err := liveNodes(db)
	if err != nil {
		return rep, err
	}

	for _, n := range nodes {
		docs, err := nodeDocs(db, n.id)
		if err != nil {
			return rep, err
		}

		rr := RepoReport{NodeID: n.id, Slug: n.slug, Status: RepoUnchanged}

		if _, err := os.Stat(NodeRepoPath(dataDir, n.id)); err != nil {
			files, err := rebuildRepo(dataDir, n, docs)
			if err != nil {
				return rep, err
			}
			rr.Status, rr.Files = RepoCreated, files
		} else if opts.RestoreExisting {
			files, err := restoreRepo(dataDir, n, docs)
			if err != nil {
				return rep, err
			}
			if len(files) > 0 {
				rr.Status, rr.Files = RepoUpdated, files
			}
		}

		rep.Repos = append(rep.Repos, rr)
	}

	return rep, nil
}

// --- Reading the canonical side ---

type repairNode struct {
	id, slug         string
	governanceConfig string
	membershipPolicy string
	followerPerms    string
}

func liveNodes(db *database.DB) ([]repairNode, error) {
	rows, err := db.Query(`SELECT id, slug, COALESCE(governance_config,''), membership_policy,
		COALESCE(follower_permissions,'')
		FROM nodes WHERE status = 'active' AND removed_at IS NULL ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	var out []repairNode
	for rows.Next() {
		var n repairNode
		if err := rows.Scan(&n.id, &n.slug, &n.governanceConfig, &n.membershipPolicy, &n.followerPerms); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// repairDoc is one canonical governance_docs row, already resolved to the
// filename its title mirrors to.
type repairDoc struct {
	filename string
	title    string
	body     string
}

// nodeDocs reads a patch's canonical documents, oldest first. Two titles can
// slugify to one filename; the older row wins, so the mapping a repair writes
// is the same one every other mirror path has been writing.
func nodeDocs(db *database.DB, nodeID string) ([]repairDoc, error) {
	rows, err := db.Query(`SELECT title, body FROM governance_docs WHERE node_id = ?
		ORDER BY created_at, id`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("list governance docs for node %s: %w", nodeID, err)
	}
	defer rows.Close()

	var docs []repairDoc
	seen := map[string]bool{}
	for rows.Next() {
		var d repairDoc
		if err := rows.Scan(&d.title, &d.body); err != nil {
			return nil, fmt.Errorf("scan governance doc: %w", err)
		}
		d.filename = Filename(d.title)
		if seen[d.filename] {
			continue
		}
		seen[d.filename] = true
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// rulesFromDB reconstructs governance-rules.json out of the database cache
// columns — the exact inverse of SyncRulesToDB. Git is normally the canonical
// store for the rules file, but a repo that is gone has no rules to be
// canonical about, and the cache is what the running instance was actually
// enforcing. Falling back to template defaults instead would silently demote
// an elected patch to `maintainer` (docs/adr/084).
func rulesFromDB(n repairNode) (string, error) {
	rules := DefaultRules()
	if n.governanceConfig != "" && n.governanceConfig != "{}" {
		// GovernanceConfig and GovernanceRules carry identical JSON tags for
		// every field they share, so the cache decodes straight over the
		// defaults; anything the cache omits keeps its default.
		if err := json.Unmarshal([]byte(n.governanceConfig), rules); err != nil {
			return "", fmt.Errorf("parse governance_config for %s: %w", n.slug, err)
		}
	}
	if n.membershipPolicy != "" {
		rules.MembershipPolicy = n.membershipPolicy
	}
	if n.followerPerms != "" && n.followerPerms != "{}" {
		json.Unmarshal([]byte(n.followerPerms), &rules.FollowerPermissions)
	}

	b, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal rules for %s: %w", n.slug, err)
	}
	return string(b) + "\n", nil
}

// --- Writing the derived side ---

// rebuildRepo creates a repo that is not there, from one synthetic commit
// holding every canonical document at once.
//
// One commit, not one per document: the commits would be a fiction either
// way, and a run of them would fabricate an order and a set of dates that no
// community ever lived through. One commit says the true thing — this whole
// tree arrived from the database in a single moment — and leaves each
// document's history exactly one entry long, which is what a patch that has
// lost its history actually has.
//
// Only what the database attests to is written. The template text a patch
// would have been forked with is deliberately not filled in: a governance
// document nobody adopted is worse than a missing one.
func rebuildRepo(dataDir string, n repairNode, docs []repairDoc) ([]string, error) {
	files := map[string]string{}
	var names []string
	for _, d := range docs {
		files[d.filename] = d.body
		names = append(names, d.filename)
	}

	rulesJSON, err := rulesFromDB(n)
	if err != nil {
		return nil, err
	}
	files[rulesFile] = rulesJSON
	names = append(names, rulesFile)

	sig := repairSignature()
	if err := initBareRepoWithFilesAs(NodeRepoPath(dataDir, n.id), files, RebuildMessage, sig); err != nil {
		return nil, fmt.Errorf("rebuild repo for %s: %w", n.slug, err)
	}

	sort.Strings(names)
	return names, nil
}

// restoreRepo brings an existing repo back in line with the database without
// touching anything that already agrees with it. Each corrected file gets its
// own commit, because here the surrounding history is real and GetHistory
// filters by filename: a document that was already current must not gain an
// entry in its log for a neighbour's repair.
func restoreRepo(dataDir string, n repairNode, docs []repairDoc) ([]string, error) {
	var written []string

	for _, d := range docs {
		if cur, err := GetDocument(dataDir, n.id, d.filename); err == nil && cur == d.body {
			continue
		}
		if _, err := DirectEdit(dataDir, n.id, d.filename, d.body,
			RepairAuthorName, RepairAuthorEmail, restoreMessage(d.title)); err != nil {
			return written, fmt.Errorf("restore %q for %s: %w", d.filename, n.slug, err)
		}
		written = append(written, d.filename)
	}

	// The rules file is only ever restored when it is absent. Git is its
	// canonical store and the database column is the derived cache, so a
	// difference between them is ordinary cache lag, not damage — rewriting
	// on a difference would mint a no-op commit on every healthy instance.
	if _, err := GetDocument(dataDir, n.id, rulesFile); err != nil {
		rulesJSON, err := rulesFromDB(n)
		if err != nil {
			return written, err
		}
		if _, err := DirectEdit(dataDir, n.id, rulesFile, rulesJSON,
			RepairAuthorName, RepairAuthorEmail, restoreMessage("governance rules")); err != nil {
			return written, fmt.Errorf("restore rules for %s: %w", n.slug, err)
		}
		written = append(written, rulesFile)
	}

	return written, nil
}

const rulesFile = "governance-rules.json"

// Filename converts a governance doc title to the kebab-case .md filename its
// git mirror lives under. This mapping is the only link between a canonical
// governance_docs row and its history in git (docs/adr/011), so it has one
// definition and every store crosses by it.
func Filename(title string) string {
	name := strings.ToLower(title)
	name = strings.ReplaceAll(name, " ", "-")
	var clean []byte
	for _, c := range []byte(name) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			clean = append(clean, c)
		}
	}
	return string(clean) + ".md"
}
