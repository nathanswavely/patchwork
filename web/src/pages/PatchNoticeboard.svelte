<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate, getPath } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { formatRelative } from '../lib/datetime.js';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';

  // The noticeboard (docs/adr/081): the room's, and only the room's. The
  // server refuses everyone else; this page just says so kindly.
  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let membershipRole = $derived(patch.value.membershipRole);
  let inRoom = $derived(membershipRole === 'member' || membershipRole === 'admin');
  let composing = $derived(getPath().endsWith('/noticeboard/new'));

  let notices = $state([]);
  let nextCursor = $state('');
  let mayPost = $state(false);
  let noticePosting = $state('members');
  let repliesDefault = $state(true);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    if (slug && inRoom) loadNotices();
    else loading = false;
  });

  async function loadNotices(after = '') {
    if (!after) loading = true;
    error = '';
    try {
      const q = after ? `?after=${encodeURIComponent(after)}` : '';
      const data = await api(`nodes/${slug}/notices${q}`);
      notices = after ? [...notices, ...(data.items || [])] : (data.items || []);
      nextCursor = data.next_cursor || '';
      mayPost = !!data.may_post;
      noticePosting = data.notice_posting || 'members';
      repliesDefault = data.replies_default !== false;
    } catch (e) {
      error = e.message || 'Failed to load the noticeboard';
    } finally {
      loading = false;
    }
  }

  // Composer. A notice is born quiet: "Tell members" is off unless the
  // author reaches for it, and it is the only way a notice rings the bell.
  let title = $state('');
  let body = $state('');
  let imageUrl = $state('');
  let imageAlt = $state('');
  let repliesOpen = $state(true);
  let tellMembers = $state(false);
  let previewMode = $state(false);
  let submitting = $state(false);

  $effect(() => {
    if (composing) repliesOpen = repliesDefault;
  });

  function openComposer() {
    title = ''; body = ''; imageUrl = ''; imageAlt = '';
    tellMembers = false; previewMode = false;
    navigate(`/patches/${slug}/noticeboard/new`);
  }

  async function putUp() {
    if (!title.trim()) { showToast('A notice needs a title', 'error'); return; }
    submitting = true;
    try {
      const n = await api(`nodes/${slug}/notices`, {
        method: 'POST',
        body: {
          title: title.trim(),
          body: body.trim(),
          image_url: imageUrl.trim(),
          image_alt: imageAlt.trim(),
          replies_open: repliesOpen,
          tell_members: tellMembers,
        },
      });
      showToast(tellMembers ? 'Notice put up and members told' : 'Notice put up', 'success');
      navigate(`/patches/${slug}/noticeboard/${n.id}`);
    } catch (e) {
      showToast(e.message || 'Could not put up the notice', 'error');
    } finally {
      submitting = false;
    }
  }
</script>

{#if !inRoom}
  <div class="permission-notice">
    <p>The noticeboard is for this patch's members.</p>
    <p class="muted">Become a member to read and put up notices.</p>
  </div>
{:else if composing}
  <div class="noticeboard-page">
    <h2 class="page-title">Put up a notice</h2>
    <form class="notice-form" onsubmit={(e) => { e.preventDefault(); putUp(); }}>
      <div class="field">
        <label for="notice-title">Title</label>
        <input id="notice-title" type="text" bind:value={title} maxlength="140" disabled={submitting} placeholder="Notice title" />
      </div>

      <div class="field">
        <label for="notice-body">
          Notice
          <div class="toggle-group">
            <button type="button" class="toggle-btn" class:active={!previewMode} onclick={() => previewMode = false}>Write</button>
            <button type="button" class="toggle-btn" class:active={previewMode} onclick={() => previewMode = true}>Preview</button>
          </div>
        </label>
        {#if previewMode}
          <div class="preview-pane">
            {#if body.trim()}
              <MarkdownRenderer content={body} />
            {:else}
              <span class="muted">Nothing to preview</span>
            {/if}
          </div>
        {:else}
          <textarea id="notice-body" bind:value={body} rows="8" disabled={submitting}
            placeholder="Markdown is supported."></textarea>
        {/if}
      </div>

      <div class="field">
        <label for="notice-image">Image address</label>
        <input id="notice-image" type="url" bind:value={imageUrl} disabled={submitting} placeholder="https://..." />
        <p class="hint muted">Link a flyer or photo you already have online. Patchwork points at it and never keeps a copy.</p>
      </div>
      {#if imageUrl.trim()}
        <div class="field">
          <label for="notice-image-alt">Describe the image</label>
          <input id="notice-image-alt" type="text" bind:value={imageAlt} disabled={submitting} placeholder="Alt text for the image" />
        </div>
      {/if}

      <label class="check-row">
        <input type="checkbox" bind:checked={repliesOpen} disabled={submitting} />
        <span>
          <strong>Take replies</strong>
          <span class="muted">You or an admin can switch this off later. Existing replies stay.</span>
        </span>
      </label>

      <label class="check-row">
        <input type="checkbox" bind:checked={tellMembers} disabled={submitting} />
        <span>
          <strong>Tell members</strong>
          <span class="muted">Rings the bell for every member of this patch. Off, the notice waits here for people to walk in.</span>
        </span>
      </label>

      <div class="form-actions">
        <button type="submit" class="btn btn-primary" disabled={submitting}>{submitting ? 'Putting up…' : 'Put up notice'}</button>
        <button type="button" class="btn btn-secondary" onclick={() => navigate(`/patches/${slug}/noticeboard`)} disabled={submitting}>Cancel</button>
      </div>
    </form>
  </div>
{:else}
  <div class="noticeboard-page">
    <div class="board-header">
      <p class="muted board-hint">
        Read by this patch's admins and members, and nobody else.
        {#if noticePosting === 'admins'}
          Its admins put up the notices.
        {/if}
      </p>
      {#if mayPost}
        <button class="btn btn-primary btn-sm" onclick={openComposer}>Put up a notice</button>
      {/if}
    </div>

    {#if loading}
      <p class="muted">Loading the noticeboard...</p>
    {:else if error}
      <p class="error-text">{error}</p>
    {:else if notices.length === 0}
      <div class="empty-state">
        <p>Nothing on the board yet.</p>
        {#if mayPost}
          <p class="muted">Put up the first notice.</p>
        {/if}
      </div>
    {:else}
      <ul class="notice-list">
        {#each notices as n (n.id)}
          <li class="notice-row">
            <a href="/patches/{slug}/noticeboard/{n.id}" class="notice-title"
              onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/noticeboard/${n.id}`); }}>
              {n.title}
            </a>
            <div class="notice-meta muted">
              <span>{n.author_display_name || n.author_username}</span>
              <span>{formatRelative(n.created_at)}</span>
              {#if n.members_told}<span class="mark">members told</span>{/if}
              {#if n.replies_open}
                <span>{n.reply_count === 1 ? '1 reply' : `${n.reply_count} replies`}</span>
              {:else}
                <span class="mark">replies off{#if n.reply_count > 0}{' · '}{n.reply_count} kept{/if}</span>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
      {#if nextCursor}
        <button class="btn btn-secondary btn-sm" onclick={() => loadNotices(nextCursor)}>Older notices</button>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .permission-notice { text-align: center; padding: 3rem 1rem; }
  .permission-notice p:first-child { font-weight: 500; margin-bottom: 0.25rem; }
  .empty-state { text-align: center; padding: 2rem 0; }
  .empty-state p:first-child { font-weight: 500; margin-bottom: 0.25rem; }

  .board-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
    margin-bottom: 1rem;
  }
  .board-hint { font-size: 0.85rem; margin: 0; }
  .page-title { font-size: 1.2rem; margin-bottom: 1rem; }

  .notice-list { list-style: none; padding: 0; margin: 0 0 1rem; }
  .notice-row {
    padding: 0.7rem 0;
    border-bottom: 1px solid var(--color-border);
  }
  .notice-row:last-child { border-bottom: none; }
  .notice-title {
    font-weight: 600;
    color: var(--color-text);
    text-decoration: none;
    font-size: 0.95rem;
  }
  .notice-title:hover { color: var(--color-primary); }
  .notice-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0.75rem;
    font-size: 0.78rem;
    margin-top: 0.2rem;
  }
  .mark {
    border: 1px solid var(--color-border);
    border-radius: 999px;
    padding: 0 0.45rem;
  }

  .notice-form { display: flex; flex-direction: column; gap: 1rem; max-width: 40rem; }
  .field { display: flex; flex-direction: column; gap: 0.25rem; }
  .field > label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .field input, .field textarea {
    padding: 0.5rem 0.65rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    font: inherit;
  }
  .hint { font-size: 0.78rem; margin: 0; }
  .toggle-group { display: inline-flex; gap: 0.25rem; }
  .toggle-btn {
    border: 1px solid var(--color-border);
    background: transparent;
    color: var(--color-text-muted);
    font-size: 0.75rem;
    padding: 0.15rem 0.5rem;
    border-radius: var(--radius);
    cursor: pointer;
  }
  .toggle-btn.active { color: var(--color-text); border-color: var(--color-primary); }
  .preview-pane {
    min-height: 8rem;
    padding: 0.65rem;
    border: 1px dashed var(--color-border);
    border-radius: var(--radius);
  }
  .check-row {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
    font-size: 0.9rem;
    cursor: pointer;
  }
  .check-row input { margin-top: 0.25rem; }
  .check-row span > .muted { display: block; font-size: 0.8rem; }
  .form-actions { display: flex; gap: 0.5rem; }
</style>
