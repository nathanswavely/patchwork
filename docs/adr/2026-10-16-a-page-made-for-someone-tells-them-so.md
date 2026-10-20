# A page made for someone tells them so

Date: 2026-10-16. Status: **accepted**; implemented. Builds on docs/adr/030 (claims are
anchored on a vetted domain), docs/adr/018 (email is one relay, the
instance's own domain and reputation), docs/adr/026 and the 2026-09-18
trust-scope record (who may act on an unclaimed patch), and docs/adr/028
(shipped text with an admin override).

## Context

A young quilt is mostly unclaimed patches. The admin, or a trusted
contributor, makes a page for a venue or a band from what is already public,
and the page sits there until its owner happens to find it. Nothing in
Patchwork tells the owner it exists. On the reference instance, 2026-10-13,
sixty-odd unclaimed patches had been made over three months and none of their
owners had been contacted.

The admin did it by hand instead, and the hand process is worth recording,
because it is the spec. Pull the list of unclaimed patches. For each one,
work out which of five situations its calendar is in: Patchwork reads the
patch's own site; Patchwork gets its events secondhand through an aggregator
feed; the site is Squarespace with events on a plain page instead of an
Events collection; the site is WordPress with no feed and no markup; there is
no site to read. Write one message with an opener everyone gets and a calendar
paragraph chosen by situation. Find an address. Send. Note who was sent what.

Two things about that process. The judgement parts were small and the same
every time: the five situations were decided by fetching the site and looking
at it, which is exactly what the event-source probe already does when someone
pastes a URL (docs/adr/031 and its amendments: ICS, then schema.org markup,
then the Squarespace JSON view). And the parts that were genuinely human were
also the parts that went wrong. A slug had a typo. Two test patches and a
duplicate were in the list. A message drafted for everyone had to be
re-drafted so it did not read as a machine.

One more fact that shapes the design. docs/adr/030 only accepts an email
claim at an address on the patch's `verification_domain`, because control of
that mailbox is the proof. An intro sent to `info@` at that domain is
therefore sent to an address that could claim the patch. The intro and the
claim are the same trust, pointed in opposite directions.

## Decision

**1. An intro is sent by a person, after seeing it, to an address a person
chose.** There is no send on insert. Creating or approving an unclaimed patch
offers an optional contact address; the patch workspace offers *Send intro*
whenever an address is on file and no intro has gone to it. Both paths render
the whole message first, with its calendar paragraph already chosen, and the
admin reads it before the button. Preview-then-confirm is how every write
tool in this codebase already works, and an email to a stranger is not the
place to drop it. The typo'd slug and the test patches are the argument: an
automatic send would have sent them.

**2. Only an instance admin sends.** An intro goes out from the instance's
own domain, on the relay docs/adr/018 configured, and the instance's sending
reputation is the admin's to spend. The trust-scope record gives trusted
contributors exactly the gates that are about one patch; this gate is about
the quilt's name. A per-patch or quilt-wide trusted contributor may enter or
suggest an address for an unclaimed patch, and it waits in the admin's view
until the admin sends. A contributor-made patch never emails anyone on its
own.

**3. Addresses are entered, never discovered.** No scraping `mailto:` links,
no `info@domain` guesses, no enrichment. An address on file is one a person
typed because they chose to write to it. That is what keeps an intro
one-to-one correspondence from a named sender, which is both the tone the
message needs and the line that keeps it out of bulk-mail territory. The
address lives on the patch as workspace-only data: never public, never on the
actor, and it does not travel through the seamrip boundary (docs/adr/002),
because it was given to this steward for one correspondence and a fork is a
steward the giver never heard of.

**4. The calendar paragraph is chosen by the probe, and the admin can
overrule it.** At preview time the server classifies the patch into one of
the five situations from the Context. The first two come from
`event_sources` as they stand: an own-site source with `status = 'ok'`, or
only an aggregator crosswalk (docs/adr/056). The other three need one fetch of
`nodes.website` through the same guarded client the sources use, and the body
in hand already decides them: schema.org Event markup or a Squarespace JSON
view means the admin should attach the source rather than send a paragraph
about it; a Squarespace page without an events collection, a WordPress page
without a feed, or nothing reachable each pick their paragraph. The fetch
happens on the admin's action, on the request, and is not a worker. The
preview shows which situation was picked and why in one line, and the admin
may pick a different paragraph. The chosen situation is recorded on the send.

**5. One send per patch per address, recorded.** A send writes a row (patch,
address, sender, situation, time) and an audit entry, and *Send intro* reads
as sent afterwards. Resend is a separate, explicit act. There are no
follow-ups and no sequence; a second message to someone who did not answer
the first is the thing this design exists to not be.

**6. An intro to the verification domain carries a claim shortcut.** When the
address is at the patch's `verification_domain`, the message includes a link
holding a single-use token, valid for days rather than hours, that counts as
the email leg of a docs/adr/030 claim. It does not sign anyone in and it
does not create an account: the recipient still arrives through a magic link,
a passkey, or an invite, and only then does the token verify an email-method
claim for that account. Setup follows as it does today. An address at any
other domain gets the ordinary claim link and the ordinary path. Changing the
address on file revokes any unused token.

**7. The text is a shipped default with an instance override.** The opener,
the five calendar paragraphs, and the sign-off ship in the binary and are
edited at Admin, the way the legal documents are (docs/adr/028): placeholders
for the patch, its page, its claim link, the event count, and the sender's
name. The default is written in the register the reference instance settled
on by hand: say what the thing is, say how it works, say what it does not do,
say the page can be taken down on request, and stop. Instances fork the
text; the structure stays.

**8. Without SMTP, the preview is still the product.** An instance with no
relay configured gets the same rendered message and a copy button instead of
a send button, and the send row is written when the admin says they sent it
elsewhere. The classification and the text are the hard part; the relay is
one line of config.

**9. An instance can turn it off.** `outreach.enabled` in instance settings,
default on. Some quilts will prefer that nobody is written to who did not ask
first, and that is a legitimate way to run a quilt.

## Amended 2026-10-19: a contact is a place to reach someone

The first cut read "contact" as "email address", and the reference
instance's first pass at its backlog found the obvious hole: many of the
organizations a young quilt lists have no public email at all and live on
Instagram. So a contact is now a place to reach someone, in two shapes
that may both be on file:

- **An email address**, as before, with everything above unchanged.
- **A link to message them on** (`nodes.contact_url`): a full `http(s)`
  address, entered by a person, never a handle guessed into a network. The
  patch's own public links are offered as one-click choices, since the
  Instagram on the listing is usually the door.

A message on a link is a **dm**: rendered from a short opener of its own
(the first two lines are all a message preview shows), the same calendar
paragraph, no subject and no sign-off. It is always copied out and sent by
hand; nothing here posts to someone else's inbox on a social site. The
send is recorded against the link with `intro_sends.channel = 'dm'`, under
the same one-per-destination rule.

**A dm never carries the claim shortcut.** Decision 6 rests on docs/adr/030:
a mailbox at the verification domain is proof of the domain. A social
account proves nothing about it, so a dm carries the ordinary claim page
and its recipient claims the ordinary way.

When both an address and a link are on file the preview opens on email,
because it is the channel that can carry the shortcut, and the admin can
switch. Copy-and-record is now offered for email too, for an admin who
prefers their own mail client to the relay.

## Rejected alternatives

- **Send on creation.** Fastest to build, and the one the hand process
  argues against: the mistakes that were caught were caught between drafting
  and sending.
- **Discovering an address from the site.** Turns a listing into a mailing
  list. Also fails in practice: the addresses on venue sites are box-office
  and booking addresses, and the person who runs the Instagram is someone
  else.
- **Letting trusted contributors send.** Their grant delegates the admin's
  review queue for one patch's calendar; it does not delegate the instance's
  domain.
- **A send-all button for the backlog.** Sixty previews is an afternoon, and
  the afternoon is where the typos are found. A list view with per-row send
  covers the backlog without it.
- **Follow-up sequences.** Patchwork has no monetization strategy and no
  funnel, so nothing needs a second touch. One message, then the page waits.
- **A scheduled classification worker.** One fetch at preview time, on the
  admin's click, answers the question when it is asked. A worker answers it
  for patches nobody is about to write to.
- **A second contact table keyed by person.** The address is about the patch
  and about one correspondence. Rows on the send, an address on the patch,
  nothing else.

## Consequences

- New column on `nodes` for the contact address (workspace-only, excluded
  from public reads, the actor, and the seamrip boundary with a stated
  reason, so `TestEveryTableHasABoundaryDecision` and its column-level
  siblings stay green).
- New table for sends; the boundary decision is *stays*, same reason.
- A migration named from the clock, per the 2026-09-16 naming record.
- `internal/eventsource` gains a classify function that returns one of five
  situations plus a one-line reason, reusing `loadItemsFor`'s probes on a
  body it already has. The event-source form can use the same function to
  say why a paste failed, which it cannot today.
- A new audit action, `outreach.send`, and a resend variant.
- The docs/adr/030 claim flow gains a token source: an email it did not
  itself send. The token's lifetime and single use are the whole of the
  difference, and both are stated above.
- Admin surfaces: a contact address on the unclaimed-patch form and the
  suggestion-approval form; *Send intro* with preview in the patch workspace
  for unclaimed patches; an unclaimed-patches list at Admin with contacted
  state; the template at Admin beside the legal documents.
- The reference instance's hand-written kit becomes the shipped default
  text, with its Lancaster specifics removed.
- A finding from the hand process that is a bug on its own: some site bodies
  make the ICS library panic rather than return an error, and nothing in the
  source path recovers. The classify function will fetch more sites than
  the attach form ever did, so that fix lands first.
