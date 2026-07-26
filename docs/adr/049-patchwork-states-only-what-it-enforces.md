# Patchwork states only what it enforces

A patch running the Formal template shows its members a governance
overview reading "Leadership: Elected Council — The community elects
admins for fixed terms. Regular elections ensure power rotates. Terms
last 12 months," and under the admin list, "3 of 7 seats filled."

None of that is true. There is no election, no term, and no seat. Six
fields in `GovernanceConfig` are stored, defaulted, serialized to the
API, rendered as prose, and — for one of them — voted on, while no code
anywhere reads them to decide anything:

| Field | Rendered as | Enforced |
|---|---|---|
| `leadership_model` | "Leadership: Elected Council" + a paragraph | no |
| `max_admins` | "3 of 7 seats filled" | no |
| `admin_term_months` | "Terms last 12 months", "before re-election" | no |
| `succession_method` | template preview, succession plan | no |
| `succession_policy` | editable in the rules editor, votable | no |
| `inactivity_days` | "may be asked to step down" | no |

Every occurrence of each is a struct field, a default, or a copy from one
struct to another (`internal/governance/rules.go`,
`internal/model/model.go`, `internal/handler/governance.go`). This is the
failure docs/adr/041 named — the rules on screen were not the rules in
force — found again by sweeping the shipped charter text after
docs/adr/048 fixed one sentence of it.

We decided **Patchwork states as fact only what Patchwork enforces. A
charter may promise anything; the platform's own surfaces may not promise
on its behalf.**

The distinction that makes this tractable is *who is speaking*. A charter
is a community writing down how it intends to behave, and a community can
absolutely agree to hold elections on a calendar Patchwork knows nothing
about — that promise is between people and needs no machinery. The
governance overview, the template preview drawer, and the structured
rules editor are not the community speaking. They are Patchwork
describing itself, and there everything asserted has to be real.

- **`max_admins` is enforced, because the fix is smaller than the
  explanation.** Promotion to admin has no seat check
  (`internal/handler/memberships.go`), so a patch with `max_admins: 7`
  can render "9 of 7 seats filled" — a fraction whose denominator is
  decorative. Of the six this is the only one where the honest reading
  and the cheap reading agree: every member who sees a seat count already
  believes it is a cap, and making it one is a guard on a single write
  path. Existing patches over their cap are left alone; the check blocks
  new promotions past the limit rather than demoting anyone, because
  removing an admin is never a side effect of a settings value.

- **The other five stop being asserted, and are not built.** Elections,
  terms, and succession are real things communities want, and building
  them is a large feature with its own design questions (who runs a
  ballot, what a term expiring *does* to a sitting admin, whether a
  patch can be left leaderless). Shipping the smallest version of that as
  a byproduct of a copy fix would repeat the mistake docs/adr/048 was
  written about. They stay in the charter documents, which is where a
  community's own promises belong, and the platform's surfaces stop
  presenting them as platform behavior.

- **`succession_policy` has nothing to do, and that is structural.** A
  patch cannot reach zero admins: leaving, banning, and demoting the last
  admin are each refused
  (`internal/handler/memberships.go` — "cannot leave as the only admin",
  "cannot ban the last admin", "cannot demote the last admin"). Succession
  answers the question those three guards prevent anyone from asking. So
  it is not merely unimplemented; there is no moment at which it could
  run. Implementing it would mean first deciding that a patch may be left
  leaderless — a real decision, adjacent to docs/adr/012's "leaving is a
  member right," which today stops at the door of the last admin. That
  decision is not made here.

- **The votable one is the worst one.** `succession_policy` is editable
  in `StructuredRulesEditor` and travels through the amendment path, so a
  patch can hold a real vote — real quorum, real threshold, real
  fourteen-day window under docs/adr/047's frozen terms — to change a
  field that no code reads. Ceremony spent on nothing is worse than a
  wrong label, because the community pays for it in attention and comes
  away believing something changed. Fields the platform does not enforce
  do not belong in the editor that proposes changes to platform behavior.

- **`inactivity_days` is already honest and stays.** "Admins inactive for
  30 days may be asked to step down" describes people asking people. It
  hedges because it is a human process, and it reads correctly whether or
  not any code is involved. It is the model for how the other five should
  read if they are kept on any platform surface at all.

**Considered and rejected: deleting the six fields.** They carry meaning
for the community even with no enforcement — a patch that chose the
Formal template did choose elected leadership, and that choice should
survive a seamrip and stay legible in the rules diff. Deleting them would
also break the round-trip `StructuredRulesEditor` deliberately preserves.
The problem was never that the values exist; it was that surfaces read
them as behavior.

**Considered and rejected: marking them "not yet enforced" in place.** A
badge next to "Terms last 12 months" is honest and still wrong, because
it leaves Patchwork narrating a feature it does not have and invites the
reader to wait for it. If the community runs its own elections, the
sentence belongs to the community's charter in the community's voice, not
to the platform's overview with an asterisk.

**Consequences.** The governance overview's Leadership section loses its
narrative paragraph and its seat count becomes real. The template preview
drawer stops explaining terms and seats as things the platform will do,
which changes what someone is agreeing to when they pick Formal at
creation — correctly, since they were previously agreeing to a promise
nobody could keep. `succession_policy` leaves the structured rules
editor. The Formal succession plan document keeps its full election
timeline, in the community's voice, where it was always legitimate.

**Status: adopted as a design boundary — the `max_admins` guard is the
only code change decided here.** The surface pass that follows from it is
scoped but not built.
