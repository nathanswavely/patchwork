/**
 * The Hosting Patch field on the event form.
 *
 * The field asks one of two things depending on the door walked in through,
 * and answers with the matching control. On the unscoped create page it is a
 * choice, so it is the patch picker over the whole quilt (CONTEXT.md "Patch
 * picker"). Editing an event cannot move it between patches, and the suggest
 * door (docs/adr/026) is already scoped to the patch it came from — neither
 * is a choice, so neither gets a control.
 *
 * Because the corpus is the whole quilt, most rows are patches the person
 * cannot post to directly. The row says which, and the button follows the
 * pick, so nobody presses "Create" and gets a submission.
 *
 * Asserted against source text — there is no Svelte render library in this
 * project (see patch-profile-window.test.js).
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { reachablePatchPickerProvider } from '../lib/finderProviders.js';
import { api } from '../lib/api.js';

vi.mock('../lib/api.js', () => ({ api: vi.fn() }));

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const form = source('pages/EventForm.svelte');
const picker = source('components/WorkspaceSearch.svelte');

describe('choosing a patch is the picker, over the whole quilt', () => {
  it('uses the reachable corpus rather than the me/nodes list', () => {
    expect(form).toContain("import { reachablePatchPickerProvider } from '../lib/finderProviders.js'");
    expect(form).toMatch(/return reachablePatchPickerProvider\(/);
    // The old corpus was the patches you belong to. Nothing should fetch it.
    expect(form).not.toContain("api('me/nodes')");
  });

  it('opens on the corpus instead of demanding a guess first', () => {
    expect(form).toMatch(/<WorkspaceSearch[\s\S]*?browse/);
    expect(picker).toMatch(/browse = false,/);
    expect(picker).toMatch(/if \(!query\.trim\(\)\) return browse \? items\.slice\(0, 12\) : \[\]/);
    expect(picker).toMatch(/\{#if open && \(query\.trim\(\) \|\| browse\)\}/);
  });

  it('does not let Enter in the picker submit the form around it', () => {
    // Without the guard, a keypress meant for the dropdown reaches the form.
    expect(picker).toMatch(/if \(!open\) return;/);
    expect(picker).toMatch(/if \(activeIndex < 0\) return;/);
  });

  it('renders the picker variant, not a select', () => {
    expect(form).toMatch(/<WorkspaceSearch[\s\S]*?variant="picker"/);
    expect(form).toMatch(/provider=\{hostingPatchProvider\}/);
    expect(form).toMatch(/onSelect=\{choosePatch\}/);
    expect(form).not.toMatch(/<select id="node"/);
  });

  it('keeps the field labelled: the picker takes the id the label points at', () => {
    expect(form).toContain('<label for="node">Hosting Patch');
    expect(form).toMatch(/inputId="node"/);
    // The prop has to actually reach the input, or the label names nothing.
    expect(picker).toMatch(/inputId = null,/);
    expect(picker).toMatch(/<input[\s\S]*?id=\{inputId\}/);
  });
});

describe('a fixed patch is stated, not asked', () => {
  it('treats edit and the suggest door as fixed', () => {
    expect(form).toMatch(/let fixed = \$derived\(isEdit \|\| !!lockSlug\)/);
  });

  it('renders the name instead of a control when fixed', () => {
    expect(form).toMatch(/\{#if fixed\}[\s\S]*?<p class="fixed-patch">\{hostingPatch\.name\}<\/p>/);
  });
});

describe('the row and the button say what will happen', () => {
  it('counts only active memberships, the way userHasNodeRole does', () => {
    // me/nodes serves 'active' and 'pending'; a pending join grants nothing.
    expect(form).toMatch(/if \(m\.status === 'active'\) roles\.set\(m\.node_slug, m\.role\)/);
    expect(form).not.toContain('getMembershipRoles');
  });

  it('mirrors CreateEvent on who posts directly (docs/adr/026)', () => {
    // Instance admin anywhere; trusted contributor on unclaimed only;
    // member or admin of an active patch. A follower is none of these.
    expect(form).toMatch(/function postsDirectly\(status, slug\)/);
    expect(form).toMatch(/if \(isAdmin\(\)\) return true/);
    expect(form).toMatch(/if \(status === 'unclaimed'\) return isTrustedContributor\(\)/);
    expect(form).toMatch(/role === 'member' \|\| role === 'admin'/);
  });

  it('shows a patch that refuses suggestions rather than hiding it', () => {
    expect(form).toMatch(/function refusalFor\(node\)/);
    expect(form).toMatch(/!node\.accept_event_suggestions/);
    expect(form).toMatch(/disabled: !!refusal/);
  });

  it('labels every other row with the review it will get', () => {
    expect(form).toMatch(/refusal \|\| 'will be reviewed'/);
  });

  it('makes the button follow the pick, not the door', () => {
    expect(form).toMatch(/let willReview = \$derived\(\s*!!hostingPatch && !postsDirectly\(/);
    expect(form).toMatch(/willReview \? 'Suggest Event' : 'Create Event'/);
    // And nothing may be submitted before a patch is chosen.
    expect(form).toMatch(/disabled=\{submitting \|\| !nodeId\}/);
  });

  it('routes the confirmation to whoever owns the queue', () => {
    expect(form).toMatch(/hostingPatch\?\.status === 'unclaimed' \? 'quilt admins' : 'patch admins'/);
  });
});

// The public listing pins visibility = 'public' (internal/handler/nodes.go),
// so a private patch the person belongs to — and can post to directly — is
// absent from it. A picker that omits it says "not on this quilt" about a
// patch they are standing in.
describe('the corpus reaches the caller’s own patches', () => {
  beforeEach(() => { api.mockReset(); });
  afterEach(() => { vi.restoreAllMocks(); });

  function pages(byQuery) {
    api.mockImplementation((path) => {
      const scoped = path.includes('scope=my');
      return Promise.resolve({ items: byQuery(scoped), next_cursor: '' });
    });
  }

  it('asks for the public set and the caller’s own, and unions them', async () => {
    pages((scoped) =>
      scoped
        ? [{ id: 'p1', name: 'Back Room', slug: 'back-room', status: 'active' }]
        : [{ id: 'p2', name: 'Gallery Row', slug: 'gallery-row', status: 'active' }]
    );
    const rows = await reachablePatchPickerProvider((n) => ({ type: 'Patches', sublabel: n.slug }));
    expect(rows.map((r) => r.label).sort()).toEqual(['Back Room', 'Gallery Row']);
    expect(api.mock.calls.some(([p]) => p.includes('scope=my'))).toBe(true);
  });

  it('lists a patch once when it is both public and the caller’s', async () => {
    const shared = { id: 'p1', name: 'The Selvage', slug: 'the-selvage', status: 'active' };
    pages(() => [shared]);
    const rows = await reachablePatchPickerProvider((n) => ({ type: 'Patches', sublabel: n.slug }));
    expect(rows).toHaveLength(1);
  });
});
