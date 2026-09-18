/**
 * The link handshake reads trust at both of its scopes
 * (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar,
 * decision 2, over docs/adr/057).
 *
 * EventLinks decided standing on an unclaimed patch from the signed-in
 * user's quilt-wide flag alone, so a person trusted on exactly the event's
 * own patch — the grant a suggestion approval hands out — was shown no link
 * control at all, while the API would have let them act. The per-patch
 * grant is not on the user object and not in memberships (an unclaimed
 * patch admits nobody), so the component now reads it from two payloads
 * that state it: the event's own `viewer_trusted` for its patch, and the
 * session's `trusted_patches` for the reach beyond it.
 *
 * Source text only — there is no Svelte render library in this project.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('the auth store exposes the per-patch scope of the grant', () => {
  const src = source('stores/auth.svelte.js');

  it('reads trusted_patches off the me payload, defaulting to none', () => {
    expect(src).toMatch(/export function getTrustedPatches\(\) \{\n\s*return user\?\.trusted_patches \|\| \[\];/);
  });

  it('keeps the quilt-wide flag as its own question', () => {
    expect(src).toMatch(/export function isTrustedContributor\(\) \{\n\s*return user\?\.trusted_contributor === true;/);
  });
});

describe('EventLinks.svelte decides standing at either scope', () => {
  const src = source('components/EventLinks.svelte');

  it('no longer answers an unclaimed patch from the quilt-wide flag alone', () => {
    expect(src).not.toMatch(/return isTrustedContributor\(\) && status === 'unclaimed';/);
  });

  it('speaksFor takes the payload’s own answer and consults the session’s list', () => {
    expect(src).toMatch(/function speaksFor\(slug, status, viewerTrusted = false\)/);
    expect(src).toMatch(/return viewerTrusted \|\| isTrustedContributor\(\) \|\| trustedSlugs\.has\(slug\);/);
    expect(src).toMatch(/trustedSlugs = \$derived\(new Set\(getTrustedPatches\(\)\.map\(\(p\) => p\.slug\)\)\)/);
    expect(src).toMatch(/import \{ isAdmin, isTrustedContributor, getTrustedPatches \} from '\.\.\/stores\/auth\.svelte\.js'/);
  });

  // ADR 057: worth nothing on a claimed patch, on either side. The status
  // check has to come before any scope of the grant is consulted, so a
  // stale trusted_patches entry or a true viewer_trusted can never widen a
  // control on an active patch.
  it('refuses every scope of the grant before it looks at one, on a claimed patch', () => {
    const fn = src.match(/function speaksFor\(slug, status, viewerTrusted = false\) \{([\s\S]*?)\n  \}/)[1];
    const statusGate = fn.indexOf("if (status !== 'unclaimed') return false;");
    const trustRead = fn.indexOf('viewerTrusted || isTrustedContributor()');
    expect(statusGate).toBeGreaterThan(-1);
    expect(trustRead).toBeGreaterThan(statusGate);
  });

  it('the owner side reads the event payload’s viewer_trusted for the event’s own patch', () => {
    expect(src).toMatch(
      /ownerAdmin = \$derived\(\n\s*speaksFor\(event\?\.node_slug, event\?\.node_status, event\?\.viewer_trusted === true\)\n\s*\)/
    );
  });

  it('the buttons for the other side list trusted patches beside admin ones', () => {
    // Admin memberships first (the line membership-standing.test.js pins),
    // then the per-patch grants, never the owner, never twice.
    expect(src).toMatch(/m\.role === 'admin' && m\.status === 'active' && m\.node_slug !== event\?\.node_slug/);
    expect(src).toMatch(/for \(const p of getTrustedPatches\(\)\) \{\n\s*if \(p\.slug !== event\?\.node_slug && !seen\.has\(p\.slug\)\)/);
    expect(src).toMatch(/\{#each spokenForPatches as patch \(patch\.slug\)\}/);
    expect(src).toMatch(/canAct = \$derived\(reachesBeyondOwn \|\| spokenForPatches\.length > 0\)/);
  });

  it('only the quilt-wide grant opens the picker; a per-patch one is enumerable', () => {
    expect(src).toMatch(/reachesBeyondOwn = \$derived\(ownerAdmin \|\| isTrustedContributor\(\)\)/);
    expect(src).not.toMatch(/reachesBeyondOwn = \$derived\([^)]*getTrustedPatches/);
  });

  it('confirming and removing on the linked side use the same rule, with the row’s status', () => {
    expect(src).toMatch(/function speaksForLinked\(l\) \{\n\s*return speaksFor\(l\.node_slug, l\.node_status\);/);
    expect(src).toMatch(/if \(l\.initiated_by === 'owner'\) return speaksForLinked\(l\);/);
    expect(src).toMatch(/function canRemove\(l\) \{\n\s*return ownerAdmin \|\| speaksForLinked\(l\);/);
    expect(src).not.toMatch(/return isAdmin\(\) \|\| getMembershipRoles\(\)\.get\(l\.node_slug\) === 'admin';/);
  });
});

describe('EventDetail hands the payload through untouched', () => {
  // The event page passes the whole event down, so `viewer_trusted` reaches
  // EventLinks without a second fetch of the patch.
  it('mounts EventLinks with the event it loaded', () => {
    expect(source('pages/EventDetail.svelte')).toMatch(/<EventLinks \{event\} onChanged=/);
  });
});
