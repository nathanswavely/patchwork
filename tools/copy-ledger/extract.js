// Pull every visitor-readable string out of the sources named in scope.js.
//
// Two texts are recorded for each hit, and the difference matters:
//
//   entry.text        the collapsed, readable form — what you review, and
//                     what the entry's ID hashes. Reflowing a paragraph in
//                     source does not orphan its review status.
//   occurrence.raw    the exact source substring — what writeback replaces.
//                     Keeping it verbatim is what makes writeback safe: it
//                     either finds an exact match or reports drift and
//                     refuses, and never guesses.
//
// Identical text in twelve files collapses to ONE entry with twelve
// occurrences. You decide "Save changes" once.

import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { REPO_ROOT, sourceFiles, FROZEN_GO_CONSTS } from './scope.js';

// Attributes whose values a person reads.
const COPY_ATTRS = ['placeholder', 'title', 'aria-label', 'alt', 'aria-description'];

// A JS/Go string literal has to clear all of these to count as prose. The
// filter is deliberately loose — a false positive costs one keystroke to
// mark `ai-fine` forever, while a false negative is a string that silently
// never gets reviewed. Over-collect on purpose.
const CODEish = new RegExp([
  '[<>{}=_|\\\\]',                    // markup, template or path syntax
  '^https?:', '\\/\\/',               // URLs
  '\\.(js|go|svelte|css|json|md)$',   // filenames
  '^[a-z-]+\\/[a-z-]+$',              // mime types, route fragments
  '\\b\\d+(px|rem|em|vh|vw|ch)\\b',   // CSS lengths
  '\\((min|max)-(width|height)',      // media queries
].join('|'), 'i');

export function collapse(s) {
  return s.replace(/\s+/g, ' ').trim();
}

export function idFor(text) {
  return crypto.createHash('sha256').update(collapse(text)).digest('hex').slice(0, 12);
}

export function tierFor(text) {
  const n = collapse(text).split(/\s+/).filter(Boolean).length;
  if (n < 6) return 'label';
  if (n < 16) return 'helper';
  return 'prose';
}

/**
 * Is the raw string starting at `index` assigned to a frozen constant?
 * Looks back for the `const NAME = ` that introduces it.
 */
function isFrozen(src, index) {
  const before = src.slice(Math.max(0, index - 200), index);
  const decl = before.match(/(?:const|var)\s+(\w+)\s*(?:=|string\s*=)\s*$/);
  return !!decl && FROZEN_GO_CONSTS.has(decl[1]);
}

/**
 * Is this raw string embedded data rather than prose? `defaults.go` ships
 * governance-rules.json templates inside backticks, and a JSON blob has no
 * author's voice to rewrite — it is machine configuration, which this
 * project already treats as a different kind of thing (docs/adr/053).
 */
function isData(body) {
  const t = body.trim();
  if (/^[[{]/.test(t)) return true;
  const kv = (t.match(/"[\w.-]+"\s*:/g) || []).length;
  return kv >= 3;
}

/** Replace every match with spaces, preserving newlines so line numbers hold. */
function blank(src, re) {
  return src.replace(re, (m) => m.replace(/[^\n]/g, ' '));
}

function lineAt(src, index) {
  let line = 1;
  for (let i = 0; i < index && i < src.length; i++) if (src[i] === '\n') line++;
  return line;
}

/** Is this chunk Svelte control flow or a bare expression rather than prose? */
// A block directive: {#if …}, {:else}, {/each}. Not copy, and not a boundary
// the text-node regex knows about — it stops at the next `<`, so an element
// wrapping a conditional yields one chunk with the directives inside it.
const BLOCK_DIRECTIVE = /\{[#:/][^{}]*\}/g;

/**
 * Split a text-node chunk at its block directives, keeping each segment's
 * offset within the chunk so writeback still points at real source.
 *
 * Everything a conditional wraps used to be discarded: `isMarkupNoise`
 * rejected any chunk containing `{#`, which is correct about the directive
 * and wrong about the sentences either side of it. A term line reading
 * "This council's term ended … / Next seat comes up …" lost both branches at
 * once, and neither was ever offered for review.
 */
function splitOnDirectives(chunk) {
  const out = [];
  let last = 0;
  for (const m of chunk.matchAll(BLOCK_DIRECTIVE)) {
    if (m.index > last) out.push({ text: chunk.slice(last, m.index), offset: last });
    last = m.index + m[0].length;
  }
  if (last < chunk.length) out.push({ text: chunk.slice(last), offset: last });
  return out;
}

// Residue of an attribute or an expression rather than a text node.
//
// The text-node regex stops at the next `<`, so an attribute containing a `>`
// — `onclick={() => {}}`, an arrow, a template literal — leaves the scan
// mid-tag, and the "text" after it is really code. Blanket-rejecting every
// chunk with a block directive hid most of that by accident; once directives
// stop discarding their neighbours it has to be named.
//
// Tested against the chunk with balanced `{…}` expressions already removed,
// never the raw chunk. A backtick or a brace *inside* an expression is
// ordinary interpolated copy — "…against the community lining.{entityName ?
// ` You're reporting ${entityName}.` : ''}" is a human-written string in this
// repo, and matching on the raw chunk threw it away.
const ATTR_RESIDUE = /[{}]|=>|="|\/>|`/;

/**
 * The chunk with its `{…}` expressions removed — to a fixed point, so nested
 * ones go too. A single pass strips only the innermost group and leaves the
 * outer braces, which reads as unbalanced code to anything checking after it:
 * `{entityName ? ` … ${entityName}.` : ''}` is one expression, not two.
 */
function stripExpressions(chunk) {
  let out = chunk;
  for (;;) {
    const next = out.replace(/\{[^{}]*\}/g, '');
    if (next === out) return out;
    out = next;
  }
}

function isMarkupNoise(chunk) {
  if (/\{[#:/]/.test(chunk)) return true;             // {#if} {:else} {/each}
  const literal = stripExpressions(chunk).trim();
  if (ATTR_RESIDUE.test(literal)) return true;        // attribute or JS, not text
  if (!literal) return true;                           // pure interpolation
  const words = literal.split(/\s+/).filter((w) => /[A-Za-z]{2}/.test(w));
  if (words.length < 1) return true;
  // A lone symbol or unit fragment left over after stripping expressions.
  if (literal.length < 3) return true;
  return false;
}

// Kinds whose line breaks carry meaning. Collapsing a Markdown list to one
// line turns four bullets into one paragraph, and writeback would then put
// that flattened line back into the file. HTML and string literals have no
// such structure, so those stay collapsed.
const BLOCK_KINDS = new Set(['md-block', 'go-doc-block']);

function pushHit(hits, { file, raw, index, src, kind, quote }) {
  const text = BLOCK_KINDS.has(kind) ? raw.replace(/\s+$/, '') : collapse(raw);
  if (!collapse(text)) return;
  hits.push({ file, raw, text, line: lineAt(src, index), kind, quote: quote || null });
}

// ---------------------------------------------------------------- Svelte ---

function extractSvelte(file, src) {
  const hits = [];

  // Script and style are handled separately / not at all; comments never.
  const markup = blank(
    blank(blank(src, /<script[\s\S]*?<\/script>/g), /<style[\s\S]*?<\/style>/g),
    /<!--[\s\S]*?-->/g);

  // Text nodes: everything between a closing '>' and the next '<'.
  const textNode = />([^<]+)</g;
  let m;
  while ((m = textNode.exec(markup)) !== null) {
    const chunk = m[1];
    if (!collapse(chunk)) continue;
    // One segment when there is no conditional, several when there is — each
    // still an exact substring of the source, so writeback lands where the
    // words actually are.
    for (const seg of splitOnDirectives(chunk)) {
      const raw = seg.text;
      if (!collapse(raw) || isMarkupNoise(raw)) continue;
      // Trim surrounding whitespace out of `raw` so writeback preserves layout.
      const lead = raw.match(/^\s*/)[0].length;
      const trail = raw.match(/\s*$/)[0].length;
      const inner = raw.slice(lead, raw.length - trail);
      if (!inner) continue;
      pushHit(hits, {
        file, raw: inner, index: m.index + 1 + seg.offset + lead, src, kind: 'markup',
      });
    }
  }

  // Copy-bearing attributes with static values.
  for (const attr of COPY_ATTRS) {
    const re = new RegExp(`\\b${attr}\\s*=\\s*(["'])([^"'{}]*?)\\1`, 'g');
    while ((m = re.exec(markup)) !== null) {
      const val = m[2];
      if (!collapse(val) || !/[A-Za-z]{2}/.test(val)) continue;
      pushHit(hits, {
        file, raw: val, index: m.index, src, kind: `attr:${attr}`, quote: m[1],
      });
    }
  }

  // Strings inside <script> — toasts, error messages, option labels.
  const scriptRe = /<script[^>]*>([\s\S]*?)<\/script>/g;
  while ((m = scriptRe.exec(src)) !== null) {
    const offset = m.index + m[0].indexOf(m[1]);
    for (const h of extractJsStrings(file, m[1], src, offset)) hits.push(h);
  }

  return hits;
}

// -------------------------------------------------------------------- JS ---

function extractJsStrings(file, code, fullSrc, offset = 0) {
  const hits = [];
  // Skip line/block comments so commentary never enters the ledger.
  const clean = blank(blank(code, /\/\*[\s\S]*?\*\//g), /(^|[^:])\/\/[^\n]*/g);
  const re = /(['"`])((?:[^\\\n]|\\.)*?)\1/g;
  let m;
  while ((m = re.exec(clean)) !== null) {
    const val = m[2];
    if (val.length < 8 || !/\s/.test(val)) continue;       // one-word → not prose
    if (!/[A-Za-z]{3}/.test(val)) continue;
    if (CODEish.test(val)) continue;
    if (!/^[A-Z(“'"]/.test(val.trim())) continue;          // sentence-shaped only
    pushHit(hits, {
      file, raw: val, index: offset + m.index + 1, src: fullSrc,
      kind: 'js-string', quote: m[1],
    });
  }
  return hits;
}

function extractJs(file, src) {
  return extractJsStrings(file, src, src, 0);
}

// -------------------------------------------------------------------- Go ---

/**
 * API error bodies: `{"error":"..."}` handed to http.Error.
 *
 * These need their own pass because neither pass around them can see them.
 * The raw-string pass skips the whole literal as embedded data (it starts
 * with `{`), and the interpreted-string pass never runs inside a backtick
 * literal — and would reject these anyway, since it requires a capital first
 * letter while 366 of the 367 in this repo start lowercase, by the convention
 * that API messages read as fragments rather than sentences.
 *
 * A visitor reads them. The SPA renders the `error` field into toasts and
 * inline form errors, so a refusal like "add a short description of the
 * image" is copy in exactly the sense the ledger means. It went uncounted
 * because the Go scope was a list of prose files and no handler was on it.
 *
 * Only the message is captured, never the JSON around it: `raw` is the
 * message substring, so writeback replaces the words and leaves the wrapper
 * intact.
 */
function extractGoErrors(file, src) {
  const hits = [];
  const clean = blank(blank(src, /\/\*[\s\S]*?\*\//g), /(^|[^:])\/\/[^\n]*/g);
  const errRe = /`\{"error":"((?:[^"\\`])*)"\}`/g;
  let m;
  while ((m = errRe.exec(clean)) !== null) {
    const val = m[1];
    if (!/[A-Za-z]{3}/.test(val) || !/\s/.test(val)) continue;
    pushHit(hits, {
      file, raw: val, index: m.index + '`{"error":"'.length, src, kind: 'go-error-body',
    });
  }
  return hits;
}

function extractGo(file, src) {
  const hits = [];
  const clean = blank(blank(src, /\/\*[\s\S]*?\*\//g), /(^|[^:])\/\/[^\n]*/g);

  // Raw (backtick) strings hold the long-form documents. Split them into
  // paragraph blocks so review happens a paragraph at a time rather than
  // in one 1,700-word textarea.
  const rawRe = /`([^`]*)`/g;
  let m;
  while ((m = rawRe.exec(clean)) !== null) {
    const body = m[1];
    if (!/[A-Za-z]{3}/.test(body)) continue;
    if (isFrozen(clean, m.index)) continue;   // hash-matched record, never rewritten
    const base = m.index + 1;
    if (isData(body)) continue;   // embedded JSON/config, not prose
    if (body.length > 200) {
      for (const b of blocks(body)) {
        if (!/[A-Za-z]{3}/.test(b.text) || isData(b.text)) continue;
        pushHit(hits, {
          file, raw: b.text, index: base + b.offset, src, kind: 'go-doc-block',
        });
      }
    } else if (/\s/.test(collapse(body))) {
      pushHit(hits, { file, raw: body, index: base, src, kind: 'go-raw-string' });
    }
  }

  hits.push(...extractGoErrors(file, src));

  // Interpreted strings: struct fields (Description, BestFor), notification
  // titles, email subjects.
  const strRe = /"((?:[^"\\\n]|\\.)*)"/g;
  const withoutRaw = blank(clean, /`[^`]*`/g);
  while ((m = strRe.exec(withoutRaw)) !== null) {
    const val = m[1];
    if (val.length < 8 || !/\s/.test(val)) continue;
    if (!/[A-Za-z]{3}/.test(val)) continue;
    if (CODEish.test(val)) continue;
    if (!/^[A-Z(“'"]/.test(val.trim())) continue;
    pushHit(hits, {
      file, raw: val, index: m.index + 1, src, kind: 'go-string', quote: '"',
    });
  }

  return hits;
}

// -------------------------------------------------------------- Markdown ---

/** Split prose into blank-line-separated blocks, skipping fenced code. */
function blocks(body) {
  const out = [];
  const lines = body.split('\n');
  let buf = [];
  let bufStart = 0;
  let offset = 0;
  let inFence = false;

  const flush = () => {
    if (!buf.length) { buf = []; return; }
    const text = buf.join('\n').replace(/\s+$/, '');
    if (text.trim()) out.push({ text: text.replace(/^\s+/, ''), offset: bufStart });
    buf = [];
  };

  for (const line of lines) {
    if (/^\s*```/.test(line)) { flush(); inFence = !inFence; offset += line.length + 1; continue; }
    if (inFence) { offset += line.length + 1; continue; }
    if (!line.trim()) { flush(); offset += line.length + 1; continue; }
    if (!buf.length) bufStart = offset + (line.length - line.replace(/^\s+/, '').length);
    buf.push(line);
    offset += line.length + 1;
  }
  flush();
  return out;
}

function extractMd(file, src) {
  const hits = [];
  const clean = blank(src, /<!--[\s\S]*?-->/g);
  for (const b of blocks(clean)) {
    if (!/[A-Za-z]{3}/.test(b.text)) continue;
    // Badge rows, link-reference lines and bare images are chrome, not copy.
    if (/^\s*[!\[]/.test(b.text) && b.text.length < 200) continue;
    pushHit(hits, { file, raw: b.text, index: b.offset, src, kind: 'md-block' });
  }
  return hits;
}

// ----------------------------------------------------------------- driver ---

const EXTRACTORS = {
  svelte: extractSvelte, js: extractJs, go: extractGo, md: extractMd,
  // Handlers come in for their API error messages and nothing else — their
  // other strings are SQL, log lines and column names.
  'go-errors': extractGoErrors,
};

/**
 * Hits from one source, without touching disk. Exported so a test can hand
 * the extractor a few lines instead of asserting against the repo, which
 * would make every test a hostage to whatever the handlers happen to say.
 */
export function extractOne(file, lang, src) {
  return EXTRACTORS[lang](file, src);
}

/** Every hit across every in-scope file, unmerged. */
export function extractAll() {
  const hits = [];
  for (const { file, lang } of sourceFiles()) {
    const src = fs.readFileSync(path.join(REPO_ROOT, file), 'utf8');
    for (const h of EXTRACTORS[lang](file, src)) hits.push(h);
  }
  return hits;
}

/** Hits folded into ledger entries, keyed by the hash of the collapsed text. */
export function extractEntries() {
  const byId = new Map();
  for (const h of extractAll()) {
    const id = idFor(h.text);
    if (!byId.has(id)) {
      byId.set(id, {
        id,
        text: h.text,
        words: h.text.split(/\s+/).filter(Boolean).length,
        tier: tierFor(h.text),
        status: 'unreviewed',
        replacement: null,
        note: null,
        occurrences: [],
      });
    }
    const e = byId.get(id);
    if (!e.occurrences.some((o) => o.file === h.file && o.line === h.line)) {
      e.occurrences.push({ file: h.file, line: h.line, kind: h.kind, raw: h.raw, quote: h.quote });
    }
  }
  for (const e of byId.values()) {
    e.occurrences.sort((a, b) => a.file.localeCompare(b.file) || a.line - b.line);
  }
  return [...byId.values()].sort(
    (a, b) => a.occurrences[0].file.localeCompare(b.occurrences[0].file)
           || a.occurrences[0].line - b.occurrences[0].line);
}
