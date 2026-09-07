// Command import loads a seamrip export (cmd/export or the admin zip,
// unpacked) into a fresh Patchwork database. Every record gets a new ID;
// relationships — including the shared-membership overlap that threads and
// the quilt are inferred from — are preserved. The old→new ID mapping is
// written to id_map.json in the input directory.
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

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/seamrip"
)

func main() {
	dbPath := flag.String("db", "new-patchwork.db", "path to new database")
	inDir := flag.String("in", "./export", "input directory")
	flag.Parse()

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		log.Fatalf("migrations fs: %v", err)
	}

	db, err := database.Open(*dbPath, migrations)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	read := func(file string) ([]map[string]any, error) {
		data, err := os.ReadFile(filepath.Join(*inDir, file))
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("warning: %s missing, skipping", file)
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var items []map[string]any
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
		return items, nil
	}

	// Which bundle this is, before anything is written. Both kinds import
	// with the same code, and they arrive at very different places: a full
	// seamrip carries email addresses, so people sign in on the new quilt
	// by magic link, while a member seamrip carries none and every person
	// in it has to be invited back (docs/adr/089). Saying so here is the
	// difference between a fork that knows it must invite its community and
	// one that finds out when nobody can sign in.
	kind, requestedBy := readKind(*inDir)
	if kind == seamrip.KindMember {
		fmt.Printf("Reading a MEMBER SEAMRIP")
		if requestedBy != "" {
			fmt.Printf(", taken by %s", requestedBy)
		}
		fmt.Print(".\n")
		fmt.Println("  It holds one member's view: no email addresses, no hidden")
		fmt.Println("  memberships, no noticeboards. People arrive as stubs and join")
		fmt.Println("  this quilt by invitation.")
		fmt.Println()
	}

	idMap, results, err := seamrip.Import(db, read, auth.NewUUIDv7)
	if err != nil {
		log.Fatalf("import: %v", err)
	}

	// An archive taken before migration 066 carries the contact card as three
	// columns on users plus a boolean per membership (docs/adr/080). Nothing
	// reads those any more, so without this the cards would arrive on the
	// fork and be invisible — a silent loss in the one mechanism that exists
	// for a community to leave with what is theirs (docs/adr/002). The same
	// conversion startup runs, run once more now that the rows are here.
	if n, err := handler.BackfillContactItems(db); err != nil {
		log.Fatalf("import: converting contact cards: %v", err)
	} else if n > 0 {
		fmt.Printf("  %-28s %d converted from the pre-066 card\n", "contact_items", n)
	}

	for _, r := range results {
		line := fmt.Sprintf("  %-28s %d imported", r.Table, r.Imported)
		if r.Skipped > 0 {
			line += fmt.Sprintf(" (%d SKIPPED — see warnings)", r.Skipped)
		}
		fmt.Println(line)
	}

	// The Label does not travel (docs/adr/023) — a fork's Label would be
	// false on arrival. But a seamrip is the best moment anyone will ever
	// have to write one, so prefill the blank Label with a removable
	// "seamripped from" provenance line pointing at the origin quilt.
	if data, err := os.ReadFile(filepath.Join(*inDir, "instance.json")); err == nil {
		var origin struct {
			Name   string `json:"name"`
			Domain string `json:"domain"`
		}
		if json.Unmarshal(data, &origin) == nil && origin.Name != "" {
			originURL := ""
			if origin.Domain != "" {
				originURL = "https://" + origin.Domain
			}
			if _, err := db.Exec(`INSERT OR IGNORE INTO label
				(id, seamripped_from_name, seamripped_from_url) VALUES (1, ?, ?)`,
				origin.Name, originURL); err == nil {
				fmt.Printf("\n  Label:       blank, prefilled with a removable \"seamripped from %s\" line.\n"+
					"               Write yours at /admin/label\n", origin.Name)
			}
		}
	}

	// Write ID map.
	idMapPath := filepath.Join(*inDir, "id_map.json")
	idMapData, _ := json.MarshalIndent(idMap, "", "  ")
	os.WriteFile(idMapPath, idMapData, 0640)

	// Verify referential integrity.
	var integrityResult string
	db.QueryRow("PRAGMA integrity_check").Scan(&integrityResult)
	fmt.Printf("\n  Integrity:   %s\n", integrityResult)

	var fkViolations int
	db.QueryRow("PRAGMA foreign_key_check").Scan(&fkViolations)
	if fkViolations == 0 {
		fmt.Println("  FK check:    ok")
	} else {
		fmt.Printf("  FK check:    %d violations\n", fkViolations)
	}

	fmt.Printf("\nImport complete. ID mapping saved to %s\n", idMapPath)
	fmt.Println("ActivityPub identifiers and keypairs are minted on first server start.")
	if kind == seamrip.KindMember {
		fmt.Println("Nobody in this archive has an email address. Invite people back with")
		fmt.Println("invite links, and each person sets their own visibility here again.")
	}
}

// readKind reports which kind of archive this directory holds, and who took
// it when that is recorded. manifest.json is the member seamrip's own file;
// instance.json carries the same key, and an archive written before either
// existed has neither and is a full seamrip.
func readKind(dir string) (string, string) {
	var meta struct {
		Kind        string `json:"kind"`
		RequestedBy string `json:"requested_by"`
	}
	for _, name := range []string{"manifest.json", "instance.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if json.Unmarshal(data, &meta) == nil && meta.Kind != "" {
			return seamrip.KindFor(meta.Kind), meta.RequestedBy
		}
	}
	return seamrip.KindFull, ""
}
