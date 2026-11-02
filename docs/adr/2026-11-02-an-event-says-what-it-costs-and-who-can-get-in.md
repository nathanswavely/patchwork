# An event says what it costs and who can get in

Date: 2026-11-02. Status: **proposed**, not built.
Applies docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md (nothing is
answered by silence; content written elsewhere carries only what its source
said), docs/adr/079 (a parser fills what the feed is authoritative about),
the default-tier rule of
docs/adr/2026-09-19-an-event-says-who-it-is-for-within-what-the-patch-allows.md
(a source default applies at insert only), and the reasoning of
docs/adr/2026-10-25-nonprofit-is-a-fact-not-a-tag.md (a fact is a column, not
a tag).

## Context

Before going to an event, people ask the same few questions. Can I afford
it? Can I get in at my age, or with my kids? Is it outside? Can I get through
the door in a wheelchair? Patchwork answers none of them. An event has a
title, a description, a place, a time, a visibility tier and its own page out
on the web (`event_url`, docs/adr/079). Anything else is in the description,
if it is anywhere, and a description can't be filtered.

Lancaster Live (lanclive.com), a county events calendar, shows what the
answers are worth. Its main filters are free admission, kids and family,
confirmed indoors or outdoors, and doors before 8. It fills them by scanning
descriptions with regular expressions, and its own code lists what that gets
wrong: "a contribution is not unconditionally free", "never infer price
from ... a numeric zero supplied by a feed plugin", "missing outdoor data is
unknown, not indoor proof". That is the inference problem in its own words.
A guessed "free" or "family-friendly" is worse than no answer, because a
person plans around it.

For a DIY and all-ages scene the age question is not a convenience. An
all-ages show and a 21+ bar gig with the same band are different events for
a sixteen-year-old. Accessibility is the same: "no step-free entrance" is
the answer a wheelchair user most needs, and it is the one a description
most often leaves out.

## Decision

**1. Four facts, each one stated or unknown.** Every column is nullable,
and NULL means not stated. No form preselects an answer, and nothing derives
one from the title or description.

| Fact | Column | Values |
|---|---|---|
| Admission | `events.admission` | `free`, `donation`, `notaflof`, `paid` |
| Minimum age | `events.min_age` | an integer; `0` means all ages |
| Setting | `events.setting` | `indoor`, `outdoor`, `both` |
| Access | `events.step_free`, `events.access_offers`, `events.access_note` | see 2 |

- **`donation` is not `free`.** Pay what you can, suggested donation, sliding
  scale and free-will offering are all `donation`. A filter for free events
  matches `free` only.
- **`notaflof` is a price with a promise.** "No one turned away for lack of
  funds" is the DIY scene's door policy: there is a set price, and nobody who
  can't pay it is refused. It differs from `donation`, which has no set
  price, and from `paid`, which makes no promise. It gets its own value
  because a promise is the thing a person with no money needs to know, and
  it is exactly what an inferred "paid" would erase.
- **"Can I get in with no money?" is one filter.** It matches `free`,
  `donation` and `notaflof`, since each says a person without money gets
  through the door. `paid` and unknown never match.
- **No price amount.** Ticket tiers, door versus advance, and currency on a
  white-label quilt are more than one column can hold honestly, and the
  amount changes more often than anything else about an event. The amount
  belongs in the description and on `event_url`.
- **Age is a number, not a label.** The legal ages for bars and venues differ
  by country, so the quilt stores the minimum and the form offers presets.
  "18+" and "21+" are the shipped presets; see "Not decided here".
- **"For kids" is not here.** All ages says a child may come. Whether an
  event is aimed at children describes what it is, which is the
  description's job and, for a patch, a tag's. Events have no tags, and this
  ADR does not give them any.

**2. Access is one tri-state and a short list.**

- `step_free`: `1`, `0` or NULL. This one alone takes a no, because "there
  are stairs and no way around them" is the answer a wheelchair user plans
  around, and silence can't say it.
- `access_offers`: a JSON array drawn from a closed set: `asl` (sign language
  interpretation), `captions`, `seating` (seating available throughout),
  `sensory_friendly` (reduced sound and light, or a quiet room). An offer is
  listed when it is made. Absence means not stated, never "not offered", so
  these need no tri-state.
- `access_note`: free text, at most 280 characters, for what the list can't
  say ("elevator entrance on Water Street", "the stage is up one step").

The set is closed so it can be filtered and translated. Adding an offer is a
change to this ADR, the way adding a report reason is.

**3. A patch may keep usual answers for its events.** A venue posts dozens of
events a month, and its doorway does not change between them. Patch Settings
gains an optional **usual event details** section holding the same fields
(`nodes.event_defaults`, JSON). They are used in two places, and nowhere else:

- **The event form pre-fills from them,** visibly and editably. A pre-fill
  the poster can see and change is the patch's stated answer, not an answer
  by silence.
- **An event source applies them at insert** to any fact its feed does not
  state, exactly as a source's default tier is applied (docs/adr/2026-09-19).
  The sync never writes them again, so a per-event correction sticks.

A band whose events move from venue to venue leaves the section empty. An
unclaimed patch has no usual details, so its imported events carry only what
their feeds state.

**4. A feed fills a fact only when it states one.** ICS and the Squarespace
collection carry none of these, so they fill nothing. schema.org JSON-LD
does, and the parser maps only explicit statements:

- `isAccessibleForFree: true` becomes `admission = free`. An `offers.price`
  of 0 does not, because feed plugins emit zero for "unknown".
- `typicalAgeRange` in the forms `"N-"` or `"N+"` becomes `min_age = N`.
  Anything else is left unknown.
- A feed statement wins over the patch's usual details, and the sync updates
  it when the feed changes, as with every field the feed owns
  (docs/adr/079). When a feed stops stating a fact, the stored answer stays.

**5. Every door that writes an event asks.** The event form on create and
edit, the non-member submission form (the submitter answers for what they
know, and may leave every fact unknown), and the CSV upload, which gains
optional columns. Every payload a form spreads back into an update carries
all six columns, or an unrelated edit resets them.

**6. What a reader sees.** A short line on the event page and on event cards:
admission, age, setting, and step-free when it is stated either way. Offers
and the note appear on the event page. An unknown fact shows nothing. The
exact wording is the maintainer's, as in docs/adr/2026-10-20.

**7. Filters match stated facts only.** The events list, calendar and map
gain narrowing parameters (`admission`, `no_one_turned_away`, `all_ages`,
`setting`, `step_free`)
beside the scope switcher (docs/adr/035). An event with an unknown answer
never matches. A patch that states its answers is found by the people who
need them, and one that doesn't is no worse off than today. The quilt is
not filtered: these are facts about events, and the quilt draws patches.

**8. They travel with the row.** The columns are in the personal export and
both seamrips, since columns travel with their tables, and archives written
before them import as NULL. Federation and Patchwork's own ICS and RSS feeds
are not changed here.

## Considered options

- **Infer the facts from the description**, as Lancaster Live does: rejected
  for the reasons in its own comments. An inference shown as a fact is a
  claim nobody made.
- **Booleans defaulting to false** (`is_free`, `is_outdoor`): rejected. Every
  untouched form would claim the event costs money and is indoors.
- **Tags on events:** rejected. docs/adr/021 makes tags a patch's
  classification and has them place patches on the quilt, and
  docs/adr/2026-10-25 already turned down a tag for a fact.
- **A price amount with currency:** deferred, for the reasons in 1.
- **Accessibility on the patch only:** rejected. The patch is not always the
  venue (a band's events move), and some offers, like interpretation, are
  made per event. The usual details in 3 carry what a venue does every
  night.
- **A tri-state for every access offer:** rejected. "No captions" is rarely
  what anyone plans around, and five yes/no/unknown questions on every event
  would get clicked through without reading.

## Consequences

- One migration adds six columns to `events` and one to `nodes`. Existing
  events are unknown on every fact, and nobody is labelled after the fact.
- The event form grows by one collapsed section. The questions cost a venue
  nothing after it fills in its usual details once.
- Filtered views reward stating facts. A feed that states nothing keeps its
  events out of every filtered view, and that is accurate.
- `access_note` and the offers are text a person wrote, so they fall under
  the existing report reasons like any description.

## Not decided here

- **Whether the age presets are an instance setting.** A UK quilt wants 18+
  and no 21+. The column takes any integer, so this can change without a
  migration.
- **The visible copy** for every label, the badge line and the filter chips.
  NOTAFLOF in particular: the acronym is common in DIY spaces and unknown
  outside them, so the label probably spells it out.
- **Federation.** ActivityPub's Event carries none of these. Writing them into
  the content, as docs/adr/2026-10-20 does for the AI label, is a separate
  decision.
- **Whether a feed's `offers.price` may ever fill `paid`.** A positive price
  is a stronger statement than a zero. It is left out until somebody shows a
  feed that gets it right.
