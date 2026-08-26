/**
 * The governance page (docs/adr/068) is the second explaining surface in
 * the app, admitted past docs/adr/040's "no help center" rule on two
 * conditions. This file is the second one: its claims are pinned to the
 * enumerations in code, so the prose cannot silently fall behind the
 * behavior it describes.
 *
 * The pinning is why the page's spine is the axes a patch configures
 * rather than a tour of features — axes are enumerable in source and
 * features are not. Add a fifth decision method or a fourth leadership
 * model and these tests go red until the page covers it.
 *
 * Known limit, stated in the ADR: this catches *added* values, not changed
 * behavior. If elections moved from approval voting to ranked choice the
 * enumerations would be unchanged and these tests would stay green over
 * stale prose. Nothing cheap catches that.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const page = source('pages/Governance.svelte');

/** The values a patch can actually be set to, read from the editor that sets them. */
function decisionMethods() {
  const block = source('components/StructuredRulesEditor.svelte')
    .match(/const DECISION_OPTIONS = \[([\s\S]*?)\];/);
  expect(block, 'DECISION_OPTIONS moved or was renamed — repoint this test').not.toBeNull();
  return [...block[1].matchAll(/value: '([^']+)'/g)].map((m) => m[1]);
}

/** The leadership models, read from the overview that describes them. */
function leadershipModels() {
  const block = source('components/GovernanceOverview.svelte')
    .match(/const models = \{([\s\S]*?)\n    \};/);
  expect(block, 'the leadership models map moved or was renamed — repoint this test').not.toBeNull();
  return [...block[1].matchAll(/^\s{6}(\w+):/gm)].map((m) => m[1]);
}

describe('The governance page is pinned to the axes it describes', () => {
  it('names every decision method a patch can be set to', () => {
    const methods = decisionMethods();
    // Guard the guard: an empty parse would make every assertion below vacuous.
    expect(methods.length).toBeGreaterThanOrEqual(4);
    for (const method of methods) {
      // Word-boundary, so "supermajority" never satisfies "majority" for us.
      expect(page, `the page never mentions the "${method}" decision method`)
        .toMatch(new RegExp(`\\b${method}`, 'i'));
    }
  });

  it('names every leadership model a patch can be set to', () => {
    const models = leadershipModels();
    expect(models.length).toBeGreaterThanOrEqual(3);
    for (const model of models) {
      expect(page, `the page never mentions the "${model}" leadership model`)
        .toMatch(new RegExp(`\\b${model}`, 'i'));
    }
  });

  it('covers deciding somewhere other than Patchwork (docs/adr/052)', () => {
    // The third venue is not in a labelled option list, so it is pinned by
    // name: a patch that decides elsewhere records attestations instead.
    expect(page).toMatch(/\belsewhere\b/i);
    expect(page).toMatch(/\battestation\b/i);
  });

  it('says what the platform does not do, never narrating unenforced behavior (docs/adr/049)', () => {
    // Two claims the ADR treats as load-bearing: Patchwork cannot verify an
    // attestation, and a lapsed term never unseats anyone.
    expect(page).toMatch(/cannot check/i);
    expect(page).toMatch(/never removes anyone/i);
  });
});

describe('The governance route is public, like About and the lining', () => {
  const app = source('App.svelte');

  it('registers /governance and renders it in the social shell', () => {
    expect(app).toContain("addRoute('/governance', 'governance')");
    expect(app).toMatch(/routeName === 'governance'\}[\s\S]{0,40}<Governance \/>/);
    expect(app).toContain("import Governance from './pages/Governance.svelte'");
  });

  it('needs no auth, and never enters the patch shell', () => {
    const authRequired = app.match(/let authRequired = \$derived\(\s*\[([\s\S]*?)\]\.includes\(routeName\)\s*\)/);
    expect(authRequired).not.toBeNull();
    expect(authRequired[1]).not.toContain("'governance'");
    // 'governanceHub' is the patch workspace; bare 'governance' must stay out
    // of the shell set or the public page would demand a slug.
    const shellRoutes = app.match(/const patchShellRoutes = new Set\(\[([\s\S]*?)\]\);/);
    expect(shellRoutes).not.toBeNull();
    expect(shellRoutes[1]).not.toMatch(/'governance'/);
  });

  it('is reachable from About, which is where the reader is sent from', () => {
    expect(source('pages/About.svelte')).toContain('href="/governance"');
  });
});
