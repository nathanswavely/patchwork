package eventsource

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// A wall clock is not an instant. Feeds hand this package both: an ICS
// DTSTART can carry Z (an instant), a TZID (a wall clock plus the zone
// to read it in), or neither, and schema.org startDate is the same three
// shapes in ISO 8601 dress. That last case — a *floating* time — is the
// one with no answer in the document: "19:00" means seven in the evening
// wherever the venue is, and nothing in the feed says where that is.
//
// Every floating time used to be read as UTC. For a quilt on Eastern
// time that put every such event four or five hours early: a 7pm show
// stored as 19:00Z and rendered back in the reader's zone as 3:00 PM.
// All-day events lost a whole day the same way, midnight UTC being the
// evening before.
//
// The zone a feed is read in is the zone of the patch that attached it
// (docs/adr/045): a venue's own calendar publishing floating times is
// publishing them in its own zone, and that is what floating means. It
// resolves patch → instance → UTC, the same chain an event's own zone
// resolves through, and it arrives here as a parameter rather than a
// package global so that two patches in two zones can sync in the same
// process without one deciding for the other.

// offsetInName matches the offset publishers bake into a TZID they made
// up: "(UTC-05:00) Eastern Time (US & Canada)", "GMT+0100", "UTC-5".
var offsetInName = regexp.MustCompile(`(?i)(?:UTC|GMT)\s*([+-])\s*(\d{1,2})(?::?(\d{2}))?`)

// resolveTZID maps a feed's TZID to a location. ianaName is non-empty
// only when the result is a tzdata zone, so a caller can hand the name
// back to a parser that resolves zones by name itself.
//
// The point of the fallbacks is that an unrecognised TZID used to make
// the whole event unreadable, and an unreadable event is dropped from
// the feed silently — the reconciler cannot tell "the parser failed"
// from "the calendar removed it". A time that is present and possibly
// an hour off beats a show that vanished.
func resolveTZID(tzid string) (loc *time.Location, ianaName string, ok bool) {
	name := strings.Trim(strings.TrimSpace(tzid), `"`)
	if name == "" {
		return nil, "", false
	}

	if loc, err := time.LoadLocation(name); err == nil {
		return loc, name, true
	}

	// Exchange and Outlook write Windows zone names, which are not
	// tzdata names and never will be. The table is CLDR's mapping,
	// trimmed to the zones a community calendar plausibly uses.
	if iana, found := windowsZones[foldZoneName(name)]; found {
		if loc, err := time.LoadLocation(iana); err == nil {
			return loc, iana, true
		}
	}

	// Last resort: a name that carries its own offset. Fixed, so it
	// gets the standard/daylight split wrong half the year — but it is
	// anchored to something the publisher wrote, which beats guessing.
	if m := offsetInName.FindStringSubmatch(name); m != nil {
		hours, _ := strconv.Atoi(m[2])
		minutes := 0
		if m[3] != "" {
			minutes, _ = strconv.Atoi(m[3])
		}
		secs := hours*3600 + minutes*60
		if m[1] == "-" {
			secs = -secs
		}
		if secs > -14*3600 && secs < 14*3600 {
			return time.FixedZone(name, secs), "", true
		}
	}

	return nil, "", false
}

// foldZoneName is the form windowsZones is keyed on: lowercased, with
// periods dropped and runs of whitespace collapsed. Half the Windows ids
// carry a period ("W. Europe Standard Time") and half do not, feeds vary
// the casing, and none of that is a distinction worth honouring.
func foldZoneName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(name, ".", ""))), " ")
}

// windowsZones maps folded Windows timezone names to IANA ones. Keys are
// written in their folded form — see foldZoneName.
var windowsZones = map[string]string{
	// North America
	"hawaiian standard time":          "Pacific/Honolulu",
	"alaskan standard time":           "America/Anchorage",
	"pacific standard time":           "America/Los_Angeles",
	"pacific standard time (mexico)":  "America/Tijuana",
	"us mountain standard time":       "America/Phoenix",
	"mountain standard time":          "America/Denver",
	"mountain standard time (mexico)": "America/Chihuahua",
	"central standard time":           "America/Chicago",
	"central standard time (mexico)":  "America/Mexico_City",
	"canada central standard time":    "America/Regina",
	"eastern standard time":           "America/New_York",
	"eastern standard time (mexico)":  "America/Cancun",
	"us eastern standard time":        "America/Indiana/Indianapolis",
	"atlantic standard time":          "America/Halifax",
	"newfoundland standard time":      "America/St_Johns",

	// Central and South America
	"sa pacific standard time":      "America/Bogota",
	"sa western standard time":      "America/La_Paz",
	"sa eastern standard time":      "America/Cayenne",
	"argentina standard time":       "America/Argentina/Buenos_Aires",
	"e south america standard time": "America/Sao_Paulo",
	"central america standard time": "America/Guatemala",

	// Europe and Africa
	"gmt standard time":              "Europe/London",
	"greenwich standard time":        "Atlantic/Reykjavik",
	"w europe standard time":         "Europe/Berlin",
	"central europe standard time":   "Europe/Budapest",
	"romance standard time":          "Europe/Paris",
	"central european standard time": "Europe/Warsaw",
	"gtb standard time":              "Europe/Bucharest",
	"fle standard time":              "Europe/Kiev",
	"e europe standard time":         "Europe/Chisinau",
	"russian standard time":          "Europe/Moscow",
	"w central africa standard time": "Africa/Lagos",
	"south africa standard time":     "Africa/Johannesburg",
	"e africa standard time":         "Africa/Nairobi",
	"egypt standard time":            "Africa/Cairo",
	"morocco standard time":          "Africa/Casablanca",

	// Asia and Oceania
	"israel standard time":        "Asia/Jerusalem",
	"arabic standard time":        "Asia/Baghdad",
	"arab standard time":          "Asia/Riyadh",
	"iran standard time":          "Asia/Tehran",
	"arabian standard time":       "Asia/Dubai",
	"pakistan standard time":      "Asia/Karachi",
	"india standard time":         "Asia/Kolkata",
	"bangladesh standard time":    "Asia/Dhaka",
	"se asia standard time":       "Asia/Bangkok",
	"china standard time":         "Asia/Shanghai",
	"singapore standard time":     "Asia/Singapore",
	"w australia standard time":   "Australia/Perth",
	"tokyo standard time":         "Asia/Tokyo",
	"korea standard time":         "Asia/Seoul",
	"cen australia standard time": "Australia/Adelaide",
	"aus central standard time":   "Australia/Darwin",
	"e australia standard time":   "Australia/Brisbane",
	"aus eastern standard time":   "Australia/Sydney",
	"tasmania standard time":      "Australia/Hobart",
	"new zealand standard time":   "Pacific/Auckland",

	// Names that are not Windows zones but show up in hand-rolled feeds.
	"utc": "UTC",
	"gmt": "UTC",
	"z":   "UTC",
}

// calendarZone reads the zone a calendar declares for itself.
// X-WR-TIMEZONE is not in RFC 5545, but Google, Apple, and most of the
// CMS plugins in between write it, and it is the only statement a feed
// full of floating times ever makes about where it is. Falls back to the
// zone the caller resolved for the patch that attached the feed.
func calendarZone(cal *ical.Calendar, fallback *time.Location) *time.Location {
	if fallback == nil {
		fallback = time.UTC
	}
	if cal != nil && cal.Component != nil {
		if p := cal.Props.Get(propCalendarTimezone); p != nil {
			if loc, _, ok := resolveTZID(p.Value); ok {
				return loc
			}
		}
	}
	return fallback
}

// propCalendarTimezone is the calendar-wide zone hint. go-ical has no
// constant for it: it is an X- property, not a standard one.
const propCalendarTimezone = "X-WR-TIMEZONE"

// normalizeTZIDs rewrites TZID parameters that go-ical cannot resolve
// into ones it can, so that an unfamiliar zone name shifts an event at
// worst instead of erasing it.
//
// Rewriting the parsed document rather than reading each property by
// hand keeps one code path: RecurrenceSet resolves EXDATE and RDATE
// zones internally, and there is no hook to reach inside it.
func normalizeTZIDs(comp *ical.Component) {
	if comp == nil {
		return
	}
	// A VTIMEZONE's own TZID names the zone it defines rather than
	// selecting one, and the DTSTARTs inside it are floating by
	// definition. Nothing in there is ours to rewrite.
	if strings.EqualFold(comp.Name, ical.CompTimezone) {
		return
	}
	for _, props := range comp.Props {
		for i := range props {
			normalizeTZIDParam(&props[i])
		}
	}
	for _, child := range comp.Children {
		normalizeTZIDs(child)
	}
}

func normalizeTZIDParam(p *ical.Prop) {
	tzid := p.Params.Get(ical.PropTimezoneID)
	if tzid == "" {
		return
	}
	if _, err := time.LoadLocation(strings.Trim(tzid, `"`)); err == nil {
		return // go-ical will resolve it the same way
	}

	loc, iana, ok := resolveTZID(tzid)
	if ok && iana != "" {
		p.Params.Set(ical.PropTimezoneID, iana)
		return
	}
	if ok {
		// A fixed zone has no name time.LoadLocation would accept, so
		// the value is converted here instead and handed on as an
		// instant. Multi-value EXDATE/RDATE lists are left to the
		// calendar zone rather than parsed apart for this.
		if t, err := time.ParseInLocation(icsDateTime, p.Value, loc); err == nil && !strings.Contains(p.Value, ",") {
			p.Value = t.UTC().Format(icsDateTimeUTC)
			p.Params.Del(ical.PropTimezoneID)
			return
		}
	}

	// Nothing resolved it. Drop the parameter so the value reads as a
	// floating time in the calendar's zone — the same treatment a feed
	// that never named a zone gets.
	p.Params.Del(ical.PropTimezoneID)
}

const (
	icsDateTime    = "20060102T150405"
	icsDateTimeUTC = "20060102T150405Z"
)

// loadZone turns a resolved zone name into a location, falling back to
// UTC rather than failing. The name has already been through the same
// validation the admin panel applies; a name that stops resolving here
// means tzdata moved under a stored value, and a feed read an hour off
// beats a feed that stops syncing.
func loadZone(name string) *time.Location {
	if loc, err := time.LoadLocation(strings.TrimSpace(name)); err == nil {
		return loc
	}
	return time.UTC
}

// ReinterpretUTCAsLocal corrects a feed that publishes local time stamped
// as UTC (docs/adr/073).
//
// The defect: a publisher holds the venue's wall clock — 7pm — and emits
// it as though the digits were UTC. Downstream that instant renders four
// or five hours early, and the offset the feed prints beside it
// ("15:00:00-04:00") is a faithful rendering of the wrong instant, so
// every parser believes it and is right to.
//
// The correction takes the instant's UTC wall clock and reads those same
// digits in the patch's zone: 19:00Z becomes 19:00 in America/New_York,
// which is 23:00Z — 7pm, the number the publisher started from.
//
// This re-derives the shift on every sync rather than storing one, which
// is what makes it survive daylight saving: the same rule yields a
// four-hour move in August and a five-hour move in November, with
// nothing to edit in between.
func ReinterpretUTCAsLocal(iso string, zone *time.Location) string {
	if zone == nil {
		return iso
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso // not an instant we wrote; leave it exactly as found
	}
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute(), u.Second(), 0, zone).
		UTC().Format(time.RFC3339)
}

// correctStampedUTC applies ReinterpretUTCAsLocal across a parsed set.
//
// StartsAt and EndsAt only. Occurrence is deliberately untouched: it is
// the identity half of the reconciler's key (docs/adr/031), not a time
// anybody reads, so shifting it would turn every recurring event into a
// delete-and-reinsert the first time the switch is flipped — duplicate
// notifications, and event_source_skips rows orphaned so events an admin
// hid come back. Leaving it means the correction lands as an UPDATE in
// place and the event keeps its id, its links and its RSVPs.
func correctStampedUTC(items []Item, zone *time.Location) []Item {
	for i := range items {
		items[i].StartsAt = ReinterpretUTCAsLocal(items[i].StartsAt, zone)
		if items[i].EndsAt != nil {
			fixed := ReinterpretUTCAsLocal(*items[i].EndsAt, zone)
			items[i].EndsAt = &fixed
		}
	}
	return items
}
