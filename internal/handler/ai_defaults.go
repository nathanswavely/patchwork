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
// Its words are held like any other copy: SlopChop's copy-check fails CI on
// a string nobody has decided, and the ledger records which parts are a
// person's and which are AI wording a person kept.
//
// {quilt_name} is substituted at serve time, as in legal_defaults.go.
// {ai_disclosure} is whichever of the three paragraphs below matches the
// quilt's disclosure switch (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md),
// the way {usage_stats} is rendered into the privacy policy: the switch
// decides what the forms ask, so the page has to say which way it is set.
//
// Formatting: one line per paragraph and per list item. The frontend renders
// markdown with breaks:true, so a wrapped paragraph would render ragged.

const aiPageProject = `## Does AI touch what you post here?

Patchwork doesn't use AI to evaluate, train on, or compile any user behavior or attributes. I am trying to build something that is human-first. Humans have been successfully building community for millennia without AI, and while there may be AI tools that promise to help with things like moderation, aggregation, etc... I am trying to move slowly in this regard as our culture figures out how to build shared language around this divisive technology. Patchwork is open source, so if you truly believe there is a particular AI tool that aligns with the mission, you are free to suggest. However be warned that you will need to make a case that goes beyond just "convenience."

The {quilt_name} server opens connections to other machines for these reasons and no others:

{outbound}

This list is built into the code, so a change that reaches out to anything else will cause the build to fail.

{ai_disclosure}

## Can AI companies take what is here?

Not as far as we can control. The site's robots.txt instructs these crawlers to stay away, and the server refuses these known crawlers:

{ai_crawlers}

If you know of any others, or any more sophisticated ways of preventing this, please let me know.

## Was this software built with AI?

Yes. Patchwork's codebase was developed with help from AI coding tools. People design it, make every decision about how its governance works, and review the features that ship. All code (human or AI) is available in the open source repository for public evaluation.

The core of what visitors and newcomers read is human written by me: the lining, the Label, and the pages that explain what Patchwork is and why it exists. A significant amount of copy, including the charter templates, the guides to how governance works, the privacy policy, and the buttons and messages around them, was drafted with AI and reviewed by me. I would love to have these more critically examined, critiqued, and even rewritten by human experts on governance and community building. If you are interested in helping, please reach out to me or better yet, contribute to the project.

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

// The three states of the disclosure paragraph. Each one describes what the
// forms do under that setting; change the forms and change these.
const (
	aiDisclosureOffText      = `**Labels.** {quilt_name} does not ask people whether AI made what they post. The stewards have turned that question off.`
	aiDisclosureOptionalText = `**Labels.** When someone posts an event, a proposal, a notice, a charter change, a patch description, or an image link, they are provided a disclosure form asking whether AI made it. Answering is optional. Anything its author says AI made carries a "Made with AI" tag. A patch can also say that its work relies on AI, and any user can hide those patches from your own browsing in your account settings.`
	aiDisclosureRequiredText = `**Labels.** When someone posts an event, a proposal, a notice, a charter change, a patch description, or an image link, a form requires them to disclose whether AI made it. Anything its author says AI made carries a "Made with AI" tag. A patch can also say that its work relies on AI, and users can hide those patches from your own browsing in your account settings.`
)
