// scripts/audit-signatures.sh decides whether a red `npm audit signatures`
// is a supply-chain event or a registry outage, and it is allowed to exit 0
// on one of them. That is exactly the kind of logic that must not be
// trusted to a careful read: the failure mode is a real mismatch quietly
// passing, and nobody would notice until it mattered.
//
// So each branch is exercised against a stub `npm` that prints what the real
// one prints. The stub goes first on PATH; the script does not know.

const { test } = require('node:test');
const assert = require('node:assert');
const { spawnSync } = require('node:child_process');
const { mkdtempSync, writeFileSync, chmodSync, mkdirSync, existsSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join, resolve, delimiter } = require('node:path');

const SCRIPT = resolve(__dirname, 'audit-signatures.sh');

// Which bash. The script runs on ubuntu-latest in CI, where this is simply
// `bash`. On Windows a bare `bash` resolves to the WSL shim in system32,
// which cannot see a stub written to the Windows temp directory and answers
// 127 to every case — a green suite would be reporting nothing. Git Bash is
// the one that behaves, so prefer it, and say so plainly if neither is here
// rather than passing by default.
const BASH = (() => {
  if (process.platform !== 'win32') return 'bash';
  // Forward slashes: Node accepts them on Windows and they keep this
  // readable without four levels of backslash escaping.
  for (const p of [
    'C:/Program Files/Git/bin/bash.exe',
    'C:/Program Files/Git/usr/bin/bash.exe',
  ]) {
    if (existsSync(p)) return p;
  }
  return null;
})();

// runWith builds a throwaway PATH whose `npm` is a shell script emitting the
// given stdout/stderr and exit code, then runs the real script against it.
function runWith({ stdout = '', stderr = '', code = 0, attempts = 2 }) {
  const dir = mkdtempSync(join(tmpdir(), 'auditsig-'));
  const bin = join(dir, 'bin');
  mkdirSync(bin);

  const stub = [
    '#!/usr/bin/env bash',
    `cat <<'STUB_OUT'\n${stdout}\nSTUB_OUT`,
    `cat >&2 <<'STUB_ERR'\n${stderr}\nSTUB_ERR`,
    `exit ${code}`,
  ].join('\n');
  writeFileSync(join(bin, 'npm'), stub);
  chmodSync(join(bin, 'npm'), 0o755);

  // spawnSync rather than execFileSync: the latter hands back stdout only,
  // and the interesting half of this script's output — which packages could
  // not be verified — goes to stderr on a run that exits 0.
  const r = spawnSync(BASH, [SCRIPT], {
    env: {
      ...process.env,
      // path.delimiter, not a literal ':': on Windows the separator is ';'
      // and a hardcoded colon corrupts PATH outright, so every case fails
      // with 127 rather than exercising the script.
      PATH: `${bin}${delimiter}${process.env.PATH}`,
      GITHUB_ACTIONS: '',
      AUDIT_SIGNATURES_ATTEMPTS: String(attempts),
      AUDIT_SIGNATURES_SLEEP: '0',
    },
    encoding: 'utf8',
  });
  return { code: r.status, out: r.stdout || '', err: r.stderr || '' };
}

const opts = BASH ? {} : { skip: 'no POSIX bash here; this script runs on the Linux CI runner' };

test('a clean audit passes', opts, () => {
  const r = runWith({ stdout: '{"invalid":[],"missing":[]}', code: 0 });
  assert.equal(r.code, 0);
  assert.match(r.out, /every package verified/);
});

test('a signature mismatch fails, and does not wait out the retries', opts, () => {
  // The one outcome the gate exists for. `invalid` is non-empty, so this is
  // npm saying the bytes are not the bytes it signed.
  const r = runWith({
    stdout: '{"invalid":[{"name":"left-pad","version":"1.0.0"}],"missing":[]}',
    stderr: 'npm error code EINTEGRITYSIGNATURE',
    code: 1,
    attempts: 3,
  });
  assert.equal(r.code, 1);
  assert.match(r.err, /does not match the signature/);
  assert.match(r.err, /left-pad/);
});

test('a mismatch fails even when a missing key is reported alongside it', opts, () => {
  // Ordering matters: the missing-key branch exits 0, so if it were checked
  // first a tampered package would ride out on a registry outage.
  const r = runWith({
    stdout: '{"invalid":[{"name":"left-pad","version":"1.0.0"}],"missing":[]}',
    stderr: 'npm error code EMISSINGSIGNATUREKEY\nnpm error @playwright/test@1.63.0 has attestations but no corresponding public key(s) can be found',
    code: 1,
  });
  assert.equal(r.code, 1, 'a tampered package must not pass because a key was also missing');
  assert.match(r.err, /does not match the signature/);
});

test('a missing key passes, names the package, and says no signature failed', opts, () => {
  const r = runWith({
    stdout: '{"invalid":[],"missing":[]}',
    stderr: 'npm error code EMISSINGSIGNATUREKEY\nnpm error @playwright/test@1.63.0 has attestations but no corresponding public key(s) can be found',
    code: 1,
  });
  assert.equal(r.code, 0);
  assert.match(r.err, /@playwright\/test@1\.63\.0/);
  assert.match(r.err, /no signature mismatched/);
});

test('an unrecognised failure fails rather than being swallowed', opts, () => {
  const r = runWith({
    stdout: '',
    stderr: 'npm error code ENETUNREACH\nnpm error request to https://registry.npmjs.org failed',
    code: 1,
  });
  assert.equal(r.code, 1);
  assert.match(r.err, /does not recognise/);
});

test('an empty invalid array is not read as a mismatch', opts, () => {
  // The guard greps the JSON, so `"invalid": []` and `"invalid": [ ]` both
  // have to read as "nothing invalid" or every missing-key run would fail.
  const r = runWith({
    stdout: '{\n  "invalid": [ ],\n  "missing": []\n}',
    stderr: 'npm error code EMISSINGSIGNATUREKEY\nnpm error @playwright/test@1.63.0 has attestations but no corresponding public key(s) can be found',
    code: 1,
  });
  assert.equal(r.code, 0);
});

// CI runs the script by path, not as an argument to bash, so it needs the
// execute bit recorded in git. Windows does not track file modes, so a
// script added from there arrives 100644 and the job dies with a bare
// "Permission denied" and exit 126 — which is what happened to the first
// push of this very change. The cases above all invoke it through `bash`,
// so none of them could ever have caught it. This reads the index, which
// is the thing CI actually checks out, and works from any platform.
test('the script is executable in git, the way CI invokes it', () => {
  const r = spawnSync('git', ['ls-files', '-s', '--', 'scripts/audit-signatures.sh'], {
    cwd: resolve(__dirname, '..'),
    encoding: 'utf8',
  });
  assert.equal(r.status, 0, 'git ls-files failed');
  assert.match(
    r.stdout,
    /^100755 /,
    `expected mode 100755, got: ${r.stdout.trim()}. Fix with: git update-index --chmod=+x scripts/audit-signatures.sh`,
  );
});
