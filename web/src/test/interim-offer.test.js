import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { resolve } from 'path';

// An interim admin is asked first (docs/adr/2026-10-19-an-interim-admin-is-asked-first.md,
// F-085). The inactivity sweep used to make three members admins and tell
// them afterwards; now it asks, and this is where the answer is given.

const read = (p) => readFileSync(resolve(__dirname, '..', p), 'utf8');

describe('InterimOffer', () => {
  const src = read('components/InterimOffer.svelte');

  it('finds the question addressed to this reader, for this patch only', () => {
    expect(src).toContain("api('users/me/interim-offers')");
    expect(src).toMatch(/\.find\(\(o\) => o\.node_slug === s\)/);
  });

  it('answers with the two verbs the server offers, and nothing else', () => {
    expect(src).toContain('api(`nodes/${slug}/interim-offer/${verb}`, { method: \'POST\' })');
    expect(src).toContain("onclick={() => answer('accept')}");
    expect(src).toContain("onclick={() => answer('decline')}");
  });

  it('says that nothing changes unless they accept, the sentence the old notice lacked', () => {
    expect(src).toContain('Nothing changes unless you accept.');
  });

  it('renders nothing for anybody who was not asked', () => {
    expect(src).toMatch(/\{#if offer\}/);
  });
});

describe('the Governance overview carries it first', () => {
  const src = read('components/GovernanceOverview.svelte');

  it('puts the question above every other banner, and refreshes standing after an answer', () => {
    const offerAt = src.indexOf('<InterimOffer {slug}');
    const voteAt = src.indexOf('Action banner');
    expect(offerAt).toBeGreaterThan(-1);
    expect(offerAt).toBeLessThan(voteAt);
    expect(src).toMatch(/onAnswered=\{async \(\) => \{ patch\.value\.reload\?\.\(\); await loadOverview\(\); \}\}/);
  });
});

describe('the no-admins line knows a question is out', () => {
  const src = read('components/GovernanceOverview.svelte');

  // It said only an instance admin could put anybody back, directly under a
  // question whose Accept does it. Checked first, and never names anyone.
  it('does not tell a patch with questions out that only an instance admin can help', () => {
    const line = src.indexOf('if (overview?.interim_offers_open)');
    expect(line).toBeGreaterThan(-1);
    expect(line).toBeLessThan(src.indexOf('if (leadershipElsewhere)'));
    expect(src).toContain('Its longest-standing active members have been asked to step in as interim admins.');
  });
});
