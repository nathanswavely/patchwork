<script>
  import { api } from '../lib/api.js';
  import { getContext } from 'svelte';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';

  const patch = getContext('patch');
  let node = $derived(patch.value.node);
  let slug = $derived(patch.value.slug);

  // Server state: the caller's open claim (survives reloads) and which
  // methods this patch supports.
  let loading = $state(true);
  let claim = $state(null);
  let methods = $state({ dns: false, meta_tag: false, email: false, admin: true });
  let verificationDomain = $state('');

  // Form state. No preselection: the method choice is consequential, and a
  // silent default is how people end up in a claim they didn't want.
  let selectedMethod = $state('');
  let evidence = $state('');
  let email = $state('');
  let claiming = $state(false);
  let withdrawing = $state(false);
  let resending = $state(false);
  let verifying = $state(false);
  let error = $state('');

  const methodInfo = [
    { id: 'dns', label: 'DNS Verification', desc: 'Add a TXT record to your domain' },
    { id: 'meta_tag', label: 'Website Meta Tag', desc: 'Add a meta tag to your homepage' },
    { id: 'email', label: 'Email Verification', desc: 'Get a link at your organization email' },
    { id: 'admin', label: 'Admin Review', desc: 'Submit evidence for manual review' },
  ];

  // Name the actual missing prerequisite, not a catch-all: email can be
  // unavailable because the domain is missing OR because the quilt can't
  // send email — a combined message misleads whichever half is fine.
  function unavailableReason(id) {
    if (!verificationDomain) return 'Requires a verified domain on this listing';
    if (id === 'email') return "This quilt can't send email";
    return 'Unavailable on this listing';
  }

  const methodLabels = Object.fromEntries(methodInfo.map((m) => [m.id, m.label]));

  $effect(() => {
    if (slug) loadClaimState();
  });

  async function loadClaimState() {
    loading = true;
    error = '';
    try {
      const data = await api(`nodes/${slug}/claims/mine`);
      claim = data.claim;
      methods = data.methods || methods;
      verificationDomain = data.verification_domain || '';
    } catch (e) {
      error = e.message || 'Failed to load claim status';
    } finally {
      loading = false;
    }
  }

  async function handleClaim() {
    if (!selectedMethod) return;
    claiming = true;
    error = '';
    try {
      const body = { method: selectedMethod };
      if (selectedMethod === 'admin') body.evidence = evidence.trim();
      if (selectedMethod === 'email') body.email = email.trim();
      const result = await api(`nodes/${slug}/claim`, { method: 'POST', body });
      claim = result;
      if (selectedMethod === 'admin') {
        showToast('Claim submitted for review', 'success');
      }
    } catch (e) {
      error = e.message || 'Failed to submit claim';
    } finally {
      claiming = false;
    }
  }

  async function handleVerify() {
    if (!claim?.id) return;
    verifying = true;
    error = '';
    try {
      const result = await api(`claims/${claim.id}/verify`, { method: 'POST' });
      if (result.verified) {
        showToast('Patch claimed! You are now the owner.', 'success');
        navigate(`/patches/${slug}`);
      } else {
        error = result.error || 'Verification failed. Please try again.';
      }
    } catch (e) {
      error = e.message || 'Verification failed';
    } finally {
      verifying = false;
    }
  }

  async function handleWithdraw() {
    if (!claim?.id) return;
    withdrawing = true;
    error = '';
    try {
      await api(`claims/${claim.id}/withdraw`, { method: 'POST' });
      showToast('Claim withdrawn', 'success');
      claim = null;
      selectedMethod = '';
      evidence = '';
      email = '';
    } catch (e) {
      error = e.message || 'Failed to withdraw claim';
    } finally {
      withdrawing = false;
    }
  }

  async function handleResend() {
    if (!claim?.id) return;
    resending = true;
    error = '';
    try {
      await api(`claims/${claim.id}/resend-email`, { method: 'POST' });
      showToast('Verification email sent', 'success');
    } catch (e) {
      error = e.message || 'Failed to resend email';
    } finally {
      resending = false;
    }
  }

  let submitDisabled = $derived(
    claiming
    || !selectedMethod
    || (selectedMethod === 'email' && !email.includes('@'))
  );
</script>

<div class="claim-page page-fade">
  <h1>Claim {node?.name || 'this patch'}</h1>
  <p class="muted">Prove you represent this organization to take ownership.</p>

  {#if loading}
    <Skeleton lines={4} height="1rem" />

  {:else if claim}
    <!-- An open claim: its instructions, actions, and a real way out. -->
    <div class="card verify-card">
      <div class="verify-header">
        <h2>Your claim is open</h2>
        <span class="badge">{methodLabels[claim.method] || claim.method}</span>
      </div>

      {#if claim.method === 'dns'}
        <p>Add a <strong>TXT record</strong> on <strong>{verificationDomain}</strong>:</p>
        <code class="verify-code">{claim.record_value}</code>
        <p class="muted">DNS changes can take a few minutes to propagate.</p>

      {:else if claim.method === 'meta_tag'}
        <p>Add this tag to the <code>&lt;head&gt;</code> of <strong>https://{verificationDomain}</strong>:</p>
        <code class="verify-code">&lt;meta name="patchwork-verify" content="{claim.meta_content}"&gt;</code>

      {:else if claim.method === 'email'}
        <p>We sent a verification link to <strong>{claim.email}</strong>. Open it to finish claiming. The link expires 24 hours after it's sent.</p>

      {:else if claim.method === 'admin'}
        <p>Your claim is with the quilt admins for review. You'll be notified when it's resolved.</p>
      {/if}

      <div class="form-actions" style="margin-top: 1rem;">
        {#if claim.method === 'dns' || claim.method === 'meta_tag'}
          <button class="btn btn-primary" onclick={handleVerify} disabled={verifying}>
            {verifying ? 'Checking...' : 'Verify Now'}
          </button>
        {:else if claim.method === 'email'}
          <button class="btn btn-primary" onclick={handleResend} disabled={resending}>
            {resending ? 'Sending...' : 'Resend Email'}
          </button>
        {/if}
        <button class="btn btn-secondary" onclick={handleWithdraw} disabled={withdrawing}>
          {withdrawing ? 'Withdrawing...' : 'Withdraw Claim'}
        </button>
      </div>
      <p class="muted withdraw-hint">Withdrawing lets you start over with a different method.</p>

      {#if error}
        <p class="error-text" style="margin-top: 0.75rem;">{error}</p>
      {/if}
    </div>

  {:else}
    <!-- Method selection -->
    <div class="methods">
      {#each methodInfo as method}
        {@const available = methods[method.id]}
        <label class="method-card" class:selected={selectedMethod === method.id} class:disabled={!available}>
          <input type="radio" name="method" value={method.id} bind:group={selectedMethod} disabled={!available} />
          <div>
            <strong>{method.label}</strong>
            <span class="muted">{available ? method.desc : unavailableReason(method.id)}</span>
          </div>
        </label>
      {/each}
    </div>

    {#if selectedMethod === 'admin'}
      <div class="field">
        <label for="evidence">Tell us why you should own this patch</label>
        <textarea id="evidence" bind:value={evidence} rows="4" placeholder="I'm the owner of this venue / I run this organization / etc."></textarea>
      </div>
    {/if}

    {#if selectedMethod === 'email'}
      <div class="field">
        <label for="claim-email">Your email at @{verificationDomain}</label>
        <input id="claim-email" type="email" bind:value={email} placeholder={'you@' + verificationDomain} />
        <span class="muted field-hint">The address must be at the organization's verified domain.</span>
      </div>
    {/if}

    {#if error}
      <p class="error-text">{error}</p>
    {/if}

    <div class="form-actions">
      <button class="btn btn-primary" onclick={handleClaim} disabled={submitDisabled}>
        {claiming ? 'Submitting...' : 'Submit Claim'}
      </button>
      <button class="btn btn-secondary" onclick={() => navigate(`/patches/${slug}`)}>Back</button>
    </div>
  {/if}
</div>

<style>
  .claim-page {
    max-width: 600px;
  }

  h1 {
    font-size: 1.4rem;
    margin-bottom: 0.25rem;
  }

  h1 + .muted {
    margin-bottom: 1.5rem;
    font-size: 0.88rem;
  }

  .methods {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-bottom: 1.25rem;
  }

  .method-card {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.75rem 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .method-card:hover {
    border-color: var(--color-primary);
  }

  .method-card.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 5%, var(--color-surface));
  }

  .method-card.disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .method-card.disabled:hover {
    border-color: var(--color-border);
  }

  .method-card input[type="radio"] {
    margin: 0;
  }

  .method-card div {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .method-card strong {
    font-size: 0.9rem;
  }

  .method-card .muted {
    font-size: 0.78rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    margin-bottom: 1.25rem;
  }

  .field label {
    font-size: 0.85rem;
    font-weight: 500;
  }

  .field-hint {
    font-size: 0.78rem;
  }

  .form-actions {
    display: flex;
    gap: 0.75rem;
  }

  .verify-card {
    margin-top: 1rem;
  }

  .verify-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.75rem;
  }

  .verify-card h2 {
    font-size: 1.1rem;
  }

  .withdraw-hint {
    font-size: 0.78rem;
    margin-top: 0.5rem;
  }

  .verify-code {
    display: block;
    padding: 0.75rem 1rem;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    font-size: 0.82rem;
    margin: 0.5rem 0;
    word-break: break-all;
    font-family: monospace;
  }
</style>
