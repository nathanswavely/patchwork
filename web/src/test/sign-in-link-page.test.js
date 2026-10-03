/**
 * The page a sign-in email's link opens
 * (docs/adr/2026-09-28-a-link-knows-where-it-was-asked-for.md).
 *
 * Loading it spends nothing. It posts the token once, signs in when the
 * server says this is the browser that asked, and otherwise shows the code
 * with a "Sign in on this device" button that spends the link.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text (CLAUDE.md, "Verifying frontend changes").
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('the sign-in link route', () => {
  const app = source('App.svelte');

  it('is registered at the path Go emails (weblink.SignInLink)', () => {
    expect(app).toContain("addRoute('/login/link/:token', 'signInLink')");
  });

  it('renders in the threshold shell with the other sign-in pages', () => {
    expect(app).toMatch(/standaloneRoutes = new Set\(\[[^\]]*'signInLink'/);
    expect(app).toContain("{:else if routeName === 'signInLink'}");
  });
});

describe('the sign-in link page', () => {
  const src = source('pages/SignInLink.svelte');

  it('opens the link by POST, never by the GET that loaded it', () => {
    expect(src).toContain("api('auth/magic-link/open', { method: 'POST', body: { token, here } })");
    expect(src).toContain('open(false)');
  });

  it('shows the code when this is not the browser that asked', () => {
    expect(src).toContain("res?.status === 'code'");
    expect(src).toContain('Your sign-in code');
    expect(src).toContain('{spacedCode}');
  });

  it('offers "Sign in on this device", which opens with here: true', () => {
    expect(src).toContain('Sign in on this device');
    expect(src).toMatch(/async function signInHere\(\)[\s\S]*?await open\(true\)/);
  });

  it('sends a first-time address to username selection (docs/adr/013)', () => {
    expect(src).toContain("res?.status === 'username_required'");
    expect(src).toContain("'/signup/complete?token=' + encodeURIComponent(res.signup_token)");
  });

  it('honours the redirect Login.svelte kept across the email round-trip', () => {
    expect(src).toContain("localStorage.getItem('patchwork_auth_redirect')");
    expect(src).toContain('isSafeRedirectPath(pending)');
  });
});
