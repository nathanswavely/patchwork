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

import fs from 'node:fs';
import path from 'node:path';
import { REPO_ROOT } from './scope.js';
import { collapse } from './extract.js';

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

// Kinds whose newlines are meaningful (Markdown lists, indented prose).
const BLOCK_KINDS = new Set(['md-block', 'go-doc-block']);

const OPEN = (id) => `<!-- copy:${id} -->`;
const CLOSE = (id) => `<!-- end:${id} -->`;
const BLOCK_RE = /<!-- copy:([0-9a-f]{12}) -->\n([\s\S]*?)\n<!-- end:\1 -->/g;

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

  const written = [];
  const skippedDirty = [];

  for (const [source, entries] of byFile) {
    const target = draftPathFor(source);

    // Never clobber writing that hasn't been pulled yet.
    if (!force && fs.existsSync(target)) {
      const unpulled = readDraft(target).filter(({ id, text }) => {
        const e = ledger.entries.find((x) => x.id === id);
        if (!e) return false;
        const current = e.replacement ?? e.text;
        return text !== '@mine' && text !== '@fine' && normalize(text, e) !== normalize(current, e);
      });
      if (unpulled.length) { skippedDirty.push(source); continue; }
    }

    entries.sort((a, b) => {
      const rank = { prose: 0, helper: 1, label: 2 };
      return rank[a.tier] - rank[b.tier] || a.occurrences[0].line - b.occurrences[0].line;
    });

    const parts = [HEADER(source, entries)];
    for (const e of entries) {
      const o = e.occurrences[0];
      const elsewhere = e.occurrences.length > 1
        ? ` · also in ${e.occurrences.length - 1} other place${e.occurrences.length > 2 ? 's' : ''}`
        : '';
      const body = e.replacement ?? e.text;
      parts.push('');
      parts.push(`\`${o.file}:${o.line}\` · ${e.tier} · ${e.words}w${elsewhere}`);
      parts.push('');
      parts.push(OPEN(e.id));
      parts.push(BLOCK_KINDS.has(o.kind) ? body : wrap(body));
      parts.push(CLOSE(e.id));
    }
    parts.push('');

    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, parts.join('\n'));
    written.push(path.relative(REPO_ROOT, target));
  }

  return { files: written.sort(), entries: queue.length, skippedDirty };
}

function normalize(text, entry) {
  const kind = entry.occurrences[0].kind;
  return BLOCK_KINDS.has(kind) ? text.replace(/\s+$/, '') : collapse(text);
}

/** Parse one draft file into {id, text} blocks. */
function readDraft(abs) {
  // Normalize to LF before matching. BLOCK_RE anchors on a bare \n, so a
  // CRLF draft matches zero blocks and pull reports "No changes found" —
  // a whole writing session dropped with no error. Drafts are prose, so a
  // CR is never meaningful; stripping here also keeps interior ones out of
  // the replacement text, which writeback would carry into source files.
  // This repo's .gitattributes can't help: COPY_DRAFTS_DIR usually points
  // at a checkout of a different repo.
  const src = fs.readFileSync(abs, 'utf8').replace(/\r\n/g, '\n');
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
 * Read every draft back into the ledger (mutated in place).
 * @returns {{recorded, unchanged, mine, fine, unknown, files}}
 */
export function pullDrafts(ledger) {
  const byId = new Map(ledger.entries.map((e) => [e.id, e]));
  const r = { recorded: 0, unchanged: 0, mine: 0, fine: 0, unknown: [], files: 0 };

  for (const abs of allDrafts()) {
    r.files++;
    for (const { id, text } of readDraft(abs)) {
      const e = byId.get(id);
      if (!e) { r.unknown.push(id); continue; }

      if (text === '@mine') { e.status = 'human'; e.replacement = null; r.mine++; continue; }
      if (text === '@fine') { e.status = 'ai-fine'; e.replacement = null; r.fine++; continue; }

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

/** Remove draft files whose entries are all decided. */
export function pruneDrafts(ledger) {
  const byId = new Map(ledger.entries.map((e) => [e.id, e]));
  const removed = [];
  for (const abs of allDrafts()) {
    const blocks = readDraft(abs);
    if (!blocks.length) continue;
    const open = blocks.filter(({ id }) => byId.get(id)?.status === 'unreviewed');
    if (open.length) continue;
    fs.unlinkSync(abs);
    removed.push(path.relative(REPO_ROOT, abs));
  }
  // Tidy any directories the removals emptied.
  const prune = (dir) => {
    if (!fs.existsSync(dir) || dir === DRAFT_DIR) return;
    if (!fs.readdirSync(dir).length) { fs.rmdirSync(dir); prune(path.dirname(dir)); }
  };
  for (const rel of removed) prune(path.dirname(path.join(REPO_ROOT, rel)));
  return removed;
}
