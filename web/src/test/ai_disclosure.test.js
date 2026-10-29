/**
 * A post says whether AI made it
 * (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md): the quilt's
 * switch, the three-answer question on every surface that publishes, the
 * "Made with AI" tag where readers see the post, the patch-level declaration
 * and its badge, the personal hide, and the report reason with its label
 * action.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text. lib/aiUse.js is plain JS and is run.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  AI_USE_OPTIONS,
  aiField,
  aiAnswerMissing,
  listAiValue,
  reportReasonLabel,
  UNLABELLED_AI,
} from '../lib/aiUse.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('lib/aiUse: the answers and the mode', () => {
  it('offers three answers, in the order the ADR lists them', () => {
    expect(AI_USE_OPTIONS.map((o) => o.value)).toEqual(['none', 'assisted', 'generated']);
    expect(AI_USE_OPTIONS.map((o) => o.label)).toEqual([
      'No AI',
      'AI helped (editing, cleanup, translation)',
      'AI made this',
    ]);
  });

  it('sends nothing when off, and what the form shows otherwise', () => {
    expect(aiField('off', 'generated')).toBeUndefined();
    expect(aiField('optional', null)).toBeNull();
    expect(aiField('optional', 'assisted')).toBe('assisted');
    expect(aiField('required', 'none')).toBe('none');
  });

  it('blocks only in required mode, and only on an unanswered question', () => {
    expect(aiAnswerMissing('optional', null)).toBe('');
    expect(aiAnswerMissing('off', null)).toBe('');
    expect(aiAnswerMissing('required', 'none')).toBe('');
    expect(aiAnswerMissing('required', 'none', null)).not.toBe('');
    expect(aiAnswerMissing('required')).toBe('');
  });

  it('tags a list row when either the words or the image were generated', () => {
    expect(listAiValue({ ai_use: 'generated' })).toBe('generated');
    expect(listAiValue({ image_ai: 'generated' })).toBe('generated');
    expect(listAiValue({ ai_use: 'assisted', image_ai: 'none' })).toBeNull();
    expect(listAiValue(null)).toBeNull();
  });

  it('names the unlabelled_ai reason in words and leaves the others alone', () => {
    expect(UNLABELLED_AI).toBe('unlabelled_ai');
    expect(reportReasonLabel('unlabelled_ai')).not.toBe('unlabelled_ai');
    expect(reportReasonLabel('Spam or scam')).toBe('Spam or scam');
  });
});

describe('quilt store: the instance switch', () => {
  const src = source('stores/quilt.svelte.js');

  it('defaults to optional and reads ai_disclosure from /instance', () => {
    expect(src).toMatch(/let instanceAiDisclosure = \$state\('optional'\)/);
    expect(src).toMatch(/data\?\.ai_disclosure/);
    expect(src).toMatch(/export function getAiDisclosure\(\)/);
  });
});

describe('AIUseQuestion: three radios, none preselected', () => {
  const src = source('components/AIUseQuestion.svelte');

  it('renders nothing when the quilt has the question off', () => {
    expect(src).toMatch(/\{#if mode !== 'off'\}/);
  });

  it('is a fieldset of radios bound to a value that starts null', () => {
    expect(src).toMatch(/<fieldset/);
    expect(src).toMatch(/value = \$bindable\(null\)/);
    expect(src).toMatch(/type="radio"/);
    expect(src).toMatch(/bind:group=\{value\}/);
    expect(src).not.toMatch(/checked/);
  });

  it('marks itself required in required mode', () => {
    expect(src).toMatch(/mode === 'required'/);
    expect(src).toMatch(/lt-required/);
  });
});

describe('MadeWithAI: only generated is shown', () => {
  const src = source('components/MadeWithAI.svelte');

  it('wears a pill for generated and nothing else', () => {
    expect(src).toMatch(/\{#if value === 'generated'\}/);
    expect(src).toMatch(/lt-pill/);
    expect(src).toMatch(/'Image made with AI' : 'Made with AI'/);
  });
});

describe('EventForm: the description and the image', () => {
  const src = source('pages/EventForm.svelte');

  it('pre-fills both on edit', () => {
    expect(src).toMatch(/aiUse = event\.ai_use \?\? null/);
    expect(src).toMatch(/imageAi = event\.image_ai \?\? null/);
  });

  it('does not ask on an imported event', () => {
    expect(src).toMatch(/imported = !!event\.source_id/);
    expect(src).toMatch(/let askAi = \$derived\(!imported && getAiDisclosure\(\) !== 'off'\)/);
  });

  it('asks about the image beside its description, only when there is one', () => {
    expect(src).toMatch(/<AIUseQuestion bind:value=\{imageAi\} legend="Was AI used to make this image\?"/);
  });

  it('checks required mode and sends both', () => {
    expect(src).toMatch(/aiAnswerMissing\(getAiDisclosure\(\), aiUse, \.\.\.\(imageUrl\.trim\(\) \? \[imageAi\] : \[\]\)\)/);
    expect(src).toMatch(/ai_use: imported \? undefined : aiField\(getAiDisclosure\(\), aiUse\)/);
    expect(src).toMatch(/image_ai: imported \? undefined : aiField\(/);
  });
});

describe('EventDetail: tags and a report', () => {
  const src = source('pages/EventDetail.svelte');

  it('tags the image and the description separately', () => {
    expect(src).toMatch(/<MadeWithAI value=\{event\.image_ai\} image \/>/);
    expect(src).toMatch(/<MadeWithAI value=\{event\.ai_use\} \/>/);
  });

  it('can be reported', () => {
    expect(src).toMatch(/<ReportButton entityType="event"/);
  });
});

describe('event lists: one tag per row', () => {
  for (const file of ['pages/EventsPage.svelte', 'pages/PatchEvents.svelte', 'components/PatchProfileGlimpses.svelte']) {
    it(`${file} tags a row made with AI`, () => {
      expect(source(file)).toMatch(/<MadeWithAI value=\{listAiValue\(event\)\} \/>/);
    });
  }
});

describe('PatchForm: description and declaration at creation and setup', () => {
  const src = source('pages/PatchForm.svelte');

  it('seeds the description answer from the listing, and never the declaration', () => {
    expect(src).toMatch(/let aiUse = \$state\(initial\?\.ai_use \?\? null\)/);
    expect(src).toMatch(/let aiWork = \$state\(null\)/);
  });

  it('asks the declaration and sends both on create and on the setup PATCH', () => {
    expect(src).toMatch(/<AIWorkQuestion bind:value=\{aiWork\}/);
    expect(src.match(/ai_use: aiField\(getAiDisclosure\(\), aiUse\),/g)).toHaveLength(2);
    expect(src.match(/ai_work: aiWork,/g)).toHaveLength(2);
  });
});

describe('AIWorkQuestion: Yes or No, neither preselected', () => {
  const src = source('components/AIWorkQuestion.svelte');

  it('asks the ADR question with examples', () => {
    expect(src).toMatch(/Does the work this patch makes or publishes rely heavily on AI-generated content\?/);
    expect(src).toMatch(/a newsletter written by a model/);
  });

  it('checks a radio only for an explicit true or false', () => {
    expect(src).toMatch(/checked=\{value === true\}/);
    expect(src).toMatch(/checked=\{value === false\}/);
  });
});

describe('PatchSettingsInfo: description, image and declaration', () => {
  const src = source('pages/PatchSettingsInfo.svelte');

  it('sends the description answer with every description edit', () => {
    expect(src).toMatch(/body: \{ description: value, ai_use: aiField\(getAiDisclosure\(\), descAiUse\) \}/);
  });

  it('saves the image answer with the image pair', () => {
    expect(src).toMatch(/image_ai: aiField\(getAiDisclosure\(\), imageUrl\.trim\(\) \? imageAi : null\)/);
  });

  it('saves the declaration on pick', () => {
    expect(src).toMatch(/saveField\('ai_work', value\)/);
    expect(src).toMatch(/<AIWorkQuestion value=\{node\?\.ai_work \?\? null\}/);
  });
});

describe('SubmitPatch: the description only', () => {
  const src = source('pages/SubmitPatch.svelte');

  it('asks and sends ai_use, and never ai_work', () => {
    expect(src).toMatch(/ai_use: aiField\(getAiDisclosure\(\), aiUse\)/);
    expect(src).not.toMatch(/ai_work/);
    expect(src).not.toMatch(/AIWorkQuestion/);
  });
});

describe('proposals and charters', () => {
  // A new charter is not asked: creating one writes no commit for the
  // AI-Use trailer to ride on. Its text changes through AmendmentEditor.
  for (const file of ['pages/ProposalForm.svelte', 'pages/AmendmentEditor.svelte', 'pages/RulesProposalEditor.svelte']) {
    it(`${file} asks, checks and sends ai_use`, () => {
      const src = source(file);
      expect(src).toMatch(/<AIUseQuestion bind:value=\{aiUse\}/);
      expect(src).toMatch(/aiAnswerMissing\(getAiDisclosure\(\), aiUse\)/);
      expect(src).toMatch(/ai_use: aiField\(getAiDisclosure\(\), aiUse\)/);
    });
  }

  it('ProposalDetail wears the tag and can be reported', () => {
    const src = source('pages/ProposalDetail.svelte');
    expect(src).toMatch(/<MadeWithAI value=\{proposal\.ai_use\} \/>/);
    expect(src).toMatch(/<ReportButton entityType="proposal"/);
  });

  it('the charter history tags a version from its trailer', () => {
    expect(source('pages/GovernanceVersionHistory.svelte')).toMatch(/<MadeWithAI value=\{ver\.ai_use\} \/>/);
  });
});

describe('notices', () => {
  it('the composer asks and sends ai_use, and the list wears the tag', () => {
    const src = source('pages/PatchNoticeboard.svelte');
    expect(src).toMatch(/<AIUseQuestion bind:value=\{aiUse\}/);
    expect(src).toMatch(/ai_use: aiField\(getAiDisclosure\(\), aiUse\)/);
    expect(src).toMatch(/<MadeWithAI value=\{n\.ai_use\} \/>/);
  });

  it('the edit pre-fills and sends it back, and the notice wears the tag', () => {
    const src = source('pages/NoticeDetail.svelte');
    expect(src).toMatch(/editAiUse = notice\.ai_use \?\? null/);
    expect(src).toMatch(/ai_use: aiField\(getAiDisclosure\(\), editAiUse\)/);
    expect(src).toMatch(/<MadeWithAI value=\{notice\.ai_use\} \/>/);
  });

  it('notice images are not asked about', () => {
    expect(source('pages/PatchNoticeboard.svelte')).not.toMatch(/image_ai/);
    expect(source('pages/NoticeDetail.svelte')).not.toMatch(/image_ai/);
  });
});

describe('the AI work badge', () => {
  it('the patch head wears it with a tooltip saying who declared it', () => {
    const src = source('components/PatchProfileHead.svelte');
    expect(src).toMatch(/\{#if node\.ai_work === true\}/);
    expect(src).toMatch(/>AI work<\/span>/);
    expect(src).toMatch(/This patch's admins say its work relies heavily on AI\./);
  });

  it('the cards wear it', () => {
    expect(source('pages/SocialHome.svelte')).toMatch(/\{#if patch\.ai_work === true\}/);
    expect(source('pages/Discover.svelte')).toMatch(/\{#if patch\.ai_work === true\}/);
  });

  it('the quilt canvas draws no mark for it', () => {
    expect(source('components/QuiltCanvas.svelte')).not.toMatch(/ai_work/);
    expect(source('lib/quiltCloth.js')).not.toMatch(/ai_work/);
  });
});

describe('AIWorkPrompt: the one-time question for admins', () => {
  const src = source('components/AIWorkPrompt.svelte');

  it('shows to a patch admin of an active patch with no answer', () => {
    expect(src).toMatch(/patch\.value\.membershipRole === 'admin'/);
    expect(src).toMatch(/isAdmin && !isUnclaimed && !!node && node\.ai_work !== true && node\.ai_work !== false/);
  });

  it('answers with a PATCH of ai_work', () => {
    expect(src).toMatch(/body: \{ ai_work: value \}/);
  });

  it('is mounted beside the setup checklist', () => {
    expect(source('components/GovernanceOverview.svelte')).toMatch(/<SetupChecklist \/>\s+<AIWorkPrompt \/>/);
  });
});

describe('settings', () => {
  it('Admin settings sets ai_disclosure with a three-way control', () => {
    const src = source('pages/AdminQuiltSettings.svelte');
    expect(src).toMatch(/body: \{ ai_disclosure: value \}/);
    expect(src).toMatch(/\{ value: 'off', label: 'Off' \}/);
    expect(src).toMatch(/\{ value: 'optional', label: 'Optional' \}/);
    expect(src).toMatch(/\{ value: 'required', label: 'Required' \}/);
  });

  it('Account settings hides AI work from discovery', () => {
    const src = source('pages/AccountSettings.svelte');
    expect(src).toMatch(/hideAiWork = !!me\.hide_ai_work/);
    expect(src).toMatch(/body: \{ hide_ai_work: value \}/);
  });
});

describe('reports', () => {
  it('ReportButton sends unlabelled_ai as a code, and only where the question is asked', () => {
    const src = source('components/ReportButton.svelte');
    expect(src).toMatch(/value: 'unlabelled_ai'/);
    expect(src).toMatch(/const AI_ASKED = new Set\(\['node', 'event', 'proposal', 'notice'\]\)/);
    expect(src).toMatch(/proposal: 'proposal'/);
    expect(src).toMatch(/<option value=\{r\.value\}>\{r\.label\}<\/option>/);
  });

  it('AdminReports labels text and images, and resolves both', () => {
    const src = source('pages/AdminReports.svelte');
    expect(src).toMatch(/<option value="label">/);
    expect(src).toMatch(/<option value="label_image">/);
    expect(src).toMatch(/label: 'resolved',/);
    expect(src).toMatch(/label_image: 'resolved',/);
    expect(src).toMatch(/reportReasonLabel\(report\.reason\)/);
  });

  it('the patch queue labels a notice and never a reply', () => {
    const src = source('pages/PatchSettingsNoticeboard.svelte');
    expect(src).toMatch(/\{#if r\.entity_type === 'notice'\}\s+<option value="label">/);
    expect(src).toMatch(/reportReasonLabel\(r\.reason\)/);
  });
});

describe('links to the AI page', () => {
  it('About and the Label both link to /ai', () => {
    expect(source('pages/About.svelte')).toMatch(/href="\/ai"/);
    expect(source('pages/Label.svelte')).toMatch(/href="\/ai"/);
  });
});
