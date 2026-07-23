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
relationship. Claiming turns it into that owner's patch.
_Avoid_: ghost patch, placeholder, stub

**Claim**:
An assertion of ownership over an unclaimed patch, pending proof — never a
reservation. One person's unproven claim doesn't stop anyone else from
proving theirs: claims on the same patch run concurrently, and the first
to verify (or be approved) wins; the rest are auto-rejected. A user holds
at most one open claim per patch.
_Avoid_: lock, hold, reservation

**Verification domain**:
The trust anchor for self-service claims on an unclaimed patch — the one
domain the platform has vetted as actually belonging to the organization.
Set only by instance admins and trusted contributors (auto-derived from
the website they supply, unless that domain is a shared platform like
Facebook or Gmail). Every self-verification method proves control of this
domain; without one, the only path to claiming is admin review. Distinct
from the website field, which is cosmetic and carries no trust.
_Avoid_: website (as a trust anchor), domain (unqualified)

**Event**:
Something a patch hosts at a time and, optionally, a place — a show, a
meeting, a workday. Deliberately the same word in the UI and the backend;
the textile coinage "pin" is retired (docs/adr/027). It collided on every
side: events literally appear as map pins on the Leaflet map, software
readers hear "pinned post", and the UI had already grown Events pages,
routes, and notification labels while "pin" survived mainly in docs. A
term nobody reaches for when building the thing isn't the thing's name.
_Avoid_: pin (retired), happening, gathering

**Member count**:
Admins plus members — never followers. A follower is an interested
observer, not a member; follower interest is its own count. The two are
never summed in anything user-facing.
_Avoid_: community size (ambiguous), total members (when it includes
followers)

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
why "repo" was rejected as a name).
_Avoid_: repo (collides with the git-backed governance repos), patch admin
area, manage area

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
The standing tag selection that narrows every discovery surface at once —
treemap, cards, map, and events. A filter is independent of the search
query and of scope: it stays on until the person clears it, so it must
never be silently active — wherever the full selection can't be shown,
a count stands in for it, and clearing is always one step. Toggled in the
filter card, the surface anchored beneath the discovery search. Filtering
is a discovery concept, like scope.
_Avoid_: facet, refinement, tag search, category filter

**Scoped finder**:
The global bar's contextual search inside a workspace or the admin panel.
It finds entities of that context only, never instance-wide. People appear
in finders only where a profile gives the result somewhere to land: a
workspace's members, the admin panel's users. There is no instance-wide
people search — people are discovered through patches.
_Avoid_: search index, command palette, global search

**Admin panel**:
The instance admin surface at /admin. Gets the same full-screen takeover
treatment as a workspace.
_Avoid_: admin area, dashboard (that is the user's personal page)

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
immediately on every surface the instance controls.
_Avoid_: account page (that is settings), dashboard (the personal page)

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

## Governance

**Charter**:
A governance document a patch keeps — community standards, bylaws,
whatever the patch writes down about how it runs itself. Versioned and
diffed; the database is canonical and a per-patch git repository mirrors
the history (docs/adr/011). Amended through proposals. "Governance doc"
remains the backend term in code and endpoints.
_Avoid_: lining (that is one specific charter, not the type), policy,
rules doc

**Lining**:
The shared baseline community-standards charter — the one every patch on
the quilt agrees to, carrying the anti-discrimination baseline. "The
lining" names this document specifically, the layer that sits behind
every patch; it is not the generic word for charters.
_Avoid_: using "lining" for governance documents in general

**Proposal**:
Something a patch votes on: discussion, then voting, then in effect.
Deliberately the same word in the UI and the backend; the textile coinage
"baste request" is retired. It explained a metaphor before it explained
the feature — every UI surface had already grown Proposals headings and
routes while "baste request" survived mainly in docs and shipped
document templates. A term nobody reaches for when building the thing
isn't the thing's name.
_Avoid_: baste request (retired), motion, petition

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
The single image that represents a whole quilt wherever quilts are listed
or switched between — the scope switcher, Connected Quilts. One uploaded
image (square, small, explicit format rules) or a chosen default block;
unset means hash-assigned from the quilt's name, stable but not chosen
(same rule as tile appearance). Instance identity: it never travels in a
seamrip — a fork re-brands.
_Avoid_: logo (that is branding.logo_url, a different slot), avatar (people
have those), favicon

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
discovery search opens its card.
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
