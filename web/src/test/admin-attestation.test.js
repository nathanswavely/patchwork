/**
 * Proving the admin role to an outside party (docs/adr/087).
 *
 * The page's whole risk is that somebody reads the result as identity, so
 * most of what is asserted here is copy: what it proves, what it does not,
 * and where the verifier gets the key. The rest is that it runs the same
 * step-up flow the other outward-facing admin actions run.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('Admin attestation page', () => {
  const src = source('pages/AdminAttestation.svelte');

  it('posts the nonce to the admin attestation endpoint', () => {
    expect(src).toContain("api('admin/attestation', { method: 'POST', body: { nonce: nonce.trim() } })");
  });

  it('runs the shared step-up flow rather than its own', () => {
    expect(src).toContain("from '../lib/stepUp.js'");
    expect(src).toContain('withStepUp(()');
    expect(src).toContain('stepUpStatus()');
  });

  it('warns about a missing passkey up front, not at the click', () => {
    expect(src).toContain('<PasskeyNotice show={!hasPasskey}');
    expect(src).toContain('PasskeyRequiredError');
  });

  it('says plainly what the attestation proves', () => {
    expect(src).toContain('an admin of this quilt signed that exact');
  });

  it('says just as plainly that it is not identity', () => {
    expect(src).toContain('What it does not prove:');
    expect(src).toContain('No username, no email, no account');
    expect(src).toContain('does not publish who its admins are');
  });

  it('shows the expiry beside the blob, because the blob does not keep', () => {
    expect(src).toContain('Good until');
    expect(src).toContain('result.statement.expires_at');
  });

  it('offers the blob in a copyable box', () => {
    expect(src).toContain('navigator.clipboard.writeText(result.attestation)');
    expect(src).toContain('class="blob"');
  });

  it('tells the admin to send the verifier to the quilt’s own key, not a link', () => {
    expect(src).toContain('result.key_url');
    expect(src).toContain('not from a');
  });

  it('bounds the nonce field the way the server bounds it', () => {
    expect(src).toContain('maxlength="64"');
    expect(src).toContain('8 to 64 printable characters');
  });

  it('never renders a username, email, or account id — there is none to render', () => {
    expect(src).not.toMatch(/result\.(user|username|email|admin)/);
  });
});

describe('Admin panel wiring', () => {
  it('gives the page a tab in the admin shell', () => {
    const shell = source('components/AdminShell.svelte');
    expect(shell).toContain("href: '/admin/attestation'");
    expect(shell).toContain("label: 'Prove admin'");
  });

  it('routes /admin/attestation to the page', () => {
    const app = source('App.svelte');
    expect(app).toContain("addRoute('/admin/attestation', 'adminAttestation')");
    expect(app).toContain("routeName === 'adminAttestation'");
    expect(app).toContain('<AdminAttestation />');
  });

  // Registering the route and rendering the component is not enough: two
  // separate lists decide whether the admin shell wraps a route and whether
  // it needs a session. Adding the page to one and not the others renders
  // "Page not found" on a route that exists, which is how this shipped the
  // first time — a defect no source-text assertion above would have caught.
  it('puts the route in every list a rendered admin page has to be in', () => {
    const app = source('App.svelte');
    const adminSet = app.match(/const adminRoutes = new Set\(\[[^\]]*\]\)/s)?.[0] || '';
    expect(adminSet).toContain("'adminAttestation'");

    const authGuard = app.match(/let authRequired = \$derived\(\s*\[[^\]]*\]/s)?.[0] || '';
    expect(authGuard).toContain("'adminAttestation'");
  });

  it('is findable from the admin finder', () => {
    const providers = source('lib/finderProviders.js');
    expect(providers).toContain("href: '/admin/attestation'");
  });
});
