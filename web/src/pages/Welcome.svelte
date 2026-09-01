<script>
  import { navigate } from '../stores/router.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';
  import { getInstanceName } from '../stores/quilt.svelte.js';
  import { dismissOnboarding } from '../lib/onboarding.js';
  import { getLabel, loadLabel, formatMoney } from '../stores/label.svelte.js';

  let user = $derived(getUser());
  let instanceName = $derived(getInstanceName());

  // The Label's onboarding panel (docs/adr/023): a compact summary beside
  // the agreement — the "what are you joining" beat. A panel, not a step:
  // a step people click past teaches them the Label is a formality. It
  // simply doesn't render when no Label is published.
  let label = $derived(getLabel());
  $effect(() => { loadLabel(); });
  let labelProseLine = $derived.by(() => {
    const prose = label?.prose || '';
    const line = prose.split('\n').map((l) => l.trim()).find((l) => l && !l.startsWith('#')) || '';
    return line.length > 140 ? line.slice(0, 140).trimEnd() + '…' : line;
  });
  let labelStewardLine = $derived.by(() => {
    const s = label?.stewards || [];
    if (s.length === 0) return '';
    const names = s.slice(0, 3).map((x) => x.display_name || `@${x.username}`);
    return s.length > 3 ? `${names.join(', ')} +${s.length - 3}` : names.join(names.length > 2 ? ', ' : ' and ');
  });

  // Steps 2 and 3 — the interests question and the patches it answered with
  // — moved to discovery mode (docs/adr/075). They were never orientation:
  // they had nothing to do with the agreement this screen restates, and
  // being spent after one showing was the defect. What stays here is the
  // post-signup beat docs/adr/040 gated, shown to someone who signed one
  // screen ago.

  // Handing off dismisses onboarding, and must: the zero-membership redirect
  // in App.svelte would otherwise pull the person straight back here from
  // the very surface this screen is sending them to.
  function handleContinue() {
    dismissOnboarding(user?.id);
    navigate('/discover');
  }

  // Skip must genuinely exit, for the same reason — without the persisted
  // dismissal the redirect softlocks on an instance with nothing to follow.
  function handleSkip() {
    dismissOnboarding(user?.id);
    navigate('/');
  }
</script>

<div class="welcome">
  <div class="step-content">
    <div class="step step-welcome">
      <h1>Welcome to {instanceName}</h1>

      <div class="explainer">
        <p>Each patch here is a group. Follow the ones you care about and your corner of the quilt takes shape.</p>
        <p><a href="/about" target="_blank" rel="noopener">What is Patchwork? &rarr;</a></p>
      </div>

      {#if label?.published}
        <div class="label-panel">
          <p class="label-panel-who">
            This quilt is stewarded by <strong>{labelStewardLine}</strong>{#if label.total_monthly_minor > 0}&nbsp;and costs
            about <strong>{formatMoney(label.total_monthly_minor, label.currency)}/month</strong> to run{/if}.
          </p>
          {#if labelProseLine}
            <p class="label-panel-quote">&ldquo;{labelProseLine}&rdquo;</p>
          {/if}
          <a href="/label" target="_blank" rel="noopener" class="label-panel-link">
            Read the Label &rarr;
          </a>
        </div>
      {/if}

      <div class="agreement">
        <h2>What's expected here</h2>
        <p>The heart of the <a href="/terms" target="_blank" rel="noopener">agreement</a> you accepted when you created your account:</p>
        <ul>
          <li>Treat every person with dignity and respect</li>
          <li>Participate in good faith</li>
          <li>Support the communities you join</li>
          <li>Report harmful behavior instead of ignoring it</li>
        </ul>
        <p class="agreement-lining-note">Every patch starts from <a href="/lining" target="_blank" rel="noopener">the lining</a>, the shared baseline behind these.</p>
      </div>

      <button class="btn btn-primary cta-btn" onclick={handleContinue}>
        Build your quilt &rarr;
      </button>
      <!-- The way out, offered at the start rather than only at the end:
           someone who wants to look around before following anything
           shouldn't have to walk further in to be let go. -->
      <button class="skip-link" onclick={handleSkip}>
        I'll explore on my own
      </button>
    </div>
  </div>
</div>

<style>
  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  /* Welcome no longer owns the full viewport (docs/adr/040): it renders
     inside ThresholdShell, whose bar already claims the top 56px. Sizing
     to the full 100vh here on top of that would push the page 56px taller
     than the viewport and force a scrollbar for no reason. */
  .welcome {
    min-height: calc(100vh - 56px);
    min-height: calc(100dvh - 56px);
    background: var(--color-bg);
    display: flex;
    flex-direction: column;
  }

  .step-content {
    flex: 1;
    display: flex;
    flex-direction: column;
  }

  .step {
    flex: 1;
    padding: 3rem 2rem 2rem;
    max-width: var(--pw-measure-narrow);
    width: 100%;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
  }

  /* Step 1: Welcome */
  .step-welcome h1 {
    font-size: clamp(1.8rem, 5vw, 2.4rem);
    font-weight: 700;
    line-height: 1.2;
    margin-bottom: 1.5rem;
    color: var(--color-text);
  }

  .explainer {
    margin-bottom: 2rem;
  }

  .explainer p {
    font-size: 0.95rem;
    line-height: 1.6;
    color: var(--color-text-muted);
    margin-bottom: 0.5rem;
  }

  /* The Label panel (docs/adr/023): the gist next to the agreement;
     the page holds the detail. */
  .label-panel {
    margin: -1rem 0 2rem;
    padding: 1rem 1.25rem;
    border: 1px dashed var(--color-border);
    border-radius: 8px;
    font-size: 0.9rem;
  }
  .label-panel-who {
    margin: 0 0 0.4rem;
  }
  .label-panel-quote {
    margin: 0 0 0.4rem;
    font-style: italic;
    opacity: 0.85;
  }
  .label-panel-link {
    font-weight: 600;
    color: var(--color-primary);
    text-decoration: none;
  }

  .agreement {
    margin-bottom: 2rem;
  }

  .agreement h2 {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 0.5rem;
    color: var(--color-text);
  }

  .agreement p {
    font-size: 0.88rem;
    color: var(--color-text-muted);
    margin-bottom: 0.5rem;
  }

  .agreement ul {
    list-style: none;
    padding: 0;
    margin: 0 0 1rem;
  }

  .agreement li {
    font-size: 0.88rem;
    color: var(--color-text);
    padding: 0.35rem 0;
    padding-left: 1.2rem;
    position: relative;
  }

  .agreement li::before {
    content: '•';
    position: absolute;
    left: 0;
    color: var(--color-primary);
    font-weight: 700;
  }

  .agreement-lining-note {
    font-size: 0.82rem;
    color: var(--color-text-muted);
  }

  .cta-btn {
    width: 100%;
    padding: 0.75rem;
    font-size: 0.95rem;
    margin-top: auto;
  }

  .skip-link {
    border: none;
    background: none;
    font-size: 0.8rem;
    color: var(--color-text-muted);
    cursor: pointer;
    padding: 0.75rem 0;
    text-align: center;
    width: 100%;
  }

  .skip-link:hover {
    color: var(--color-primary);
  }

  @media (min-width: 640px) {
    .step {
      padding: 4rem 2rem 3rem;
    }
  }
</style>
