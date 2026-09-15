/**
 * Membership invitations (docs/adr/098).
 *
 * An invite-only patch had no door: JoinNode refuses on invite_only and no
 * route added a member, so a person told "you're in" saw only Follow. An
 * admin now invites by username and the person accepts or declines. Until
 * they answer the row is status 'invited', which is not membership
 * anywhere — not in the store's roles, not in the zero-memberships
 * onboarding redirect, not in My Patches.
 *
 * The store half runs the module; the component half asserts against
 * source text, because there is no Svelte render library in this project.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const memberships = [
  { node_slug: 'the-selvage', role: 'admin', status: 'active', joined_at: '2026-01-02T00:00:00Z' },
  { node_slug: 'quarry-house', role: 'member', status: 'pending', joined_at: '2026-01-05T00:00:00Z' },
];
const invitations = [
  { node_slug: 'the-choir', node_name: 'The Choir', status: 'invited', invited_at: '2026-01-06T00:00:00Z' },
];

describe('memberships store — an invitation is not a membership', () => {
  let store;

  beforeEach(async () => {
    vi.resetModules();
    vi.doMock('../lib/api.js', () => ({
      api: async (path) => (path === 'users/me/invitations' ? { items: invitations } : { items: memberships }),
    }));
    store = await import('../stores/memberships.svelte.js');
    await store.loadMemberships();
  });

  it('keeps invitations out of getMemberships(), which every "my patches" surface counts', () => {
    // App.svelte's onboarding redirect fires on getMemberships().length === 0;
    // a person with one invitation and no memberships must still be sent
    // to onboarding, not shown a My Patches with a patch they are not in.
    const slugs = store.getMemberships().map((m) => m.node_slug);
    expect(slugs).toEqual(['the-selvage', 'quarry-house']);
    expect(slugs).not.toContain('the-choir');
  });

  it('reports no role for an invited patch', () => {
    expect(store.getMembershipRoles().has('the-choir')).toBe(false);
    expect(store.getPendingMembershipSlugs().has('the-choir')).toBe(false);
  });

  it('exposes the invitation by name, for the relationship row alone', () => {
    expect(store.getInvitedMembershipSlugs().has('the-choir')).toBe(true);
    expect(store.getInvitedMembershipSlugs().has('the-selvage')).toBe(false);
    expect(store.getInvitations()).toHaveLength(1);
  });

  it('clears invitations on logout with the rest', () => {
    store.clearMemberships();
    expect(store.getInvitedMembershipSlugs().size).toBe(0);
    expect(store.getInvitations()).toEqual([]);
  });

  it('does not mistake a stubbed me/nodes payload for invitations', async () => {
    // A stub that answers every path with the same rows — the shape of
    // every other test's mock — must not turn a membership into an invite.
    vi.resetModules();
    vi.doMock('../lib/api.js', () => ({ api: async () => ({ items: memberships }) }));
    const s = await import('../stores/memberships.svelte.js');
    await s.loadMemberships();
    expect(s.getInvitedMembershipSlugs().size).toBe(0);
  });
});

describe('PatchRelationship — the invited state offers Accept and Decline instead of Follow', () => {
  const src = source('components/PatchRelationship.svelte');

  it('takes an invited prop and derives a state that is not standing', () => {
    expect(src).toMatch(/invited = false,/);
    expect(src).toMatch(/let invitedHere = \$derived\(invited && !standing && !isBanned && !hasMoved\)/);
  });

  it('renders the invited branch ahead of Follow, with both answers in the open', () => {
    const invitedBranch = src.indexOf('{:else if invitedHere}');
    const followBranch = src.indexOf('{:else if !hasMoved}');
    expect(invitedBranch).toBeGreaterThan(-1);
    expect(invitedBranch).toBeLessThan(followBranch);
    expect(src).toMatch(/>You've been invited to join</);
    expect(src).toMatch(/onclick=\{handleAccept\}[^>]*>Accept</);
    expect(src).toMatch(/onclick=\{handleDecline\}[^>]*>Decline</);
  });

  it('posts to the invitation endpoints, never to join or leave', () => {
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/invitations\/accept`, \{ method: 'POST' \}\)/);
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/invitations\/decline`, \{ method: 'POST' \}\)/);
  });

  it('withholds the membership rung while an invitation is open', () => {
    expect(src).toMatch(/let canBecomeMember = \$derived\([\s\S]*?!invitedHere &&[\s\S]*?\)/);
  });
});

describe('the relationship row is told about the invitation on both surfaces that mount it', () => {
  for (const rel of ['components/PatchProfileHead.svelte', 'components/PatchShell.svelte']) {
    it(`${rel} reads the store by name and passes invited`, () => {
      const src = source(rel);
      expect(src).toMatch(/getInvitedMembershipSlugs/);
      expect(src).toMatch(/let invited = \$derived\(getInvitedMembershipSlugs\(\)\.has\(slug\)\)/);
      expect(src).toMatch(/\{requestPending\}\s*\n\s*\{invited\}/);
    });
  }
});

describe('PatchSettingsMembers — the admin side', () => {
  const src = source('pages/PatchSettingsMembers.svelte');

  it('has an invite-by-username form posting to the invitations route', () => {
    expect(src).toMatch(/<h3 class="section-heading">Invite a member<\/h3>/);
    expect(src).toMatch(/aria-label="Username to invite"/);
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/invitations`, \{ method: 'POST', body: \{ username \} \}\)/);
  });

  it('reads the invited list from the members payload and offers Rescind', () => {
    expect(src).toMatch(/invited = data\.invited \|\| \[\]/);
    expect(src).toMatch(/\{#each invited as person \(person\.user_id\)\}/);
    expect(src).toMatch(/label="Rescind"/);
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/invitations\/\$\{userId\}`, \{ method: 'DELETE' \}\)/);
  });

  it('says what Invite does not do — it asks, it never admits', () => {
    expect(src).toMatch(/They're notified and join only if they accept\./);
  });
});
