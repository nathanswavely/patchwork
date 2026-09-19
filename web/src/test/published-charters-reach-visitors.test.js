/**
 * A charter published to everyone is reachable by everyone (docs/adr/036),
 * and the profile is where a visitor finds it (docs/adr/042: the patch
 * profile is a window, and every door names a room).
 *
 * This is F-052, found twice by the governance simulation. A coalition on
 * the Minimal template published its September minutes on purpose; a
 * signed-out visitor saw nothing about documents anywhere on the patch,
 * because `follower_permissions.charters: false` — a flag about *followers*
 * and the *members-only* shelf — had been wired to suppress the whole
 * governance glimpse. docs/adr/050 already ruled that follower permissions
 * gate taking part and workspace navigation, never readability.
 *
 * Source-text assertions: there is no Svelte render library in this project
 * (see router.test.js, patch-profile-window.test.js).
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('The governance glimpse is not gated on follower permissions', () => {
  const src = source('components/PatchProfileGlimpses.svelte');

  it('asks about governance on every claimed patch, whoever is looking', () => {
    expect(src).toMatch(/canSeeGovernance = \$derived\(!isUnclaimed\);/);
  });

  // The exact wiring that caused F-052.
  it('never reads charters or proposals off follower permissions to decide the section', () => {
    expect(src).not.toMatch(/followerPermissions\?\.charters/);
    expect(src).not.toMatch(/followerPermissions\?\.proposals/);
  });

  it('still treats an unclaimed patch as having no governance at all (docs/adr/039)', () => {
    expect(src).toMatch(/canSeeGovernance = \$derived\(\s*!isUnclaimed/);
    expect(src).toMatch(/showGovernance = \$derived\(\s*canSeeGovernance/);
  });

  it('renders the section when the room has something in it, or the viewer has standing', () => {
    expect(src).toMatch(
      /showGovernance = \$derived\(\s*canSeeGovernance && \(governanceDocs\.length > 0 \|\| recentProposals\.length > 0 \|\| hasStanding \|\| isAdmin\)/
    );
  });
});

describe('The profile gives the promised minutes somewhere to press', () => {
  const src = source('components/PatchProfileGlimpses.svelte');

  it('names the room, and points at the documents index rather than the container', () => {
    expect(src).toContain('/patches/{slug}/governance/docs');
    expect(src).toMatch(/>Documents<\/a>/);
  });

  // Honest door: a patch with nothing published grows no door onto an
  // empty room.
  it('renders the Documents door only when this viewer has a document to open', () => {
    expect(src).toMatch(/\{#if governanceDocs\.length > 0\}\s*<a\s+class="section-action"/);
  });
});

describe('An empty list says which kind of empty it is', () => {
  const glimpse = source('components/PatchProfileGlimpses.svelte');
  const list = source('pages/GovernanceList.svelte');

  it('reads the viewer-scoped signal off the listing that applied the rule', () => {
    expect(glimpse).toMatch(/governancePublishedOnly = charterData\.published_only === true/);
    expect(list).toMatch(/publishedOnly = data\.published_only === true/);
  });

  // Three now, not two
  // (docs/adr/2026-09-18-the-default-should-match-the-assumption.md): a
  // withheld record is a third kind of empty, and it has to be distinguished
  // from "nothing decided yet" for the same reason the other two are
  // distinguished from each other. A follower is the viewer who sees it —
  // they have standing enough for the section to render, and are an outsider
  // for the read.
  it('gives the glimpse three sentences, not two', () => {
    expect(glimpse).toMatch(/\{#if recordWithheld\}\s*Proposals and decisions here are not public\./);
    expect(glimpse).toMatch(/\{:else if governancePublishedOnly\}\s*Nothing published yet\./);
    expect(glimpse).toMatch(/\{:else\}\s*Nothing recorded yet\./);
  });

  it('reads the withheld signal off the listing that applied the rule', () => {
    expect(glimpse).toMatch(/recordWithheld = proposalData\.public_governance_record === 'nobody'/);
  });

  it('gives the documents page two sentences, not one', () => {
    expect(list).toContain("publishedOnly ? 'Nothing published yet.' : 'No governance documents yet.'");
  });

  // The signal is about the viewer, so neither sentence states or implies
  // that anything in particular is being withheld (docs/adr/036).
  it('says what the listing is without counting what it is not showing', () => {
    expect(list).toContain('Members-only documents are not listed here.');
    expect(list).not.toMatch(/hidden_count|withheld_count|members_only_count/);
    expect(glimpse).not.toMatch(/hidden_count|withheld_count|members_only_count/);
  });
});

describe('The documents page no longer refuses a follower the whole room', () => {
  const src = source('pages/GovernanceList.svelte');

  it('drops the blanket follower refusal (docs/adr/050)', () => {
    expect(src).not.toMatch(/permissionDenied/);
    expect(src).not.toContain('This content is only visible to members.');
    expect(src).not.toContain('Become a member to access documents.');
  });

  it('shows whatever the server handed over, which is already per-document filtered', () => {
    expect(src).toMatch(/docs = data\.items \|\| data \|\| \[\]/);
  });
});

describe('A patch that decides elsewhere is told which door the minutes go through', () => {
  const src = source('pages/GovernanceList.svelte');

  it('reads the proposal venue (docs/adr/052)', () => {
    expect(src).toMatch(/decidesElsewhere = \(rules\?\.proposal_venue \|\| 'patchwork'\) === 'elsewhere'/);
  });

  it('distinguishes a document from an adopted text, for the admin who has both buttons', () => {
    expect(src).toMatch(/\{#if decidesElsewhere\}/);
    expect(src).toMatch(/Minutes of a meeting go in a document\./);
    expect(src).toMatch(/replaces a charter with the version a meeting\s+adopted\./);
  });

  it('says it only where it applies — a patch that votes here has one door', () => {
    const block = src.match(/\{#if decidesElsewhere\}([\s\S]*?)\{\/if\}/);
    expect(block, 'the elsewhere signpost moved — repoint this test').not.toBeNull();
    expect(block[1]).toContain('Minutes of a meeting go in a document.');
  });
});
