import { describe, it, expect, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { api } from '../lib/api.js';
import {
  loadTags,
  getRankedTags,
  areTagsLoaded,
} from '../stores/quilt.svelte.js';

vi.mock('../lib/api.js', () => ({ api: vi.fn() }));

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

// The suggestion engine (docs/adr/075): what this quilt wears, ranked by how
// many patches wear it. Identical for every viewer, anonymous included.
//
// These run against one module instance, in order, deliberately. Isolating
// them with vi.resetModules() re-transforms the store's whole graph — it
// reaches phosphor's icon set through patchIcons — and blew the hook timeout
// on every case. The first test is the one that needs a pristine module, so
// it goes first and the rest build on it.
describe('the suggestion engine', () => {
  it('does not claim the vocabulary is empty before it has loaded', () => {
    // An empty vocabulary means two different things either side of the
    // round-trip. Discovery mode branches on it, and read the pre-load
    // emptiness as "this quilt has no question to ask" — every visitor
    // shipped straight past the question to the answer.
    expect(areTagsLoaded()).toBe(false);
    expect(getRankedTags()).toEqual([]);
  });

  it('ranks tags by how many patches wear them, then alphabetically', async () => {
    api.mockResolvedValue({
      items: [
        { name: 'zine', node_count: 2 },
        { name: 'music', node_count: 9 },
        { name: 'craft', node_count: 2 },
        { name: 'unworn', node_count: 0 },
      ],
    });
    await loadTags();
    expect(areTagsLoaded()).toBe(true);
    expect(getRankedTags()).toEqual(['music', 'craft', 'zine', 'unworn']);
  });

  it('settles the loaded flag even when the request fails', async () => {
    // Otherwise a quilt whose tags endpoint is down shows a question it can
    // never populate, forever.
    api.mockRejectedValue(new Error('offline'));
    await loadTags();
    expect(areTagsLoaded()).toBe(true);
    expect(getRankedTags()).toEqual([]);
  });
});

// Welcome is two things fused, and they split (docs/adr/075).
describe('the Welcome split', () => {
  it('leaves orientation behind and nothing else', () => {
    const src = source('pages/Welcome.svelte');
    expect(src).toContain("What's expected here");
    // The interests question and the patch list it answered with are gone.
    expect(src).not.toContain('selectedInterests');
    expect(src).not.toContain('matchingPatches');
    expect(src).not.toContain('step-dots');
  });

  it('hands off to discovery mode rather than containing it', () => {
    const src = source('pages/Welcome.svelte');
    expect(src).toContain("navigate('/discover')");
  });

  it('dismisses onboarding on the way out, both doors', () => {
    // The zero-membership redirect would otherwise pull the person straight
    // back to /welcome from the surface /welcome just sent them to.
    const src = source('pages/Welcome.svelte');
    const handoff = src.slice(src.indexOf('function handleContinue'));
    expect(handoff).toContain('dismissOnboarding');
    const skip = src.slice(src.indexOf('function handleSkip'));
    expect(skip).toContain('dismissOnboarding');
  });

  it('keeps /welcome auth-gated and gives /discover its own public route', () => {
    // docs/adr/040 is not amended: its gating was always about orientation.
    const src = source('App.svelte');
    expect(src).toContain("addRoute('/discover', 'discover')");
    expect(src).toContain("routeName === 'welcome' && isAuthChecked() && !isLoggedIn()");
  });

  it('exempts discovery mode from the onboarding redirect', () => {
    const src = source('App.svelte');
    const line = src.split('\n').find((l) => l.includes("'signupComplete', 'claimPatch'"));
    expect(line).toContain("'discover'");
  });
});

// The surface itself (docs/adr/075).
describe('discovery mode', () => {
  it('asks one question before showing an answer', () => {
    const src = source('pages/Discover.svelte');
    expect(src).toContain('What are you drawn to?');
    expect(src).toContain("phase = $state('ask')");
  });

  it('carries no quilt canvas — that is the show-everything gesture', () => {
    const src = source('pages/Discover.svelte');
    expect(src).not.toContain('QuiltCanvas');
    expect(src).not.toContain('MapView');
  });

  it('orders the answer by what is happening, not by a count of nothing', () => {
    const src = source('pages/Discover.svelte');
    expect(src).toContain('byWhatIsHappening');
    expect(src).toContain('nextEventByNode');
    // Omitting `from` means upcoming (CLAUDE.md); an unbounded list would be
    // the quilt's oldest events.
    expect(src).toContain("api('events?limit=200')");
    expect(src).not.toContain('member_count + follower_count');
  });

  it('sends anonymous visitors to sign-in rather than failing the follow', () => {
    const src = source('pages/Discover.svelte');
    const fn = src.slice(src.indexOf('async function toggleFollow'));
    expect(fn.slice(0, 400)).toContain("navigate('/login')");
  });

  it('stores nothing about who saw what', () => {
    // The flow records nothing about the visit: no localStorage, and none of
    // the onboarding dismissal machinery /welcome uses. Assert the machinery
    // rather than the word — the bulletin's offer (docs/adr/076) explains in
    // a comment why it lives here and not in onboarding, and prose about a
    // module is not a use of it.
    //
    // The bulletin preference the offer writes is not a counter-example: it
    // is a setting the person chose, not a record of what they were shown.
    const src = source('pages/Discover.svelte');
    expect(src).not.toContain('localStorage');
    expect(src).not.toContain("lib/onboarding.js");
    expect(src).not.toContain('dismissOnboarding');
    expect(src).not.toContain('isOnboardingDismissed');
  });

  it('has a standing door in the rail', () => {
    // Its absence was why nobody could return to the flow.
    const src = source('components/SocialShell.svelte');
    expect(src).toContain("id: 'discover'");
    expect(src).toContain("href: '/discover'");
  });
});

// The bulletin's offer (docs/adr/076) lives at the end of the flow.
describe('the bulletin offer', () => {
  it('is two named choices, not a checkbox', () => {
    // docs/adr/040's register binds even though a mail preference is a
    // setting rather than a signature: a pre-checked box would be
    // default-on wearing opt-in's clothes.
    const src = source('pages/Discover.svelte');
    expect(src).toContain("Tell me who's new");
    expect(src).toContain('No thanks');
    expect(src).not.toContain('type="checkbox"');
  });

  it('records a decline, so the offer does not come back every visit', () => {
    // Declining writes an explicit no. Without that, "no thanks" would be
    // indistinguishable from never having been asked.
    const src = source('pages/Discover.svelte');
    expect(src).toContain('answerBulletin(false)');
    expect(src).toContain('bulletinDecided');
  });

  it('asks nobody who is signed out, and nobody who already answered', () => {
    const src = source('pages/Discover.svelte');
    expect(src).toContain('isLoggedIn() && !bulletinDecided');
  });
});

// The engine's other consumer (docs/adr/075 consequences).
describe('filter chips', () => {
  const chips = () => source('components/FilterChips.svelte');

  it('render in usage order too — raw vocabulary order was never a decision', () => {
    expect(chips()).toContain('let ranked = $derived(getRankedTags());');
    // The row is planned from the ranked list and drawn from the plan; it
    // never reorders, only stops early.
    expect(chips()).toContain('return ranked.filter((t) => keep.has(t));');
    expect(chips()).toContain('let shown = $derived(open ? findTags(order, find) : rowTags);');
  });

  it('never hide an active tag behind the count', () => {
    // docs/adr/033: a filter is announced where it bites. A selected tag
    // that ranks below the fold is pinned into the row, in rank order.
    expect(chips()).toContain('const keep = new Set(selected);');
    expect(chips()).toContain('for (const t of ranked) if (selected.has(t)) budget -= w(t);');
  });

  it('keep the ruler out of reach and out of the page width', () => {
    expect(chips()).toContain('<div class="chips-measure" aria-hidden="true" inert>');
    expect(chips()).toMatch(/\.chips-measure \{[^}]*width: 0;[^}]*overflow: hidden;[^}]*visibility: hidden;/);
  });

  it('grow the row into a card from "N more", or from find when every tag fits', () => {
    expect(chips()).toContain('{hiddenCount} more');
    expect(chips()).toMatch(/class="chips-round chips-find-open"[\s\S]*?onclick=\{openCard\}/);
    expect(chips()).toContain('<div class="chips-card pw-canvas-card" bind:this={cardEl}></div>');
  });

  it('keep the first row as it stood when the card opens', () => {
    // The open list is the row, then the rest. Re-planning the row while
    // open would move a chip under the pointer.
    expect(chips()).toContain('order = [...rowTags, ...ranked.filter((t) => !rowTags.includes(t))];');
    expect(chips()).toContain('if (!open) rowWidth = measuredRowWidth;');
    // The card leaves the flow; the root holds the closed height so the
    // page under it does not move.
    expect(chips()).toContain('style:height={open ? `${closedHeight}px` : null}');
  });

  it('cap the find field at the width a tag needs, not the card', () => {
    expect(chips()).toMatch(/\.chips-find \{[^}]*flex: 0 1 280px;/);
    expect(chips()).toMatch(/@container \(max-width: 380px\) \{\s*\.chips-count \{\s*display: none;/);
  });

  it('animate with a clip and a FLIP, and not at all under reduced motion', () => {
    expect(chips()).toContain("const CLIP_SHUT = 'inset(-16px -16px 100% -16px)';");
    expect(chips()).toContain("window.matchMedia('(prefers-reduced-motion: reduce)').matches");
    // A close during the open starts from where things are on screen.
    expect(chips()).toContain('new DOMMatrix(getComputedStyle(rowEl).transform)');
  });

  it('show and hide from one button that never moves', () => {
    // Hidden, the funnel stands alone; shown, the same spot leads the row
    // as a hide button. Not an X: an X read as clearing the filter.
    expect(chips()).toMatch(/class="chips-toggle"[\s\S]*?aria-label="Show filters[\s\S]*?<FunnelSimple /);
    expect(chips()).toMatch(/class="chips-toggle on"[\s\S]*?aria-label="Hide filters"[\s\S]*?<EyeSlash /);
    expect(chips()).not.toContain('chips-collapse');
    // One size in both states, or showing the chips would move it.
    expect(chips()).toMatch(/\.chips-toggle \{[^}]*width: 29px;[^}]*height: 29px;/);
  });

  it('slide the row home past the funnel when the card closes', () => {
    expect(chips()).toContain('const dx = home.left + collapseWidth + GAP - (chip.left - shift.e);');
    expect(chips()).toContain("return rowEl?.querySelector('.lt-chip');");
  });

  it('close on Done, Escape, or a click outside, and keep a canvas click from going through', () => {
    expect(chips()).toContain("if (open && e.key === 'Escape') closeCard({ refocus: true });");
    expect(chips()).toContain('<svelte:window onclickcapture={onWindowClickCapture} onkeydown={onWindowKeydown} />');
    expect(chips()).toContain("el.hasAttribute('data-canvas')");
    expect(source('pages/SocialHome.svelte')).toContain('<div class="quilt-pane" data-canvas');
  });

  it('give the phone sheet the finder, without stealing the keyboard', () => {
    expect(chips()).toMatch(/\{#if variant === 'sheet'\}[\s\S]*?<TagFinder \/>/);
  });
});

// The phone sheet's body: every tag, found by typing.
describe('tag finder', () => {
  const finder = () => source('components/TagFinder.svelte');

  it('lists every tag in usage order, narrowed by the shared find rule', () => {
    expect(finder()).toContain('findTags(getRankedTags(), find)');
  });

  it('applies at once, with no Apply step', () => {
    expect(finder()).toContain('onclick={() => toggleTag(tag)}');
    expect(finder()).not.toMatch(/>\s*Apply\b/);
  });

  it('takes the first match on Enter and starts the field over', () => {
    expect(finder()).toContain('toggleTag(shown[0]);');
    expect(finder()).toContain("find = '';");
  });

  it('says so when nothing matches', () => {
    expect(finder()).toContain('No tag matches');
  });

  it('keeps the chips plain, like the row', () => {
    // The same reason as the row (tag counts, below): node_count is a
    // whole-quilt number, and this list narrows My Quilt and events too.
    expect(finder()).not.toContain('getTagCounts');
  });
});

// Per-tag counts (docs/adr/022, re-decided). The count is true on the
// question and a different quantity on the filter, so it lives on one only.
describe('tag counts', () => {
  it('says how many patches wear each tag, on the question', () => {
    const src = source('pages/Discover.svelte');
    expect(src).toContain('getTagCounts');
    expect(src).toContain('tag-count');
  });

  it('keeps the filter chips plain', () => {
    // node_count is whole-quilt and public. The chips narrow My Quilt, the
    // map, and the events list — where it would describe a corpus the person
    // is not looking at, or count patches beside a list of events.
    const src = source('components/FilterChips.svelte');
    expect(src).not.toContain('getTagCounts');
  });
});
