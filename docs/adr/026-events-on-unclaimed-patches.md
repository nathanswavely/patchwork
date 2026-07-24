# Events on unclaimed patches: submissions plus a trusted-contributor grant

Unclaimed patches had no working door for events: `CreateEvent` allowed
unclaimed nodes but required membership, and unclaimed patches are
follow-only — so in practice only the instance admin could keep a venue's
calendar alive before its owner arrived. That made the quilt look dead
exactly where it most needed to look alive (a touring band should be able
to get their show onto Spark Hall's listing).

We decided on a ladder modeled on Wikipedia's autopatrolled pattern:

- **Anyone logged in may submit an event** to a patch they don't run. It
  enters a review queue — no auto-approve for events, ever; instance-wide
  auto-approval would just be the spam hole this design exists to avoid.
- **Review is owed to whoever owns the calendar.** Submissions to
  unclaimed patches are reviewed by the instance admin, who holds those
  calendars in trust; submissions to active patches are reviewed by that
  patch's admins, never the instance admin.
- **Trusted contributor** is an instance-level grant, given and revoked
  explicitly by the instance admin (approval counts are a signal, never a
  trigger). It waives review **only on unclaimed patches** — it is the
  instance admin delegating their own queue, and must not reach into
  active patches, where it would let instance authority override patch
  autonomy. Backend: a flag on users, orthogonal to every patch role.
- **Edits follow the same door as creation** (trusted → direct, others →
  re-review); deleting your own event is always free. Every event on an
  unclaimed patch wears a **community-submitted** label derived from the
  patch's status, not stored per event.
- **On claim, the calendar transfers whole:** events survive unlabeled
  (adoption), pending submissions move to the new admins' queue, and
  trusted contributors become ordinary suggesters there.
- One config switch: `submissions.enabled` gates patch and event
  submissions together — they are the same community posture.
- The grant does not travel in a seamrip, and pending submissions are not
  exported: trust is granted per-instance, and a fork's steward re-grants
  it. (This also keeps older exports importable — the users table shape
  is unchanged.)

## Rejected alternatives

- **Anonymous submissions.** The ladder needs identity to accumulate
  trust, and the patch-submission path already requires auth; auth here
  is one magic link.
- **Federated / cross-quilt submissions.** Accounts are per-instance and
  mutations are never cross-origin. ADR 024's infrastructure (instance
  actor, signed inbox, neighbor quilts) makes a future **neighbor-vouched
  submission** feasible — the home instance's actor forwards a member's
  submission, so we trust the vouching instance, never an enumerable
  remote person — but that is future work with its own vouching and
  revocation semantics. For now a traveler creates a local account and
  lives on the submission rung permanently, which is fine: one event, one
  review.
- **Automatic promotion** (N approvals → trusted). Nobody climbs the
  contributor ladder by point accumulation anywhere else in Patchwork.
