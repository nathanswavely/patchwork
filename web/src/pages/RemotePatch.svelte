<script>
  /**
   * A patch on another quilt, rendered read-only in-app from its public
   * API (docs/adr/024). Looking happens here; belonging happens on their
   * soil — everything deeper than public data is a doorway out. The one
   * action is Follow, which posts to the person's HOME instance.
   */
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import {
    findRemoteFollow, followRemotePatch, unfollowRemotePatch, fetchQuiltInfo, colorForQuilt,
  } from '../stores/multiQuilt.svelte.js';
  import { textOnColor } from '../lib/quiltTheme.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { ArrowSquareOut, Heart, CalendarBlank, Users } from 'phosphor-svelte';

  let { host = '', slug = '' } = $props();

  // Loopback hosts run plain http in dev; everything real is https.
  const origin = $derived(
    /^(localhost|127\.|\[::1\])/.test(host) ? `http://${host}` : `https://${host}`
  );

  let node = $state(null);
  let events = $state([]);
  let quiltInfo = $state(null);
  let loading = $state(true);
  let error = $state('');
  let busy = $state(false);

  let sashColor = $derived(colorForQuilt(origin, quiltInfo));
  let follow = $derived(node ? findRemoteFollow(origin, slug) : null);

  async function load() {
    loading = true;
    error = '';
    try {
      const [nodeRes, eventsRes, info] = await Promise.all([
        fetch(`${origin}/api/v1/nodes/${encodeURIComponent(slug)}`),
        fetch(`${origin}/api/v1/events?node_slug=${encodeURIComponent(slug)}&limit=5`).catch(() => null),
        fetchQuiltInfo(origin),
      ]);
      if (!nodeRes.ok) throw new Error(`HTTP ${nodeRes.status}`);
      const data = await nodeRes.json();
      node = data.node || data;
      quiltInfo = info;
      if (eventsRes?.ok) {
        const ed = await eventsRes.json();
        events = ed.items || ed || [];
      }
    } catch {
      error = "Couldn't reach this patch. Its quilt may be unreachable or may not allow browsing from other quilts.";
    } finally {
      loading = false;
    }
  }

  async function toggleFollow() {
    if (busy || !node) return;
    busy = true;
    try {
      if (follow) {
        await unfollowRemotePatch(follow.id);
        showToast('Unfollowed');
      } else {
        await followRemotePatch({ quiltUrl: origin, node });
        showToast(`Following ${node.name}`);
      }
    } catch (e) {
      showToast(e.data?.error || 'Something went wrong', 'error');
    } finally {
      busy = false;
    }
  }

  function fmtDate(iso) {
    try {
      return new Date(iso).toLocaleDateString(undefined, {
        weekday: 'short', month: 'short', day: 'numeric',
      });
    } catch {
      return iso;
    }
  }

  $effect(() => {
    void host; void slug;
    load();
  });
</script>

<div class="page-container remote-patch" style="--sash-color: {sashColor}">
  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <div class="remote-error">
      <p>{error}</p>
      <a class="btn btn-secondary" href="{origin}/patches/{slug}" target="_blank" rel="noopener">
        Try their site <ArrowSquareOut size={14} />
      </a>
    </div>
  {:else if node}
    <div class="remote-frame">
      <div class="remote-sash">
        <img src="{origin}/api/v1/instance/icon" alt="" width="18" height="18" />
        <span style="color: {textOnColor(sashColor)}">
          On {quiltInfo?.name || host}, another quilt
        </span>
      </div>

      <header class="remote-head">
        <h1>{node.name}</h1>
        {#if node.description}
          <p class="remote-desc">{node.description}</p>
        {/if}
        <div class="remote-meta">
          <span><Users size={15} /> {node.member_count || 0} members</span>
          <span><CalendarBlank size={15} /> {node.event_count || events.length || 0} events</span>
          {#each node.tags || [] as tag (tag)}
            <span class="remote-tag">{tag}</span>
          {/each}
        </div>
        <div class="remote-actions">
          {#if isLoggedIn()}
            <button class="btn {follow ? 'btn-secondary' : 'btn-primary'}" onclick={toggleFollow} disabled={busy}>
              <Heart size={15} weight={follow ? 'fill' : 'regular'} />
              {follow ? 'Following' : 'Follow'}
            </button>
          {/if}
          <a class="btn btn-secondary" href="{origin}/patches/{slug}" target="_blank" rel="noopener">
            Visit on {quiltInfo?.name || host} <ArrowSquareOut size={14} />
          </a>
        </div>
      </header>

      {#if events.length > 0}
        <section class="remote-events">
          <h2>Upcoming events</h2>
          <ul>
            {#each events as ev (ev.id)}
              <li>
                <span class="remote-event-date">{fmtDate(ev.starts_at)}</span>
                <span class="remote-event-title">{ev.title}</span>
                {#if ev.location}<span class="muted">· {ev.location}</span>{/if}
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      <p class="remote-note muted">
        You're viewing this patch's public face from your own quilt. Joining,
        RSVPs, and everything deeper live on
        <a href={origin} target="_blank" rel="noopener">{quiltInfo?.name || host}</a>.
      </p>
    </div>
  {/if}
</div>

<style>
  .remote-frame {
    border: 4px solid var(--sash-color);
    border-radius: 12px;
    overflow: hidden;
    background: var(--color-surface);
  }

  .remote-sash {
    display: flex;
    align-items: center;
    gap: 8px;
    background: var(--sash-color);
    padding: 6px 14px;
    font-size: 0.8rem;
    font-weight: 700;
  }

  .remote-sash img {
    border-radius: 3px;
  }

  .remote-head {
    padding: 1.2rem 1.4rem 0.8rem;
  }

  .remote-head h1 {
    margin: 0 0 0.4rem;
    font-family: var(--font-display);
  }

  .remote-desc {
    color: var(--color-text-muted);
    margin: 0 0 0.8rem;
    line-height: 1.5;
  }

  .remote-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.9rem;
    align-items: center;
    font-size: 0.85rem;
    color: var(--color-text-muted);
    margin-bottom: 1rem;
  }

  .remote-meta span {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .remote-tag {
    border: 1px solid var(--color-border);
    border-radius: 999px;
    padding: 1px 9px;
    font-size: 0.75rem;
  }

  .remote-actions {
    display: flex;
    gap: 0.6rem;
    flex-wrap: wrap;
    margin-bottom: 0.6rem;
  }

  .remote-actions .btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .remote-events {
    padding: 0 1.4rem 0.6rem;
  }

  .remote-events h2 {
    font-size: 0.95rem;
    margin: 0.4rem 0 0.5rem;
  }

  .remote-events ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .remote-event-date {
    font-weight: 700;
    font-size: 0.8rem;
    margin-right: 0.5rem;
  }

  .remote-note {
    padding: 0.6rem 1.4rem 1.1rem;
    font-size: 0.8rem;
    margin: 0;
  }

  .remote-error {
    text-align: center;
    padding: 3rem 1rem;
    color: var(--color-text-muted);
  }

  .remote-error .btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-top: 0.8rem;
  }
</style>
