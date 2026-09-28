/**
 * The shared mobile fixes from the 2026-09-28 phone audit. Nothing here
 * renders, so these assert the rules that were missing, in the files that
 * were missing them; the reason for each is in the comment beside it.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('the shell owns the gutter', () => {
  it('gives .container-narrow no padding of its own', () => {
    const css = source('app.css');
    const rule = css.match(/\.container-narrow \{[^}]*\}/)?.[0] || '';
    expect(rule).toContain('max-width: 640px');
    expect(rule).not.toMatch(/padding:\s*0 1rem/);
  });
});

describe('a row wraps rather than squeezing its name', () => {
  const cases = [
    ['pages/PatchSettingsMembers.svelte', '.member-info {\n      flex: 1 1 100%;'],
    ['pages/UserSettingsPatches.svelte', '.patch-info {\n      flex: 1 1 100%;'],
    ['pages/EventDetail.svelte', '.detail-header h1 {\n    flex: 1 1 14rem;'],
    ['pages/SecuritySettings.svelte', '.cred-info {\n    flex: 1 1 12rem;'],
    ['pages/PatchSettingsNoticeboard.svelte', '.setting-info { flex: 1 1 14rem;'],
    ['pages/PatchNoticeboard.svelte', '.board-hint { flex: 1 1 16rem;'],
  ];
  for (const [file, rule] of cases) {
    it(file, () => {
      expect(source(file).replace(/\r\n/g, '\n')).toContain(rule);
    });
  }

  it('gives the event-source URL field the full width on a phone', () => {
    const src = source('pages/PatchSettingsSources.svelte').replace(/\r\n/g, '\n');
    expect(src).toMatch(/\.add-form input \{\n\s*flex: 1 1 100%;/);
  });
});

describe('ConfirmAction fits the row it opens in', () => {
  const src = source('components/ConfirmAction.svelte');

  it('lets its confirm state wrap', () => {
    expect(src).toMatch(/\.confirm-group \{\s*flex-wrap: wrap;/);
  });

  it('keeps the prompt for a screen reader while hiding it on a phone', () => {
    expect(src).toContain('<span class="confirm-prompt">Are you sure?</span>');
    expect(src).toMatch(/@media \(max-width: 640px\) \{\s*\.confirm-prompt \{\s*position: absolute;/);
  });
});

describe('a finger gets a finger-sized target', () => {
  const coarse = (file) => {
    const src = source(file);
    const i = src.indexOf('@media (pointer: coarse)');
    return i >= 0 ? src.slice(i) : '';
  };

  it('in the shared controls', () => {
    expect(coarse('components/TagPicker.svelte')).toContain('min-height: 36px');
    expect(coarse('components/SegmentedControl.svelte')).toContain('min-height: 40px');
    expect(coarse('components/Modal.svelte')).toContain('width: 44px');
    expect(coarse('components/ToggleSwitch.svelte')).toContain('padding: 11px 4px');
    expect(coarse('components/GlobalBar.svelte')).toContain('min-height: 44px');
    expect(coarse('components/PatchOverflow.svelte')).toContain('min-height: 44px');
    expect(coarse('components/PatchRelationship.svelte')).toContain('min-height: 44px');
  });

  it('for a text-shaped button, without moving the line it sits in', () => {
    const css = source('app.css');
    expect(css).toMatch(/\.btn-link \{\s*padding-block: 0\.6rem;\s*margin-block: -0\.6rem;/);
  });
});

describe('admin tables have a phone form', () => {
  it('Users writes each control once and renders it in the table and the phone list', () => {
    const src = source('pages/AdminUsers.svelte');
    for (const name of ['emailControl', 'roleControl', 'trustControl', 'statusBadge', 'accountControl']) {
      expect(src).toContain(`{#snippet ${name}(u)}`);
      // Once in a table cell, once in the phone detail.
      expect(src.split(`{@render ${name}(u)}`).length - 1).toBe(2);
    }
    expect(src).toContain('{:else if narrow}');
    expect(src).toContain('<ul class="user-list">');
  });

  it('Users puts the search and list before the invite form on a phone', () => {
    const src = source('pages/AdminUsers.svelte');
    const list = src.indexOf('<ul class="user-list">');
    const lateInvite = src.lastIndexOf('<section class="invite-section card">');
    expect(src.indexOf('{#if !narrow}')).toBeLessThan(src.indexOf('<div class="search-bar">'));
    expect(lateInvite).toBeGreaterThan(list);
  });

  it('the audit log reads as a list on a phone and names a target by the tail of its id', () => {
    const src = source('pages/AdminAuditLog.svelte');
    expect(src).toContain('<ul class="entry-list">');
    expect(src).toContain('entry.entity_id.slice(-8)');
    expect(src).not.toContain('entity_id?.substring(0, 8)');
  });
});
