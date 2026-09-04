/**
 * The contact card (docs/adr/080): how to reach you, kept once on the
 * account and shared patch by patch. Three surfaces, and none of them is
 * public — every assertion here is about the card staying inside the room.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('Account settings: the card is edited whole, and says it is not public', () => {
  const src = source('pages/AccountSettings.svelte');

  it('saves the card as one object, never a field at a time', () => {
    expect(src).toMatch(/contact_card:\s*\{\s*phone:.*\n.*email:.*\n.*note:/);
  });

  it('tells the person nothing on the card is public until shared', () => {
    expect(src).toContain('Nothing here is public');
    expect(src).toContain('Not shared with any patch yet.');
  });

  it('keeps the contact email apart from the sign-in address', () => {
    expect(src).toContain('Separate from the address you sign in with. That one is never shared.');
  });
});

describe('My Patches: contact sharing is a second switch beside visibility', () => {
  const src = source('pages/UserSettingsPatches.svelte');

  it('flips share_contact through the membership switch endpoint', () => {
    expect(src).toMatch(/users\/me\/memberships\/\$\{m\.node_id\}/);
    expect(src).toContain('body: { share_contact: !m.share_contact }');
  });

  it('offers the switch on admin and member rows only — followers are not in the room', () => {
    const rendered = src.match(/\{@render contactToggle\(m\)\}/g) || [];
    expect(rendered).toHaveLength(2);
    const following = src.slice(src.indexOf('<h3 class="section-heading">Following</h3>'));
    expect(following).not.toContain('contactToggle');
  });

  it('warns that sharing reaches people who join later', () => {
    expect(src).toContain('including anyone who joins later');
  });
});

describe('Members room: the card shows only where the API sent it', () => {
  const src = source('pages/PatchMembers.svelte');

  it('renders contact only from the per-member contact object', () => {
    expect(src).toContain('{#if member.contact}');
    expect(src).toMatch(/href="tel:\{member\.contact\.phone/);
    expect(src).toMatch(/href="mailto:\{member\.contact\.email\}"/);
  });

  it('offers sharing to a member in the room whose own row carries no card', () => {
    expect(src).toContain("let inRoom = $derived(membershipRole === 'member' || membershipRole === 'admin');");
    expect(src).toContain('let offerSharing = $derived(inRoom && myRow && !myRow.contact);');
  });
});

describe('The public profile never learns the card', () => {
  it('UserProfile.svelte does not reach for contact', () => {
    expect(source('pages/UserProfile.svelte')).not.toMatch(/contact/i);
  });
});
