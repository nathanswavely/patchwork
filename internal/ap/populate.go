package ap

import "github.com/patchwork-toolkit/patchwork/internal/database"

// PopulateAPIds fills in ap_id fields for any entities that don't have them yet.
// Called on startup to backfill existing data.
func PopulateAPIds(db *database.DB, domain string) error {
	if domain == "" {
		domain = "localhost"
	}

	// Users
	_, err := db.Exec(`UPDATE users SET ap_id = 'https://' || ? || '/ap/users/' || id WHERE ap_id IS NULL OR ap_id = ''`, domain)
	if err != nil {
		return err
	}

	// Nodes
	_, err = db.Exec(`UPDATE nodes SET ap_id = 'https://' || ? || '/ap/nodes/' || id WHERE ap_id IS NULL OR ap_id = ''`, domain)
	if err != nil {
		return err
	}

	// Events
	_, err = db.Exec(`UPDATE events SET ap_id = 'https://' || ? || '/ap/events/' || id WHERE ap_id IS NULL OR ap_id = ''`, domain)
	if err != nil {
		return err
	}

	// Proposals
	_, err = db.Exec(`UPDATE proposals SET ap_id = 'https://' || ? || '/ap/proposals/' || id WHERE ap_id IS NULL OR ap_id = ''`, domain)
	if err != nil {
		return err
	}

	return nil
}
