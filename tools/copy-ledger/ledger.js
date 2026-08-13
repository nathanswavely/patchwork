// Load, merge and save copy/ledger.json.
//
// The ledger is the artifact, not the tooling. It is checked in because it
// IS the transparency claim: a machine-checkable record of which words were
// drafted by a model and which were written by a person, with the drafts
// kept alongside the replacements. A paragraph asserting "the copy is human"
// is worth less than a file anyone can diff.

import fs from 'node:fs';
import path from 'node:path';
import { LEDGER_PATH } from './scope.js';
import { extractEntries, idFor, tierFor } from './extract.js';

// unreviewed → a model drafted it and nobody has looked yet (the default)
// rewritten  → a person wrote a replacement; writeback has not applied it
// human      → the text in source is a person's words
// ai-fine    → deliberately left as drafted; mechanical or not voice-bearing
export const STATUSES = ['unreviewed', 'rewritten', 'human', 'ai-fine'];

export const EMPTY = { version: 1, strict: false, entries: [], retired: [] };

export function load() {
  if (!fs.existsSync(LEDGER_PATH)) return structuredClone(EMPTY);
  const raw = JSON.parse(fs.readFileSync(LEDGER_PATH, 'utf8'));
  return { ...structuredClone(EMPTY), ...raw };
}

export function save(ledger) {
  fs.mkdirSync(path.dirname(LEDGER_PATH), { recursive: true });
  fs.writeFileSync(LEDGER_PATH, JSON.stringify(ledger, null, 2) + '\n');
}

/**
 * Re-extract from source and fold the result into an existing ledger,
 * preserving every human decision.
 *
 * Returns { ledger, added, removed, drifted } so callers can report honestly
 * instead of silently reconciling.
 */
export function sync(existing = load()) {
  const fresh = extractEntries();
  const prior = new Map(existing.entries.map((e) => [e.id, e]));
  const retired = new Map((existing.retired || []).map((e) => [e.id, e]));

  const added = [];
  const entries = fresh.map((f) => {
    const seen = prior.get(f.id) || retired.get(f.id);
    if (!seen) { added.push(f); return f; }
    return {
      ...f,
      status: seen.status || 'unreviewed',
      replacement: seen.replacement ?? null,
      note: seen.note ?? null,
      // The draft this text replaced, kept for the public record.
      ...(seen.previous_ai_text ? { previous_ai_text: seen.previous_ai_text } : {}),
    };
  });

  const freshIds = new Set(fresh.map((e) => e.id));
  const removed = existing.entries.filter((e) => !freshIds.has(e.id));

  // A decision that took thought is worth keeping even after the string
  // leaves the source; an untouched `unreviewed` carries no information.
  const keptRetired = [...retired.values(), ...removed]
    .filter((e) => e.status && e.status !== 'unreviewed')
    .filter((e) => !freshIds.has(e.id));
  const dedupedRetired = [...new Map(keptRetired.map((e) => [e.id, e])).values()];

  return {
    ledger: { ...existing, version: 1, entries, retired: dedupedRetired },
    added,
    removed,
  };
}

export function stats(ledger) {
  const byStatus = Object.fromEntries(STATUSES.map((s) => [s, 0]));
  // Words per status, not just counts. `wordsDone` lumps human in with
  // ai-fine, which is the right number for a progress bar and the wrong one
  // for a provenance claim — the report printed it beside "Written by a
  // person" and overstated human authorship by every accepted draft.
  const wordsByStatus = Object.fromEntries(STATUSES.map((s) => [s, 0]));
  const byTier = { label: { done: 0, total: 0 }, helper: { done: 0, total: 0 }, prose: { done: 0, total: 0 } };
  let words = 0;
  let wordsDone = 0;

  for (const e of ledger.entries) {
    byStatus[e.status] = (byStatus[e.status] || 0) + 1;
    wordsByStatus[e.status] = (wordsByStatus[e.status] || 0) + e.words;
    const done = e.status === 'human' || e.status === 'ai-fine';
    byTier[e.tier].total++;
    if (done) byTier[e.tier].done++;
    words += e.words;
    if (done) wordsDone += e.words;
  }

  const total = ledger.entries.length;
  const done = byStatus.human + byStatus['ai-fine'];
  return { total, done, pending: total - done, byStatus, wordsByStatus, byTier, words, wordsDone };
}

/** Apply one review decision to an entry, in place. Returns the entry. */
export function decide(entry, { status, replacement, note }) {
  if (status && !STATUSES.includes(status)) throw new Error(`bad status: ${status}`);
  if (status) entry.status = status;
  if (replacement !== undefined) entry.replacement = replacement || null;
  if (note !== undefined) entry.note = note || null;
  if (entry.status === 'rewritten' && !entry.replacement) entry.status = 'unreviewed';
  return entry;
}

/**
 * Fold an applied replacement back into the entry: the new text becomes the
 * entry's identity, and the model's draft is preserved as the record of what
 * was replaced.
 */
export function promote(entry, newText) {
  const previous = entry.previous_ai_text || entry.text;
  entry.previous_ai_text = previous;
  entry.text = newText;
  entry.id = idFor(newText);
  entry.words = newText.split(/\s+/).filter(Boolean).length;
  entry.tier = tierFor(newText);
  entry.replacement = null;
  entry.status = 'human';
  return entry;
}
