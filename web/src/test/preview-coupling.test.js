import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// Pointing at a patch previews it (docs/adr/078, decision 7). The rule is
// about input, not about surfaces: where there is a pointer, pointing
// previews and clicking opens; where there is none there is a single
// gesture, so the first tap previews into the docked card. The quilt and
// the map therefore behave the same as each other on the same hardware,
// which is the property these guard.
const read = (p) => readFileSync(resolve(__dirname, '..', p), 'utf8');
const home = read('pages/SocialHome.svelte');
const map = read('components/MapView.svelte');
const quilt = read('components/QuiltCanvas.svelte');

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
    // On a touch screen the single gesture belongs to the docked card. A
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
    expect(quilt).toMatch(/label\.addEventListener\('mouseenter'[\s\S]{0,160}onPatchHover\(tileData\)/);
    expect(quilt).toMatch(/label\.addEventListener\('mouseleave'[\s\S]{0,120}onPatchHover\(null\)/);
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

describe('the docked card', () => {
  it('is a sheet on the screen, not a card in a box', () => {
    // It rests on the bottom edge and covers the tab bar, the way every
    // other app's sheet does. The sheet carries the surface, the corners and
    // the shadow; the card inside it gives all four up, or the two frames
    // read as a card sitting in a container.
    expect(home).toMatch(/\.docked-card \{[^}]*bottom: 0;/);
    expect(home).toMatch(/\.docked-card \{[^}]*border-radius: 14px 14px 0 0/);
    expect(home).toMatch(/\.patch-card\.at-foot \{[^}]*border: none/);
    expect(home).toMatch(/\.patch-card\.at-foot \{[^}]*box-shadow: none/);
  });

  it('can be pulled down, and a pull that falls short is not a press', () => {
    // The handle is a button too — a drag is not a gesture every reader has,
    // and it is the only control a keyboard would otherwise find nothing
    // behind. Which is why the click has to be swallowed after a real pull:
    // a pointerdown, a move and an up on a button still end in a click, so
    // without this a sheet that sprang back would dismiss anyway.
    expect(home).toMatch(/onpointerdown=\{dragStart\}/);
    expect(home).toMatch(/function handlePress\(\)[\s\S]{0,160}if \(dragPulled\)/);
    expect(home).toMatch(/dragY = Math\.max\(0, e\.clientY - dragFrom\)/);
    // Downward only: a sheet that rose past its own top would promise a
    // taller state it does not have.
    expect(home).toMatch(/if \(dragY > \(sheet\?\.offsetHeight \|\| \d+\) \/ 3\) docked = null/);
  });

  it('closes by tapping past it, on either surface', () => {
    // A sheet you can only dismiss by finding its one small button is a
    // sheet in the reader's way. The quilt reports a click that landed on no
    // patch through the same name the map already used.
    expect(quilt).toMatch(/onBackgroundClick/);
    expect(map).toMatch(/onBackgroundClick/);
    expect(home.match(/onBackgroundClick=\{\(\) => \{ docked = null; \}\}/g) || [])
      .toHaveLength(2);
    // And a pan is not a tap on whatever was underneath it — the same
    // distinction the name badges keep about a drag that ends in a click.
    expect(quilt).toMatch(/if \(quiltGestureMoved\) return;/);
    expect(quilt).toMatch(/\.on\('start', \(\) => \{ quiltGestureMoved = false; \}\)/);
    // A filler is padding, not a patch, so it is background too.
    expect(quilt).toMatch(/event\.target\.closest\?\.\('g\.tile'\)/);
  });

  it('stands outside the pane whose chrome it covers', () => {
    // .quilt-pane is its own stacking context at z-index 0, to keep
    // Leaflet's ~1000s off the app's chrome. Inside it, no z-index the card
    // could carry would clear the floating buttons — the filter button sat
    // on the card's description. Out here it answers to the root context.
    expect(home).toMatch(/\.docked-card \{[^}]*position: fixed/);
    // A sibling of both panes, not a child of the quilt pane: it is written
    // after the cards pane, which the quilt pane closes before.
    expect(home.indexOf('class="cards-pane"')).toBeLessThan(home.indexOf('{#if docked}'));
    const z = home.match(/\.docked-card \{[^}]*z-index: (\d+)/);
    expect(Number(z?.[1])).toBeGreaterThan(60); // the global bar
  });

  it('names the tap that opens the patch', () => {
    // The card is reached by a tap on the quilt, so nothing has told the
    // reader that another tap opens the patch. In the pane it sits in a grid
    // where tapping a card is the convention it already is.
    expect(home).toMatch(/\{#if atFoot\}[\s\S]{0,400}View patch/);
    expect(home).toMatch(/@render patchCard\(docked, true\)/);
  });

  it('spells out the viewer’s standing in both homes', () => {
    // Follower, member and admin are the ladder a reader is here to learn.
    // An icon-only cluster beside the dismiss was tried and rejected: a
    // wrench teaches nobody what an admin is, and "Member" is a status, so
    // as a bare disc it is a button that does nothing when pressed.
    expect(home).toMatch(/\{#snippet relationship\(patch, inRow = false\)\}/);
    expect(home).toMatch(/<span>Manage<\/span>/);
    expect(home).toMatch(/<span>Member<\/span>/);
    // And in the row it must not look pressable, since it cannot be pressed.
    expect(home).toMatch(/\.card-member-chip\.in-row \{[^}]*border: none/);
  });
});
