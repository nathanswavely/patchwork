package handler

import (
	"strconv"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Changing the quilt's zone is the patch-level change at a wider radius
// (docs/adr/105, the follow-up docs/adr/101 wrote down so it would not be
// forgotten).
//
// A patch's zone is inherited unless the patch sets one, and an event's is
// inherited unless the event sets one, so the instance zone is the last rung
// of docs/adr/045's chain and the reading of every event that never named a
// zone rests on it. Moving it leaves every instant exactly where it was and
// changes every one of those readings — on every patch at once, with nobody
// on those patches involved in the decision.
//
// The rule is docs/adr/101's, unchanged: a zone change that would alter any
// reading is refused until somebody says which they meant. The same two
// answers, the same words for them, the same exclusions — an event that pins
// its own zone already said where it is, and an imported one is the feed's
// fact. What is different is the blast radius, so the refusal counts the
// patches as well as the events: "134 events across 9 patches" is a sentence
// an instance admin can weigh, and "134 events" is not.
//
// A patch that sets its own zone is untouched, because it never inherited.
// That is the whole of the safety here: a community that has said where it
// keeps time does not have its calendar moved by an instance setting.

// planInstanceZoneChange reports what moving the quilt's zone would do to
// the events that inherit it. A nil return means nothing reads differently:
// the zones agree on every affected calendar, or nothing inherits.
func planInstanceZoneChange(db *database.DB, from, to string) (*zoneChange, error) {
	if strings.TrimSpace(from) == strings.TrimSpace(to) {
		return nil, nil
	}
	fromLoc, toLoc, err := zoneLocations(from, to)
	if err != nil {
		return nil, err
	}

	// Inheriting at both rungs, still on a calendar, and not a feed's.
	//
	// Every patch that is still here, not only the active ones: an archived
	// patch's events keep their rows and can come back, and re-anchoring is
	// about keeping a clock reading true rather than about who can currently
	// see it. Leaving them out would mean a restored patch's calendar had
	// silently shifted while it was away.
	rows, err := db.Query(
		`SELECT e.id, e.starts_at, COALESCE(e.ends_at,''), e.node_id
		 FROM events e JOIN nodes n ON n.id = e.node_id
		 WHERE (e.timezone IS NULL OR e.timezone = '')
		   AND e.removed_at IS NULL AND e.source_id IS NULL
		   AND (n.timezone IS NULL OR n.timezone = '')
		   AND n.removed_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plan := &zoneChange{From: from, To: to}
	patches := map[string]bool{}
	for rows.Next() {
		var e zoneAffectedEvent
		if rows.Scan(&e.ID, &e.StartsAt, &e.EndsAt, &e.NodeID) != nil {
			continue
		}
		if !zoneReadingChanges(e.StartsAt, fromLoc, toLoc) &&
			!zoneReadingChanges(e.EndsAt, fromLoc, toLoc) {
			continue
		}
		plan.affected = append(plan.affected, e)
		patches[e.NodeID] = true
	}
	plan.EventsAffected = len(plan.affected)
	if plan.EventsAffected == 0 {
		return nil, nil
	}
	plan.PatchesAffected = len(patches)
	return plan, nil
}

// zoneChangeSubject is the noun phrase the quilt-wide refusal counts in.
// The patch-level one names no patches, because on one patch the answer is
// always one and a count of one patch reads as a mistake.
func zoneChangeSubject(plan *zoneChange) string {
	events := "events"
	if plan.EventsAffected == 1 {
		events = "event"
	}
	if plan.PatchesAffected == 0 {
		return events
	}
	patches := "patches"
	if plan.PatchesAffected == 1 {
		patches = "patch"
	}
	return events + " across " + strconv.Itoa(plan.PatchesAffected) + " " + patches
}
