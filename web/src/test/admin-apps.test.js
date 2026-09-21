/**
 * The apps a quilt vouches for
 * (docs/adr/2026-09-20-an-instance-vouches-for-an-app.md).
 *
 * Nothing here renders, so what is asserted is the page's source text: that
 * it talks to the one endpoint, that the add — and only the add — runs through
 * step-up, and that the copy says the thing the feature turns on. The copy is
 * load-bearing: an admin who reads "listing an app" as a directory entry has
 * misunderstood a passkey grant.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { adminTabs } from '../lib/adminPanel.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('Admin apps page', () => {
  const src = source('pages/AdminApps.svelte');

  it('reads and writes the one admin endpoint', () => {
    expect(src).toContain("api('admin/native-apps')");
    expect(src).toContain("api('admin/native-apps', { method: 'POST', body })");
    expect(src).toContain('api(`admin/native-apps/${app.id}`, { method: \'DELETE\' })');
  });

  it('wraps the add in step-up, because listing an app is a grant outward', () => {
    expect(src).toContain("from '../lib/stepUp.js'");
    expect(src).toContain("withStepUp(() => api('admin/native-apps', { method: 'POST', body }))");
    expect(src).toContain('stepUpStatus()');
    expect(src).toContain('<PasskeyNotice show={!hasPasskey}');
    expect(src).toContain('PasskeyRequiredError');
  });

  it('does not wrap the remove, because taking trust back is the safe direction', () => {
    const remove = src.slice(src.indexOf('async function handleRemove'), src.indexOf('function platformLabel'));
    expect(remove).not.toContain('withStepUp');
  });

  it('says what listing an app does, in the words the decision uses', () => {
    expect(src).toContain("A native app can hold this quilt's passkeys only if approved by this quilt.");
    expect(src).toContain('Listing an app publishes its identifier at the address the app stores check.');
  });

  it('shows the two published addresses and says Apple caches for up to a day', () => {
    expect(src).toContain('{urls.apple');
    expect(src).toContain('{urls.android');
    expect(src).toContain('Apple caches the file for up to a day');
  });

  it('says that an unlisted platform publishes nothing at all', () => {
    expect(src).toContain('answers "not found" until you list an app');
    expect(src).toContain('publishes neither file');
  });

  it('lists a row with its platform, label, identifier, fingerprints and date', () => {
    expect(src).toContain('{platformLabel(app.platform)}');
    expect(src).toContain('{app.label}');
    expect(src).toContain('{app.identifier}');
    expect(src).toContain('{#each app.fingerprints as print (print)}');
    expect(src).toContain('Listed {addedOn(app.created_at)}');
    expect(src).toContain('>Remove</button>');
  });

  it('asks for a per-platform identifier and only asks Android for fingerprints', () => {
    expect(src).toContain("apple: 'ABCDE12345.org.example.app'");
    expect(src).toContain("android: 'org.example.app'");
    expect(src).toContain('placeholder={PLACEHOLDERS[platform]}');
    expect(src).toContain("{#if platform === 'android'}");
    expect(src).toContain('<textarea');
    expect(src).toContain("body.fingerprints = fingerprints");
    expect(src).toContain(".split('\\n')");
  });

  it('bounds the label the way the server bounds it', () => {
    expect(src).toContain('maxlength="64"');
  });

  it('shows the API’s own refusal inline rather than one shrug', () => {
    expect(src).toContain('e.data?.error');
    expect(src).toContain('{#if error}');
  });
});

describe('Admin panel wiring', () => {
  it('gives the page a section of the admin panel, last under Settings', () => {
    const registry = source('lib/adminPanel.js');
    expect(registry).toContain("{ id: 'apps', label: 'Apps' }");
    const settings = adminTabs().find((t) => t.id === 'settings');
    expect(settings.sections.at(-1).id).toBe('apps');
    expect(settings.sections.at(-1).href).toBe('/admin/settings/apps');
  });

  it('routes /admin/settings/apps to the page', () => {
    const app = source('App.svelte');
    expect(app).toContain("addRoute('/admin/settings/apps', 'adminApps')");
    expect(app).toContain("routeName === 'adminApps'");
    expect(app).toContain('<AdminApps />');
  });

  // Registering the route and rendering the component is not enough: two
  // separate lists decide whether the admin shell wraps a route and whether
  // it needs a session, and a page in one but not the others renders "Page
  // not found" on a route that exists.
  it('puts the route in every list a rendered admin page has to be in', () => {
    const app = source('App.svelte');
    const adminSet = app.match(/const adminRoutes = new Set\(\[[^\]]*\]\)/s)?.[0] || '';
    expect(adminSet).toContain("'adminApps'");
    const authGuard = app.match(/let authRequired = \$derived\(\s*\[[^\]]*\]/s)?.[0] || '';
    expect(authGuard).toContain("'adminApps'");
  });

  it('is findable from the admin finder', () => {
    expect(source('lib/finderProviders.js')).toContain("href: '/admin/settings/apps'");
  });

  // The section is new, so no retired flat path ever named it: a redirect
  // entry would invent a URL that never existed.
  it('claims no legacy flat path, because there never was one', () => {
    expect(source('lib/adminPanel.js')).not.toContain("apps: '/admin/settings/apps'");
  });
});
