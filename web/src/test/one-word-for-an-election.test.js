import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { resolve, join } from 'node:path';

const SRC = resolve(__dirname, '..');

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) {
      if (name === 'test') continue;
      walk(p, out);
    } else if (/\.(svelte|js)$/.test(name)) {
      out.push(p);
    }
  }
  return out;
}

/** Source with comments removed: commentary is ours, copy is theirs. */
function shipped(file) {
  return readFileSync(file, 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

// F-107. "Four words for one event. It took me a few minutes of clicking
// between the pages to be sure they were all the same election and not four
// different ones I was somehow behind on." — Devon. Ana went looking for a
// contest she had missed; Sam sat on the word "unsettled"; Priya read
// "Settled nothing" as a dispute.

describe('the product has one word for an election (F-107)', () => {
  // `contest_open`, `contest_id` and the rest are backend names and stay:
  // CLAUDE.md gives the backend generic terms precisely so the UI can have
  // the textile ones. What must not survive is the word reaching a reader.
  it('never says "contest" to a reader', () => {
    const offenders = [];
    for (const file of walk(SRC)) {
      const src = shipped(file);
      // Strip identifiers built on the stem before looking for the word.
      const prose = src
        .replace(/\bcontest(ed|s)?[_A-Za-z]*\b(?=\s*[:=,)\]}]|['"]?\s*[:=])/g, '')
        .replace(/\bcontest_[a-z]+\b/g, '')
        .replace(/\bseats_contested\b|\bcontestedIn\b|\bseat-contest\b|\bnextContestOpens\b|\bcontestDue\b/g, '');
      for (const m of prose.matchAll(/\bcontests?\b/gi)) {
        // "the next election contests the seats that exist" — the verb is
        // not the noun, and nobody reads it as a name for the event.
        const around = prose.slice(Math.max(0, m.index - 30), m.index + 30);
        if (/election contests/.test(around)) continue;
        offenders.push(`${file.slice(SRC.length + 1)}: ...${around.replace(/\s+/g, ' ')}...`);
      }
    }
    expect(offenders, 'the UI word is "election" (CONTEXT.md)').toEqual([]);
  });

  // The panel headed itself "The ballot", which read as a fourth name for
  // the event rather than as the phase it is. A ballot is what a voter
  // casts.
  it('heads the panel with the phase, in the bell\'s own words', () => {
    const panel = shipped(resolve(SRC, 'components/ElectionPanel.svelte'));
    expect(panel).toContain("{#if phase === 'nominating'}Nominations{:else if phase === 'voting'}Voting{:else}Result{/if}");
  });
});

describe('an outcome a reader can place (F-107)', () => {
  // CONTEXT.md has always said a lapse reads "not decided" on every
  // surface. Two of them said "lapsed".
  it('says "not decided" for a lapse, not the state name', () => {
    for (const f of ['pages/ProposalList.svelte', 'components/PatchProfileGlimpses.svelte']) {
      const src = shipped(resolve(SRC, f));
      expect(src).toContain("return 'not decided';");
    }
  });

  // "Unsettled like unresolved? Unsettled like still running?"
  it('says "nobody elected" rather than "unsettled"', () => {
    for (const f of ['pages/ProposalList.svelte', 'components/PatchProfileGlimpses.svelte']) {
      const src = shipped(resolve(SRC, f));
      expect(src).toContain("return 'nobody elected';");
    }
  });

  // The state keeps its name — the filter and the badge colour read it.
  it('keeps the state name where the machine reads it', () => {
    const list = shipped(resolve(SRC, 'pages/ProposalList.svelte'));
    expect(list).toContain("if (status === 'withdrawn' || status === 'lapsed' || status === 'unsettled') return 'status-withdrawn';");
    expect(list).toContain("statusClass(rowStatus(proposal))");
    expect(list).toContain('{rowLabel(proposal)}');
  });

  // Priya read "Settled nothing" as "tied or disputed, something
  // contentious", so the Record leads with the fact instead.
  it('the Record leads with what happened', () => {
    const rec = shipped(resolve(SRC, 'pages/GovernanceRecord.svelte'));
    expect(rec).toContain("return 'Nobody was elected. The seats it was for are unchanged.';");
    expect(rec).not.toContain("'Settled nothing. Nobody was elected.'");
  });
});
