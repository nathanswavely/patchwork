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
      <span class="lt-count lt-count-corner lt-tone-error badge-count">{getUnread() > 99 ? '99+' : getUnread()}</span>
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
    min-width: var(--lt-target);
    min-height: var(--lt-target);
  }

  .bell-btn:hover {
    color: var(--color-text);
  }

  .badge-count {
    top: -2px;
    right: -4px;
  }
</style>
