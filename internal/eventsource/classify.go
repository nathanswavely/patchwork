package eventsource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Situation is where a patch's calendar stands, as far as Patchwork can
// read it (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md,
// decision 4). It picks the calendar paragraph of an intro email, and it
// is the sentence the event-source form could not say before: not only
// that a paste failed, but what kind of site it failed on.
type Situation string

const (
	// SituationOwn: a source of the patch's own is attached and syncing.
	SituationOwn Situation = "own"
	// SituationAggregator: events arrive only through an aggregator
	// crosswalk (docs/adr/056), secondhand.
	SituationAggregator Situation = "aggregator"
	// SituationAttachable: nothing is attached, but the site carries a
	// calendar Patchwork can read. The answer is to attach it, not to
	// write a paragraph about it.
	SituationAttachable Situation = "attachable"
	// SituationSquarespace: a Squarespace site whose events live on a
	// plain page rather than an Events collection.
	SituationSquarespace Situation = "squarespace"
	// SituationWordPress: a WordPress site publishing no feed and no
	// markup.
	SituationWordPress Situation = "wordpress"
	// SituationOther: a reachable site on no platform Patchwork
	// recognizes, publishing nothing it can read.
	SituationOther Situation = "other"
	// SituationNone: no website on the patch, or one that cannot be
	// fetched.
	SituationNone Situation = "none"
)

// Situations lists every value a template needs a paragraph for.
// Attachable is deliberately absent: it is advice to the admin, and an
// intro that goes out while it stands uses Other.
var Situations = []Situation{
	SituationOwn, SituationAggregator, SituationSquarespace,
	SituationWordPress, SituationOther, SituationNone,
}

// ValidSituation reports whether s names a paragraph.
func ValidSituation(s string) bool {
	for _, v := range Situations {
		if string(v) == s {
			return true
		}
	}
	return false
}

// Classification is one situation with the one-line reason the preview
// shows beside it, so the admin can disagree with it on the facts.
type Classification struct {
	Situation Situation `json:"situation"`
	Reason    string    `json:"reason"`
	// AttachURL is set for Attachable: the address to paste into the
	// event-source form.
	AttachURL string `json:"attach_url,omitempty"`
	// AggregatorName is set for Aggregator: whose calendar the events
	// come through, for the paragraph to name.
	AggregatorName string `json:"aggregator_name,omitempty"`
}

// classifyFetchTimeout bounds the one fetch Classify makes. The admin is
// waiting on a preview, so the answer has to arrive or stop.
const classifyFetchTimeout = 15 * time.Second

// Classify answers where nodeID's calendar stands. The first two
// situations come from event_sources as they are. The rest cost one
// fetch of the website, through the same guarded client the sources use,
// on the caller's request: nothing here runs on a schedule, and no
// result is persisted, because the question is only asked when somebody
// is about to write to the patch.
func Classify(ctx context.Context, db *database.DB, nodeID, website string) Classification {
	var own int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM event_sources WHERE node_id = ? AND aggregator_id IS NULL AND status = 'ok'`,
		nodeID,
	).Scan(&own); err != nil {
		return Classification{Situation: SituationNone, Reason: "could not read the patch's sources: " + err.Error()}
	}
	if own > 0 {
		return Classification{Situation: SituationOwn, Reason: "a source on the patch's own site is attached and syncing"}
	}

	var aggName string
	if err := db.QueryRow(
		`SELECT a.name FROM event_sources s JOIN aggregators a ON a.id = s.aggregator_id
		 WHERE s.node_id = ? ORDER BY s.created_at LIMIT 1`, nodeID,
	).Scan(&aggName); err == nil && aggName != "" {
		return Classification{
			Situation:      SituationAggregator,
			Reason:         "events arrive only through " + aggName,
			AggregatorName: aggName,
		}
	}

	website = strings.TrimSpace(website)
	if website == "" {
		return Classification{Situation: SituationNone, Reason: "the patch has no website"}
	}

	fctx, cancel := context.WithTimeout(ctx, classifyFetchTimeout)
	defer cancel()
	result, err := fetchFeed(fctx, website, "", "")
	if err != nil {
		return Classification{Situation: SituationNone, Reason: "the website could not be read: " + err.Error()}
	}
	return classifyBody(fctx, website, result.Body)
}

// classifyBody is Classify after the fetch, split out so a test can hand
// it a page without a server.
func classifyBody(ctx context.Context, website string, body []byte) Classification {
	now := time.Now().UTC()

	// The probes the event-source form runs, in the same order
	// (loadItemsFor), on a body already in hand.
	if items, err := ParseJSONLD(body, now, time.UTC); err == nil && len(items) > 0 {
		return Classification{
			Situation: SituationAttachable,
			Reason:    fmt.Sprintf("the page carries schema.org Event markup (%d events); attach it", len(items)),
			AttachURL: website,
		}
	}
	if looksLikeTribe(website, body) {
		return Classification{
			Situation: SituationAttachable,
			Reason:    "the site runs The Events Calendar, whose export Patchwork reads; attach it",
			AttachURL: website,
		}
	}

	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "squarespace") {
		if jsonURL, err := squarespaceJSONURL(website); err == nil {
			if ssResult, err := fetchFeed(ctx, jsonURL, "", ""); err == nil && !ssResult.NotModified {
				if items, err := ParseSquarespace(ssResult.Body, now, website); err == nil && len(items) > 0 {
					return Classification{
						Situation: SituationAttachable,
						Reason:    fmt.Sprintf("the page is a Squarespace Events collection (%d events); attach it", len(items)),
						AttachURL: website,
					}
				}
			}
		}
		return Classification{
			Situation: SituationSquarespace,
			Reason:    "a Squarespace site, but this page is not an Events collection",
		}
	}
	if strings.Contains(lower, "wp-content") || strings.Contains(lower, "wp-json") {
		return Classification{
			Situation: SituationWordPress,
			Reason:    "a WordPress site publishing no feed and no Event markup",
		}
	}
	return Classification{
		Situation: SituationOther,
		Reason:    "the site publishes nothing a calendar can subscribe to",
	}
}
