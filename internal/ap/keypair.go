package ap

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// EnsureKeypair generates and stores a keypair for an entity if it doesn't have one.
func EnsureKeypair(db *database.DB, table, entityID string) (publicKey, privateKey string, err error) {
	// Check if keypair already exists
	var existingPub, existingPriv string
	err = db.QueryRow(
		"SELECT COALESCE(public_key,''), COALESCE(private_key,'') FROM "+table+" WHERE id = ?",
		entityID,
	).Scan(&existingPub, &existingPriv)
	if err != nil {
		return "", "", err
	}
	if existingPub != "" && existingPriv != "" {
		return existingPub, existingPriv, nil
	}

	// Generate new keypair
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		return "", "", err
	}

	// Store
	_, err = db.Exec("UPDATE "+table+" SET public_key = ?, private_key = ? WHERE id = ?", pub, priv, entityID)
	if err != nil {
		return "", "", err
	}

	return pub, priv, nil
}

// EnsureUserKeypair generates a keypair for a user.
func EnsureUserKeypair(db *database.DB, userID string) (string, string, error) {
	return EnsureKeypair(db, "users", userID)
}

// EnsureNodeKeypair generates a keypair for a node.
func EnsureNodeKeypair(db *database.DB, nodeID string) (string, string, error) {
	return EnsureKeypair(db, "nodes", nodeID)
}

// BackfillKeypairs generates keypairs for any users or nodes that don't have
// one yet. Keypairs are normally created when an entity is created, but
// entities that predate federation (e.g. seeded data) have none — without a
// key they cannot sign outbound activities and their AP actor document omits
// publicKey. Safe to call on every startup: it only touches rows missing a key.
// Returns the number of users and nodes backfilled.
func BackfillKeypairs(db *database.DB) (users, nodes int, err error) {
	users, err = backfillTable(db, "users")
	if err != nil {
		return 0, 0, fmt.Errorf("backfill users: %w", err)
	}
	nodes, err = backfillTable(db, "nodes")
	if err != nil {
		return users, 0, fmt.Errorf("backfill nodes: %w", err)
	}
	return users, nodes, nil
}

func backfillTable(db *database.DB, table string) (int, error) {
	rows, err := db.Query(
		"SELECT id FROM " + table + " WHERE public_key IS NULL OR public_key = '' OR private_key IS NULL OR private_key = ''",
	)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	count := 0
	for _, id := range ids {
		if _, _, err := EnsureKeypair(db, table, id); err != nil {
			return count, fmt.Errorf("entity %s: %w", id, err)
		}
		count++
	}
	return count, nil
}

// BackfillAPIDs rewrites locally-generated AP IDs whose domain no longer
// matches the configured instance domain — after a domain change, or a dev
// database seeded under "localhost". Served actor documents already compute
// IDs from the live domain, but signing-key lookup (PrivateKeyForActor)
// matches the stored ap_id column, so a stale domain breaks every outbound
// delivery. Only IDs with this instance's /ap/<kind>/<row id> shape are
// touched; remote URIs stored in the same columns (e.g. federated vote
// activity IDs) never carry the row's own id tail and are left alone.
// Safe to call on every startup. Returns the number of rows rewritten.
func BackfillAPIDs(db *database.DB, domain string) (int, error) {
	total := 0
	for _, kind := range []string{"users", "nodes", "events", "proposals"} {
		res, err := db.Exec(
			"UPDATE "+kind+" SET ap_id = 'https://' || ? || '/ap/' || ? || '/' || id"+
				" WHERE ap_id LIKE 'https://%/ap/' || ? || '/' || id"+
				" AND ap_id != 'https://' || ? || '/ap/' || ? || '/' || id",
			domain, kind, kind, domain, kind,
		)
		if err != nil {
			return total, fmt.Errorf("rewrite %s ap_ids: %w", kind, err)
		}
		if n, err := res.RowsAffected(); err == nil {
			total += int(n)
		}
	}
	return total, nil
}

// PrivateKeyForActor looks up the signing key for a local actor by its AP ID.
// It returns the HTTP Signatures keyID (actorAPID#main-key) and the PEM-encoded
// private key. The actor may be a local user, node, or the instance service
// actor. Returns an error if the actor is not local or has no private key.
func PrivateKeyForActor(db *database.DB, actorAPID string) (keyID, privateKeyPEM string, err error) {
	if actorAPID == "" {
		return "", "", fmt.Errorf("empty actor id")
	}

	var priv sql.NullString
	// Try users first, then nodes — AP IDs are unique across both via the ap_id column.
	err = db.QueryRow("SELECT private_key FROM users WHERE ap_id = ?", actorAPID).Scan(&priv)
	if errors.Is(err, sql.ErrNoRows) {
		err = db.QueryRow("SELECT private_key FROM nodes WHERE ap_id = ?", actorAPID).Scan(&priv)
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = db.QueryRow("SELECT private_key FROM instance_actor WHERE ap_id = ?", actorAPID).Scan(&priv)
	}
	if err != nil {
		return "", "", fmt.Errorf("lookup actor %q: %w", actorAPID, err)
	}
	if !priv.Valid || priv.String == "" {
		return "", "", fmt.Errorf("actor %q has no private key", actorAPID)
	}

	return actorAPID + "#main-key", priv.String, nil
}
