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
    globals: true,
    setupFiles: ['./src/test/setup.js'],
    // Playwright specs live in e2e/ and cannot run under vitest.
    include: ['src/**/*.{test,spec}.js'],
  },
});
