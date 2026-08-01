#!/usr/bin/env node
// Copy ledger CLI. See tools/copy-ledger/README.md.
//
//   sync      re-extract from source, preserving every review decision
//   stats     progress, by tier and by file
//   review    open the local review UI
//   apply     write approved replacements into source (dry run by default)
//   check     CI gate — fails on copy that entered source unreviewed
//   report    render the ledger as a publishable transparency summary

import fs from 'node:fs';
import path from 'node:path';
import { REPO_ROOT, LEDGER_PATH } from './scope.js';
import { load, save, sync, stats, STATUSES } from './ledger.js';
import { writeback } from './writeback.js';

const [cmd, ...argv] = process.argv.slice(2);
const flag = (name) => argv.includes(`--${name}`);
const rel = (p) => path.relative(REPO_ROOT, p) || p;

const GREEN = '\x1b[32m'; const RED = '\x1b[31m';
const YEL = '\x1b[33m'; const DIM = '\x1b[2m'; const OFF = '\x1b[0m';

function pct(n, d) { return d === 0 ? '100%' : `${Math.round((n / d) * 100)}%`; }

function bar(done, total, width = 28) {
  const filled = total === 0 ? width : Math.round((done / total) * width);
  return `${'█'.repeat(filled)}${'░'.repeat(Math.max(0, width - filled))}`;
}

// ------------------------------------------------------------------ sync ---

function cmdSync() {
  const before = load();
  const { ledger, added, removed } = sync(before);

  if (flag('rebaseline') || !before.baseline) {
    ledger.baseline = ledger.entries
      .filter((e) => e.status === 'unreviewed').map((e) => e.id);
    console.log(`${YEL}baseline stamped:${OFF} ${ledger.baseline.length} strings accepted as existing debt.`);
    console.log(`${DIM}New copy added after this point must be reviewed before CI passes.${OFF}`);
  } else {
    ledger.baseline = before.baseline;
  }

  save(ledger);
  const s = stats(ledger);
  console.log(`${GREEN}synced${OFF} ${rel(LEDGER_PATH)} — ${s.total} entries, ${s.words.toLocaleString()} words`);
  if (added.length) console.log(`  ${added.length} new`);
  if (removed.length) console.log(`  ${removed.length} no longer in source`);
}

// ----------------------------------------------------------------- stats ---

function cmdStats() {
  const ledger = load();
  const s = stats(ledger);

  console.log(`\n  ${bar(s.done, s.total)}  ${s.done}/${s.total} strings (${pct(s.done, s.total)})`);
  console.log(`  ${DIM}${s.wordsDone.toLocaleString()} of ${s.words.toLocaleString()} words decided${OFF}\n`);

  for (const tier of ['prose', 'helper', 'label']) {
    const t = s.byTier[tier];
    const label = { prose: 'paragraphs (16+ words)', helper: 'helper text (6–15)', label: 'labels (2–5)' }[tier];
    console.log(`  ${label.padEnd(24)} ${bar(t.done, t.total, 18)} ${String(t.done).padStart(4)}/${t.total}`);
  }

  console.log('');
  for (const st of STATUSES) {
    if (!s.byStatus[st]) continue;
    console.log(`  ${st.padEnd(12)} ${s.byStatus[st]}`);
  }

  // Where the remaining work lives.
  const byFile = new Map();
  for (const e of ledger.entries) {
    if (e.status !== 'unreviewed') continue;
    for (const o of e.occurrences) {
      byFile.set(o.file, (byFile.get(o.file) || 0) + e.words);
    }
  }
  const top = [...byFile.entries()].sort((a, b) => b[1] - a[1]).slice(0, 12);
  if (top.length) {
    console.log(`\n  ${DIM}most unreviewed words:${OFF}`);
    for (const [file, words] of top) console.log(`    ${String(words).padStart(5)}  ${file}`);
  }
  console.log('');
}

// ----------------------------------------------------------------- apply ---

function cmdApply() {
  const ledger = load();
  const apply = flag('apply');
  const r = writeback(ledger, { apply });

  if (!r.entries && !r.blocked.length) {
    console.log('Nothing to apply. Mark entries as `rewritten` in the review UI first.');
    return;
  }

  for (const f of r.files) console.log(`  ${f.file} ${DIM}(${f.edits} occurrence${f.edits === 1 ? '' : 's'})${OFF}`);

  if (r.blocked.length) {
    console.log(`\n${RED}blocked — ${r.blocked.length} entr${r.blocked.length === 1 ? 'y' : 'ies'} not applied:${OFF}`);
    for (const b of r.blocked) {
      console.log(`  ${DIM}${b.entry.occurrences[0].file}:${b.entry.occurrences[0].line}${OFF}`);
      console.log(`    "${b.entry.text.slice(0, 70)}${b.entry.text.length > 70 ? '…' : ''}"`);
      console.log(`    ${RED}${b.reason}${OFF}`);
    }
  }

  if (apply) {
    save(ledger);
    console.log(`\n${GREEN}applied${OFF} ${r.entries} entries across ${r.occurrences} occurrences.`);
    console.log(`${DIM}Ledger updated: replaced drafts kept as previous_ai_text.${OFF}`);
    console.log(`${DIM}Review the diff before committing — writeback edits source.${OFF}`);
  } else {
    console.log(`\n${YEL}dry run${OFF} — ${r.entries} entries / ${r.occurrences} occurrences would change.`);
    console.log(`Re-run with ${GREEN}--apply${OFF} to write.`);
  }
}

// ----------------------------------------------------------------- check ---

function cmdCheck() {
  if (!fs.existsSync(LEDGER_PATH)) {
    console.error(`${RED}no ledger${OFF} at ${rel(LEDGER_PATH)} — run: make copy-sync`);
    process.exit(1);
  }

  const onDisk = load();
  const { ledger: fresh, added } = sync(onDisk);
  const baseline = new Set(onDisk.baseline || []);

  // The ratchet: copy that entered source without a review decision, and
  // that isn't part of the debt we explicitly accepted at baseline.
  const unledgered = fresh.entries.filter(
    (e) => e.status === 'unreviewed' && !baseline.has(e.id));

  const backlog = fresh.entries.filter(
    (e) => e.status === 'unreviewed' && baseline.has(e.id));
  const pendingApply = fresh.entries.filter((e) => e.status === 'rewritten');

  if (backlog.length) {
    console.log(`${DIM}backlog: ${backlog.length} strings still unreviewed from baseline (not a failure)${OFF}`);
  }
  if (pendingApply.length) {
    console.log(`${YEL}${pendingApply.length} rewritten but not applied${OFF} — run: make copy-apply`);
  }

  if (onDisk.strict && backlog.length) {
    console.error(`\n${RED}strict mode: ${backlog.length} strings still unreviewed.${OFF}`);
    process.exit(1);
  }

  if (!unledgered.length) {
    console.log(`${GREEN}✓${OFF} every string in source has an author on record.`);
    return;
  }

  console.error(`\n${RED}✗ ${unledgered.length} new string${unledgered.length === 1 ? '' : 's'} entered source without a review decision:${OFF}\n`);
  for (const e of unledgered.slice(0, 25)) {
    const o = e.occurrences[0];
    console.error(`  ${o.file}:${o.line}`);
    console.error(`    "${e.text.slice(0, 88)}${e.text.length > 88 ? '…' : ''}"`);
  }
  if (unledgered.length > 25) console.error(`  ${DIM}…and ${unledgered.length - 25} more${OFF}`);

  console.error(`\n  Decide each one, then commit the ledger:`);
  console.error(`    make copy-review      ${DIM}# write your own version, or mark it as-is${OFF}`);
  console.error(`    make copy-apply       ${DIM}# write approved text into source${OFF}\n`);
  process.exit(1);
}

// ---------------------------------------------------------------- report ---

function cmdReport() {
  const ledger = load();
  const s = stats(ledger);
  const replaced = ledger.entries.filter((e) => e.previous_ai_text);
  const out = path.join(REPO_ROOT, 'copy', 'REPORT.md');

  const lines = [];
  lines.push('# Copy provenance');
  lines.push('');
  lines.push('Generated by `make copy-report` from `copy/ledger.json`. It counts every');
  lines.push('visitor-readable string in this repository and records who wrote it.');
  lines.push('');
  lines.push('| | strings | words |');
  lines.push('|---|---:|---:|');
  lines.push(`| Written by a person | ${s.byStatus.human} | ${s.wordsDone.toLocaleString()} |`);
  lines.push(`| Left as drafted, deliberately | ${s.byStatus['ai-fine']} | |`);
  lines.push(`| Not yet reviewed | ${s.byStatus.unreviewed} | |`);
  lines.push(`| **Total** | **${s.total}** | **${s.words.toLocaleString()}** |`);
  lines.push('');
  lines.push(`Human-authored or deliberately accepted: **${pct(s.done, s.total)}** of strings.`);
  lines.push('');
  lines.push('## By tier');
  lines.push('');
  lines.push('| Tier | Decided | Total |');
  lines.push('|---|---:|---:|');
  for (const tier of ['prose', 'helper', 'label']) {
    const t = s.byTier[tier];
    const name = { prose: 'Paragraphs (16+ words)', helper: 'Helper text (6–15 words)', label: 'Labels (2–5 words)' }[tier];
    lines.push(`| ${name} | ${t.done} | ${t.total} |`);
  }
  lines.push('');
  lines.push('## Out of scope');
  lines.push('');
  lines.push('- `docs/adr/` — architecture decision records. Engineering reasoning, not');
  lines.push('  the project\'s voice; drafted with AI assistance and left that way.');
  lines.push('- `CODE_OF_CONDUCT.md` — the Contributor Covenant, an external standard text.');
  lines.push('- Source comments, tests, commit messages.');
  lines.push('');
  lines.push(`## Replaced drafts (${replaced.length})`);
  lines.push('');
  if (!replaced.length) {
    lines.push('_None yet._');
  } else {
    lines.push('Where a model\'s draft was replaced by a person, both are kept.');
    lines.push('');
    for (const e of replaced.slice(0, 400)) {
      lines.push(`- \`${e.occurrences[0]?.file || '—'}\``);
      lines.push(`  - draft: ${JSON.stringify(e.previous_ai_text)}`);
      lines.push(`  - now: ${JSON.stringify(e.text)}`);
    }
  }
  lines.push('');

  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.writeFileSync(out, lines.join('\n'));
  console.log(`${GREEN}wrote${OFF} ${rel(out)}`);
}

// ------------------------------------------------------------------------ ---

const COMMANDS = {
  sync: cmdSync, stats: cmdStats, apply: cmdApply, check: cmdCheck, report: cmdReport,
  review: async () => { await import('./serve.js'); },
};

if (!COMMANDS[cmd]) {
  console.error('usage: node tools/copy-ledger/cli.js <sync|stats|review|apply|check|report>');
  process.exit(1);
}
await COMMANDS[cmd]();
