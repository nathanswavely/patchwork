package patchwork

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Both numbered ranges are closed. See
// docs/adr/2026-09-16-a-name-nobody-has-to-ask-for.md: a sequential number is
// claimed from a counter whose real state is the union of every branch in
// flight, which no branch can see, so two worktrees pick the same number, are
// both right locally, and merge clean. New records are named from the clock
// and a sentence instead, which every worktree can do alone.
//
// These are the last numbers ever issued. They are not "the highest so far".
//
// The cutover ADR closed the spaces at 115 and 074. Two branches were in
// flight at that moment and landed 116, 117 and 075 afterwards, which is the
// exact blind spot the ADR describes. They keep their names, because a
// merged migration cannot be renamed without a permanent renamedMigrations
// entry to rescue every database that already ran it, and the whole point of
// the cutover was to stop paying that kind of cost. So the spaces closed one
// step later than first written. See the 2026-09-18 addendum in that ADR.
const (
	lastNumberedADR       = 117
	lastNumberedMigration = 75
)

var (
	// 115-a-steward-holds-only-what-nobody-else-holds.md
	legacyADR = regexp.MustCompile(`^(\d{3})-[a-z0-9]+(-[a-z0-9]+)*\.md$`)
	// 2026-09-16-a-name-nobody-has-to-ask-for.md
	datedADR = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-[a-z0-9]+(-[a-z0-9]+)*\.md$`)
	// 074_suggested_tags.sql
	legacyMigration = regexp.MustCompile(`^(\d{3})_[a-z0-9]+(_[a-z0-9]+)*\.sql$`)
	// 20260916T143207_suggested_tags.sql
	stampedMigration = regexp.MustCompile(`^(\d{8}T\d{6})_[a-z0-9]+(_[a-z0-9]+)*\.sql$`)
)

// The realistic way this decays is mimicry: an agent reads 115 numbered files,
// pattern-matches, and writes 116-foo.md. That reads as consistent and is the
// one thing the cutover forbids. Nothing else here arbitrates anything — after
// the cutover there is no contended resource for two branches to race on.
func TestNewRecordsAreNotNumbered(t *testing.T) {
	t.Run("adr", func(t *testing.T) {
		seen := map[int]string{}
		for _, name := range dirEntries(t, "docs/adr", ".md") {
			if name == "README.md" {
				continue
			}

			if m := datedADR.FindStringSubmatch(name); m != nil {
				if _, err := time.Parse("2006-01-02", m[1]); err != nil {
					t.Errorf("ADR %s: %q is not a real date", name, m[1])
				}
				continue
			}

			m := legacyADR.FindStringSubmatch(name)
			if m == nil {
				t.Errorf("ADR %s is named neither YYYY-MM-DD-slug.md (new) nor NNN-slug.md (legacy); "+
					"new ADRs take today's date, see docs/adr/2026-09-16-a-name-nobody-has-to-ask-for.md", name)
				continue
			}

			number, _ := strconv.Atoi(m[1])
			if number > lastNumberedADR {
				t.Errorf("ADR %s takes number %03d, but the ADR number space closed at %03d; "+
					"name it %s-<slug>.md instead",
					name, number, lastNumberedADR, time.Now().Format("2006-01-02"))
			}
			if prior, dup := seen[number]; dup {
				t.Errorf("ADRs %s and %s share the number %03d", prior, name, number)
			}
			seen[number] = name
		}
	})

	t.Run("migration", func(t *testing.T) {
		for _, name := range dirEntries(t, "migrations", ".sql") {
			if m := stampedMigration.FindStringSubmatch(name); m != nil {
				if _, err := time.Parse("20060102T150405", m[1]); err != nil {
					t.Errorf("migration %s: %q is not a real timestamp", name, m[1])
				}
				continue
			}

			m := legacyMigration.FindStringSubmatch(name)
			if m == nil {
				t.Errorf("migration %s is named neither YYYYMMDDTHHMMSS_slug.sql (new) nor NNN_slug.sql "+
					"(legacy); see docs/adr/2026-09-16-a-name-nobody-has-to-ask-for.md", name)
				continue
			}

			number, _ := strconv.Atoi(m[1])
			if number > lastNumberedMigration {
				// Renaming it later is not a repair: the runner records the
				// whole filename, migrations are not idempotent, and Open's
				// error is fatal, so a number that reaches a database is a
				// rename away from a fleet that will not boot.
				t.Errorf("migration %s takes number %03d, but the migration number space closed at %03d; "+
					"name it %s_<slug>.sql instead",
					name, number, lastNumberedMigration, time.Now().UTC().Format("20060102T150405"))
			}
		}
	})
}

// Every new ADR carries its date in the filename, so the index is no longer
// the only place chronology lives. It is still the only place *status* lives,
// and an ADR missing from it is invisible to anyone reading the directory as a
// document rather than a folder.
func TestEveryADRIsInTheIndex(t *testing.T) {
	index, err := os.ReadFile("docs/adr/README.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(index)

	for _, name := range dirEntries(t, "docs/adr", ".md") {
		if name == "README.md" {
			continue
		}
		if !strings.Contains(body, name) {
			t.Errorf("ADR %s is not listed in docs/adr/README.md", name)
		}
	}
}

func dirEntries(t *testing.T, dir, suffix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		t.Fatalf("no %s files under %s", suffix, dir)
	}
	return names
}
