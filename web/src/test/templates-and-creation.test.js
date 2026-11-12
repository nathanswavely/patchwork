/**
 * The templates and patch-creation decisions of 2026-11-11: F-018 (the
 * preview lists what the patch gets), F-032 (a card says who decides) and
 * F-024 (creation asks for the timezone).
 *
 * Source-text assertions, as everywhere here; the pages were checked in a
 * browser against a copy of the simulation instance.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('F-032: a template card says who decides', () => {
  const form = source('pages/PatchForm.svelte');
  const cards = form.slice(form.indexOf('const templates = ['), form.indexOf('];', form.indexOf('const templates = [')));

  it('names no group size on any card', () => {
    expect(cards).not.toMatch(/people\)/);
    expect(cards).not.toMatch(/\\u2013|\d+\+/);
  });

  it('says on every card that decisions can be made at a meeting', () => {
    expect(form).toContain("const elsewhereLine = 'Decisions can be made at a meeting instead, and recorded here.';");
    expect(form).toMatch(/\{#each templates as t\}[\s\S]*?<span class="template-meta">\{elsewhereLine\}<\/span>[\s\S]*?\{\/each\}/);
  });

  it('opens each description with who decides', () => {
    expect(cards).toContain("desc: 'You decide.");
    expect(cards).toContain("desc: 'Members vote for three days");
    expect(cards).toContain("desc: 'Members vote for a week");
    expect(cards).toContain("desc: 'Members elect a council");
  });
});

describe('F-032: the preview leads with who decides', () => {
  const drawer = source('components/TemplatePreviewDrawer.svelte');
  const fn = drawer.slice(drawer.indexOf('function explainRules'), drawer.indexOf('return explanations;'));

  it('puts leadership and method before any vote arithmetic', () => {
    const order = ["label: 'Leadership'", "label: 'Decision method'", "label: 'Quorum'", "label: 'Voting period'"]
      .map((l) => fn.indexOf(l));
    expect(order.every((i) => i >= 0)).toBe(true);
    expect([...order].sort((a, b) => a - b)).toEqual(order);
  });

  it('shows quorum and the voting window only where members vote', () => {
    expect(fn).toContain("const votes = rules.decision_method && rules.decision_method !== 'admin';");
    expect(fn).toContain('if (votes && rules.quorum_percent !== undefined)');
    expect(fn).toContain('if (votes && rules.default_vote_duration_hours)');
  });

  it('says the meeting line the server sends', () => {
    expect(drawer).toContain('{data.template.elsewhere}');
  });
});

describe('F-018: the preview lists the documents the patch starts with', () => {
  const drawer = source('components/TemplatePreviewDrawer.svelte');

  it('names each document by the title its row will carry', () => {
    expect(drawer).toContain('{doc.title || docLabel(doc.filename)}');
  });

  it('says the documents beyond the lining start members-only', () => {
    expect(drawer).toContain('Documents this patch starts with ({data.documents.length})');
    expect(drawer).toContain('The others start visible to members only, until an admin publishes them.');
  });
});

describe('F-024: creation asks for the timezone', () => {
  const form = source('pages/PatchForm.svelte');

  it('preselects the device zone, and only a place', () => {
    expect(form).toContain('Intl.DateTimeFormat().resolvedOptions().timeZone');
    expect(form).toMatch(/return isPlaceZone\(z\) \? z : '';/);
    expect(form).toContain("if (!zoneChoice) zoneChoice = deviceZone || quiltZone || 'other';");
  });

  it("offers the quilt's zone beside it, and any other place", () => {
    expect(form).toContain('let quiltZone = $derived(getInstanceTimezone());');
    expect(form).toContain('(this quilt)');
    expect(form).toContain("opts.push({ value: 'other', label: 'Another timezone' });");
  });

  it('sends the choice with the creation, checked as Patch Settings checks it', () => {
    expect(form).toContain('timezone: chosenZone || undefined,');
    expect(form).toMatch(/chosenZone && !isPlaceZone\(chosenZone\)/);
  });

  it('says a refused zone under the field, not only at the foot of the form', () => {
    expect(form).toMatch(/\{#if otherZoneBad\}\s*<p class="lt-error-text">That is not a place this quilt keeps time in\. Try America\/New_York\.<\/p>/);
  });

  it('asks at creation only, never in setup', () => {
    expect(form).toMatch(/\{#if mode !== 'setup'\}\s*<div class="field">\s*<label class="lt-label" for="patch-timezone">Timezone<\/label>/);
  });
});
