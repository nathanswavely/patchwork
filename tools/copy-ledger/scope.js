// What the copy ledger looks at, and what it deliberately ignores.
//
// Scope is a policy decision, not a technical one. The ledger's promise is
// "every word a visitor can read has a named author", so the list below is
// the set of files that produce visitor-readable words. Engineering records
// (docs/adr/*) are excluded on purpose: they are the project's reasoning,
// not its voice, and rewriting 50k words of decision history by hand would
// buy nothing. The colophon says so plainly rather than the ledger pretending
// otherwise.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const REPO_ROOT = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)), '..', '..');

export const LEDGER_PATH = path.join(REPO_ROOT, 'copy', 'ledger.json');

// Directory trees walked for Svelte/JS sources.
const WEB_SRC = 'web/src';

// Go files carrying prose a user reads. Not every Go file — only the ones
// that hold shipped text. `_test.go` is excluded by the walker.
const GO_FILES = [
  'internal/handler/legal_defaults.go',   // privacy policy + user agreement
  'internal/governance/lining.go',        // the baseline charter's lineage
  'internal/governance/defaults.go',      // template descriptions + charters
  'internal/notifications/types.go',      // notification titles/bodies
  'internal/notifications/email.go',      // email subjects/bodies
  'internal/notifications/reminders.go',
  'internal/notifications/inactivity.go',
];

// Markdown a newcomer actually reads. ADRs are excluded (see note above).
//
// CODE_OF_CONDUCT.md is deliberately absent: it is the Contributor Covenant,
// authored by neither a person here nor a model, and rewriting a standard
// text to sound like us would defeat the point of adopting a standard text.
const MD_FILES = [
  'README.md',
  'docs/START-A-QUILT.md',
  'CONTRIBUTING.md',
];

// Go constants whose text is frozen by contract and must never be rewritten.
//
// These are not "copy we decided to keep" — they are hash-matched historical
// records. `AutoUpdateLinings` classifies a patch's lining by comparing it
// against these exact bytes to decide whether it is stale and should heal
// (docs/adr/037, internal/governance/lining.go). Change one character and
// every patch still carrying that text stops being recognised, silently
// stops healing, and starts wearing an "Amended lining" badge it never
// earned. So they are kept out of the queue entirely: you cannot rewrite
// what the tool never offers you. `copy-report` lists them as excluded
// rather than pretending they don't exist.
export const FROZEN_GO_CONSTS = new Set([
  'legacyLiningOriginal',
  'legacyLiningHumanized',
]);

// Substrings that mark a Svelte/JS file as machinery rather than voice.
const SKIP_PATH = [
  '/node_modules/', '/dist/', '.test.js', '.spec.js', '/e2e/',
];

function walk(dir, out = []) {
  let entries;
  try { entries = fs.readdirSync(dir, { withFileTypes: true }); }
  catch { return out; }
  for (const e of entries) {
    const full = path.join(dir, e.target || e.name);
    if (e.isDirectory()) walk(full, out);
    else out.push(full);
  }
  return out;
}

/** Every in-scope file, as {file (repo-relative), lang}. */
export function sourceFiles() {
  const out = [];
  const webRoot = path.join(REPO_ROOT, WEB_SRC);
  for (const abs of walk(webRoot)) {
    const rel = path.relative(REPO_ROOT, abs).split(path.sep).join('/');
    if (SKIP_PATH.some((s) => `/${rel}`.includes(s))) continue;
    if (rel.endsWith('.svelte')) out.push({ file: rel, lang: 'svelte' });
    else if (rel.endsWith('.js')) out.push({ file: rel, lang: 'js' });
  }
  for (const rel of GO_FILES) {
    if (fs.existsSync(path.join(REPO_ROOT, rel))) out.push({ file: rel, lang: 'go' });
  }
  for (const rel of MD_FILES) {
    if (fs.existsSync(path.join(REPO_ROOT, rel))) out.push({ file: rel, lang: 'md' });
  }
  out.sort((a, b) => a.file.localeCompare(b.file));
  return out;
}
