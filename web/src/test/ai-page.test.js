/**
 * The AI page (docs/adr/2026-10-20-the-site-says-where-ai-is.md). There is no
 * render library here, so these hold the wiring: the page is reachable at
 * /ai from every footer, it reads the one public endpoint, and the only
 * part an admin can edit is the stewards' part.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('the AI page', () => {
  const page = source('pages/AiPage.svelte');

  it('is routed at /ai', () => {
    const app = source('App.svelte');
    expect(app).toContain("addRoute('/ai', 'ai');");
    expect(app).toMatch(/routeName === 'ai'\}\s*<AiPage \/>/);
  });

  it('is linked from both footer variants, beside Privacy', () => {
    const footer = source('components/LabelFooter.svelte');
    // The onclick arrow carries a ">", so match lazily to the link text.
    const links = footer.match(/<a href="\/ai".*?>AI<\/a>/g) || [];
    expect(links).toHaveLength(2);
  });

  it('reads the public endpoint and renders both parts', () => {
    expect(page).toContain("api('ai')");
    expect(page).toContain('data.project_markdown');
    expect(page).toContain('data.stewards_markdown');
  });

  it('names a row for every visitor page the server reports', () => {
    for (const id of ['lining', 'charters', 'about', 'label', 'legal', 'ai']) {
      expect(page).toMatch(new RegExp(`\\b${id}: '`));
    }
  });

  it('states the not-copy count rather than hiding it', () => {
    expect(page).toContain('data.copy.not_copy');
  });

  it('says so when the stewards have written nothing', () => {
    expect(page).toMatch(/\{:else\}\s*<p class="lt-muted">The stewards of/);
  });
});

describe('admin editing of the AI page', () => {
  const admin = source('pages/AdminLegal.svelte');

  it('edits only the stewards part, through its own endpoint', () => {
    expect(admin).toContain("api('admin/ai-page', { method: 'PUT'");
    expect(admin).toContain("api('admin/ai-page', { method: 'DELETE' })");
    expect(admin).not.toContain('project_markdown');
  });
});
