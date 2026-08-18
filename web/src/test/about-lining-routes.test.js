/**
 * The About page and the lining page (CONTEXT.md "About page", "Lining",
 * docs/adr/040) are public surfaces reached from the global bar and the
 * intro card. There is no Svelte render library here, so — matching
 * scope-routing.test.js's approach — the routing wiring is asserted
 * against source text: the route must be registered, and it must render
 * inside SocialShell like the Label does, not fall through to the
 * "Page not found" branch or get swallowed by an auth gate.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('About and Lining routes', () => {
  const app = source('App.svelte');

  it('registers /about and /lining', () => {
    expect(app).toContain("addRoute('/about', 'about')");
    expect(app).toContain("addRoute('/lining', 'lining')");
  });

  it('renders both inside the social shell, not the standalone/admin/workspace shells', () => {
    expect(app).toMatch(/routeName === 'about'[\s\S]{0,20}<About \/>/);
    expect(app).toMatch(/routeName === 'lining'[\s\S]{0,20}<Lining \/>/);
    // Neither route requires auth — both are public, readable logged out,
    // like /label and the legal documents.
    const authRequiredMatch = app.match(/let authRequired = \$derived\(\s*\[([\s\S]*?)\]\.includes\(routeName\)\s*\)/);
    expect(authRequiredMatch).not.toBeNull();
    expect(authRequiredMatch[1]).not.toContain("'about'");
    expect(authRequiredMatch[1]).not.toContain("'lining'");
  });

  it('imports the About and Lining page components', () => {
    expect(app).toContain("import About from './pages/About.svelte'");
    expect(app).toContain("import Lining from './pages/Lining.svelte'");
  });
});

describe('The sidebar exposes "What is Patchwork?" to anonymous visitors', () => {
  const shell = source('components/SocialShell.svelte');
  const bar = source('components/GlobalBar.svelte');

  it('links to /about from the sidebar rail, not the bar', () => {
    expect(shell).toContain('href="/about"');
    expect(shell).toContain('What is Patchwork?');
    // The bar carries no /about link anymore (a comment may still name it).
    expect(bar).not.toContain('href="/about"');
  });
});

describe('The footer keeps a standing path to /about after sign-in', () => {
  const footer = source('components/LabelFooter.svelte');

  it('links to /about from both densities, ungated', () => {
    // The sidebar entry above is anonymous-only, so this is the only
    // standing path once you sign in — it must not sit inside the
    // `label?.published` branches the Label links live in.
    const aboutLinks = footer.match(/href="\/about"[\s\S]*?>About Patchwork</g) || [];
    expect(aboutLinks).toHaveLength(2); // overlay strip + page footer
    // Gated the way the legal links are — which is to say, not at all.
    const gatedBlocks = footer.match(/\{#if label\?\.published\}[\s\S]*?\{\/if\}/g) || [];
    for (const block of gatedBlocks) expect(block).not.toContain('/about');
  });
});

describe('SocialShell mounts the intro card', () => {
  const shell = source('components/SocialShell.svelte');

  it('imports and renders IntroCard', () => {
    expect(shell).toContain("import IntroCard from './IntroCard.svelte'");
    expect(shell).toMatch(/<IntroCard\s+\{routeName\}\s*\/>/);
  });
});
