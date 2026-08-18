// The local review UI. Not part of the Patchwork binary, not served by it,
// not embedded — this is a workbench that runs on your machine while you
// write, and nothing here ships to anyone.

import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { REPO_ROOT } from './scope.js';
import { load, save, stats, decide } from './ledger.js';
import { writeback } from './writeback.js';
import { outstandingDrafts, draftDirLabel } from './drafts.js';

const HERE = path.dirname(fileURLToPath(import.meta.url));
// An explicit COPY_LEDGER_PORT is a request and is honoured or fails loudly;
// the built-in default is only a preference (see the listen block below).
const PORT_REQUESTED = !!process.env.COPY_LEDGER_PORT;
const PORT = Number(process.env.COPY_LEDGER_PORT || 5175);

function json(res, code, body) {
  const payload = JSON.stringify(body);
  res.writeHead(code, {
    'Content-Type': 'application/json',
    'Content-Length': Buffer.byteLength(payload),
    'Cache-Control': 'no-store',
  });
  res.end(payload);
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let data = '';
    req.on('data', (c) => {
      data += c;
      if (data.length > 1e6) { req.destroy(); reject(new Error('body too large')); }
    });
    req.on('end', () => {
      try { resolve(data ? JSON.parse(data) : {}); }
      catch (e) { reject(e); }
    });
  });
}

/** A few lines either side of an occurrence, so you can see what you're editing. */
function context(file, line, span = 4) {
  // Confine reads to the repo — this server is local, but a path that walks
  // out of the tree is never something the UI legitimately asks for.
  const abs = path.resolve(REPO_ROOT, file);
  if (!abs.startsWith(REPO_ROOT + path.sep)) return null;
  if (!fs.existsSync(abs)) return null;
  const lines = fs.readFileSync(abs, 'utf8').split('\n');
  const from = Math.max(0, line - 1 - span);
  const to = Math.min(lines.length, line + span);
  return { from: from + 1, lines: lines.slice(from, to) };
}

/**
 * What is checked out elsewhere, in the shape the page needs: which files,
 * and which entry ids they hold. The UI can't decide a string honestly
 * without knowing it is already being decided somewhere else.
 */
function draftState(ledger) {
  const files = outstandingDrafts(ledger);
  const checkedOut = {};
  for (const d of files) for (const id of d.ids) checkedOut[id] = d.rel;
  return {
    dir: draftDirLabel(),
    files: files.map((d) => ({
      source: d.source, rel: d.rel, blocks: d.blocks,
      written: d.dirty.length, stale: d.stale.length,
    })),
    checkedOut,
  };
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://localhost:${PORT}`);

  try {
    if (req.method === 'GET' && (url.pathname === '/' || url.pathname === '/index.html')) {
      const html = fs.readFileSync(path.join(HERE, 'ui.html'));
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' });
      return res.end(html);
    }

    if (req.method === 'GET' && url.pathname === '/api/ledger') {
      const ledger = load();
      return json(res, 200, {
        entries: ledger.entries, stats: stats(ledger), strict: !!ledger.strict,
        drafts: draftState(ledger),
      });
    }

    if (req.method === 'GET' && url.pathname === '/api/context') {
      const file = url.searchParams.get('file');
      const line = Number(url.searchParams.get('line') || 1);
      if (!file) return json(res, 400, { error: 'file required' });
      return json(res, 200, { context: context(file, line) });
    }

    if (req.method === 'POST' && url.pathname === '/api/decide') {
      const { id, status, replacement, note, force } = await readBody(req);
      const ledger = load();
      const entry = ledger.entries.find((e) => e.id === id);
      if (!entry) return json(res, 404, { error: 'no such entry' });

      // This string is checked out in a draft somewhere. Deciding it here
      // rewrites it, which retires its id, which orphans the marker holding
      // whatever is being written on the other surface. One file, one place.
      const held = force ? null : outstandingDrafts(ledger).find((d) => d.ids.includes(id));
      if (held) {
        return json(res, 409, {
          error: `checked out in ${held.rel}`,
          code: 'checked-out',
          draft: { rel: held.rel, source: held.source },
        });
      }

      decide(entry, { status, replacement, note });
      save(ledger);
      return json(res, 200, { entry, stats: stats(ledger) });
    }

    if (req.method === 'POST' && url.pathname === '/api/apply') {
      const { dryRun = true } = await readBody(req);
      const ledger = load();
      const result = writeback(ledger, { apply: !dryRun });
      if (!dryRun) save(ledger);
      return json(res, 200, {
        ...result,
        blocked: result.blocked.map((b) => ({ id: b.entry.id, text: b.entry.text, reason: b.reason })),
        stats: stats(load()),
      });
    }

    json(res, 404, { error: 'not found' });
  } catch (err) {
    json(res, 500, { error: String(err && err.message || err) });
  }
});


function announce() {
  const { port } = server.address();
  console.log(`\n  Copy review  →  \x1b[36mhttp://localhost:${port}\x1b[0m`);
  console.log(`  \x1b[2mEdits save to copy/ledger.json as you go. Ctrl-C when done.\x1b[0m\n`);
}

// Windows reserves moving port ranges for Hyper-V and WSL, so a port that
// worked yesterday starts refusing with EACCES rather than EADDRINUSE, and
// the ranges creep: 5041-5340 swallowed this default and two of the vite
// ports beside it. Asking the OS for any free port and printing what we got
// beats hand-editing a number every time the ranges shift. An explicit
// COPY_LEDGER_PORT is a request, so it fails loudly instead.
server.once('error', (err) => {
  const takeable = err.code === 'EACCES' || err.code === 'EADDRINUSE';
  if (PORT_REQUESTED || !takeable) {
    console.error(`\n  Cannot listen on 127.0.0.1:${PORT} — ${err.code}.\n`);
    process.exit(1);
  }
  console.log(`  \x1b[2m${PORT} is unavailable (${err.code}); asking for any free port.\x1b[0m`);
  server.once('error', (e) => {
    console.error(`\n  Cannot start the review server — ${e.code}.\n`);
    process.exit(1);
  });
  server.listen(0, '127.0.0.1', announce);
});

server.listen(PORT, '127.0.0.1', announce);
