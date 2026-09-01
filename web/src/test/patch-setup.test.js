/**
 * docs/adr/039 — unclaimed patches carry no governance; claims complete
 * through setup.
 *
 * There is no Svelte render library in this project (see router.test.js
 * and scope-routing.test.js for the established pattern), so component
 * wiring is asserted against source text alongside functional tests of
 * the pure/router pieces that can run standalone.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

let routerModule;

describe('the /patches/:slug/setup route resolves correctly', () => {
  beforeEach(async () => {
    vi.resetModules();
    delete window.location;
    window.location = new URL('http://localhost/');
    window.location.pathname = '/';
    window.location.search = '';
    window.history.pushState = vi.fn();
    window.history.replaceState = vi.fn();
    routerModule = await import('../stores/router.svelte.js');
  });

  it('matches ahead of the bare patch profile and the claim route', () => {
    routerModule.addRoute('/patches/new', 'patchNew');
    routerModule.addRoute('/patches/:slug/claim', 'claimPatch');
    routerModule.addRoute('/patches/:slug/setup', 'patchSetup');
    routerModule.addRoute('/patches/:slug', 'patchProfile');

    routerModule.navigate('/patches/the-selvage/setup');
    const match = routerModule.matchRoute();
    expect(match.name).toBe('patchSetup');
    expect(match.params.slug).toBe('the-selvage');
  });

  it('leaves the claim route and bare profile alone', () => {
    routerModule.addRoute('/patches/:slug/claim', 'claimPatch');
    routerModule.addRoute('/patches/:slug/setup', 'patchSetup');
    routerModule.addRoute('/patches/:slug', 'patchProfile');

    routerModule.navigate('/patches/the-selvage/claim');
    expect(routerModule.matchRoute().name).toBe('claimPatch');

    routerModule.navigate('/patches/the-selvage');
    expect(routerModule.matchRoute().name).toBe('patchProfile');
  });
});

describe('App wires the setup route and guards it', () => {
  const src = source('App.svelte');

  it('registers the route and imports the page', () => {
    expect(src).toContain("addRoute('/patches/:slug/setup', 'patchSetup')");
    expect(src).toContain("import PatchSetup from './pages/PatchSetup.svelte'");
    expect(src).toMatch(/routeName === 'patchSetup'[\s\S]{0,40}<PatchSetup slug={routeParams\.slug} \/>/);
  });

  it('requires auth, same as the claim page', () => {
    const authRequired = src.match(/let authRequired = \$derived\(([\s\S]*?)\);/);
    expect(authRequired, 'authRequired block not found').toBeTruthy();
    expect(authRequired[1]).toContain('claimPatch');
    expect(authRequired[1]).toContain('patchSetup');
  });

  it('is exempt from the zero-memberships onboarding redirect', () => {
    // Assert the exemption, not the whole literal: the list grows as new
    // routes a fresh account can deliberately reach are added (docs/adr/075
    // added 'discover'), and pinning every member made an unrelated change
    // fail here.
    const exempt = src.match(/if \(!\[([^\]]*)\]\.includes\(routeName\)/);
    expect(exempt, 'onboarding-redirect exempt list not found').toBeTruthy();
    expect(exempt[1]).toContain("'patchSetup'");
  });

  it('gives claimPatch its own workspace tab id, not governance', () => {
    // Folding claimPatch into 'governance' would make PatchShell's
    // unclaimed-governance redirect (below) bounce the claim page itself.
    expect(src).toMatch(/if \(name === 'claimPatch'\) return 'claim';/);
  });
});

describe('PatchShell redirects unclaimed patches off any governance route', () => {
  it('never lets an unclaimed patch render a governance-tagged route', () => {
    const src = source('components/PatchShell.svelte');
    expect(src).toMatch(/isUnclaimed && activeTab === 'governance'/);
    expect(src).toMatch(/navigate\(`\$\{basePath\}\/events`\)/);
  });
});

describe('GovernanceList bounces unclaimed patches instead of showing an empty list', () => {
  it('redirects before loading docs', () => {
    const src = source('pages/GovernanceList.svelte');
    expect(src).toMatch(/if \(slug && isUnclaimed\)[\s\S]*?navigate\(`\/patches\/\$\{slug\}\/events`\)/);
  });
});

describe('PatchProfile treats unclaimed governance/lining as absent, not empty', () => {
  const src = source('pages/PatchProfile.svelte');

  // The page became one glimpse per room (docs/adr/042); absence is now
  // expressed by the glimpse's visibility derivation rather than by an
  // {#if} per section, but the rule is unchanged.
  it('never fetches proposals, charters, or members for an unclaimed patch', () => {
    expect(src).toMatch(/const wantGovernance = !isUnclaimed/);
    expect(src).toMatch(/isUnclaimed \? Promise\.resolve\(\{ items: \[\] \}\) : api\(`nodes\/\$\{slug\}\/members/);
  });

  it('gates the governance and members glimpses, and the amended-lining badge, on claim state', () => {
    expect(src).toMatch(/\{#if !isUnclaimed && liningStatus === 'diverged'\}/);
    expect(src).toMatch(/canSeeGovernance = \$derived\(\s*!isUnclaimed/);
    expect(src).toMatch(/showGovernance = \$derived\(\s*canSeeGovernance/);
    expect(src).toMatch(/showMembers = \$derived\(!isUnclaimed/);
  });
});

describe('PatchForm reuses one component for creation and setup (docs/adr/039)', () => {
  const src = source('pages/PatchForm.svelte');

  it('accepts setup-mode props and parameterizes the copy', () => {
    expect(src).toMatch(/mode = 'create'/);
    expect(src).toContain('Set up your patch');
    expect(src).toContain('Finish setup');
  });

  it('shows the slug read-only with the setup hint, never an editable field', () => {
    expect(src).toContain("This is the patch's existing address");
    expect(src).toMatch(/<input id="setup-slug" type="text" value={setupSlug} disabled readonly \/>/);
  });

  it('shows the approval expiry', () => {
    expect(src).toContain('Approval expires');
  });

  it('posts to the claim setup endpoint before patching the node', () => {
    expect(src).toMatch(/api\(`claims\/\$\{claimId\}\/setup`, \{ method: 'POST', body: \{ template \} \}\)/);
    expect(src).toMatch(/api\(`nodes\/\$\{setupSlug\}`, \{[\s\S]*?method: 'PATCH'/);
  });

  it('handles the expired (410) and no-longer-claimable (409) responses', () => {
    expect(src).toMatch(/e\.status === 410/);
    expect(src).toMatch(/e\.status === 409/);
    expect(src).toContain('This patch is no longer claimable.');
  });

  it('never shows the create-only submission suggestion in setup mode', () => {
    const setupBranch = src.match(/\{#if mode === 'setup'\}[\s\S]*?\{:else\}([\s\S]*?)\{\/if\}\s*\n\s*<form/);
    expect(setupBranch, 'heading branch not found').toBeTruthy();
    expect(setupBranch[1]).toContain('Suggest a patch');
  });

  it('does not re-randomize appearance for setup — it seeds from the listing', () => {
    expect(src).toMatch(/mode !== 'setup' \|\| !initial/);
    expect(src).toMatch(/paletteForPatch\(initial\.id, ap\)/);
  });
});

describe('PatchSetup guards the route on an approved claim', () => {
  const src = source('pages/PatchSetup.svelte');

  it('fetches claims/mine and redirects away without an approved claim', () => {
    expect(src).toContain('claims/mine');
    expect(src).toMatch(/if \(!c \|\| c\.status !== 'approved'\)/);
    expect(src).toMatch(/navigate\(`\/patches\/\$\{slug\}`\)/);
  });

  it('passes claim id and expiry through to PatchForm', () => {
    expect(src).toContain('claimId={claim.id}');
    expect(src).toContain('expiresAt={claim.setup_expires_at}');
    expect(src).toContain('mode="setup"');
  });
});

describe('ClaimPatch surfaces the approved state and routes verify to setup', () => {
  const src = source('pages/ClaimPatch.svelte');

  it('shows an approved claim distinctly from a pending one', () => {
    expect(src).toMatch(/claim && claim\.status === 'approved'/);
    expect(src).toContain('Claim approved');
    expect(src).toMatch(/href="\/patches\/\{slug\}\/setup"/);
  });

  it('shows the approval expiry on the approved card', () => {
    expect(src).toMatch(/claim\.setup_expires_at/);
  });

  it('sends a successful verification to setup, not straight to the profile', () => {
    expect(src).toMatch(/navigate\(result\.setup_required === false \? `\/patches\/\$\{slug\}` : `\/patches\/\$\{slug\}\/setup`\)/);
  });
});
