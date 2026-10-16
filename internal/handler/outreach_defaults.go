package handler

// The intro email that ships with Patchwork
// (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md, decision
// 7). An instance replaces any part of it at Admin → Settings → Outreach;
// the structure stays: say what the thing is, say how it works, say what
// it does not do, say the page can be taken down on request, and stop.
//
// Placeholders, filled at preview time:
//
//	{patch_name}       the patch
//	{patch_url}        its public page
//	{claim_url}        its claim page (carries the shortcut token when sent
//	                   to the verification domain)
//	{quilt_name}       this quilt
//	{quilt_url}        this quilt's address
//	{sender_name}      the admin sending it
//	{event_count}      upcoming events on the patch right now
//	{aggregator_name}  whose calendar the events come through (aggregator
//	                   paragraph only)

const defaultIntroSubject = "{patch_name} is on {quilt_name}"

const defaultIntroOpener = `Hi,

I'm {sender_name}, and I run {quilt_name} ({quilt_url}), a shared calendar and map of what's going on around here. There are no ads, no algorithm, and no plan to make money from it, and the software is open source.

I made a page for {patch_name} on it at {patch_url}, using what you already have public. If anything on it is wrong, tell me and I'll fix it, and if you'd rather not be listed at all I'll take it down. You can also claim the page and run it yourselves: {claim_url}. It takes a few minutes and lets you edit the description, post events, and hand it off to whoever does your socials.`

var defaultIntroCalendar = map[string]string{
	"own": `Your events are already on there, {event_count} right now. {quilt_name} reads the calendar on your site a few times a day, so anything you add there shows up on its own and you never have to post it twice. If one of them looks off that's my end, so let me know.`,

	"aggregator": `Your events are on there too, but secondhand, from the {aggregator_name} calendar, so only what gets submitted there shows up. {quilt_name} can read a calendar straight off your own site instead, which would be the full list, but yours doesn't publish a feed yet. If there's someone who manages the site I'm glad to talk to them about it. Either way you can post events directly once you've claimed the page.`,

	"squarespace": `Events are the one thing I couldn't bring over. {quilt_name} can read a calendar straight off a site, but your events page is a regular Squarespace page rather than an Events page, so there's nothing for it to subscribe to. Switching the page type (Pages, then the plus, then Events) looks the same to your visitors, and I can connect it once it's up. If that's more than you want to deal with, claim the page and post events there, or send me a list and I'll put them in.`,

	"wordpress": `Events are the one thing I couldn't bring over, since your site doesn't publish a calendar feed and that's what {quilt_name} reads. On WordPress the free Events Calendar plugin does it once iCal export is turned on, and schema.org Event markup works too. If someone handles your site I'm glad to talk to them. Otherwise claim the page and post events there, or send me a list and I'll put them in.`,

	"other": `Events are the one thing I couldn't bring over, since your site doesn't publish a calendar feed and that's what {quilt_name} reads. If someone handles your site I'm glad to talk to them about it. Otherwise claim the page and post events there, or send me a list and I'll put them in.`,

	"none": `Events are the one thing I couldn't bring over, since there's no calendar on a site for me to read. If you claim the page you can post them there, or send me your next few dates and I'll put them in.`,
}

const defaultIntroSignoff = `Thanks,
{sender_name}`
