// Records: reading one collection out of one repository (docs/adr/063).
//
// This is the whole atproto read surface — a listRecords call against the
// account's own PDS. No relay, no AppView, no firehose (ADR 058's first
// constraint), and no auth: the records are public.
package atproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// EventCollection is the community-owned calendar lexicon. It belongs to
// lexicon.community rather than to any one app, which is what makes it
// safe to read: nobody here can change it out from under a publisher.
const EventCollection = "community.lexicon.calendar.event"

// maxPages bounds a listRecords walk. A venue's calendar is small; a
// repository that never stops paging is a bug or a hostile host, and
// either way this is a poller and not a crawler.
const maxPages = 10

// Record is one repository record with its key.
type Record struct {
	URI   string          `json:"uri"`
	CID   string          `json:"cid"`
	Value json.RawMessage `json:"value"`
}

// Rkey returns the record key — the last path segment of the AT-URI. It is
// stable for the life of the record, which is what the event reconciler
// keys on (docs/adr/063 decision 4).
func (r Record) Rkey() string {
	i := strings.LastIndex(r.URI, "/")
	if i == -1 || i == len(r.URI)-1 {
		return ""
	}
	return r.URI[i+1:]
}

type listResponse struct {
	Records []Record `json:"records"`
	Cursor  string   `json:"cursor"`
}

// ListRecords walks one collection in one repository, following the
// cursor. `pds` is the endpoint from the DID document's service entry.
func (r Resolver) ListRecords(pds, did, collection string) ([]Record, error) {
	if pds == "" {
		return nil, errors.New("no PDS endpoint")
	}
	var all []Record
	cursor := ""
	for page := 0; page < maxPages; page++ {
		q := url.Values{}
		q.Set("repo", did)
		q.Set("collection", collection)
		q.Set("limit", "100")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		body, err := r.fetch(pds + "/xrpc/com.atproto.repo.listRecords?" + q.Encode())
		if err != nil {
			return nil, err
		}
		var resp listResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("listRecords response is not valid JSON: %w", err)
		}
		all = append(all, resp.Records...)
		// A PDS that keeps handing back the same cursor would loop until
		// maxPages; an empty page ends the walk regardless.
		if resp.Cursor == "" || resp.Cursor == cursor || len(resp.Records) == 0 {
			break
		}
		cursor = resp.Cursor
	}
	return all, nil
}

// ParseATURI splits at://<did>/<collection> into its parts. Patchwork
// stores a source this way so the feed survives a handle rename
// (docs/adr/063 decision 1).
func ParseATURI(uri string) (did, collection string, err error) {
	rest, ok := strings.CutPrefix(uri, "at://")
	if !ok {
		return "", "", errors.New("not an at:// URI")
	}
	did, collection, ok = strings.Cut(rest, "/")
	if !ok || did == "" || collection == "" {
		return "", "", errors.New("at:// URI needs a repository and a collection")
	}
	if !strings.HasPrefix(did, "did:") {
		return "", "", errors.New("at:// URI must name a DID, not a handle")
	}
	return did, collection, nil
}

// ATURIFor builds the stored form for a repository's event collection.
func ATURIFor(did string) string { return "at://" + did + "/" + EventCollection }
