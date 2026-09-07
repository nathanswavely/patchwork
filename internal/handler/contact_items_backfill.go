package handler

import (
	"fmt"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// BackfillContactItems moves migration 062's contact cards into the item
// shape docs/adr/083 gives them, and returns the number of items created.
//
// The conversion is exact: each non-empty column becomes one item, and every
// membership that had share_contact on gains one share row per item of that
// person's. Nobody's disclosure changes by a field — the same people can read
// the same values on the other side, which is the only acceptable outcome for
// a migration that moves somebody's phone number.
//
// It runs in Go rather than in migration 066 because new rows need UUIDv7 ids
// and SQLite cannot mint one, matching ap.BackfillAPIDs and
// BackfillNodeGovernanceRepos.
//
// NOT YET CALLED FROM main.go, deliberately. Because it clears the legacy
// columns as it reads them, calling it while the handlers still serve cards
// from users.contact_* would empty every card on the next boot. It is wired
// into startup in the same change that moves the read path onto items — that
// ordering is the whole reason it is safe to clear as we go.
//
// The legacy columns are cleared in the same transaction that reads them, so
// the data lives in exactly one place at every instant. That is what makes
// this idempotent: a second run sees nothing to convert. Guarding on "does
// this person have items yet" instead would resurrect items they deleted
// after upgrading, which is the one failure mode a contact feature must not
// have.
func BackfillContactItems(db *database.DB) (int, error) {
	// The legacy columns disappear in a later migration; until then a build
	// running against an already-retired schema must not fail to start.
	if !hasColumn(db, "users", "contact_phone") {
		return 0, nil
	}

	rows, err := db.Query(`SELECT id, contact_phone, contact_email, contact_note
		FROM users
		WHERE contact_phone != '' OR contact_email != '' OR contact_note != ''`)
	if err != nil {
		return 0, fmt.Errorf("list legacy cards: %w", err)
	}
	type card struct{ id, phone, email, note string }
	var cards []card
	for rows.Next() {
		var c card
		if err := rows.Scan(&c.id, &c.phone, &c.email, &c.note); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan legacy card: %w", err)
		}
		cards = append(cards, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read legacy cards: %w", err)
	}
	if len(cards) == 0 {
		// Still clear any stranded switches: a share_contact set on an empty
		// card showed nothing before and must grant nothing now.
		_, err := db.Exec(`UPDATE memberships SET share_contact = 0 WHERE share_contact = 1`)
		return 0, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	created := 0
	for _, c := range cards {
		// Ordered the way the old form read, so a person's card looks the
		// same on the other side.
		legacy := []struct{ kind, value string }{
			{model.ContactKindPhone, c.phone},
			{model.ContactKindEmail, c.email},
			{model.ContactKindNote, c.note},
		}
		var itemIDs []string
		pos := 0
		for _, l := range legacy {
			if l.value == "" {
				continue
			}
			id := auth.NewUUIDv7()
			if _, err := tx.Exec(
				`INSERT INTO contact_items (id, user_id, kind, value, position) VALUES (?, ?, ?, ?, ?)`,
				id, c.id, l.kind, l.value, pos,
			); err != nil {
				return 0, fmt.Errorf("insert item for %s: %w", c.id, err)
			}
			itemIDs = append(itemIDs, id)
			pos++
			created++
		}

		// Every patch the whole card was shared with gets every item of it.
		// Only member/admin rows ever showed a card, so only they carry over.
		shared, err := tx.Query(`SELECT node_id FROM memberships
			WHERE user_id = ? AND share_contact = 1
			  AND status = 'active' AND role IN ('member','admin')`, c.id)
		if err != nil {
			return 0, fmt.Errorf("list shares for %s: %w", c.id, err)
		}
		var nodeIDs []string
		for shared.Next() {
			var n string
			if err := shared.Scan(&n); err != nil {
				shared.Close()
				return 0, fmt.Errorf("scan share: %w", err)
			}
			nodeIDs = append(nodeIDs, n)
		}
		shared.Close()
		if err := shared.Err(); err != nil {
			return 0, fmt.Errorf("read shares for %s: %w", c.id, err)
		}
		for _, n := range nodeIDs {
			for _, item := range itemIDs {
				if _, err := tx.Exec(
					`INSERT OR IGNORE INTO contact_item_shares (item_id, node_id) VALUES (?, ?)`,
					item, n,
				); err != nil {
					return 0, fmt.Errorf("insert share: %w", err)
				}
			}
		}

		if _, err := tx.Exec(
			`UPDATE users SET contact_phone = '', contact_email = '', contact_note = '' WHERE id = ?`,
			c.id,
		); err != nil {
			return 0, fmt.Errorf("clear legacy card for %s: %w", c.id, err)
		}
	}

	if _, err := tx.Exec(`UPDATE memberships SET share_contact = 0 WHERE share_contact = 1`); err != nil {
		return 0, fmt.Errorf("clear legacy switches: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return created, nil
}

// hasColumn reports whether a table still carries a column. Used so a build
// can start against a schema that has already retired the legacy card.
func hasColumn(db *database.DB, table, column string) bool {
	rows, err := db.Query(`SELECT 1 FROM pragma_table_info(?) WHERE name = ?`, table, column)
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}
