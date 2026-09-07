import { describe, it, expect, beforeEach, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

/**
 * A pending join request is not standing.
 *
 * `GET /api/v1/me/nodes` serves status='active' and status='pending' rows
 * alike (internal/handler/memberships.go, ListMyMemberships), and a pending
 * row carries role='member' — that is what the row will become, not what it
 * is. The store used to build its slug → role map from `role` alone, so a
 * person whose request nobody had answered read as a member on every
 * surface that consulted it.
 *
 * The server never agreed: userHasNodeRole checks status independently, and
 * the node payload's `membership_role` is set only for an active row
 * (nodes.go). So this was a client-side misrepresentation, not a permission
 * hole — which is exactly why no API test could catch it, and why the fix
 * belongs in the store rather than in each of its callers.
 *
 * These run the store rather than reading its source: it is plain logic,
 * and a source-text assertion here would pass on a map built the old way
 * with a comment about status above it.
 */

const items = [
  { node_slug: 'the-selvage', role: 'admin', status: 'active', joined_at: '2026-01-02T00:00:00Z' },
  { node_slug: 'gallery-row', role: 'member', status: 'active', joined_at: '2026-01-03T00:00:00Z' },
  { node_slug: 'tin-shop', role: 'follower', status: 'active', joined_at: '2026-01-04T00:00:00Z' },
  // The row this whole file exists for.
  { node_slug: 'quarry-house', role: 'member', status: 'pending', joined_at: '2026-01-05T00:00:00Z' },
];

let store;

describe('membership standing', () => {
  beforeEach(async () => {
    vi.resetModules();
    vi.doMock('../lib/api.js', () => ({ api: async () => ({ items }) }));
    store = await import('../stores/memberships.svelte.js');
    await store.loadMemberships();
  });

  it('keeps a pending request out of the role map', () => {
    const roles = store.getMembershipRoles();
    expect(roles.get('quarry-house')).toBeUndefined();
    expect(roles.has('quarry-house')).toBe(false);
  });

  it('still reports every active row, at its own role', () => {
    const roles = store.getMembershipRoles();
    expect(roles.get('the-selvage')).toBe('admin');
    expect(roles.get('gallery-row')).toBe('member');
    expect(roles.get('tin-shop')).toBe('follower');
    expect(roles.size).toBe(3);
  });

  it('keeps the pending request askable by name', () => {
    // Dropping the row from the role map must not lose the fact. A surface
    // offering "Follow" or "Become a member" to someone already waiting on
    // an answer offers a control the server refuses with a 409
    // (memberships.go: "membership request already pending").
    const requested = store.getPendingMembershipSlugs();
    expect(requested.has('quarry-house')).toBe(true);
    expect(requested.size).toBe(1);
  });

  it('clears both on logout', () => {
    store.clearMemberships();
    expect(store.getMembershipRoles().size).toBe(0);
    expect(store.getPendingMembershipSlugs().size).toBe(0);
  });
});

/**
 * The consumers. Source-text, because these are surfaces and there is no
 * Svelte render library here — they guard that each one asks the question
 * it means, not that the pixels land.
 */
const read = (p) => readFileSync(resolve(__dirname, '..', p), 'utf8');

describe('surfaces that state where you stand', () => {
  it('greets a new member from the active row, not the pending one', () => {
    // The unlock panel reads getMemberships() directly rather than the role
    // map, so the store's fix does not reach it — it checks status itself.
    // Greeting a would-be member with what membership unlocked would be a
    // promise the patch has not made yet.
    const src = read('components/UnlockPanel.svelte');
    expect(src).toMatch(/membership\?\.status === 'active' &&\s*\n\s*membership\?\.role === 'member'/);
  });

  it('offers withdraw on a request, and never leave', () => {
    // Leave and withdraw are different verbs on the server: leave takes
    // active rows only, because nobody admitted this person and there is
    // no community to exit. A Requested row calling leave would 400.
    const settings = read('pages/UserSettingsPatches.svelte');
    expect(settings).toMatch(/api\(`nodes\/\$\{slug\}\/withdraw`, \{ method: 'POST' \}\)/);
    expect(settings).toMatch(/label="Withdraw"[\s\S]{0,120}handleWithdraw\(m\.node_slug\)/);

    const rel = read('components/PatchRelationship.svelte');
    expect(rel).toMatch(/api\(`nodes\/\$\{slug\}\/withdraw`, \{ method: 'POST' \}\)/);
    // Behind the menu, like every other undo in this row — not a button
    // sitting next to the thing it undoes.
    expect(rel).toMatch(/role="menuitem" onclick=\{handleWithdraw\}[\s\S]{0,60}Withdraw request/);
    // And it must not reach for leave, which would 400 on a pending row.
    expect(rel).not.toMatch(/awaiting[\s\S]{0,400}handleLeave/);
  });

  it('files a request under Requested, not under Member of', () => {
    // Settings reads me/nodes directly too. A pending row under "Member of"
    // came with the switches an actual member owns (visibility, contact
    // sharing) over a standing this person does not have.
    const src = read('pages/UserSettingsPatches.svelte');
    expect(src).toMatch(/let active = \$derived\(patches\.filter\(m => m\.status === 'active'\)\)/);
    expect(src).toMatch(/adminPatches = \$derived\(active\./);
    expect(src).toMatch(/memberPatches = \$derived\(active\./);
    expect(src).toMatch(/followerPatches = \$derived\(active\./);
    expect(src).toMatch(/pendingPatches = \$derived\(patches\.filter\(m => m\.status === 'pending'\)\)/);
    expect(src).toMatch(/>Requested</);
  });

  it('says "Requested" where it would otherwise offer a Follow that 409s', () => {
    for (const p of ['pages/Discover.svelte', 'pages/SocialHome.svelte']) {
      const src = read(p);
      expect(src).toContain('getPendingMembershipSlugs');
      expect(src).toMatch(/Requested/);
    }
  });

  it('offers Follow on Discover only where Follow can succeed', () => {
    // Discover's Follow posts a join, and the server refuses it for anyone
    // who already stands here: 409 "already a member" for a member or an
    // admin, 409 "membership request already pending" for a request still
    // out. All three are stated instead, in the relationship row's own
    // words, and only follower/never-met keep the button.
    const src = read('pages/Discover.svelte');
    expect(src).toMatch(/function standingLabel\(slug\)/);
    expect(src).toMatch(/if \(requested\.has\(slug\)\) return 'Requested'/);
    expect(src).toMatch(/if \(role === 'admin'\) return 'Admin'/);
    expect(src).toMatch(/if \(role === 'member'\) return 'Member'/);
    // The button is reached only when standingLabel found nothing.
    expect(src).toMatch(/\{#if standing\}[\s\S]{0,200}\{:else\}[\s\S]{0,300}onclick=\{\(\) => toggleFollow\(patch\)\}/);
    // And one control for both lists, so the two cannot drift apart again.
    expect(src.match(/@render followControl\(patch\)/g)).toHaveLength(2);
    expect(src.match(/onclick=\{\(\) => toggleFollow\(patch\)\}/g)).toHaveLength(1);
  });

  it("borrows the relationship row's words rather than inventing its own", () => {
    // A reader meeting "Member" on Discover and "Member" on the patch's
    // page should not have to work out whether they mean the same thing.
    const rel = read('components/PatchRelationship.svelte');
    expect(rel).toContain("label: 'Member'");
    expect(rel).toContain("label: 'Admin'");
  });

  it('leaves the admin-only gates alone, since a pending row is never admin', () => {
    // EventLinks asks only for 'admin', which no pending row can carry, and
    // had already worked around this store per-consumer when it needed an
    // active admin list. Nothing there needed changing — this pins that.
    const src = read('components/EventLinks.svelte');
    expect(src).toMatch(/m\.role === 'admin' && m\.status === 'active'/);
  });
});
