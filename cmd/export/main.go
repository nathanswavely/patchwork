// Command export writes a Patchwork instance's portable data to a directory
// of JSON files (the seamrip mechanism). See internal/seamrip for the export
// scope. Import the result with cmd/import.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/seamrip"
)

func main() {
	dbPath := flag.String("db", "data/patchwork.db", "path to patchwork database")
	outDir := flag.String("out", "./export", "output directory")
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

	if err := os.MkdirAll(*outDir, 0750); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	var totalSize int64
	var fileCount int

	writeFile(*outDir, "instance.json", map[string]interface{}{
		"name":    "Patchwork Export",
		"version": "dev",
	}, &totalSize, &fileCount)

	err = seamrip.Export(db, func(t seamrip.Table, items []map[string]any) error {
		fmt.Printf("  %-32s %d records\n", t.File, len(items))
		writeFile(*outDir, t.File, items, &totalSize, &fileCount)
		return nil
	})
	if err != nil {
		log.Fatalf("export: %v", err)
	}

	if err := os.WriteFile(filepath.Join(*outDir, "README.txt"), []byte(seamrip.ReadmeText), 0640); err == nil {
		fileCount++
	}

	fmt.Printf("\nExport complete:\n")
	fmt.Printf("  Files:      %d\n", fileCount)
	fmt.Printf("  Total size: %s\n", formatBytes(totalSize))
	fmt.Printf("  Output dir: %s\n", *outDir)
}

func writeFile(dir, name string, data interface{}, totalSize *int64, fileCount *int) {
	path := filepath.Join(dir, name)
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("warning: marshal %s: %v", name, err)
		return
	}
	if err := os.WriteFile(path, b, 0640); err != nil {
		log.Printf("warning: write %s: %v", name, err)
		return
	}
	*totalSize += int64(len(b))
	*fileCount++
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}
