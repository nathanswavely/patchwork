/**
 * The backend binary an e2e suite runs, and the command that seeds it.
 *
 * Both suites used to compile under a clock. The seeded suite's webServer
 * command was `go run ./cmd/patchwork`, so Playwright's 60s budget covered
 * fetching modules and compiling cgo/sqlite as well as booting; on a CI runner
 * that restored a near-empty Go cache the compile alone took 40-60s and the
 * run was a coin flip (run 39063472666, 2026-10-17: attempt 1 timed out at
 * 60s, the rerun of the same commit booted in 39s). The compile now happens
 * here, before any server is launched, and Playwright only times the boot.
 *
 * CI builds once and hands the paths in: PATCHWORK_E2E_BIN for the server,
 * PATCHWORK_E2E_SEED_BIN for the seeder, so the seeded and empty-instance
 * suites share one binary. Deliberately not the Makefile's PATCHWORK_BIN:
 * CONTRIBUTING.md suggests pointing every worktree at one path, and a suite
 * honouring that would run whichever worktree built last, the wrong-code
 * failure e2e/ports.js exists to prevent. A local run builds into its own
 * data directory instead.
 */
import { execSync } from 'child_process';
import { mkdirSync } from 'fs';
import path from 'path';
import { REPO_ROOT } from './ports.js';

/**
 * Path of the backend binary for a suite, building it when CI has not.
 * `dataDir` is the suite's own directory under the repo root; the binary
 * lives there so two suites, or two checkouts, never overwrite each other's.
 */
export function backendBinary(dataDir) {
  if (process.env.PATCHWORK_E2E_BIN) {
    return path.resolve(REPO_ROOT, process.env.PATCHWORK_E2E_BIN);
  }
  // Build once and run the binary directly: killing a `go run` wrapper on
  // Windows would orphan the actual server process. A stable path per
  // checkout also means Windows Firewall has one executable to remember,
  // not a new one per run (every listen is loopback regardless).
  const bin = path.join(REPO_ROOT, dataDir, process.platform === 'win32' ? 'patchwork-e2e.exe' : 'patchwork-e2e');
  // The backend creates its data directory when it opens the database, but
  // the binary lands there first.
  mkdirSync(path.dirname(bin), { recursive: true });
  console.log(`Building backend (${path.relative(REPO_ROOT, bin)})...`);
  execSync(`go build -o "${bin}" ./cmd/patchwork`, { cwd: REPO_ROOT, stdio: 'inherit', timeout: 180000 });
  return bin;
}

/** The seeder command, run from the repo root. */
export function seedCommand() {
  if (process.env.PATCHWORK_E2E_SEED_BIN) {
    return `"${path.resolve(REPO_ROOT, process.env.PATCHWORK_E2E_SEED_BIN)}"`;
  }
  // Locally, `go run` is fine here: it runs after the backend build, so the
  // module and build caches are warm and this is seconds, not a cold compile.
  return 'go run ./cmd/seed/';
}
