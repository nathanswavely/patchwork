/**
 * Decisions made elsewhere, recorded here (docs/adr/052).
 *
 * The traps these guard are all cases where the page could quietly say
 * something the server would refuse, or claim something the community did not:
 *
 *   - Reading is public. An attestation's whole value is that the people who
 *     were in the room can check it, so the records must not sit behind a
 *     membership check the way a members-only charter does.
 *   - An unrealized name is not a person holding anything. If the page renders
 *     it like any other admin, the record has fabricated a relationship — the
 *     line CONTEXT.md draws for unclaimed patches, one level down.
 *   - Recording promotes and demotes admins, so it goes through step-up like
 *     every other power move, with the passkey notice shown *before* someone
 *     fills the form rather than after.
 *   - A patch that decides elsewhere must not be narrated as if it ran the
 *     mechanic its leadership_model names. That is docs/adr/049's failure
 *     reintroduced by the feature that was supposed to end it.
 *
 * There is no Svelte render library in this project (see router.test.js,
 * patch-setup.test.js), so component wiring is asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('AttestationRecords — the record', () => {
  const src = source('components/AttestationRecords.svelte');

  it('reads the records without gating on membership', () => {
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/attestations`\)/);
    // No role check stands between a visitor and the record.
    expect(src).not.toMatch(/isMember/);
  });

  it('shows the record in force as the one nothing has superseded', () => {
    expect(src).toMatch(/current = \$derived\(records\.find\(\(r\) => !r\.superseded_by\)/);
  });

  it('keeps corrected records readable rather than erasing them', () => {
    expect(src).toMatch(/earlier = \$derived\(records\.filter\(\(r\) => r\.superseded_by\)\)/);
  });

  it('marks a name nobody has claimed instead of rendering it as an admin', () => {
    expect(src).toMatch(/class:unrealized=\{!n\.realized\}/);
    expect(src).toMatch(/hasn't joined/);
  });

  it('says plainly that an unnamed person holds nothing until they join', () => {
    expect(src).toMatch(/holds nothing until they join/);
  });
});

describe('AttestationRecords — the admin actions', () => {
  const src = source('components/AttestationRecords.svelte');

  it('records through step-up, because it promotes and demotes admins', () => {
    expect(src).toMatch(/withStepUp\(\(\) =>\s*\n?\s*api\(`nodes\/\$\{slug\}\/attestations`, \{\s*\n?\s*method: 'POST'/);
  });

  it('links a name through step-up too', () => {
    expect(src).toMatch(/withStepUp\(\(\) =>\s*\n?\s*api\(`nodes\/\$\{slug\}\/attestation-names\/\$\{nameID\}`/);
  });

  it('warns about a missing passkey before the form is filled, not after', () => {
    expect(src).toMatch(/import PasskeyNotice/);
    expect(src).toMatch(/<PasskeyNotice show=\{!hasPasskey\}/);
  });

  it('corrects by superseding — never by editing the record in place', () => {
    expect(src).toMatch(/supersedes_id: correcting \|\| undefined/);
    // No update or delete path exists for a record.
    expect(src).not.toMatch(/method: 'PUT'/);
    expect(src).not.toMatch(/method: 'DELETE'/);
  });

  it('prefills a correction from the record it corrects', () => {
    expect(src).toMatch(/const base = correctID \? records\.find\(\(r\) => r\.id === correctID\) : null/);
  });

  it('offers only members as people a name can be linked to', () => {
    expect(src).toMatch(/x\.role === 'admin' \|\| x\.role === 'member'/);
  });
});

describe('GovernanceOverview — a patch that decides elsewhere', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('reads the venue from the rules rather than inferring it', () => {
    expect(src).toMatch(/leadershipElsewhere = \$derived\(overview\?\.rules\?\.leadership_venue === 'elsewhere'\)/);
  });

  it('stops narrating a leadership mechanic the patch does not run', () => {
    expect(src).toMatch(/if \(rules\.leadership_venue === 'elsewhere'\) return '';/);
  });

  it('does not offer a successor on a patch whose admins are chosen elsewhere', () => {
    expect(src).toMatch(/\{#if isMaintainerModel && !leadershipElsewhere\}/);
  });

  it('shows the records where leadership already renders', () => {
    expect(src).toContain('<AttestationRecords {slug} isAdmin={isAdmin}');
  });
});

describe('formatDay — a calendar date has no timezone to shift', () => {
  const src = source('lib/datetime.js');

  it('reads a bare YYYY-MM-DD as parts rather than as a UTC instant', () => {
    // `new Date('2026-03-14')` is UTC midnight, which renders as the 13th west
    // of Greenwich — an attestation would name the day before the meeting.
    expect(src).toMatch(/const dateOnly = \/\^\(\\d\{4\}\)-\(\\d\{2\}\)-\(\\d\{2\}\)\$\/\.exec\(iso\)/);
    expect(src).toMatch(/new Date\(Number\(y\), Number\(m\) - 1, Number\(d\)\)/);
  });
});

describe('AttestationRecords — terms', () => {
  const src = source('components/AttestationRecords.svelte');

  it('takes hasTerms as a prop rather than guessing the model', () => {
    expect(src).toMatch(/let \{ slug = '', isAdmin = false, hasTerms = false \} = \$props\(\)/);
  });

  it('only sends a term where the model has one', () => {
    expect(src).toMatch(/term_ends_at: hasTerms && termEndsAt \? termEndsAt : undefined/);
  });

  it('reads a lapsed term as a calendar date, not a UTC instant', () => {
    // Same trap as formatDay: a bare YYYY-MM-DD parsed as UTC midnight would
    // read as lapsed a day early west of Greenwich.
    expect(src).toContain('const parts = /^(\\d{4})-(\\d{2})-(\\d{2})$/.exec(t)');
    expect(src).toContain('new Date(Number(parts[1]), Number(parts[2]) - 1, Number(parts[3]))');
  });

  it('says a lapsed term removes nobody', () => {
    expect(src).toMatch(/The council serves until a successor is elected/);
  });
});

describe('GovernanceOverview — terms belong to elected leadership', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('passes hasTerms only for the elected model', () => {
    expect(src).toMatch(/hasTerms=\{overview\?\.rules\?\.leadership_model === 'elected'\}/);
  });
});
