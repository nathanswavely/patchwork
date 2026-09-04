import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

// Below 768px SocialShell's rail is a translucent bottom tab bar that
// OVERLAYS the page, so every surface that ends at the foot of the screen
// has to leave room for it. That room was reserved twice — a 84px rule in
// the mobile block and an intro-card gutter written as
// `:global(body) .social-main` — and the body-prefixed one won on
// specificity, so a phone with no intro card up (anyone signed in) reserved
// 12px for a 51px bar and the page ran underneath it. One token, declared
// once, consumed everywhere.
describe('mobile bottom nav clearance', () => {
  const css = () => source('app.css');
  const shell = () => source('components/SocialShell.svelte');

  it('declares the bar footprint as one token, zero above the breakpoint', () => {
    // 0px above the breakpoint is what lets every consumer write
    // `calc(var(--pw-nav-h) + <gap>)` with no media query of its own.
    expect(css()).toMatch(/--pw-nav-h:\s*0px/);
    expect(css()).toMatch(/--pw-nav-h:\s*calc\(51px \+ env\(safe-area-inset-bottom, 0px\)\)/);
  });

  it('shares the shell breakpoint for that token', () => {
    const block = css().slice(css().indexOf('--- The mobile bottom tab bar'));
    expect(block).toContain('@media (max-width: 768px)');
    expect(shell()).toContain('@media (max-width: 768px)');
  });

  it('gives the bar the height the token reserves for it', () => {
    // Otherwise 51px is a guess about a box whose contents could change.
    const src = shell();
    expect(src).toContain('height: var(--pw-nav-h)');
    expect(src).toContain('box-sizing: border-box');
  });

  it('reserves the foot of the page in exactly one declaration', () => {
    const src = shell();
    const bottoms = src.match(/^\s*padding-bottom:.*$/gm) || [];
    expect(bottoms).toHaveLength(1);
    expect(bottoms[0]).toContain('var(--pw-nav-h)');
    // max(), not a sum: the intro card publishes its footprint measured from
    // the viewport bottom, so its number already contains the bar's.
    expect(bottoms[0]).toContain('max(');
    expect(bottoms[0]).toContain('var(--intro-card-h');
  });

  it('keeps that declaration out of reach of a specificity fight', () => {
    // The rule that caused this: a `:global(body)`-prefixed selector
    // outranks a plain `.social-main` one however it is ordered.
    // (as a selector — the rule's history is written down in a comment
    // there, which is the point of keeping it out of the CSS)
    expect(shell()).not.toMatch(/:global\(body\)\s+\.social-main\s*\{/);
  });

  it('has nothing at the foot repeating a number of its own', () => {
    const foot = [
      ['components/SocialShell.svelte', shell()],
      ['components/Toast.svelte', source('components/Toast.svelte')],
      ['components/MapView.svelte', source('components/MapView.svelte')],
      ['components/IntroCard.svelte', source('components/IntroCard.svelte')],
    ];
    for (const [name, src] of foot) {
      // 84/108/60 were three guesses at the same bar, and none of the three
      // carried env(safe-area-inset-bottom) on its own.
      expect(src, name).not.toMatch(/bottom:\s*(84|108|60)px/);
      expect(src, name).not.toMatch(/padding-bottom:\s*(84|108|60)px/);
    }
  });
});
