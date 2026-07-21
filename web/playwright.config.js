import { defineConfig } from '@playwright/test';
import { execSync } from 'child_process';
import { existsSync } from 'fs';
import path from 'path';
import { API_PORT, WEB_PORT, API_URL, WEB_URL, DB_PATH, REPO_ROOT } from './e2e/ports.js';

// Everything in this file runs before the servers start — Playwright launches
// `webServer` first and only then calls globalSetup, so anything the backend
// needs in order to compile or boot has to be arranged here.

// The Go binary embeds web/dist (embed.FS), so `go run ./cmd/patchwork` does
// not compile without it. A fresh checkout has no dist/; build it once. The
// suite itself is served by Vite, not by the embedded copy.
if (!existsSync(path.join(REPO_ROOT, 'web', 'dist', 'index.html'))) {
  console.log('web/dist missing — building SPA once for embed.FS...');
  execSync('npx vite build', { cwd: path.join(REPO_ROOT, 'web'), stdio: 'inherit', timeout: 180000 });
}

// patchwork.yaml is gitignored, so a fresh worktree doesn't have one. The
// example differs only in values the suite overrides or ignores, so falling
// back to it means `npx playwright test` works in a checkout with no setup.
const CONFIG_FILE = existsSync(path.join(REPO_ROOT, 'patchwork.yaml'))
  ? 'patchwork.yaml'
  : 'patchwork.yaml.example';

// Worth stating out loud: the ports move per checkout, so a failure to bind
// or connect is only diagnosable if the run says where it expected things.
// Workers re-evaluate this file, so log from the runner process only.
if (process.env.TEST_WORKER_INDEX === undefined) {
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
      // `go run` (no platform-specific binary path) starts the backend on both
      // Windows (cmd.exe) and Linux/macOS; `./patchwork` was a POSIX-only invocation.
      command: `go run ./cmd/patchwork/ -config ${CONFIG_FILE}`,
      cwd: REPO_ROOT,
      env: { PATCHWORK_PORT: String(API_PORT), PATCHWORK_DB_PATH: DB_PATH },
      url: `${API_URL}/api/v1/health`,
      reuseExistingServer: false,
      // 60s: `go run` compiles before running and this project uses cgo (go-sqlite3),
      // so a cold first start can be slow.
      timeout: 60000,
    },
    {
      command: `npx vite --port ${WEB_PORT} --strictPort`,
      env: { PATCHWORK_API_PORT: String(API_PORT) },
      url: WEB_URL,
      reuseExistingServer: false,
      timeout: 10000,
    },
  ],
});
