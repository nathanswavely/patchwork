import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// Pointing at a patch previews it (docs/adr/078, decision 7). The rule is
// about input, not about surfaces: where there is a pointer, pointing
// previews and clicking opens; where there is none there is a single
// gesture, and it opens. The quilt and the map therefore behave the same as
// each other on the same hardware, which is the property these guard.
//
// What the gesture hands back changed with docs/adr/094 — the patch's own
// profile, docked, rather than a card about it — so the sheet's mechanics
// below assert against DockedProfile. The rule they belong to is unchanged.
const read = (p) => readFileSync(resolve(__dirname, '..', p), 'utf8');
const home = read('pages/SocialHome.svelte');
const map = read('components/MapView.svelte');
const quilt = read('components/QuiltCanvas.svelte');
const dock = read('components/DockedProfile.svelte');

describe('previewing a patch', () => {
  it('is one fact about the page, not one per surface', () => {
    // A previewed id per surface would let the map and the quilt disagree
    // about what is being pointed at, and the card would have to pick.
    expect(home).toMatch(/let previewing = \$state\(null\)/);
    expect(home).toMatch(/function preview\(patch, fromMap = false\)/);
  });

  it('is reported by both surfaces through the same door', () => {
    expect(map).toMatch(/onPatchHover/);
    expect(quilt).toMatch(/onPatchHover/);
    // Both funnel into the one preview(), rather than each keeping its own.
    expect(home.match(/onPatchHover=\{\(\w+\) => hasPointer && preview\(\w+, true\)\}/g) || [])
      .toHaveLength(2);
  });

  it('never fires where there is no pointer', () => {
    // On a touch screen the single gesture opens the patch. A
    // synthesised mouseover on tap must not preview as well, or a tap would
    // do two things at once.
    expect(home).toMatch(/hasPointer && preview\(/);
    expect(home).toMatch(/hasPointer = \$state\(window\.matchMedia/);
  });

  it('brings a card to the reader only when the pointer is elsewhere', () => {
    // Scrolling the list under a pointer that is *on* the list would move
    // the card out from under it — the hover would chase itself.
    expect(home).toMatch(/if \(!fromMap \|\| !patch\) return;/);
    expect(home).toMatch(/scrollIntoView/);
  });

  it('answers on any tile, badge or no badge', () => {
    // The tip used to be suppressed on tiles that had won a name badge, so
    // the same gesture did different things depending on a label collision
    // the reader cannot see — and the name is the one thing in the tip they
    // already had.
    expect(quilt).not.toMatch(/labeledPatchIds\.has\(tile\.data\.id\)/);
    // A badge is stacked above the svg, so crossing onto one is a mouseleave
    // for the tile under it: the badge reports through the same door, or the
    // tip and the pane's highlight both drop when the pointer finds the name.
    expect(quilt).toMatch(/label\.addEventListener\('pointerenter'[\s\S]{0,160}onPatchHover\(tileData\)/);
    expect(quilt).toMatch(/label\.addEventListener\('pointerleave'[\s\S]{0,120}onPatchHover\(null\)/);
  });

  it('builds no hovering tip where there is nothing to hover with', () => {
    // Touch synthesises the mouseenter on tap and never synthesises the
    // leave, so on a phone the tip arrived with the tap and then stayed over
    // the surface the tap was meant to be reading. Not built rather than
    // guarded at each call site: with no element every showTooltip is a no-op.
    expect(quilt).toMatch(
      /if \(interactive && window\.matchMedia\('\(hover: hover\) and \(pointer: fine\)'\)\.matches\)/,
    );
    expect(quilt).toMatch(/function showTooltip[\s\S]{0,60}if \(!tooltip\) return;/);
  });

  it('shows the tip for a hovering pointer only, and never past its click', () => {
    // The build-time media query is a statement about the device; a finger
    // on a touchscreen laptop contradicts it per gesture, and its
    // pointerenter is the front half of a tap. So each enter asks the
    // pointer, and a click — answered by the docked profile — hides the tip
    // rather than leaving it on top of that answer.
    expect(quilt).toMatch(/function isHoverPointer\(event\)[\s\S]{0,80}pointerType !== 'touch'/);
    expect(quilt).not.toMatch(/\.on\('mouseenter'/);
    expect(quilt).not.toMatch(/addEventListener\('mouseenter'/);
    expect(quilt).toMatch(/\.on\('pointerenter', function\(event\) \{\s*if \(!isHoverPointer\(event\)\) return;/);
    expect(quilt).toMatch(/addEventListener\('pointerenter', \(event\) => \{\s*if \(!isHoverPointer\(event\)\) return;/);
    expect(quilt).toMatch(/\.on\('click', function\(event\) \{\s*holdTooltip\(event\);/);
    expect(quilt).toMatch(/addEventListener\('click', \(event\) => \{\s*holdTooltip\(event\);/);
    // Hidden is not enough: opening the profile relays out the surface under
    // a cursor that has not moved, and Chrome re-enters the tile beneath it.
    // The hold lifts on the pointer's own movement, never on the page's.
    expect(quilt).toMatch(/function tipHeld\(event\)/);
    expect(quilt).toMatch(/if \(!tipHeld\(event\)\) showTooltip\(tile\.data/);
    expect(quilt).toMatch(/if \(!tipHeld\(event\)\) showTooltip\(tileData/);
  });

  it('emphasises the marker without rebuilding the layer', () => {
    // The pointer crosses cards far faster than a marker layer can be torn
    // down and stitched again.
    expect(map).toMatch(/markerById/);
    expect(map).toMatch(/classList\.add\('is-previewing'\)/);
    expect(map).not.toMatch(/hoveredId[\s\S]{0,200}updateMarkers\(\)/);
  });

  it('emphasises with transforms, so nothing reflows', () => {
    // A card that changes size on hover shifts the list; a pin that changes
    // its footprint moves off its own coordinate.
    expect(home).toMatch(/\.patch-card\.previewing \{[^}]*border-color/);
    expect(home).not.toMatch(/\.patch-card\.previewing \{[^}]*(width|height|padding|margin):/);
    // And the scale goes on the child, never the marker: Leaflet positions
    // the marker with a transform, and CSS composes `scale` *with* it rather
    // than beside it — scaling the marker multiplies its translation and
    // throws the pin across the map.
    expect(map).toMatch(/\.patch-marker\.is-previewing svg\) \{[^}]*scale:/);
    expect(map).not.toMatch(/\.patch-marker\.is-previewing\) \{[^}]*scale:/);
  });
});

describe('the docked profile', () => {
  it('is a sheet on the screen, not a card in a box', () => {
    // It rests on the bottom edge and covers the tab bar, the way every
    // other app's sheet does, and carries the surface, the corners and the
    // shadow itself — the profile inside it draws no frame of its own, or
    // the two read as a card sitting in a container.
    expect(dock).toMatch(/\.dock\.sheet \{[^}]*border-radius: 14px 14px 0 0/);
    expect(dock).toMatch(/\.dock\.sheet \{[^}]*box-shadow:/);
    // A tall box translated down to show only its head, rather than a short
    // box that grows: a transform animates for free, and animating height
    // would relayout the profile on every frame of a pull.
    expect(dock).toMatch(/\.dock\.sheet \{[^}]*transform: translateY\(var\(--dock-y\)\)/);
  });

  it('can be pulled down, and a pull that falls short is not a press', () => {
    // The handle is a button too — a drag is not a gesture every reader has,
    // and it is the only control a keyboard would otherwise find nothing
    // behind. Which is why the click has to be swallowed after a real pull:
    // a pointerdown, a move and an up on a button still end in a click, so
    // without this a sheet that sprang back would act anyway.
    expect(dock).toMatch(/onpointerdown=\{dragStart\}/);
    expect(dock).toMatch(/function handlePress\(\)[\s\S]{0,160}if \(dragPulled\)/);
    // Expanded, the pull is downward only: a sheet at full screen has no
    // taller state to promise. At rest it goes both ways — up to full
    // screen, down to dismissed, which is the one pull that leaves
    // (docs/adr/094).
    expect(dock).toMatch(/dragY = expanded \? Math\.max\(0, dy\) : dy/);
    expect(dock).toMatch(/if \(travel < -reach\) expanded = true;/);
    expect(dock).toMatch(/else if \(travel > reach\) onClose\(\);/);
  });

  it('closes by tapping past it, on either surface', () => {
    // A sheet you can only dismiss by finding its one small button is a
    // sheet in the reader's way. The quilt reports a click that landed on no
    // patch through the same name the map already used.
    expect(quilt).toMatch(/onBackgroundClick/);
    expect(map).toMatch(/onBackgroundClick/);
    expect((home.match(/onBackgroundClick=\{backgroundClick\}/g) || [])).toHaveLength(2);
    // And a pan is not a tap on whatever was underneath it — the same
    // distinction the name badges keep about a drag that ends in a click.
    expect(quilt).toMatch(/if \(quiltGestureMoved\) return;/);
    expect(quilt).toMatch(/\.on\('start', \(\) => \{ quiltGestureMoved = false; \}\)/);
    // A filler is padding, not a patch, so it is background too.
    expect(quilt).toMatch(/event\.target\.closest\?\.\('g\.tile'\)/);
  });

  it('stands outside the pane whose chrome it covers', () => {
    // .quilt-pane is its own stacking context at z-index 0, to keep
    // Leaflet's ~1000s off the app's chrome. Inside it, no z-index the sheet
    // could carry would clear the floating buttons — the filter button sat
    // on the card's description. Out here it answers to the root context.
    expect(dock).toMatch(/\.dock\.sheet \{[^}]*position: fixed/);
    // A sibling of both panes, not a child of the quilt pane: it is written
    // after the cards pane, which the quilt pane closes before.
    expect(home.indexOf('class="cards-pane"')).toBeLessThan(home.indexOf("{#if dockedSlug && dockForm === 'sheet'}"));
    const z = dock.match(/\.dock\.sheet \{[^}]*z-index: (\d+)/);
    expect(Number(z?.[1])).toBeGreaterThan(60); // the global bar
  });

  it('needs no label naming the tap that opens the patch', () => {
    // The card at the foot had to say "View patch", because nothing had told
    // a reader that another tap would open it. The sheet *is* the patch, so
    // the label went with the home (docs/adr/094 decision 1).
    expect(home).not.toContain('View patch');
    expect(dock).not.toContain('View patch');
  });

  it("spells out the viewer’s standing in the card's one home", () => {
    // Follower, member and admin are the ladder a reader is here to learn.
    // An icon-only cluster beside the dismiss was tried and rejected: a
    // wrench teaches nobody what an admin is, and "Member" is a status, so
    // as a bare disc it is a button that does nothing when pressed.
    expect(home).toMatch(/\{#snippet relationship\(patch\)\}/);
    expect(home).toMatch(/<span>Manage<\/span>/);
    expect(home).toMatch(/<span>Member<\/span>/);
    // A join request nobody has answered is its own standing, and not
    // membership: me/nodes serves pending rows with role='member', so the
    // card asks the store for them by name rather than reading a role.
    expect(home).toMatch(/<span>Requested<\/span>/);
    expect(home).toContain('getPendingMembershipSlugs');
    // The action row those two had a second, unpressable placement in is
    // gone with the docked home (docs/adr/094).
    expect(home).not.toContain('in-row');
  });
});
