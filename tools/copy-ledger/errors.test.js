// API error messages are copy, and they have to survive the round trip.
//
// A visitor reads these: the SPA renders the `error` field into toasts and
// inline form errors, so a refusal is read at the moment it matters most to
// somebody — when the thing they tried did not work. They went uncounted
// because the Go scope was a list of prose files and no handler was on it.
//
// The property that matters here is not extraction, it is writeback. Only the
// message inside `{"error":"..."}` is claimed; if a replacement ever escaped
// that substring it would rewrite the JSON around it and every API response
// from that path would stop parsing. So each case checks the wrapper is still
// intact afterwards, not merely that the words changed.
//
//   node --test tools/copy-ledger/     (or: make copy-test)

import assert from 'node:assert/strict';
import test from 'node:test';
import { extractOne } from './extract.js';
import { encodeFor } from './writeback.js';

const SRC = [
  'package handler',
  '',
  'func H(w http.ResponseWriter) {',
  '\thttp.Error(w, `{"error":"only an admin of this patch can record a decision"}`, 403)',
  '\thttp.Error(w, `{"error":"node not found"}`, 404)',
  '\t// http.Error(w, `{"error":"a commented-out refusal"}`, 400)',
  '\tlog.Printf("election: %s seated %d", slug, n)',
  '\tdb.Exec(`SELECT id FROM nodes WHERE slug = ?`, slug)',
  '}',
].join('\n');

function errorHits(src) {
  return extractOne('internal/handler/x.go', 'go-errors', src)
    .filter((h) => h.kind === 'go-error-body');
}

test('an API error message is claimed, and nothing around it is', () => {
  const hits = errorHits(SRC);
  const texts = hits.map((h) => h.text);

  assert.ok(texts.includes('only an admin of this patch can record a decision'));
  assert.ok(texts.includes('node not found'));

  // A log line and a SQL statement are not copy, and a commented-out refusal
  // is not shipped.
  assert.ok(!texts.some((t) => t.includes('seated')), 'log format string claimed');
  assert.ok(!texts.some((t) => t.includes('SELECT')), 'SQL claimed');
  assert.ok(!texts.some((t) => t.includes('commented-out')), 'commented-out code claimed');

  // The message alone, never the JSON around it — `raw` is what writeback
  // replaces, so a wrapper here would be a wrapper overwritten.
  for (const h of hits) {
    assert.ok(!h.raw.includes('{"error"'), `raw carries the wrapper: ${h.raw}`);
    assert.ok(!h.raw.includes('`'), `raw carries a backtick: ${h.raw}`);
  }
});

test('a replacement lands inside the wrapper and leaves it parseable', () => {
  const hit = errorHits(SRC).find((h) => h.text === 'node not found');
  const enc = encodeFor(hit.kind, hit.quote, 'we could not find that patch');
  assert.equal(enc.problem, undefined);

  const after = SRC.replace(hit.raw, enc.value);
  assert.ok(after.includes('`{"error":"we could not find that patch"}`'));

  // The body still parses as the JSON the handler promises. Matched on the
  // replaced message rather than the first body in the file — there are
  // several, and grabbing whichever came first is how this assertion passed
  // for the wrong reason on the first attempt.
  const body = after
    .match(/`(\{"error":"we could not find that patch"\})`/)[1];
  assert.equal(JSON.parse(body).error, 'we could not find that patch');
});

test('a quote in a replacement is escaped rather than breaking the body', () => {
  const enc = encodeFor('go-error-body', null, 'that "name" is already taken');
  assert.equal(enc.problem, undefined);

  const body = `{"error":"${enc.value}"}`;
  assert.equal(JSON.parse(body).error, 'that "name" is already taken');
});

test('a backtick or backslash is refused rather than written', () => {
  // A backtick would close the Go raw string. A backslash would land in the
  // JSON as an escape nobody asked for.
  assert.ok(encodeFor('go-error-body', null, 'use `make dev`').problem);
  assert.ok(encodeFor('go-error-body', null, 'a path like C:\\Users').problem);
});

// ------------------------------------------------------- conditional text ---

// Text inside a conditional is still text.
//
// The text-node scan stops at the next `<`, so an element wrapping an
// `{#if}` hands the extractor one chunk with the directives inside it — and
// the noise filter rejected any chunk containing `{#`, which is right about
// the directive and wrong about the sentences either side of it. A term line
// reading "This council's term ended … / Next seat comes up …" lost both
// branches at once and neither was ever offered for review.

const IFELSE = [
  '<p class="term-line">',
  '  {#if termLapsed}',
  "    This council's term ended {formatDay(end)}. It serves until a successor is elected.",
  '  {:else}',
  '    Next seat comes up {formatDay(end)}.',
  '  {/if}',
  '</p>',
].join('\n');

function markupHits(src) {
  return extractOne('web/src/components/X.svelte', 'svelte', src)
    .filter((h) => h.kind === 'markup');
}

test('both branches of a conditional are offered, and the directives are not', () => {
  const texts = markupHits(IFELSE).map((h) => h.text);
  assert.equal(texts.length, 2);
  assert.ok(texts.some((t) => t.startsWith("This council's term ended")));
  assert.ok(texts.some((t) => t.startsWith('Next seat comes up')));
  assert.ok(!texts.some((t) => /\{[#:/]/.test(t)), 'a directive was claimed as copy');
});

test('each branch is an exact, unique substring so writeback can find it', () => {
  for (const h of markupHits(IFELSE)) {
    assert.equal(IFELSE.split(h.raw).length - 1, 1, `not unique in source: ${h.raw}`);
  }
});

test('an expression inside the copy survives, nested ones included', () => {
  // One expression, not two: a single strip pass removes `${name}` and leaves
  // the outer braces, which then reads as unbalanced code. This exact string
  // is human-written in the repo and was being thrown away.
  const src = "<p>\n  Reports go to the admins.{name ? ` You're reporting ${name}.` : ''}\n</p>";
  const texts = markupHits(src).map((h) => h.text);
  assert.equal(texts.length, 1);
  assert.ok(texts[0].startsWith('Reports go to the admins.'));
});

test('attribute residue is refused rather than filed as copy', () => {
  // An attribute holding a `>` leaves the scan mid-tag: `onclick={() => {`
  // ends the "tag" early, so the chunk runs `} title="x">Close` and the label
  // is welded to the code in front of it.
  //
  // Neither version recovers "Close" — that needs a real parser, and this
  // asserts the reachable property instead: the fragment is refused. Before,
  // it was accepted, and `} title="x">Close` sat in the queue as a string
  // somebody was expected to rewrite.
  const src = '<button onclick={() => { open = false; }} title="x">Close</button>';
  for (const h of markupHits(src)) {
    assert.ok(!/[{}]|=>|="/.test(h.text), `code filed as copy: ${h.text}`);
  }
});

// --------------------------------------------------------------- SQL, not ---

// A query is not copy, and a sentence that opens like one still is.
//
// The data test only knew about JSON — a leading `[` or `{` — so queries in
// the notification files walked past it and sat in the review queue as prose.
// One was a 79-word SELECT somebody was being asked to rewrite in the
// project's voice.
//
// The correction has to be careful in the other direction: "Select a patch"
// and "Update your event details." are real copy here, so an opening verb
// alone cannot be the signal.

const withSQL = (q) => `package notifications\n\nfunc F() {\n\tdb.Query(\`${q}\`)\n}`;

test('a query is not offered as copy', () => {
  const queries = [
    'SELECT id, name FROM nodes WHERE slug = ?',
    "INSERT OR IGNORE INTO reminders_sent (id, kind) VALUES (?, 'x')",
    'UPDATE memberships SET role = ? WHERE user_id = ?',
    'DELETE FROM sessions WHERE expires_at < ?',
  ];
  for (const q of queries) {
    const hits = extractOne('internal/notifications/x.go', 'go', withSQL(q));
    assert.equal(hits.length, 0, `claimed as copy: ${q}`);
  }
});

test('a sentence that opens like a query is still copy', () => {
  // The verb alone is not the signal; the clause keyword after it is.
  const src = [
    'package governance',
    '',
    'var Copy = []string{',
    '\t"Select a patch to continue.",',
    '\t"Update your event details.",',
    '}',
  ].join('\n');
  const texts = extractOne('internal/governance/defaults.go', 'go', src).map((h) => h.text);
  assert.ok(texts.includes('Select a patch to continue.'));
  assert.ok(texts.includes('Update your event details.'));
});
