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
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const form = source('pages/EventForm.svelte');
const picker = source('components/WorkspaceSearch.svelte');

describe('choosing a patch is the picker, over the whole quilt', () => {
  it('uses the shared picker corpus rather than the me/nodes list', () => {
    expect(form).toContain("import { patchPickerProvider } from '../lib/finderProviders.js'");
    expect(form).toMatch(/return patchPickerProvider\(/);
    // The old corpus was the patches you belong to. Nothing should fetch it.
    expect(form).not.toContain("api('me/nodes')");
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
