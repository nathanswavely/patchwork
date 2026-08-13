// Review copy as Markdown, anywhere, with no server running.
//
// `copy-draft` writes the queue out as Markdown files mirroring the source
// tree under copy/drafts/. You edit them wherever you like — GitHub's web
// editor on a phone, a text editor on a plane, anything that opens a file —
// and `copy-pull` reads your writing back into the ledger.
//
// This exists because the review UI needs a machine that can serve it and a
// browser that can reach that machine, which rules out most of the places
// writing actually happens. Markdown in a git repo has neither requirement,
// and it carries a bonus the local UI can't: when you edit a draft on the
// branch, the commit is yours, so `git blame` becomes a second, independent
// record of who wrote the words. The ledger says so; the history proves it.
//
// A draft is a **checkout**. While one exists, that file's strings are being
// decided somewhere this process cannot see, and deciding them again in the
// review UI is how the two surfaces diverge — see `outstandingDrafts` below.

import fs from 'node:fs';
import path from 'node:path';
import { REPO_ROOT } from './scope.js';
import { collapse, idFor } from './extract.js';

// Where drafts live. Defaults to copy/drafts/ inside this repo, but the
// drafts are just ID-keyed files — nothing ties them to the repo they
// describe. Point COPY_DRAFTS_DIR at a checkout of a separate (private)
// repo and the working copy stays out of the public tree entirely:
//
//   export COPY_DRAFTS_DIR=~/src/patchwork-copy/drafts
//
// The public repo then only ever sees finished copy changes and the
// ledger entry recording who wrote them — never the half-written middle.
export const DRAFT_DIR = process.env.COPY_DRAFTS_DIR
  ? path.resolve(process.env.COPY_DRAFTS_DIR)
  : path.join(REPO_ROOT, 'copy', 'drafts');

// What each block looked like at the moment it was drafted, kept beside the
// drafts themselves so it travels with them into the private repo.
//
// Without it, "did you write in this block?" can only be answered by asking
// the ledger what the text is *now* — and the whole problem is that the
// ledger may have moved on since. With it, an untouched block is recognisable
// as untouched even after its entry has been decided elsewhere, rewritten, or
// retired outright. That is the difference between skipping a stale block and
// writing it back over a newer decision.
const RENDERED_PATH = path.join(DRAFT_DIR, '.rendered.json');

// Kinds whose newlines are meaningful (Markdown lists, indented prose).
const BLOCK_KINDS = new Set(['md-block', 'go-doc-block']);

const OPEN = (id) => `<!-- copy:${id} -->`;
const CLOSE = (id) => `<!-- end:${id} -->`;
const BLOCK_RE = /<!-- copy:([0-9a-f]{12}) -->\n([\s\S]*?)\n<!-- end:\1 -->/g;

function loadRendered() {
  try { return JSON.parse(fs.readFileSync(RENDERED_PATH, 'utf8')).files || {}; }
  catch { return {}; }
}

function saveRendered(files) {
  fs.mkdirSync(DRAFT_DIR, { recursive: true });
  fs.writeFileSync(RENDERED_PATH, JSON.stringify({ version: 1, files }, null, 2) + '\n');
}

/** Soft-wrap a single-line string for comfortable editing. */
function wrap(text, width = 76) {
  const words = text.split(' ');
  const lines = [];
  let line = '';
  for (const w of words) {
    if (line && (line + ' ' + w).length > width) { lines.push(line); line = w; }
    else line = line ? `${line} ${w}` : w;
  }
  if (line) lines.push(line);
  return lines.join('\n');
}

function draftPathFor(file) {
  return path.join(DRAFT_DIR, `${file}.md`);
}

/**
 * A path worth printing: repo-relative when it is inside the repo, absolute
 * when it isn't. COPY_DRAFTS_DIR usually points somewhere else entirely, and
 * `../../../AppData/Local/...` helps nobody find a file.
 */
export function label(abs) {
  const r = path.relative(REPO_ROOT, abs);
  return r && !r.startsWith('..') ? r.split(path.sep).join('/') : abs;
}

/** DRAFT_DIR, in that same printable form. */
export const draftDirLabel = () => label(DRAFT_DIR);

/** The source file a draft path describes: …/drafts/README.md.md → README.md */
function sourceFor(abs) {
  return path.relative(DRAFT_DIR, abs).split(path.sep).join('/').replace(/\.md$/, '');
}

const HEADER = (file, entries) => `# ${file}

${entries.length} string${entries.length === 1 ? '' : 's'} · \
${entries.reduce((n, e) => n + e.words, 0).toLocaleString()} words

Write your version between the markers, replacing what's there. Anything
you leave untouched is skipped, so you can do three and come back.

- Replace a whole block with \`@mine\` if the words are already yours.
- Replace it with \`@fine\` to leave a model's draft in place on purpose
  (error messages, form labels — things with no voice to reclaim).

Leave the \`<!-- copy:… -->\` markers alone. They are how your writing
finds its way back into the source.

While this file exists, decide these strings **here** and not in
\`make copy-review\` — the review UI can rewrite a string out from under a
marker, and then this writing has nowhere to land.

When you're done: \`make copy-pull\`, then \`make copy-apply APPLY=1\`.

---
`;

/**
 * Write the queue to Markdown.
 * @returns {{files: string[], entries: number, skippedDirty: string[]}}
 */
export function writeDrafts(ledger, { file = null, tier = null, force = false } = {}) {
  const queue = ledger.entries.filter((e) =>
    (e.status === 'unreviewed' || e.status === 'rewritten') &&
    (!file || e.occurrences.some((o) => o.file === file)) &&
    (!tier || e.tier === tier));

  // Group by the file an entry primarily lives in.
  const byFile = new Map();
  for (const e of queue) {
    const home = e.occurrences[0].file;
    if (!byFile.has(home)) byFile.set(home, []);
    byFile.get(home).push(e);
  }

  const outstanding = new Map(
    outstandingDrafts(ledger).map((d) => [d.source, d]));
  const rendered = loadRendered();
  const written = [];
  const skippedDirty = [];

  for (const [source, entries] of byFile) {
    const target = draftPathFor(source);

    // Never clobber writing that hasn't been pulled yet.
    const held = outstanding.get(source);
    if (!force && held && (held.dirty.length || held.orphanedWriting.length)) {
      skippedDirty.push(source);
      continue;
    }

    entries.sort((a, b) => {
      const rank = { prose: 0, helper: 1, label: 2 };
      return rank[a.tier] - rank[b.tier] || a.occurrences[0].line - b.occurrences[0].line;
    });

    const blocks = {};
    const parts = [HEADER(source, entries)];
    for (const e of entries) {
      const o = e.occurrences[0];
      const elsewhere = e.occurrences.length > 1
        ? ` · also in ${e.occurrences.length - 1} other place${e.occurrences.length > 2 ? 's' : ''}`
        : '';
      const body = e.replacement ?? e.text;
      const shown = BLOCK_KINDS.has(o.kind) ? body : wrap(body);
      parts.push('');
      parts.push(`\`${o.file}:${o.line}\` · ${e.tier} · ${e.words}w${elsewhere}`);
      parts.push('');
      parts.push(OPEN(e.id));
      parts.push(shown);
      parts.push(CLOSE(e.id));
      blocks[e.id] = { t: shown.trim(), k: o.kind };
    }
    parts.push('');

    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, parts.join('\n'));
    rendered[source] = { drafted_at: new Date().toISOString(), blocks };
    written.push(label(target));
  }

  if (written.length) saveRendered(rendered);
  return { files: written.sort(), entries: queue.length, skippedDirty };
}

function normalizeKind(text, kind) {
  return BLOCK_KINDS.has(kind) ? text.replace(/\s+$/, '') : collapse(text);
}

function normalize(text, entry) {
  return normalizeKind(text, entry.occurrences[0].kind);
}

/**
 * What a block was rendered from, as `{t, k}` — or null if that is not
 * knowable, in which case callers must assume the block holds writing.
 *
 * The manifest answers this for anything drafted since it existed. Older
 * drafts get two fallbacks, both aimed at the same question — *did the ledger
 * move, or did you write?* A string that was rewritten and applied leaves its
 * draft on the entry that replaced it (`previous_ai_text`, which still hashes
 * to the stale marker's id); a string that left the source entirely leaves it
 * in `retired`. Between them, an untouched stale block is usually still
 * recognisable as untouched, which keeps a re-draft from rescuing 21 blocks
 * of writing nobody did.
 */
function draftedAs(id, asDrafted, ledger) {
  if (asDrafted[id]) return asDrafted[id];

  const retired = (ledger.retired || []).find((e) => e.id === id);
  if (retired) return { t: retired.text, k: retired.occurrences?.[0]?.kind };

  for (const e of ledger.entries) {
    if (e.previous_ai_text && idFor(e.previous_ai_text) === id) {
      return { t: e.previous_ai_text, k: e.occurrences[0].kind };
    }
  }
  return null;
}

/**
 * Has this block been written in since it was drafted?
 * `null` when there is no way to tell — treat that as yes.
 */
function touchedSince(text, id, asDrafted, ledger, entry) {
  const was = draftedAs(id, asDrafted, ledger);
  if (was) return normalizeKind(text, was.k) !== normalizeKind(was.t, was.k);
  if (!entry) return null;
  return normalize(text, entry) !== normalize(entry.replacement ?? entry.text, entry);
}

/** Parse one draft file into {id, text} blocks. */
function readDraft(abs) {
  const src = fs.readFileSync(abs, 'utf8');
  const out = [];
  let m;
  BLOCK_RE.lastIndex = 0;
  while ((m = BLOCK_RE.exec(src)) !== null) out.push({ id: m[1], text: m[2].trim() });
  return out;
}

function allDrafts(dir = DRAFT_DIR, out = []) {
  if (!fs.existsSync(dir)) return out;
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) allDrafts(full, out);
    else if (e.name.endsWith('.md')) out.push(full);
  }
  return out;
}

/**
 * Every draft file currently on disk, and what state it is in.
 *
 * This is the only honest answer to "is anyone else deciding this string?",
 * and three surfaces need it: `copy-draft` (don't clobber), `copy-stats`
 * (show the state before a session, not after), and the review UI (refuse a
 * decision on a string that is checked out somewhere else).
 *
 * Per file:
 *   dirty            — live markers holding writing not yet pulled
 *   stale            — markers whose entry is gone; the source moved under them
 *   orphanedWriting  — stale markers that hold writing, which pulling cannot
 *                      rescue because there is no entry left to record it on
 *   ids              — every entry id this file has checked out
 */
export function outstandingDrafts(ledger) {
  const byId = new Map(ledger.entries.map((e) => [e.id, e]));
  const rendered = loadRendered();
  const out = [];

  for (const abs of allDrafts()) {
    const source = sourceFor(abs);
    const blocks = readDraft(abs);
    if (!blocks.length) continue;

    const asDrafted = rendered[source]?.blocks || {};
    const dirty = [];
    const stale = [];
    const ids = [];

    for (const { id, text } of blocks) {
      const entry = byId.get(id);
      // Untouched means "the same as when it was drafted". Asking the ledger
      // what the text is *now* is a guess, and the guess goes wrong exactly
      // when it matters — after the other surface moved the string.
      const touched = touchedSince(text, id, asDrafted, ledger, entry);

      // A pulled-and-applied block goes stale too — its writing is in the
      // source now, under a new id that is this block's own text. That is a
      // finished job, not writing at risk, so it must not be rescued as if
      // it were homeless.
      if (!entry) { stale.push({ id, text, touched, landed: byId.has(idFor(text)) }); continue; }
      ids.push(id);
      if (touched !== false) dirty.push({ id, text });
    }

    out.push({
      source,
      path: abs,
      rel: label(abs),
      blocks: blocks.length,
      dirty,
      stale,
      orphanedWriting: stale.filter((s) => s.touched !== false && !s.landed),
      ids,
      draftedAt: rendered[source]?.drafted_at || null,
    });
  }

  return out.sort((a, b) => a.source.localeCompare(b.source));
}

/** Which outstanding draft, if any, has this entry checked out. */
export function draftHolding(ledger, id) {
  return outstandingDrafts(ledger).find((d) => d.ids.includes(id)) || null;
}

/**
 * Read every draft back into the ledger (mutated in place).
 * @returns {{recorded, unchanged, mine, fine, unknown, files}}
 */
export function pullDrafts(ledger) {
  const byId = new Map(ledger.entries.map((e) => [e.id, e]));
  const rendered = loadRendered();
  const r = { recorded: 0, unchanged: 0, mine: 0, fine: 0, unknown: [], files: 0 };

  for (const abs of allDrafts()) {
    r.files++;
    const source = sourceFor(abs);
    const asDrafted = rendered[source]?.blocks || {};

    for (const { id, text } of readDraft(abs)) {
      const e = byId.get(id);
      const touched = touchedSince(text, id, asDrafted, ledger, e);
      if (!e) {
        // `landed`: this block's own writing is already in source under a new
        // id — a marker gone stale because the work finished, not because it
        // was lost. See outstandingDrafts.
        r.unknown.push({ id, file: source, touched, landed: byId.has(idFor(text)) });
        continue;
      }

      if (text === '@mine') { e.status = 'human'; e.replacement = null; r.mine++; continue; }
      if (text === '@fine') { e.status = 'ai-fine'; e.replacement = null; r.fine++; continue; }

      // A block nobody touched carries no decision, so it must not make one —
      // even when the ledger has since moved and the two no longer agree. This
      // is the case that used to write a draft's stale rendering back over a
      // newer decision made in the review UI.
      if (touched === false) { r.unchanged++; continue; }

      const next = normalize(text, e);
      if (!next) { r.unchanged++; continue; }
      if (next === normalize(e.text, e)) { r.unchanged++; continue; }

      e.status = 'rewritten';
      e.replacement = next;
      r.recorded++;
    }
  }
  return r;
}

/**
 * Remove draft files whose entries are all decided.
 * @returns {{removed: string[], kept: string[]}} kept = files held back
 *   because they contain writing with no entry left to record it on.
 */
export function pruneDrafts(ledger) {
  const byId = new Map(ledger.entries.map((e) => [e.id, e]));
  const rendered = loadRendered();
  const removed = [];
  const kept = [];

  for (const d of outstandingDrafts(ledger)) {
    if (d.orphanedWriting.length) { kept.push(d.rel); continue; }
    const open = d.ids.filter((id) => byId.get(id)?.status === 'unreviewed');
    if (open.length) continue;
    fs.unlinkSync(d.path);
    delete rendered[d.source];
    removed.push(d.rel);
  }
  if (removed.length) saveRendered(rendered);

  // Tidy any directories the removals emptied.
  const prune = (dir) => {
    if (!fs.existsSync(dir) || dir === DRAFT_DIR) return;
    if (!fs.readdirSync(dir).length) { fs.rmdirSync(dir); prune(path.dirname(dir)); }
  };
  for (const rel of removed) prune(path.dirname(path.join(REPO_ROOT, rel)));
  return { removed, kept };
}

/**
 * Copy writing out of stale markers before anything overwrites them.
 *
 * A stale marker's writing cannot be recorded — the entry it named is gone —
 * but it is still someone's sentence, so it is set down beside the draft in a
 * file the tool will never read back, rather than dropped on the floor.
 *
 * @returns {string[]} repo-relative paths written
 */
export function rescueOrphans(ledger, sources = null) {
  const written = [];
  for (const d of outstandingDrafts(ledger)) {
    if (sources && !sources.includes(d.source)) continue;
    const target = path.join(DRAFT_DIR, `${d.source}.orphaned.txt`);
    // Rescuing twice is easy to do and rescuing the same sentence twice helps
    // nobody, so an already-rescued block is left alone.
    const already = fs.existsSync(target) ? fs.readFileSync(target, 'utf8') : '';
    const orphans = d.orphanedWriting.filter((o) => !already.includes(o.text));
    if (!orphans.length) continue;

    const lines = [
      `# Writing rescued from ${d.source}`,
      `# ${new Date().toISOString()}`,
      '#',
      '# These blocks were edited, but the strings they named are no longer in',
      '# the ledger — the source changed underneath the draft. Nothing can',
      '# record them automatically. Paste what you want to keep into a fresh',
      '# draft (`make copy-draft FILE=' + d.source + '`) or the review UI.',
      '',
    ];
    for (const o of orphans) {
      lines.push(`--- ${o.id}`);
      lines.push(o.text);
      lines.push('');
    }
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.appendFileSync(target, lines.join('\n'));
    written.push(label(target));
  }
  return written;
}
