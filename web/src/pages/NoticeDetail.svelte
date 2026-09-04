<script>
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate, getParams } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';
  import { formatRelative } from '../lib/datetime.js';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import ReportButton from '../components/ReportButton.svelte';
  import ConfirmAction from '../components/ConfirmAction.svelte';

  // One notice and its replies (docs/adr/081). Replies are a flat list — no
  // reply to a reply, no reactions — and the reply box is there only while
  // the notice takes replies. Removal is a hard delete: the row goes.
  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isAdmin = $derived(patch.value.membershipRole === 'admin');
  let noticeId = $derived(getParams().id || '');
  let me = $derived(getUser());

  let notice = $state(null);
  let mayEdit = $state(false);
  let mayManage = $state(false);
  let replies = $state([]);
  let nextCursor = $state('');
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    if (noticeId) load();
  });

  $effect(() => {
    if (notice) patch.value.setBreadcrumbExtra?.([{ label: notice.title }]);
  });

  async function load() {
    loading = true;
    error = '';
    try {
      const data = await api(`notices/${noticeId}`);
      notice = data.notice;
      mayEdit = !!data.may_edit;
      mayManage = !!data.may_manage;
      await loadReplies();
    } catch (e) {
      error = e.message || 'Notice not found';
      notice = null;
    } finally {
      loading = false;
    }
  }

  async function loadReplies(after = '') {
    const q = after ? `?after=${encodeURIComponent(after)}` : '';
    const data = await api(`notices/${noticeId}/replies${q}`);
    replies = after ? [...replies, ...(data.items || [])] : (data.items || []);
    nextCursor = data.next_cursor || '';
  }

  // The reply switch (docs/adr/081, tool 1): the author's or an admin's,
  // flippable at any time. Off keeps the replies made and removes the box.
  async function setReplies(open) {
    try {
      notice = await api(`notices/${noticeId}`, { method: 'PATCH', body: { replies_open: open } });
      showToast(open ? 'Replies are on' : 'Replies are off. Existing replies stay.', 'info');
    } catch (e) {
      showToast(e.message || 'Could not change replies', 'error');
    }
  }

  async function takeDown() {
    try {
      await api(`notices/${noticeId}`, { method: 'DELETE' });
      showToast('Notice taken down', 'info');
      navigate(`/patches/${slug}/noticeboard`);
    } catch (e) {
      showToast(e.message || 'Could not take the notice down', 'error');
    }
  }

  // Editing: the author's alone.
  let editing = $state(false);
  let editTitle = $state('');
  let editBody = $state('');
  let editImageUrl = $state('');
  let editImageAlt = $state('');
  let saving = $state(false);

  function startEdit() {
    editTitle = notice.title;
    editBody = notice.body;
    editImageUrl = notice.image_url || '';
    editImageAlt = notice.image_alt || '';
    editing = true;
  }

  async function saveEdit() {
    saving = true;
    try {
      notice = await api(`notices/${noticeId}`, {
        method: 'PATCH',
        body: { title: editTitle.trim(), body: editBody.trim(), image_url: editImageUrl.trim(), image_alt: editImageAlt.trim() },
      });
      editing = false;
      showToast('Notice updated', 'success');
    } catch (e) {
      showToast(e.message || 'Could not save', 'error');
    } finally {
      saving = false;
    }
  }

  // Replies.
  let replyText = $state('');
  let replying = $state(false);
  let deletingId = $state(null);

  async function sendReply() {
    if (!replyText.trim()) return;
    replying = true;
    try {
      const r = await api(`notices/${noticeId}/replies`, { method: 'POST', body: { body: replyText.trim() } });
      replies = [...replies, r];
      replyText = '';
      if (notice) notice = { ...notice, reply_count: (notice.reply_count || 0) + 1 };
    } catch (e) {
      showToast(e.message || 'Could not reply', 'error');
    } finally {
      replying = false;
    }
  }

  // Two-step removal, as the proposal thread does it: the first click asks.
  async function removeReply(id) {
    if (deletingId !== id) { deletingId = id; return; }
    try {
      await api(`replies/${id}`, { method: 'DELETE' });
      replies = replies.filter((r) => r.id !== id);
      if (notice) notice = { ...notice, reply_count: Math.max(0, (notice.reply_count || 1) - 1) };
    } catch (e) {
      showToast(e.message || 'Could not remove the reply', 'error');
    } finally {
      deletingId = null;
    }
  }
</script>

<div class="notice-page">
  <a href="/patches/{slug}/noticeboard" class="back muted" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/noticeboard`); }}>← Noticeboard</a>

  {#if loading}
    <p class="muted">Loading...</p>
  {:else if error || !notice}
    <p class="error-text">{error || 'Notice not found'}</p>
  {:else if editing}
    <form class="edit-form" onsubmit={(e) => { e.preventDefault(); saveEdit(); }}>
      <div class="field">
        <label for="edit-title">Title</label>
        <input id="edit-title" type="text" bind:value={editTitle} maxlength="140" disabled={saving} />
      </div>
      <div class="field">
        <label for="edit-body">Notice</label>
        <textarea id="edit-body" bind:value={editBody} rows="8" disabled={saving}></textarea>
      </div>
      <div class="field">
        <label for="edit-image">Image address</label>
        <input id="edit-image" type="url" bind:value={editImageUrl} disabled={saving} placeholder="https://..." />
      </div>
      {#if editImageUrl.trim()}
        <div class="field">
          <label for="edit-image-alt">Describe the image</label>
          <input id="edit-image-alt" type="text" bind:value={editImageAlt} disabled={saving} />
        </div>
      {/if}
      <div class="form-actions">
        <button type="submit" class="btn btn-primary" disabled={saving}>{saving ? 'Saving…' : 'Save'}</button>
        <button type="button" class="btn btn-secondary" onclick={() => editing = false} disabled={saving}>Cancel</button>
      </div>
    </form>
  {:else}
    <article class="notice">
      <h2 class="notice-title">{notice.title}</h2>
      <div class="notice-meta muted">
        <span>{notice.author_display_name || notice.author_username}</span>
        <span>{formatRelative(notice.created_at)}</span>
        {#if notice.members_told}<span class="mark">members told</span>{/if}
        {#if !notice.replies_open}<span class="mark">replies off</span>{/if}
      </div>

      {#if notice.image_url}
        <img class="notice-image" src={notice.image_url} alt={notice.image_alt} loading="lazy" />
      {/if}

      {#if notice.body}
        <div class="notice-body">
          <MarkdownRenderer content={notice.body} />
        </div>
      {/if}

      <div class="notice-actions">
        {#if mayEdit}
          <button class="action-link" onclick={startEdit}>Edit</button>
        {/if}
        {#if mayManage}
          <button class="action-link" onclick={() => setReplies(!notice.replies_open)}>
            {notice.replies_open ? 'Switch replies off' : 'Switch replies on'}
          </button>
          <ConfirmAction label="Take down" confirmLabel="Yes, take it down" variant="danger" onConfirm={takeDown} />
        {/if}
        {#if me && notice.author_id !== me.id}
          <ReportButton entityType="notice" entityId={notice.id} entityName={notice.title} />
        {/if}
      </div>
    </article>

    <section class="replies">
      <h3 class="replies-heading">
        {replies.length === 0 ? 'No replies' : replies.length === 1 ? '1 reply' : `${replies.length} replies`}
        {#if !notice.replies_open}<span class="muted replies-note">{' · '}Replies are off on this notice.</span>{/if}
      </h3>

      {#each replies as reply (reply.id)}
        <div class="comment-card">
          <div class="comment-header">
            <span class="comment-author">{reply.author_display_name || reply.author_username}</span>
            <span class="comment-time">{formatRelative(reply.created_at)}</span>
          </div>
          <div class="comment-body">
            <MarkdownRenderer content={reply.body} />
          </div>
          <div class="comment-footer">
            <div class="comment-actions">
              {#if me && (reply.author_id === me.id || isAdmin)}
                <button class="action-link danger" onclick={() => removeReply(reply.id)}>
                  {deletingId === reply.id ? 'Confirm remove?' : 'Remove'}
                </button>
              {/if}
              {#if me && reply.author_id !== me.id}
                <ReportButton entityType="reply" entityId={reply.id} entityName="this reply" />
              {/if}
            </div>
          </div>
        </div>
      {/each}
      {#if nextCursor}
        <button class="btn btn-secondary btn-sm" onclick={() => loadReplies(nextCursor)}>More replies</button>
      {/if}

      {#if notice.replies_open}
        <form class="reply-form" onsubmit={(e) => { e.preventDefault(); sendReply(); }}>
          <textarea bind:value={replyText} rows="3" placeholder="Reply to this notice" disabled={replying}></textarea>
          <div class="form-actions">
            <button type="submit" class="btn btn-primary btn-sm" disabled={replying || !replyText.trim()}>{replying ? 'Sending…' : 'Reply'}</button>
          </div>
        </form>
      {/if}
    </section>
  {/if}
</div>

<style>
  .back { display: inline-block; font-size: 0.85rem; text-decoration: none; margin-bottom: 0.75rem; }
  .back:hover { color: var(--color-primary); }
  .notice-title { font-size: 1.3rem; margin: 0 0 0.25rem; }
  .notice-meta { display: flex; flex-wrap: wrap; gap: 0.25rem 0.75rem; font-size: 0.8rem; margin-bottom: 0.75rem; }
  .mark { border: 1px solid var(--color-border); border-radius: 999px; padding: 0 0.45rem; }
  .notice-image { max-width: 100%; border-radius: var(--radius); margin-bottom: 0.75rem; }
  .notice-body { font-size: 0.95rem; line-height: 1.6; }
  .notice-actions { display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap; margin: 0.75rem 0 1.5rem; }
  .action-link { border: none; background: none; color: var(--color-primary); font-size: 0.8rem; cursor: pointer; padding: 0; }
  .action-link:hover { text-decoration: underline; }
  .action-link.danger { color: var(--color-error); }

  .replies-heading { font-size: 0.95rem; margin: 0 0 0.75rem; }
  .replies-note { font-weight: 400; font-size: 0.85rem; }
  .comment-card { padding: 0.75rem; border: 1px solid var(--color-border); border-radius: var(--radius); background: var(--color-surface); margin-bottom: 0.75rem; }
  .comment-header { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.35rem; }
  .comment-author { font-size: 0.85rem; font-weight: 600; color: var(--color-text); }
  .comment-time { font-size: 0.75rem; color: var(--color-text-muted); }
  .comment-body { font-size: 0.9rem; line-height: 1.6; }
  .comment-body :global(p) { margin: 0.25rem 0; }
  .comment-footer { display: flex; justify-content: flex-end; margin-top: 0.5rem; }
  .comment-actions { display: flex; gap: 0.75rem; align-items: center; }

  .reply-form, .edit-form { display: flex; flex-direction: column; gap: 0.5rem; margin-top: 0.75rem; }
  .edit-form { gap: 1rem; max-width: 40rem; }
  .field { display: flex; flex-direction: column; gap: 0.25rem; }
  .field > label { font-size: 0.85rem; font-weight: 500; color: var(--color-text-muted); }
  .field input, .field textarea, .reply-form textarea {
    padding: 0.5rem 0.65rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    font: inherit;
  }
  .form-actions { display: flex; gap: 0.5rem; }
</style>
