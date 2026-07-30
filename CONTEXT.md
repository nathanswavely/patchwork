# Patchwork

Glossary for the Patchwork community organizing platform. Backend code uses
generic terms; the UI speaks textile. This file pins down the canonical
language so the two never drift.

## Language

**Patch**:
A single community entity (band, venue, collective). Flat and equal — no
hierarchy between patches. Backend term: node.
_Avoid_: group, community (as an entity name), stitch

**Unclaimed patch**:
A directory listing for a real place or organization whose owner hasn't
arrived yet. Follow-only: it can be followed and claimed, never joined —
membership in an organization that hasn't admitted anyone is a fabricated
relationship, and for the same reason it carries no governance and no
lining: agreement by an organization that hasn't arrived is fabricated
consent. Claiming turns it into that owner's patch.
_Avoid_: ghost patch, placeholder, stub

**Claim**:
An assertion of ownership over an unclaimed patch, pending proof — never a
reservation. One person's unproven claim doesn't stop anyone else from
proving theirs: claims on the same patch run concurrently, and the first
to verify (or be approved) wins; the rest are auto-rejected. A user holds
at most one open claim per patch. A verified or approved claim is a
single-use, expiring right to enter patch setup — until setup is
submitted, the patch is still unclaimed to every visitor, and an expired
claim simply makes the patch claimable again.
_Avoid_: lock, hold, reservation; "claimed" for a patch whose setup
hasn't been submitted

**Patch setup**:
The completion of a claim: the patch creation flow, prepopulated with the
unclaimed listing's data. A claim is creation with prepopulated fields,
not a handoff — the claimant can change anything creation allows except
the slug (the patch's existing public address) and the verification
domain (the trust anchor the claim just proved). Submitting setup is the
moment the patch becomes active, the claimant becomes its admin, and the
lining is adopted — shown out loud, exactly as at creation.
_Avoid_: handoff, transfer, onboarding

**Verification domain**:
The trust anchor for self-service claims on an unclaimed patch — the one
domain the platform has vetted as actually belonging to the organization.
Set only by instance admins and trusted contributors (auto-derived from
the website they supply, unless that domain is a shared platform like
Facebook or Gmail). Every self-verification method proves control of this
domain; without one, the only path to claiming is admin review. Distinct
from the website field, which is cosmetic and carries no trust.
_Avoid_: website (as a trust anchor), domain (unqualified)

**Archive**:
The end of a patch's public life, done from its danger zone by a patch
admin (or an instance admin, who is also the only one who can archive an
unclaimed patch). An archived patch vanishes from the quilt and every
route refuses it, but nothing is erased — memberships, events, and
history stay frozen in place. Not deletion: wipe erases, archive
preserves. The seamrip export zip is never called an archive in the UI.
_Avoid_: delete (as the UI word for this), remove, hide

**Restore**:
The instance-admin-only action that returns an archived patch to the
status it held before archiving, everything intact. Deliberately not
self-service — archive keeps its gravity because undoing it takes a
human with site-wide responsibility (docs/adr/034). Restore is silent:
the patch reappearing is the announcement.
_Avoid_: unarchive, undelete, reactivate

**Event**:
Something a patch hosts at a time and, optionally, a place — a show, a
meeting, a workday. Deliberately the same word in the UI and the backend;
the textile coinage "pin" is retired (docs/adr/027). It collided on every
side: events literally appear as map pins on the Leaflet map, software
readers hear "pinned post", and the UI had already grown Events pages,
routes, and notification labels while "pin" survived mainly in docs. A
term nobody reaches for when building the thing isn't the thing's name.
_Avoid_: pin (retired), happening, gathering

**Event link**:
An explicit, mutual association between an event and a patch beyond its
owner — admins on one side propose, admins on the other confirm, and
either side removes it at will. The event stays the owner's to edit; the
linked patch gains presence: the event card reads "with" the linked patch,
and the event counts as that patch's activity. Flat — a link carries no
role label. Not a thread (threads are inferred; a link is declared), and
distinct from detach's territory: severing a link is "remove", never
"unlink" (docs/adr/032).
_Avoid_: co-host, tagged patch, on the bill, unlink (reserved against
detach), shared event

**Cross-quilt mention**:
A display-only doorway on an event page pointing at a patch on another
quilt — "with Cool Band ⧉" — created when the owner pastes a remote patch
URL. Not an event link: no handshake, no calendar placement, no counts;
it has the standing of naming the band in the description
(docs/adr/032).
_Avoid_: remote link, federation link

**Thread**:
A connection between two patches, inferred from shared admins and members
— never declared, never drawn by hand. Followers don't make one: interest
is not overlap. A thread is what the quilt's proximity means, so it has no
control anywhere in the UI; the only way to make one is for people to
belong to both patches. Distinct from placement affinity, the broader
internal weighting the layout runs on (shared events, shared followers,
shared tags), none of which is a thread.
_Avoid_: edge (the removed explicit-connection concept), link (that is an
event link, which is declared), connection (in UI copy — say thread)

**Member count**:
Admins plus members — never followers. A follower is an interested
observer, not a member; follower interest is its own count. The two are
never summed in anything user-facing.
_Avoid_: community size (ambiguous), total members (when it includes
followers)

**Upcoming events**:
A patch's events that have not yet started — the number the patch profile
states. Distinct from **event count**, which is every active event a patch
owns or is linked to, past and future, and is what the quilt tile and
cross-quilt snapshots carry. The two are never labelled with each other's
word. A capped page of rows is never the count: reading five rows and
printing "5" is how a venue with forty shows came to advertise five.
_Avoid_: events (when the number is upcoming-only), event count (that is
the all-time number)

**Tag**:
A label a patch wears, chosen by that patch's admins — many per patch —
from a single vocabulary curated by the instance admin. Patch admins pick
from the list; only instance admins change the list. Tags power discovery:
filtering, onboarding interests, and the tag-derived motif. Shared tags
also weakly attract patches in the quilt — a declared similarity that
matters most for patches too new or thin to have member overlap, and that
never outweighs a single shared member. This attraction is part of the
placement algorithm, not a user-facing concept, and is not a thread.
There is no second classification system — "category" is this concept and
never a separate one. Tags never label people or events — an event matches
a tag through its patch.
_Avoid_: category, genre, topic, label (as a noun)

**Quilt**:
The treemap visualization of all patches on one instance, placed by member
overlap (with shared tags as a weak attractor for patches that lack it). The quilt is a view, not an entity. By sanctioned metonymy, "the
quilt" also names the whole instance's community fabric as its people speak
of it — Connected Quilts, quilt icon, Quilt settings, multi-quilt. The
metonym never replaces "instance" in backend code.
_Avoid_: tree, map (for this visualization)

**Instance**:
One self-hosted Patchwork deployment. UI term: a Patchwork.
_Avoid_: site, server (when meaning the deployment)

**Patch admin**:
A person with the admin role on a specific patch. Customizes that patch,
including its tile appearance.
_Avoid_: quilt admin, moderator, owner (as a role name)

**Instance admin**:
A person with the site-wide admin role on an instance. Curates instance-wide
options; does not override per-patch choices.
_Avoid_: quilt admin, superadmin

**Steward**:
A person publicly accountable for how a quilt is run, named on its Label.
Stewardship is a stated responsibility, not a permission: holding the
instance-admin role never publishes a person, and a person without it can
still be a steward. Each steward owns whether they are listed and says in
their own words what they look after; a handle alone is enough, no legal
name required. A Label always names at least one — the buck stops
somewhere — but no one is conscripted into being the face. The people who
govern the commons are these same stewards doing the money-specific part
of the job, never a second role.
_Avoid_: quilt admin, operator, maintainer, owner, host (that is the
fiscal host or the machine)

## Shell & navigation

**Global bar**:
The single slim top bar that persists on every screen: context crumb, a
contextual search, the notification bell, and the user menu. The bell and
user menu never move.
_Avoid_: top bar (per-context), header, navbar

**Context crumb**:
The global bar's leading slot. The quilt mark alone in discovery; mark /
patch name in a workspace; mark / Administration in the admin panel. The
mark is always the exit home.
_Avoid_: breadcrumb (it is not a path trail), logo

**Workspace**:
A patch's full-screen management and participation surface at the
canonical patch URLs (/patches/:slug/governance|members|events|settings;
/manage/* survives only as a redirect — docs/adr/003). Takes over below
the global bar with its own top-level nav. Not admin-only: members vote
there, followers can browse.
Design analogy: a GitHub repository — analogy only (docs/adr/005 records
why "repo" was rejected as a name). Chiefly an architecture word: the UI
speaks it in exactly one place, the patch profile's overflow ("Workspace
view"), and reaches it everywhere else by naming the room — Governance,
Members, Events, Settings (docs/adr/042).
_Avoid_: repo (collides with the git-backed governance repos), patch admin
area, manage area, workspace (as the label on a patch profile's primary
way in — that door names a room or does not exist)

**Scope**:
The switch between seeing "My Quilt" and the whole instance. A discovery
concept — it does not exist inside a workspace or the admin panel.
_Avoid_: workspace (as the switcher label), lens, view, filter (that word
belongs to tag filtering)

**My Quilt**:
The scope showing every patch a person has a relationship with — admin,
member, *and* follower alike. Deliberately wider than belonging: scope
answers "what do I care to see", not "where do I stand", so a person who
only follows still has a quilt of their own. This is the one place follows
sit beside memberships; the role mark still distinguishes them wherever a
relationship is drawn, and a follow never becomes a membership because it
appeared here. My Quilt is per-person and applies identically to every
surface that renders the quilt's patches — treemap, cards, and map.
Patches followed on other quilts appear here too, grouped by their source
quilt and never intermingled with home patches; a remote region can only
ever hold follows, since membership doesn't cross quilts.
_Avoid_: my patches (implies membership only), my memberships, joined

**Filter**:
The standing narrowing state — the tag selection plus the search chip —
that narrows every discovery surface at once: treemap, cards, map, and
events. Independent of scope: it stays on until the person clears it, so
it must never be silently active — its home is the filter chips, and
clearing is always one step. Session-ephemeral: reload clears it; nothing
else does. Filtering is a discovery concept, like scope.
_Avoid_: facet, refinement, tag search, category filter

**Filter chips**:
The filter's control and indicator, one thing: the full tag vocabulary
as toggleable chips, the search chip among them, living on every
discovery surface — over a canvas's top edge (quilt, map), at the top of
the page flow elsewhere. Wherever narrowing bites, the chips stand on
it; non-discovery pages carry nothing, since nothing on them is
narrowed. Collapsible to a single badged button — one shared preference,
open by default on desktop, closed on mobile; on mobile canvases the
chips open as a sheet, which is open-while-using, never a preference.
_Avoid_: filter card (retired), filter bar, chip row, tag toggles

**Search chip**:
The search query's standing form: a chip among the filter chips, set
only by the search dropdown's "Show matches on the quilt" row, cleared
like any tag chip. Typing alone never sets it — narrowing is always an
explicit act.
_Avoid_: saved search, active query, query lens (internal shorthand only)

**Search**:
The global bar's contextual search: an autocomplete dropdown over the
entities of the current context, opened by typing, resolved by picking a
result. Context decides the corpus — discovery: all public patches and
upcoming events, plus the one action row that sets the search chip and (when
submissions are open) a navigation row to suggest a missing patch — the
suggest row leaves for the submission form like any result, and never sets
the chip, so "the one action row" still names the narrowing act alone;
workspace: that patch's members, proposals, documents, events; admin
panel: users, reports, submissions. Never instance-wide people: people
appear only where a context legitimizes the result (a workspace's
members, the admin panel's users) — people are discovered through
patches. Code artifacts keep the older "finder" naming (WorkspaceSearch,
finderProviders); prose does not.
_Avoid_: finder, scoped finder (renamed), search index, command palette,
global search

**Admin panel**:
The instance admin surface at /admin. Gets the same full-screen takeover
treatment as a workspace.
_Avoid_: admin area, dashboard (that is the user's personal page)

## Layout & spacing

**Gutter**:
The inset between the viewport edge and content. Exactly one gutter applies
to any point on screen — it is set once by the shell and never re-applied
by a page or a surface inside it. Distinct from a card's own inner padding,
which is a different token on a different scale: shrinking the gutter for a
phone must not shrink what's inside the cards.
_Avoid_: sashing (that is the framing between remote-quilt regions — it
lives on the quilt canvas, not in page layout), margin, page padding,
container padding

**Card**:
A bordered surface holding something that could move — an item that would
still make sense on a different page (a patch tile, an event, a member, a
proposal in a list). Sections of a page are not cards: a settings section
is the page, so it separates with a heading and a rule, not a box.
_Avoid_: panel, box, tile (that is a patch on the quilt), section (a
section may or may not be a card)

**Interruption**:
The one other thing that earns a border: a surface that is loud on purpose
because it breaks the reading flow — a danger zone, a warning callout, an
unsaved-changes banner. A closed category, deliberately small, so that
"but this needs a box" can't re-admit page sections as cards.
_Avoid_: alert card, notice card, banner (that is the global bar's word)

## People & profiles

**Person**:
A human with an account, as the UI speaks of them. Deliberately no textile
coinage — people are not artifacts; the textile vocabulary is reserved for
the things people make and do. Backend term: user. The role words (admin,
member, follower) carry the relationships.
_Avoid_: quilter (brushes against quilt, the instance view), maker,
stitcher (collides with the removed stitch concept)

**Profile**:
A person's public page at /users/:username — name, avatar, bio, and their
visible memberships, each showing its role (admin or member) so the
contributor ladder is legible. Follows never appear on a profile; only the
person sees their own follows. Readable by anonymous visitors. The person's
federated actor carries identity only (name, bio, avatar); memberships
appear on the profile but never federate, so hiding one takes effect
immediately on every surface the instance controls. A patch's equivalent
public page is the patch profile.
_Avoid_: account page (that is settings), dashboard (the personal page),
profile (unqualified, when a patch is meant — say patch profile)

**Username**:
A person's permanent handle, chosen by them at account creation — never
derived from their email address, never assigned. Lowercase letters,
digits, and hyphens only; immutable once chosen, because it is the
profile URL (/users/:username) and the federated acct: identity.
Usernames share one namespace with patch slugs (WebFinger resolves both),
so a username can never claim an existing patch's slug. Reserved words
(admin, patchwork, …) can't be claimed by anyone.
_Avoid_: handle (informal), account name, login (nobody logs in with it)

**Membership visibility**:
A per-membership choice owned by the member: visible or hidden. One switch
controls both directions — whether the membership appears on the person's
profile and whether the person appears in the patch's public member list.
Default: visible. A hidden membership is still seen by that patch's admins
and members inside the workspace. There is no profile-only or list-only
hiding; the two surfaces never disagree.
_Avoid_: private membership (collides with private patches), profile
visibility (it is per-membership, not per-profile)

**Role mark**:
The icon that carries a person's relationship to a patch, used the same
way everywhere the relationship shows: heart = follower, three users =
member, wrench = admin. The quilt name badge's star means belonging — patches
where the person is a member or admin — never a follow; a followed patch
is marked with the heart. Where space allows, the mark is paired with its
word rather than standing alone.
_Avoid_: star for follows, favorite, bookmark, owner (as a role name)

**Patch profile**:
A patch's public page at /patches/:slug — the face it shows the street.
Read at a glance: cover, description, and a glimpse of each of the
patch's surfaces. Deliberately not the workspace and deliberately without
the workspace's tab row; a person with standing enters through the
glimpses themselves — each one both a preview of a room and the way into
it — rather than through a door named for the container. The single
exception is the overflow's "Workspace view", a fallback that exists
because it costs nothing, not because the glimpses need it. Paired with a
person's Profile, which does the same job for a person.
_Avoid_: patch page (too vague — every /patches/:slug/* URL is one), patch
home, landing page, lobby

**Relationship row**:
The one row of controls on a patch profile, and the only thing on the page
that is a control rather than a glimpse. It says where the viewer stands
and offers the next rung, and it holds nothing else — no navigation, no
contribution, no moderation. At most two controls in any state: a standing
control, and the next rung if there is one. The rung's word is always
"Become a member"; "Join" belongs to joining the quilt and is never used
for a patch.
_Avoid_: action bar (it holds one kind of act, not all of them), CTA row,
join buttons

**Standing control**:
The resting form of a person's standing with a patch — "Following",
"Member", "Admin" — shown as a control rather than inferred from whatever
verb happens to be on offer. Leaving is inside it, never beside it:
Unfollow and Leave are its menu, so departure costs one deliberate extra
step and the exit never sits at the weight of a rung. It is the only place
standing is stated on a public page, so a member learns the page knows
them without having to read the word "Leave".
_Avoid_: follow button (it is not a toggle), membership badge (a badge is
not clickable), leave button

**Trusted contributor**:
An instance-level grant — given and revoked by the instance admin, never
earned automatically — that lets a person record events on unclaimed patches
without review. Orthogonal to patch roles: not a rung between member and
admin, and worth nothing on active patches, where every suggestion still
goes through that patch's admins. Review is owed to whoever owns the
calendar; the grant only waives the instance admin's own queue. Trust is
per-instance — standing on another quilt earns it nowhere.
_Avoid_: correspondent, steward, moderator, contributor (alone),
trusted user

**Community-submitted**:
The label every event on an unclaimed patch wears: recorded by the
community, not announced by the organization. Derived from the patch's
unclaimed status, never stored per event — even an instance admin's event
on an unclaimed patch carries it. An event suggested to an active patch
and approved by its admins is simply an event: adoption erases provenance.
_Avoid_: unverified (doubts the event, not the source), unofficial
(apologizes for it), community-recorded

**Session**:
A person's signed-in presence in one browser or device — the thing a
cookie holds. Deliberately has no textile coinage: the UI says plain
"session" and "active sessions", because a security surface where someone
decides which of their devices to sign out has to be read literally, and a
quilt metaphor would only fog it. A person sees and revokes only their own
sessions; no one, not even an instance admin, sees another's. Each carries
a coarse device label ("Chrome on Windows") derived from the stored
User-Agent — recognition, not fingerprinting. Signing out the current
session is logout; "sign out everywhere else" keeps it and cuts the rest.
_Avoid_: any textile term, login (the act, not the standing state),
device (a session is per-browser, not per-machine)

## Onboarding

**Intro card**:
The non-blocking card an anonymous visitor meets on their first landing
on any public surface: three sentences on what this quilt is and that no
algorithm runs it, with the two ways onward — the About page and
joining — plus a worded decline ("I'll lurk for now"), because reading
this quilt has never required an account and Join should not be the only
named way forward. Overlaid on a corner of the surface, never covering
it; on deep-linked landings (a shared event, a patch profile) it takes
its most compact form and never competes with the shared content.
Dismissed once, gone forever; the sidebar's "What is Patchwork?" entry
remains as the standing path to the About page. Signed-in people never
see it.
_Avoid_: welcome modal, splash screen, announcement banner, popup (as a
UI word)

**Join sheet**:
The statement shown between clicking Join and standing as a member or
requester: the patch's membership policy, its lining state (including an
amended lining, with the changes one link away), and its published
charters. A lens over the patch's public face, never a bypass of document
visibility — a members-only charter stays unseen. Informative, not
contractual: no checkbox; joining informed is the agreement. On
approval-required patches it carries the one optional intro message to
the admins — a field, never a questionnaire. Follows never see it:
following has no ceremony.
_Avoid_: join agreement, application form, consent modal, membership form

**Unlock panel**:
The dismissible panel a new member meets on their first workspace visit
after joining or being approved: what membership just made visible —
members-only charters, proposals and their vote, the member list. The
full governance introduction happens here, after acceptance, because
this is when the documents become readable: nobody agrees to documents
they weren't allowed to read. A panel, not a wizard — it never blocks
the workspace.
_Avoid_: welcome wizard, tour, member orientation (as a blocking step)

**Setup checklist**:
The panel a patch's admins see in their workspace until their patch has
its footing: tile designed, tags chosen, whereabouts stated (skippable —
not every patch is a place), first event posted, patch link shared,
governance decided (optional — a band never needs it). Completion is
derived from real state, never stored progress, so it cannot lie or nag
about done work. Per-admin dismissible, collapses when complete, blocks
nothing — the workspace is fully usable from second one. Applies equally
to a claim-completed owner. Deliberately never says "invite members":
account invites are instance-scoped (docs/adr/001); a patch grows by its
link traveling.
_Avoid_: onboarding wizard, getting-started guide (that is prose, this
is state), progress tracker, gamification

## Governance

**Charter**:
A governance document a patch keeps — community standards, bylaws,
whatever the patch writes down about how it runs itself. Versioned and
diffed; the database is canonical and a per-patch git repository mirrors
the history (docs/adr/011). Amended through proposals. Each charter
carries its own visibility — members only (the default for a new one) or
public; a patch **publishes** a charter, it isn't published by default
(docs/adr/036). "Governance doc" remains the backend term in code and
endpoints.
_Avoid_: lining (that is one specific charter, not the type), policy,
rules doc; "private charter" (the pair is members only / public, and
"private" is the patch-level word)

**Lining**:
The shared baseline community-standards charter — the one every active
patch on the quilt agrees to, carrying the anti-discrimination baseline.
Adoption is an act performed by a person, at patch creation or patch
setup; unclaimed patches carry no lining because nobody has performed it.
"The lining" names this document specifically, the layer that sits behind
every patch; it is not the generic word for charters. The lining is
project-owned: its text ships with Patchwork itself, and no instance
admin setting changes it — a quilt that wants a different baseline forks
the open source repo.
_Avoid_: using "lining" for governance documents in general; "default
lining" implying an instance could have a non-default one

**Amended lining**:
The user-facing state of a patch whose lining text no longer matches any
version Patchwork has shipped. Every active patch starts with the lining
and may amend it by proposal — but the lining is always public, the amendment is
public, and the patch wears an "Amended lining" badge. Reverting to a
shipped version clears the state. Viewers and instance admins can filter
amended-lining patches out of discovery. A patch on an older shipped
version is stale, not amended — staleness is resolved by auto-update, not
worn as a badge. "Diverged" is the backend/predicate term.
_Avoid_: diverged (in UI copy), modified/forked lining

**Proposal**:
Something a patch votes on. It opens for voting the moment it is raised
(docs/adr/048) and runs until its window closes: voting, then approved or
rejected, then in effect. Discussion happens alongside the vote in the
proposal's Discussion tab, not in a stage before it — the `draft` and
`discussion` states in the migration-016 column are retired and nothing
writes them.
Deliberately the same word in the UI and the backend; the textile coinage
"baste request" is retired. It explained a metaphor before it explained
the feature — every UI surface had already grown Proposals headings and
routes while "baste request" survived mainly in docs and shipped
document templates. A term nobody reaches for when building the thing
isn't the thing's name.
_Avoid_: baste request (retired), motion, petition

**Follower permissions**:
What a patch offers its followers — which workspace tabs they get, and
whether they may take part in proposals. Not access control: the events,
proposals, and member lists behind those tabs are public reads, so
switching one off hides a tab from a patch's own followers while leaving
the same data readable by anyone signed out (docs/adr/050). The one
exception is charters, which carry real per-document visibility and so
can genuinely be withheld. A patch that switches proposals off is saying
followers are not part of its deliberation, and that is honoured where it
can be — commenting — rather than pretended at by hiding public pages.
_Avoid_: permissions (in prose about what followers can *see* — the word
promises a boundary that is only there for charters), access level

**Electorate**:
The people who may vote in a patch: active admins and members who have
been there at least as long as the patch's minimum voting tenure. One
set, not two — the gate that admits a vote, the denominator quorum
divides by, the ballots a tally counts, and every surface that asks
someone to vote all name this same group (docs/adr/044). A follower is
never in it. Neither is an instance admin who holds no role in the patch:
they curate the instance, they don't vote in its patches. Distinct from
**member count**, which is the same roles without the tenure condition —
a patch can have twelve members and an electorate of four. The electorate
is who may *decide*, and nothing else gates on it: authoring a proposal
is an admin/member act with no tenure condition, and commenting is open
to followers too. Tenure asks whether someone has been here long enough
to decide, so it applies only where deciding happens.
_Avoid_: eligible members (blurs into member count), voters (that's who
actually voted), the membership (follower-inclusive)

**Voting terms**:
The rules a vote runs under, fixed when voting opens and unchanged for
that vote's life: who may vote, what quorum it needs, what threshold
carries it. A patch can amend its rules at any time; a vote already in
progress keeps the terms it opened with, because rules that move
mid-vote redraw a contest people have already cast ballots in. A rules
amendment therefore resolves under the old terms — you don't get to use
the new rules to pass the change to the new rules. Whether an approved
amendment applies itself is not a term: that switch takes effect the
moment it is flipped, because it is a safety valve, not a rule of the
contest.
_Avoid_: current rules (the point is that they may no longer be
current), snapshot (implementation), frozen rules (says how, not what)

**Direct change**:
A governance change an admin applies immediately on a patch whose
decision method is admin-decides — born applied, never voted. Stored as
an instantly-applied proposal record so it shares the governance
timeline, notifications, and history with voted proposals; the UI never
says "propose", "submit", or "vote" for one. Which framing a patch gets
follows the rules in force, not its size: the words are "change these
rules" / "rule change · applied by …" on admin-decides patches, and
"proposal" everywhere a vote actually happens.
_Avoid_: proposal (nothing is proposed to anyone), fast-track (an
implementation word, not a concept), edit (undersells that it's tracked
and visible)

**Venue**:
Where a patch's decisions actually happen — in Patchwork, or somewhere
Patchwork is not. Declared per patch and asked twice, once of proposals
and once of leadership, because a community can hold its board election
in a room and still decide everything else here. A patch with no votes to
convene for (one that runs on a maintainer) has no leadership venue to
declare.
_Avoid_: external (says where it isn't), offline (an AGM on Zoom is not
offline), integration, source of truth

**Attestation**:
A record of a decision the community made at a venue that isn't
Patchwork — an election held at the annual meeting, an amendment carried
on a show of hands. Its claim is "the membership decided this,
elsewhere", which is a far larger claim than a **direct change**'s "an
admin decided this, under rules that allow it", so the two are never
worded alike. Only offered where the patch has declared that venue:
somewhere that decides here, attesting would be a way around the vote.
Corrected only by a later attestation naming the one it corrects, never
by editing — a record that can be rewritten unseen is worth nothing.
Patchwork cannot check one, and does not pretend to: it records who
asserted what, and when, in public.

What can be attested is a **text a meeting adopted** — a charter, bylaws,
an operating agreement — and who a meeting seated. Never the **lining**,
whose body only a passed amendment proposal changes (docs/adr/037), and
never the governance rules: those are configuration rather than a text
anyone adopts, so on a patch that decides elsewhere they move by **direct
change** instead. That line also closes a hole, since a rules attestation
could have flipped the very venue that permits attesting.
_Avoid_: import, sync, manual override, verified (nothing was verified),
minutes (those are the community's own document)

**Unrealized name**:
Someone an attestation names who has not joined the patch. The record is
the community's own statement about itself, so it may name anyone; what
requires the person to have arrived is the **effect** — holding admin,
counting toward member count, feeding the quilt's affinity, receiving
anything. An unrealized name holds none of it and is never quietly
upgraded: joining is what turns it into the role the record already said
they held.
_Avoid_: pending member, ghost admin, placeholder, invited (they may
never have been)

**Council**:
The admin set of a patch that elects its leadership — not a separate body
and not a fourth role. "An elected council of up to 7 admins" is the
Formal charter's own phrasing: the council is simply who holds admin,
chosen by election rather than by appointment. A patch that appoints its
admins has no council and never sees the word.
_Avoid_: board, committee, leadership team, steward (that is quilt-level,
publicly accountable, and not a permission)

**Seat**:
A governed admin position that outlives whoever fills it, holding a term
end whether occupied or vacant. Seats exist only where leadership is
elected — they are what a term attaches to, so a patch with no terms has
admins rather than seats. A seat's holder holds the ordinary admin role;
there is no permission a seated admin has that an appointed one lacks.
The Formal charter's "when a seat opens" on a meritocratic patch is a
figure of speech for "we could use another admin", not this.
_Avoid_: chair, post, slot, council seat (redundant), position

**Term**:
The clock on a seat — the period its holder serves before that seat
stands again. The clock belongs to the seat, not the person: someone
appointed to a vacancy serves out what remains, so filling a gap can
never manufacture a fresh mandate. Terms and seats arrive together, and
only under elected leadership; the other two models rotate their admins
without either. Distinct from **tenure**, which is how long someone has
been a member and is what gates voting: a term is about holding power,
tenure is about having standing to decide.
_Avoid_: tenure (that is membership length), mandate, session

**Election cycle**:
The run of an election from nominations opening to the council being
seated. A patch's cycles are anchored by nothing configured — adopting
elected leadership starts the first one, and each cycle after is
scheduled from when the last seated its council. Distinct from a
**term**, which is what the seating starts.
_Avoid_: election season, campaign, the election (when the period rather
than the contest is meant), annual meeting (there is no meeting)

**Lapsed term**:
A seat whose term has ended while its holder is still serving — because
the election cycle has not come round, or because it ran and settled
nothing. The holder loses nothing and keeps serving until a successor is
elected. Deliberately not a removal: a clock must never be able to leave
a patch leaderless, and a community content with its leadership is not
overruled for failing to hold a vote. Publicly visible, because a council
that has not faced election in years should show it.
_Avoid_: expired (says the person is invalid), overdue, vacant (it is
filled), holdover (the mechanism's name, not a word for members)

**Election**:
A proposal that fills one or more seats. It is a proposal like any other
— same electorate, same quorum, same terms fixed when voting opens —
differing only in what a ballot says and how the result is read: the
electorate approves as many candidates as it likes, and the most-approved
take the open seats.
_Avoid_: race, poll, vote (that is the act of casting), ranked-choice
(elections approve, they do not rank)

**Candidate**:
A person named on an election's ballot for a seat. Note the collision:
**standing** in this glossary is a person's relationship to a patch (see
standing control), so a candidate *runs* for a seat and is never said to
"stand" for one.
_Avoid_: nominee (there is no separate nomination step), runner, standing
for a seat

**Recusal**:
A patch rule that keeps the person a proposal is *about* from voting on
it — a nominee sits out their own nomination. Off by default and chosen
per patch. Recusal is about one proposal, never about the person: they
remain in the electorate for everything else, and it stands down entirely
rather than leave a vote with nobody eligible to decide it. Being a term
of the contest, it freezes when voting opens like every other rule that
decides an outcome.
_Avoid_: conflict of interest (names the reason, not the rule),
abstention (that is a ballot someone chose to cast), self-vote ban

**Ballot**:
What one person casts in one contest — approve, reject, or abstain on an
ordinary proposal; the set of candidates they approve in an election. One
ballot per person per contest, always. Distinct from the **tally**, which
is what the ballots add up to, and from the **electorate**, which is who
was entitled to cast one.
_Avoid_: vote (the act of casting), ranking, preference

## Event sources

**Event source**:
A standing feed a patch pulls events from — an ICS calendar URL (a Google
Calendar's secret address, a venue tool's calendar export), a Squarespace
events page, or any page carrying schema.org Event markup (Humanitix host
pages among them) — the kind is auto-detected from a pasted address.
Attached by a
patch admin to their own patch, or by an instance admin to an unclaimed
patch, never by anyone else: attaching is vouching for the feed once, so
imported events publish without per-event review (docs/adr/031). The
source stays authoritative — its events are read-only and follow the feed
until detached. An unreachable feed never removes anything; only a
successful fetch that no longer carries an event cancels it. The UI may
say "feed" informally.
_Avoid_: calendar sync (implies two-way), import (a one-time act; a
source is standing), integration (vague), crawler

**Detach**:
The explicit act of cutting one imported event loose from its event
source: it becomes an ordinary local event — editable, deletable, no
longer synced — and the source ignores it from then on. The escape hatch
that lets imported events stay read-only without trapping admins.
_Avoid_: unlink, unsync, override

**Event upload**:
A one-time batch of events from a spreadsheet (CSV), previewed row by
row before anything is created. An admin act, deliberately narrower than
single-event posting: patch admins upload to their own patch; on
unclaimed patches the instance admin and trusted contributors do,
members and suggesters never. Not an event source — nothing syncs and
nothing stays authoritative; the rows become ordinary events the moment
they land. Re-uploading skips rows already on the calendar, and uploads
are quiet: a season arriving is not forty notifications.
_Avoid_: import (that is the sources' word for synced events), bulk
create (backend term), CSV sync (nothing syncs)

**Personal feed**:
A person's private calendar feed of every event on their My Quilt,
subscribed from their own calendar app via a secret URL they can
regenerate at any time — read-only, and never shown to anyone else.
Distinct from a patch's public calendar feed, which is anonymous and
carries only public events.
_Avoid_: my calendar (that is the person's own app), export (a download,
not a subscription), feed token (the secret is part of the URL, not a
credential the person handles)

## Quilt identity

**Quilt settings**:
The admin panel tab (/admin/quilt) where the instance's community identity
lives: rename, description, quilt icon, data export, and the danger zone.
Community identity is editable by the instance admin in the UI; deployment
concerns (domain, ports, SMTP, federation) stay in patchwork.yaml and
belong to whoever operates the machine.
_Avoid_: instance settings (UI label), site settings, general settings

**Quilt icon**:
The block that represents a whole quilt wherever quilts are listed or
switched between — the scope switcher, Connected Quilts. Drafted in the
block drafter, the same one patches use for their tiles, and stored as a
design (block plus bundle) that the server renders to SVG; there is no
image upload (docs/adr/043). Unset means hash-assigned from the quilt's
name, stable but not chosen (same rule as tile appearance). Instance
identity: it never travels in a seamrip — a fork re-brands.
_Avoid_: logo (that is branding.logo_url, a different slot), avatar (people
have those), favicon

**About page**:
The public orientation page at /about: what this quilt is and how a
Patchwork works, in the product's voice — patches, following and joining,
the quilt view, no algorithm, community-run. Orientation, not disclosure:
it exposes the Label's gist inline and hands off to /label for the full
statement of who runs the quilt and what it costs; the two pages never
trade jobs. The community's own voice here is the instance description
and the exposed Label, never a separate editable document. Reached from
the sidebar's "What is Patchwork?" entry (anonymous visitors only) and
the intro card.
_Avoid_: help page, FAQ, marketing page, landing page (discovery is the
landing), about (as a name for the Label)

**Label**:
The quilt's public statement of how it is run and paid for — who stewards
it, what outside services it depends on and why, what those cost, and how
to support or challenge the people running it. Named for the label sewn
to the back of a real quilt: maker, date, materials. A disclosure about
the deployment, not a biography of its admin — stewards are named inside
it and link to their own profiles, so the page survives a handoff
unchanged. Knowing what a quilt is made of is what makes a seamrip
actionable, so the Label always says where the door is (docs/adr/023).
Costs on it are stated by the stewards, not audited, and it says so —
each figure carries the date it was stated and the page admits when those
have gone stale.
_Avoid_: about page (boilerplate register, and it hides that the subject
is the deployment), colophon (right idea, wrong audience), credits,
imprint, quilt label (that is the name badge on a tile)

**Wipe**:
The danger-zone action that erases a quilt's community data — every patch,
person, event, proposal, and governance record — returning the deployment
to first-run. The deployment itself (domain, config, container) survives;
wiping data is not tearing down the machine.
_Avoid_: delete quilt (ambiguous about the deployment), reset (too soft),
uninstall

## Multi-quilt

**Quilt switcher**:
The global-bar dropdown holding two clearly different kinds of rows:
scope (this quilt / My Quilt), which changes the view in place, and
connected quilts, every one a doorway that opens that quilt's own site.
Objects blend, places don't: no surface ever renders another quilt's
whole view inside this one — a place is visited at its own address.
Only My Quilt blends, and only follows.
_Avoid_: instance switcher, scope switcher (scope is one kind of row
here, not the whole control)

**Neighbor quilt**:
A quilt this instance has publicly connected to, curated by the instance
admin in Quilt settings. A statement the community makes about its own
adjacency — every visitor, anonymous included, sees neighbor quilts in the
quilt switcher.
_Avoid_: partner instance, linked instance, sister quilt

**Connected quilt**:
A quilt a signed-in person has personally added for themselves, on top of
the instance's neighbor quilts. Personal, account-backed, and invisible to
everyone else. The switcher shows neighbors and connected quilts together.
_Avoid_: my quilts (collides with My Quilt, the scope), followed quilt
(quilts are connected; patches are followed)

**Remote follow**:
A follow of a patch that lives on another quilt. The UI word is simply
"follow" — same heart, same promise — because the relationship is the
same; "remote follow" exists only to talk about the mechanics. Recorded by
the follower's home instance; where both quilts federate, the person's
quilt follows the patch on their behalf — one relayed follow no matter
how many of its people follow — so events come back as notifications while
no person is ever listed on the remote quilt. A follow is as private
across quilts as it is at home. Remote follows are what My Quilt draws in a
source quilt's region — with or without that quilt staying connected, and
even while its instance is unreachable (a stored snapshot keeps the tile).
Only the person ends a follow: a patch gone from public view is marked,
never auto-unfollowed, because deletion and going-private are
indistinguishable from outside.
_Avoid_: bookmark, watch, subscription

**Doorway**:
The labeled link that hands you to another quilt's own site: every
switcher entry for another quilt, and the deeper-than-looking actions on
a remote patch card (join, RSVP, workspace). Whole quilts are always
entered through doorways — places are visited at their own address,
never rendered inside this one. A doorway is always marked as leaving.
Declining cross-quilt reads (the multi-quilt flag off) is respected,
never proxied around: such a quilt's My Quilt region draws from
snapshots, and its patches' cards fall back to pure doorways.
_Avoid_: external link (undersells the concept), redirect, mirror

**Remote patch card**:
The one in-app surface for another quilt's content: a read-only card
about a single patch from another quilt, framed in its source quilt's
sashing color, always naming where it lives. A card about the patch,
never that quilt's site embedded. Follow lives here (and posts home);
everything deeper is a doorway. Reached from My Quilt tiles,
notifications, and pasted patch links — pasting a patch's URL into the
search opens its card.
_Avoid_: remote profile, embedded view, preview (it is the full public
face)

**Sashing**:
The strip that frames each source quilt's region when My Quilt draws more
than one quilt — colored by that quilt's own branding color and carrying
its icon and name. Tiles inside keep their chosen appearance; difference
lives between tiles, never on them. Quilts are peers: once sashing
appears, every region gets it, the home quilt included. A single-region
My Quilt has no sashing at all.
_Avoid_: border (vague), frame, group outline

**Registry**:
A shareable published list of quilts that seeds the switcher for whoever
opens it — a discovery flyer, not a data source. Opening a registry link
overlays its quilts for that visit only; nothing is saved unless a person
connects a quilt themselves.
_Avoid_: directory, index, federation list

## Seed data

**Seed**:
The single fictional dataset loaded by the seeder for local development,
E2E tests, and evaluation demos. A fixture, never production content — it
must not bootstrap a real instance, and the seeder refuses databases that
hold real users.
_Avoid_: seed profile (the multi-profile mechanism is gone — docs/adr/010),
sample deployment, demo instance (both suggest something deployable)

**Real places, fictional actors**:
The rule for real-world references in seed profiles. Real geography —
streets, parks, neighborhoods, the city — may appear as setting. Any actor
in the fiction (a patch, a venue hosting a seeded event, an employer or
partner in a bio) must be invented. The test: if the reference puts a real
organization into a fabricated relationship, it's out.
_Avoid_: "based on real venues", "real Lancaster orgs"

## Leaving

**Seamrip**:
The full export of an instance's data — including member emails — that
lets a community stand up again elsewhere. A custody transfer: it moves
other people's secrets, so it is admin-gated by design (docs/adr/012).
_Avoid_: fork (the git operation), backup (that is an ops practice, not
an egress right)

**Member seamrip**:
An export any member can take of what they can already see — enough to
seed a fork, never containing other people's secrets (emails, hidden
memberships). People join the fork by choice and re-set their own
visibility there.
_Avoid_: public export (it includes member-visible data, not just
public), scrape

**Personal export**:
Everything about *you* — profile, your memberships including hidden
flags, your proposals, votes, and comments. A member right, no admin
involved.
_Avoid_: my data download, GDPR export (the right exists regardless of
jurisdiction)

**Seamripped from**:
A quilt's own line on its Label naming the quilt it forked out of, with a
link. Prefilled on import and removable without ceremony — lineage is
worth recording, but a community that left its stewards is never made to
keep a link to them. The inverse of a moved-to pointer: that one is the
old home pointing forward, this one is the new home looking back, and
neither forwards anything automatically.
_Avoid_: parent quilt (there is no hierarchy between instances), upstream
(git register), origin

**Moved-to pointer**:
A profile's or patch's own signpost to its new home on another quilt.
Local first; federated Move emission is future work.
_Avoid_: redirect (nothing is forwarded automatically), migration (the
pointer points; people choose)

## Place

**Address**:
A patch's free-text description of where it is, in its own words —
"Lancaster, PA", "above the record shop on Prince St", or nothing at all.
Prose meant for people to read, never parsed and never geocoded. Naming a
place here does not put the patch on the map: an address and a map position
are separate acts, so a patch can say where it is without being findable
there. Backend column: `nodes.address`. The word `location` is reserved for
an event's venue text (`events.location`) and never names this field.
_Avoid_: location (it means the event field), place, venue (events have
those), where

**Location**:
An event's venue in prose, written name-first — "The Selvage", or "Lanc
Workshop & Tool Library, 433 Ice Avenue, Lancaster, PA". One free-text
field, never parsed into parts and never geocoded; a map position is a
separate act (`events.latitude`, `events.longitude`). Name-first is a
contract, not a coincidence: every importer that assembles a location puts
the venue ahead of the street, so a surface short on room truncates from
the tail and still names the place (docs/adr/046). The patch's own prose
field is **Address** above — the two never name each other. Backend
column: `events.location`.
_Avoid_: address (that is the patch's field), venue and address as two
fields (it is one), place, where

**Map location**:
A patch's placed marker on the map — a numeric latitude/longitude pair a
patch admin sets by dragging a marker on the Leaflet map the app already
ships, never geocoded from any text. Deliberately plain, no textile
coinage: it is a coordinate, not a woven thing. Independent of the address
above it — an address is prose, a map location is a placed point, and
naming one never sets the other. Unset position means the patch is simply
off the map; there is no separate on/off flag. Placement is manual and
explicit (open the picker, drop or drag the marker, save), so its
coarseness is the admin's to choose — a marker can sit at neighbourhood
level on purpose. Backend columns: `nodes.latitude`, `nodes.longitude`.
_Avoid_: pin (retired — docs/adr/027), geocode, coordinates (as the UI
label), address (that is the prose field, not the marker)

## Tile appearance

**Tile**:
A patch as rendered in the quilt: its palette, block, and rotation.
_Avoid_: square, cell

**Name badge**:
The pill floating over a tile that names its patch: motif on the identity
color, the patch's name, and the viewer's role mark where one applies.
Badges reveal progressively as tiles earn on-screen room, and where badges
would crowd, the larger tile's wins — the rest wait for a closer zoom. A
badge's shape comes from its name alone, never from its tile's size or its
position on screen; at the viewport edge it clips rather than reshapes.
_Avoid_: label / quilt label (the Label is the stewardship disclosure),
pin (retired — docs/adr/027), card, marker

**Palette**:
A named, pre-cut bundle of fabrics a tile is drawn with (the classic
album-art sets). A palette is curated, never free-form; it is one kind of
bundle, not a separate color system.
_Avoid_: theme (overloaded: light/dark UI theme), color scheme

**Block**:
A geometric quilt pattern that a tile is drawn as. Either curated — a
named traditional pattern (Pinwheel, Ohio Star, Log Cabin…) — or drafted
by the patch's admins in the block drafter.
_Avoid_: pattern (overloaded), design

**Starter block**:
A traditional pattern offered as a draft to open and take apart rather
than a fixed choice to pick between — how drafting the quilt icon begins
(docs/adr/043). Choosing one replaces what is on the canvas; nothing is
saved until you save.
_Avoid_: template, preset, default block (that is the old fixed-choice thing)

**Drafting**:
Designing a block on the grid, the way quilters draft on graph paper:
choose a grid, sew seams between anchors, color the resulting pieces with
fabrics from the bundle. The tool is the block drafter; its output is a
drafted block.
_Avoid_: custom block editor, designer, builder

**Seam**:
A straight line sewn between two anchors. Seams split every piece they
cross. A design has a seam budget — seams are counted, not unlimited.
_Avoid_: line, stroke, edge

**Anchor**:
A point on a cell wall where a seam can start or end: corners plus fixed
subdivision points. Anchor density is a function of grid size — finer
grids offer fewer subdivision points per cell. Seams connect anchors only;
there is no free placement.
_Avoid_: vertex, snap point, handle

**Piece**:
A colorable region of a block: a grid cell, or the part of one left after
seams cut through it. Every piece is colored with one fabric from the
bundle. Pieces are always local to one cell — a seam crossing many cells
makes pieces in each, so edits never scramble colors elsewhere.
_Avoid_: face, region, shape, segment

**Fabric**:
A single color as design material. Pieces are colored by fabric slot,
never by raw color value, so a design stays recolorable by swapping
fabrics.
_Avoid_: color (as the stored concept), hex, swatch (that is a fabric as
displayed on the wall)

**Fabric wall**:
The curated set of swatches every design draws from — one wall for the
whole quilt, tuned so any combination coexists. Users pick from the wall;
there is no free color picker.
_Avoid_: color picker, color library, swatch book (that is a bundle's
register)

**Bundle**:
The handful of fabrics a design draws with, chosen from the fabric wall
into a fixed number of slots. Slot one is the patch's identity color. The
classic palettes are pre-cut bundles. Every block is drawn from a bundle,
curated and drafted alike — there is one color system, not a drafter-only
mode.
_Avoid_: color scheme, custom palette (a palette is pre-cut by definition)

**Motif**:
The small mark shown beside a patch's name (quilt name badges, patch
cards). Chosen from a curated set; unset means it is derived from the
patch's tags — each tag in the vocabulary may carry a motif, and the
patch's first motif-bearing tag wins — falling back to the quilt mark.
Motifs are marks, never uploaded images. Backend key: `icon`.
_Avoid_: icon (overloaded — the UI is full of icons), logo, avatar,
emblem, label (that is the quilt's stewardship disclosure)

**Appearance**:
A patch's chosen palette + block + rotation + motif, treated as one
concept. Unset appearance means the tile is hash-assigned from the patch
ID and the motif is tag-derived — stable but not chosen. User-facing
name: "Patch Appearance".
_Avoid_: theme, style, customization (as a noun), tile settings

**Identity color**:
The single color that represents a patch anywhere it isn't drawn as a full
tile (card banners, quilt name badges). Always the patch's palette primary.
Distinct from tag colors, which color tags, not patches.
_Avoid_: brand color, accent
