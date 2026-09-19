/**
 * Trust has a scope now
 * (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar,
 * decision 2): the per-patch "direct or suggest" decision must read that
 * patch's own `viewer_trusted` (GET /api/v1/nodes/{slug}), not the
 * signed-in user's quilt-wide flag — a per-patch grant would otherwise be
 * invisible everywhere but the suggest form, which is the one place that is
 * about no patch and keeps reading the flag (SubmitPatch.svelte).
 *
 * Source text only — there is no Svelte render library in this project.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { eventPostingRight } from '../lib/patchWorkspace.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('eventPostingRight takes a per-patch viewerTrusted, not a global flag', () => {
  it('grants direct posting on an unclaimed patch when viewerTrusted is true', () => {
    expect(
      eventPostingRight({ signedIn: true, isUnclaimed: true, viewerTrusted: true })
    ).toBe('direct');
  });

  it('no longer recognises the retired trustedContributor key', () => {
    expect(
      eventPostingRight({
        signedIn: true,
        isUnclaimed: true,
        trustedContributor: true,
        submissionsEnabled: true,
      })
    ).toBe('suggest');
  });
});

describe('PatchEvents.svelte reads the patch’s standing, not the user’s', () => {
  const src = source('pages/PatchEvents.svelte');

  it('feeds eventPostingRight from node.viewer_trusted', () => {
    expect(src).toMatch(/viewerTrusted,\n/);
    expect(src).not.toMatch(/trustedContributor:\s*!!getUser\(\)\?\.trusted_contributor/);
  });

  it('gates bulk upload on the patch’s viewer_trusted, not the retired user flag', () => {
    expect(src).toMatch(/isInstanceAdmin\(\) \|\| viewerTrusted\)/);
  });

  it('no longer imports getUser for this', () => {
    expect(src).not.toMatch(/import \{ isLoggedIn, isAdmin as isInstanceAdmin, getUser \}/);
  });
});

describe('PatchProfileGlimpses.svelte reads the patch’s standing, not the user’s', () => {
  const src = source('components/PatchProfileGlimpses.svelte');

  it('feeds eventPostingRight from node.viewer_trusted', () => {
    expect(src).toMatch(/viewerTrusted: viewerTrusted === true/);
    expect(src).not.toMatch(/trustedContributor:\s*!!getUser\(\)\?\.trusted_contributor/);
  });

  it('no longer imports getUser for this', () => {
    expect(src).not.toMatch(/import \{ isLoggedIn, isAdmin as isInstanceAdmin, getUser \}/);
  });
});

describe('a trusted contributor reaches Event Sources without needing a Settings tab', () => {
  const events = source('pages/PatchEvents.svelte');
  const settings = source('pages/PatchSettings.svelte');

  // workspaceTabs only grants the Settings tab to isAdmin (patch-workspace
  // test.js #6), and a per-patch grant is not patch admin or instance
  // admin standing — so there is no tab to click through at all.
  it('PatchEvents offers a Sources link when viewer_trusted and not this patch’s admin', () => {
    expect(events).toMatch(
      /showSourcesLink = \$derived\(isUnclaimed && viewerTrusted && !patch\.value\.isAdmin\)/
    );
    expect(events).toMatch(/\{#if showSourcesLink\}[\s\S]{0,400}settings\/sources/);
  });

  it('PatchSettings opens for a sources-only trusted contributor, offering exactly Sources', () => {
    expect(settings).toMatch(
      /sourcesOnly = \$derived\(!isAdmin && isUnclaimed && viewerTrusted\)/
    );
    expect(settings).toMatch(/canViewSettings = \$derived\(isAdmin \|\| sourcesOnly\)/);
    expect(settings).toMatch(/\{#if !canViewSettings\}/);
    expect(settings).toMatch(/\{#if sourcesOnly\}[\s\S]{0,400}<PatchSettingsSources \/>/);
    // Every other section stays behind the admin-only branches below it.
    expect(settings).toMatch(/\{:else if activePage === 'info'\}/);
  });

  it('defaults and redirects a sources-only viewer to /sources, never /info', () => {
    expect(settings).toMatch(/defaultSection = \$derived\(sourcesOnly \? 'sources' : 'info'\)/);
    expect(settings).toMatch(/replaceRoute\(`\/patches\/\$\{slug\}\/settings\/\$\{defaultSection\}`\)/);
  });

  it('gives a sources-only viewer a sidebar of one section', () => {
    expect(settings).toMatch(
      /sectionDefs = \$derived\(\s*sourcesOnly\s*\?\s*\[\{ id: 'sources', label: 'Event Sources' \}\]/
    );
  });
});

// The node endpoint sends `viewer_trusted` beside `is_unclaimed`, at the top
// of the payload and not on the node object. The first cut read it off the
// node and every page quietly saw false: a per-patch trusted suggester was
// offered "Suggest an event" on the patch they had just been trusted on.
describe('viewer_trusted is read where the payload puts it', () => {
  it('PatchShell lifts it into the patch context beside isUnclaimed', () => {
    const src = source('components/PatchShell.svelte');
    expect(src).toMatch(/viewerTrusted = data\.viewer_trusted === true/);
    expect(src).toMatch(/isUnclaimed,\n\s*viewerTrusted,\n/);
  });

  it('PatchProfileHead loads it and PatchProfile hands it to the glimpses', () => {
    expect(source('components/PatchProfileHead.svelte')).toMatch(/viewerTrusted: data\.viewer_trusted === true/);
    expect(source('pages/PatchProfile.svelte')).toMatch(/viewerTrusted=\{loaded\?\.viewerTrusted \?\? false\}/);
  });

  it('no page reads viewer_trusted off the node object', () => {
    for (const f of ['pages/PatchEvents.svelte', 'pages/PatchSettings.svelte', 'components/PatchProfileGlimpses.svelte']) {
      expect(source(f)).not.toMatch(/node\?\.viewer_trusted/);
    }
  });
});
