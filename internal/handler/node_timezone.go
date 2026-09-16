package handler

import (
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// Changing a patch's zone is a change to what its events say.
//
// docs/adr/067 draws the line this file walks: an event's stored instant
// and its wall-clock reading are different things. `starts_at` is the
// encoding; the zone is what reconstructs the fact. A patch's events
// inherit the patch's zone, so moving the patch from one zone to another
// leaves every instant exactly where it was and changes every reading —
// four rehearsals typed as 7pm start saying 2pm, and nothing in the
// product ever said a word about it.
//
// Both answers are defensible and neither may be silent:
//
//   - keep_clock re-anchors. The wall clock was the fact the organizer
//     typed (docs/adr/045: "the wall clock at the venue *is* the fact"),
//     so 7pm stays 7pm and the stored instants move by the offset.
//   - keep_instant leaves the rows alone. The events happened, or will
//     happen, at the moment recorded; their readings change.
//
// So the write path refuses to guess. A zone change that would alter any
// reading is a 409 naming the count and the two answers, and the caller
// re-sends with `timezone_events` set. Whichever it picks, the response
// says what was done and to how many.
//
// One class of event is never re-anchored: an imported one (docs/adr/031).
// A feed's instant is the feed's fact, arrived at by the ingest chain
// docs/adr/067 decision 3 built, and rewriting it here would undo that
// reading with a guess. Imported events always keep their instant, are
// not counted toward the choice, and the next sync would overwrite the
// rewrite anyway.

const (
	zoneKeepClock   = "keep_clock"
	zoneKeepInstant = "keep_instant"
)

// zoneChange is what a pending timezone change does to the events that
// inherit it — a patch's own zone, or the whole quilt's (docs/adr/105).
type zoneChange struct {
	From string `json:"from"`
	To   string `json:"to"`
	Mode string `json:"mode"`
	// EventsAffected is how many inheriting, re-anchorable events read as
	// a different wall clock after the change.
	EventsAffected int `json:"events_affected"`
	// EventsMoved is how many stored instants were actually rewritten —
	// equal to EventsAffected under keep_clock, zero under keep_instant.
	EventsMoved int `json:"events_moved"`
	// PatchesAffected is how many patches those events belong to. Only the
	// quilt-wide change carries it: on one patch the answer is always one,
	// and a count of one patch in the copy would read as a mistake.
	PatchesAffected int `json:"patches_affected,omitempty"`

	affected []zoneAffectedEvent
}

type zoneAffectedEvent struct {
	ID       string
	StartsAt string
	EndsAt   string
	NodeID   string
}

// resolveNodeZone collapses a patch's stored zone through the chain
// docs/adr/045 specifies. Empty means inherit, and the instance's zone is
// the terminating rung — settings.EffectiveTimezone never returns "".
func resolveNodeZone(db *database.DB, stored string) string {
	if s := strings.TrimSpace(stored); s != "" {
		return s
	}
	return settings.EffectiveTimezone(db)
}

// planZoneChange reports what moving a patch from one stored zone to
// another would do. A nil return means nothing reads differently and the
// change can just be applied: the zones agree on this patch's calendar
// (docs/adr/067 decision 4 — America/New_York and America/Detroit are
// two names for one clock), the patch has no events, or every event it
// has pins its own zone.
func planZoneChange(db *database.DB, nodeID, storedFrom, storedTo string) (*zoneChange, error) {
	from := resolveNodeZone(db, storedFrom)
	to := resolveNodeZone(db, storedTo)
	if from == to {
		return nil, nil
	}
	fromLoc, err := time.LoadLocation(from)
	if err != nil {
		return nil, err
	}
	toLoc, err := time.LoadLocation(to)
	if err != nil {
		return nil, err
	}

	// Inheriting (timezone NULL or ''), this patch's own, still on the
	// calendar, and not a feed's. A confirmed link from another patch is
	// that patch's event and inherits that patch's zone, so it is not
	// here either.
	rows, err := db.Query(
		`SELECT id, starts_at, COALESCE(ends_at,'') FROM events
		 WHERE node_id = ? AND (timezone IS NULL OR timezone = '')
		   AND removed_at IS NULL AND source_id IS NULL`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plan := &zoneChange{From: from, To: to}
	for rows.Next() {
		var e zoneAffectedEvent
		if err := rows.Scan(&e.ID, &e.StartsAt, &e.EndsAt); err != nil {
			continue
		}
		if !zoneReadingChanges(e.StartsAt, fromLoc, toLoc) &&
			!zoneReadingChanges(e.EndsAt, fromLoc, toLoc) {
			continue
		}
		plan.affected = append(plan.affected, e)
	}
	plan.EventsAffected = len(plan.affected)
	if plan.EventsAffected == 0 {
		return nil, nil
	}
	return plan, nil
}

// zoneLocations resolves a plan's two zone names. Shared so the planner
// and the writer never disagree about what a name means.
func zoneLocations(from, to string) (*time.Location, *time.Location, error) {
	fromLoc, err := time.LoadLocation(from)
	if err != nil {
		return nil, nil, err
	}
	toLoc, err := time.LoadLocation(to)
	if err != nil {
		return nil, nil, err
	}
	return fromLoc, toLoc, nil
}

// applyZoneChange rewrites the affected instants under keep_clock and
// touches nothing under keep_instant, setting EventsMoved either way.
//
// Called after the patch's own row is updated, not before: the two writes
// are not in one transaction, and of the two half-states the survivable
// one is a zone that saved with its events unmoved. The other — events
// re-anchored to a zone the patch does not have — is a calendar nobody
// can reason about.
func applyZoneChange(db *database.DB, plan *zoneChange) error {
	if plan.Mode != zoneKeepClock {
		return nil
	}
	fromLoc, err := time.LoadLocation(plan.From)
	if err != nil {
		return err
	}
	toLoc, err := time.LoadLocation(plan.To)
	if err != nil {
		return err
	}
	for _, e := range plan.affected {
		starts, ok := reanchorInstant(e.StartsAt, fromLoc, toLoc)
		if !ok {
			continue
		}
		ends := e.EndsAt
		if ends != "" {
			if v, ok := reanchorInstant(ends, fromLoc, toLoc); ok {
				ends = v
			}
		}
		var endsArg any
		if ends != "" {
			endsArg = ends
		}
		if _, err := db.Exec(
			`UPDATE events SET starts_at = ?, ends_at = ?,
			 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
			starts, endsArg, e.ID,
		); err != nil {
			return err
		}
		plan.EventsMoved++
	}
	return nil
}

// zoneReadingChanges reports whether an instant renders as a different
// wall clock in the two zones. Comparing zone *names* is not this test:
// docs/adr/067 decision 4 found that America/New_York and
// America/Detroit never disagree, and a patch moving between them should
// not be asked a question with no consequences.
func zoneReadingChanges(instant string, from, to *time.Location) bool {
	if strings.TrimSpace(instant) == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, instant)
	if err != nil {
		return false
	}
	const wall = "2006-01-02T15:04:05"
	return t.In(from).Format(wall) != t.In(to).Format(wall)
}

// reanchorInstant reads an instant's wall clock in `from` and re-encodes
// that same wall clock in `to`. 7pm Eastern becomes 7pm Pacific, which is
// a different moment and the same fact.
//
// The stored precision is preserved because `starts_at` holds two of them
// — .000Z from the browser, zero-fraction from feed ingest — and the
// range comparisons are lexicographic (docs/adr/045's dayBound, and
// event_upload's dedupe key). Rewriting a row into the other precision
// would be a second, invisible change.
func reanchorInstant(instant string, from, to *time.Location) (string, bool) {
	t, err := time.Parse(time.RFC3339, instant)
	if err != nil {
		return instant, false
	}
	w := t.In(from)
	moved := time.Date(w.Year(), w.Month(), w.Day(), w.Hour(), w.Minute(), w.Second(), w.Nanosecond(), to)
	layout := time.RFC3339
	if strings.Contains(instant, ".") {
		layout = "2006-01-02T15:04:05.000Z07:00"
	}
	return moved.UTC().Format(layout), true
}
