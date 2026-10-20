<script>
  import { api } from '../lib/api.js';
  import { formatDay } from '../lib/datetime.js';

  let { proposalId = '' } = $props();

  let revisions = $state([]);
  let loading = $state(true);

  $effect(() => {
    if (proposalId) {
      loadRevisions();
    }
  });

  async function loadRevisions() {
    loading = true;
    try {
      const res = await api(`proposals/${proposalId}/revisions`);
      revisions = res.items || [];
    } catch {
      revisions = [];
    } finally {
      loading = false;
    }
  }

  function timeAgo(dateStr) {
    if (!dateStr) return '';
    const now = Date.now();
    const then = new Date(dateStr).getTime();
    const diff = now - then;
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days}d ago`;
    return formatDay(dateStr);
  }
</script>

<!-- A tab has to say something in every state. This rendered literally
     nothing for a proposal nobody had edited: no heading, no sentence, an
     empty panel a reader cannot tell from a page that failed to load.

     The threshold is > 0, not > 1. A row in proposal_revisions is written
     per edit (handler.CreateRevision snapshots the pre-edit text; nothing
     seeds revision 1 at creation), so `> 1` hid a proposal's only edit as
     well, and the unedited case fell through to the same nothing. -->
{#if !loading && revisions.length > 0}
  <div class="revision-history">
    <h3>Revision History</h3>
    <div class="timeline">
      {#each revisions as rev}
        <div class="timeline-entry">
          <div class="timeline-dot"></div>
          <div class="timeline-content">
            <span class="revision-label">Revision {rev.revision_number}</span>
            {#if rev.change_note}
              <span class="revision-note"> &mdash; {rev.change_note}</span>
            {/if}
            <span class="revision-meta">by {rev.author_name} &middot; {timeAgo(rev.created_at)}</span>
          </div>
        </div>
      {/each}
    </div>
  </div>
{:else if !loading}
  <div class="revision-history">
    <h3>Revision History</h3>
    <p class="lt-muted">No edits yet. Any change to this proposal is recorded here.</p>
  </div>
{/if}

<style>
  .revision-history {
    margin-top: 1.5rem;
    padding-top: 1rem;
    border-top: 1px solid var(--color-border);
  }

  .revision-history h3 {
    font-size: 0.9rem;
    margin-bottom: 0.75rem;
  }

  .timeline {
    display: flex;
    flex-direction: column;
    gap: 0;
    padding-left: 0.75rem;
  }

  .timeline-entry {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    position: relative;
    padding-bottom: 0.75rem;
  }

  .timeline-entry:not(:last-child)::before {
    content: '';
    position: absolute;
    left: 4px;
    top: 12px;
    bottom: 0;
    width: 1px;
    background: var(--color-border);
  }

  .timeline-dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--color-primary);
    flex-shrink: 0;
    margin-top: 4px;
  }

  .timeline-content {
    font-size: 0.85rem;
    line-height: 1.4;
  }

  .revision-label {
    font-weight: 600;
    color: var(--color-text);
  }

  .revision-note {
    color: var(--color-text);
  }

  .revision-meta {
    display: block;
    font-size: 0.78rem;
    color: var(--color-text-muted);
    margin-top: 0.1rem;
  }
</style>
