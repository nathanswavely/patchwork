import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { patchSettingsSections } from '../lib/patchWorkspace.js';
import { adminTabs, legacyAdminPath } from '../lib/adminPanel.js';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const intro = shipped('../pages/PatchSettingsIntro.svelte');
const outreach = shipped('../pages/AdminOutreach.svelte');
const claim = shipped('../pages/ClaimPatch.svelte');
const settings = shipped('../pages/PatchSettings.svelte');
const app = shipped('../App.svelte');

// docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md: an intro
// is sent by a person, after seeing it, to an address a person chose.

describe('where the intro lives', () => {
  it('is a section of an unclaimed patch only', () => {
    expect(patchSettingsSections({ isUnclaimed: true }).map((s) => s.id)).toContain('intro');
    expect(patchSettingsSections({ isUnclaimed: false }).map((s) => s.id)).not.toContain('intro');
  });

  // Decision 2: a trusted contributor may enter the address and nothing
  // more, so their two-section sidebar carries Intro beside Sources.
  it('reaches a trusted contributor whose grant covers the patch', () => {
    expect(settings).toContain("[{ id: 'sources', label: 'Event Sources' }, { id: 'intro', label: 'Intro' }]");
    expect(settings).toContain("{#if sourcesOnly && activePage === 'intro'}");
  });

  it('has an admin page for the backlog and the template', () => {
    const s = adminTabs().find((t) => t.id === 'settings');
    expect(s.sections.find((x) => x.id === 'outreach').href).toBe('/admin/settings/outreach');
    expect(legacyAdminPath('outreach')).toBe('/admin/settings/outreach');
    expect(app).toContain("addRoute('/admin/settings/outreach', 'adminOutreach');");
    expect(app).toContain("addRoute('/patches/:slug/settings/intro', 'patchSettingsIntro');");
  });
});

describe('a person sends, after seeing it (decisions 1, 2, 5)', () => {
  it('only an instance admin can send; the address field is for anyone the grant reaches', () => {
    expect(intro).toContain("let canSend = $derived(getUser()?.role === 'admin');");
    expect(intro).toContain('{#if canSend}');
  });

  it('renders the whole message before a send button exists', () => {
    expect(intro).toContain("api(`nodes/${slug}/intro/preview`");
    expect(intro).toContain('bind:value={subject}');
    expect(intro).toContain('bind:value={body}');
    expect(intro).toContain("onclick={() => send('smtp')}");
  });

  // Decision 4: the probe picks, the admin may overrule.
  it('lets the admin pick a different calendar paragraph', () => {
    expect(intro).toContain('onchange={() => runPreview(situation)}');
    expect(intro).toContain('Picked because {preview.classification?.reason}.');
  });

  it('asks before a second send to the same address', () => {
    expect(intro).toContain("if (resend && !confirm('An intro already went to this address. Send another one?')) return;");
  });

  // Decision 8: no relay, still the product.
  it('offers copy-and-record where there is no relay', () => {
    expect(intro).toContain("onclick={() => send('copied')}");
    expect(intro).toContain('This quilt has no email relay');
  });
});

describe('the admin page (decision 9)', () => {
  it('carries the switch and the template', () => {
    expect(outreach).toContain('setEnabled(e.currentTarget.checked)');
    expect(outreach).toContain("api('admin/outreach/template', { method: 'DELETE' })");
  });

  // Rejected alternative: no send-all. Each row leads to one patch's send.
  it('has no send-all button', () => {
    expect(outreach).not.toMatch(/send all/i);
    expect(outreach).toContain('navigate(`/patches/${slug}/settings/intro`)');
  });
});

describe('the claim shortcut (decision 6)', () => {
  it('reads the token off the link and claims with it', () => {
    expect(claim).toContain("let introToken = $derived(getQuery().get('intro') || '');");
    expect(claim).toContain('body: { intro_token: introToken }');
  });

  // It does not sign anyone in. The claim route is auth-gated, and the
  // gate's Log In carries the whole path, query included, so the token
  // rides through sign-in and the page needs no sign-in branch of its own.
  it('relies on the auth gate to carry the link through sign-in', () => {
    expect(claim).not.toContain('isLoggedIn');
    expect(app).toMatch(/authRequired = \$derived\(\s*\[[^\]]*'claimPatch'/);
    expect(app).toContain("const target = getPath() + (qs ? `?${qs}` : '');");
  });
});
