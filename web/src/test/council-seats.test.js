/**
 * A seat is a chair you can count (docs/adr/100).
 *
 * Two founders of elected patches made colleagues admins from the dropdown on
 * Settings → Members, on patches whose own governance page says "The
 * community elects admins for fixed terms." Either the election means
 * something or the dropdown does. These tests hold the three surfaces the
 * answer lives on: the dropdown withdraws Admin and says what fills a seat
 * instead, the governance hub counts the chairs and lets an admin add or
 * remove an empty one, and the proposal form refuses a membership proposal
 * that names nobody.
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

describe('PatchSettingsMembers — the dropdown does not make an admin', () => {
  const src = source('pages/PatchSettingsMembers.svelte');

  it('reads the leadership model and venue before deciding', () => {
    expect(src).toMatch(/leadership_model === 'elected'/);
    expect(src).toMatch(/leadership_venue !== 'elsewhere'/);
  });

  it('withdraws the Admin option on an elected patch, keeping it for a sitting admin', () => {
    expect(src).toMatch(/\{#if !electedCouncil \|\| member\.role === 'admin'\}\s*\n\s*<option value="admin">Admin<\/option>/);
  });

  it('says what fills a seat instead, with the vacancy or the date', () => {
    expect(src).toMatch(/This patch elects its council, so admins are not made here\./);
    expect(src).toMatch(/the members ratify it/);
    expect(src).toMatch(/the next contest opens \{formatDay\(nextContestOpens\)\}/);
  });
});

describe('GovernanceOverview — the council is countable', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('renders the seats only where the patch elects here', () => {
    expect(src).toMatch(/let isElectedModel = \$derived\(overview\?\.rules\?\.leadership_model === 'elected'\)/);
    expect(src).toMatch(/let showsCouncil = \$derived\(isElectedModel && !leadershipElsewhere\)/);
    expect(src).toMatch(/\{#if showsCouncil\}/);
  });

  it('lists every seat, held or vacant, with its own term end', () => {
    expect(src).toMatch(/\{#each seats as seat\}/);
    expect(src).toMatch(/\{seat\.holder_id \? \(seat\.display_name \|\| seat\.username\) : 'Vacant'\}/);
    expect(src).toMatch(/Term ends \{formatDay\(seat\.term_ends_at\)\}/);
  });

  it('offers the seat controls to an admin of this patch only, and removal only on an empty chair', () => {
    expect(src).toMatch(/let isPatchAdmin = \$derived\(membershipRole === 'admin'\)/);
    expect(src).toMatch(/\{#if isPatchAdmin && !seat\.holder_id\}/);
    expect(src).toMatch(/Remove seat/);
    expect(src).toMatch(/Add a seat/);
  });

  it('says adding a chair is not the same as filling it', () => {
    expect(src).toMatch(/Adding a seat does not make anybody an admin\. The community fills it\./);
  });

  it('tells a member looking for a way onto the council where it is', () => {
    expect(src).toMatch(/A vacant seat is filled by nomination: an admin puts a name forward and the members ratify it\./);
    expect(src).toMatch(/The next contest opens \$\{formatDay\(nextContestOpens\)\}, and any member may stand then\./);
  });

  it('posts and deletes against the seat routes', () => {
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/seats`, \{ method: 'POST' \}\)/);
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/seats\/\$\{seatId\}`, \{ method: 'DELETE' \}\)/);
  });
});

describe('ProposalForm — a membership proposal names somebody', () => {
  const src = source('pages/ProposalForm.svelte');

  it('offers the Membership type only to someone who can actually nominate', () => {
    expect(src).toMatch(/let canNominate = \$derived\(isPatchAdmin && \(isMeritocratic \|\| \(isElected && vacantSeats > 0\)\)\)/);
    expect(src).toMatch(/\.\.\.\(canNominate\s*\n\s*\? \[\{ value: 'membership'/);
  });

  it('has no "Nobody" option left on the nominee picker, and blocks an empty one', () => {
    expect(src).not.toMatch(/This is an ordinary membership proposal/);
    expect(src).toMatch(/<option value="">Choose a member<\/option>/);
    expect(src).toMatch(/if \(proposalType === 'membership' && !nomineeId\)/);
  });

  it('steers away from the type when an elected council has no vacancy', () => {
    expect(src).toMatch(/\{#if isElected && !canNominate\}/);
    expect(src).toMatch(/Every seat on the council is held, so there is nobody to nominate\./);
  });

  it('says the appointee serves the seat out rather than starting a fresh term', () => {
    expect(src).toMatch(/they take the vacant seat and serve out its term/);
  });
});
