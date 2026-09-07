package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every conversion startup runs must also run after an import.
//
// `make import` builds a fork's database *after* the process has started, so
// a backfill wired into main.go has already run against an empty schema and
// will not run again. docs/adr/083 shipped that way: an archive taken before
// migration 068 carried the contact card as three columns, the conversion
// never ran on it, and every card in the archive arrived on the fork in
// columns nothing reads — silently, with the fork coming up looking complete.
//
// A seamrip is the mechanism a community has for leaving with what is theirs
// (docs/adr/002), so failing it quietly is the worst way for it to fail.
// This reads both entry points and requires they agree, or that the
// difference is stated here.
//
// Exempt is for a conversion that genuinely has no work to do on a fresh
// import — a repair for a state only a long-running instance reaches. Each
// needs its sentence.
var backfillsExemptFromImport = map[string]string{
	"BackfillAPIDs":               "federation identity is minted for the fork's own domain at its next boot, not carried in; an imported row has no ap_id to heal.",
	"BackfillKeypairs":            "keys never travel (docs/adr/002), so the fork generates its own on the boot that follows the import.",
	"BackfillNodeGovernanceRepos": "the import writes the repos as it writes the nodes; this heals an instance whose repo creation failed at runtime.",
}

func backfillCalls(t *testing.T, path string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// Package-qualified calls only, so a local helper named backfillSomething
	// is not mistaken for one.
	re := regexp.MustCompile(`\b(?:handler|ap)\.(Backfill\w+)\(`)
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	return out
}

func TestEveryStartupBackfillAlsoRunsOnImport(t *testing.T) {
	startup := backfillCalls(t, "../patchwork/main.go")
	if len(startup) == 0 {
		t.Fatal("found no backfills in cmd/patchwork/main.go — the pattern this test reads has changed")
	}
	onImport := backfillCalls(t, "main.go")

	var missing []string
	for name := range startup {
		if onImport[name] {
			continue
		}
		if reason, exempt := backfillsExemptFromImport[name]; exempt {
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%s is exempt from the import path with no reason given", name)
			}
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("%s runs at startup but not after an import: an archive's rows land after the "+
			"process has already booted, so the conversion never sees them and the fork comes up "+
			"looking complete. Call it in cmd/import, or add it to backfillsExemptFromImport with "+
			"the reason it has nothing to do on a fresh import", name)
	}

	// An exemption for something that is no longer a startup backfill is an
	// exemption nobody reads.
	for name := range backfillsExemptFromImport {
		if !startup[name] {
			t.Errorf("backfillsExemptFromImport names %q, which main.go does not call any more", name)
		}
	}
}
