package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// requireSignature, when true, rejects inbound activities whose HTTP Signature
// cannot be verified. It is a package var so tests can disable verification for
// cases that exercise activity handling rather than signature checking.
var requireSignature = true

// signatureMaxSkew bounds how far an inbound request's signed Date header may
// drift from our clock. A valid signature still covers a fixed Date, so an
// attacker replaying a captured POST cannot refresh it — rejecting stale or
// future-dated requests closes that replay window. 5 minutes tolerates ordinary
// clock skew between hosts while keeping the window tight. Package var so tests
// can adjust it.
var signatureMaxSkew = 5 * time.Minute

// timeNow is a clock seam so tests can pin the current time when exercising the
// Date-skew check.
var timeNow = time.Now

// verifyInbound checks the HTTP Signature on an incoming activity POST. It
// reads and restores the request body (so the caller can parse the activity
// afterward, since verification consumes r.Body) and returns the VERIFIED
// actor URL — the keyId's actor, whose key actually signed the request.
// Callers must bind this to the activity's actor field (checkActorBinding):
// a valid signature only proves who sent the POST, not that the sender is
// the actor the activity claims to be from. Empty when verification is
// disabled (tests).
func verifyInbound(r *http.Request) (verifiedActor string, err error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	r.Body.Close()
	if err != nil {
		return "", err
	}
	// Restore the body so downstream readers (readActivity) still work.
	r.Body = io.NopCloser(bytes.NewReader(body))

	if !requireSignature {
		return "", nil
	}

	// If a Digest header is present, it must match the body (the signature
	// covers the digest, so a mismatch means tampering).
	if d := r.Header.Get("Digest"); d != "" {
		sum := sha256.Sum256(body)
		want := "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
		if !strings.EqualFold(d, want) {
			return "", errSignature("digest mismatch")
		}
	}

	keyID := signatureKeyID(r.Header.Get("Signature"))
	if keyID == "" {
		return "", errSignature("missing or malformed Signature header")
	}

	// keyID is "<actorID>#main-key"; strip the fragment to get the actor URL.
	actorID := keyID
	if i := strings.Index(actorID, "#"); i >= 0 {
		actorID = actorID[:i]
	}

	remote, err := ap.FetchActor(r.Context(), actorID)
	if err != nil {
		return "", errSignature("fetch signing actor: " + err.Error())
	}
	if err := ap.VerifySignature(r, remote.PublicKey); err != nil {
		return "", errSignature("invalid signature: " + err.Error())
	}

	// The signature is valid, so the signed Date header is authentic. Reject it
	// if it falls outside the skew window to prevent replay of captured POSTs.
	if err := checkDateSkew(r.Header.Get("Date")); err != nil {
		return "", err
	}
	return actorID, nil
}

// checkActorBinding requires the activity's actor field to be the actor
// whose key signed the request. Without it, anyone with a valid fediverse
// key could deliver activities in someone else's name (e.g. forged event
// notifications from a patch they don't control). No-op when verification
// is disabled (verifiedActor empty).
func checkActorBinding(verifiedActor string, activity map[string]interface{}) error {
	if verifiedActor == "" {
		return nil
	}
	claimed, _ := activity["actor"].(string)
	if claimed != verifiedActor {
		return errSignature("activity actor " + claimed + " is not the signing actor " + verifiedActor)
	}
	return nil
}

// checkDateSkew rejects a Date header that is missing, unparseable, or further
// than signatureMaxSkew from the current time (in either direction).
func checkDateSkew(date string) error {
	if date == "" {
		return errSignature("missing Date header")
	}
	t, err := http.ParseTime(date)
	if err != nil {
		return errSignature("unparseable Date header")
	}
	if diff := timeNow().Sub(t); diff > signatureMaxSkew || diff < -signatureMaxSkew {
		return errSignature("Date header outside acceptable skew window")
	}
	return nil
}

// signatureKeyID extracts the keyId parameter from a Signature header.
func signatureKeyID(header string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "keyId=") {
			return strings.Trim(strings.TrimPrefix(part, "keyId="), `"`)
		}
	}
	return ""
}

type errSignature string

func (e errSignature) Error() string { return string(e) }

// APUserInbox handles POST /ap/users/{id}/inbox.
// Receives activities from remote instances addressed to a local user.
func APUserInbox(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("id")
		if userID == "" {
			http.Error(w, `{"error":"user id required"}`, http.StatusBadRequest)
			return
		}

		// Verify user exists.
		var exists int
		if err := db.QueryRow("SELECT 1 FROM users WHERE id = ? AND suspended_at IS NULL", userID).Scan(&exists); err != nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		// Verify the HTTP Signature before acting on the activity.
		verifiedActor, err := verifyInbound(r)
		if err != nil {
			log.Printf("ap: inbox signature verification failed for user %s: %v", userID, err)
			http.Error(w, `{"error":"signature verification failed"}`, http.StatusUnauthorized)
			return
		}

		activity, err := readActivity(r)
		if err != nil {
			http.Error(w, `{"error":"invalid activity"}`, http.StatusBadRequest)
			return
		}
		if err := checkActorBinding(verifiedActor, activity); err != nil {
			log.Printf("ap: user inbox actor binding failed: %v", err)
			http.Error(w, `{"error":"activity actor does not match signature"}`, http.StatusUnauthorized)
			return
		}

		activityType, _ := activity["type"].(string)
		switch activityType {
		case "Follow":
			handleFollowUser(r.Context(), w, db, userID, activity)
		case "Undo":
			handleUndo(w, db, "user", userID, activity)
		default:
			log.Printf("ap: inbox received unhandled activity type %q for user %s", activityType, userID)
			w.WriteHeader(http.StatusAccepted)
		}
	}
}

// APNodeInbox handles POST /ap/nodes/{id}/inbox.
// Receives activities from remote instances addressed to a local node (patch).
func APNodeInbox(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		if nodeID == "" {
			http.Error(w, `{"error":"node id required"}`, http.StatusBadRequest)
			return
		}

		// Verify node exists and is active.
		var exists int
		if err := db.QueryRow("SELECT 1 FROM nodes WHERE id = ? AND status IN ('active','unclaimed') AND removed_at IS NULL", nodeID).Scan(&exists); err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Verify the HTTP Signature before acting on the activity.
		verifiedActor, err := verifyInbound(r)
		if err != nil {
			log.Printf("ap: inbox signature verification failed for node %s: %v", nodeID, err)
			http.Error(w, `{"error":"signature verification failed"}`, http.StatusUnauthorized)
			return
		}

		activity, err := readActivity(r)
		if err != nil {
			http.Error(w, `{"error":"invalid activity"}`, http.StatusBadRequest)
			return
		}
		if err := checkActorBinding(verifiedActor, activity); err != nil {
			log.Printf("ap: node inbox actor binding failed: %v", err)
			http.Error(w, `{"error":"activity actor does not match signature"}`, http.StatusUnauthorized)
			return
		}

		activityType, _ := activity["type"].(string)
		switch activityType {
		case "Follow":
			handleFollowNode(r.Context(), w, db, nodeID, activity)
		case "Undo":
			handleUndo(w, db, "node", nodeID, activity)
		default:
			log.Printf("ap: inbox received unhandled activity type %q for node %s", activityType, nodeID)
			w.WriteHeader(http.StatusAccepted)
		}
	}
}

// readActivity reads and parses the request body as a JSON activity.
func readActivity(r *http.Request) (map[string]interface{}, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	var activity map[string]interface{}
	if err := json.Unmarshal(body, &activity); err != nil {
		return nil, err
	}
	return activity, nil
}

// resolveRemoteInbox returns the inbox URL to deliver activities (e.g. the
// Accept of a Follow) back to a remote actor. It fetches the actor document and
// uses its declared inbox; if the fetch fails or the document omits an inbox, it
// falls back to the conventional {actorID}/inbox. The fetched actor is cached by
// ap.FetchActor, so this is cheap when verifyInbound already fetched the same
// actor on the way in.
func resolveRemoteInbox(ctx context.Context, remoteActorID string) string {
	if remote, err := ap.FetchActor(ctx, remoteActorID); err == nil && remote.Inbox != "" {
		return remote.Inbox
	}
	return remoteActorID + "/inbox"
}

// handleFollowNode processes a Follow activity targeting a local node.
func handleFollowNode(ctx context.Context, w http.ResponseWriter, db *database.DB, nodeID string, activity map[string]interface{}) {
	remoteActorID, _ := activity["actor"].(string)
	if remoteActorID == "" {
		http.Error(w, `{"error":"actor required"}`, http.StatusBadRequest)
		return
	}

	// Only allow following public nodes.
	var visibility string
	if err := db.QueryRow("SELECT visibility FROM nodes WHERE id = ?", nodeID).Scan(&visibility); err != nil {
		http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
		return
	}
	if visibility != "public" {
		http.Error(w, `{"error":"cannot follow non-public node"}`, http.StatusForbidden)
		return
	}

	remoteInbox := resolveRemoteInbox(ctx, remoteActorID)

	// Insert into ap_followers (ignore duplicate).
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted)
		 VALUES (?, 'node', ?, ?, ?, 1)
		 ON CONFLICT(local_actor_id, remote_actor_id) DO UPDATE SET accepted = 1`,
		id, nodeID, remoteActorID, remoteInbox,
	)
	if err != nil {
		log.Printf("ap: failed to insert follower: %v", err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	// Queue an Accept(Follow) activity for delivery.
	domain := ap.GetDomain()
	localActorID := ap.NodeAPID(domain, nodeID)
	accept := ap.BuildAcceptFollow(localActorID, activity)
	if err := ap.QueueActivity(db, accept, remoteInbox); err != nil {
		log.Printf("ap: failed to queue Accept(Follow): %v", err)
		// Don't fail the request; the follow is recorded even if the Accept isn't queued.
	}

	log.Printf("ap: node %s followed by remote actor %s", nodeID, remoteActorID)
	w.WriteHeader(http.StatusAccepted)
}

// handleFollowUser processes a Follow activity targeting a local user.
func handleFollowUser(ctx context.Context, w http.ResponseWriter, db *database.DB, userID string, activity map[string]interface{}) {
	remoteActorID, _ := activity["actor"].(string)
	if remoteActorID == "" {
		http.Error(w, `{"error":"actor required"}`, http.StatusBadRequest)
		return
	}

	remoteInbox := resolveRemoteInbox(ctx, remoteActorID)

	// Insert into ap_followers (ignore duplicate).
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted)
		 VALUES (?, 'user', ?, ?, ?, 1)
		 ON CONFLICT(local_actor_id, remote_actor_id) DO UPDATE SET accepted = 1`,
		id, userID, remoteActorID, remoteInbox,
	)
	if err != nil {
		log.Printf("ap: failed to insert follower: %v", err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	// Queue an Accept(Follow) activity for delivery.
	domain := ap.GetDomain()
	localActorID := ap.UserAPID(domain, userID)
	accept := ap.BuildAcceptFollow(localActorID, activity)
	if err := ap.QueueActivity(db, accept, remoteInbox); err != nil {
		log.Printf("ap: failed to queue Accept(Follow): %v", err)
	}

	log.Printf("ap: user %s followed by remote actor %s", userID, remoteActorID)
	w.WriteHeader(http.StatusAccepted)
}

// handleUndo processes an Undo activity. Currently only handles Undo(Follow).
func handleUndo(w http.ResponseWriter, db *database.DB, actorType, localID string, activity map[string]interface{}) {
	remoteActorID, _ := activity["actor"].(string)
	if remoteActorID == "" {
		http.Error(w, `{"error":"actor required"}`, http.StatusBadRequest)
		return
	}

	// Extract the inner object to determine what is being undone.
	innerObj, ok := activity["object"].(map[string]interface{})
	if !ok {
		log.Printf("ap: Undo activity has non-object 'object' field from %s", remoteActorID)
		http.Error(w, `{"error":"invalid undo object"}`, http.StatusBadRequest)
		return
	}

	innerType, _ := innerObj["type"].(string)
	switch innerType {
	case "Follow":
		// Delete the follow relationship.
		result, err := db.Exec(
			`DELETE FROM ap_followers WHERE local_actor_type = ? AND local_actor_id = ? AND remote_actor_id = ?`,
			actorType, localID, remoteActorID,
		)
		if err != nil {
			log.Printf("ap: failed to delete follower: %v", err)
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		log.Printf("ap: %s %s unfollowed by remote actor %s (rows deleted: %d)", actorType, localID, remoteActorID, rows)
		w.WriteHeader(http.StatusAccepted)
	default:
		log.Printf("ap: Undo of unhandled type %q from %s", innerType, remoteActorID)
		w.WriteHeader(http.StatusAccepted)
	}
}

// APPreview returns the AP JSON representation of a node for admin review.
// This is defined elsewhere; declaring it here would conflict.
// The function is already in ap.go.


