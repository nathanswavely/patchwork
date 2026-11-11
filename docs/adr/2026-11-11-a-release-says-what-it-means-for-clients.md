# A release says what it means for clients

Date: 2026-11-11. Status: **accepted**; implemented. Amends docs/adr/085.

## Context

The web app ships with the server: the two are one image, so they can never
disagree about the API. Every other client ships on its own schedule. The
iOS app goes through App Store review. Other apps, if anyone builds them, and
anyone's script against `/api/v1` meet a new server before they have been
updated, and keep meeting older ones too.

docs/adr/085 gave the operator two flags and a page of prose. Nothing in it
was addressed to the people who build clients. Some releases grew a "For
client developers" section anyway (v0.35.0 has one), but nobody was asked
for it, and v0.34.0 shows what that costs. It shipped the `ai_disclosure`
switch (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md). On
`required`, every create without `ai_use` became a 400. For the iOS app that
meant every event, patch, suggestion, proposal and notice it could post. The
release notes said nothing about it to clients. It was found later, by a
parity audit of the iOS app. It was not `breaking` in 085's sense: an operator
who updated got a working instance, and the switch defaults to `optional`.
It broke the clients on any quilt whose admin turned it on.

## Decision

From v0.36.0, a release's notes answer for clients the way they already
answer for operators, and `cmd/releasenotes` refuses a file that does not.

1. **`client_breaking`**, a required boolean in the front matter. It is true
   when a client that worked against the previous release can be refused, or
   can misread something, on an instance running this one. That includes
   behind a setting this release adds, if an admin can turn it on, because a
   client cannot know in advance which quilts will. It is carried into
   `release.json` beside the other two, so a client's own tooling can gate
   on it.
2. **A `## For client developers` section** in the body, spelled exactly
   that way. It lists every change to what the API accepts or returns, and
   says first whether any of it is breaking and what a client must do. A
   release with nothing for clients says so in a line. The section is
   required even then: "nothing" is a decision somebody made, and a missing
   section cannot be told apart from a forgotten one.
3. **Files before v0.36.0 are left as they are.** The key is optional there,
   but a real boolean when present, and `release.json` reports `false`. They
   were written before anyone was asked. Rewriting them would put words in
   releases that never said them.

As with 085, only a person can make the call. The checker can tell that the
section is there and not empty. It cannot tell whether what it says is true.

## Considered and not done

**An OpenAPI document, or a generated schema.** It is the real answer once
clients are built by people outside the project: it can be diffed between
tags and generated into client code. It is a large piece of work against
handlers that decode their own request structs, and it would describe shapes
without saying which settings refuse which requests. The section is what a
client developer needs now. A schema, if it comes, should feed the section,
not replace it.

**A capability endpoint.** `/api/v1/instance` already reports the modules a
quilt has on and its `ai_disclosure` mode. A client should keep reading
settings from there rather than inferring them from the version. That is a
habit for each new setting, not a mechanism, and it is not decided here.

**Folding it into `breaking`.** 085 already lists "an API change an existing
client notices" as breaking. But an operator gating unattended updates on
`breaking` should not hold back a release that changes nothing for their
instance. The two readers want different answers, which is why 085 split
`irreversible_migrations` out in the first place.
