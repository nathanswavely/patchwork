import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      // Default backend port; the empty-instance e2e suite
      // (playwright.empty.config.js) points this at its own backend.
      '/api': `http://127.0.0.1:${process.env.PATCHWORK_API_PORT || 8090}`,
    },
  },
  test: {
    environment: 'happy-dom',
    // The suite runs in one fixed zone, everywhere.
    //
    // An event renders in its own zone and is annotated only when that zone
    // is not the reader's (docs/adr/045, web/src/lib/datetime.js), so a test
    // of that annotation asserts on output that depends on where the machine
    // running it happens to be. event-zone.test.js encoded the reader as UTC
    // and five of its fourteen cases failed on any other zone — green on the
    // GitHub runners, which are UTC, and red on a clean checkout in
    // Lancaster, where they read as a regression rather than as a setup
    // difference. Pinning here rather than in ci.yaml is the point: CI and a
    // contributor's laptop have to agree, and only the config both of them
    // load can make that so.
    env: { TZ: 'UTC' },
    globals: true,
    setupFiles: ['./src/test/setup.js'],
    // Playwright specs live in e2e/ and cannot run under vitest.
    include: ['src/**/*.{test,spec}.js'],
  },
});
