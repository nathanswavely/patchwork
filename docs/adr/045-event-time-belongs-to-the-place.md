# Event time belongs to the place, not the reader

Patchwork has no timezone concept anywhere. Not in `patchwork.yaml.example`,
not as a column in any migration, not in the API, not in the frontend. A
repo-wide grep for `timezone|timeZone|time_zone` returns two comments and a
test name.

What exists instead is a convention nobody wrote down: `events.starts_at`
is `TEXT NOT NULL` (`migrations/001_initial.sql:92`) holding an ISO 8601
instant in UTC, and every reader renders it with the *viewer's browser*
zone — `new Date(iso).toLocaleTimeString('en-US', …)`, duplicated across
`EventsPage.svelte:217`, `EventDetail.svelte:75`, `PatchEvents.svelte:174`,
`Dashboard.svelte:76`, `PatchProfile.svelte:155`,
`AdminEventSubmissions.svelte:54`, and `finderProviders.js:58`.

For a video call that is the correct behaviour: the instant is the fact and
each participant's clock is a rendering of it. Patchwork does not host video
calls. It hosts a show at The Selvage that starts at 8pm — 8pm on the
flyer, 8pm at the door, 8pm for the band loading in. The wall clock at the
venue *is* the fact, and the UTC instant is the encoding. We have been
storing the encoding and throwing away the fact, then reconstructing it
against whatever zone the reader's laptop happens to be set to.

Stored as `2026-07-23T00:00:00Z`, a Lancaster 8pm show renders to a viewer
on UTC as midnight **on the following day**. Both the events list and the
patch profile lead with a date, so the visible symptom is not a wrong clock
time — it is the wrong day. The event is on Thursday's flyer and Friday's
website. A traveling organizer, a misconfigured clock, a CI screenshot, and
a European following a Lancaster patch all see a different calendar than
the community that scheduled it.

The absence has already produced four bugs that read as unrelated:

- **The edit form shifts every event it touches.** `EventForm.svelte:55`
  prefills a `datetime-local` input with `event.starts_at.slice(0, 16)` —
  the first sixteen characters of a **UTC** string — into a control that
  interprets its value as **local**. Line 123 then submits
  `new Date(startsAt).toISOString()`, converting local→UTC a second time.
  Opening an event and saving it moves the event by the editor's UTC
  offset. In Lancaster that is four or five hours per round trip, and it
  compounds with each edit.
- **`Today` and `Tomorrow` return nothing, always.** `getDateRange`
  (`EventsPage.svelte:27-64`) emits bare dates like `2026-07-26` for both
  `from` and `to`; the handler compares them against the column as text
  (`events.go:96-98`, `e.starts_at <= ?`). SQLite sorts
  `'2026-07-26T19:00:00.000Z'` *after* `'2026-07-26'`, so the last day of
  every range is dropped — and when `from == to`, that is the whole range.
- **Every preset is off by one day east of UTC.** The same function builds
  `today` as a local midnight `Date` and then formats it through
  `toISOString().slice(0, 10)` (`:29-30`), which reads that local midnight
  back in UTC.
- **Floating and all-day feed times land on the wrong day.**
  `parse.go:203` calls `e.DateTimeStart(time.UTC)`. go-ical honours an
  explicit `TZID` correctly, but the location argument is the fallback for
  values carrying no zone — so a floating `DTSTART:20260722T190000` and an
  all-day `VALUE=DATE` from a New York venue are both read as UTC. The
  all-day case becomes `2026-07-22T00:00:00Z` and renders in Lancaster as
  July 21st, 8pm. `jsonld.go:143` documents the same behaviour explicitly.
  Only the TZID path has a test (`parse_test.go:184`).

None of these are fixable in isolation, because each one is a component
guessing at a fact the system never recorded.

We decided:

- **An event carries the zone it happens in.** `events.timezone`, an IANA
  name (`America/New_York`), stored beside the instant rather than baked
  into it. The instant remains the sort key, the index, and the wire
  format; the zone is what lets any reader reconstruct the wall clock the
  organizer actually meant. This is the only shape that survives
  federation, and federation is the whole reason it cannot be an instance
  setting — see below.

- **The zone is inherited, never asked for.** Resolution runs
  event → patch (`nodes.timezone`) → instance → UTC, and the first
  non-null wins. An organizer posting a show at their own venue never sees
  a timezone control; the patch already knows where it is. The control
  appears only on an event whose zone differs from its patch's — a touring
  band's out-of-town date — and it is the only case that justifies asking.
  A field that must be filled for every event is a field that will be
  filled wrong.

- **The instance default lives beside the coordinates it agrees with.**
  `geographic.timezone` in `patchwork.yaml`, next to the `latitude` and
  `longitude` that already declare where this quilt is, with an
  `instance_settings` override editable from the admin panel — the same
  bootstrap-default-plus-override pattern `instance.name` and
  `instance.description` already use, where the override wins. A quilt
  that sets neither falls back to UTC and gets a dashboard warning, the
  posture SMTP already established: warn, don't refuse to start.

- **The API returns a resolved zone, never a null one.** Every event
  payload carries `timezone` alongside `starts_at`, already collapsed
  through the chain. Clients — including other quilts' SPAs reading us
  cross-origin — never reimplement the fallback, and never need to fetch
  the patch to render the event.

- **Formatting happens in exactly one module.** `web/src/lib/datetime.js`,
  taking `(iso, tz)` and going through `Intl.DateTimeFormat` with a
  `timeZone` option. The seven duplicated formatters collapse into it. The
  module shows a zone abbreviation **only when the event's zone differs
  from the viewer's** — `8:00 PM` at home, `8:00 PM EDT` when it doesn't
  match. Annotating every time teaches the reader to ignore the
  annotation; annotating the surprising ones is what makes the merged
  cross-quilt feed legible.

- **Day boundaries are computed in the event's zone and sent as
  instants.** `getDateRange` returns full RFC 3339 bounds rather than bare
  dates, which fixes the dropped last day and the east-of-UTC off-by-one
  together. "Today" is a question about a calendar, and a calendar belongs
  to a place.

- **Ingested times with no zone are read in the source patch's zone.**
  `parse.go:203` and `jsonld.go:143` take the resolved zone instead of
  `time.UTC`. A venue's own calendar feed publishing floating times is
  publishing them in its own zone; that is what floating means.

**Considered and rejected: a single instance-wide timezone.** This is the
tempting answer and it is nearly right — a community quilt is inherently
local, and the Lancaster reference instance serves one city. It fails on
the two surfaces this project has already committed to. ADR 024's **My
Quilt** merges patches followed on other quilts into one event feed; under
an instance-wide zone, a Portland show at 8pm PDT renders to a Lancaster
reader as 11pm, and a 10pm show renders on tomorrow's date — the original
bug, relocated to the surface built to blend other people's events. ADR 025
makes it worse by construction: a global Patchwork is an instance, and an
instance spanning every timezone cannot have one. The rejection is not
about correctness in the abstract; a single zone is a schema that cannot
represent the product as documented.

**Considered and rejected: storing wall-clock time plus zone, deriving the
instant.** Strictly more correct for the future. A weekly 8pm show should
stay 8pm across a DST transition, and if a legislature changes DST rules
after we stored the instant, the stored instant becomes wrong while the
wall clock stays right. It was rejected on blast radius: every server-side
comparison is a lexicographic string compare against a UTC RFC 3339 value
— `feeds.go:158`, `reminders.go:119`, `sync.go:340`, `sync.go:389`,
`parse.go:67`, the `starts_at` index, and the keyset cursor — and changing
the stored format invalidates all of them at once. Instant-plus-zone gets
the rendering fix with no query rewritten. The DST-rule-change window is
the accepted cost, and it is narrow: rule changes are announced years out,
and a recurring series can be re-derived from its zone when one lands.

**Considered and rejected: leaving viewer-local rendering and documenting
it.** Documenting `eventCsv.js:5`-style ("times are read in your timezone")
across every surface would make the behaviour honest without making it
correct. The reader is not the authority on when a physical gathering
starts. And a note cannot fix the four bugs above, because each of them is
a *guess* — the fix is to stop guessing, which requires storing the answer.

Consequences: the migration is additive and no event row needs a backfill.
Existing `starts_at` values were produced by browsers standing in Eastern
time and are therefore already correct **instants** — setting
`geographic.timezone: America/New_York` retroactively renders the entire
Lancaster corpus right, so the live instance is fixed by a config value
rather than a data rewrite. Quilts that set nothing keep exactly today's
behaviour under a UTC default, which makes the change safe to ship before
any operator acts on it.

ActivityPub has no standard property for an event's zone; `startTime` stays
a spec-correct UTC instant and remote events fall back to their actor's
zone where we can learn it and to display-without-annotation where we
cannot. The Patchwork-to-Patchwork JSON read path is ours and carries
`timezone` directly, so the cross-quilt case — the one that motivated this
— is served exactly.

Two adjacent problems are named here and deliberately left out of scope.
**All-day events are a distinct kind, not a time**; a `VALUE=DATE` feed
entry has no clock and no zone, and representing it as midnight-in-a-zone
is a second lie layered on the first. It wants an `all_day` flag and its
own ADR. And `starts_at` currently holds **two incompatible precisions** —
`.000Z` from the browser (`EventForm.svelte:123`, `eventCsv.js:173`) versus
zero-fraction RFC 3339 from feed ingest (`parse.go:221`) — which is
harmless for ordering but breaks the exact-match dedupe key in
`event_upload.go:131`, letting the same event upload twice. Both are real;
neither is a timezone decision.

Finally, the four bugs above are not follow-ups. They are the reason this
decision exists, and the edit-form shift in particular is corrupting data
on the live instance today, independent of any rendering choice — it is the
part of this work that should land first and can land alone.
