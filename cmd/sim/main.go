// Command sim drives a throwaway instance through time (docs/adr/096).
//
// The product has no notion of simulated time and never will. This tool
// moves the *world* instead: every stored instant slides into the past by the
// amount asked for, and then the product's own hourly passes run once — the
// election sweep and the reminder worker — so the database is in exactly the
// state the server would have found after that much real time. Personas are
// ordinary accounts with a fixed session token each, so a script or an agent
// can act as any of them from a cookie.
//
//	go run ./cmd/sim -db data/sim/patchwork.db personas cmd/sim/personas.example.yaml
//	go run ./cmd/sim -db data/sim/patchwork.db advance 30d
//	go run ./cmd/sim -db data/sim/patchwork.db status
//
// It refuses to touch a database that is not marked as a simulation, for the
// same reason cmd/seed refuses a database with real users in it.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

const (
	markerFile = "SIMULATION"
	clockFile  = "clock.json"
	// Persona emails live under this domain so the marker check can tell a
	// simulation's accounts from a community's.
	personaDomain = "sim.localhost"
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: sim [-db path] <command> [args]

  personas <file.yaml>   create (or refresh) persona accounts and their session tokens
  advance <duration>     move the world back by a duration (30d, 720h, 2w) and run the sweeps
  sweep                  run the election and reminder passes without moving anything
  status                 what every patch's governance is doing right now
  now                    the simulated date (real clock plus everything advanced so far)

`)
	flag.PrintDefaults()
}

func main() {
	dbPath := flag.String("db", "data/sim/patchwork.db", "path to the simulation database")
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	dir := filepath.Dir(*dbPath)

	var err error
	switch args[0] {
	case "personas":
		if len(args) != 2 {
			err = errors.New("personas needs a yaml file")
			break
		}
		err = cmdPersonas(*dbPath, dir, args[1])
	case "advance":
		if len(args) != 2 {
			err = errors.New("advance needs a duration, e.g. 30d")
			break
		}
		err = cmdAdvance(*dbPath, dir, args[1])
	case "sweep":
		err = cmdSweep(*dbPath, dir)
	case "status":
		err = cmdStatus(*dbPath, dir)
	case "now":
		err = cmdNow(dir)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("sim %s: %v", args[0], err)
	}
}

func open(dbPath, dir string) (*database.DB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	db, err := database.Open(dbPath, migrations)
	if err != nil {
		return nil, err
	}
	// Same derivations the server makes from the same path, so a sweep that
	// applies an amendment writes the repo the server reads.
	ap.SetDomain("localhost")
	governance.SetDataDir(dir)
	return db, nil
}

// requireSimulation is the guard. A database is a simulation once `personas`
// has marked it; nothing else in this tool will move a world it didn't mint.
func requireSimulation(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, markerFile)); err != nil {
		return fmt.Errorf("%s is not marked as a simulation (no %s file); run `sim personas` first", dir, markerFile)
	}
	return nil
}

// ---- personas --------------------------------------------------------------

type personaFile struct {
	Personas []persona `yaml:"personas"`
}

type persona struct {
	Username    string `yaml:"username"`
	DisplayName string `yaml:"display_name"`
	Email       string `yaml:"email"`
	Bio         string `yaml:"bio"`
	Role        string `yaml:"role"` // "admin" makes an instance admin; default member
}

type mintedPersona struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	Token       string `json:"token"`
	Cookie      string `json:"cookie"`
}

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

func cmdPersonas(dbPath, dir, file string) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var pf personaFile
	if err := yaml.Unmarshal(raw, &pf); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if len(pf.Personas) == 0 {
		return fmt.Errorf("%s lists no personas", file)
	}
	for i, p := range pf.Personas {
		if !usernameRe.MatchString(p.Username) {
			return fmt.Errorf("persona %d: username %q must be lowercase letters, digits, _ or -", i, p.Username)
		}
		if p.DisplayName == "" {
			return fmt.Errorf("persona %s: display_name is required", p.Username)
		}
		if p.Role != "" && p.Role != "admin" && p.Role != "member" {
			return fmt.Errorf("persona %s: role must be admin or member", p.Username)
		}
	}

	db, err := open(dbPath, dir)
	if err != nil {
		return err
	}
	defer db.Close()

	// The seed's rule, restated: refuse a database that holds people who are
	// not ours. The system sentinel (migration 015) doesn't count.
	var others int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users
		WHERE id != '00000000-0000-0000-0000-000000000000'
		  AND COALESCE(email,'') NOT LIKE '%@' || ?`, personaDomain).Scan(&others); err != nil {
		return err
	}
	if others > 0 {
		return fmt.Errorf("refusing: %s holds %d accounts that are not simulation personas", dbPath, others)
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	expires := time.Now().Add(10 * 365 * 24 * time.Hour).UTC().Format(time.RFC3339)
	var minted []mintedPersona
	for _, p := range pf.Personas {
		email := p.Email
		if email == "" {
			email = p.Username + "@" + personaDomain
		}
		role := p.Role
		if role == "" {
			role = "member"
		}
		var id string
		err := db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&id)
		if err != nil {
			id = auth.NewUUIDv7()
			if _, err := db.Exec(`INSERT INTO users (id, email, username, display_name, bio, role, created_at, updated_at, ap_id)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, email, p.Username, p.DisplayName, p.Bio, role, now, now, ap.UserAPID(ap.GetDomain(), id)); err != nil {
				return fmt.Errorf("persona %s: %w", p.Username, err)
			}
		} else if _, err := db.Exec(`UPDATE users SET display_name = ?, bio = ?, role = ? WHERE id = ?`,
			p.DisplayName, p.Bio, role, id); err != nil {
			return fmt.Errorf("persona %s: %w", p.Username, err)
		}

		token := "sim-" + p.Username
		hash := auth.HashToken(token)
		db.Exec(`DELETE FROM sessions WHERE token = ?`, hash)
		if _, err := db.Exec(`INSERT INTO sessions (id, user_id, token, expires_at, ip_address) VALUES (?, ?, ?, ?, ?)`,
			auth.NewUUIDv7(), id, hash, expires, "127.0.0.1"); err != nil {
			return fmt.Errorf("persona %s session: %w", p.Username, err)
		}
		minted = append(minted, mintedPersona{
			Username: p.Username, DisplayName: p.DisplayName, UserID: id, Token: token,
			Cookie: "patchwork_session=" + token,
		})
	}

	if err := os.WriteFile(filepath.Join(dir, markerFile),
		[]byte("This database is a governance simulation (docs/adr/096). cmd/sim may move its clock.\n"), 0o644); err != nil {
		return err
	}
	out, _ := json.MarshalIndent(minted, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "personas.json"), out, 0o644); err != nil {
		return err
	}

	fmt.Printf("%d personas ready in %s\n\n", len(minted), dbPath)
	fmt.Printf("  %-14s %-24s %s\n", "username", "display name", "session token")
	for _, m := range minted {
		fmt.Printf("  %-14s %-24s %s\n", m.Username, m.DisplayName, m.Token)
	}
	fmt.Printf("\nAct as one with the cookie, e.g.\n  curl -b 'patchwork_session=%s' -H 'X-Patchwork-Request: true' http://localhost:8097/api/v1/auth/me\nTokens are also in %s\n",
		minted[0].Token, filepath.Join(dir, "personas.json"))
	return nil
}

// ---- advance / sweep -------------------------------------------------------

type clockLedger struct {
	TotalHours float64      `json:"total_hours"`
	Entries    []clockEntry `json:"entries"`
}

type clockEntry struct {
	At string `json:"at"` // real time the advance ran
	By string `json:"by"`
}

func readLedger(dir string) (clockLedger, error) {
	var l clockLedger
	raw, err := os.ReadFile(filepath.Join(dir, clockFile))
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return l, err
	}
	return l, json.Unmarshal(raw, &l)
}

func writeLedger(dir string, l clockLedger) error {
	raw, _ := json.MarshalIndent(l, "", "  ")
	return os.WriteFile(filepath.Join(dir, clockFile), raw, 0o644)
}

// parseAdvance accepts Go durations plus the units a calendar is described in.
func parseAdvance(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	for suffix, unit := range map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour} {
		if strings.HasSuffix(s, suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(s, suffix), 64)
			if err != nil {
				return 0, fmt.Errorf("bad duration %q", s)
			}
			return time.Duration(n * float64(unit)), nil
		}
	}
	return time.ParseDuration(s)
}

func cmdAdvance(dbPath, dir, arg string) error {
	if err := requireSimulation(dir); err != nil {
		return err
	}
	by, err := parseAdvance(arg)
	if err != nil {
		return err
	}
	if by%(24*time.Hour) != 0 {
		fmt.Println("note: date-only columns (a seat's term end) move in whole days; a fractional day rounds down for them")
	}
	db, err := open(dbPath, dir)
	if err != nil {
		return err
	}
	defer db.Close()

	ledger, err := readLedger(dir)
	if err != nil {
		return err
	}

	// A month passes one day at a time. The server's passes run hourly, and
	// several of them only act inside a window — the "voting ends soon"
	// reminder fires for a deadline inside the next 24 hours, an election
	// opens nominations a lead time ahead of a term end. One 30-day jump
	// steps over every such window and reports a world in which none of
	// them happened. Daily steps with the sweeps between are close enough
	// to hourly for windows measured in days, and cheap.
	step := 24 * time.Hour
	if by < step {
		step = by
	}
	total := ShiftReport{By: by, Tables: map[string]int{}, Skipped: map[string]string{}}
	remaining := by
	for day := 1; remaining > 0; day++ {
		this := step
		if remaining < step {
			this = remaining
		}
		before := snapshot(db)
		report, err := shiftWorld(db.DB, this)
		if err != nil {
			return err
		}
		ledger.TotalHours += this.Hours()
		if err := writeLedger(dir, ledger); err != nil {
			return err
		}
		for t, n := range report.Tables {
			total.Tables[t] += n
		}
		total.Instants += report.Instants
		total.Skipped = report.Skipped

		runSweeps(db)
		after := snapshot(db)
		if diff := diffLines(before, after); len(diff) > 0 {
			fmt.Printf("day %d (%s):\n", day, simulatedNow(ledger).Format("Mon 2006-01-02"))
			for _, l := range diff {
				fmt.Println(l)
			}
		}
		remaining -= this
	}
	ledger.Entries = append(ledger.Entries, clockEntry{At: time.Now().UTC().Format(time.RFC3339), By: by.String()})
	if err := writeLedger(dir, ledger); err != nil {
		return err
	}

	fmt.Println()
	fmt.Print(total)
	for t, why := range total.Skipped {
		fmt.Printf("  %-32s kept (%s)\n", t, why)
	}
	fmt.Printf("\ntoday is %s; the world is %s\n",
		simulatedNow(ledger).Format("Mon 2006-01-02 15:04 UTC"), worldAge(ledger))
	return nil
}

func cmdSweep(dbPath, dir string) error {
	if err := requireSimulation(dir); err != nil {
		return err
	}
	db, err := open(dbPath, dir)
	if err != nil {
		return err
	}
	defer db.Close()
	before := snapshot(db)
	runSweeps(db)
	printDiff(before, snapshot(db))
	return nil
}

// runSweeps is the server's hourly work, run once, now. Both passes are the
// production functions — nothing here resolves anything the server wouldn't.
func runSweeps(db *database.DB) {
	n := notifications.NewNotifier(db)
	handler.SetNotifier(n)
	handler.SweepElections(db)
	handler.SweepProposals(db)
	notifications.RunReminders(n)
	// Handlers notify on a goroutine; give the in-app channel a moment to
	// land its rows before the process exits.
	time.Sleep(1500 * time.Millisecond)
}

// ---- snapshot & diff -------------------------------------------------------

type proposalState struct {
	Title, Node, Status, State string
}

type worldSnapshot struct {
	Proposals     map[string]proposalState
	Seats         int
	Notifications int
	Audit         int
}

func snapshot(db *database.DB) worldSnapshot {
	s := worldSnapshot{Proposals: map[string]proposalState{}}
	rows, err := db.Query(`SELECT p.id, p.title, n.slug, p.status, COALESCE(p.state,'')
	                       FROM proposals p JOIN nodes n ON n.id = p.node_id`)
	if err == nil {
		for rows.Next() {
			var id string
			var ps proposalState
			if rows.Scan(&id, &ps.Title, &ps.Node, &ps.Status, &ps.State) == nil {
				s.Proposals[id] = ps
			}
		}
		rows.Close()
	}
	db.QueryRow(`SELECT COUNT(*) FROM seats`).Scan(&s.Seats)
	db.QueryRow(`SELECT COUNT(*) FROM notifications`).Scan(&s.Notifications)
	db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&s.Audit)
	return s
}

func printDiff(a, b worldSnapshot) {
	fmt.Println("after the sweeps:")
	lines := diffLines(a, b)
	if len(lines) == 0 {
		fmt.Println("  nothing moved")
		return
	}
	for _, l := range lines {
		fmt.Println(l)
	}
}

// diffLines is what the sweeps changed between two snapshots, one line each;
// empty when nothing moved.
func diffLines(a, b worldSnapshot) []string {
	var out []string
	for id, after := range b.Proposals {
		before, existed := a.Proposals[id]
		switch {
		case !existed:
			out = append(out, fmt.Sprintf("  new proposal   %-28s %-24q %s/%s", after.Node, after.Title, after.Status, after.State))
		case before.Status != after.Status || before.State != after.State:
			out = append(out, fmt.Sprintf("  proposal       %-28s %-24q %s/%s -> %s/%s", after.Node, after.Title, before.Status, before.State, after.Status, after.State))
		}
	}
	if d := b.Seats - a.Seats; d != 0 {
		out = append(out, fmt.Sprintf("  seats          %+d", d))
	}
	if d := b.Notifications - a.Notifications; d != 0 {
		out = append(out, fmt.Sprintf("  notifications  %+d", d))
	}
	if d := b.Audit - a.Audit; d != 0 {
		out = append(out, fmt.Sprintf("  audit entries  %+d", d))
	}
	return out
}

// ---- status / now ----------------------------------------------------------

// simulatedNow is the world's present, and it is simply now.
//
// This used to add the ledger's total, and that was double counting. Moving
// every stored instant *back* by a fortnight is what makes the world a
// fortnight older; the present it is older *than* is the real clock, because
// the server stamps every new row with the real clock and always will. Two
// simulated members caught the error before I did — a secretary's minutes of
// "28 September" came back stamped the 15th, and she rightly said a record
// that argues with its own dates is worth less than the notebook it came
// out of. The record was right and the tool was wrong.
func simulatedNow(l clockLedger) time.Time {
	return time.Now().UTC()
}

// worldAge is how far the world has been moved, which is the number worth
// printing beside the date: the content is this much older than today.
func worldAge(l clockLedger) string {
	days := l.TotalHours / 24
	if days == 0 {
		return "unmoved"
	}
	return fmt.Sprintf("aged by %.0f day(s) across %d advance(s)", days, len(l.Entries))
}

func cmdNow(dir string) error {
	l, err := readLedger(dir)
	if err != nil {
		return err
	}
	fmt.Printf("today     %s\nthe world %s\n\nThis is the date to give a persona: the server stamps everything it\nwrites from here with this clock. The age is carried by the content\nalready in the world, not by the calendar.\n",
		simulatedNow(l).Format("Mon 2006-01-02 15:04 UTC"), worldAge(l))
	return nil
}

func cmdStatus(dbPath, dir string) error {
	db, err := open(dbPath, dir)
	if err != nil {
		return err
	}
	defer db.Close()
	l, _ := readLedger(dir)
	fmt.Printf("today %s; the world is %s\n\n",
		simulatedNow(l).Format("Mon 2006-01-02 15:04 UTC"), worldAge(l))

	rows, err := db.Query(`SELECT id, slug, name, membership_policy, COALESCE(governance_config,'{}')
	                       FROM nodes WHERE status = 'active' AND removed_at IS NULL ORDER BY name`)
	if err != nil {
		return err
	}
	type node struct{ id, slug, name, policy, gc string }
	var nodes []node
	for rows.Next() {
		var n node
		if rows.Scan(&n.id, &n.slug, &n.name, &n.policy, &n.gc) == nil {
			nodes = append(nodes, n)
		}
	}
	rows.Close()
	if len(nodes) == 0 {
		fmt.Println("no active patches yet")
		return nil
	}

	for _, n := range nodes {
		var gc model.GovernanceConfig
		json.Unmarshal([]byte(n.gc), &gc)
		venue := func(v string) string {
			if v == "" {
				return "patchwork"
			}
			return v
		}
		fmt.Printf("%s (%s)  %s · leadership %s@%s · proposals %s · decide %s",
			n.name, n.slug, n.policy, or(gc.LeadershipModel, "unset"), venue(gc.LeadershipVenue),
			venue(gc.ProposalVenue), or(gc.DecisionMethod, "unset"))
		if gc.AdminTermMonths > 0 {
			fmt.Printf(" · terms %dmo", gc.AdminTermMonths)
		}
		fmt.Println()

		fmt.Printf("  admins:   %s\n", strings.Join(queryStrings(db,
			`SELECT u.username FROM memberships m JOIN users u ON u.id = m.user_id
			 WHERE m.node_id = ? AND m.role = 'admin' AND m.status = 'active' ORDER BY u.username`, n.id), ", "))
		if seats := queryStrings(db,
			`SELECT u.username || ' until ' || COALESCE(s.term_ends_at, 'next election')
			 FROM seats s JOIN users u ON u.id = s.holder_id WHERE s.node_id = ? ORDER BY s.term_ends_at`, n.id); len(seats) > 0 {
			fmt.Printf("  seats:    %s\n", strings.Join(seats, ", "))
		}
		var members, pending int
		db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role IN ('admin','member') AND status = 'active'`, n.id).Scan(&members)
		db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND status = 'pending'`, n.id).Scan(&pending)
		fmt.Printf("  people:   %d voting, %d waiting to be admitted\n", members, pending)

		prows, err := db.Query(`SELECT title, status, COALESCE(state,''), COALESCE(voting_ends_at,''),
		                               COALESCE(nominations_close_at,''), seats_contested
		                        FROM proposals WHERE node_id = ? AND status = 'open' ORDER BY created_at`, n.id)
		if err == nil {
			for prows.Next() {
				var title, status, state, ends, noms string
				var seats int
				if prows.Scan(&title, &status, &state, &ends, &noms, &seats) != nil {
					continue
				}
				kind := "proposal"
				if seats > 0 {
					kind = fmt.Sprintf("election (%d seats)", seats)
				}
				fmt.Printf("  open:     %-22s %q  %s", kind, title, or(state, status))
				if noms != "" {
					fmt.Printf("  nominations close %s", noms)
				}
				if ends != "" {
					fmt.Printf("  voting ends %s", ends)
				}
				fmt.Println()
			}
			prows.Close()
		}
		fmt.Println()
	}
	return nil
}

func queryStrings(db *database.DB, q string, args ...any) []string {
	rows, err := db.Query(q, args...)
	if err != nil {
		return []string{"(" + err.Error() + ")"}
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			out = append(out, s)
		}
	}
	return out
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
