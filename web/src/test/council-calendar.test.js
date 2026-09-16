/**
 * One story about a seat (F-067, F-068), and the drawer marked Rejected
 * (docs/adr/097, amended).
 *
 * A member could see two vacant chairs and, beside them, three general
 * sentences — what an elected patch is, how a vacancy is filled, when the
 * next seat comes up. All true, all about different chairs at different
 * times. The council page now says one thing per chair, tells the reader
 * what they can do today (including "nothing until <date>"), and gives an
 * admin the box that sets a seat's term end — next to the chairs, not on
 * the proposal form.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('GovernanceOverview — one sentence per chair', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('reads the route the server picked for each seat rather than guessing', () => {
    expect(src).toMatch(/function seatLine\(seat\)/);
    expect(src).toMatch(/seat\.fill === 'contest_open'/);
    expect(src).toMatch(/seat\.fill === 'nomination'/);
    expect(src).toMatch(/seat\.fill === 'contest_scheduled'/);
  });

  it('says of a vacant chair how it is filled and that it can be filled today', () => {
    expect(src).toMatch(
      /Filled by nomination: an admin puts a member forward and the members ratify it\. That can happen today\./
    );
  });

  it('says of a held chair when its term ends and when it is contested', () => {
    expect(src).toMatch(/Term ends \$\{formatDay\(seat\.term_ends_at\)\}\. Contested at the election that opens \$\{formatDay\(seat\.contest_opens\)\}\./);
  });

  // Both found by reading the rendered page rather than the suite: a vacant
  // row said how it gets filled and never showed the date an admin had just
  // set on it, and a follower was told to ask for a nomination they are not
  // eligible for.
  it('shows a vacant chair its own term, since the appointee serves it out', () => {
    expect(src).toMatch(/Whoever takes it serves out the term, to \$\{formatDay\(seat\.term_ends_at\)\}\./);
  });

  it('does not send a follower after a nomination they cannot receive', () => {
    expect(src).toContain('A vacant seat is filled by nomination, and only this patch');
    expect(src).toContain('members can be nominated for one.');
  });

  it('says holdover out loud on a chair whose term has already run out', () => {
    expect(src).toMatch(/seat\.contest_due/);
    expect(src).toMatch(/the holder serves until a successor is elected/);
  });

  it('renders the per-seat line against every chair, held or vacant', () => {
    expect(src).toMatch(/\{#each seats as seat\}/);
    expect(src).toMatch(/\{seat\.vacant \? 'Vacant' : \(seat\.display_name \|\| seat\.username\)\}/);
    expect(src).toMatch(/<span class="seat-fate muted">\{seatLine\(seat\)\}<\/span>/);
  });

  it('stops reciting the general elected description above the chairs', () => {
    // The sentence stays in the models map — that map is the enumeration
    // governance-page.test.js pins the explaining page to — and is withheld
    // where the council block is about to say it with dates.
    expect(src).toMatch(/\{#if !showsCouncil && describeLeadership\(rules\)\}/);
    // And the one council-wide date line is gone from the page that now
    // carries a date per chair.
    expect(src).toMatch(/\{#if nextTermEnd && !showsCouncil\}/);
  });

  it('keeps the term length as one fact about the council, not a fourth sentence', () => {
    expect(src).toMatch(/Each term runs \$\{rules\.admin_term_months\} months\./);
  });
});

describe('GovernanceOverview — what you can do about it today', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('answers the reader in the second person, by role', () => {
    expect(src).toMatch(/let councilAction = \$derived\.by\(\(\) => \{/);
    expect(src).toMatch(/You can nominate a member for a vacant seat today\./);
    expect(src).toMatch(/Only an admin can put a name forward for a vacant seat\. Ask one to nominate you\./);
  });

  it('says "nothing until <date>" when that is the honest answer', () => {
    expect(src).toMatch(
      /There is nothing to do until \$\{formatDay\(nextContestOpens\)\}, when the next contest opens and any member may stand\./
    );
  });

  it('puts that answer next to the chairs', () => {
    expect(src).toMatch(/\{#if councilAction\}\s*\n\s*<p class="council-do">\{councilAction\}<\/p>/);
  });
});

describe('GovernanceOverview — the term-end box, next to the chairs', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('offers a date control per seat to this patch’s admin only', () => {
    expect(src).toMatch(/\{#if isPatchAdmin\}\s*\n\s*<div class="seat-controls">/);
    expect(src).toMatch(/<input type="date" bind:value=\{termDraft\}/);
    expect(src).toMatch(/\{seat\.term_ends_at \? 'Change term end' : 'Set term end'\}/);
  });

  it('PATCHes the seat route', () => {
    expect(src).toMatch(
      /api\(`nodes\/\$\{slug\}\/seats\/\$\{seatId\}`, \{ method: 'PATCH', body: \{ term_ends_at: termDraft \} \}\)/
    );
  });

  it('states the rule where the admin is standing: forward yes, back no', () => {
    expect(src).toMatch(/A vacant seat takes any future date; a held one can only be brought/);
    expect(src).toMatch(/pushing it back would extend a term nobody voted for/);
    expect(src).toMatch(/the\s+holder serves until a successor is elected/);
  });

  it('still refuses to remove a chair anybody is sitting in', () => {
    expect(src).toMatch(/\{#if seat\.vacant\}\s*\n\s*<button class="btn btn-sm" disabled=\{seatBusy\} onclick=\{\(\) => removeSeat\(seat\.id\)\}>/);
  });
});

describe('ProposalList — the filter label promises what the drawer holds', () => {
  const src = source('pages/ProposalList.svelte');

  it('carries a Not decided chip beside Rejected', () => {
    expect(src).toMatch(/\['rejected', 'Rejected'\], \['not_decided', 'Not decided'\], \['all', 'All'\]/);
  });

  it('sends the outcome to the server rather than filtering in the page', () => {
    expect(src).toMatch(/\?status=\$\{encodeURIComponent\(statusFilter\)\}/);
  });

  it('keeps the row wording it already had for a lapse and an unsettled contest', () => {
    expect(src).toMatch(/if \(p\.state === 'lapsed'\) return 'lapsed'/);
    expect(src).toMatch(/if \(p\.state === 'unsettled'\) return 'unsettled'/);
  });

  it('does not print the raw filter value at an empty drawer', () => {
    expect(src).not.toMatch(/with status "\$\{statusFilter\}"/);
    expect(src).toMatch(/Nothing here has lapsed or settled nothing\./);
  });
});

describe('PatchSettingsMembers — the dropdown points at both ways to make room', () => {
  const src = source('pages/PatchSettingsMembers.svelte');

  it('reads the seat payload’s own vacancy flag', () => {
    expect(src).toMatch(/councilSeats\.filter\(\(seat\) => seat\.vacant\)\.length/);
  });

  it('names the term end as the other way to open a contest sooner', () => {
    expect(src).toMatch(/or bring a seat's term end forward there/);
  });
});
