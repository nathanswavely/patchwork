/**
 * Where the e2e stacks live.
 *
 * The e2e suites run their own backend, their own Vite server, and their own
 * database — they share nothing with `make dev`. The seeded suite used to
 * share all three: ports 8090/5173 with `reuseExistingServer: true`, and
 * data/patchwork.db. That silently tested whatever code happened to own those
 * ports. With several git worktrees of this repo open at once (a normal day
 * here), a run could reuse a *different worktree's* servers and report green
 * or red for source you never edited. It also wiped your dev database on
 * every run.
 *
 * Two rules keep that from coming back:
 *
 *   1. Ports are derived from the checkout path, so every worktree gets its
 *      own set and concurrent runs don't meet. The same checkout always gets
 *      the same set, so a leftover server from a crashed run is a loud port
 *      conflict in the worktree that owns it, not a silent hijack elsewhere.
 *   2. Nothing is ever reused. The Playwright configs set
 *      `reuseExistingServer: false` and Vite runs with --strictPort, so an
 *      occupied port fails the run instead of quietly serving other code.
 *
 * Set the PATCHWORK_*_PORT variables below to pin exact ports (CI, or
 * debugging against a stack you started yourself).
 */
import { createHash } from 'crypto';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

/** Repo root — the module resolves it the same way from configs and specs. */
export const REPO_ROOT = path.resolve(__dirname, '..', '..');

// Bases sit clear of the dev stack (8090/5173) and of the launch.json
// embedded-SPA backend (8092). 50 slots is far more worktrees than anyone
// runs tests in at once; a collision would be a bind failure naming the
// port, not a wrong-code run.
const SLOTS = 50;
const offset = parseInt(
  createHash('sha256').update(REPO_ROOT).digest('hex').slice(0, 8), 16,
) % SLOTS;

const port = (envVar, base) => Number(process.env[envVar]) || base + offset;

// Seeded suite (playwright.config.js).
export const API_PORT = port('PATCHWORK_API_PORT', 8190);
export const WEB_PORT = port('PATCHWORK_WEB_PORT', 5273);
export const API_URL = `http://127.0.0.1:${API_PORT}`;
export const WEB_URL = `http://localhost:${WEB_PORT}`;

// Empty-instance suite (playwright.empty.config.js), which boots a second,
// unseeded stack and must not meet the seeded one either.
export const EMPTY_API_PORT = port('PATCHWORK_EMPTY_API_PORT', 8290);
export const EMPTY_WEB_PORT = port('PATCHWORK_EMPTY_WEB_PORT', 5373);
export const EMPTY_API_URL = `http://127.0.0.1:${EMPTY_API_PORT}`;
export const EMPTY_WEB_URL = `http://localhost:${EMPTY_WEB_PORT}`;

/**
 * The seeded suite's own database, kept out of data/patchwork.db so running
 * tests never wipes a dev instance. Relative to the repo root: both the
 * backend and the seeder run with the repo root as their working directory,
 * and the backend derives its governance repo directory from this path.
 */
export const DB_PATH = 'data/e2e/patchwork.db';
