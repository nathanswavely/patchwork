package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// feedEvent is one row bound for an outbound feed.
type feedEvent struct {
	ID          string
	Title       string
	Description string
	Location    string
	Latitude    *float64
	Longitude   *float64
	StartsAt    string
	EndsAt      *string
	NodeSlug    string
	NodeName    string
	CreatedAt   string
	UpdatedAt   string
}

const feedWindowBack = 30 * 24 * time.Hour
const feedMaxEvents = 500

func scanFeedEvents(db *database.DB, query string, args ...any) ([]feedEvent, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []feedEvent{}
	for rows.Next() {
		var e feedEvent
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.Location, &e.Latitude,
			&e.Longitude, &e.StartsAt, &e.EndsAt, &e.NodeSlug, &e.NodeName,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// feedEtag derives a cache validator from the feed's contents; calendar
// apps poll aggressively and an unchanged feed should cost nothing.
func feedEtag(events []feedEvent) string {
	h := sha256.New()
	for _, e := range events {
		fmt.Fprintf(h, "%s|%s\n", e.ID, e.UpdatedAt)
	}
	return `"` + hex.EncodeToString(h.Sum(nil))[:32] + `"`
}

// escapeICSText escapes an iCalendar TEXT value (RFC 5545 §3.3.11).
func escapeICSText(s string) string {
	r := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\n", "\\n", "\r", "")
	return r.Replace(s)
}

func writeICS(w http.ResponseWriter, r *http.Request, cfg *config.Config, calName string, events []feedEvent) {
	etag := feedEtag(events)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// go-ical refuses to encode a componentless VCALENDAR (RFC 5545 wants
	// ≥1 component), which would turn a quiet venue's feed into a
	// zero-byte 200 that calendar apps reject. Serve the minimal empty
	// calendar by hand — clients accept it, and the subscription stays
	// valid until the first event arrives.
	if len(events) == 0 {
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "public, max-age=300")
		fmt.Fprintf(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Patchwork//%s//EN\r\nX-WR-CALNAME:%s\r\nEND:VCALENDAR\r\n",
			escapeICSText(cfg.Instance.Domain), escapeICSText(calName))
		return
	}

	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Patchwork//"+cfg.Instance.Domain+"//EN")
	// De-facto standard calendar name properties.
	cal.Props.SetText("X-WR-CALNAME", calName)

	for _, fe := range events {
		start, err := time.Parse(time.RFC3339, fe.StartsAt)
		if err != nil {
			continue // an event a calendar can't place has no feed row
		}
		ev := ical.NewEvent()
		ev.Props.SetText(ical.PropUID, fe.ID+"@"+cfg.Instance.Domain)
		if stamp, err := time.Parse(time.RFC3339, fe.UpdatedAt); err == nil {
			ev.Props.SetDateTime(ical.PropDateTimeStamp, stamp.UTC())
		} else {
			ev.Props.SetDateTime(ical.PropDateTimeStamp, start.UTC())
		}
		ev.Props.SetDateTime(ical.PropDateTimeStart, start.UTC())
		if fe.EndsAt != nil {
			if end, err := time.Parse(time.RFC3339, *fe.EndsAt); err == nil {
				ev.Props.SetDateTime(ical.PropDateTimeEnd, end.UTC())
			}
		}
		ev.Props.SetText(ical.PropSummary, fe.Title)
		if fe.Description != "" {
			ev.Props.SetText(ical.PropDescription, fe.Description)
		}
		if fe.Location != "" {
			ev.Props.SetText(ical.PropLocation, fe.Location)
		}
		if fe.Latitude != nil && fe.Longitude != nil {
			geo := ical.NewProp(ical.PropGeo)
			geo.Value = strconv.FormatFloat(*fe.Latitude, 'f', -1, 64) + ";" + strconv.FormatFloat(*fe.Longitude, 'f', -1, 64)
			ev.Props.Set(geo)
		}
		ev.Props.SetText(ical.PropURL, "https://"+cfg.Instance.Domain+"/patches/"+fe.NodeSlug+"/events/"+fe.ID)
		cal.Children = append(cal.Children, ev.Component)
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=300")
	if err := ical.NewEncoder(w).Encode(cal); err != nil {
		// Headers are gone; nothing to do but log-level silence. The
		// client sees a truncated body and retries on its next poll.
		return
	}
}

// publicNodeFeedEvents loads the feed rows for one public patch.
func publicNodeFeedEvents(db *database.DB, slug string) (nodeName string, events []feedEvent, ok bool) {
	var nodeID string
	err := db.QueryRow(
		`SELECT id, name FROM nodes WHERE slug = ? AND visibility = 'public'
		 AND status IN ('active','unclaimed') AND removed_at IS NULL`, slug,
	).Scan(&nodeID, &nodeName)
	if err != nil {
		return "", nil, false
	}
	since := time.Now().Add(-feedWindowBack).UTC().Format(time.RFC3339)
	// Own events plus confirmed event links (docs/adr/032) — a linked
	// gig belongs on the patch's public calendar. The owner patch must
	// itself be public and alive for its events to blend here.
	events, err = scanFeedEvents(db,
		`SELECT e.id, e.title, e.description, e.location, e.latitude, e.longitude,
		 e.starts_at, e.ends_at, n.slug, n.name, e.created_at, e.updated_at
		 FROM events e JOIN nodes n ON e.node_id = n.id
		 WHERE (e.node_id = ? OR EXISTS (
		    SELECT 1 FROM event_links el WHERE el.event_id = e.id
		    AND el.node_id = ? AND el.status = 'confirmed'))
		 AND (n.visibility = 'public' OR n.id = ?)
		 AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
		 AND e.status = 'active' AND e.visibility = 'public'
		 AND e.removed_at IS NULL AND e.starts_at >= ?
		 ORDER BY e.starts_at LIMIT ?`, nodeID, nodeID, nodeID, since, feedMaxEvents)
	if err != nil {
		return "", nil, false
	}
	return nodeName, events, true
}

// NodeICSFeed handles GET /api/v1/nodes/{slug}/events.ics — the
// subscribable public calendar of one patch (docs/adr/031). Anonymous,
// public events only; also how one Patchwork becomes another's source.
func NodeICSFeed(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeName, events, ok := publicNodeFeedEvents(db, r.PathValue("slug"))
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeICS(w, r, cfg, nodeName+" — "+cfg.Instance.Name, events)
	}
}

// RSS 2.0 skeleton, stdlib xml only.
type rssDoc struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate,omitempty"`
}

// NodeRSSFeed handles GET /api/v1/nodes/{slug}/events.rss — the same
// public events as the ICS feed, for feed readers and site embeds.
func NodeRSSFeed(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeName, events, ok := publicNodeFeedEvents(db, r.PathValue("slug"))
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		etag := feedEtag(events)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		slug := r.PathValue("slug")
		doc := rssDoc{
			Version: "2.0",
			Channel: rssChannel{
				Title:       nodeName + " — events",
				Link:        "https://" + cfg.Instance.Domain + "/patches/" + slug + "/events",
				Description: "Events from " + nodeName + " on " + cfg.Instance.Name,
			},
		}
		for _, e := range events {
			desc := e.Description
			if when, err := time.Parse(time.RFC3339, e.StartsAt); err == nil {
				stamp := when.UTC().Format("Monday, January 2 2006, 15:04 MST")
				if desc == "" {
					desc = stamp
				} else {
					desc = stamp + "\n\n" + desc
				}
			}
			item := rssItem{
				Title:       e.Title,
				Link:        "https://" + cfg.Instance.Domain + "/patches/" + e.NodeSlug + "/events/" + e.ID,
				GUID:        e.ID + "@" + cfg.Instance.Domain,
				Description: desc,
			}
			if created, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
				item.PubDate = created.UTC().Format(time.RFC1123Z)
			}
			doc.Channel.Items = append(doc.Channel.Items, item)
		}

		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write([]byte(xml.Header))
		xml.NewEncoder(w).Encode(doc)
	}
}

// PersonalICSFeed handles GET /api/v1/feeds/{secret}/events.ics — one
// calendar of every event a person can see across the patches they
// belong to or follow, on this instance (docs/adr/031). The secret in
// the URL is the whole credential (calendar apps can't send cookies);
// it is stored hashed and regenerable, and grants nothing but this read.
func PersonalICSFeed(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := r.PathValue("secret")
		if len(secret) < 32 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var userID string
		err := db.QueryRow(
			`SELECT id FROM users WHERE feed_secret_hash = ? AND suspended_at IS NULL`,
			auth.HashToken(secret),
		).Scan(&userID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		since := time.Now().Add(-feedWindowBack).UTC().Format(time.RFC3339)
		// Public events from every patch the person has a relationship
		// with — including confirmed event links (docs/adr/032), so a
		// followed band's linked gig lands here too. Members-only
		// (private/unlisted) events only where the person belongs to the
		// event's own patch; a link never widens visibility.
		events, err := scanFeedEvents(db,
			`SELECT DISTINCT e.id, e.title, e.description, e.location, e.latitude, e.longitude,
			 e.starts_at, e.ends_at, n.slug, n.name, e.created_at, e.updated_at
			 FROM events e
			 JOIN nodes n ON e.node_id = n.id
			 JOIN memberships m ON m.user_id = ? AND m.status = 'active'
			   AND (m.node_id = e.node_id OR EXISTS (
			      SELECT 1 FROM event_links el WHERE el.event_id = e.id
			      AND el.node_id = m.node_id AND el.status = 'confirmed'))
			 WHERE e.status = 'active' AND e.removed_at IS NULL AND n.removed_at IS NULL
			 AND n.status IN ('active','unclaimed')
			 AND (e.visibility = 'public' OR (m.node_id = e.node_id AND m.role IN ('member','admin')))
			 AND e.starts_at >= ?
			 ORDER BY e.starts_at LIMIT ?`, userID, since, feedMaxEvents)
		if err != nil {
			http.Error(w, "feed unavailable", http.StatusInternalServerError)
			return
		}
		writeICS(w, r, cfg, "My Quilt — "+cfg.Instance.Name, events)
	}
}

// FeedSecretStatus handles GET /api/v1/users/me/feed-secret.
func FeedSecretStatus(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		var hash *string
		db.QueryRow(`SELECT feed_secret_hash FROM users WHERE id = ?`, user.ID).Scan(&hash)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"enabled": hash != nil})
	}
}

// GenerateFeedSecret handles POST /api/v1/users/me/feed-secret: create
// or replace the personal feed secret. The URL is shown once — only the
// hash is stored — and regenerating revokes the old URL.
func GenerateFeedSecret(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			http.Error(w, `{"error":"failed to generate feed secret"}`, http.StatusInternalServerError)
			return
		}
		secret := hex.EncodeToString(raw)
		if _, err := db.Exec(`UPDATE users SET feed_secret_hash = ? WHERE id = ?`, auth.HashToken(secret), user.ID); err != nil {
			http.Error(w, `{"error":"failed to generate feed secret"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "feed_secret.generate", "user", user.ID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"url": "https://" + cfg.Instance.Domain + "/api/v1/feeds/" + secret + "/events.ics",
		})
	}
}

// DeleteFeedSecret handles DELETE /api/v1/users/me/feed-secret.
func DeleteFeedSecret(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		if _, err := db.Exec(`UPDATE users SET feed_secret_hash = NULL WHERE id = ?`, user.ID); err != nil {
			http.Error(w, `{"error":"failed to disable feed"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "feed_secret.delete", "user", user.ID, "{}", clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
