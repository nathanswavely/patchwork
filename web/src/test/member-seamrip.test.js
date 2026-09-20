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

  // The heading leads with plain words now and keeps the product's own
  // word beside them: "Member seamrip" above a Danger Zone gave no clue
  // which way it cut, and one reader spent half a minute deciding whether
  // it would remove her (F-103).
  it('offers the control under its own heading', () => {
    expect(src).toContain('<h2>A copy of this quilt (seamrip)</h2>');
    expect(src).toMatch(/onclick=\{takeMemberSeamrip\}/);
  });

  it('fetches the member seamrip endpoint with the session', () => {
    expect(src).toContain("fetch('/api/v1/users/me/seamrip', { credentials: 'same-origin' })");
  });

  it('says it needs no admin, which is the whole point of the affordance', () => {
    expect(src).toContain('without');
    expect(src).toContain('waiting for an admin');
  });

  // Whitespace-normalised, because these are sentences in a paragraph and a
  // line wrap is not a change of promise.
  const prose = src.replace(/\s+/g, ' ');

  it('names what is not in it, so nobody assumes the bundle is a contact list', () => {
    expect(prose).toContain('It does not hold email addresses, contact cards, or noticeboards');
  });

  it('does not promise a narrower file than the boundary hands over', () => {
    // It used to say the archive held nothing "from a patch you are not in".
    // It holds every public patch on the quilt — which is what
    // `internal/seamrip/memberview.go` implements and what the README inside
    // the zip says. A member who read the old sentence and then opened the
    // file found five patches she was not in (docs/adr/106).
    expect(prose).not.toContain('anything from a patch you are not in');
    expect(prose).toContain('every public patch on this quilt and the private ones you belong to');
  });

  it('says people arrive as stubs and re-set their own visibility', () => {
    expect(prose).toContain('each person sets their own visibility there');
  });

  it('takes the filename the server chose rather than inventing one', () => {
    expect(src).toContain("a.download = named ? named[1] : 'patchwork-member-seamrip.zip'");
  });

  it('does not route the download through an admin surface', () => {
    expect(src).not.toContain('admin/export');
  });

  it('leaves the personal export beside it, not replaced by it', () => {
    expect(src).toContain('<h2>Your own data</h2>');
  });
});
