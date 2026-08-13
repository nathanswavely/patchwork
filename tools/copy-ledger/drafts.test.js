// What happens when the same file is decided on both review surfaces at once.
//
// The property under test is the one that makes divergence a papercut rather
// than data loss: a draft block nobody wrote in must never be pulled back over
// a decision made in the review UI since. Everything else here — stale marker
// reporting, the checkout list, orphan rescue — exists to make that state
// visible before a session instead of after one.
//
//   node --test tools/copy-ledger/     (or: make copy-test)

import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { idFor } from './extract.js';

// Each case gets its own draft directory, and drafts.js reads COPY_DRAFTS_DIR
// once at import — so the module is re-imported per case with a cache-busting
// query, which is also a small proof that nothing assumes copy/drafts/.
let n = 0;
async function withDrafts(fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'copy-drafts-'));
  process.env.COPY_DRAFTS_DIR = dir;
  try {
    const mod = await import(`./drafts.js?case=${n++}`);
    return await fn(mod, dir);
  } finally {
    delete process.env.COPY_DRAFTS_DIR;
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

const AI = 'A sentence some model produced for the front door of the site.';
const NOW_IN_SOURCE = 'The replacement a person wrote, now in source.';

// Ids are content-derived — that is the whole reason a marker can go stale —
// so fixtures derive theirs the same way rather than making one up.
const entry = (over = {}) => {
  const text = over.text ?? AI;
  return {
    id: idFor(text), text, words: text.split(/\s+/).length, tier: 'helper',
    status: 'unreviewed', replacement: null,
    occurrences: [{ file: 'README.md', line: 12, kind: 'md-line', raw: text }],
    ...over,
  };
};

/** The ledger after that string was rewritten in the UI and applied. */
const afterApply = () => ledgerWith(entry({
  text: NOW_IN_SOURCE, status: 'human', previous_ai_text: AI,
}));

const ledgerWith = (...entries) => ({ version: 1, entries, retired: [] });

/** Overwrite one block's body, the way editing the draft by hand would. */
function editBlock(dir, source, id, body) {
  const p = path.join(dir, `${source}.md`);
  const src = fs.readFileSync(p, 'utf8');
  const re = new RegExp(`(<!-- copy:${id} -->\\n)[\\s\\S]*?(\\n<!-- end:${id} -->)`);
  fs.writeFileSync(p, src.replace(re, `$1${body}$2`));
}

test('an untouched draft never overwrites a decision made in the UI', async () => {
  await withDrafts(async ({ writeDrafts, pullDrafts }, dir) => {
    const ledger = ledgerWith(entry());
    writeDrafts(ledger);

    // Meanwhile, in the review UI: the same string is rewritten.
    ledger.entries[0].status = 'rewritten';
    ledger.entries[0].replacement = 'Words a person wrote at the workbench.';

    const r = pullDrafts(ledger);
    assert.equal(r.unchanged, 1);
    assert.equal(r.recorded, 0);
    assert.equal(ledger.entries[0].replacement, 'Words a person wrote at the workbench.');
  });
});

// The regression this file exists for. A draft cut from an entry that already
// had a replacement renders *the replacement*, so "same as the ledger's text"
// was not the same question as "did you write in this block" — and an
// untouched block read as a rewrite, reinstating a superseded replacement over
// a newer `human`.
test('a draft cut from a replacement does not reinstate it after the UI moves on', async () => {
  await withDrafts(async ({ writeDrafts, pullDrafts }, dir) => {
    const ledger = ledgerWith(entry({
      status: 'rewritten', replacement: 'An early attempt, later abandoned.',
    }));
    writeDrafts(ledger);

    // In the UI: that attempt is dropped, and the original is accepted as-is.
    ledger.entries[0].status = 'human';
    ledger.entries[0].replacement = null;

    const r = pullDrafts(ledger);
    assert.equal(r.recorded, 0, 'an untouched block recorded a decision');
    assert.equal(ledger.entries[0].status, 'human');
    assert.equal(ledger.entries[0].replacement, null);
  });
});

test('writing in a draft is still pulled', async () => {
  await withDrafts(async ({ writeDrafts, pullDrafts }, dir) => {
    const ledger = ledgerWith(entry());
    writeDrafts(ledger);
    editBlock(dir, 'README.md', ledger.entries[0].id, 'A sentence I wrote on a train.');

    const r = pullDrafts(ledger);
    assert.equal(r.recorded, 1);
    assert.equal(ledger.entries[0].status, 'rewritten');
    assert.equal(ledger.entries[0].replacement, 'A sentence I wrote on a train.');
  });
});

test('a stale marker is reported, never applied to another entry', async () => {
  await withDrafts(async ({ writeDrafts, pullDrafts }, dir) => {
    const ledger = ledgerWith(entry());
    writeDrafts(ledger);
    editBlock(dir, 'README.md', ledger.entries[0].id, 'Writing with nowhere to land.');

    // The string was decided and applied in the UI: new text, new id.
    const moved = afterApply();

    const r = pullDrafts(moved);
    assert.equal(r.unknown.length, 1);
    assert.equal(r.unknown[0].file, 'README.md');
    assert.equal(r.unknown[0].touched, true, 'edited orphan reported as untouched');
    assert.equal(moved.entries[0].status, 'human');
    assert.equal(moved.entries[0].replacement, null);
  });
});

// Without this, a re-draft would "rescue" every untouched block in the file as
// if it were writing — 21 blocks of nobody's prose, in the incident that
// prompted all this.
test('an untouched stale marker is recognised even with no manifest', async () => {
  await withDrafts(async ({ writeDrafts, outstandingDrafts }, dir) => {
    const ledger = ledgerWith(entry());
    writeDrafts(ledger);
    fs.rmSync(path.join(dir, '.rendered.json'));

    const [d] = outstandingDrafts(afterApply());
    assert.equal(d.stale.length, 1);
    assert.equal(d.stale[0].touched, false);
    assert.equal(d.orphanedWriting.length, 0, 'untouched block treated as writing');
  });
});

// Pull, then apply, and the marker goes stale for the happiest reason there
// is: the writing is in the source now. Rescuing it would file a finished job
// as a loss.
test('writing that already landed in source is not treated as orphaned', async () => {
  await withDrafts(async ({ writeDrafts, outstandingDrafts, rescueOrphans }, dir) => {
    const ledger = ledgerWith(entry());
    writeDrafts(ledger);
    const mine = 'A sentence I wrote on the bus, and then applied.';
    editBlock(dir, 'README.md', ledger.entries[0].id, mine);

    // Pulled, applied: the entry now *is* that sentence, under a new id.
    const applied = ledgerWith(entry({ text: mine, status: 'human', previous_ai_text: AI }));

    const [d] = outstandingDrafts(applied);
    assert.equal(d.stale.length, 1);
    assert.equal(d.stale[0].landed, true);
    assert.equal(d.orphanedWriting.length, 0);
    assert.deepEqual(rescueOrphans(applied), []);
  });
});

test('outstandingDrafts lists what each file holds', async () => {
  await withDrafts(async ({ writeDrafts, outstandingDrafts, draftHolding }, dir) => {
    const other = entry({
      text: 'Another string entirely, longer than five words.',
      occurrences: [{ file: 'README.md', line: 30, kind: 'md-line', raw: 'x' }],
    });
    const ledger = ledgerWith(entry(), other);
    writeDrafts(ledger);

    let [d] = outstandingDrafts(ledger);
    assert.equal(d.source, 'README.md');
    assert.equal(d.blocks, 2);
    assert.equal(d.dirty.length, 0, 'a fresh draft holds no unpulled writing');
    assert.deepEqual(d.ids.sort(), [idFor(AI), other.id].sort());

    editBlock(dir, 'README.md', other.id, '@mine');
    [d] = outstandingDrafts(ledger);
    assert.equal(d.dirty.length, 1, '@mine is an unpulled decision');
    assert.equal(draftHolding(ledger, idFor(AI)).source, 'README.md');
    assert.equal(draftHolding(ledger, 'nosuchentry00'), null);
  });
});

test('copy-draft will not overwrite a draft holding writing', async () => {
  await withDrafts(async ({ writeDrafts }, dir) => {
    const ledger = ledgerWith(entry());
    writeDrafts(ledger);
    editBlock(dir, 'README.md', ledger.entries[0].id, 'Half a thought, still going.');

    const again = writeDrafts(ledger);
    assert.deepEqual(again.skippedDirty, ['README.md']);
    assert.match(fs.readFileSync(path.join(dir, 'README.md.md'), 'utf8'), /Half a thought/);

    writeDrafts(ledger, { force: true });
    assert.doesNotMatch(fs.readFileSync(path.join(dir, 'README.md.md'), 'utf8'), /Half a thought/);
  });
});

test('orphaned writing is set down before anything can overwrite it', async () => {
  await withDrafts(async ({ writeDrafts, rescueOrphans, pruneDrafts }, dir) => {
    const ledger = ledgerWith(entry());
    writeDrafts(ledger);
    editBlock(dir, 'README.md', ledger.entries[0].id, 'The one good sentence of the day.');

    const moved = afterApply();

    // Pruning must not take a file that still holds unrecordable writing.
    assert.deepEqual(pruneDrafts(moved).removed, []);
    assert.equal(pruneDrafts(moved).kept.length, 1);

    const written = rescueOrphans(moved, ['README.md']);
    assert.equal(written.length, 1);
    const saved = fs.readFileSync(path.join(dir, 'README.md.orphaned.txt'), 'utf8');
    assert.match(saved, /The one good sentence of the day\./);

    // Rescuing twice must not copy the same sentence twice.
    rescueOrphans(moved, ['README.md']);
    assert.equal(fs.readFileSync(path.join(dir, 'README.md.orphaned.txt'), 'utf8'), saved);
  });
});

test('a finished draft is still cleared', async () => {
  await withDrafts(async ({ writeDrafts, pullDrafts, pruneDrafts }, dir) => {
    const ledger = ledgerWith(entry());
    writeDrafts(ledger);
    editBlock(dir, 'README.md', ledger.entries[0].id, '@mine');

    pullDrafts(ledger);
    assert.equal(ledger.entries[0].status, 'human');
    assert.equal(pruneDrafts(ledger).removed.length, 1);
    assert.equal(fs.existsSync(path.join(dir, 'README.md.md')), false);
  });
});
