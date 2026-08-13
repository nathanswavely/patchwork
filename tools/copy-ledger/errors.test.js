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
