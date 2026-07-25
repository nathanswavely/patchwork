<script>
  import { Bell } from 'phosphor-svelte';
  import { getUnread, refreshUnread } from '../stores/notifications.svelte.js';

  let { onOpen = () => {} } = $props();

  let intervalId = $state(null);

  // The poll reconciles with the server (notifications can arrive while
  // you sit on a page). Reading one drops the badge immediately instead,
  // via the store — see notifications.svelte.js.
  $effect(() => {
    refreshUnread();
    intervalId = setInterval(refreshUnread, 60000);
    return () => {
      if (intervalId) clearInterval(intervalId);
    };
  });
</script>

<div class="notif-container">
  <button class="bell-btn" onclick={onOpen} aria-label="Notifications">
    <Bell size={18} weight="duotone" />
    {#if getUnread() > 0}
      <span class="badge-count">{getUnread() > 99 ? '99+' : getUnread()}</span>
    {/if}
  </button>
</div>

<style>
  .notif-container {
    position: relative;
  }

  .bell-btn {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.4rem;
    border: none;
    background: none;
    color: var(--color-text-muted);
    cursor: pointer;
    border-radius: var(--radius);
  }

  .bell-btn:hover {
    color: var(--color-text);
  }

  .badge-count {
    position: absolute;
    top: -2px;
    right: -4px;
    background: var(--color-error);
    color: var(--color-on-error);
    font-size: 0.6rem;
    font-weight: 700;
    min-width: 16px;
    height: 16px;
    line-height: 16px;
    text-align: center;
    border-radius: 8px;
    padding: 0 3px;
  }
</style>
