/**
 * The quilt's name in the global bar on a narrow screen. It used to be
 * hidden under 900px, leaving a phone with only the icon. Now it stays and
 * gives way: it shrinks in the row and, when it is cut off, fades out at the
 * right edge. Nothing here renders, so these assert the rules that do it.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8').replace(/\r\n/g, '\n');
}

const shell = () => source('components/SocialShell.svelte');

function narrowBlock() {
  const css = shell();
  const start = css.indexOf('@media (max-width: 900px) {');
  expect(start).toBeGreaterThan(-1);
  return css.slice(start, css.indexOf('\n  }\n', start));
}

describe('the quilt name on a narrow bar', () => {
  it('is no longer hidden', () => {
    expect(shell()).not.toMatch(/\.logo-label \{\s*display: none;/);
  });

  it('shrinks on one line instead of pushing the bar', () => {
    const block = narrowBlock();
    expect(block).toMatch(/\.scope-switcher,\s*\.scope-btn \{\s*flex-shrink: 1;\s*min-width: 0;/);
    const label = block.match(/\.logo-label \{[^}]*\}/)?.[0] || '';
    expect(label).toContain('min-width: 0;');
    expect(label).toContain('overflow: hidden;');
    expect(label).toContain('white-space: nowrap;');
    // .lt-type-name ends in an ellipsis; the fade replaces it.
    expect(label).toContain('text-overflow: clip;');
  });

  it('fades with a mask, and only when it is cut off', () => {
    const block = narrowBlock();
    // A mask, not an overlay of the bar colour, so it holds in both themes
    // and over the quilt.
    expect(block).toMatch(/\.logo-label\.clipped \{[^}]*mask-image: linear-gradient\(to right, #000 calc\(100% - 2rem\), transparent\);/);
    expect(shell()).toMatch(/el\.scrollWidth - el\.clientWidth > 1/);
    expect(shell()).toContain('class:clipped={logoLabelClipped}');
  });

  it('keeps the whole name as a title when it is cut off', () => {
    expect(shell()).toContain('title={logoLabelClipped ? activeScopeLabel : undefined}');
  });

  it('leaves the search its room beside it', () => {
    expect(shell()).toMatch(/@media \(min-width: 769px\) and \(max-width: 900px\) \{\s*\.logo-label \{\s*max-width: 11rem;/);
  });
});
