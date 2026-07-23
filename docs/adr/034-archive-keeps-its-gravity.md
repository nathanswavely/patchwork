# Archive keeps its gravity — restore is an instance-admin action

Archiving a patch (`DELETE /api/v1/nodes/{slug}`) was billed in the
danger zone as "cannot be undone," but it never deleted anything: it
flips `nodes.status` to `archived` and every slug-based route stops
resolving the patch. The only recovery was hand-run SQL. That gap needed
a real restore path, and the design question was who holds it.

We decided **restore belongs to the instance admin alone**, and that the
mechanics keep archive feeling like an ending, not a toggle:

- **No self-service undo.** A patch admin cannot restore their own
  archive. Every archived patch on a live instance was archived under
  "cannot be undone" expectations; letting the same role flip it back
  would retroactively weaken what those admins thought they did, and
  would turn archive into a visibility switch a single admin could
  flap. A human with site-wide responsibility in the loop is a feature:
  restore requests are rare, and the instance admin can ask who's
  asking and why. (A patch-admin grace window was considered and
  rejected as complexity without a demonstrated need.)
- **Restore returns the patch to what it was, via `archived_from`.**
  Archiving overwrites the status, and instance admins archive
  *unclaimed* patches too (junk listings, duplicates). Blind
  `status='active'` would resurrect an ex-unclaimed patch as claimed
  with zero admins — unreachable and unclaimable, since the claim flow
  gates on `status='unclaimed'`. A new `archived_from` column is set by
  both archive paths and read once at restore. Rows archived before the
  column exists fall back to inference: any active admin membership →
  `active`, else `unclaimed` (sound today because the last-admin guards
  keep active patches at ≥1 admin and an archived patch's memberships
  are frozen).
- **The surface is a dedicated admin-panel section, keyed by ID.** A
  small "Archived patches" list (name, archived date, prior status),
  `GET /api/v1/admin/nodes?status=archived` and
  `POST /api/v1/admin/nodes/{id}/restore`. ID, not slug — every
  slug-based route refuses archived patches, and that refusal stays
  absolute. The list filters `removed_at IS NULL`: rejected community
  submissions also carry `status='archived'` but with `removed_at` set,
  and they are refuse, not archives. If a general admin patches
  inventory is ever built, this list collapses into it as a filter.
- **Restore is silent.** An audit event (`node.restore`, beside the
  existing `node.delete`) and nothing else — no notifications, no AP
  activity. Archive itself is silent; the patch reappearing in the
  quilt, feeds, and its AP actor resolving again is the announcement.
  Deliveries to remote followers resume naturally; the actor's 404s
  while archived read as transient from outside.
- **Plain words: Archive / Restore.** Every textile coinage we tried
  ("fold away", "off the frame") was euphemism, not structure. The
  danger-zone copy drops the now-false "cannot be undone" for the
  true hint: only the instance admin can restore.

Consequences: an archived patch holds its slug forever (`nodes.slug` is
UNIQUE and `uniqueSlug` counts all rows), so a community that re-creates
an archived patch gets a suffixed slug, and a later restore yields two
same-named patches — visible to the instance admin at restore time,
their call to resolve. The event-source sync worker's missing
node-status filter (it syncs archived patches' feeds today) is a
pre-existing bug tracked separately; once fixed at the query level,
sources resume syncing on restore for free.
