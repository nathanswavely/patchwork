/**
 * The governance-mechanics decisions of 2026-10-20 (F-034, F-042, F-057,
 * F-066, F-078), held where they reach the page.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text. Each was also read in the browser.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(__dirname, '..', relPath), 'utf8');
}

describe('F-042: a member can stand for a vacant seat', () => {
  const form = source('pages/ProposalForm.svelte');

  it('offers standing to a plain member of an elected patch with a chair to stand for', () => {
    expect(form).toMatch(/let canStand = \$derived\(membershipRole === 'member' && isElected && standableSeats > 0\)/);
    expect(form).toMatch(/s\.vacant && s\.fill === 'nomination'/);
    expect(form).toMatch(/value: 'stand', label: 'Stand for a seat'/);
  });

  it('sends a membership proposal about its own author', () => {
    expect(form).toMatch(/if \(proposalType === 'stand'\) \{\s*payload\.proposal_type = 'membership';\s*payload\.target_user_id = getUser\(\)\?\.id;/);
  });

  it('opens on the type the council page asked for', () => {
    expect(form).toMatch(/askedType === 'membership' \|\| askedType === 'stand'/);
  });
});

describe('F-057: a member can file the minutes', () => {
  const list = source('pages/GovernanceList.svelte');
  const edit = source('pages/GovernanceEdit.svelte');

  it('opens the door to a member of a patch that decides anything elsewhere', () => {
    expect(list).toMatch(/meetsElsewhere = decidesElsewhere \|\| rules\?\.leadership_venue === 'elsewhere'/);
    expect(list).toMatch(/\{#if isLoggedIn\(\) && \(isAdmin \|\| memberFiles\)\}/);
  });

  it('does not offer a member the publish choice the server refuses', () => {
    expect(edit).toMatch(/\{#if canPublish\}/);
    expect(edit).toMatch(/An admin can publish it later\./);
  });
});

describe('F-066: the council changing size is on the Record', () => {
  const record = source('pages/GovernanceRecord.svelte');

  it('says what the council went to and who changed it', () => {
    expect(record).toMatch(/e\.outcome === 'seat_added' \|\| e\.outcome === 'seat_removed'/);
    expect(record).toMatch(/The council went to \$\{e\.seats\} seat/);
  });
});

describe('F-078: who stood down, and a record entered in error', () => {
  const records = source('components/AttestationRecords.svelte');
  const record = source('pages/GovernanceRecord.svelte');

  it('has a field for who the decision removed, and sends it', () => {
    expect(records).toMatch(/Who stood down/);
    expect(records).toMatch(/stood_down: stoodDown/);
  });

  it('strikes rather than deletes, behind the step-up gate', () => {
    expect(records).toMatch(/withStepUp\(\(\) => api\(`nodes\/\$\{slug\}\/attestations\/\$\{id\}\/strike`, \{ method: 'POST' \}\)\)/);
    expect(records).not.toMatch(/method: 'DELETE'/);
  });

  it('crosses a struck record out on the Record instead of hiding it', () => {
    expect(record).toMatch(/class:struck=\{e\.entered_in_error\}/);
    expect(record).toMatch(/text-decoration: line-through/);
    expect(record).toMatch(/\$\{listOf\(e\.stood_down\)\} stood down\./);
  });
});

describe('F-034: a patch that meets elsewhere says where', () => {
  const editor = source('components/StructuredRulesEditor.svelte');
  const overview = source('components/GovernanceOverview.svelte');

  it('edits the note only beside a venue that is elsewhere, and clears it otherwise', () => {
    expect(editor).toMatch(/\{#if meetsElsewhere\}/);
    expect(editor).toMatch(/venue_note: meetsElsewhere \? venueNote\.trim\(\) : ''/);
    expect(editor).toMatch(/maxlength="200"/);
  });

  it('names the place in place of the bare "outside Patchwork"', () => {
    expect(overview).toMatch(/Proposals are decided where this patch meets: \{venuePlace\}/);
    expect(overview).toMatch(/Admins are chosen where this patch meets: \{venuePlace\}/);
  });
});
