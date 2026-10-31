import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { welcomePath, welcomeNext } from '../lib/onboarding.js';
import { isSafeRedirectPath } from '../stores/router.svelte.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

// Six governance-simulation findings in the app shell and its copy. These
// assert source text; each was also checked on screen against a running app.

describe('F-072: a notification body is read in full', () => {
  // The row is one component now, shared with the bell's dropdown, which
  // clamps through its own .clamp rule. The page must never ask for it.
  const row = source('components/NotificationRow.svelte');
  const rule = row.slice(row.indexOf('.notif-body {'), row.indexOf('}', row.indexOf('.notif-body {')));
  const page = source('pages/Notifications.svelte');

  it('does not clamp the body to one line', () => {
    expect(rule).not.toContain('nowrap');
    expect(rule).not.toContain('text-overflow');
    expect(rule).not.toContain('line-clamp');
  });

  it('the page shows the body whole; only the dropdown clamps', () => {
    expect(page).toMatch(/<NotificationRow [^>]*onDismiss/);
    expect(page).not.toMatch(/<NotificationRow [^>]*clamp/);
    expect(source('components/NotificationDropdown.svelte')).toMatch(/<NotificationRow [^>]*clamp/);
  });

  it('keeps a long URL from widening the row', () => {
    expect(rule).toContain('overflow-wrap: anywhere');
  });
});

describe('F-053: welcome keeps the address that was asked for', () => {
  it('carries a same-origin path through /welcome', () => {
    expect(welcomePath('/patches/the-selvage')).toBe('/welcome?next=%2Fpatches%2Fthe-selvage');
    expect(welcomeNext(new URLSearchParams('next=%2Fpatches%2Fthe-selvage'))).toBe('/patches/the-selvage');
  });

  it('keeps a query string on the requested path', () => {
    const path = welcomePath('/patches/x/manage?tab=members');
    expect(welcomeNext(new URLSearchParams(path.split('?')[1]))).toBe('/patches/x/manage?tab=members');
  });

  it('drops anything that would leave the site', () => {
    for (const bad of ['https://evil.example', '//evil.example', '/\\evil.example', '/\t/evil.example', 'javascript:alert(1)', '']) {
      expect(welcomePath(bad)).toBe('/welcome');
      expect(welcomeNext(new URLSearchParams({ next: bad }))).toBe('');
    }
  });

  it('does not carry home or welcome itself', () => {
    expect(welcomePath('/')).toBe('/welcome');
    expect(welcomePath('/welcome?next=%2Fx')).toBe('/welcome');
  });

  it('rejects control characters the URL parser would strip', () => {
    expect(isSafeRedirectPath('/\n/evil.example')).toBe(false);
    expect(isSafeRedirectPath('/patches/ok')).toBe(true);
  });

  it('is what the zero-membership redirect sends, replacing history', () => {
    const src = source('App.svelte');
    expect(src).toContain('replaceRoute(welcomePath(getPath()');
    expect(src).not.toContain("navigate('/welcome')");
  });

  it('returns the person there from the primary button', () => {
    const src = source('pages/Welcome.svelte');
    expect(src).toContain('welcomeNext(getQuery())');
    expect(src).toContain("navigate(next || '/discover')");
  });
});

describe('F-031: a refused copy puts the text on screen', () => {
  for (const file of ['components/SetupChecklist.svelte', 'components/SubscribeFeeds.svelte', 'pages/AccountSettings.svelte']) {
    it(`${file} selects the text instead of only saying so`, () => {
      const src = source(file);
      expect(src).not.toContain('Select the address instead');
      expect(src).toContain('use:selectWhen=');
      expect(src).toContain('user-select: all');
    });
  }

  it('SetupChecklist shows the address it never displayed before', () => {
    const src = source('components/SetupChecklist.svelte');
    expect(src).toMatch(/<code class="copy-fallback"[^>]*>\{copyFailed\.value\}<\/code>/);
  });
});

describe('F-077: PasskeyNotice stays quiet for someone who can confirm', () => {
  const src = source('components/PasskeyNotice.svelte');

  it('reads the step-up status itself', () => {
    expect(src).toContain("import { stepUpStatus } from '../lib/stepUp.js'");
    expect(src).toContain('recovery_ready');
    expect(src).toContain('status?.active');
  });

  it('renders only once the answer says nobody can confirm', () => {
    expect(src).toContain('{#if show && status && !canConfirmAnyway}');
  });

  it('keeps the wording F-047 gave it', () => {
    expect(src).toMatch(/a recovery code confirms too/);
  });
});

describe('F-034: a signed-out visitor learns joining is a request', () => {
  const src = source('components/PatchRelationship.svelte');

  it('says so only signed out, on an approval-required patch', () => {
    expect(src).toMatch(/canBecomeMember && !isLoggedIn\(\) && node\?\.membership_policy === 'approval_required'/);
    expect(src).toContain('Joining is a request a patch admin answers.');
  });
});

describe('F-037: the creation tile says it was picked at random', () => {
  it('names the random starting point', () => {
    expect(source('pages/PatchForm.svelte')).toMatch(/The one shown is a random\s+starting point\./);
  });
});
