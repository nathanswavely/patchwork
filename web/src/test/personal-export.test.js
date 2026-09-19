/**
 * Personal export (docs/adr/012, affordance 1): the "Download my data"
 * control on Account settings. It is the member's data-rights baseline, so
 * the assertions here are about it being reachable without an admin and
 * about the sentence next to it telling the truth the handler enforces.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('Account settings: downloading your own record', () => {
  const src = source('pages/AccountSettings.svelte');

  it('offers the control under its own heading', () => {
    expect(src).toContain('<h2>Download my data</h2>');
    expect(src).toMatch(/onclick=\{downloadMyData\}/);
  });

  it('fetches the personal export endpoint with the session', () => {
    expect(src).toContain("fetch('/api/v1/users/me/export', { credentials: 'same-origin' })");
  });

  it('says what is in the file, hidden memberships included', () => {
    expect(src).toContain('every membership including the ones you keep hidden');
  });

  it('says what is not in it, so nobody fears the file is a key', () => {
    expect(src).toContain('no sign-in secrets');
  });

  it('takes the filename the server chose rather than inventing one', () => {
    expect(src).toContain("res.headers.get('Content-Disposition')");
    expect(src).toMatch(/a\.download = named \? named\[1\]/);
  });

  it('does not route the download through an admin surface', () => {
    expect(src).not.toContain('admin/export');
  });
});
