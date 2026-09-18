# ADR: Counting visitors without watching anyone

Date: 2026-09-18. Status: accepted. Amends ADR 028 (the shipped privacy
policy gains a paragraph that is rendered from a setting, because the thing
it describes is a switch). Decided when the Lancaster quilt started getting
users and its steward wanted to know how many.

## Context

A quilt that has started to get users wants two numbers: how many people
read it, and whether they do anything once they have. The software kept
neither. The shipped Caddyfile writes no access log, the Go server has no
request logging, and the admin panel's Overview deliberately dropped its
size-and-growth counts because a steward could not act on them
(`internal/handler/admin_overview.go`). The privacy policy meanwhile claimed
"the web server keeps ordinary request logs", which was not true of anything
that ships.

The obvious answers are all third parties or second processes. A hosted
analytics script is what the shipped policy says this site does not have.
Self-hosting Plausible or Umami means a second service and a Postgres, which
breaks the single-binary, runs-on-a-Pi rule and would have to be repeated by
every quilt that seamrips. A Caddy access log plus GoAccess is a fine tool
for one operator and ships nothing to the next.

The peers this project follows converge on one pattern. Mastodon, Codeberg,
Forgejo, Discourse's default and Wikipedia set a session cookie, describe it
in a paragraph of the privacy policy, and show no banner. Where a project
counts visitors at all, it counts them in house (Wikimedia's EventLogging)
or with a cookieless salted hash of address and user agent held only in
memory (Plausible, Umami, GoatCounter). A banner appears exactly where a
third-party cookie-setting tool was added (mozilla.org with Google
Analytics; Discourse only via a theme component once an admin adds such a
tool).

The law bears this out for a US deployment: a signed-in session cookie is
strictly necessary everywhere, no US state requires a banner, Pennsylvania's
privacy bill has not passed the Senate, and CCPA does not reach a
non-commercial site. For a quilt that might one day be run in Europe, the
strictest reading of the ePrivacy rules (the EDPB's 2024 guidelines treat
IP-based identification as within scope; the CNIL exempts audience
measurement only when it is first party, anonymous in aggregate, holds no
cross-site identifier, is disclosed, and is bounded in retention) sets a bar
the design below was written to clear.

## Decision

**No banner, and no separate cookie policy.** The Cookies section of the
privacy policy is the cookie policy, as it is for every project named above.
It now also names the browser-storage conveniences the app keeps (a collapsed
sidebar, a draft, that you have seen the introduction), because a policy that
lists one cookie and omits localStorage is a policy that was not read
against the code.

**Visitor counting is a server-side gauge, off by default.** When an
instance admin turns it on, a middleware wrapped around the SPA handler
alone counts two things and nothing else:

- page views per **route pattern** per UTC day: `/patches/{slug}`, never
  `/patches/the-selvage`, and never a query string;
- distinct visitors per UTC day, where a visitor is `sha256(salt ‖ ip ‖ ua)`
  under a 32-byte salt drawn at random, held only in memory, and replaced at
  midnight UTC and on every restart. Raw addresses and user agents are never
  written anywhere. No two days can be joined, because nothing that could
  join them survives the day.

Counts accumulate in memory and are written to two tables of daily totals
(`usage_days`, `usage_visitors`) once a minute; rows older than thirteen
months are deleted. API calls, assets, non-navigation fetches (by
`Sec-Fetch-Dest`), and any user agent that announces itself as software
(crawlers, link-preview fetchers, federation servers, command-line clients)
are not counted. Paths outside the SPA's route table collapse to one
`(other)` bucket so a scanner cannot grow the table.

**There is no script in the page.** The counter sees document loads, so a
person who opens one page and clicks through six is one view. That
undercounts, on purpose: the alternative is a beacon from the browser, which
is the "analytics script" the policy says this site does not run, and which
the EDPB counts as gaining access to the device. A gauge of reach does not
need the extra precision; a product funnel would, and this is not one.

**Engagement is a query, not a collection.** The Usage tab shows accounts
created, patches joined and followed, events posted and proposals opened in
the same window, read from the tables the quilt keeps anyway. Those numbers
are the same whether counting is on or off, and they answer the question a
page-view total cannot: whether anyone did anything.

**The policy reads the switch.** `{usage_stats}` in the shipped privacy
policy is substituted at serve time with one of two paragraphs, so the
default text is true in both states and the act of turning counting on is
also the act of disclosing it. The "on" paragraph states every property
above in plain words; each is a line of `internal/middleware/usage.go`, and
the two change together. The policy's claim about request logs is corrected
to what ships. It also now names the map tile servers (OpenFreeMap, and
OpenStreetMap's raster fallback), the one third-party fetch a page here
makes.

**The switch lives on the Usage tab**, beside the numbers, rather than in
Quilt settings: the decision to have counts and the counts themselves should
never be on different pages. Turning it on or off is audited on its own line
(`admin.usage_stats_set`), and a **Clear** deletes every row and whatever the
counter holds in memory, audited as `admin.usage_cleared`.

**The tables stay behind in a seamrip.** They are this deployment's traffic,
not the community's record, and a fork starts by counting nothing.

## Consequences

- A steward gets daily page loads, daily distinct visitors, a per-route
  breakdown, and an activity summary, from the binary, on a Pi, with no
  third party and no consent prompt.
- The counts undercount single-page navigation and count a returning
  browser twice on a day the server restarted. Both are stated on the tab.
- Every quilt that seamrips inherits the switch, off, and a policy that says
  so.
- An operator who adds a Caddy access log is adding a record the policy
  does not describe. `docs/DEPLOYMENT.md` says how to do it with the client
  address masked and a short retention, and says the policy has to change
  in the same breath.
- The visitor count is the one place the software hashes an address at
  all. If a future feature wants a per-user or per-session metric, that is
  a different decision and needs a different ADR, not a third column here.
