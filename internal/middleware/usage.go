package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Usage counts: page views per route per day and distinct visitors per day,
// on the server, with no script in the page
// (docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md).
//
// What makes this an infrastructure gauge and not analytics is what it
// refuses to keep. The finest grain stored is a day and a route pattern:
// never a URL with its query string, never a moment, never an address. A
// visitor is told apart within one day by a hash of the client address and
// user agent under a salt drawn at random and held only in memory; the salt
// is replaced at midnight UTC and on every restart, so nothing on disk can
// turn a count back into a person and no two days can be joined. Counts sit
// in memory and are written to SQLite as totals once a minute.
//
// The counter only ever wraps the SPA handler, so it sees document loads
// and not API traffic or assets; a person clicking around the app after the
// first load is one view. That undercounts, deliberately: the alternative is
// a beacon from the page, which is the "analytics script" the shipped
// privacy policy says this site does not run.

// UsageRetention is how long a daily row is kept. Thirteen months matches
// the longest lifetime the strictest audience-measurement exemption (CNIL)
// tolerates, and gives a steward the same month a year apart.
const UsageRetention = "-13 months"

// usageFlushInterval bounds how much counting a crash loses.
const usageFlushInterval = time.Minute

// UsageCounter accumulates counts in memory and flushes them to the
// database. One exists per process; Wrap installs it on a handler.
type UsageCounter struct {
	db      *database.DB
	enabled func() bool

	mu       sync.Mutex
	day      string
	salt     [32]byte
	views    map[string]int            // path -> views, for the current day
	visitors map[[16]byte]struct{}     // hashes seen today
	flushed  int                       // visitors already written for today
	pending  map[string]map[string]int // day -> path -> views not yet written (day rolled over mid-interval)
	pendingV map[string]int            // day -> visitors not yet written after rollover
}

// NewUsageCounter builds a counter. enabled is read per request so the admin
// switch takes effect without a restart; when it answers false nothing is
// hashed and nothing is counted.
func NewUsageCounter(db *database.DB, enabled func() bool) *UsageCounter {
	c := &UsageCounter{db: db, enabled: enabled}
	c.rotate(time.Now().UTC().Format("2006-01-02"))
	return c
}

// rotate starts a new day: a fresh salt, empty sets. Caller holds mu, or is
// the constructor.
func (c *UsageCounter) rotate(day string) {
	if c.day != "" {
		// Carry the old day's unwritten counts to the next flush.
		if c.pending == nil {
			c.pending = map[string]map[string]int{}
			c.pendingV = map[string]int{}
		}
		if len(c.views) > 0 {
			c.pending[c.day] = c.views
		}
		if n := len(c.visitors) - c.flushed; n > 0 {
			c.pendingV[c.day] += n
		}
	}
	c.day = day
	if _, err := rand.Read(c.salt[:]); err != nil {
		// Without randomness the hash would be stable across days, which is
		// exactly the property this design forbids; count nothing instead.
		log.Printf("usage: no random salt (%v); visitor counting paused", err)
		c.salt = [32]byte{}
	}
	c.views = map[string]int{}
	c.visitors = map[[16]byte]struct{}{}
	c.flushed = 0
}

// Start flushes on a ticker until ctx ends, then flushes once more.
func (c *UsageCounter) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(usageFlushInterval)
		defer ticker.Stop()
		pruned := ""
		for {
			select {
			case <-ctx.Done():
				c.Flush()
				return
			case <-ticker.C:
				c.Flush()
				if today := time.Now().UTC().Format("2006-01-02"); today != pruned {
					c.prune()
					pruned = today
				}
			}
		}
	}()
}

// Wrap counts document loads served by next. It is meant for the SPA
// handler only.
func (c *UsageCounter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Method == http.MethodGet && c.enabled() && countableDocument(r) {
			c.Record(ClientIP(r), r.UserAgent(), NormalizeUsagePath(r.URL.Path))
		}
	})
}

// countableDocument says whether a request is a person loading a page:
// not an asset, not a fetch the browser labels as something other than a
// navigation, and not a crawler or preview bot announcing itself.
func countableDocument(r *http.Request) bool {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/assets/") || strings.HasPrefix(p, "/ap/") || strings.HasPrefix(p, "/.well-known/") {
		return false
	}
	if last := p[strings.LastIndex(p, "/")+1:]; strings.Contains(last, ".") {
		return false // a file: bundle, stylesheet, icon, font, tile
	}
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" && dest != "document" {
		return false
	}
	ua := strings.ToLower(r.UserAgent())
	if ua == "" {
		return false
	}
	for _, tok := range usageBotTokens {
		if strings.Contains(ua, tok) {
			return false
		}
	}
	return true
}

// usageBotTokens are substrings that mark a user agent as software rather
// than a person: search crawlers, link-preview fetchers, federation
// servers, and command-line clients. Matched case-insensitively.
var usageBotTokens = []string{
	"bot", "crawl", "spider", "slurp", "preview", "fetch", "feed", "externalhit", "whatsapp",
	"curl/", "wget/", "python", "go-http-client", "http.rb", "okhttp",
	"java/", "libwww", "headless", "lighthouse", "monitor", "uptime",
	"mastodon", "pleroma", "misskey", "akkoma", "friendica",
}

// Record counts one page load. Exported so tests can drive it directly.
func (c *UsageCounter) Record(ip, ua, path string) {
	today := time.Now().UTC().Format("2006-01-02")
	c.mu.Lock()
	defer c.mu.Unlock()
	if today != c.day {
		c.rotate(today)
	}
	c.views[path]++
	if c.salt == ([32]byte{}) {
		return
	}
	h := sha256.New()
	h.Write(c.salt[:])
	h.Write([]byte(ip))
	h.Write([]byte{0})
	h.Write([]byte(ua))
	var key [16]byte
	copy(key[:], h.Sum(nil))
	c.visitors[key] = struct{}{}
}

// Flush writes accumulated counts to the database as totals.
func (c *UsageCounter) Flush() {
	c.mu.Lock()
	type dayViews struct {
		day   string
		views map[string]int
	}
	var batches []dayViews
	visitors := map[string]int{}
	for d, v := range c.pending {
		batches = append(batches, dayViews{d, v})
	}
	for d, n := range c.pendingV {
		visitors[d] += n
	}
	c.pending, c.pendingV = nil, nil
	if len(c.views) > 0 {
		batches = append(batches, dayViews{c.day, c.views})
		c.views = map[string]int{}
	}
	if n := len(c.visitors) - c.flushed; n > 0 {
		visitors[c.day] += n
		c.flushed = len(c.visitors)
	}
	c.mu.Unlock()

	for _, b := range batches {
		for path, n := range b.views {
			if _, err := c.db.Exec(`INSERT INTO usage_days (day, path, views) VALUES (?, ?, ?)
				ON CONFLICT(day, path) DO UPDATE SET views = views + excluded.views`, b.day, path, n); err != nil {
				log.Printf("usage: flush views: %v", err)
			}
		}
	}
	for d, n := range visitors {
		if _, err := c.db.Exec(`INSERT INTO usage_visitors (day, visitors) VALUES (?, ?)
			ON CONFLICT(day) DO UPDATE SET visitors = visitors + excluded.visitors`, d, n); err != nil {
			log.Printf("usage: flush visitors: %v", err)
		}
	}
}

// prune drops rows older than the retention window.
func (c *UsageCounter) prune() {
	for _, table := range []string{"usage_days", "usage_visitors"} {
		if _, err := c.db.Exec(`DELETE FROM `+table+` WHERE day < date('now', ?)`, UsageRetention); err != nil {
			log.Printf("usage: prune %s: %v", table, err)
		}
	}
}

// Reset forgets everything counted today that is not yet written, so a
// Clear from the admin panel is not undone by the next flush.
func (c *UsageCounter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending, c.pendingV = nil, nil
	c.views = map[string]int{}
	c.visitors = map[[16]byte]struct{}{}
	c.flushed = 0
}

// The SPA's first path segments. Anything else is a scan or a typo and is
// counted under one bucket so the table cannot be grown by junk.
var usageKnownRoots = map[string]bool{
	"my": true, "patches": true, "map": true, "events": true, "submit": true,
	"label": true, "about": true, "lining": true, "governance": true,
	"privacy": true, "terms": true, "claims": true, "quilts": true,
	"users": true, "login": true, "invite": true, "signup": true,
	"welcome": true, "discover": true, "settings": true,
	"notifications": true, "activity": true, "dashboard": true, "admin": true,
}

// usageRootsWithSlug are the roots whose second segment names one thing
// (a patch, a person, a host, an event, a token) and is replaced.
var usageRootsWithSlug = map[string]string{
	"patches": "{slug}", "users": "{username}", "quilts": "{host}",
	"events": "{id}", "invite": "{token}",
}

// usageLiteralSeconds are second segments under a slug root that are
// pages of their own, not a thing's name.
var usageLiteralSeconds = map[string]bool{"new": true, "my": true}

var usageIDLike = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NormalizeUsagePath turns a request path into the route pattern it was
// served under: '/patches/the-selvage/events/0199…' becomes
// '/patches/{slug}/events/{id}'. Query strings never reach it. At most four
// segments survive, and a root the SPA does not have collapses to
// '(other)'.
func NormalizeUsagePath(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return "/"
	}
	segs := strings.Split(p, "/")
	if !usageKnownRoots[segs[0]] {
		return "(other)"
	}
	if len(segs) > 4 {
		segs = segs[:4]
	}
	if ph, ok := usageRootsWithSlug[segs[0]]; ok && len(segs) > 1 && !usageLiteralSeconds[segs[1]] {
		segs[1] = ph
	}
	if segs[0] == "quilts" && len(segs) > 3 && segs[2] == "patches" {
		segs[3] = "{slug}"
	}
	for i := 2; i < len(segs); i++ {
		if usageIDLike.MatchString(segs[i]) {
			segs[i] = "{id}"
		}
	}
	return "/" + strings.Join(segs, "/")
}
