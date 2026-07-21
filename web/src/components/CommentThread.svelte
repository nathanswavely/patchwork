<script>
  import { api } from '../lib/api.js';
  import MarkdownRenderer from './MarkdownRenderer.svelte';
  import { showToast } from '../stores/toast.svelte.js';

  let { proposalId = '', isMember = false, isAdmin = false } = $props();

  let comments = $state([]);
  let loading = $state(true);
  let error = $state('');
  let newComment = $state('');
  let submitting = $state(false);
  let replyingTo = $state(null);
  let replyText = $state('');
  let editingId = $state(null);
  let editText = $state('');
  // Two-step delete: first click arms the confirm, second click deletes.
  let deletingId = $state(null);

  const REACTION_EMOJIS = ['\u{1F44D}', '\u{1F44E}', '\u{2764}\u{FE0F}', '\u{1F914}', '\u{1F389}', '\u{1F440}'];

  $effect(() => {
    if (proposalId) {
      loadComments();
    }
  });

  async function loadComments() {
    loading = true;
    error = '';
    try {
      const res = await api(`proposals/${proposalId}/comments`);
      comments = res.items || [];
    } catch (e) {
      error = e.message || 'Failed to load comments';
      comments = [];
    } finally {
      loading = false;
    }
  }

  async function postComment() {
    if (!newComment.trim()) return;
    submitting = true;
    try {
      await api(`proposals/${proposalId}/comments`, {
        method: 'POST',
        body: { body: newComment.trim() },
      });
      newComment = '';
      await loadComments();
      showToast('Comment posted', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to post comment', 'error');
    } finally {
      submitting = false;
    }
  }

  async function postReply(parentId) {
    if (!replyText.trim()) return;
    submitting = true;
    try {
      await api(`proposals/${proposalId}/comments`, {
        method: 'POST',
        body: { body: replyText.trim(), parent_id: parentId },
      });
      replyText = '';
      replyingTo = null;
      await loadComments();
      showToast('Reply posted', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to post reply', 'error');
    } finally {
      submitting = false;
    }
  }

  async function saveEdit(commentId) {
    if (!editText.trim()) return;
    submitting = true;
    try {
      await api(`comments/${commentId}`, {
        method: 'PATCH',
        body: { body: editText.trim() },
      });
      editingId = null;
      editText = '';
      await loadComments();
      showToast('Comment updated', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to update comment', 'error');
    } finally {
      submitting = false;
    }
  }

  async function deleteComment(commentId) {
    if (deletingId !== commentId) {
      deletingId = commentId;
      return;
    }
    submitting = true;
    try {
      await api(`comments/${commentId}`, { method: 'DELETE' });
      deletingId = null;
      await loadComments();
      showToast('Comment deleted', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to delete comment', 'error');
    } finally {
      submitting = false;
    }
  }

  async function toggleReaction(commentId, emoji, alreadyReacted) {
    try {
      if (alreadyReacted) {
        await api(`comments/${commentId}/reactions/${encodeURIComponent(emoji)}`, {
          method: 'DELETE',
        });
      } else {
        await api(`comments/${commentId}/reactions`, {
          method: 'POST',
          body: { emoji },
        });
      }
      await loadComments();
    } catch (e) {
      showToast(e.message || 'Failed to update reaction', 'error');
    }
  }

  function timeAgo(dateStr) {
    if (!dateStr) return '';
    const now = Date.now();
    const then = new Date(dateStr).getTime();
    const diff = now - then;
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins} minute${mins > 1 ? 's' : ''} ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours} hour${hours > 1 ? 's' : ''} ago`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days} day${days > 1 ? 's' : ''} ago`;
    const months = Math.floor(days / 30);
    return `${months} month${months > 1 ? 's' : ''} ago`;
  }

  function startEdit(comment) {
    editingId = comment.id;
    editText = comment.body;
  }

  function startReply(commentId) {
    replyingTo = commentId;
    replyText = '';
  }
</script>

<div class="comment-thread">
  {#if loading}
    <p class="muted">Loading comments...</p>
  {:else if error}
    <p class="error-text">{error}</p>
  {:else}
    {#if comments.length === 0}
      <p class="muted">No comments yet.</p>
    {/if}

    {#each comments as comment}
      <div class="comment-card">
        <div class="comment-header">
          <span class="comment-author">{comment.author_name || 'Anonymous'}</span>
          <span class="comment-time">{timeAgo(comment.created_at)}</span>
        </div>

        {#if editingId === comment.id}
          <div class="comment-edit">
            <textarea bind:value={editText} rows="3" disabled={submitting}></textarea>
            <div class="comment-edit-actions">
              <button class="btn btn-primary btn-sm" onclick={() => saveEdit(comment.id)} disabled={submitting}>Save</button>
              <button class="btn btn-secondary btn-sm" onclick={() => { editingId = null; editText = ''; }}>Cancel</button>
            </div>
          </div>
        {:else}
          <div class="comment-body">
            <MarkdownRenderer content={comment.body} />
          </div>
        {/if}

        <div class="comment-footer">
          <div class="reaction-bar">
            {#each REACTION_EMOJIS as emoji}
              {@const reaction = (comment.reactions || []).find(r => r.emoji === emoji)}
              {@const count = reaction?.count || 0}
              {@const me = reaction?.me || false}
              {#if count > 0 || isMember}
                <button
                  class="reaction-btn"
                  class:active={me}
                  onclick={() => toggleReaction(comment.id, emoji, me)}
                  disabled={!isMember}
                >
                  {emoji}{#if count > 0}<span class="reaction-count">{count}</span>{/if}
                </button>
              {/if}
            {/each}
          </div>
          <div class="comment-actions">
            {#if isMember}
              <button class="action-link" onclick={() => startReply(comment.id)}>Reply</button>
            {/if}
            {#if comment.author_id && comment.is_mine}
              <button class="action-link" onclick={() => startEdit(comment)}>Edit</button>
            {/if}
            {#if comment.is_mine || isAdmin}
              <button class="action-link danger" onclick={() => deleteComment(comment.id)} disabled={submitting}>
                {deletingId === comment.id ? 'Confirm delete?' : 'Delete'}
              </button>
            {/if}
          </div>
        </div>

        {#if replyingTo === comment.id}
          <div class="reply-form">
            <textarea
              bind:value={replyText}
              rows="2"
              placeholder="Write a reply..."
              disabled={submitting}
            ></textarea>
            <div class="reply-form-actions">
              <button class="btn btn-primary btn-sm" onclick={() => postReply(comment.id)} disabled={submitting || !replyText.trim()}>Reply</button>
              <button class="btn btn-secondary btn-sm" onclick={() => { replyingTo = null; replyText = ''; }}>Cancel</button>
            </div>
          </div>
        {/if}

        {#if comment.replies && comment.replies.length > 0}
          {#each comment.replies as reply, depth}
            {@const nestLevel = Math.min(depth, 2)}
            <div class="reply-card" style="margin-left: {Math.min(1.5, 1.5)}rem;">
              <div class="comment-header">
                <span class="comment-author">{reply.author_name || 'Anonymous'}</span>
                <span class="comment-time">{timeAgo(reply.created_at)}</span>
              </div>

              {#if editingId === reply.id}
                <div class="comment-edit">
                  <textarea bind:value={editText} rows="2" disabled={submitting}></textarea>
                  <div class="comment-edit-actions">
                    <button class="btn btn-primary btn-sm" onclick={() => saveEdit(reply.id)} disabled={submitting}>Save</button>
                    <button class="btn btn-secondary btn-sm" onclick={() => { editingId = null; editText = ''; }}>Cancel</button>
                  </div>
                </div>
              {:else}
                <div class="comment-body">
                  <MarkdownRenderer content={reply.body} />
                </div>
              {/if}

              <div class="comment-footer">
                <div class="reaction-bar">
                  {#each REACTION_EMOJIS as emoji}
                    {@const reaction = (reply.reactions || []).find(r => r.emoji === emoji)}
                    {@const count = reaction?.count || 0}
                    {@const me = reaction?.me || false}
                    {#if count > 0 || isMember}
                      <button
                        class="reaction-btn"
                        class:active={me}
                        onclick={() => toggleReaction(reply.id, emoji, me)}
                        disabled={!isMember}
                      >
                        {emoji}{#if count > 0}<span class="reaction-count">{count}</span>{/if}
                      </button>
                    {/if}
                  {/each}
                </div>
                <div class="comment-actions">
                  {#if reply.is_mine || isAdmin}
                    <button class="action-link danger" onclick={() => deleteComment(reply.id)} disabled={submitting}>
                      {deletingId === reply.id ? 'Confirm delete?' : 'Delete'}
                    </button>
                  {/if}
                  {#if reply.is_mine}
                    <button class="action-link" onclick={() => startEdit(reply)}>Edit</button>
                  {/if}
                </div>
              </div>

              {#if reply.replies && reply.replies.length > 0}
                {#each reply.replies as nestedReply}
                  <div class="reply-card" style="margin-left: 1.5rem;">
                    <div class="comment-header">
                      <span class="comment-author">{nestedReply.author_name || 'Anonymous'}</span>
                      <span class="comment-time">{timeAgo(nestedReply.created_at)}</span>
                    </div>
                    <div class="comment-body">
                      <MarkdownRenderer content={nestedReply.body} />
                    </div>
                    <div class="comment-footer">
                      <div class="reaction-bar">
                        {#each REACTION_EMOJIS as emoji}
                          {@const reaction = (nestedReply.reactions || []).find(r => r.emoji === emoji)}
                          {@const count = reaction?.count || 0}
                          {@const me = reaction?.me || false}
                          {#if count > 0 || isMember}
                            <button
                              class="reaction-btn"
                              class:active={me}
                              onclick={() => toggleReaction(nestedReply.id, emoji, me)}
                              disabled={!isMember}
                            >
                              {emoji}{#if count > 0}<span class="reaction-count">{count}</span>{/if}
                            </button>
                          {/if}
                        {/each}
                      </div>
                    </div>
                  </div>
                {/each}
              {/if}
            </div>
          {/each}
        {/if}
      </div>
    {/each}

    {#if isMember}
      <div class="new-comment">
        <textarea
          bind:value={newComment}
          rows="3"
          placeholder="Add a comment..."
          disabled={submitting}
        ></textarea>
        <button class="btn btn-primary" onclick={postComment} disabled={submitting || !newComment.trim()}>
          {submitting ? 'Posting...' : 'Comment'}
        </button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .comment-card {
    padding: 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    margin-bottom: 0.75rem;
  }

  .comment-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.35rem;
  }

  .comment-author {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--color-text);
  }

  .comment-time {
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .comment-body {
    font-size: 0.9rem;
    line-height: 1.6;
  }

  .comment-body :global(p) {
    margin: 0.25rem 0;
  }

  .comment-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 0.5rem;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .reaction-bar {
    display: flex;
    gap: 0.25rem;
    flex-wrap: wrap;
  }

  .reaction-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    padding: 0.15rem 0.4rem;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    background: var(--color-bg);
    font-size: 0.8rem;
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .reaction-btn:hover:not(:disabled) {
    border-color: var(--color-primary);
  }

  .reaction-btn.active {
    background: color-mix(in srgb, var(--color-primary) 10%, transparent);
    border-color: var(--color-primary);
  }

  .reaction-btn:disabled {
    cursor: default;
    opacity: 0.7;
  }

  .reaction-count {
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--color-text);
  }

  .comment-actions {
    display: flex;
    gap: 0.75rem;
  }

  .action-link {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: 0.8rem;
    cursor: pointer;
    padding: 0;
  }

  .action-link:hover {
    text-decoration: underline;
  }

  .action-link.danger {
    color: var(--color-error);
  }

  .reply-card {
    padding: 0.6rem 0 0.6rem 0.75rem;
    border-left: 2px solid var(--color-border);
    margin-top: 0.5rem;
  }

  .reply-form {
    margin-top: 0.5rem;
    padding: 0.5rem;
    background: var(--color-bg);
    border-radius: var(--radius);
  }

  .reply-form textarea {
    width: 100%;
    resize: vertical;
    min-height: 60px;
    margin-bottom: 0.35rem;
  }

  .reply-form-actions {
    display: flex;
    gap: 0.4rem;
  }

  .comment-edit textarea {
    width: 100%;
    resize: vertical;
    min-height: 60px;
    margin-bottom: 0.35rem;
  }

  .comment-edit-actions {
    display: flex;
    gap: 0.4rem;
  }

  .new-comment {
    margin-top: 1rem;
    padding-top: 1rem;
    border-top: 1px solid var(--color-border);
  }

  .new-comment textarea {
    width: 100%;
    resize: vertical;
    min-height: 80px;
    margin-bottom: 0.5rem;
  }

</style>
