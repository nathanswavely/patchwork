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

	idMap, results, err := seamrip.Import(db, read, auth.NewUUIDv7)
	if err != nil {
		log.Fatalf("import: %v", err)
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
}
