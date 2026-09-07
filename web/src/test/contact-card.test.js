/**
 * The contact card (docs/adr/083, superseding 080): the ways a person is
 * willing to be reached, kept once on the account as typed items and shared
 * one item into one patch at a time. Every assertion here is about an item
 * reaching exactly the room it was given to, and no further.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('Account settings: the card is items, and this page cannot grant', () => {
  const src = source('pages/AccountSettings.svelte');

  it('edits items through the item endpoints, not a whole-card object', () => {
    expect(src).toContain("api('users/me/contact-items'");
    expect(src).toMatch(/users\/me\/contact-items\/\$\{it\.id\}/);
    expect(src).not.toContain('contact_card:');
  });

  it('cannot grant from here — the only sharing action takes one back', () => {
    // docs/adr/083 decision 5: item-first writes may only reduce exposure.
    // Granting is patch-first, so this page must carry no picker of patches
    // and call nothing that adds a share.
    expect(src).toContain("contact-items/${it.id}/shares`, { method: 'DELETE' }");
    expect(src).not.toContain('contact-shares');
  });

  it('says the surface stopped, not that access was revoked', () => {
    // Unsharing stops the showing, not the knowing: anyone who already read
    // the value still has it, so no copy may promise otherwise.
    expect(src).toContain('No longer shown to');
  });

  it('tells the person nothing on the card is public, and sharing is per patch', () => {
    expect(src).toContain('Nothing here is public');
    expect(src).toContain('one patch at a\n        time');
    expect(src).toContain('Not shared with any patch');
  });

  it('keeps the contact email apart from the sign-in address', () => {
    expect(src).toContain('Separate from the address you sign in with. That one is never shared.');
  });
});

describe('My Patches: sharing is patch-first, per item', () => {
  const src = source('pages/UserSettingsPatches.svelte');

  it('shares per item, into one patch, by replacing the whole set', () => {
    // docs/adr/083: PUT of the set, not a boolean per membership — and a
    // partial update of a disclosure set is a request to half-apply it.
    expect(src).toMatch(/nodes\/\$\{m\.node_slug\}\/contact-shares/);
    expect(src).toContain("method: 'PUT'");
    expect(src).toContain('body: { item_ids: sharingIds }');
    expect(src).not.toContain('share_contact');
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

  it('offers sharing to a member in the room who shares no card', () => {
    expect(src).toContain("let inRoom = $derived(membershipRole === 'member' || membershipRole === 'admin');");
    expect(src).toContain('let offerSharing = $derived(inRoom && !viewerSharesContact);');
  });

  it('asks the room, not the loaded page, who is sharing', () => {
    // Both facts used to be read off `members`, so a member on page 4 was
    // never offered the switch: their own row had not arrived to say they
    // were missing a card.
    expect(src).toContain('viewerSharesContact = !!data.viewer_shares_contact');
    expect(src).toContain('anyContact = !!data.any_contact_shared');
    expect(src).not.toContain('members.find(');
    expect(src).not.toContain('members.some(');
  });
});

describe('Members room: the count is admins plus members, never followers', () => {
  const src = source('pages/PatchMembers.svelte');

  it('takes both counts from the server, never from the loaded page', () => {
    expect(src).toContain('memberCount = data.member_count');
    expect(src).toContain('followerCount = data.follower_count');
    // A paged listing counted client-side reports the page size as the
    // patch's size — the header said "20 members" of a patch with 60.
    expect(src).not.toMatch(/members\.filter\(.*\)\.length/);
    expect(src).not.toMatch(/\{members\.length\} members/);
  });

  it('follows next_cursor so a patch bigger than one page is reachable', () => {
    expect(src).toContain("nextCursor = data.next_cursor || ''");
    expect(src).toContain('loadMembers(nextCursor)');
    expect(src).toMatch(/members = after \? \[\.\.\.members, \.\.\.items\]/);
  });
});

describe('The profile is a window onto the room, never a wider one', () => {
  const src = source('pages/UserProfile.svelte');

  it('renders only what the server sent, and asks for nothing extra', () => {
    // The predicate lives in the API. If the page ever fetched contact data
    // on its own it would be deciding disclosure in the client.
    expect(src).toContain("$derived(profile?.contact || [])");
    expect(src).not.toMatch(/api\(['`][^'`]*contact/);
  });

  it('never names the patch an item came through', () => {
    // The granting membership may be private or hidden, and docs/adr/006
    // keeps those off this page — so the copy is audience-shaped, not
    // provenance-shaped.
    const start = src.indexOf('<ul class="profile-contact">');
    const section = src.slice(start, src.indexOf('</ul>', start));
    expect(start).toBeGreaterThan(-1);
    expect(section).not.toMatch(/node_slug|node_name|patch/i);
    expect(src).toContain('Shared with people they organize with');
  });

  it('shows no heading at all to a visitor who shares no room', () => {
    // An empty "Reach them" section would tell the internet a card exists.
    expect(src).toContain('{#if contact.length > 0}');
  });
});
