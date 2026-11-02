import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { resolve, relative, join } from 'node:path';

// Every read of a paged list, asked how it gets past the first page.
//
// Every list endpoint is cursor-paged: twenty rows unless asked, a hundred at
// most, the rest behind `next_cursor` (parsePaginationParams). A screen that
// reads one page and treats it as the whole list reads correctly while the
// list is short and quietly lies once it is not. On 2026-11-02 eight screens
// were doing it at once: a venue's Events tab showed its 20 oldest of 264
// events, a search inside a patch never found row 21, and an admin could not
// manage anyone past the twentieth member.
//
// So, in the style of the route walk (TestEveryMutatingRouteRefusesAStranger):
// every frontend `api(` GET of an endpoint in paged-endpoints.json must do one
// of three things, or be named below with a reason.
//
//   1. Follow the cursor: the code right after the call reads `next_cursor`
//      (a "Load more", a walk). Going through `allPages`, `allMembers` or
//      `allMyMemberships` does this by construction and never calls `api(`.
//   2. Ask a narrowing question: `from=`/`to=` (a date range it shows),
//      `search=` (the server finds the matches), or `limit=1` (a probe that
//      asks whether anything exists, or reads a count off the payload).
//   3. Be in ALLOWED with a one-line reason, for a small limit that is the
//      point (a glimpse, a dropdown of the newest ten).
//
// paged-endpoints.json is kept in step with the Go handlers by
// TestPagedEndpointListMatchesTheHandlers (cmd/patchwork), so a new paged
// handler reaches this test without anybody remembering to add it.

const SRC = resolve(__dirname, '..');

const ALLOWED = [
  {
    file: 'components/NotificationDropdown.svelte',
    call: 'notifications?limit=10',
    reason: 'the bell shows the ten newest and links to /notifications, which pages',
  },
  {
    file: 'components/PatchProfileGlimpses.svelte',
    call: '/members?limit=12',
    reason: 'a glimpse of a few faces; the Members tab beside it pages',
  },
  {
    file: 'components/PatchProfileGlimpses.svelte',
    call: '/proposals?limit=3',
    reason: 'a glimpse of the three newest, which states the total it is drawn from',
  },
  {
    file: 'pages/Dashboard.svelte',
    call: '/proposals?status=open&limit=',
    reason: 'reads only awaiting_your_vote, which the server counts across the whole patch',
  },
];

const endpoints = JSON.parse(readFileSync(resolve(__dirname, 'paged-endpoints.json'), 'utf8')).endpoints;

// "GET /api/v1/nodes/{slug}/members" -> /^nodes\/[^/]+\/members$/
const pagedRoutes = endpoints.map((e) => {
  const path = e.replace(/^GET \/api\/v1\//, '');
  const re = new RegExp('^' + path.split('/').map((seg) =>
    /^\{.*\}$/.test(seg) ? '[^/]+' : seg.replace(/[.*+?^$()|[\]\\]/g, '\\$&'),
  ).join('/') + '$');
  return { endpoint: e, re };
});

function sourceFiles(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      if (name === 'test' || name === 'vendor') continue;
      out.push(...sourceFiles(full));
    } else if (/\.(svelte|js)$/.test(name) && !/\.test\.js$/.test(name)) {
      out.push(full);
    }
  }
  return out;
}

// Comments blanked to spaces, keeping every newline, so a comment quoting an
// old call is not mistaken for one and line numbers still mean something.
function stripComments(src) {
  const blank = (m) => m.replace(/[^\n]/g, ' ');
  return src
    .replace(/<!--[\s\S]*?-->/g, blank)
    .replace(/\/\*[\s\S]*?\*\//g, blank)
    .replace(/^[ \t]*\/\/.*$/gm, blank);
}

// Reads a string or template literal starting at src[i], returning its raw
// text with every ${...} replaced by \0, and the index after it.
function readLiteral(src, i) {
  const quote = src[i];
  let j = i + 1;
  let text = '';
  while (j < src.length && src[j] !== quote) {
    if (src[j] === '\\') {
      text += src[j + 1];
      j += 2;
    } else if (quote === '`' && src[j] === '$' && src[j + 1] === '{') {
      let depth = 1;
      j += 2;
      while (j < src.length && depth > 0) {
        if (src[j] === '{') depth++;
        else if (src[j] === '}') depth--;
        else if (src[j] === '`') {
          j = readLiteral(src, j).end - 1;
        }
        j++;
      }
      text += '\0';
    } else {
      text += src[j];
      j++;
    }
  }
  return { text, end: j + 1 };
}

// Index just past the ')' closing the call whose '(' is at open.
function callEnd(src, open) {
  let depth = 0;
  for (let j = open; j < src.length; j++) {
    const c = src[j];
    if (c === '`' || c === "'" || c === '"') {
      j = readLiteral(src, j).end - 1;
    } else if (c === '(') depth++;
    else if (c === ')') {
      depth--;
      if (depth === 0) return j + 1;
    }
  }
  return src.length;
}

function apiCalls() {
  const calls = [];
  for (const full of sourceFiles(SRC)) {
    const file = relative(SRC, full).replace(/\\/g, '/');
    const src = stripComments(readFileSync(full, 'utf8'));
    const re = /\bapi\(/g;
    let m;
    while ((m = re.exec(src))) {
      if (/function\s+$/.test(src.slice(Math.max(0, m.index - 20), m.index))) continue;
      const open = m.index + 3;
      const end = callEnd(src, open);
      const args = src.slice(open + 1, end - 1);
      const line = src.slice(0, m.index).split('\n').length;

      let k = 0;
      while (/\s/.test(args[k] || '')) k++;
      const first = args[k];
      let path = null;
      let rest = args;
      if (first === '`' || first === "'" || first === '"') {
        const lit = readLiteral(args, k);
        path = lit.text;
        rest = args.slice(lit.end);
      }
      const method = (rest.match(/method:\s*['"`](\w+)/) || [])[1] || 'GET';
      if (method.toUpperCase() !== 'GET') continue;

      const after = src.slice(end).split('\n').slice(0, 12).join('\n');
      calls.push({ file, line, path, followsCursor: /next_cursor/.test(after) });
    }
  }
  return calls;
}

function routeOf(path) {
  if (path === null || path.startsWith('\0')) return null;
  const [route] = path.split('?');
  return route.replace(/\0$/, '');
}

function narrows(path) {
  const query = path.includes('?') ? path.slice(path.indexOf('?')) : '';
  return /[?&](from|to|search)=/.test(query) || /[?&]limit=1(?![0-9\0])/.test(query);
}

function allowedEntry(call) {
  return ALLOWED.find((a) => a.file === call.file && call.path !== null && call.path.includes(a.call));
}

const calls = apiCalls();

const paged = calls.filter((c) => {
  const route = routeOf(c.path);
  return route !== null && pagedRoutes.some((p) => p.re.test(route));
});

// A path that starts with an expression could be any endpoint at all. The
// one that exists is the walker's own call, which reads the cursor.
const dynamic = calls.filter((c) => routeOf(c.path) === null);

function describeCall(c) {
  return `${c.file}:${c.line} api(${c.path === null ? '<expression>' : JSON.stringify(c.path.replace(/\0/g, '${…}'))})`;
}

describe('every read of a paged list gets past the first page', () => {
  it('finds the paged reads it is meant to check', () => {
    // A parse that silently found nothing would pass every test below.
    expect(calls.length).toBeGreaterThan(100);
    expect(paged.length).toBeGreaterThan(15);
    expect(paged.some((c) => c.file === 'pages/Notifications.svelte')).toBe(true);
  });

  it('follows next_cursor, narrows the question, or says why not', () => {
    const offenders = paged
      .filter((c) => !c.followsCursor && !narrows(c.path) && !allowedEntry(c))
      .map(describeCall);
    expect(offenders, 'read one page of a paged list: page it (allPages, or "Load more" on next_cursor), ask for a range or a search, or add it to ALLOWED with a reason').toEqual([]);
  });

  it('never calls api() on a path it cannot name unless it reads the cursor', () => {
    const offenders = dynamic.filter((c) => !c.followsCursor).map(describeCall);
    expect(offenders).toEqual([]);
  });

  it('keeps no allowlist entry that no longer excuses anything', () => {
    const stale = ALLOWED.filter((a) => !paged.some((c) =>
      allowedEntry(c) === a && !c.followsCursor && !narrows(c.path),
    )).map((a) => `${a.file}: ${a.call}`);
    expect(stale, 'remove these from ALLOWED: the call they named is gone or already pages').toEqual([]);
  });

  it('gives every allowlist entry a reason', () => {
    for (const a of ALLOWED) expect(a.reason.length).toBeGreaterThan(10);
  });
});

describe('the screens this test was written for', () => {
  const read = (p) => readFileSync(resolve(SRC, p), 'utf8');

  it('the workspace Events tab asks for the month it shows', () => {
    const src = read('pages/PatchEvents.svelte');
    expect(src).toContain('monthRange(viewYear, viewMonth)');
    expect(src).toContain("allPages('events', { node_slug: slug, from: bounds.from, to: bounds.to })");
    expect(src).not.toContain('include_past=true');
  });

  it('the finders ask the server for matches in paged lists', () => {
    const src = read('lib/finderProviders.js');
    for (const q of ['proposals?search=', 'members?search=', 'admin/reports?search=', 'admin/submissions?search=', 'admin/users?search=']) {
      expect(src).toContain(q);
    }
    expect(read('components/PatchShell.svelte')).toContain('search={finderSearch}');
    expect(read('components/AdminShell.svelte')).toContain('search={finderSearch}');
  });

  it('patch verification asks for its own claims', () => {
    expect(read('pages/PatchSettingsVerification.svelte')).toContain("allPages('admin/claims', { status: 'pending', node_id: nodeID })");
  });

  it('Discover no longer asks for more than the server will give', () => {
    const src = read('pages/Discover.svelte');
    expect(src).not.toContain('limit=200');
    expect(src).toContain("allPages('events', { from: upcomingFrom(), to: dayEnd(until) })");
  });
});
