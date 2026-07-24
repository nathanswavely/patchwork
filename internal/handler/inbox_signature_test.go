package handler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// signedFollowRequest builds a Follow activity POST signed with privPEM, dated now.
func signedFollowRequest(t *testing.T, nodeID, remoteActor, keyID, privPEM string) *http.Request {
	return signedFollowRequestAt(t, nodeID, remoteActor, keyID, privPEM, time.Now())
}

// signedFollowRequestAt is like signedFollowRequest but stamps (and signs) a
// specific Date, so tests can exercise the skew window.
func signedFollowRequestAt(t *testing.T, nodeID, remoteActor, keyID, privPEM string, date time.Time) *http.Request {
	t.Helper()
	follow := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Follow",
		"actor":    remoteActor,
		"object":   fmt.Sprintf("https://%s/ap/nodes/%s", ap.GetDomain(), nodeID),
	}
	body, err := json.Marshal(follow)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	r := httptest.NewRequest("POST", "/ap/nodes/"+nodeID+"/inbox", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/activity+json")
	r.Header.Set("Date", date.UTC().Format(http.TimeFormat))
	sum := sha256.Sum256(body)
	r.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))

	if keyID != "" && privPEM != "" {
		if err := ap.SignRequest(r, keyID, privPEM); err != nil {
			t.Fatalf("sign request: %v", err)
		}
	}
	return r
}

func TestAPNodeInbox_ValidSignatureAccepted(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "sigowner1", "member")
	nodeID := createTestNode(t, db, owner.ID, "Sig Patch", "sig-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	pub, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	remoteActor := "https://remote.example/ap/users/signed-user"
	keyID := remoteActor + "#main-key"

	// Stub the fetcher to return our generated public key for this actor.
	restore := ap.SetActorFetcher(func(_ context.Context, id string) (*ap.RemoteActor, error) {
		return &ap.RemoteActor{ID: remoteActor, Inbox: remoteActor + "/inbox", PublicKey: pub}, nil
	})
	defer ap.SetActorFetcher(restore)

	r := signedFollowRequest(t, nodeID, remoteActor, keyID, priv)
	w := servePublicMux(t, "POST", "/ap/nodes/{id}/inbox", handler.APNodeInbox(db), r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for valid signature, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM ap_followers WHERE local_actor_id = ? AND remote_actor_id = ?", nodeID, remoteActor).Scan(&count)
	if count != 1 {
		t.Errorf("expected follower recorded, got %d", count)
	}
}

func TestAPNodeInbox_UnsignedRejected(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "sigowner2", "member")
	nodeID := createTestNode(t, db, owner.ID, "Unsigned Patch", "unsigned-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	// No signature applied.
	r := signedFollowRequest(t, nodeID, "https://remote.example/ap/users/nobody", "", "")
	w := servePublicMux(t, "POST", "/ap/nodes/{id}/inbox", handler.APNodeInbox(db), r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unsigned request, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPNodeInbox_StaleDateRejected(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "sigowner4", "member")
	nodeID := createTestNode(t, db, owner.ID, "Stale Patch", "stale-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	pub, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	remoteActor := "https://remote.example/ap/users/stale-user"
	keyID := remoteActor + "#main-key"

	restore := ap.SetActorFetcher(func(_ context.Context, id string) (*ap.RemoteActor, error) {
		return &ap.RemoteActor{ID: remoteActor, Inbox: remoteActor + "/inbox", PublicKey: pub}, nil
	})
	defer ap.SetActorFetcher(restore)

	// Signature is valid, but the signed Date is well outside the skew window —
	// the shape of a replayed POST. It must be rejected.
	r := signedFollowRequestAt(t, nodeID, remoteActor, keyID, priv, time.Now().Add(-time.Hour))
	w := servePublicMux(t, "POST", "/ap/nodes/{id}/inbox", handler.APNodeInbox(db), r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for stale Date, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM ap_followers WHERE local_actor_id = ? AND remote_actor_id = ?", nodeID, remoteActor).Scan(&count)
	if count != 0 {
		t.Errorf("expected no follower recorded for stale request, got %d", count)
	}
}

func TestAPNodeInbox_WrongKeyRejected(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "sigowner3", "member")
	nodeID := createTestNode(t, db, owner.ID, "Wrongkey Patch", "wrongkey-patch", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	// Sign with one key, but have the fetcher return a different public key.
	_, signingPriv, _ := ap.GenerateKeyPair()
	otherPub, _, _ := ap.GenerateKeyPair()

	remoteActor := "https://remote.example/ap/users/imposter"
	keyID := remoteActor + "#main-key"

	restore := ap.SetActorFetcher(func(_ context.Context, id string) (*ap.RemoteActor, error) {
		return &ap.RemoteActor{ID: remoteActor, PublicKey: otherPub}, nil
	})
	defer ap.SetActorFetcher(restore)

	r := signedFollowRequest(t, nodeID, remoteActor, keyID, signingPriv)
	w := servePublicMux(t, "POST", "/ap/nodes/{id}/inbox", handler.APNodeInbox(db), r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for mismatched key, got %d: %s", w.Code, w.Body.String())
	}
}
