package ap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
)

// RemoteActor holds the fields we care about from a fetched remote actor document.
type RemoteActor struct {
	ID        string
	Inbox     string
	PublicKey string // PEM-encoded
}

// actorFetcher fetches and parses a remote actor document. It is a package
// variable so tests can substitute a stub instead of making real HTTP calls.
var actorFetcher = httpFetchActor

// actorCacheTTL is how long a fetched actor (its inbox + public key) stays valid
// in the cache. Inbound verification fetches the signing actor's key on every
// POST; caching collapses that to one fetch per actor per TTL. Keys rotate
// rarely, so a day is a reasonable balance between freshness and load.
var actorCacheTTL = 24 * time.Hour

type actorCacheEntry struct {
	actor   *RemoteActor
	expires time.Time
}

var (
	actorCacheMu sync.Mutex
	actorCache   = map[string]actorCacheEntry{}
)

// FetchActor retrieves a remote actor document by its AP ID (URL) and extracts
// the inbox URL and public key PEM. Successful results are cached for
// actorCacheTTL, keyed by actor ID, so repeated inbound POSTs from the same
// actor don't refetch the document each time.
func FetchActor(ctx context.Context, actorID string) (*RemoteActor, error) {
	actorCacheMu.Lock()
	if e, ok := actorCache[actorID]; ok && timeNow().Before(e.expires) {
		actorCacheMu.Unlock()
		return e.actor, nil
	}
	actorCacheMu.Unlock()

	actor, err := actorFetcher(ctx, actorID)
	if err != nil {
		return nil, err
	}

	actorCacheMu.Lock()
	actorCache[actorID] = actorCacheEntry{actor: actor, expires: timeNow().Add(actorCacheTTL)}
	actorCacheMu.Unlock()
	return actor, nil
}

// ClearActorCache drops all cached actors. Tests call it for isolation; it is
// also invoked when the fetcher is swapped.
func ClearActorCache() {
	actorCacheMu.Lock()
	actorCache = map[string]actorCacheEntry{}
	actorCacheMu.Unlock()
}

// SetActorFetcher overrides the actor fetcher (used in tests). It returns the
// previous fetcher so callers can restore it, and clears the cache so a stubbed
// fetcher isn't shadowed by entries from a prior fetcher.
func SetActorFetcher(f func(ctx context.Context, actorID string) (*RemoteActor, error)) func(ctx context.Context, actorID string) (*RemoteActor, error) {
	prev := actorFetcher
	actorFetcher = f
	ClearActorCache()
	return prev
}

// timeNow is a clock seam so tests can control cache expiry.
var timeNow = time.Now

// SetAllowPrivateAddresses toggles the shared private-address guard
// (internal/safehttp) and returns the previous value so callers can
// restore it. Kept here so existing callers and tests need no rewiring.
func SetAllowPrivateAddresses(v bool) bool {
	return safehttp.SetAllowPrivateAddresses(v)
}

// fetchClient is the SSRF-guarded HTTP client for all remote actor
// fetches: actor URLs are attacker-influenced (inbound keyIds,
// user-supplied remote follows).
var fetchClient = safehttp.NewClient(10 * time.Second)

func httpFetchActor(ctx context.Context, actorID string) (*RemoteActor, error) {
	if actorID == "" {
		return nil, fmt.Errorf("empty actor id")
	}
	if u, err := url.Parse(actorID); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("actor id must be an http(s) URL")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", actorID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("User-Agent", "Patchwork/1.0")

	resp, err := fetchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch actor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch actor: http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("read actor body: %w", err)
	}

	return parseActor(body)
}

// parseActor extracts the fields we need from an actor document's JSON.
func parseActor(body []byte) (*RemoteActor, error) {
	var doc struct {
		ID        string `json:"id"`
		Inbox     string `json:"inbox"`
		PublicKey struct {
			PublicKeyPem string `json:"publicKeyPem"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse actor: %w", err)
	}
	if doc.PublicKey.PublicKeyPem == "" {
		return nil, fmt.Errorf("actor has no public key")
	}
	return &RemoteActor{
		ID:        doc.ID,
		Inbox:     doc.Inbox,
		PublicKey: doc.PublicKey.PublicKeyPem,
	}, nil
}
