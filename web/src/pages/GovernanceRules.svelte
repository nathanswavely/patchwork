<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { readableRules } from '../lib/governanceRules.js';
  import GovernanceShell from '../components/GovernanceShell.svelte';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';

  // A patch's rules, to read.
  //
  // There was no way to do that. The only route into the full rule set was
  // a link called "Propose a change to these rules", and the only page
  // behind it was the change form: twenty-odd fields, a review screen and a
  // Submit button. The co-op's admin wanted to check one setting and did
  // not click it. "I did not want to propose anything, I wanted to *look*.
  // I genuinely hesitated for a minute before clicking it, because on this
  // site proposing a thing seems to start a vote and I did not want to
  // start a vote by accident just to read our own settings."
  //
  // He was also sent here by a notification. When a patch closes its
  // charters to followers the notice says "you can grant it again in
  // Governance" and links to the hub, where that switch is not: it is at
  // the bottom of the change form. Reading is the thing the notice was
  // asking him to do, so this is where it points now.
  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isUnclaimed = $derived(patch.value.isUnclaimed);
  let isAdmin = $derived(patch.value.isAdmin);
  let membershipRole = $derived(patch.value.membershipRole);
  let canPropose = $derived(membershipRole === 'admin' || membershipRole === 'member');

  let rules = $state(null);
  let followerPermissions = $state(null);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    if (slug && isUnclaimed) {
      navigate(`/patches/${slug}/events`);
      return;
    }
    if (slug) load();
  });

  async function load() {
    loading = true;
    error = '';
    try {
      // The whole rule set, from the endpoint whose contract is that it
      // sends the whole rule set. It answers the fields flat, with the
      // follower permissions nested inside.
      const data = await api(`nodes/${slug}/governance/rules`);
      rules = data;
      followerPermissions = data.follower_permissions || null;
    } catch (e) {
      error = e.message || 'Could not load these rules';
      rules = null;
    } finally {
      loading = false;
    }
  }

  let lines = $derived(readableRules(rules, followerPermissions));

  // An admin on an admin-decided patch changes these directly; everyone
  // else who may act proposes. Naming which one it is here is the same
  // answer the change form owes on its review screen, and the reason the
  // co-op's admin backed out of that screen rather than press Submit.
  let directChange = $derived(
    membershipRole === 'admin' &&
    (rules?.decision_method === 'admin' || rules?.proposal_venue === 'elsewhere')
  );
</script>

<GovernanceShell activeSection="rules">
  {#snippet children()}
    <div class="rules-page page-fade">
      <div class="rules-head">
        <h1>Rules</h1>
        <p class="muted">
          What this patch has agreed, as it stands today. Nothing on this
          page changes anything.
        </p>
      </div>

      {#if loading}
        <Skeleton lines={8} height="1rem" />
      {:else if error}
        <ErrorState message={error} retry={load} />
      {:else if lines.length === 0}
        <p class="muted empty">This patch has set no governance rules.</p>
      {:else}
        <dl class="rule-list">
          {#each lines as line}
            <div class="rule">
              <dt>{line.label}</dt>
              <dd>
                {line.value}
                {#if line.hint}<span class="rule-hint muted">{line.hint}</span>{/if}
              </dd>
            </div>
          {/each}
        </dl>

        {#if isAdmin || canPropose}
          <div class="rules-actions">
            <a
              class="btn btn-secondary"
              href="/patches/{slug}/governance/rules/propose"
              onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/rules/propose`); }}
            >{directChange ? 'Change these rules' : 'Propose a change'}</a>
            <!-- What pressing it will do, before it is pressed. -->
            <span class="muted action-note">
              {#if directChange}
                Your change takes effect when you save it.
              {:else}
                Your change goes to the members as a proposal and is decided by a vote.
              {/if}
            </span>
          </div>
        {/if}
      {/if}
    </div>
  {/snippet}
</GovernanceShell>

<style>
  .rules-page {
    max-width: var(--pw-measure);
    margin: 0 auto;
    padding-top: 2rem;
  }

  .rules-head {
    margin-bottom: 1.5rem;
  }

  .rules-head h1 {
    margin-bottom: 0.35rem;
  }

  .rules-head p {
    font-size: 0.88rem;
    line-height: 1.6;
    margin: 0;
  }

  .empty {
    font-size: 0.88rem;
    padding: 2rem 0;
  }

  .rule-list {
    margin: 0;
    padding: 0;
  }

  .rule {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 1rem;
    padding: 0.7rem 0;
    border-top: 1px solid var(--color-border);
  }

  .rule:last-child {
    border-bottom: 1px solid var(--color-border);
  }

  .rule dt {
    flex: 0 0 15rem;
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .rule dd {
    flex: 1 1 14rem;
    margin: 0;
    font-size: 0.9rem;
  }

  .rule-hint {
    display: block;
    font-size: 0.82rem;
    line-height: 1.5;
    margin-top: 0.2rem;
  }

  .rules-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem;
    margin-top: 1.25rem;
  }

  .action-note {
    font-size: 0.83rem;
  }
</style>
