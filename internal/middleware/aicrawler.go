package middleware

import (
	"net/http"
	"strings"
)

// AI training/scraping crawlers get two layers of "no":
//
//  1. RobotsTxt serves a /robots.txt that opts out of them by product token.
//     Well-behaved crawlers (GPTBot, ClaudeBot, CCBot, and the robots-only
//     opt-out tokens Google-Extended / Applebot-Extended) honor this.
//  2. BlockAICrawlers rejects the ones that show up anyway with a 403, matching
//     on User-Agent, for the crawlers that ignore robots.txt.
//
// Both layers deliberately leave federation, link previews, and search alone:
//   - ActivityPub/federation servers (Mastodon, Pleroma, "http.rb", …) send
//     their own user agents and don't read robots.txt — untouched by either.
//   - Link-preview bots (facebookexternalhit, Discordbot, Slackbot, Twitterbot,
//     Signal) carry none of the tokens below — the SEO middleware still serves
//     them og: tags.
//   - Search crawlers (Googlebot, Bingbot, Applebot) are not in either list.
//
// The two lists are intentionally NOT identical: Google-Extended and
// Applebot-Extended are robots.txt-only product tokens — they are never sent
// as a real request User-Agent (Googlebot/Applebot fetch with their normal
// UA), so they belong only in robots.txt, never in the 403 matcher.

// aiCrawlerUATokens are lowercase substrings of User-Agent strings belonging to
// AI crawlers we refuse to serve. Matched case-insensitively against the raw
// UA. Every token here is specific enough that it cannot appear in a
// federation, link-preview, or search-engine user agent. Note "facebookbot"
// (Meta's LLM crawler) does not match "facebookexternalhit" (Meta's link
// preview fetcher), so shared-link previews on Facebook keep working.
var aiCrawlerUATokens = []string{
	"gptbot",           // OpenAI training crawler
	"oai-searchbot",    // OpenAI search index
	"chatgpt-user",     // ChatGPT on-demand fetches
	"claudebot",        // Anthropic training crawler
	"claude-web",       // Anthropic (legacy)
	"anthropic-ai",     // Anthropic (legacy)
	"ccbot",            // Common Crawl (feeds many training sets)
	"perplexitybot",    // Perplexity index
	"perplexity-user",  // Perplexity on-demand fetches
	"bytespider",       // ByteDance/TikTok
	"amazonbot",        // Amazon (feeds Alexa/AI)
	"cohere-ai",        // Cohere
	"diffbot",          // Diffbot knowledge graph
	"facebookbot",      // Meta LLM crawler (NOT facebookexternalhit)
	"meta-externalagent", // Meta AI crawler
	"imagesiftbot",     // ImageSift (Hive) image scraper
	"omgilibot",        // Omgili/Webz.io data resale
	"omgili",           // Omgili (bare token)
	"timpibot",         // Timpi
	"youbot",           // You.com
	"scrapy",           // common scraping framework default UA
}

// robotsTxt is served verbatim at /robots.txt. It groups the disallowed AI
// crawlers (including the robots-only opt-out tokens) and explicitly allows
// everything else, so search engines and previews stay welcome.
const robotsTxt = `# Patchwork
# AI training and scraping crawlers are not welcome here. This file opts out
# of them by name. Federation (ActivityPub) and link-preview bots do not use
# robots.txt and are unaffected; search engines are explicitly allowed.

User-agent: GPTBot
User-agent: OAI-SearchBot
User-agent: ChatGPT-User
User-agent: ClaudeBot
User-agent: Claude-Web
User-agent: anthropic-ai
User-agent: CCBot
User-agent: Google-Extended
User-agent: Applebot-Extended
User-agent: PerplexityBot
User-agent: Perplexity-User
User-agent: Bytespider
User-agent: Amazonbot
User-agent: cohere-ai
User-agent: Diffbot
User-agent: FacebookBot
User-agent: meta-externalagent
User-agent: ImagesiftBot
User-agent: Omgilibot
User-agent: Omgili
User-agent: Timpibot
User-agent: YouBot
Disallow: /

User-agent: *
Allow: /
`

// RobotsTxt serves the static robots.txt opt-out. Mount at GET /robots.txt.
func RobotsTxt() http.HandlerFunc {
	body := []byte(robotsTxt)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(body)
	}
}

// BlockAICrawlers returns 403 to requests whose User-Agent matches a known AI
// crawler token, for the crawlers that ignore robots.txt. It should sit at the
// outermost edge of the middleware stack so matching requests are rejected
// before any handler work. It never touches federation, preview, or search
// user agents — see the package notes above.
func BlockAICrawlers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := strings.ToLower(r.Header.Get("User-Agent"))
		if ua != "" {
			for _, tok := range aiCrawlerUATokens {
				if strings.Contains(ua, tok) {
					w.Header().Set("X-Robots-Tag", "noindex, nofollow")
					http.Error(w, "AI crawlers are not permitted on this instance.", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
