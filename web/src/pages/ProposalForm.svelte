<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { allMembers } from '../lib/patchMembers.js';
  import { navigate, getQuery } from '../stores/router.svelte.js';
  import { formatDay } from '../lib/datetime.js';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import SegmentedControl from '../components/SegmentedControl.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let membershipRole = $derived(patch.value.membershipRole);

  // The New Proposal button on the proposals list is already gated, but this
  // route is reachable by URL, so the page states the rule itself. From
  // the role: following carries no governance rights, and docs/adr/117 is
  // why is_member now agrees.
  let canPropose = $derived(membershipRole === 'member' || membershipRole === 'admin');

  // The patch's decision method decides what this form is (docs/adr/092).
  // On an admin-decides patch the maintainer decides every proposal: an
  // admin's is a direct change unless they ask the members first, and a
  // member's is a request to the maintainer. The form used to offer everyone
  // a voting duration and a "Submit Proposal" button, then apply an admin's
  // instantly — docs/adr/041 says the UI never says "propose", "submit" or
  // "vote" for a direct change, and the rules editor honoured that while
  // this page did not. `membershipRole`, not `isAdmin`: the node payload
  // sets is_admin for instance admins too, and deciding here needs the
  // patch's own role.
  let decisionMethod = $derived(patch.value.node?.governance_config?.decision_method || '');
  let adminDecides = $derived(decisionMethod === 'admin');
  let isPatchAdmin = $derived(membershipRole === 'admin');
  // Where the patch decides (docs/adr/053). Elsewhere, CreateProposal opens
  // a proposal for discussion with no ballot and no clock, and the decision
  // comes back as an attestation — so there is no window to choose and
  // nothing to "submit" to a vote. The form used to offer both regardless.
  let decidesElsewhere = $derived(patch.value.node?.governance_config?.proposal_venue === 'elsewhere');
  let putToVote = $state(false);
  // The cases in CreateProposal's order, so the form says what the server
  // will do: an admin's direct change wins even elsewhere; past that a
  // patch that decides elsewhere discusses, whoever raised it; and only
  // then does admin-decides send a member's to the maintainer or let an
  // admin ask the members first.
  let directChange = $derived(adminDecides && isPatchAdmin && !putToVote);
  let forDiscussion = $derived(decidesElsewhere && !directChange);
  let advisoryVote = $derived(adminDecides && isPatchAdmin && putToVote && !decidesElsewhere);
  let toMaintainer = $derived(adminDecides && !isPatchAdmin && !decidesElsewhere);
  let asksDuration = $derived(!forDiscussion && (!adminDecides || advisoryVote));
  // The document door's own label, so the pointer to it names the button
  // the reader will actually find (GovernanceList, GovernanceDetail).
  let docDoor = $derived(adminDecides && isPatchAdmin ? 'Edit' : 'Propose change');

  $effect(() => {
    patch.value.setBreadcrumbExtra?.([{ label: adminDecides && isPatchAdmin ? 'New change' : 'New Proposal' }]);
    return () => patch.value.setBreadcrumbExtra?.([]);
  });

  let title = $state('');
  let body = $state('');
  // The council page links here with ?type=membership from a vacant chair's
  // "Nominate a member", so the form opens on the thing that was clicked
  // rather than on Action with the reader hunting for the radio they just
  // asked for. The $effect below still drops it if this patch cannot
  // nominate, so a stale or hand-typed link cannot select an option the
  // server would refuse.
  let proposalType = $state(getQuery().get('type') === 'membership' ? 'membership' : 'action');
  // The patch's own default window, which is also what the server uses
  // when none is sent. 72 hours stands in only where the patch names none.
  let durationHours = $state(72);
  let durationTouched = $state(false);
  let defaultDuration = $derived(patch.value.node?.governance_config?.default_vote_duration_hours || 72);
  $effect(() => {
    if (!durationTouched) durationHours = defaultDuration;
  });
  let submitting = $state(false);
  let error = $state('');
  let previewMode = $state(false);

  const baseDurationOptions = [
    { value: 24, label: '24 hours' },
    { value: 48, label: '48 hours' },
    { value: 72, label: '72 hours (3 days)' },
    { value: 168, label: '1 week' },
    { value: 336, label: '2 weeks' },
  ];
  // A patch whose default is not one of the presets still gets it offered,
  // rather than a select that silently shows the first option.
  let durationOptions = $derived(
    baseDurationOptions.some((o) => o.value === defaultDuration)
      ? baseDurationOptions
      : [...baseDurationOptions, { value: defaultDuration, label: hoursLabel(defaultDuration) }].sort((a, b) => a.value - b.value)
  );
  function hoursLabel(h) {
    if (h % 168 === 0) {
      const w = h / 168;
      return w === 1 ? '1 week' : `${w} weeks`;
    }
    if (h % 24 === 0) {
      const d = h / 24;
      return d === 1 ? '1 day' : `${d} days`;
    }
    return `${h} hours`;
  }

  // Nominating an admin (docs/adr/051, docs/adr/100). A meritocratic patch
  // ratifies admins, and an elected one fills a mid-term vacancy the same
  // way — so there the nomination also needs a vacant seat to land in.
  // `membershipRole`, not `isAdmin`: the node payload sets is_admin for
  // instance admins too, and the server refuses them.
  let nomineeId = $state('');
  let leadershipModel = $state('');
  let vacantSeats = $state(0);
  // Whether the governance overview has answered yet, either way.
  let govLoaded = $state(false);
  let nextContestOpens = $state('');
  let nominatable = $state([]);
  let isMeritocratic = $derived(leadershipModel === 'meritocratic');
  let isElected = $derived(leadershipModel === 'elected');
  let canNominate = $derived(isPatchAdmin && (isMeritocratic || (isElected && vacantSeats > 0)));

  // A membership proposal is a nomination and nothing else (docs/adr/100):
  // it has to name the person it is about, and only someone who may nominate
  // can raise one. Offering the type to everyone is what left a member with
  // no way to ask for a seat except a targetless vote on his own name.
  let typeOptions = $derived([
    { value: 'action', label: 'Action', description: 'Propose a concrete action for the community to take' },
    ...(canNominate
      ? [{ value: 'membership', label: 'Membership', description: 'Nominate an active member for admin' }]
      : []),
    { value: 'other', label: 'Other', description: 'Any other proposal that needs community input' },
  ]);

  $effect(() => {
    if (slug) loadGovernanceContext();
  });

  // Keep the chosen type valid: an elected patch whose last seat was filled
  // stops offering Membership, and a radio left on a vanished option would
  // submit a type the server refuses.
  // Waits for the overview: `canNominate` is false until it lands, so an
  // unguarded reset would undo the ?type=membership the council page just
  // sent and drop the reader back on Action.
  $effect(() => {
    if (govLoaded && proposalType === 'membership' && !canNominate) proposalType = 'action';
  });

  async function loadGovernanceContext() {
    try {
      const ov = await api(`nodes/${slug}/governance/overview`);
      leadershipModel = ov?.rules?.leadership_venue === 'elsewhere' ? '' : (ov?.rules?.leadership_model || '');
      // `vacant`, not an absent holder_id: a chair whose holder this
      // viewer is not shown carries no holder_id either (docs/adr/095's
      // roster rule reaching the council), and counting those as vacancies
      // would offer a nomination into an occupied seat.
      vacantSeats = (ov?.seats || []).filter((s) => s.vacant).length;
      nextContestOpens = ov?.next_contest_opens || '';
      if (!canNominate) return;
      // Only plain members can be nominated: an admin already holds the role,
      // and a follower is not on the ladder. Every page of them: the listing
      // is paged, and a picker built on page one hid everyone after it.
      nominatable = (await allMembers(slug, { role: 'member' })).filter((m) => m.role === 'member');
    } catch {
      leadershipModel = '';
      nominatable = [];
    } finally {
      govLoaded = true;
    }
  }

  async function handleSubmit() {
    if (!title.trim()) {
      error = 'Title is required';
      return;
    }
    if (proposalType === 'membership' && !nomineeId) {
      error = 'Choose the member this nomination is about';
      return;
    }

    error = '';
    submitting = true;
    try {
      const payload = {
        title: title.trim(),
        body: body.trim() || '',
        proposal_type: proposalType,
      };
      // A window only where a vote will run. A direct change and a request
      // to the maintainer have no ballot, so sending one would be a number
      // the server ignores and the form pretended to mean something.
      if (asksDuration) payload.duration_hours = durationHours;
      if (adminDecides && isPatchAdmin) payload.put_to_vote = putToVote;
      // Only a membership proposal carries a subject, and only where the
      // community ratifies admins.
      if (proposalType === 'membership' && nomineeId) {
        payload.target_user_id = nomineeId;
      }
      const result = await api(`nodes/${slug}/proposals`, {
        method: 'POST',
        body: payload,
      });
      navigate(`/patches/${slug}/governance/${result.id}`);
    } catch (e) {
      error = e.message || 'Failed to create proposal';
    } finally {
      submitting = false;
    }
  }
</script>

<!-- Outside .container-narrow on purpose: a centered notice inheriting that
     column would center on the column instead of the page. -->
{#if !canPropose}
  <div class="lt-empty lt-fade-in">
    <p class="lt-empty-title">Only members can create proposals.</p>
    <p class="lt-muted">Become a member to take part in how this patch governs itself.</p>
  </div>
{:else}
<div class="lt-fade-in">
  <div class="container-narrow">
    <div style="padding-top: 2rem;">
      <p class="amendment-hint lt-muted">Want to change a governance document or rules? Go to the <a href="/patches/{slug}/governance/docs" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/docs`); }}>Documents</a> page and click "{docDoor}" on the document you want to change.</p>

      <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
        <div class="field">
          <label class="lt-label" for="title"><span>Title <span class="lt-required">*</span></span></label>
          <input class="lt-field" id="title" type="text" bind:value={title} disabled={submitting} required />
        </div>

        <div class="field">
          <label class="lt-label">
            <span>Description</span>
            <SegmentedControl
              options={[{ value: 'write', label: 'Write' }, { value: 'preview', label: 'Preview' }]}
              value={previewMode ? 'preview' : 'write'}
              label="Write or preview"
              onchange={(v) => { previewMode = v === 'preview'; }}
            />
          </label>
          {#if previewMode}
            <div class="preview-pane">
              {#if body.trim()}
                <MarkdownRenderer content={body} />
              {:else}
                <span class="lt-muted">Nothing to preview</span>
              {/if}
            </div>
          {:else}
            <textarea
              class="lt-field"
              id="body"
              bind:value={body}
              rows="8"
              placeholder="Explain your proposal and why it matters... Use **bold**, *italic*, and - lists."
              disabled={submitting}
            ></textarea>
          {/if}
        </div>

        <div class="field">
          <label class="lt-label">Type</label>
          <div class="type-radio-group">
            {#each typeOptions as opt}
              <label class="type-radio-option" class:selected={proposalType === opt.value}>
                <input
                  type="radio"
                  name="proposal-type"
                  value={opt.value}
                  bind:group={proposalType}
                  disabled={submitting}
                />
                <div class="type-radio-content">
                  <span class="type-radio-label">{opt.label}</span>
                  <span class="type-radio-desc">{opt.description}</span>
                </div>
              </label>
            {/each}
          </div>

          <!-- Nominating for admin (docs/adr/051). Only appears where the
               community ratifies admins and only for someone who may
               nominate; everywhere else a membership proposal is ordinary. -->
          {#if proposalType === 'membership' && canNominate}
            <div class="nominee-field">
              <label class="lt-label" for="nominee"><span>Nominate for admin <span class="lt-required">*</span></span></label>
              {#if nominatable.length > 0}
                <select class="lt-field" id="nominee" bind:value={nomineeId} disabled={submitting} required>
                  <option value="">Choose a member</option>
                  {#each nominatable as m}
                    <option value={m.user_id}>{m.display_name || m.username}</option>
                  {/each}
                </select>
                <p class="nominee-hint lt-muted">
                  {#if isElected}
                    If this passes, they take the vacant seat and serve out its term. Nobody has to approve it afterwards.
                  {:else}
                    If this passes, they become an admin. Nobody has to approve it afterwards.
                  {/if}
                </p>
              {:else}
                <p class="nominee-hint lt-muted">There is nobody to nominate yet.</p>
              {/if}
            </div>
          {/if}

          <!-- An elected patch with no vacancy: say where the way onto the
               council actually is, rather than leaving someone to invent
               one (docs/adr/100). -->
          {#if isElected && !canNominate}
            <p class="nominee-hint lt-muted">
              {#if !isPatchAdmin}
                Admins here hold seats on the council. A vacant seat is filled by nomination, which an admin raises; otherwise a seat comes up at an election any member may stand in.
              {:else if nextContestOpens}
                Every seat on the council is held, so there is nobody to nominate. The next election opens {formatDay(nextContestOpens)}. You can add a seat on the Governance page.
              {:else}
                Every seat on the council is held, so there is nobody to nominate. You can add a seat on the Governance page.
              {/if}
            </p>
          {/if}
        </div>

        <!-- Who decides this (docs/adr/092). On an admin-decides patch an
             admin chooses between applying now and asking the members first;
             a member is told their proposal goes to the maintainer. On every
             voting patch nothing appears here: the vote is the decision. -->
        {#if adminDecides && isPatchAdmin}
          <div class="field">
            <label class="lt-label">How this gets decided</label>
            <div class="type-radio-group">
              <label class="type-radio-option" class:selected={!putToVote}>
                <input type="radio" name="ceremony" value={false} bind:group={putToVote} disabled={submitting} />
                <div class="type-radio-content">
                  <span class="type-radio-label">Apply it now</span>
                  <span class="type-radio-desc">You decide this patch's proposals. This takes effect as soon as you save it.</span>
                </div>
              </label>
              <label class="type-radio-option" class:selected={putToVote}>
                <input type="radio" name="ceremony" value={true} bind:group={putToVote} disabled={submitting} />
                <div class="type-radio-content">
                  {#if decidesElsewhere}
                    <span class="type-radio-label">Open it for discussion</span>
                    <span class="type-radio-desc">Nobody votes here. It stays open for discussion until the meeting's decision is recorded.</span>
                  {:else}
                    <span class="type-radio-label">Ask the members first</span>
                    <span class="type-radio-desc">Opens an advisory vote. You still decide, at any time, with the tally in front of you.</span>
                  {/if}
                </div>
              </label>
            </div>
          </div>
        {:else if toMaintainer}
          <p class="ceremony-note lt-muted">
            This patch's maintainer decides proposals. Yours goes to them, and they may ask the members before deciding.
          </p>
        {:else if forDiscussion}
          <p class="ceremony-note lt-muted">
            This patch decides its proposals at its meetings. Yours opens here for discussion, with no vote, and the decision is recorded once the meeting has made it.
          </p>
        {/if}

        {#if asksDuration}
          <div class="field">
            <label class="lt-label" for="duration">{advisoryVote ? 'How long to ask' : 'Voting Duration'}</label>
            <select class="lt-field" id="duration" bind:value={durationHours} onchange={() => { durationTouched = true; }} disabled={submitting}>
              {#each durationOptions as opt}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
            <span class="duration-tip">This patch's default is {hoursLabel(defaultDuration)}.</span>
          </div>
        {/if}

        {#if error}
          <p class="lt-error-text">{error}</p>
        {/if}

        <div class="lt-actions field-actions">
          <button type="submit" class="lt-btn lt-btn-primary" disabled={submitting}>
            {#if directChange}
              {submitting ? 'Applying...' : 'Apply change'}
            {:else if advisoryVote}
              {submitting ? 'Opening...' : 'Open advisory vote'}
            {:else if toMaintainer}
              {submitting ? 'Sending...' : 'Send to the maintainer'}
            {:else if forDiscussion}
              {submitting ? 'Opening...' : 'Open for discussion'}
            {:else}
              {submitting ? 'Creating...' : 'Submit Proposal'}
            {/if}
          </button>
          <button
            type="button"
            class="lt-btn lt-btn-secondary"
            onclick={() => navigate(`/patches/${slug}/governance`)}
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
</div>
{/if}

<style>

  form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  /* Direct child only: the field's header label (e.g. "Description" + its
     Write/Preview toggle). Without `>` this also matched the nested
     .type-radio-option labels and shoved their radio and text to opposite
     edges via space-between. */
  /* A label's words and its Write/Preview toggle share a line. */
  .field > label {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
  }

  textarea {
    min-height: 160px;
  }

  .preview-pane {
    padding: 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    min-height: 160px;
    background: var(--color-bg);
    line-height: 1.6;
    font-size: 0.9rem;
  }

  .preview-pane :global(strong) { font-weight: 700; }
  .preview-pane :global(em) { font-style: italic; }
  .preview-pane :global(ul) { padding-left: 1.5rem; margin: 0.5rem 0; }
  .preview-pane :global(p) { margin: 0.5rem 0; }

  .nominee-field {
    margin-top: 0.75rem;
  }

  .nominee-field select {
    width: 100%;
  }

  .nominee-hint {
    font-size: 0.78rem;
    margin: 0.35rem 0 0;
  }

  .type-radio-group {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .type-radio-option {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .type-radio-option:hover {
    border-color: var(--color-primary);
  }

  .type-radio-option.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 5%, transparent);
  }

  .type-radio-option input[type="radio"] {
    margin-top: 0.15rem;
    accent-color: var(--color-primary);
  }

  .type-radio-content {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .type-radio-label {
    font-size: 0.9rem;
    font-weight: 500;
    color: var(--color-text);
  }

  .type-radio-desc {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .ceremony-note {
    font-size: 0.88rem;
    margin: 0 0 1.25rem;
  }

  .duration-tip {
    font-size: 0.8rem;
    color: var(--color-text-muted);
    margin-top: 0.25rem;
  }

  .field-actions {
    padding-top: 0.5rem;
  }
</style>
