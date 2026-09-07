/**
 * Self-serve account deletion (docs/adr/086).
 *
 * These assert on source text, which is all this suite can do — they cannot
 * see a rendered page. So they check the two things a rendering bug would
 * not hide: that the danger zone goes through the same step-up path as the
 * other irreversible actions, and that the copy on it actually describes
 * what the server does rather than a friendlier version of it.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('Account settings: the danger zone', () => {
  const src = source('pages/AccountSettings.svelte');

  it('calls DELETE /users/me through the shared step-up wrapper', () => {
    expect(src).toContain("withStepUp(() => api('users/me', {");
    expect(src).toContain("method: 'DELETE',");
    expect(src).toContain('body: { confirm_username: confirmUsername },');
  });

  it('warns about a missing passkey before the button, not after', () => {
    expect(src).toContain('import PasskeyNotice');
    expect(src).toContain('<PasskeyNotice show={!hasPasskey} action="delete your account" />');
    expect(src).toContain('stepUpStatus().then');
  });

  it('will not arm until the username is typed exactly', () => {
    expect(src).toContain('disabled={confirmUsername !== user.username}');
    expect(src).toContain('Type your username to confirm:');
  });

  it('says both halves of what deletion does', () => {
    expect(src).toContain('<strong>Erased:</strong>');
    expect(src).toContain('<strong>Kept:</strong>');
    expect(src).toContain('Deleted account');
    expect(src).toContain("record stays whole");
  });

  it('says the handle is retired rather than freed', () => {
    expect(src).toContain('your username stays');
    expect(src).toContain('retired so nobody else can take it');
  });

  it('states the two limits the server cannot reach past', () => {
    expect(src).toContain('federated to other servers');
    expect(src).toContain('export taken before today');
  });

  it('is honest that it cannot be undone', () => {
    expect(src).toContain('This happens immediately and cannot be undone.');
  });

  it('lists the patches the server refused over, by name and link', () => {
    expect(src).toContain('blockingPatches = e?.data?.patches || []');
    expect(src).toContain('{#each blockingPatches as p}');
    expect(src).toContain('name a successor, promote another admin, or hold an election');
  });

  it('hard-reloads after success, because every store still holds the deleted person', () => {
    expect(src).toContain('localStorage.clear();');
    expect(src).toContain("window.location.href = '/';");
  });
});
