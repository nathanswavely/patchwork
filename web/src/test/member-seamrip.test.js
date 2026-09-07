/**
 * Member seamrip (docs/adr/012, affordance 2; docs/adr/089): the "Take a
 * copy" control on Account settings. It sits beside "Download my data"
 * because both are egress a member holds without an admin, and the
 * assertions here are about the sentence next to it matching what the
 * boundary actually carries.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('Account settings: taking a copy of the quilt', () => {
  const src = source('pages/AccountSettings.svelte');

  it('offers the control under its own heading', () => {
    expect(src).toContain('<h2>Member seamrip</h2>');
    expect(src).toMatch(/onclick=\{takeMemberSeamrip\}/);
  });

  it('fetches the member seamrip endpoint with the session', () => {
    expect(src).toContain("fetch('/api/v1/users/me/seamrip', { credentials: 'same-origin' })");
  });

  it('says it needs no admin, which is the whole point of the affordance', () => {
    expect(src).toContain('without');
    expect(src).toContain('waiting for an admin');
  });

  it('names what is not in it, so nobody assumes the bundle is a contact list', () => {
    expect(src).toContain('It does not');
    expect(src).toContain('email addresses, contact cards, noticeboards');
  });

  it('says people arrive as stubs and re-set their own visibility', () => {
    expect(src).toContain('sets their own');
  });

  it('takes the filename the server chose rather than inventing one', () => {
    expect(src).toContain("a.download = named ? named[1] : 'patchwork-member-seamrip.zip'");
  });

  it('does not route the download through an admin surface', () => {
    expect(src).not.toContain('admin/export');
  });

  it('leaves the personal export beside it, not replaced by it', () => {
    expect(src).toContain('<h2>Download my data</h2>');
  });
});
