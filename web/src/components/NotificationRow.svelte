<script>
  /**
   * One notification, as the bell's dropdown and the notifications page both
   * show it: who sent it, the title, the body, and when, with a dot for
   * unread. lib/notificationRow.js decides the sender line and the time.
   *
   * clamp: the dropdown holds a body to two lines; the page shows it whole,
   * because some bodies are the only account of what happened (docs/adr/097's
   * lapse sentence).
   * onDismiss: the page's ×. The dropdown has none.
   */
  import { X } from 'phosphor-svelte';
  import { describeNotification, notificationTime } from '../lib/notificationRow.js';

  let { notif, onOpen, onDismiss = null, clamp = false } = $props();

  let parts = $derived(describeNotification(notif));
  let unread = $derived(!notif.read_at);
</script>

<div class="notif-item" class:unread>
  <button class="notif-main" onclick={() => onOpen(notif)}>
    <span class="notif-dot" aria-hidden="true"></span>
    <span class="notif-text">
      {#if unread}<span class="lt-sr-only">Unread: </span>{/if}
      {#if parts.context}<span class="notif-context">{parts.context}</span>{/if}
      <span class="notif-title">{parts.title}</span>
      {#if parts.body}<span class="notif-body" class:clamp>{parts.body}</span>{/if}
      <time class="notif-time" datetime={notif.created_at}>{notificationTime(notif.created_at)}</time>
    </span>
  </button>
  {#if onDismiss}
    <button class="notif-dismiss" onclick={() => onDismiss(notif)} aria-label="Dismiss notification" title="Dismiss">
      <X size={14} />
    </button>
  {/if}
</div>

<style>
  .notif-item {
    display: flex;
    align-items: flex-start;
    border-bottom: 1px solid var(--color-border);
  }

  .notif-item:last-child {
    border-bottom: none;
  }

  .notif-item:hover,
  .notif-item:focus-within {
    background: var(--color-overlay);
  }

  .notif-main {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px 16px 12px 12px;
    border: none;
    background: none;
    cursor: pointer;
    text-align: left;
    color: inherit;
    font: inherit;
  }

  /* Unread is a dot and a heavier title, and nothing else. A tinted row
     for every unread one turned a busy list into a blue sheet. */
  .notif-dot {
    flex-shrink: 0;
    width: 7px;
    height: 7px;
    margin-top: 6px;
    border-radius: 50%;
  }

  .unread .notif-dot {
    background: var(--color-primary);
  }

  .notif-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .notif-context {
    font-size: 0.75rem;
    color: var(--color-text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .notif-title {
    font-size: 0.88rem;
    color: var(--color-text);
    overflow-wrap: anywhere;
  }

  .unread .notif-title {
    font-weight: 600;
  }

  /* overflow-wrap keeps a long URL or handle from widening the row. */
  .notif-body {
    font-size: 0.82rem;
    line-height: 1.45;
    color: var(--color-text-muted);
    overflow-wrap: anywhere;
  }

  .notif-body.clamp {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .notif-time {
    margin-top: 2px;
    font-size: 0.72rem;
    color: var(--color-text-muted);
  }

  .notif-dismiss {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: var(--lt-target);
    min-height: var(--lt-target);
    margin: 4px 4px 0 0;
    border: none;
    border-radius: var(--radius);
    background: none;
    color: var(--color-text-muted);
    cursor: pointer;
    opacity: 0;
  }

  .notif-item:hover .notif-dismiss,
  .notif-dismiss:focus-visible {
    opacity: 1;
  }

  .notif-dismiss:hover {
    color: var(--color-text);
    background: var(--color-overlay);
  }

  /* No hover on a touch screen, so the × is always there. */
  @media (hover: none) {
    .notif-dismiss {
      opacity: 1;
    }
  }
</style>
