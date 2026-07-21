package ap

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// The instance service actor (docs/adr/024): one Application actor per
// quilt that relays Follows of remote patches on behalf of all its
// people. It exists so that no person's actor is ever enumerable in a
// remote followers collection — follows are as private across quilts as
// they are at home.

// InstanceAPID returns the AP ID of the instance service actor.
func InstanceAPID(domain string) string {
	return fmt.Sprintf("https://%s/ap/instance", domain)
}

// EnsureInstanceActor creates the instance service actor keypair if it
// doesn't exist yet, and heals its ap_id after a domain change. Safe to
// call on every startup.
func EnsureInstanceActor(db *database.DB, domain string) error {
	apID := InstanceAPID(domain)

	var existing string
	err := db.QueryRow("SELECT ap_id FROM instance_actor WHERE id = 1").Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		pub, priv, err := GenerateKeyPair()
		if err != nil {
			return fmt.Errorf("generate instance actor keypair: %w", err)
		}
		_, err = db.Exec(
			"INSERT INTO instance_actor (id, ap_id, public_key, private_key) VALUES (1, ?, ?, ?)",
			apID, pub, priv,
		)
		return err
	}
	if err != nil {
		return err
	}
	if existing != apID {
		_, err = db.Exec("UPDATE instance_actor SET ap_id = ? WHERE id = 1", apID)
	}
	return err
}

// InstanceActorKeys returns the instance actor's AP ID and public key.
func InstanceActorKeys(db *database.DB) (apID, publicKey string, err error) {
	err = db.QueryRow("SELECT ap_id, public_key FROM instance_actor WHERE id = 1").Scan(&apID, &publicKey)
	return apID, publicKey, err
}

// BuildFollow builds a Follow activity from the instance actor to a
// remote actor. The activity ID doubles as the reference an Accept or
// Undo can point back to.
func BuildFollow(followID, instanceActorID, remoteActorID string) map[string]interface{} {
	return map[string]interface{}{
		"@context": Context,
		"type":     "Follow",
		"id":       followID,
		"actor":    instanceActorID,
		"object":   remoteActorID,
	}
}

// BuildUndoFollow builds an Undo(Follow) mirroring a previously sent Follow.
func BuildUndoFollow(followID, instanceActorID, remoteActorID string) map[string]interface{} {
	return map[string]interface{}{
		"@context": Context,
		"type":     "Undo",
		"actor":    instanceActorID,
		"object":   BuildFollow(followID, instanceActorID, remoteActorID),
	}
}
