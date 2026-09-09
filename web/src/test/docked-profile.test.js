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
  it('mounts the glimpses at rest but defers their fetches to the pull', () => {
    // At rest the first section starts under the buttons and the fold cuts
    // it — the cut is what says there is more — while a tap on the quilt
    // stays one request: the rooms are asked only once the sheet is up.
    const src = dock();
    expect(src).toMatch(/glimpsesActive = \$derived\(!isSheet \|\| expanded\)/);
    expect(src).toMatch(/active=\{glimpsesActive\}/);
    expect(src).not.toMatch(/\{#if showGlimpses\}/);
    const glimpses = source('components/PatchProfileGlimpses.svelte');
    expect(glimpses).toMatch(/if \(s && active\) loadActivity\(gov\);/);
    // And an empty state waits for the answer: "no events" about a patch
    // nobody has asked yet is a lie at the fold.
    expect(glimpses).not.toMatch(/\{:else\}\s*<p class="glimpse-empty/);
    expect(glimpses).toMatch(/\{:else if loaded\}\s*<p class="glimpse-empty/);
  });

  it('rests at the head plus a peek of what follows', () => {
    const src = dock();
    expect(src).toMatch(/const PEEK = 96;/);
    expect(src).toMatch(/sheet\.getBoundingClientRect\(\)\.top\) \+ PEEK;/);
    // A tap on the cut-off content pulls the sheet up; a link in it is
    // still a link.
    expect(src).toMatch(/if \(isSheet && !expanded && !e\.target\.closest\('a, button'\)\) expanded = true;/);
  });

  it('lets the surface pick the form, since it measures the room', () => {
    expect(home()).toMatch(/dockForm = \$derived\(winW <= 768 \? 'sheet' : 'panel'\)/);
  });

  it("puts the panel in the cards pane's slot, in the list's place", () => {
    // The profile takes the list's box — same slot, same width — and the
    // list is back when it is dismissed. Nothing else on the surface moves.
    const src = home();
    expect(src).toMatch(/<div class="cards-pane"[^>]*>\s*\{#if dockedSlug && dockForm === 'panel'\}\s*<DockedProfile/);
    expect(src).toMatch(/\{#if dockedSlug && dockForm === 'sheet'\}\s*<DockedProfile/);
    // A card in the pane, not a box floating over it.
    expect(dock()).toMatch(/\.dock\.panel \{\s*position: relative;\s*flex: 1;/);
    expect(dock()).not.toMatch(/\.dock\.panel \{[^}]*width: 45%/);
    // And the clicked card grows into it.
    expect(src).toMatch(/dockOrigin = rect \? \{ left: rect\.left/);
    expect(src).toMatch(/origin=\{dockOrigin\}/);
    expect(dock()).toMatch(/el\.animate\(/);
  });

  it('leaves Escape to a dialog open above it', () => {
    // Both listen on the window; one Escape used to close the join sheet
    // and the profile it was joining from together.
    expect(dock()).toMatch(/if \(document\.querySelector\('\[role="dialog"\]\[aria-modal="true"\]'\)\) return;\s*onClose\(\);/);
  });

  it('keeps the shell on the surface while a profile is docked', () => {
    // A shell told "patchProfile" leaves quilt mode — bordered bar, page
    // gutters, no view pill, no chips — around a quilt that is still there.
    expect(app()).toMatch(/<SocialShell\s+routeName=\{docked \? docked\.routeName : routeName\}\s+quiltScope=\{docked \? docked\.quiltScope : quiltScope\}/);
  });

  it("measures the rest height to the head's foot, not the head's height", () => {
    // The head's height alone left the 26px handle above it unaccounted
    // for, which put the relationship row below the fold.
    expect(dock()).toMatch(/head\.getBoundingClientRect\(\)\.bottom - sheet\.getBoundingClientRect\(\)\.top/);
    // The handle lies over the cover, out of the flow, so it cannot
    // reintroduce the 26px the measurement above was written to catch.
    expect(dock()).toMatch(/\.dock-handle \{\s*position: absolute;/);
  });

  it('lays the head out for a sheet, and the container owns the gap below it', () => {
    // The head is the same rendering in every container; the sheet is a
    // layout the container asks for — cover to the sheet's edges, text left,
    // the relationship row's controls filling the width for a thumb.
    expect(dock()).toMatch(/layout="docked"/);
    const headSrc = head();
    expect(headSrc).toMatch(/layout = 'page'/);
    expect(headSrc).toMatch(/class="profile-head" class:docked=\{layout === 'docked'\}/);
    expect(headSrc).toMatch(/\.profile-head\.docked \.profile-cover \{\s*margin: 0 calc\(-1 \* var\(--pw-gutter\)\)/);
    // At rest the sheet ends at the row, so its standing menu opens upward
    // — the sheet's rule, since the panel has room below.
    expect(dock()).toMatch(/\.dock\.sheet \.dock-head :global\(\.standing-menu\) \{\s*top: auto;\s*bottom:/);
    // The head ends at its row (no bottom margin), so the rule under it
    // would sit on the buttons unless each container sets the gap.
    expect(headSrc).toMatch(/\.profile-actions \{\s*display: flex;\s*justify-content: center;\s*\}/);
    expect(dock()).toMatch(/\.dock-glimpses \{\s*margin-top: 1\.5rem;/);
    expect(source('pages/PatchProfile.svelte')).toMatch(/\.profile-glimpses \{\s*margin-top: 1\.5rem;/);
  });

  it('is never modal — no scrim over a live canvas', () => {
    const src = dock();
    expect(src).not.toContain('sidepanel-backdrop');
    expect(src).not.toMatch(/--color-scrim/);
    expect(src).toMatch(/e\.key !== 'Escape'/);
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
