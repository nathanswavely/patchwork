package database_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Every value the code writes to a CHECK-constrained column is a value that
// column accepts.
//
// Removing a member was broken from migration 004 until 065. UpdateMember
// wrote `UPDATE memberships SET status = 'banned'`, but the column's CHECK
// listed only ('active','pending','left'). Every ban failed the constraint:
// the admin got a 500 and the person stayed an active member. Five downstream
// behaviours were dead code in consequence — JoinNode's 403 for a removed
// person, the reinstate branch, is_banned on the patch page, the SPA's removal
// notice, and the admin-only ?status=banned filter. None of them were wrong.
// None of them could ever be true.
//
// Nothing caught it because nothing could: the schema and the SQL that writes
// to it are checked by different things, and neither one is wrong on its own.
// The write only fails when the two are read together, at runtime, on a path
// no test covered. This test reads them together.
//
// It is deliberately anchored on SQL statements rather than on Go identifiers.
// A statement names its table, so `UPDATE memberships SET status = 'banned'`
// attributes exactly; a variable named `existingStatus` does not, and column
// names like `status` and `role` are shared across tables whose allowed sets
// differ — `moderator` is a value users.role accepts and memberships.role
// forbids.
//
// Attribution never guesses. A qualified column resolves through the
// statement's aliases to one table. A bare one belongs to every table in the
// statement carrying that column, and if any of those leaves it unconstrained
// the reference is dropped rather than pinned on a neighbour — which is what
// makes `n.status = 'unclaimed'` beside `m.status = 'active'` quiet, nodes
// having no CHECK on status at all. The cost of that conservatism is a missed
// bug in an ambiguous join; the alternative was a wall of false positives,
// which is how a guard test gets deleted.
func TestEveryWrittenEnumValueIsAllowed(t *testing.T) {
	allowed, columns := schemaFacts(t)
	if len(allowed) == 0 {
		t.Fatal("no CHECK (col IN (...)) constraints found — the scanner is broken, not the schema")
	}

	// Literals a statement writes or compares that its column does not accept,
	// but that are correct as written. Empty today; an entry needs a reason.
	type exception struct{ table, col, value string }
	permitted := map[exception]string{}

	seen := map[string]bool{}
	exercised := map[string]bool{}
	stmtCount := 0
	totalPairs := 0
	for _, cols := range allowed {
		totalPairs += len(cols)
	}
	var violations []string
	for _, stmt := range sqlLiterals(t, repoRoot(t)) {
		stmtCount++
		scope := tablesIn(stmt.sql)
		if len(scope.tables) == 0 {
			continue
		}
		for _, use := range columnValues(stmt.sql, scope) {
			owners := ownersOf(use, scope, columns)
			if len(owners) == 0 {
				continue
			}

			// If any owner leaves the column unconstrained, the value may
			// legitimately be that table's. Prove nothing rather than guess.
			unconstrained := false
			accepted := false
			for _, tbl := range owners {
				vals, constrained := allowed[tbl][use.col]
				if !constrained {
					unconstrained = true
					break
				}
				if vals[use.value] {
					accepted = true
				}
			}
			for _, tbl := range owners {
				if _, c := allowed[tbl][use.col]; c {
					exercised[tbl+"."+use.col] = true
				}
			}
			if unconstrained || accepted {
				continue
			}

			sort.Strings(owners)
			if _, fine := permitted[exception{owners[0], use.col, use.value}]; fine {
				continue
			}
			var sets []string
			for _, c := range owners {
				sets = append(sets, fmt.Sprintf("%s.%s accepts %v", c, use.col, sorted(allowed[c][use.col])))
			}
			v := fmt.Sprintf(
				"%s: uses %s = %q, which %s does not accept\n"+
					"      %s\n"+
					"      statement: %s",
				stmt.pos, use.col, use.value, owners[0], strings.Join(sets, "; "), truncate(stmt.sql))
			if seen[v] {
				continue
			}
			seen[v] = true
			violations = append(violations, v)
		}
	}

	// A scanner that reads nothing passes forever. These floors sit well under
	// what the tree holds today (884 statements, 17 of 21 constrained columns
	// reached — the other four are only ever written through bound parameters,
	// where no literal exists to check). They are here to fail loudly if the
	// walk, the parser, or the attribution ever quietly stops finding things,
	// not to be tightened as the codebase grows.
	if stmtCount < 400 {
		t.Errorf("only %d SQL statements found in the tree; the source scan has gone blind", stmtCount)
	}
	if len(exercised) < 12 {
		t.Errorf("only %d of %d constrained columns were reached (%v); attribution has gone blind",
			len(exercised), totalPairs, sorted(exercised))
	}

	sort.Strings(violations)
	for _, v := range violations {
		t.Errorf("%s\n"+
			"      Either widen the CHECK in a new migration (SQLite cannot alter one in\n"+
			"      place — rebuild the table, as migrations/065 did), or write a value the\n"+
			"      column accepts. A value the constraint rejects is not a write that fails\n"+
			"      loudly: it is a feature that silently never worked.", v)
	}
}

// ---- schema ----

var checkRe = regexp.MustCompile(`(?is)CHECK\s*\(\s*(\w+)\s+IN\s*\(([^)]*)\)\s*\)`)

// schemaFacts returns, from a freshly migrated database, the values each
// CHECK-constrained column allows and the full column list of every table.
// The second is what keeps attribution honest: a statement joining nodes and
// memberships has two candidate owners for a bare `status`, and only one of
// them constrains it.
func schemaFacts(t *testing.T) (allowed map[string]map[string]map[string]bool, columns map[string]map[string]bool) {
	t.Helper()
	tmp, err := os.CreateTemp("", "schema-enum-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(tmp.Name(), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	rows, err := db.Query(`SELECT name, sql FROM sqlite_master
	                       WHERE type = 'table' AND sql LIKE '%CHECK%'`)
	if err != nil {
		t.Fatal(err)
	}
	allowed = map[string]map[string]map[string]bool{}
	for rows.Next() {
		var name, sql string
		if rows.Scan(&name, &sql) != nil {
			continue
		}
		for _, m := range checkRe.FindAllStringSubmatch(sql, -1) {
			col := strings.ToLower(m[1])
			vals := map[string]bool{}
			for _, v := range litRe.FindAllStringSubmatch(m[2], -1) {
				vals[v[1]] = true
			}
			if len(vals) == 0 {
				continue
			}
			if allowed[name] == nil {
				allowed[name] = map[string]map[string]bool{}
			}
			allowed[name][col] = vals
		}
	}
	rows.Close()

	columns = map[string]map[string]bool{}
	tbls, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tbls.Next() {
		var n string
		if tbls.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	tbls.Close()
	for _, n := range names {
		cols, err := db.Query(fmt.Sprintf("PRAGMA table_info(%q)", n))
		if err != nil {
			t.Fatal(err)
		}
		columns[n] = map[string]bool{}
		for cols.Next() {
			var cid int
			var cname, ctype string
			var notnull, pk int
			var dflt any
			if cols.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk) == nil {
				columns[n][strings.ToLower(cname)] = true
			}
		}
		cols.Close()
	}
	return allowed, columns
}

// ---- source ----

type sqlStmt struct {
	pos string
	sql string
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected the repo root at %s: %v", root, err)
	}
	return root
}

var sqlVerb = regexp.MustCompile(`(?is)\b(INSERT\s+INTO|UPDATE|SELECT|DELETE\s+FROM)\b`)

// sqlLiterals collects every string constant in the non-test Go source that
// looks like SQL. go/parser rather than a grep, so quoting and escapes are the
// language's problem and not this test's.
func sqlLiterals(t *testing.T, root string) []sqlStmt {
	t.Helper()
	var out []sqlStmt
	fset := token.NewFileSet()

	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			// Test files may hand-craft rows to set up a fixture; the schema
			// enforces itself there, loudly, when they run.
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				t.Errorf("parse %s: %v", path, perr)
				return nil
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, uerr := strconv.Unquote(lit.Value)
				if uerr != nil || !sqlVerb.MatchString(s) {
					return true
				}
				rel, _ := filepath.Rel(root, path)
				p := fset.Position(lit.Pos())
				out = append(out, sqlStmt{pos: fmt.Sprintf("%s:%d", rel, p.Line), sql: s})
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	return out
}

// ---- statement shredding ----

var (
	// A table and, when present, the alias it is bound to for this statement.
	tableRe = regexp.MustCompile(`(?is)\b(?:INSERT\s+INTO|UPDATE|FROM|JOIN)\s+([a-z_][a-z0-9_]*)(?:\s+(?:AS\s+)?([a-z_][a-z0-9_]*))?`)
	// [qualifier.]col = 'lit' / != 'lit'.
	eqRe = regexp.MustCompile(`(?is)(?:\b([a-z_][a-z0-9_]*)\.)?\b([a-z_][a-z0-9_]*)\s*(?:=|!=|<>)\s*'([^']*)'`)
	// [qualifier.]col IN ('a', 'b') — not col IN (?, ?).
	inRe     = regexp.MustCompile(`(?is)(?:\b([a-z_][a-z0-9_]*)\.)?\b([a-z_][a-z0-9_]*)\s+IN\s*\(\s*('[^)]*)\)`)
	insertRe = regexp.MustCompile(`(?is)INSERT\s+(?:OR\s+\w+\s+)?INTO\s+([a-z_][a-z0-9_]*)\s*\(([^)]*)\)\s*VALUES\s*\((.*)`)
	quotedRe = regexp.MustCompile(`^'([^']*)'$`)
	litRe    = regexp.MustCompile(`'([^']*)'`)
)

// SQL keywords that can follow a table name and must not be read as an alias.
var notAnAlias = map[string]bool{
	"set": true, "where": true, "on": true, "values": true, "select": true,
	"left": true, "inner": true, "outer": true, "right": true, "cross": true,
	"join": true, "group": true, "order": true, "limit": true, "having": true,
	"union": true, "using": true, "as": true, "and": true, "or": true,
	"natural": true, "where_": true,
}

// scope is the set of tables a statement names, and the names — aliases and
// bare table names alike — by which its columns can refer to them.
type scope struct {
	tables []string
	alias  map[string]string
}

func tablesIn(sql string) scope {
	sc := scope{alias: map[string]string{}}
	seen := map[string]bool{}
	for _, m := range tableRe.FindAllStringSubmatch(sql, -1) {
		table := strings.ToLower(m[1])
		if notAnAlias[table] {
			continue
		}
		if !seen[table] {
			seen[table] = true
			sc.tables = append(sc.tables, table)
		}
		sc.alias[table] = table
		if a := strings.ToLower(m[2]); a != "" && !notAnAlias[a] {
			sc.alias[a] = table
		}
	}
	return sc
}

type colValue struct{ qualifier, col, value string }

// columnValues pulls every (column, literal) pair a statement pins down:
// equality and IN comparisons anywhere, plus the positional values of an
// INSERT, which name no column beside their value.
func columnValues(sql string, sc scope) []colValue {
	var out []colValue
	for _, m := range eqRe.FindAllStringSubmatch(sql, -1) {
		out = append(out, colValue{strings.ToLower(m[1]), strings.ToLower(m[2]), m[3]})
	}
	for _, m := range inRe.FindAllStringSubmatch(sql, -1) {
		q, col := strings.ToLower(m[1]), strings.ToLower(m[2])
		for _, v := range litRe.FindAllStringSubmatch(m[3], -1) {
			out = append(out, colValue{q, col, v[1]})
		}
	}
	if m := insertRe.FindStringSubmatch(sql); m != nil {
		table := strings.ToLower(m[1])
		cols := splitTopLevel(m[2])
		vals := splitTopLevel(closingGroup(m[3]))
		if len(cols) == len(vals) {
			for i, c := range cols {
				if q := quotedRe.FindStringSubmatch(strings.TrimSpace(vals[i])); q != nil {
					// An INSERT names its table, so qualify it explicitly.
					out = append(out, colValue{table, strings.ToLower(strings.TrimSpace(c)), q[1]})
				}
			}
		}
	}
	return out
}

// closingGroup returns the text up to the paren that closes the VALUES list.
func closingGroup(s string) string {
	depth, inQuote := 0, false
	for i, r := range s {
		switch {
		case r == '\'':
			inQuote = !inQuote
		case inQuote:
		case r == '(':
			depth++
		case r == ')':
			if depth == 0 {
				return s[:i]
			}
			depth--
		}
	}
	return s
}

// splitTopLevel splits on commas that are not inside parens or quotes, so
// strftime('%Y-%m-%dT%H:%M:%fZ', 'now') stays one element.
func splitTopLevel(s string) []string {
	var out []string
	depth, inQuote, start := 0, false, 0
	for i, r := range s {
		switch {
		case r == '\'':
			inQuote = !inQuote
		case inQuote:
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ',' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func truncate(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

// ownersOf resolves which table a (possibly qualified) column belongs to.
// A qualified column resolves to exactly one table; a bare one to every table
// in scope carrying a column of that name. An empty result means the reference
// could not be attributed and nothing may be concluded from it.
func ownersOf(use colValue, sc scope, columns map[string]map[string]bool) []string {
	if use.qualifier != "" {
		tbl, known := sc.alias[use.qualifier]
		if !known {
			return nil // a CTE or subquery alias, not a table we know
		}
		return []string{tbl}
	}
	var owners []string
	for _, tbl := range sc.tables {
		if columns[tbl][use.col] {
			owners = append(owners, tbl)
		}
	}
	return owners
}

// The guard above is only worth its failure message if it attributes columns to
// the right table. These are the shapes that made the first draft cry wolf:
// a column qualified by an alias, a checked column joined beside an unchecked
// one of the same name, and an INSERT whose values include a function call.
//
// If a future false positive tempts someone to loosen the scanner, loosen it
// here first and watch what stops being caught.
func TestSQLAttribution(t *testing.T) {
	// Just enough schema to attribute against.
	columns := map[string]map[string]bool{
		"memberships": {"id": true, "role": true, "status": true, "node_id": true},
		"nodes":       {"id": true, "status": true, "slug": true},
		"events":      {"id": true, "status": true},
		"proposals":   {"id": true, "status": true},
		"users":       {"id": true, "role": true},
	}

	tests := []struct {
		name string
		sql  string
		want []string // "table.col=value" per attributed literal
	}{{
		name: "unqualified column in a single-table update",
		sql:  `UPDATE memberships SET status = 'banned' WHERE id = ?`,
		want: []string{"memberships.status=banned"},
	}, {
		name: "alias resolves to its table, not to the other one in the join",
		sql:  `SELECT n.id FROM nodes n JOIN memberships m ON m.node_id = n.id WHERE n.status = 'unclaimed' AND m.status = 'active'`,
		want: []string{"nodes.status=unclaimed", "memberships.status=active"},
	}, {
		name: "bare table name works as its own qualifier",
		sql:  `SELECT id FROM nodes WHERE nodes.status = 'archived'`,
		want: []string{"nodes.status=archived"},
	}, {
		name: "a bare column in a join belongs to every candidate",
		sql:  `SELECT e.id FROM events e JOIN memberships ON 1=1 WHERE status = 'pending_review'`,
		want: []string{"events.status=pending_review", "memberships.status=pending_review"},
	}, {
		name: "insert attributes positionally and ignores function arguments",
		sql:  `INSERT INTO memberships (id, role, status, joined_at) VALUES (?, 'follower', 'banned', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
		want: []string{"memberships.role=follower", "memberships.status=banned"},
	}, {
		name: "IN-list expands to one literal each",
		sql:  `SELECT id FROM proposals WHERE status IN ('open', 'approved')`,
		want: []string{"proposals.status=open", "proposals.status=approved"},
	}, {
		// The literal here is qualified by a subquery alias, which names no
		// table. Attributing it to nodes because nodes is the only table in
		// sight is exactly the guess that produced the first draft's false
		// positives, so it is left unattributed instead.
		name: "a subquery alias is not attributed to the table behind it",
		sql:  `SELECT id FROM (SELECT id, status FROM nodes) sub WHERE sub.status = 'unclaimed'`,
		want: nil,
	}, {
		name: "bound parameters are not literals",
		sql:  `UPDATE memberships SET status = ?, role = ? WHERE id = ?`,
		want: nil,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := tablesIn(tt.sql)
			var got []string
			for _, use := range columnValues(tt.sql, sc) {
				for _, owner := range ownersOf(use, sc, columns) {
					if !columns[owner][use.col] {
						continue // the column is not that table's
					}
					got = append(got, fmt.Sprintf("%s.%s=%s", owner, use.col, use.value))
				}
			}
			sort.Strings(got)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Errorf("attribution mismatch\n got: %v\nwant: %v", got, want)
			}
		})
	}
}
