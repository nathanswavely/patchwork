<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import StructuredRulesEditor from '../components/StructuredRulesEditor.svelte';
  import StructuredRulesDiff from '../components/StructuredRulesDiff.svelte';
  import Skeleton from '../components/Skeleton.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isAdmin = $derived(patch.value.isAdmin);
  let membershipRole = $derived(patch.value.membershipRole);

  // The entry point on the governance hub is gated, but the route is
  // reachable by URL, so the page states the rule itself rather than
  // trusting the button that sent you. Not `isMember` — the node payload
  // sets is_member for followers too, and following carries no governance
  // rights.
  let canPropose = $derived(membershipRole === 'member' || membershipRole === 'admin');

  let currentRules = $state(null);
  let proposedRules = $state(null);
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

  // Step: 'editing' or 'reviewing'
  let step = $state('editing');
  let title = $state('');
  let description = $state('');
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

  // Generate title from changed fields.
  const TITLE_VALUES = {
    admin: 'admin decides',
    majority: 'majority vote',
    supermajority: 'supermajority vote',
    consensus: 'consensus',
    approval_required: 'approval-required',
    invite_only: 'invite-only',
  };
  function titleValue(v) {
    return TITLE_VALUES[v] || String(v).replace(/_/g, ' ');
  }
  function generateTitle() {
    if (!currentRules || !proposedRules) return 'Update governance rules';
    const changes = [];
    if (currentRules.decision_method !== proposedRules.decision_method) {
      changes.push(`decisions by ${titleValue(proposedRules.decision_method)}`);
    }
    if (currentRules.quorum_percent !== proposedRules.quorum_percent) {
      changes.push(`set quorum to ${proposedRules.quorum_percent}%`);
    }
    if (currentRules.default_vote_duration_hours !== proposedRules.default_vote_duration_hours) {
      const days = Math.round(proposedRules.default_vote_duration_hours / 24);
      changes.push(`${days}-day voting period`);
    }
    if (currentRules.membership_policy !== proposedRules.membership_policy) {
      changes.push(`${titleValue(proposedRules.membership_policy)} membership`);
    }
    if (currentRules.leadership_model !== proposedRules.leadership_model) {
      changes.push(`${titleValue(proposedRules.leadership_model)} leadership`);
    }
    if (changes.length === 0) return 'Update governance rules';
    return changes.join(', ').replace(/^./, c => c.toUpperCase());
  }

  function goToReview() {
    if (!title) title = generateTitle();
    step = 'reviewing';
  }

  async function handleSubmit() {
    if (!title.trim()) {
      showToast('Title is required', 'error');
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
      };
      const result = await api(`nodes/${slug}/proposals`, { method: 'POST', body: payload });
      if (draftKey) localStorage.removeItem(draftKey);
      showToast(directChange ? 'Change applied' : 'Proposal created', 'success');
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
  <div class="permission-notice page-fade">
    <p>Only members can propose a change to these rules.</p>
    <p class="muted">Become a member to take part in how this patch governs itself.</p>
  </div>
{:else}
<div class="rules-editor page-fade">
  {#if loading}
    <Skeleton lines={1} height="2rem" width="50%" />
    <Skeleton lines={6} height="0.9rem" />
  {:else if error}
    <div class="error-state">
      <p class="error-text">{error}</p>
      <button class="btn btn-secondary" onclick={loadRules}>Retry</button>
    </div>
  {:else if currentRules}

    <div class="editor-header">
      <span class="editor-context muted">{directChange ? 'Changing' : 'Proposing changes to'}</span>
      <h1>Governance Rules</h1>
    </div>

    {#if draftRestored}
      <div class="draft-banner">
        <span>Restored unsaved changes from {draftTimestamp ? new Date(draftTimestamp).toLocaleTimeString() : 'earlier'}</span>
        <button class="btn-link danger" onclick={discardDraft}>Discard</button>
      </div>
    {/if}

    {#if step === 'editing'}
      <div class="editor-card">
        <StructuredRulesEditor
          currentRules={currentRules}
          onSave={handleRulesChange}
        />

        <div class="editor-card-footer">
          <button class="btn btn-primary" onclick={goToReview} disabled={!hasChanges}>
            {directChange ? 'Review & apply' : 'Review & submit'}
          </button>
          <button class="btn btn-secondary" onclick={() => navigate(`/patches/${slug}/governance`)}>
            Cancel
          </button>
        </div>
      </div>

      {#if hasChanges}
        <div class="changes-section">
          <h2>Changes</h2>
          <StructuredRulesDiff {currentRules} {proposedRules} />
        </div>
      {/if}

    {:else}
      <div class="review-step">
        <div class="review-card">
          <div class="review-card-header muted">Your changes</div>
          <div class="review-card-body">
            <StructuredRulesDiff {currentRules} {proposedRules} />
          </div>
        </div>

        <div class="review-card">
          <div class="review-card-header">{directChange ? 'Describe this change' : 'Describe your proposal'}</div>
          <div class="review-card-body">
            <div class="field">
              <label for="rules-title">Summary of changes</label>
              <input id="rules-title" type="text" bind:value={title} disabled={submitting} />
            </div>

            <div class="field">
              <label for="rules-desc">{directChange ? 'Why this change?' : 'Why are you proposing this?'} <span class="muted">(optional)</span></label>
              <textarea id="rules-desc" bind:value={description} rows="4" disabled={submitting} placeholder="Help others understand why this change matters."></textarea>
            </div>

            <div class="review-actions">
              <button class="btn btn-primary" onclick={handleSubmit} disabled={submitting || !title.trim()}>
                {#if directChange}
                  {submitting ? 'Applying...' : 'Apply change'}
                {:else}
                  {submitting ? 'Submitting...' : 'Submit proposal'}
                {/if}
              </button>
              <button class="btn btn-secondary" onclick={() => { step = 'editing'; }} disabled={submitting}>
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

  .field label {
    font-size: 0.85rem;
    font-weight: 500;
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

  .permission-notice {
    text-align: center;
    padding: 3rem 1rem;
  }

  .permission-notice p:first-child {
    font-weight: 500;
    margin-bottom: 0.25rem;
  }
</style>
