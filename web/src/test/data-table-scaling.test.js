import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const APP_CSS = source('app.css');
const TABLE_PAGES = ['pages/AdminUsers.svelte', 'pages/AdminAuditLog.svelte'];

// A table can't reflow, so on a phone the browser squeezes whatever looks
// squeezable — which is exactly the interactive columns: the role <select> on
// AdminUsers collapsed to a bare arrow. The fix is that the table is sized by
// its columns and the wrapper scrolls, and it only holds while the pages leave
// the sizing to app.css. There is no render library here (see CLAUDE.md), so
// this reads the source: a page that reintroduces its own width undoes it.
describe('the shared data table', () => {
  it('is sized by its columns, not by the viewport', () => {
    const rule = APP_CSS.match(/^\.data-table \{[^}]*\}/m)?.[0];
    expect(rule).toBeTruthy();
    expect(rule).toMatch(/min-width:\s*max-content/);
  });

  it('scrolls the wrapper rather than the page', () => {
    const rule = APP_CSS.match(/^\.table-wrapper \{[^}]*\}/m)?.[0];
    expect(rule).toBeTruthy();
    expect(rule).toMatch(/overflow-x:\s*auto/);
  });

  it('keeps controls at their own width against the mobile full-width rule', () => {
    expect(APP_CSS).toMatch(/\.data-table select,\s*\n\s*\.data-table input \{[^}]*width:\s*auto/);
    expect(APP_CSS).toMatch(/\.data-table select \{[^}]*min-width:/);
  });

  it('is not re-sized by the pages that use it', () => {
    for (const page of TABLE_PAGES) {
      const styles = (source(page).split('<style>')[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
      expect(styles, `${page} restates the table's width`).not.toMatch(
        /\.data-table[^{]*\{[^}]*\bwidth:/
      );
      expect(styles, `${page} restates the wrapper's overflow`).not.toMatch(
        /\.table-wrapper[^{]*\{/
      );
    }
  });
});
