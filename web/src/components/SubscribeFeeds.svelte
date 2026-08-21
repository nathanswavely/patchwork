<script>
  /**
   * The three ways to subscribe to a patch's events, shared by the patch
   * profile's overflow modal and the workspace Events tab so the two can't
   * drift apart.
   *
   * The Mastodon handle (docs/adr/059) is the third one. Every patch has
   * had an actor since the AP layer shipped — `NodeToActor` sets
   * preferredUsername to the slug — and no surface had ever said so.
   *
   * Callers gate this block on the patch being public (docs/adr/031: feeds
   * exist only for public patches, which is also the line federation
   * draws). The handle row additionally needs the quilt to federate: with
   * federation off the /ap/* routes are not mounted, so the address would
   * resolve to nothing.
   */
  import { showToast } from '../stores/toast.svelte.js';
  import {
    getInstanceDomain,
    getInstanceFederation,
    getInstanceName,
  } from '../stores/quilt.svelte.js';

  let { slug = '' } = $props();

  let icsUrl = $derived(`${location.origin}/api/v1/nodes/${slug}/events.ics`);
  let rssUrl = $derived(`${location.origin}/api/v1/nodes/${slug}/events.rss`);

  let handleAvailable = $derived(getInstanceFederation() && !!getInstanceDomain());
  let handle = $derived(`@${slug}@${getInstanceDomain()}`);

  async function copyValue(value) {
    try {
      await navigator.clipboard.writeText(value);
      showToast('Copied');
    } catch {
      showToast('Copy failed. Select the address instead.', 'error');
    }
  }
</script>

<div class="feed-row">
  <span class="feed-label">Calendar (ICS)</span>
  <code class="feed-url">{icsUrl}</code>
  <button class="btn btn-secondary btn-sm" onclick={() => copyValue(icsUrl)}>Copy</button>
</div>
<div class="feed-row">
  <span class="feed-label">RSS</span>
  <code class="feed-url">{rssUrl}</code>
  <button class="btn btn-secondary btn-sm" onclick={() => copyValue(rssUrl)}>Copy</button>
</div>
{#if handleAvailable}
  <div class="feed-row">
    <span class="feed-label">Mastodon</span>
    <code class="feed-url">{handle}</code>
    <button class="btn btn-secondary btn-sm" onclick={() => copyValue(handle)}>Copy</button>
  </div>
{/if}
<p class="muted feed-hint">Paste the calendar address into your calendar app to follow this patch's events.</p>
{#if handleAvailable}
  <p class="muted feed-hint">
    Search the handle in Mastodon to follow new events there. Replies don't
    reach the patch. The address belongs to {getInstanceName()}, so it
    doesn't travel if this community moves to another quilt.
  </p>
{/if}

<style>
  .feed-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
    padding: 0.4rem 0;
  }

  .feed-label {
    font-size: 0.8rem;
    font-weight: 600;
    min-width: 7rem;
    flex-shrink: 0;
    /* The overflow's Subscribe modal centers its text. Without this the
       three values align differently from each other: the long feed URLs
       overflow and appear left-aligned, while the short handle centers. */
    text-align: left;
  }

  .feed-url {
    flex: 1;
    min-width: 0;
    text-align: left;
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-muted);
  }

  .feed-hint {
    font-size: 0.8rem;
    margin: 0.5rem 0 0;
  }
</style>
