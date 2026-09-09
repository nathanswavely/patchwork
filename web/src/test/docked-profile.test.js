/**
 * The docked profile (CONTEXT.md "Docked profile", docs/adr/094): a
 * discovery surface hands back the patch's own profile, docked over it, at
 * the patch's own address.
 *
 * Source text, as everything on the frontend is here — there is no Svelte
 * render library in this project (see patch-profile-window.test.js). What
 * these can catch is a rule quietly deleted; the geometry and the gestures
 * were read in a browser, which is the only thing that can see them.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const dock = () => source('components/DockedProfile.svelte');
const app = () => source('App.svelte');
const home = () => source('pages/SocialHome.svelte');
const shell = () => source('components/SocialShell.svelte');
const head = () => source('components/PatchProfileHead.svelte');

describe('what a surface hands back', () => {
  it('is the profile itself, not a card about the patch', () => {
    const src = dock();
    expect(src).toContain('PatchProfileHead');
    expect(src).toContain('PatchProfileGlimpses');
  });

  it('leaves the patch card one home — the pane', () => {
    // The docked card retires with its second home: there is no second tap
    // to teach when the sheet is the patch, so the action row and "View
    // patch" go with it (docs/adr/094 decision 1).
    const src = home();
    expect(src).not.toContain('patchCard(docked');
    expect(src).not.toContain('View patch');
    expect(src).not.toMatch(/class:at-foot/);
  });

  it("docks a remote patch's remote patch card instead", () => {
    // The container is chosen by the room and its occupant by the address,
    // so the one patch that can have no local profile answers the same
    // gesture (docs/adr/024, docs/adr/094 decision 7).
    const src = dock();
    expect(src).toContain('RemotePatch');
    expect(src).toMatch(/\{#if host\}/);
  });
});

describe('one address, two containers', () => {
  it('keeps the surface mounted under a docked profile', () => {
    // The router matches one route and App mounts one page, so without this
    // /patches/:slug unmounts the canvas — the zoom and pan the dock exists
    // to keep (docs/adr/094 decision 3).
    const src = app();
    expect(src).toMatch(/\{:else if isSocialHome \|\| docked\}/);
    expect(src).toContain('dockedSlug={docked ? routeParams.slug : null}');
  });

  it('remembers the surface only while a profile is docked over it', () => {
    // A profile reached from a third page has no surface to keep, so it is
    // the page — and a cold link never had one recorded.
    const src = app();
    expect(src).toMatch(/dockableRoutes = new Set\(\['patchProfile', 'remotePatch'\]\)/);
    expect(src).toMatch(/else if \(!dockableRoutes\.has\(name\)\) \{\s*dockedOver = null;/);
  });

  it('dismisses by going back, because opening pushed', () => {
    expect(app()).toMatch(/function closeDock\(\)\s*\{\s*history\.back\(\);/);
  });

  it('replaces rather than pushes when the docked patch changes', () => {
    // One dismiss should not walk back through every patch a reader
    // glanced at.
    expect(home()).toMatch(/if \(dockedSlug\) replaceRoute\(path\);\s*else navigate\(path\);/);
  });
});

describe("two heights on a phone, none in the pane's slot", () => {
  it('defers the glimpses to the pull, and only on a sheet', () => {
    const src = dock();
    expect(src).toMatch(/showGlimpses = \$derived\(!isSheet \|\| expanded\)/);
  });

  it('lets the surface pick the form, since it measures the room', () => {
    expect(home()).toMatch(/dockForm = \$derived\(winW <= 768 \? 'sheet' : 'panel'\)/);
  });

  it("measures the rest height to the head's foot, not the head's height", () => {
    // The head's height alone left the 26px handle above it unaccounted
    // for, which put the relationship row below the fold.
    expect(dock()).toMatch(/head\.getBoundingClientRect\(\)\.bottom - sheet\.getBoundingClientRect\(\)\.top/);
  });

  it('is never modal — no scrim over a live canvas', () => {
    const src = dock();
    expect(src).not.toContain('sidepanel-backdrop');
    expect(src).not.toMatch(/--color-scrim/);
    expect(src).toMatch(/e\.key === 'Escape'/);
  });
});

describe('the seed paints the first frame and never guesses', () => {
  it('takes the row the surface already has', () => {
    expect(home()).toMatch(/dockedSeed = \$derived\.by/);
    expect(dock()).toContain('{seed}');
    expect(head()).toMatch(/let \{ slug = '', seed = null/);
  });

  it('withholds the upcoming count until the payload carries it', () => {
    // The tree carries the all-time `event_count`, and the two are never
    // labelled with each other's word (CONTEXT.md "Upcoming events").
    const src = head();
    expect(src).toMatch(/node\.upcoming_event_count !== undefined/);
    expect(src).not.toMatch(/event_count \|\| 0\} Upcoming/);
  });

  it('reads standing from the store until then, pending included', () => {
    // me/nodes sends active and pending rows and never banned ones, so a
    // ban can only come from the payload (docs/adr/088).
    const src = head();
    expect(src).toContain('getMembershipRoles');
    expect(src).toMatch(/isBanned = \$derived\(loaded \? loaded\.isBanned : false\)/);
  });
});

describe('the canvas chrome steps aside for a sheet', () => {
  it('takes the whole floating row with it, not just the view pill', () => {
    // SocialHome already stepped the pill aside; the two FABs in the shell
    // only avoided the collision by being painted over (docs/adr/094).
    const src = shell();
    expect(src).toMatch(/dockOpen = false,/);
    expect(src).toMatch(/isQuiltRoute && label\?\.published && !dockOpen/);
    expect(src).toMatch(/\{#if isQuiltRoute && !dockOpen\}/);
    expect(home()).toMatch(/class:hidden=\{!!dockedSlug && dockForm === 'sheet'\}/);
  });

  it('closes a sheet whose toggle is about to be hidden', () => {
    const src = shell();
    expect(src).toMatch(/filterSheetOpen && isQuiltRoute && !dockOpen/);
    expect(src).toMatch(/labelSheetOpen && label\?\.published && !dockOpen/);
  });

  it('dismisses on a tap behind, and only where there is surface to tap', () => {
    // At rest a phone's sheet leaves the canvas showing (docs/adr/078); in
    // the pane's slot the canvas is live, so a pan that begins with a click
    // must not throw away what is being read.
    expect(home()).toMatch(/if \(dockedSlug && dockForm === 'sheet'\) onDockClose\(\)/);
  });
});
