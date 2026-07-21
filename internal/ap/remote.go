package ap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"
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

// allowPrivateAddresses disables the SSRF guard below. Off in production;
// a seam for tests and for deliberately federating loopback dev instances.
var allowPrivateAddresses = false

// SetAllowPrivateAddresses toggles the private-address guard and returns
// the previous value so callers can restore it.
func SetAllowPrivateAddresses(v bool) bool {
	prev := allowPrivateAddresses
	allowPrivateAddresses = v
	return prev
}

// isPublicIP reports whether ip is a plausible public unicast address.
// Loopback, RFC1918/ULA, link-local, multicast, and unspecified addresses
// are all refused: actor URLs are attacker-influenced (inbound keyIds,
// user-supplied remote follows), and fetching them must never become a
// probe of the host's own network (SSRF).
func isPublicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified())
}

// guardedDialContext dials like net.Dialer but refuses non-public
// addresses at connect time — after DNS resolution, so DNS-rebinding to a
// private IP is caught, and inside the transport, so redirects are
// covered by the same check.
func guardedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("ssrf guard: %w", err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("ssrf guard: unparseable address %q", host)
			}
			if !allowPrivateAddresses && !isPublicIP(ip) {
				return fmt.Errorf("ssrf guard: refusing non-public address %s", ip)
			}
			return nil
		},
	}
	return d.DialContext(ctx, network, addr)
}

// fetchClient is the HTTP client for all remote actor fetches.
var fetchClient = &http.Client{
	Timeout:   10 * time.Second,
	Transport: &http.Transport{DialContext: guardedDialContext},
}

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
