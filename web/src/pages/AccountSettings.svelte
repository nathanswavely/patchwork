<script>
  import { api } from '../lib/api.js';
  import { getUser, checkAuth } from '../stores/auth.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { navigate } from '../stores/router.svelte.js';

  let user = $derived(getUser());
  let displayName = $state('');
  let bio = $state('');
  let links = $state([]);
  let saving = $state(false);
  let saveMessage = $state('');
  let hydrated = false;

  // Landing preference (docs/adr/035): start on the whole quilt (default)
  // or on My Quilt. Saved on toggle — a single switch doesn't need a Save
  // button.
  let startOnMyQuilt = $state(false);
  let landingSaving = $state(false);

  async function setStartOnMyQuilt(value) {
    landingSaving = true;
    try {
      await api('auth/me', { method: 'PATCH', body: { start_on_my_quilt: value } });
      startOnMyQuilt = value;
      await checkAuth();
      showToast(value ? 'Patchwork will open on My Quilt' : 'Patchwork will open on the whole quilt', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    }
    landingSaving = false;
  }

  // Steward listing (docs/adr/023): the person's own switch. Appears only
  // when this person is on the Label's stewards list — listed, or invited
  // by an instance admin and not yet accepted.
  let steward = $state(null);
  let stewardBlurb = $state('');
  let stewardSaving = $state(false);

  $effect(() => {
    api('users/me/steward')
      .then((s) => {
        steward = s;
        stewardBlurb = s.blurb || '';
      })
      .catch(() => { steward = null; });
  });

  async function setStewardListed(listed) {
    stewardSaving = true;
    try {
      await api('users/me/steward', {
        method: 'PATCH',
        body: { listed, blurb: stewardBlurb },
      });
      steward = { ...steward, listed, blurb: stewardBlurb };
      showToast(listed
        ? 'You are listed on the Label'
        : 'You are no longer listed on the Label', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    }
    stewardSaving = false;
  }

  async function declineSteward() {
    stewardSaving = true;
    try {
      await api('users/me/steward', { method: 'DELETE' });
      steward = { steward: false };
      showToast('Removed from the stewards list', 'info');
    } catch (e) {
      showToast(e.message || 'Failed to remove', 'error');
    }
    stewardSaving = false;
  }

  $effect(() => {
    if (user && !hydrated) {
      displayName = user.display_name || '';
      bio = user.bio || '';
      links = (user.links || []).map((l) => ({ ...l }));
      startOnMyQuilt = !!user.start_on_my_quilt;
      hydrated = true;
    }
  });

  function addLink() {
    links = [...links, { url: '', label: '' }];
  }

  function removeLink(i) {
    links = links.filter((_, idx) => idx !== i);
  }

  // Personal feed (docs/adr/031): one calendar of everything on My
  // Quilt, subscribed by secret URL. The URL is shown once — only its
  // hash is stored — and regenerating revokes the old one.
  let feedEnabled = $state(false);
  let feedUrl = $state('');
  let feedBusy = $state(false);

  $effect(() => {
    api('users/me/feed-secret')
      .then((s) => { feedEnabled = !!s.enabled; })
      .catch(() => { feedEnabled = false; });
  });

  async function generateFeed() {
    feedBusy = true;
    try {
      const res = await api('users/me/feed-secret', { method: 'POST' });
      feedUrl = res.url;
      feedEnabled = true;
    } catch (e) {
      showToast(e.message || 'Failed to create feed', 'error');
    }
    feedBusy = false;
  }

  async function disableFeed() {
    feedBusy = true;
    try {
      await api('users/me/feed-secret', { method: 'DELETE' });
      feedEnabled = false;
      feedUrl = '';
      showToast('Personal feed disabled', 'info');
    } catch (e) {
      showToast(e.message || 'Failed to disable feed', 'error');
    }
    feedBusy = false;
  }

  async function copyFeedUrl() {
    try {
      await navigator.clipboard.writeText(feedUrl);
      showToast('Copied');
    } catch {
      showToast('Copy failed — select the address instead', 'error');
    }
  }

  async function saveProfile() {
    saving = true;
    saveMessage = '';
    try {
      await api('auth/me', {
        method: 'PATCH',
        body: {
          display_name: displayName.trim(),
          bio: bio.trim(),
          links: links
            .map((l) => ({ url: l.url.trim(), label: l.label.trim() }))
            .filter((l) => l.url),
        },
      });
      await checkAuth();
      hydrated = false;
      saveMessage = 'Saved';
      showToast('Profile saved', 'success');
    } catch (e) {
      saveMessage = e.message || 'Failed to save';
      showToast('Something went wrong. Please try again.', 'error');
    } finally {
      saving = false;
    }
  }
</script>

<div class="page-fade">
  {#if user}
    <section class="card">
      <h2>Profile</h2>
      <p class="muted profile-hint">
        Your <a href="/users/{user.username}" onclick={(e) => { e.preventDefault(); navigate(`/users/${user.username}`); }}>public profile</a>
        shows your name, bio, links, and the memberships you keep visible.
      </p>
      <form onsubmit={(e) => { e.preventDefault(); saveProfile(); }}>
        <div class="field">
          <label for="username-display">Username</label>
          <input id="username-display" type="text" value={user.username} disabled />
          <small class="muted">Username cannot be changed.</small>
        </div>
        <div class="field">
          <label for="display-name">Display Name</label>
          <input id="display-name" type="text" bind:value={displayName} disabled={saving} />
        </div>
        <div class="field">
          <label for="bio">Bio</label>
          <textarea id="bio" rows="3" bind:value={bio} disabled={saving}></textarea>
        </div>
        <div class="field">
          <span class="field-label">Links</span>
          {#each links as link, i}
            <div class="link-row">
              <input type="text" placeholder="Label" bind:value={link.label} disabled={saving} class="link-label" />
              <input type="url" placeholder="https://…" bind:value={link.url} disabled={saving} class="link-url" />
              <button type="button" class="btn btn-sm link-remove" onclick={() => removeLink(i)} title="Remove link">✕</button>
            </div>
          {/each}
          <button type="button" class="btn btn-secondary btn-sm add-link" onclick={addLink} disabled={saving}>
            Add link
          </button>
        </div>
        <div class="field-actions">
          <button type="submit" class="btn btn-primary" disabled={saving}>
            {saving ? 'Saving...' : 'Save'}
          </button>
          {#if saveMessage}
            <span class={saveMessage === 'Saved' ? 'success-text' : 'error-text'}>
              {saveMessage}
            </span>
          {/if}
        </div>
      </form>
    </section>

    <section class="card">
      <h2>When you open Patchwork</h2>
      <p class="muted profile-hint">
        Patchwork opens on the whole quilt. Switch this on to start on My
        Quilt instead — the patches you're part of and follow. You can
        always move between them from the switcher.
      </p>
      <label class="toggle-row">
        <input
          type="checkbox"
          checked={startOnMyQuilt}
          disabled={landingSaving}
          onchange={(e) => setStartOnMyQuilt(e.currentTarget.checked)}
        />
        <span>Start on My Quilt</span>
      </label>
    </section>

    <section class="card">
      <h2>Personal feed</h2>
      <p class="muted profile-hint">
        A calendar feed of every event on your My Quilt, for your calendar
        app. The address is a secret — anyone who has it can read your feed.
      </p>
      {#if feedUrl}
        <div class="feed-url-row">
          <code class="feed-url">{feedUrl}</code>
          <button class="btn btn-secondary" onclick={copyFeedUrl}>Copy</button>
        </div>
        <p class="muted profile-hint">
          Copy it now — this address won't be shown again. Regenerating
          replaces it.
        </p>
      {/if}
      <div class="field-actions">
        {#if feedEnabled}
          <button class="btn btn-secondary" onclick={generateFeed} disabled={feedBusy}>
            Regenerate address
          </button>
          <button class="btn btn-secondary" onclick={disableFeed} disabled={feedBusy}>
            Disable
          </button>
        {:else}
          <button class="btn btn-primary" onclick={generateFeed} disabled={feedBusy}>
            Create feed
          </button>
        {/if}
      </div>
    </section>

    {#if steward?.steward}
      <section class="card">
        <h2>Steward listing</h2>
        {#if steward.listed}
          <p class="muted profile-hint">
            You're named on this quilt's <a href="/label" onclick={(e) => { e.preventDefault(); navigate('/label'); }}>Label</a>
            as one of its stewards. That listing belongs to you, and you can
            unlist yourself any time.
          </p>
        {:else}
          <p class="muted profile-hint">
            An instance admin wants to name you on this quilt's
            <a href="/label" onclick={(e) => { e.preventDefault(); navigate('/label'); }}>Label</a>
            as a steward. Nothing appears until you accept, and you can
            take it back later.
          </p>
        {/if}
        <div class="field">
          <label for="steward-blurb">What you look after (in your own words)</label>
          <input id="steward-blurb" type="text" maxlength="200"
            placeholder="keeps the lights on and pays the bill"
            bind:value={stewardBlurb} disabled={stewardSaving} />
        </div>
        <div class="field-actions">
          {#if steward.listed}
            <button class="btn btn-primary" onclick={() => setStewardListed(true)} disabled={stewardSaving}>
              Save
            </button>
            <button class="btn btn-secondary" onclick={() => setStewardListed(false)} disabled={stewardSaving}>
              Unlist me
            </button>
          {:else}
            <button class="btn btn-primary" onclick={() => setStewardListed(true)} disabled={stewardSaving}>
              Accept and list me
            </button>
            <button class="btn btn-secondary" onclick={declineSteward} disabled={stewardSaving}>
              Decline
            </button>
          {/if}
        </div>
      </section>
    {/if}
  {/if}
</div>

<style>
  h2 {
    font-size: 1.1rem;
    margin-bottom: 0.5rem;
  }

  .profile-hint {
    font-size: 0.85rem;
    margin-bottom: 1rem;
  }

  .feed-url-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: 0.5rem;
    min-width: 0;
  }

  .feed-url {
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
    min-width: 0;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .field label,
  .field-label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .link-row {
    display: flex;
    gap: 0.4rem;
    align-items: center;
  }

  .link-label {
    flex: 0 0 30%;
  }

  .link-url {
    flex: 1;
  }

  .link-remove {
    flex-shrink: 0;
    border: 1px solid var(--color-border);
    background: var(--color-surface);
    color: var(--color-text-muted);
  }

  .add-link {
    align-self: flex-start;
    margin-top: 0.25rem;
  }

  .field-actions {
    display: flex;
    align-items: center;
    gap: 1rem;
  }

  .toggle-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    cursor: pointer;
    font-size: 0.9rem;
    font-weight: 500;
  }

  .toggle-row input {
    width: 1.05rem;
    height: 1.05rem;
    accent-color: var(--color-primary);
    cursor: pointer;
  }
</style>
