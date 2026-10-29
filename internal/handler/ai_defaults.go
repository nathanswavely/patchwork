package handler

// The project part of the AI page (docs/adr/2026-10-20-the-site-says-where-ai-is.md).
// It ships in the binary and an instance cannot edit it, for the reason
// docs/adr/037 pins the lining: a deployment that could keep this page and
// change its facts would turn a disclosure into decoration. The stewards'
// part, which an instance does write, is stored separately
// (settings.KeyAIPageStewards).
//
// Every claim here is rendered from, or tested against, the thing it
// describes:
//
//   - {ai_crawlers} is middleware.AICrawlerNames(), and
//     TestAICrawlerNamesAreRefused proves each one is turned away;
//   - {outbound} is aiPageOutbound below, and
//     TestEveryOutboundCallerIsOnTheAIPage fails the build when the server
//     gains a way to reach another machine that this list does not name;
//   - TestNoAIServiceDependency fails when go.mod or web/package.json gains
//     a model provider's SDK.
//
// And the page's own words are a person's: TestAIPageWordsAreAPersons reads
// the copy ledger and fails until every string in this file and in
// web/src/pages/AiPage.svelte is recorded as human. A disclosure page left as
// a model drafted it would break the policy it states.
//
// {quilt_name} is substituted at serve time, as in legal_defaults.go.
//
// Formatting: one line per paragraph and per list item. The frontend renders
// markdown with breaks:true, so a wrapped paragraph would render ragged.

const aiPageProject = `*Draft. A model wrote this text from the design record. A person rewrites it before this page ships, and a test holds the release until they have.*

## Does AI touch what you post here?

No. Nothing on {quilt_name} sends what you write, or anything about you, to an AI model. No model summarizes, ranks, rewrites, moderates or generates anything here. Accounts are for people, and nothing on this site posts by itself.

The server opens connections to other machines for these reasons and no others:

{outbound}

That list is checked against the code. A change that lets the server reach anything else fails the build until this page says what it is.

## Can AI companies take what is here?

Not with permission. The site's robots.txt asks these crawlers to stay away, and the server refuses the ones that arrive under their own name anyway:

{ai_crawlers}

That has limits. A scraper that lies about its name looks like any other browser and gets in. And public posts that federate are copied to other servers, which keep them under their own rules.

## Was this software built with AI?

Yes. Patchwork's codebase was developed with significant help from AI coding tools. People design it, makes every decision about how its governance works, and reviews the features that ship. All code (human or AI) is available in the open source repository for public evaluation.

The writing a visitor meets first is a person's: the lining every patch agrees to, the Label, and the pages that explain what Patchwork is and why it exists. Much of the rest, including the charter templates, the guides to how governance works, and the buttons and messages around them, was drafted with AI and reviewed by a person before it shipped.

The project's repository keeps [a public record of who wrote each string](https://github.com/nathanswavely/patchwork/blob/main/copy/REPORT.md) a visitor can read.`

// aiPageOutbound is every way the running server reaches another machine,
// and what the page says about each. Files are repository paths; the test
// scans the source for outbound calls and fails on any file not listed here
// or in its short list of exemptions (a healthcheck to itself, a command an
// operator runs by hand).
var aiPageOutbound = []struct {
	Files []string
	Says  string
}{
	{
		[]string{"internal/ap/delivery.go", "internal/ap/remote.go"},
		"Other servers in the fediverse, when the stewards have turned federation on, to deliver public posts to their followers and to read their public keys.",
	},
	{
		[]string{"internal/eventsource/fetch.go", "internal/handler/event_sources.go"},
		"Calendars a patch has connected, to copy in their events.",
	},
	{
		[]string{"internal/handler/claims.go"},
		"A patch's own website and domain records, when someone claims the patch and the site checks that they run it.",
	},
	{
		[]string{"internal/mail/mail.go"},
		"The mail server the stewards set up, to send sign-in links and the notifications you turn on.",
	},
}
