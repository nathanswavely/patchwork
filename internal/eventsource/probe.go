package eventsource

import (
	"context"
	"time"
)

// Probe reads a calendar at a URL that is not attached to anything and
// reports how many of its items start from now on.
//
// It exists for the feed a patch suggestion carries
// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
// decision 6): the address is fetched and parsed while the suggester is
// still looking at the form, so a URL Patchwork cannot read is refused now
// rather than never, and the count is what the reviewing admin reads before
// deciding whether to attach it.
//
// It runs the same detection `Sync` runs — ICS first, then schema.org
// markup, then a Squarespace events view — through the same SSRF-guarded
// client, because a suggestion is somebody else's input and the probe is an
// outbound fetch at an address they chose. Nothing is written: there is no
// source row yet, and there must not be one on a patch nobody has approved.
//
// The zone is UTC rather than a patch's, because the patch does not exist
// yet and the only thing read out of these items is how many of them are
// still to come. Once the source attaches, `Sync` resolves the real zone and
// this count stops being read.
func Probe(ctx context.Context, feedURL string) (int, error) {
	src := &Source{Type: "ics", URL: feedURL, Zone: time.UTC}
	items, _, err := loadItemsFor(ctx, src)
	if err != nil {
		return 0, err
	}
	// Item.StartsAt is RFC3339 in UTC, so a string compare orders it.
	now := time.Now().UTC().Format(time.RFC3339)
	count := 0
	for _, it := range items {
		if it.StartsAt >= now {
			count++
		}
	}
	return count, nil
}
