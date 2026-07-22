package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

func blockHandler() http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return middleware.BlockAICrawlers(ok)
}

func requestWithUA(ua string) int {
	r := httptest.NewRequest("GET", "/", nil)
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	}
	w := httptest.NewRecorder()
	blockHandler().ServeHTTP(w, r)
	return w.Code
}

func TestBlockAICrawlers_Blocked(t *testing.T) {
	// A representative sample of real crawler UA strings that must be rejected.
	blocked := []string{
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; GPTBot/1.2; +https://openai.com/gptbot",
		"Mozilla/5.0 (compatible; ClaudeBot/1.0; +claudebot@anthropic.com)",
		"CCBot/2.0 (https://commoncrawl.org/faq/)",
		"Mozilla/5.0 (compatible; PerplexityBot/1.0; +https://perplexity.ai/perplexitybot)",
		"Mozilla/5.0 (compatible; Bytespider; https://zhanzhang.toutiao.com/)",
		"meta-externalagent/1.1 (+https://developers.facebook.com/docs/sharing/webmasters/crawler)",
	}
	for _, ua := range blocked {
		if code := requestWithUA(ua); code != http.StatusForbidden {
			t.Errorf("expected 403 for %q, got %d", ua, code)
		}
	}
}

func TestBlockAICrawlers_Allowed(t *testing.T) {
	// These must pass: federation, link-preview, and search agents, plus a
	// normal browser and an empty UA. facebookexternalhit is the trap — it
	// must NOT be caught by the "facebookbot" token.
	allowed := []string{
		"",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0 Safari/537.36",
		"http.rb/5.1.1 (Mastodon/4.2.1; +https://mastodon.social/)",
		"Pleroma 2.5.0; https://pleroma.example",
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
		"Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"Mozilla/5.0 (compatible; Applebot/0.1; +http://www.apple.com/go/applebot)",
	}
	for _, ua := range allowed {
		if code := requestWithUA(ua); code != http.StatusOK {
			t.Errorf("expected 200 for %q, got %d", ua, code)
		}
	}
}

func TestRobotsTxt(t *testing.T) {
	r := httptest.NewRequest("GET", "/robots.txt", nil)
	w := httptest.NewRecorder()
	middleware.RobotsTxt().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain, got %q", ct)
	}
	body := w.Body.String()
	for _, tok := range []string{"GPTBot", "ClaudeBot", "Google-Extended", "Disallow: /"} {
		if !strings.Contains(body, tok) {
			t.Errorf("robots.txt missing %q", tok)
		}
	}
}
