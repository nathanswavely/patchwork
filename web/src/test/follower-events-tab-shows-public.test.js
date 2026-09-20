/**
 * docs/adr/2026-09-19-an-event-says-who-it-is-for-within-what-the-patch-allows.md
 * (Amended 2026-09-19): follower_permissions.events stopped being a
 * tab-hiding switch and became an enforced read ceiling in Go. Hiding the
 * Events tab from a follower on a switch-off patch was the same costume
 * docs/adr/050 named for `proposals` — the same public events are already on
 * the quilt, the map, /events, the ICS feed and the link, so the tab
 * withheld nothing and just made the page overstate what it was hiding.
 *
 * There is no Svelte render library here, so what these assert is source
 * text: the follower-side refusal is gone, the tab id is unconditional, and
 * the rules editor states the switch's real effect.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { workspaceTabs } from '../lib/patchWorkspace.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('PatchEvents.svelte drops the follower refusal for events', () => {
  const src = source('pages/PatchEvents.svelte');

  it('no longer computes a permissionDenied off follower_permissions.events', () => {
    expect(src).not.toMatch(/permissionDenied/);
    expect(src).not.toContain('This content is only visible to members.');
    expect(src).not.toContain('Become a member to access events.');
  });

  it('renders whatever the server already handed over', () => {
    expect(src).toMatch(/events = data\.items \|\| data \|\| \[\]/);
  });
});

describe('workspaceTabs never hides events for a follower', () => {
  it('keeps the Events tab with the switch off', () => {
    const tabs = workspaceTabs({
      isUnclaimed: false,
      isAdmin: false,
      membershipRole: 'follower',
      followerPermissions: { events: false, proposals: true, charters: true, members: true },
    });
    expect(tabs.map((t) => t.id)).toContain('events');
  });

  it('keeps the Events tab with the switch on, same as before', () => {
    const tabs = workspaceTabs({
      isUnclaimed: false,
      isAdmin: false,
      membershipRole: 'follower',
      followerPermissions: { events: true, proposals: true, charters: true, members: true },
    });
    expect(tabs.map((t) => t.id)).toContain('events');
  });
});

describe('The rules editor says what the Events switch does', () => {
  const src = source('components/StructuredRulesEditor.svelte');

  it('states the effect beside the checkbox', () => {
    expect(src).toContain("Off: followers see this patch's public events only.");
  });

  it('leaves the other three switches without new helper text', () => {
    // This change is about `events` only (CLAUDE.md): proposals, charters
    // and members keep their bare checkboxes.
    const followerBlock = src.slice(src.indexOf('Follower Permissions'), src.indexOf('</div>', src.indexOf('Follower Permissions')));
    expect(followerBlock).toMatch(/Proposals<\/label>\s*<label>/);
    expect(followerBlock).toMatch(/Charters<\/label>\s*<label>/);
  });
});
