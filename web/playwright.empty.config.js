import { defineConfig } from '@playwright/test';
import { EMPTY_API_PORT, EMPTY_WEB_PORT, EMPTY_WEB_URL } from './e2e/ports.js';

// Empty-instance e2e suite: boots the backend against a completely fresh,
// unseeded database and walks first-run onboarding as the instance's first
// user. This covers the empty states the seeded suite (playwright.config.js)
// structurally can't hit — there is deliberately NO seed step here.
//
// Run with: npm run test:e2e:empty
//
// Uses its own ports, derived from the checkout path (see e2e/ports.js), so
// it can run alongside a seeded dev stack, alongside the seeded e2e suite,
// and alongside the same suites in other worktrees. The backend is spawned by
// e2e-empty/global-setup.js (not webServer) because the first-user signup
// needs the magic-link URL that the SMTP-less backend prints to its log.
export default defineConfig({
  testDir: './e2e-empty',
  timeout: 30000,
  retries: 0,
  globalSetup: './e2e-empty/global-setup.js',
  use: {
    baseURL: EMPTY_WEB_URL,
    headless: true,
    screenshot: 'only-on-failure',
    ...(process.env.PW_CHROMIUM_PATH
      ? { launchOptions: { executablePath: process.env.PW_CHROMIUM_PATH } }
      : {}),
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
  webServer: [
    {
      command: `npx vite --port ${EMPTY_WEB_PORT} --strictPort`,
      url: EMPTY_WEB_URL,
      // Never reuse: a vite already on this port may proxy to a stale
      // backend. strictPort makes a leftover server a loud failure instead.
      reuseExistingServer: false,
      timeout: 10000,
      env: { PATCHWORK_API_PORT: String(EMPTY_API_PORT) },
    },
  ],
});
