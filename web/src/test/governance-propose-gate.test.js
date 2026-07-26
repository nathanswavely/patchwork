/**
 * Proposing a rules change is a member act. A follower is an interested
 * observer, not a member (CONTEXT.md "Member count"), so neither the entry
 * point nor the editor route may offer it to one.
 *
 * The trap this guards: the node payload's `is_member` is true for ANY active
 * membership, followers included, so `isMember` alone is not the member test —
 * every gate here has to derive from `membershipRole`. There is no Svelte
 * render library in this project (see router.test.js, patch-setup.test.js), so
 * component wiring is asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('GovernanceOverview — propose entry point', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('derives the propose gate from membershipRole, not the follower-inclusive is_member', () => {
    expect(src).toMatch(/canPropose = \$derived\(\s*isAdmin \|\| membershipRole === 'member' \|\| membershipRole === 'admin'\s*\)/);
  });

  it('gates the propose action on canPropose', () => {
    expect(src).toMatch(/\{:else if canPropose\}/);
  });

  it('never gates an action on a bare isMember', () => {
    expect(src).not.toMatch(/\{#if isMember\b/);
    expect(src).not.toMatch(/\{:else if isMember\b/);
  });
});

describe('RulesProposalEditor — the route itself', () => {
  const src = source('pages/RulesProposalEditor.svelte');

  it('gates on membershipRole, since the route is reachable by URL and not only by the button', () => {
    expect(src).toMatch(/canPropose = \$derived\(\s*isAdmin \|\| membershipRole === 'member' \|\| membershipRole === 'admin'\s*\)/);
  });

  it('refuses the editor before anything else renders', () => {
    expect(src).toMatch(/\{#if !canPropose\}/);
    expect(src).toContain('Only members can propose a change to these rules.');
  });
});
