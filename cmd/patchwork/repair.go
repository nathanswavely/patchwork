package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

// runGovernanceRepair rebuilds the bare governance repos from the database and
// prints what it did, then exits (docs/adr/084).
//
// It lives as a mode of the server binary rather than a cmd/repair of its own
// for one deciding reason: the distroless runtime image ships exactly one
// executable, /patchwork. A separate binary would be unrunnable in the
// deployment where this is most needed — a Docker instance restored from a
// database backup alone. Riding along here also means the repair reads the
// same patchwork.yaml the server does and derives the data directory by the
// same rule (the database file's parent), so it cannot repair a different
// directory from the one the server will open.
//
// Run it with the server stopped: it commits into the repos the running
// process would be writing to.
func runGovernanceRepair(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "repair: config: %v\n", err)
		os.Exit(1)
	}

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		fmt.Fprintf(os.Stderr, "repair: migrations: %v\n", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg.Database.Path, migrations)
	if err != nil {
		fmt.Fprintf(os.Stderr, "repair: database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	dataDir := filepath.Dir(cfg.Database.Path)
	governance.SetDataDir(dataDir)

	fmt.Printf("Repairing governance repos under %s\n\n", filepath.Join(dataDir, "governance"))

	report, err := governance.Repair(db, dataDir, governance.RepairOptions{RestoreExisting: true})
	if err != nil {
		printRepairReport(report)
		fmt.Fprintf(os.Stderr, "\nrepair: %v\n", err)
		os.Exit(1)
	}

	printRepairReport(report)
}

func printRepairReport(report *governance.Report) {
	if report == nil {
		return
	}

	fmt.Printf("  %-40s %s\n", "(instance baseline)", report.InstanceRepo)
	for _, r := range report.Repos {
		fmt.Printf("  %-40s %s", r.Slug, r.Status)
		if len(r.Files) > 0 {
			fmt.Printf("  (%d file(s))", len(r.Files))
		}
		fmt.Println()
		for _, f := range r.Files {
			fmt.Printf("  %-40s   %s\n", "", f)
		}
	}

	checked, created, updated, unchanged := report.Counts()
	fmt.Printf("\n%d repo(s) checked: %d created, %d updated, %d unchanged\n",
		checked, created, updated, unchanged)
	if created > 0 || updated > 0 {
		fmt.Println("Rebuilt and restored versions are labelled in each charter's history view.")
	}
}
