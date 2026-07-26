/**
 * The patch profile is a window, not a lobby (docs/adr/042).
 *
 * eventPostingRight is a pure function and is tested as one. The rest is
 * asserted against source text — there is no Svelte render library in this
 * project (see router.test.js, onboarding-workspace-surfaces.test.js).
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { eventPostingRight } from '../lib/patchWorkspace.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('eventPostingRight names the outcome, not the patch state', () => {
  const unclaimed = { signedIn: true, isUnclaimed: true, submissionsEnabled: true };

  it('offers nothing to signed-out or banned visitors', () => {
    expect(eventPostingRight({ signedIn: false })).toBe('none');
    expect(eventPostingRight({ signedIn: true, isBanned: true })).toBe('none');
  });

  // The bug this replaces: the old check short-circuited on isUnclaimed
  // before considering direct-post rights, so these two were offered a
  // review queue they bypass (events.go).
  it('lets the instance admin and trusted contributors post directly to an unclaimed patch', () => {
    expect(eventPostingRight({ ...unclaimed, isInstanceAdmin: true })).toBe('direct');
    expect(eventPostingRight({ ...unclaimed, trustedContributor: true })).toBe('direct');
  });

  it('sends everyone else on an unclaimed patch through review', () => {
    expect(eventPostingRight(unclaimed)).toBe('suggest');
  });

  it('honours the instance-wide submissions switch', () => {
    expect(eventPostingRight({ ...unclaimed, submissionsEnabled: false })).toBe('none');
  });

  it('lets members and admins post directly to an active patch', () => {
    expect(eventPostingRight({ signedIn: true, isMemberOrAdmin: true })).toBe('direct');
  });

  // Following is frictionless and grants no write rights. The server used
  // to call userHasMembership here, which counts followers too, so a
  // follower's event published straight to the calendar.
  it('sends a follower through review like any other visitor', () => {
    expect(eventPostingRight({
      signedIn: true, isMemberOrAdmin: false, submissionsEnabled: true, acceptSuggestions: true,
    })).toBe('suggest');
    expect(eventPostingRight({
      signedIn: true, isMemberOrAdmin: false, submissionsEnabled: true, acceptSuggestions: false,
    })).toBe('none');
  });

  it('offers a stranger the suggestion door only where the patch opted in', () => {
    const base = { signedIn: true, submissionsEnabled: true };
    expect(eventPostingRight({ ...base, acceptSuggestions: true })).toBe('suggest');
    expect(eventPostingRight({ ...base, acceptSuggestions: false })).toBe('none');
  });
});

describe('PatchProfile', () => {
  const src = source('pages/PatchProfile.svelte');

  it('carries no door named for the container — Manage and Governance pills are gone', () => {
    expect(src).not.toMatch(/>\s*Manage\s*</);
    expect(src).not.toMatch(/class="btn btn-secondary"[^>]*>\s*Governance\s*</);
  });

  it('holds exactly one control: the relationship row', () => {
    expect(src).toContain('<PatchRelationship');
    // Report, Subscribe and the workspace fallback live in the overflow.
    expect(src).toContain('<PatchOverflow');
    expect(src).not.toContain('<ReportButton');
  });

  it('navigates instead of opening modals — the windows are not painted shut', () => {
    expect(src).not.toContain('<Modal');
    expect(src).not.toMatch(/modalType/);
    expect(src).toMatch(/governance\/docs\/\$\{doc\.id\}/);
    expect(src).toMatch(/governance\/\$\{proposal\.id\}/);
  });

  it('gives every room a glimpse whose heading is its door', () => {
    for (const room of ['events', 'members', 'governance']) {
      expect(src).toMatch(new RegExp(`class="section-title" href="/patches/\\{slug\\}/${room}"`));
    }
  });

  it('shows the members glimpse the page never had (docs/adr/006)', () => {
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/members\?limit=\d+`\)/);
    expect(src).toContain('class="member-list"');
  });

  it('renders a glimpse when its room has content, or the viewer may act in it, or has standing', () => {
    expect(src).toMatch(/showEvents = \$derived\(recentEvents\.length > 0 \|\| hasStanding/);
    expect(src).toMatch(/showMembers = \$derived\(!isUnclaimed &&/);
    expect(src).toMatch(/postingRight !== 'none'/);
  });

  it('keeps governance and members off unclaimed patches (docs/adr/039)', () => {
    expect(src).toMatch(/canSeeGovernance = \$derived\(\s*!isUnclaimed/);
    expect(src).toMatch(/showMembers = \$derived\(!isUnclaimed/);
  });

  it('states the unclaimed fact as a header line, with the claim inside it', () => {
    expect(src).toContain('No one runs this patch yet.');
    expect(src).toContain('class="state-notice"');
    // Not a competing primary button any more.
    expect(src).not.toMatch(/btn-primary[^>]*>\s*Claim this patch/);
  });

  it('routes an instance admin to the claim queue rather than offering them a claim', () => {
    expect(src).toMatch(/\{#if isAdmin\}[\s\S]{0,200}\/admin\/claims/);
  });

  it('derives standing from the membership role, never from instance-admin power', () => {
    expect(src).toMatch(/hasStanding = \$derived\(\['follower', 'member', 'admin'\]\.includes\(membershipRole\)\)/);
  });
});

describe('PatchRelationship', () => {
  const src = source('components/PatchRelationship.svelte');

  it('puts the exit inside the standing control, never beside it', () => {
    expect(src).toMatch(/standing-menu[\s\S]{0,200}handleLeave/);
    expect(src).not.toMatch(/class="btn[^"]*"[^>]*onclick={handleLeave}/);
  });

  it('states standing at rest for follower, member and admin alike', () => {
    expect(src).toContain("label: 'Following'");
    expect(src).toContain("label: 'Member'");
    expect(src).toContain("label: 'Admin'");
  });

  it('makes Follow the stranger primary and the membership rung secondary', () => {
    expect(src).toMatch(/class="btn btn-primary \{btnSize\}" onclick={handleFollow}/);
    expect(src).toMatch(/standing \? 'btn-primary' : 'btn-secondary'/);
  });

  it('drops the membership rung where the server would refuse it', () => {
    expect(src).toMatch(/!isUnclaimed &&\s*node\?\.membership_policy !== 'invite_only'/);
  });

  it('never says "Join" for a patch — that word belongs to joining the quilt', () => {
    expect(src).not.toMatch(/>Join</);
  });
});

describe('PatchOverflow', () => {
  const src = source('components/PatchOverflow.svelte');

  it('brings the per-patch feeds to the public page', () => {
    expect(src).toMatch(/events\.ics/);
    expect(src).toMatch(/events\.rss/);
    expect(src).toMatch(/feedAvailable = \$derived\(node\?\.visibility === 'public'\)/);
  });

  it('carries the workspace fallback for people with standing only', () => {
    expect(src).toContain('Workspace view');
    expect(src).toMatch(/canEnterWorkspace = \$derived\(hasStanding \|\| isAdmin\)/);
  });

  it('sends an unclaimed patch to its events calendar, which is its workspace root', () => {
    expect(src).toMatch(/isUnclaimed \? 'events' : 'governance'/);
  });

  it('homes Report, and hides it from the people who run the patch', () => {
    expect(src).toMatch(/canReport = \$derived\(isLoggedIn\(\) && !isAdmin/);
  });
});
