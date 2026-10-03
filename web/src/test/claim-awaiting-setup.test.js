/**
 * An approved claim's two standing surfaces (docs/adr/039).
 *
 * Approving a claim does not activate the patch: it hands the claimant a
 * 14-day right to submit setup, which is where the patch goes active and
 * they become its admin. Correct, and invisible. The claim dropped out of
 * the admin queue (which listed 'pending' only), the patch page went on
 * saying "No one runs this patch yet" to everyone including the claimant,
 * and My Patches is built from memberships — which an approved claimant does
 * not have yet. So the approval's only trace was the notification announcing
 * it, and the window could close without either party seeing anything.
 *
 * Both additions are surfaces you have to be someone to see: the instance
 * admin's own panel, and the claimant's own settings. Neither is the
 * "awaiting setup" badge ADR 039 forbids — that rule is about what visitors
 * read on the patch, which still says unclaimed to everybody.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const adminSrc = source('pages/AdminClaims.svelte');
const patchesSrc = source('pages/UserSettingsPatches.svelte');

describe('Admin claims: approval leaves a trace an admin can return to', () => {
  it('loads the approved queue alongside the review queue', () => {
    expect(adminSrc).toContain("api('admin/claims?status=approved')");
    expect(adminSrc).toMatch(/awaitingSetup = approved\?\.items \|\| \[\]/);
  });

  it('files them under their own heading, separate from review', () => {
    expect(adminSrc).toContain('<h2 class="queue-heading">Awaiting review</h2>');
    expect(adminSrc).toContain('<h2 class="queue-heading">Approved, awaiting setup</h2>');
  });

  it('says how long the wait lasts', () => {
    const awaiting = adminSrc.slice(adminSrc.indexOf('Approved, awaiting setup'));
    expect(awaiting).toContain('approval expires {formatDate(claim.setup_expires_at)}');
  });

  it('offers no verdict on a claim already decided', () => {
    const awaiting = adminSrc.slice(adminSrc.indexOf('Approved, awaiting setup'));
    expect(awaiting).not.toContain("handleAction(claim.id, 'approve')");
    expect(awaiting).not.toContain("handleAction(claim.id, 'reject')");
  });

  it('empties only when both queues are empty', () => {
    expect(adminSrc).toContain('{:else if claims.length === 0 && awaitingSetup.length === 0}');
    expect(adminSrc).not.toContain('No pending claims.');
  });

  it('refreshes both queues after a verdict, so an approval lands in the other one', () => {
    const load = adminSrc.slice(adminSrc.indexOf('async function loadClaims'));
    expect(load).toContain("api('admin/claims')");
    expect(load).toContain("api('admin/claims?status=approved')");
    expect(adminSrc).toMatch(/handleAction[\s\S]{0,600}await loadClaims\(\)/);
  });
});

describe('My Patches: the claim you owe an act on', () => {
  it('loads the caller\'s claims, which memberships cannot supply', () => {
    expect(patchesSrc).toContain("api('users/me/claims')");
    expect(patchesSrc).toContain('<h3 class="section-heading">Claiming</h3>');
  });

  it('distinguishes a right to act from a wait on someone else', () => {
    const claiming = patchesSrc.slice(
      patchesSrc.indexOf('<h3 class="section-heading">Claiming</h3>'),
      patchesSrc.indexOf('<h3 class="section-heading">Archived</h3>')
    );
    expect(claiming).toContain('ready to set up');
    expect(claiming).toContain('claim under review');
    expect(claiming).toContain('Set up patch');
  });

  it('sends the approved claimant to setup, which is where the patch activates', () => {
    expect(patchesSrc).toMatch(/navigate\(`\/patches\/\$\{c\.node_slug\}\/setup`\)/);
  });

  it('says the patch still reads unclaimed to everyone else', () => {
    // The one fact this row cannot show and the claimant most needs: their
    // approval changed nothing anybody else can see.
    expect(patchesSrc).toContain(
      'the patch still reads as unclaimed to everyone else'
    );
  });

  it('shows the deadline on the approved row', () => {
    expect(patchesSrc).toContain('expires {formatDate(c.setup_expires_at)}');
  });

  // Every section this page can render has to count toward "you have
  // nothing here", or the page tells somebody looking straight at a row
  // that they have none. Archived patches joined the list (F-124) and the
  // condition grew with them.
  it('does not tell a claimant with no memberships that they have nothing', () => {
    expect(patchesSrc).toContain(
      '{#if patches.length === 0 && myClaims.length === 0 && archivedPatches.length === 0}'
    );
    expect(patchesSrc).not.toContain('{:else if patches.length === 0}');
  });
});
