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
      hydrated = true;
    }
  });

  function addLink() {
    links = [...links, { url: '', label: '' }];
  }

  function removeLink(i) {
    links = links.filter((_, idx) => idx !== i);
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
</style>
