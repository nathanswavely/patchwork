<script>
  import { getToasts } from '../stores/toast.svelte.js';

  let toasts = $derived(getToasts());
</script>

{#if toasts.length > 0}
  <div class="toast-container">
    {#each toasts as toast (toast.id)}
      <div class="toast toast-{toast.type}">
        {toast.message}
      </div>
    {/each}
  </div>
{/if}

<style>
  .toast-container {
    position: fixed;
    /* Above the mobile tab bar, which overlays the foot of the page.
       --pw-nav-h is 0px above that breakpoint, so this is still the plain
       1.5rem on desktop. */
    bottom: calc(var(--pw-nav-h) + 1.5rem);
    right: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    z-index: 1000;
    max-width: 360px;
  }

  /* One drawing, three colorways. The status is the whole slip — a tinted
     ground and a border of the same hue — rather than a colored bar down
     the left edge: in this codebase a left border means quoted or nested
     (MarkdownRenderer's blockquote, CommentThread's indent), so a toast
     wearing one was borrowing a mark that already meant something else.

     Text stays --color-text in every colorway. Coloring a whole sentence
     with --color-error reads as alarm even when the sentence is only
     telling you a patch is unclaimed; --color-error is for the short
     label (.error-text), not for prose. */
  .toast {
    --toast-hue: var(--color-text-muted);
    /* How much of the hue the ground and the edge take. Named rather than
       inlined because dark needs more of both: the same 9% that tints a
       cream surface visibly is invisible over #1c2028, and the dark
       surface is only a few steps off the dark background to begin with. */
    --toast-tint: 9%;
    --toast-edge: 40%;
    padding: 0.7rem 0.9rem;
    border-radius: var(--radius);
    font-size: 0.875rem;
    line-height: 1.4;
    color: var(--color-text);
    background: color-mix(in srgb, var(--toast-hue) var(--toast-tint), var(--color-surface));
    border: 1px solid color-mix(in srgb, var(--toast-hue) var(--toast-edge), var(--color-border));
    /* The same lift the other floating chrome uses (GlobalBar, SocialShell). */
    box-shadow: 0 4px 16px var(--color-shadow);
    animation: slideUp 200ms ease;
  }

  :global([data-theme='dark']) .toast {
    --toast-tint: 14%;
    --toast-edge: 55%;
  }

  .toast-success {
    --toast-hue: var(--color-success);
  }

  .toast-error {
    --toast-hue: var(--color-error);
  }

  .toast-info {
    --toast-hue: var(--color-primary);
  }

  @keyframes slideUp {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .toast {
      animation: none;
    }
  }

  @media (max-width: 640px) {
    .toast-container {
      left: 1rem;
      right: 1rem;
      max-width: none;
    }
  }
</style>
