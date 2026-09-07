package handler_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// personalTables derives, from the schema's own foreign keys, the set of
// tables whose rows are about one person.
//
// It starts at users and follows keys inward, but recurses ONLY through
// tables the boundary calls purged — the ones that are the person alone.
// That is what keeps the set meaningful: nodes points at users through
// owner_id and is kept, so the walk stops there instead of running on
// through events and proposals and swallowing the schema. contact_items is
// purged, so the walk goes on to contact_item_shares, which points at no
// person directly and is nonetheless entirely about one.
func personalTables(t *testing.T, db *database.DB, rules map[string]handler.DeletionRuleView) map[string]bool {
	t.Helper()

	var tables []string
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'
		AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			tables = append(tables, n)
		}
	}
	rows.Close()

	// references[target] = tables holding a foreign key into it.
	references := map[string][]string{}
	for _, name := range tables {
		fks, err := db.Query(`SELECT "table" FROM pragma_foreign_key_list(?)`, name)
		if err != nil {
			continue
		}
		for fks.Next() {
			var target string
			if fks.Scan(&target) == nil {
				references[target] = append(references[target], name)
			}
		}
		fks.Close()
	}

	personal := map[string]bool{"users": true}
	frontier := []string{"users"}
	// Tables whose reference to a person is not a declared foreign key still
	// have to answer; they are seeded here rather than found.
	for name := range handler.DeletionAlsoPersonal() {
		personal[name] = true
		if r, ok := rules[name]; ok && r.IsPurged {
			frontier = append(frontier, name)
		}
	}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		for _, child := range references[cur] {
			if personal[child] {
				continue
			}
			personal[child] = true
			// Only a purged table's children are themselves personal. An
			// undeclared table is treated as a leaf; the test reports it
			// rather than walking through a decision nobody made.
			if r, ok := rules[child]; ok && r.IsPurged {
				frontier = append(frontier, child)
			}
		}
	}
	return personal
}

// Every table whose rows are about a person says what deletion does with it.
//
// The failure this guards is the one docs/adr/086 built in and cannot see:
// the users row is kept as a tombstone, so ON DELETE CASCADE never fires,
// and a table added without a thought stays behind in full. contact_items
// shipped that way and left a phone number in the database after the person
// deleted their account — visible to nobody, because the memberships went,
// but not erased, which is what deletion promises.
//
// This is the third list a new table has to join by hand. The other two —
// the seamrip boundary and the member view — already fail the build when it
// does not. Now so does this one.
func TestEveryPersonalTableHasADeletionRule(t *testing.T) {
	db := setupTestDB(t)
	rules := handler.DeletionRules()
	personal := personalTables(t, db, rules)

	var undecided []string
	for name := range personal {
		if _, ok := rules[name]; !ok {
			undecided = append(undecided, name)
		}
	}
	sort.Strings(undecided)
	for _, name := range undecided {
		t.Errorf("table %q holds rows about a person and has no deletion rule: add it to "+
			"deletionRules() as purged (the rows are the person alone), kept (a community "+
			"record that outlives them), or emptied — and if purged, add the DELETE to the "+
			"purge list, because the tombstone means CASCADE will not fire for you", name)
	}

	// A rule declared for a table that does not exist is a rule nobody reads.
	for name := range rules {
		if !personal[name] {
			t.Errorf("deletionRules() declares %q, which the schema does not reach from users — "+
				"either the table is gone or the walk no longer finds it", name)
		}
	}

	// Every rule needs its sentence: the reasoning is the point, not the label.
	for name, r := range rules {
		if strings.TrimSpace(r.Why) == "" {
			t.Errorf("deletion rule for %q has no reason", name)
		}
	}
}

// A table the boundary calls purged must actually be purged.
//
// Declaring the rule and forgetting the DELETE is the same bug one step
// later, and it reads as done in the file where the decision lives.
func TestEveryPurgedTableIsActuallyPurged(t *testing.T) {
	statements := handler.DeletionPurgeStatements()
	for name, r := range handler.DeletionRules() {
		if !r.IsPurged {
			continue
		}
		found := false
		for _, q := range statements {
			if strings.Contains(q, "FROM "+name+" ") || strings.HasSuffix(q, "FROM "+name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("deletionRules() calls %q purged, but no statement in the purge list "+
				"deletes from it — the rule says the rows go and nothing takes them", name)
		}
	}
}
