package seamrip

import (
	"fmt"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// SentinelUserID owns unclaimed patches. It exists in every database (created
// by migration 015) and is excluded from export, so it maps to itself.
const SentinelUserID = "00000000-0000-0000-0000-000000000000"

// ImportResult reports what happened per table.
type ImportResult struct {
	Table    string
	Imported int
	Skipped  int
}

// Import inserts previously exported data into db, minting a new ID for every
// record while preserving relationships. read returns the decoded rows for an
// export file (nil for a missing file — the table is skipped). newID mints
// fresh IDs (auth.NewUUIDv7 in production; injected to avoid an import
// cycle). It returns the old→new ID map alongside per-table counts.
func Import(db *database.DB, read func(file string) ([]map[string]any, error), newID func() string) (map[string]string, []ImportResult, error) {
	idMap := map[string]string{SentinelUserID: SentinelUserID}

	// remap rewrites an exported ID through the map, minting on first sight.
	// nil/empty stays nil so nullable FK columns import as NULL.
	remap := func(old any) any {
		s, ok := old.(string)
		if !ok || s == "" {
			return nil
		}
		if mapped, exists := idMap[s]; exists {
			return mapped
		}
		minted := newID()
		idMap[s] = minted
		return minted
	}

	var results []ImportResult
	for _, t := range Tables() {
		items, err := read(t.File)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", t.File, err)
		}
		if items == nil {
			results = append(results, ImportResult{Table: t.Name})
			continue
		}

		names := make([]string, len(t.Columns))
		placeholders := make([]string, len(t.Columns))
		for i, col := range t.Columns {
			names[i] = col.Name
			placeholders[i] = "?"
		}
		insert := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			t.Name, strings.Join(names, ", "), strings.Join(placeholders, ", "))

		res := ImportResult{Table: t.Name}
		// Rows can reference other rows in the same table (threaded comment
		// parent_id), so retry failures until a pass makes no progress.
		pending := items
		for len(pending) > 0 {
			var failed []map[string]any
			for _, item := range pending {
				args := make([]any, len(t.Columns))
				for i, col := range t.Columns {
					if col.Remap {
						args[i] = remap(item[col.Name])
					} else {
						args[i] = item[col.Name]
					}
				}
				if _, err := db.Exec(insert, args...); err != nil {
					failed = append(failed, item)
					continue
				}
				res.Imported++
			}
			if len(failed) == len(pending) {
				res.Skipped = len(failed)
				break
			}
			pending = failed
		}
		results = append(results, res)
	}

	return idMap, results, nil
}
