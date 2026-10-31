<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';
  import { markGovernanceDecided } from '../lib/onboarding.js';
  import StructuredRulesEditor from '../components/StructuredRulesEditor.svelte';
  import StructuredRulesDiff from '../components/StructuredRulesDiff.svelte';
  import { summarizeRuleChanges } from '../lib/rulesDiff.js';
  import Skeleton from '../components/Skeleton.svelte';
  import AIUseQuestion from '../components/AIUseQuestion.svelte';
  import { aiField, aiAnswerMissing } from '../lib/aiUse.js';
  import { getAiDisclosure } from '../stores/quilt.svelte.js';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isAdmin = $derived(patch.value.isAdmin);
  let membershipRole = $derived(patch.value.membershipRole);

  // The entry point on the governance hub is gated, but the route is
  // reachable by URL, so the page states the rule itself rather than
  // trusting the button that sent you. Derived from the role: following
  // carries no governance rights, and docs/adr/117 is why is_member now
  // agrees.
  let canPropose = $derived(membershipRole === 'member' || membershipRole === 'admin');

  let currentRules = $state(null);
  let proposedRules = $state(null);
  // Who votes here today, so the editor can say what a quorum or a tenure
  // bar comes to for this patch rather than leaving a founder to guess
  // (docs/adr/104). Its own request: the rules payload is spread back into
  // the submission, so nothing that is not a rule may travel in it.
  let electorate = $state(null);
  let loading = $state(true);
  let error = $state('');

  // Direct change (docs/adr/041): under admin-decides rules an admin's
  // submission applies immediately — the words propose/submit/vote never
  // appear for one.
  //
  // The admin the server means is this patch's admin (`isNodeAdmin`), not
  // `isAdmin`, which the node payload also sets for instance admins. An
  // instance admin who is a plain member here would have been told "Change
  // applied" while the server opened a vote.
  //
  // A patch that decides its proposals elsewhere is the second case
  // (docs/adr/053): the rules file is machine configuration, not a text a
  // meeting adopts, so it never comes back as an attestation and a rules
  // proposal there would sit open forever. An admin applies it directly; the
  // server refuses everyone else with the same reason.
  let directChange = $derived(
    membershipRole === 'admin' &&
    (currentRules?.decision_method === 'admin' || currentRules?.proposal_venue === 'elsewhere')
  );

  // What pressing the button will actually do, in a sentence, before it is
  // pressed.
  //
  // The review screen showed a diff, a title field and a Submit button and
  // never said whether submitting changed the setting or started a
  // fortnight-long vote. A co-op's only admin ticked one box to find out
  // what would happen, read "Proposed Rule Changes ... (17 fields
  // unchanged)" and a Submit button, could not tell which it was, and
  // backed out (F-117). The duration is the patch's own, because "a vote"
  // and "a vote that runs for two weeks" are different things to agree to.
  let voteLength = $derived.by(() => {
    const hours = currentRules?.default_vote_duration_hours ?? 0;
    if (!hours) return '';
    const days = Math.round(hours / 24);
    if (days >= 1) return days === 1 ? ' for a day' : ` for ${days} days`;
    return ` for ${hours} hours`;
  });

  // The two paths besides a direct change and a vote, in CreateProposal's
  // order. On a patch that decides elsewhere the server refuses a rules
  // proposal from anyone but an admin (docs/adr/053), so the page says so
  // and does not offer a button that can only fail. On an admin-decides
  // patch a member's change is a request to the maintainer, born
  // `awaiting_admin` with no ballot (docs/adr/092): telling them it "opens a
  // vote" described a vote that never runs.
  let refusedHere = $derived(!directChange && currentRules?.proposal_venue === 'elsewhere');
  let toMaintainer = $derived(
    !directChange && !refusedHere && currentRules?.decision_method === 'admin'
  );

  let submitConsequence = $derived(
    directChange
      ? 'Saving changes these rules now. Nobody votes on it.'
      : refusedHere
        ? 'This patch decides at its meetings, and the rules are not something a meeting adopts, so an admin applies a rules change directly. Ask an admin to make this one.'
        : toMaintainer
          ? "This goes to this patch's maintainer, who decides whether to apply it and may ask the members first. These rules do not change unless they approve it."
          : `This goes to the members as a proposal and opens a vote${voteLength}. These rules do not change unless it carries.`
  );

  // Step: 'editing' or 'reviewing'
  let step = $state('editing');
  let title = $state('');
  let description = $state('');
  // Whether AI wrote the proposal
  // (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md). Null until
  // somebody answers.
  let aiUse = $state(null);
  let submitting = $state(false);

  let currentRulesJSON = $derived(currentRules ? JSON.stringify(currentRules) : '');
  let proposedRulesJSON = $derived(proposedRules ? JSON.stringify(proposedRules) : '');
  let hasChanges = $derived(currentRulesJSON !== proposedRulesJSON);

  // Auto-save.
  let draftKey = $derived(slug ? `rules-draft-${slug}` : null);
  let draftRestored = $state(false);
  let draftTimestamp = $state(null);

  $effect(() => {
    if (slug) loadRules();
  });

  $effect(() => {
    patch.value.setBreadcrumbExtra?.([{ label: directChange ? 'Change rules' : 'Propose rules change' }]);
    return () => patch.value.setBreadcrumbExtra?.([]);
  });

  // Restore draft after rules load.
  $effect(() => {
    if (draftKey && currentRules) {
      try {
        const saved = localStorage.getItem(draftKey);
        if (saved) {
          const parsed = JSON.parse(saved);
          if (parsed.rules && JSON.stringify(parsed.rules) !== currentRulesJSON) {
            proposedRules = parsed.rules;
            draftTimestamp = parsed.timestamp;
            draftRestored = true;
          }
        }
      } catch {}
    }
  });

  // Debounced auto-save.
  $effect(() => {
    if (!draftKey || !hasChanges) return;
    const timeout = setTimeout(() => {
      localStorage.setItem(draftKey, JSON.stringify({
        rules: proposedRules,
        timestamp: Date.now(),
      }));
    }, 1000);
    return () => clearTimeout(timeout);
  });

  // Warn before navigating away.
  $effect(() => {
    if (!hasChanges) return;
    const handler = (e) => { e.preventDefault(); };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });

  async function loadRules() {
    loading = true;
    error = '';
    try {
      const data = await api(`nodes/${slug}/governance/rules`);
      currentRules = data.rules || data;
      proposedRules = JSON.parse(JSON.stringify(currentRules)); // deep copy
      // Best effort, and never fatal: the form works without the sentences,
      // and failing to load them must not stop somebody changing their rules.
      try {
        electorate = await api(`nodes/${slug}/governance/electorate`);
      } catch {
        electorate = null;
      }
    } catch (e) {
      error = e.message || 'Failed to load rules';
    } finally {
      loading = false;
    }
  }

  function discardDraft() {
    if (currentRules) proposedRules = JSON.parse(JSON.stringify(currentRules));
    draftRestored = false;
    if (draftKey) localStorage.removeItem(draftKey);
  }

  function handleRulesChange(newRules) {
    proposedRules = newRules;
  }

  // The prefilled Summary of changes, derived from the same answer the diff
  // beside it renders (web/src/lib/rulesDiff.js). It used to be generated
  // here from a list of five fields out of eighteen, so a change to either
  // venue produced "Update governance rules" over a diff naming both, and
  // three changes at once produced the name of one of them (F-038). The
  // value is submitted as `change_summary`, which is the entry the
  // governance Record keeps, so a summary describing a subset as the whole
  // is a misfiled act rather than a bad placeholder.
  //
  // `suggestedTitle` is only ever written into an empty field or over a
  // suggestion nobody has edited. Going back to editing, changing something
  // else and returning refreshes it; a sentence the author typed is theirs
  // and stays.
  let suggestedTitle = $state('');

  function goToReview() {
    const suggestion = summarizeRuleChanges(currentRules, proposedRules);
    if (!title || title === suggestedTitle) {
      title = suggestion;
      suggestedTitle = suggestion;
    }
    step = 'reviewing';
  }

  async function handleSubmit() {
    if (!title.trim()) {
      showToast('Title is required', 'error');
      return;
    }
    const aiMissing = aiAnswerMissing(getAiDisclosure(), aiUse);
    if (aiMissing) {
      showToast(aiMissing, 'error');
      return;
    }
    submitting = true;
    try {
      const payload = {
        title: title.trim(),
        body: description.trim() || '',
        proposal_type: 'amendment',
        target_doc: 'governance-rules.json',
        proposed_body: JSON.stringify(proposedRules, null, 2),
        proposed_title: 'Governance Rules',
        change_summary: title.trim(),
        ai_use: aiField(getAiDisclosure(), aiUse),
      };
      const result = await api(`nodes/${slug}/proposals`, { method: 'POST', body: payload });
      // Changing the rules is the plainest answer to "decide how you
      // govern", so it ticks that checklist item (lib/onboarding.js). Only
      // for this patch's own admins, who are the only people the checklist
      // is shown to, and only once the write has landed.
      if (membershipRole === 'admin' && patch.value.node?.id) {
        markGovernanceDecided(getUser()?.id, patch.value.node.id);
      }
      if (draftKey) localStorage.removeItem(draftKey);
      showToast(directChange ? 'Change applied' : toMaintainer ? 'Sent to the maintainer' : 'Proposal created', 'success');
      navigate(`/patches/${slug}/governance/${result.id}`);
    } catch (e) {
      showToast(e.message || 'Failed to create proposal', 'error');
    } finally {
      submitting = false;
    }
  }
</script>

<!-- The notice sits outside .rules-editor on purpose: that wrapper's measure
     cap is for the form's line length, and a centered notice inheriting it
     would center on the column instead of the page — visibly off-center, and
     narrower than the same notice everywhere else it appears. -->
{#if !canPropose}
  <div class="lt-empty lt-fade-in">
    <p class="lt-empty-title">Only members can propose a change to these rules.</p>
    <p class="lt-muted">Become a member to take part in how this patch governs itself.</p>
  </div>
{:else}
<div class="rules-editor lt-fade-in">
  {#if loading}
    <Skeleton lines={1} height="2rem" width="50%" />
    <Skeleton lines={6} height="0.9rem" />
  {:else if error}
    <div class="error-state">
      <p class="lt-error-text">{error}</p>
      <button class="lt-btn lt-btn-secondary" onclick={loadRules}>Retry</button>
    </div>
  {:else if currentRules}

    <div class="editor-header">
      <span class="editor-context lt-muted">{directChange ? 'Changing' : 'Proposing changes to'}</span>
      <h1>Governance Rules</h1>
    </div>

    {#if draftRestored}
      <div class="draft-banner">
        <span>Restored unsaved changes from {draftTimestamp ? new Date(draftTimestamp).toLocaleTimeString() : 'earlier'}</span>
        <button class="lt-btn-link lt-tone-error" onclick={discardDraft}>Discard</button>
      </div>
    {/if}

    {#if step === 'editing'}
      <div class="editor-card">
        <!-- Both halves of the change: the stored rules it is measured
             against, and the change in progress the form opens on. This step
             is unmounted while the review step is up, so the pending change
             has to come back in through a prop. -->
        <StructuredRulesEditor
          currentRules={currentRules}
          proposedRules={proposedRules}
          electorate={electorate}
          onSave={handleRulesChange}
        />

        <div class="editor-card-footer">
          <button class="lt-btn lt-btn-primary" onclick={goToReview} disabled={!hasChanges}>
            {directChange ? 'Review & apply' : 'Review & submit'}
          </button>
          <button class="lt-btn lt-btn-secondary" onclick={() => navigate(`/patches/${slug}/governance`)}>
            Cancel
          </button>
        </div>
      </div>

      {#if hasChanges}
        <div class="changes-section">
          <h2>Changes</h2>
          <StructuredRulesDiff {currentRules} {proposedRules} heading={directChange ? 'Rule changes' : undefined} />
        </div>
      {/if}

    {:else}
      <div class="review-step">
        <div class="review-card">
          <div class="review-card-header lt-muted">Your changes</div>
          <div class="review-card-body">
            <StructuredRulesDiff {currentRules} {proposedRules} heading={directChange ? 'Rule changes' : undefined} />
          </div>
        </div>

        <div class="review-card">
          <div class="review-card-header">{directChange ? 'Describe this change' : 'Describe your proposal'}</div>
          <div class="review-card-body">
            <div class="field">
              <label class="lt-label" for="rules-title">Summary of changes</label>
              <input class="lt-field" id="rules-title" type="text" bind:value={title} disabled={submitting} />
            </div>

            <div class="field">
              <label class="lt-label" for="rules-desc"><span>{directChange ? 'Why this change?' : 'Why are you proposing this?'} <span class="lt-optional">(optional)</span></span></label>
              <textarea class="lt-field" id="rules-desc" bind:value={description} rows="4" disabled={submitting} placeholder="Help others understand why this change matters."></textarea>
            </div>

            {#if getAiDisclosure() !== 'off'}
              <div class="field">
                <AIUseQuestion bind:value={aiUse} disabled={submitting} />
              </div>
            {/if}

            <p class="submit-consequence lt-muted">{submitConsequence}</p>

            <div class="review-actions">
              <button class="lt-btn lt-btn-primary" onclick={handleSubmit} disabled={submitting || !title.trim() || refusedHere}>
                {#if directChange}
                  {submitting ? 'Applying...' : 'Apply change'}
                {:else if toMaintainer}
                  {submitting ? 'Sending...' : 'Send to the maintainer'}
                {:else}
                  {submitting ? 'Submitting...' : 'Submit proposal'}
                {/if}
              </button>
              <button class="lt-btn lt-btn-secondary" onclick={() => { step = 'editing'; }} disabled={submitting}>
                Back to editing
              </button>
            </div>
          </div>
        </div>
      </div>
    {/if}

  {/if}
</div>
{/if}

<style>
  .rules-editor {
    max-width: var(--pw-measure-wide);
  }

  .editor-header {
    margin-bottom: 1.5rem;
  }

  .editor-context {
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .draft-banner {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.6rem 0.85rem;
    margin-bottom: 1rem;
    border: 1px solid color-mix(in srgb, var(--color-accent) 40%, var(--color-border));
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--color-accent) 8%, var(--color-surface));
    font-size: 0.82rem;
  }

  .editor-header h1 {
    font-size: 1.3rem;
    margin: 0.15rem 0;
  }

  .changes-section {
    margin-top: 1.5rem;
    padding-top: 1rem;
    border-top: 1px solid var(--color-border);
  }

  .changes-section h2 {
    font-size: 0.88rem;
    font-weight: 600;
    color: var(--color-text-muted);
    margin-bottom: 0.75rem;
  }

  .editor-card {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--color-surface);
  }

  .editor-card-footer {
    display: flex;
    gap: 0.75rem;
    padding: 0.75rem;
    border-top: 1px solid var(--color-border);
    background: var(--color-bg);
  }

  .review-step {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    margin-top: 1rem;
  }

  .review-card {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .review-card-header {
    padding: 0.6rem 0.85rem;
    font-size: 0.82rem;
    font-weight: 600;
    background: var(--color-bg);
    border-bottom: 1px solid var(--color-border);
  }

  .review-card-body {
    padding: 1.25rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    margin-bottom: 1rem;
  }

  .submit-consequence {
    font-size: 0.85rem;
    line-height: 1.55;
    margin: 0 0 0.75rem;
  }

  .review-actions {
    display: flex;
    gap: 0.75rem;
    padding-top: 0.5rem;
  }

  .error-state {
    padding: 2rem 0;
    text-align: center;
  }
</style>
