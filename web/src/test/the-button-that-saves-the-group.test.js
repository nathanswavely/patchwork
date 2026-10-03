import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const page = shipped('../pages/AccountSettings.svelte');

// F-103. Two downloads three inches apart: one saves a person, one saves the
// community, and the headings said neither.
//
// "If I'd taken the obvious-sounding one — the one called 'Download my
// data' — I'd have walked away thinking I had the press's records and I'd
// have had a file with one person in it."
//
// And the one that does save the group sat directly above **Danger Zone**
// under a label that gave no clue which way it cut: "I sat there for a good
// half-minute working out whether 'Member seamrip' meant 'rip out this
// member' — as in, remove me."

describe('the two downloads say whose data they are (F-103)', () => {
  it('names the personal one after the person', () => {
    expect(page).toContain('<h2>Your own data</h2>');
    expect(page).not.toContain('<h2>Download my data</h2>');
  });

  it('leads the other with plain words, keeping the product\'s own beside them', () => {
    expect(page).toContain('<h2>A copy of this quilt (seamrip)</h2>');
    expect(page).not.toContain('<h2>Member seamrip</h2>');
  });

  // Each closes the other's trap: a reader who takes one must be told the
  // other exists, or the wrong choice is silent.
  it('each points at the other', () => {
    expect(page).toContain('For your community\'s own');
    expect(page).toContain('records, take a copy of the quilt below.');
  });

  // The half-minute she spent wondering whether it removed her.
  it('says the copy takes nothing away, next to Danger Zone', () => {
    expect(page).toContain('Nothing here removes you or changes this quilt.');
    // And what to do with the file, since the archive is no use to her
    // unless it reaches whoever stands the new quilt up.
    expect(page).toContain('Any Patchwork can');
    expect(page).toContain('read the file, so it goes to whoever sets the new one up.');
  });

  // Order matters: the personal one is read first, so it is the one that
  // has to disclaim the wider scope.
  it('keeps the personal section above the quilt copy', () => {
    expect(page.indexOf('<h2>Your own data</h2>'))
      .toBeLessThan(page.indexOf('<h2>A copy of this quilt (seamrip)</h2>'));
  });
});
