import { defineConfig } from '@playwright/test';
import { execSync } from 'child_process';
import { existsSync } from 'fs';
import path from 'path';
import { API_PORT, WEB_PORT, API_URL, WEB_URL, DB_PATH, REPO_ROOT } from './e2e/ports.js';
import { backendBinary } from './e2e/backend.js';

// Everything in this file runs before the servers start — Playwright launches
// `webServer` first and only then calls globalSetup, so anything the backend
// needs in order to compile or boot has to be arranged here.
//
// Workers re-evaluate this file, so the one-time work below runs in the
// runner process only (TEST_WORKER_INDEX is set in a worker and not there).
const runner = process.env.TEST_WORKER_INDEX === undefined;

// The Go binary embeds web/dist (embed.FS), so the backend does not compile
// without it. A fresh checkout has no dist/; build it once. The suite itself
// is served by Vite, not by the embedded copy.
if (runner && !existsSync(path.join(REPO_ROOT, 'web', 'dist', 'index.html'))) {
  console.log('web/dist missing — building SPA once for embed.FS...');
  execSync('npx vite build', { cwd: path.join(REPO_ROOT, 'web'), stdio: 'inherit', timeout: 180000 });
}

// Compiled here, before Playwright's webServer timer starts, never under it:
// see e2e/backend.js for the CI run that made this a rule. Workers never
// launch the server, so they never need the path.
const BACKEND = runner ? backendBinary(path.dirname(DB_PATH)) : null;

// patchwork.yaml is gitignored, so a fresh worktree doesn't have one. The
// example differs only in values the suite overrides or ignores, so falling
// back to it means `npx playwright test` works in a checkout with no setup.
const CONFIG_FILE = existsSync(path.join(REPO_ROOT, 'patchwork.yaml'))
  ? 'patchwork.yaml'
  : 'patchwork.yaml.example';

// Worth stating out loud: the ports move per checkout, so a failure to bind
// or connect is only diagnosable if the run says where it expected things.
if (runner) {
  console.log(`e2e stack: backend :${API_PORT}, vite :${WEB_PORT}, db ${DB_PATH}, config ${CONFIG_FILE}`);
}

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  retries: 0,
  // Spec files run in parallel workers against one shared backend + database.
  // This is safe because each mutating spec owns the entities it mutates —
  // see the data-ownership map in e2e/setup.js before adding mutations.
  globalSetup: './e2e/global-setup.js',
  use: {
    baseURL: WEB_URL,
    headless: true,
    screenshot: 'only-on-failure',
    // Allow CI/sandbox environments with a system Chromium to override the
    // browser binary instead of downloading a pinned build.
    ...(process.env.PW_CHROMIUM_PATH
      ? { launchOptions: { executablePath: process.env.PW_CHROMIUM_PATH } }
      : {}),
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
  // Both servers belong to this run and are torn down with it. Neither is
  // ever reused: a server already on these ports is another run's (or a
  // crashed run's leftovers), and adopting it would test its code, not this
  // checkout's — the failure mode this suite exists to catch. See e2e/ports.js.
  webServer: [
    {
      // The binary built above, by its absolute path, which both cmd.exe and
      // a POSIX shell run (`./patchwork` was POSIX-only; `go run` was
      // portable but put the compile under the timer below).
      command: `"${BACKEND}" -config ${CONFIG_FILE}`,
      cwd: REPO_ROOT,
      // Loopback only: nothing off this machine should reach a test stack, and
      // Windows Firewall prompts for every all-interface listen.
      env: { PATCHWORK_HOST: '127.0.0.1', PATCHWORK_PORT: String(API_PORT), PATCHWORK_DB_PATH: DB_PATH },
      url: `${API_URL}/api/v1/health`,
      reuseExistingServer: false,
      // 60s covers only the boot now (integrity check, migrations, listen);
      // the compile is already done. Kept generous rather than trimmed: a
      // shared CI runner can stall on disk for a while, and nothing here is
      // measuring speed.
      timeout: 60000,
    },
    {
      command: `npx vite --port ${WEB_PORT} --strictPort`,
      env: { PATCHWORK_API_PORT: String(API_PORT) },
      url: WEB_URL,
      reuseExistingServer: false,
      // 60s, not 10s: this budget is time-to-first-response, not time-to-listen,
      // and those are far apart on a cold dependency cache. Vite reports "ready"
      // in ~2s either way, then blocks the first request while it pre-bundles.
      // Measured cold on this project: vite 6 answered in 4.7s, vite 8 (rolldown)
      // in 14.7s. CI is always cold — fresh checkout, `npm ci`, no
      // node_modules/.vite — so 10s failed there every time while a warm local
      // run passed.
      timeout: 60000,
    },
  ],
});
