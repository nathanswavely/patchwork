/**
 * Whether a row from the proposals listing is a direct change: approved with
 * no ballots, born applied under admin-decides rules (docs/adr/041), so it
 * says "applied" rather than "approved" and names who applied it.
 *
 * Never an election. A contest the calendar opened can settle with no
 * ballots too, and treating it as a direct change put the patch's founder
 * down as having "applied" a council election nobody applied (F-137).
 * The listing sends `election_phase` for that reason, the same field the
 * detail page reads (docs/adr/109).
 *
 * Shared by the proposals list and the patch profile's governance glimpse,
 * which read the same endpoint and must not name one row two ways.
 */
export function isDirectRow(p) {
  if (!p || p.election_phase || (p.seats_contested || 0) > 0) return false;
  return p.status === 'approved' && !((p.approve_count || 0) + (p.reject_count || 0) + (p.abstain_count || 0));
}
