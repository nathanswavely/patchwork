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
  import { selectWhen } from '../lib/selectText.js';
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

  // The value the clipboard last refused. Its row then shows the whole
  // address, unclipped and selected, to copy by hand (F-031). A fresh
  // object per failure so a second refusal selects it again.
  let copyFailed = $state(null);
  let failedFor = (value) => (copyFailed?.value === value ? copyFailed : null);

  async function copyValue(value) {
    try {
      await navigator.clipboard.writeText(value);
      copyFailed = null;
      showToast('Copied');
    } catch {
      copyFailed = { value };
      showToast('Copy failed. The address is selected for you to copy.', 'error');
    }
  }
</script>

<div class="feed-row">
  <span class="feed-label">Calendar (ICS)</span>
  <code class="feed-url" class:whole={failedFor(icsUrl)} use:selectWhen={failedFor(icsUrl)}>{icsUrl}</code>
  <button class="lt-btn lt-btn-secondary lt-btn-sm" onclick={() => copyValue(icsUrl)}>Copy</button>
</div>
<div class="feed-row">
  <span class="feed-label">RSS</span>
  <code class="feed-url" class:whole={failedFor(rssUrl)} use:selectWhen={failedFor(rssUrl)}>{rssUrl}</code>
  <button class="lt-btn lt-btn-secondary lt-btn-sm" onclick={() => copyValue(rssUrl)}>Copy</button>
</div>
{#if handleAvailable}
  <div class="feed-row">
    <span class="feed-label">Mastodon</span>
    <code class="feed-url" class:whole={failedFor(handle)} use:selectWhen={failedFor(handle)}>{handle}</code>
    <button class="lt-btn lt-btn-secondary lt-btn-sm" onclick={() => copyValue(handle)}>Copy</button>
  </div>
{/if}
<p class="lt-muted feed-hint">Paste the calendar address into your calendar app to follow this patch's events.</p>
{#if handleAvailable}
  <p class="lt-muted feed-hint">
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

  /* After a refused copy: the whole value, wrapped, for selecting by hand. */
  .feed-url.whole {
    white-space: normal;
    overflow-wrap: anywhere;
    user-select: all;
    color: var(--color-text);
  }

  .feed-hint {
    font-size: 0.8rem;
    margin: 0.5rem 0 0;
  }
</style>
