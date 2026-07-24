package handler

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// The instance service actor (docs/adr/024): an Application actor that
// relays remote-patch Follows for all local users, so no person is ever
// enumerable in a remote followers collection. Its inbox receives the
// Accepts of those Follows and the Create activities remote patches
// broadcast to their followers — which fan out as local notifications.

// APInstanceActor serves GET /ap/instance.
func APInstanceActor(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apID, publicKey, err := ap.InstanceActorKeys(db)
		if err != nil {
			http.Error(w, `{"error":"instance actor not initialized"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()
		resp := map[string]interface{}{
			"@context":          ap.Context,
			"type":              "Application",
			"id":                apID,
			"name":              settings.EffectiveName(db, cfg),
			"preferredUsername": domain,
			"summary":           "Patchwork instance service actor — relays cross-quilt follows for this quilt's people (see the software's ADR 024).",
			"url":               "https://" + domain,
			"inbox":             apID + "/inbox",
			"outbox":            apID + "/outbox",
			"publicKey":         publicKeyObject(apID, publicKey),
		}
		writeAP(w, resp)
	}
}

// APInstanceInbox handles POST /ap/instance/inbox.
func APInstanceInbox(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		verifiedActor, err := verifyInbound(r)
		if err != nil {
			log.Printf("ap: instance inbox signature verification failed: %v", err)
			http.Error(w, `{"error":"signature verification failed"}`, http.StatusUnauthorized)
			return
		}

		activity, err := readActivity(r)
		if err != nil {
			http.Error(w, `{"error":"invalid activity"}`, http.StatusBadRequest)
			return
		}
		// Bind the claimed actor to the signing actor. Without this, any
		// keyholder on the fediverse could deliver a Create "from" a patch
		// they don't control and inject forged event notifications — the
		// exact trust the instance-actor relay exists to provide.
		if err := checkActorBinding(verifiedActor, activity); err != nil {
			log.Printf("ap: instance inbox actor binding failed: %v", err)
			http.Error(w, `{"error":"activity actor does not match signature"}`, http.StatusUnauthorized)
			return
		}

		activityType, _ := activity["type"].(string)
		switch activityType {
		case "Accept":
			handleInstanceAccept(w, db, activity)
		case "Reject":
			remoteActorID, _ := activity["actor"].(string)
			log.Printf("ap: instance follow rejected by %s", remoteActorID)
			w.WriteHeader(http.StatusAccepted)
		case "Create", "Update":
			handleInstanceCreate(w, db, activityType, activity)
		default:
			log.Printf("ap: instance inbox received unhandled activity type %q", activityType)
			w.WriteHeader(http.StatusAccepted)
		}
	}
}

// handleInstanceAccept marks the instance actor's outbound Follow of a
// remote patch as accepted. The Accept must wrap a Follow that this
// instance's actor actually sent — an Accept for someone else's Follow
// (or for nothing) changes no state.
func handleInstanceAccept(w http.ResponseWriter, db *database.DB, activity map[string]interface{}) {
	remoteActorID, _ := activity["actor"].(string)
	if remoteActorID == "" {
		http.Error(w, `{"error":"actor required"}`, http.StatusBadRequest)
		return
	}

	object, _ := activity["object"].(map[string]interface{})
	objectType, _ := object["type"].(string)
	objectActor, _ := object["actor"].(string)
	if objectType != "Follow" || objectActor != ap.InstanceAPID(ap.GetDomain()) {
		log.Printf("ap: ignoring Accept from %s that doesn't wrap our Follow", remoteActorID)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if _, err := db.Exec("UPDATE ap_following SET accepted = 1 WHERE remote_actor_id = ?", remoteActorID); err != nil {
		log.Printf("ap: mark follow accepted for %s: %v", remoteActorID, err)
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleInstanceCreate turns an inbound Create/Update from a followed
// remote patch into notifications for every local person who follows it
// (docs/adr/024). The actor is the remote patch (already bound to the
// HTTP signature by the caller); recipients come from remote_follows
// rows keyed by its AP id.
func handleInstanceCreate(w http.ResponseWriter, db *database.DB, activityType string, activity map[string]interface{}) {
	remoteActorID, _ := activity["actor"].(string)
	if remoteActorID == "" {
		http.Error(w, `{"error":"actor required"}`, http.StatusBadRequest)
		return
	}

	object, _ := activity["object"].(map[string]interface{})
	if object == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	objectType, _ := object["type"].(string)
	objectName, _ := object["name"].(string)
	if objectName == "" {
		// Only named objects (events, proposals, docs) are worth a
		// notification; ignore the rest.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	rows, err := db.Query(
		"SELECT user_id, quilt_url, node_slug, node_name FROM remote_follows WHERE node_ap_id = ?",
		remoteActorID,
	)
	if err != nil {
		log.Printf("ap: load remote follows for %s: %v", remoteActorID, err)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	defer rows.Close()

	// An edited event is news too, but it isn't a NEW event — and the distinct
	// title also keeps it from being swallowed by the Create's dedup row.
	var title string
	switch {
	case objectType == "Event" && activityType == "Update":
		title = "Updated event: " + objectName
	case objectType == "Event":
		title = "New event: " + objectName
	case activityType == "Update":
		title = "Updated from a followed patch: " + objectName
	default:
		title = "New from a followed patch: " + objectName
	}

	// The link carries the object's id tail so two same-named events from
	// one patch stay distinct notifications, and redeliveries of the SAME
	// event (delivery retries, re-broadcasts) still dedup exactly.
	objectID, _ := object["id"].(string)

	for rows.Next() {
		var userID, quiltURLRow, nodeSlug, nodeName string
		if err := rows.Scan(&userID, &quiltURLRow, &nodeSlug, &nodeName); err != nil {
			continue
		}
		body := nodeName
		if body == "" {
			body = nodeSlug
		}
		rowLink := "/quilts/" + quiltHost(quiltURLRow) + "/patches/" + nodeSlug
		if tail := idTail(objectID); tail != "" {
			rowLink += "?event=" + tail
		}

		var dup int
		db.QueryRow(
			"SELECT 1 FROM notifications WHERE user_id = ? AND type = 'remote.event' AND title = ? AND link = ?",
			userID, title, rowLink,
		).Scan(&dup)
		if dup == 1 {
			continue
		}
		CreateNotification(db, userID, "remote.event", title, body, rowLink)
	}

	w.WriteHeader(http.StatusAccepted)
}

// idTail returns the last path segment of an AP object id — enough to
// distinguish objects in dedup keys without carrying a full URL around.
func idTail(apID string) string {
	if apID == "" {
		return ""
	}
	trimmed := strings.TrimRight(apID, "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
}

// quiltHost reduces a quilt origin URL to its host for use in SPA routes.
func quiltHost(origin string) string {
	if u, err := url.Parse(origin); err == nil && u.Host != "" {
		return u.Host
	}
	return strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
}
