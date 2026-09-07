<script>
  import { api } from '../lib/api.js';
  import { getUser, checkAuth } from '../stores/auth.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { navigate } from '../stores/router.svelte.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from '../components/PasskeyNotice.svelte';

  let user = $derived(getUser());
  let displayName = $state('');
  // Personal discovery filter (docs/adr/037).
  let hideAmendedLinings = $state(false);
  let hideAmendedSaving = $state(false);
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
    api('auth/me')
      .then((me) => { hideAmendedLinings = !!me.hide_amended_linings; })
      .catch(() => {});
  });

  async function saveHideAmended(value) {
    hideAmendedSaving = true;
    try {
      await api('auth/me', { method: 'PATCH', body: { hide_amended_linings: value } });
      hideAmendedLinings = value;
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    } finally {
      hideAmendedSaving = false;
    }
  }

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

  // Contact card (docs/adr/083): the ways you are willing to be reached,
  // kept once here as typed items. Nothing here is public. This page owns
  // the items; which patch sees which item is decided per patch, in My
  // Patches — granting is patch-first because that is where the intent
  // forms. The only sharing action on this page is taking one back.
  const CONTACT_KINDS = [
    { value: 'phone', label: 'Phone' },
    { value: 'email', label: 'Email' },
    { value: 'handle', label: 'Handle' },
    { value: 'note', label: 'Note' },
  ];
  const KIND_MAX = { phone: 60, email: 254, handle: 100, note: 200 };
  const KIND_PLACEHOLDER = {
    phone: '+1 717 555 0100',
    email: 'reach@example.com',
    handle: '@you on Signal',
    note: 'Text before calling',
  };
  const MAX_CONTACT_ITEMS = 12;

  let contactItems = $state([]);
  let contactBusy = $state(false);
  let draft = $state(null);

  $effect(() => {
    if (user && !hydrated) {
      displayName = user.display_name || '';
      bio = user.bio || '';
      links = (user.links || []).map((l) => ({ ...l }));
      startOnMyQuilt = !!user.start_on_my_quilt;
      hydrated = true;
    }
  });

  $effect(() => {
    loadContactItems();
  });

  async function loadContactItems() {
    try {
      const data = await api('users/me/contact-items');
      // `saved` is the copy the server holds, so a row knows when it is dirty
      // without asking again.
      contactItems = (data.items || []).map((it) => ({ ...it, saved: { ...it } }));
    } catch {
      contactItems = [];
    }
  }

  function isDirty(it) {
    return it.value !== it.saved.value || it.kind !== it.saved.kind || (it.label || '') !== (it.saved.label || '');
  }

  function startDraft() {
    draft = { kind: 'phone', value: '', label: '' };
  }

  async function addContactItem() {
    if (!draft || !draft.value.trim()) return;
    contactBusy = true;
    try {
      await api('users/me/contact-items', {
        method: 'POST',
        body: { kind: draft.kind, value: draft.value.trim(), label: draft.label.trim() },
      });
      draft = null;
      await loadContactItems();
      showToast('Contact item added', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to add', 'error');
    } finally {
      contactBusy = false;
    }
  }

  async function saveContactItem(it) {
    contactBusy = true;
    try {
      await api(`users/me/contact-items/${it.id}`, {
        method: 'PATCH',
        body: { kind: it.kind, value: it.value.trim(), label: (it.label || '').trim() },
      });
      await loadContactItems();
      // Editing a value in place keeps every share it already has: sharing is
      // a pairing, not a copy, so a new number reaches the same rooms.
      showToast('Saved. The patches you share it with see the new value', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    } finally {
      contactBusy = false;
    }
  }

  async function removeContactItem(it) {
    contactBusy = true;
    try {
      await api(`users/me/contact-items/${it.id}`, { method: 'DELETE' });
      await loadContactItems();
      showToast('Contact item removed', 'info');
    } catch (e) {
      showToast(e.message || 'Failed to remove', 'error');
    } finally {
      contactBusy = false;
    }
  }

  async function stopSharingEverywhere(it) {
    contactBusy = true;
    try {
      const res = await api(`users/me/contact-items/${it.id}/shares`, { method: 'DELETE' });
      await loadContactItems();
      // Deliberately not "revoked": this stops the surface, not the knowing.
      showToast(`No longer shown to ${res.unshared_from === 1 ? '1 patch' : `${res.unshared_from} patches`}`, 'info');
    } catch (e) {
      showToast(e.message || 'Failed to stop sharing', 'error');
    } finally {
      contactBusy = false;
    }
  }

  // The moved-to pointer (docs/adr/090): where this person went, on their
  // own profile and on their AP actor. Independent of any patch — a person
  // can move without their patches, and the other way round.
  let movedTo = $state('');
  let movedSaving = $state(false);
  let movedError = $state('');

  $effect(() => {
    api('auth/me')
      .then((me) => { movedTo = me.moved_to || ''; })
      .catch(() => {});
  });

  async function saveMovedTo() {
    movedSaving = true;
    movedError = '';
    try {
      await api('auth/me', { method: 'PATCH', body: { moved_to: movedTo.trim() } });
      await checkAuth();
      showToast('Saved', 'success');
    } catch (e) {
      movedError = e.message || 'Could not save that link';
    } finally {
      movedSaving = false;
    }
  }

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
      showToast('Copy failed. Select the address instead.', 'error');
    }
  }

  // Download my data (docs/adr/012, affordance 1): the person's own record,
  // no admin involved. Gated behind the session, so it can't be a plain link
  // — fetch it and hand the browser the blob, the way the admin export does.
  let downloadingData = $state(false);

  async function downloadMyData() {
    downloadingData = true;
    try {
      const res = await fetch('/api/v1/users/me/export', { credentials: 'same-origin' });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        throw new Error(body?.error || `Download failed (${res.status})`);
      }
      // The server names the file; fall back only if the header is missing.
      const disposition = res.headers.get('Content-Disposition') || '';
      const named = disposition.match(/filename="?([^";]+)"?/);
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = named ? named[1] : 'patchwork-my-data.json';
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      showToast(e.message || 'Download failed', 'error');
    }
    downloadingData = false;
  }

  // Member seamrip (docs/adr/012, affordance 2; docs/adr/089): the quilt as
  // this person can already see it, in the import format, so a fork needs
  // nobody's permission. Same shape as the download above — behind the
  // session, so the browser gets a blob rather than a link — and a zip
  // rather than one JSON file, because it is the whole archive.
  let takingSeamrip = $state(false);

  async function takeMemberSeamrip() {
    takingSeamrip = true;
    try {
      const res = await fetch('/api/v1/users/me/seamrip', { credentials: 'same-origin' });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        throw new Error(body?.error || `Download failed (${res.status})`);
      }
      const disposition = res.headers.get('Content-Disposition') || '';
      const named = disposition.match(/filename="?([^";]+)"?/);
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = named ? named[1] : 'patchwork-member-seamrip.zip';
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      showToast(e.message || 'Download failed', 'error');
    }
    takingSeamrip = false;
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
  // Danger zone: deleting your own account (docs/adr/086).
  //
  // Step-up gated like the other irreversible actions, so the status is read
  // on load — discovering you need a passkey at the moment you are trying to
  // leave is the failure mode PasskeyNotice exists to avoid.
  let hasPasskey = $state(true);
  let confirmUsername = $state('');
  let deleteArmed = $state(false);
  let deleting = $state(false);
  // Patches the server refused over: the person is their only admin.
  let blockingPatches = $state([]);

  $effect(() => {
    stepUpStatus().then((s) => { hasPasskey = !!s.has_passkey; });
  });

  async function deleteAccount() {
    deleting = true;
    try {
      await withStepUp(() => api('users/me', {
        method: 'DELETE',
        body: { confirm_username: confirmUsername },
      }));
      // The session went with the account. Hard reload rather than a
      // client-side navigate: every store still holds the deleted person.
      localStorage.clear();
      window.location.href = '/';
    } catch (e) {
      if (e instanceof PasskeyRequiredError) hasPasskey = false;
      blockingPatches = e?.data?.patches || [];
      showToast(e.message || 'Failed to delete account', 'error');
      deleting = false;
      deleteArmed = false;
    }
  }
</script>

<div class="page-fade">
  {#if user}
    <section class="pw-section">
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

    <section class="pw-section">
      <h2>I have moved</h2>
      <p class="muted profile-hint">
        Point your profile at wherever you are now. Your page here keeps
        working and says where you went. This is only about you, not about
        the patches you belong to. Clear the field to undo it.
      </p>
      <form onsubmit={(e) => { e.preventDefault(); saveMovedTo(); }}>
        <div class="field">
          <label for="moved-to">New address</label>
          <input
            id="moved-to"
            type="url"
            bind:value={movedTo}
            disabled={movedSaving}
            placeholder="https://their-quilt.example.com/users/you"
          />
          {#if movedError}<small class="error-text">{movedError}</small>{/if}
        </div>
        <div class="field-actions">
          <button type="submit" class="btn btn-primary" disabled={movedSaving}>
            {movedSaving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </form>
    </section>

    <section class="pw-section">
      <h2>Contact card</h2>
      <p class="muted profile-hint">
        The ways you are willing to be reached. Nothing here is public and
        nothing is shared by adding it — you give each item to one patch at a
        time, in
        <a href="/settings/patches" onclick={(e) => { e.preventDefault(); navigate('/settings/patches'); }}>My Patches</a>.
        A patch's admins and members can then see it; its followers cannot.
      </p>

      {#if contactItems.length === 0 && !draft}
        <p class="muted">Nothing on your card yet.</p>
      {/if}

      <ul class="contact-items">
        {#each contactItems as item (item.id)}
          <li class="contact-item-row">
            <div class="contact-item-fields">
              <select bind:value={item.kind} disabled={contactBusy} aria-label="Kind">
                {#each CONTACT_KINDS as k}
                  <option value={k.value}>{k.label}</option>
                {/each}
              </select>
              <input
                type="text"
                bind:value={item.value}
                disabled={contactBusy}
                maxlength={KIND_MAX[item.kind]}
                placeholder={KIND_PLACEHOLDER[item.kind]}
                aria-label="Value"
              />
              <input
                type="text"
                bind:value={item.label}
                disabled={contactBusy}
                maxlength="40"
                placeholder="Label (optional)"
                aria-label="Label"
              />
            </div>
            <div class="contact-item-meta">
              <span class="muted">
                {#if item.shared_with === 1}
                  Shared with 1 patch
                {:else if item.shared_with > 1}
                  Shared with {item.shared_with} patches
                {:else}
                  Not shared with any patch
                {/if}
              </span>
              <div class="contact-item-actions">
                {#if isDirty(item)}
                  <button type="button" class="btn btn-primary btn-sm" disabled={contactBusy} onclick={() => saveContactItem(item)}>Save</button>
                {/if}
                {#if item.shared_with > 0}
                  <button type="button" class="btn btn-sm" disabled={contactBusy} onclick={() => stopSharingEverywhere(item)}>
                    Stop sharing everywhere
                  </button>
                {/if}
                <button type="button" class="btn btn-sm btn-danger" disabled={contactBusy} onclick={() => removeContactItem(item)}>Remove</button>
              </div>
            </div>
          </li>
        {/each}
      </ul>

      {#if draft}
        <form class="contact-draft" onsubmit={(e) => { e.preventDefault(); addContactItem(); }}>
          <div class="contact-item-fields">
            <select bind:value={draft.kind} disabled={contactBusy} aria-label="Kind">
              {#each CONTACT_KINDS as k}
                <option value={k.value}>{k.label}</option>
              {/each}
            </select>
            <input
              type="text"
              bind:value={draft.value}
              disabled={contactBusy}
              maxlength={KIND_MAX[draft.kind]}
              placeholder={KIND_PLACEHOLDER[draft.kind]}
              aria-label="Value"
            />
            <input type="text" bind:value={draft.label} disabled={contactBusy} maxlength="40" placeholder="Label (optional)" aria-label="Label" />
          </div>
          {#if draft.kind === 'email'}
            <small class="muted">Separate from the address you sign in with. That one is never shared.</small>
          {/if}
          <div class="field-actions">
            <button type="submit" class="btn btn-primary" disabled={contactBusy || !draft.value.trim()}>Add item</button>
            <button type="button" class="btn" disabled={contactBusy} onclick={() => (draft = null)}>Cancel</button>
          </div>
        </form>
      {:else if contactItems.length < MAX_CONTACT_ITEMS}
        <div class="field-actions">
          <button type="button" class="btn" disabled={contactBusy} onclick={startDraft}>Add contact item</button>
        </div>
      {:else}
        <p class="muted">A contact card holds {MAX_CONTACT_ITEMS} items at most.</p>
      {/if}
    </section>

    <section class="pw-section">
      <h2>When you open Patchwork</h2>
      <p class="muted profile-hint">
        Patchwork opens on the whole quilt. Switch this on to start on My Quilt instead.
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

    <section class="pw-section">
      <h2>Discovery</h2>
      <label class="toggle-row">
        <input
          type="checkbox"
          checked={hideAmendedLinings}
          disabled={hideAmendedSaving}
          onchange={(e) => saveHideAmended(e.target.checked)}
        />
        <span>Hide patches that amended the lining</span>
      </label>
      <p class="muted profile-hint">
        Keeps patches that changed the shared community standards out of your
        quilt, search, map, and event feed. Direct links still work.
      </p>
    </section>

    <section class="pw-section">
      <h2>Personal feed</h2>
      <p class="muted profile-hint">
        A calendar feed of every event on your My Quilt, for your calendar
        app. The address is a secret. Anyone who has it can read your feed.
      </p>
      {#if feedUrl}
        <div class="feed-url-row">
          <code class="feed-url">{feedUrl}</code>
          <button class="btn btn-secondary" onclick={copyFeedUrl}>Copy</button>
        </div>
        <p class="muted profile-hint">
          Copy it now. This address won't be shown again, and regenerating
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

    <section class="pw-section">
      <h2>Download my data</h2>
      <p class="muted profile-hint">
        One file with everything this quilt holds about you: your profile and
        contact card, every membership including the ones you keep hidden, the
        proposals, votes, comments, notices and events you wrote, and your
        settings. Nothing anyone else wrote, and no sign-in secrets — so it is
        safe to keep, and it will not let anyone into your account.
      </p>
      <div class="field-actions">
        <button class="btn btn-secondary" onclick={downloadMyData} disabled={downloadingData}>
          {downloadingData ? 'Preparing...' : 'Download my data'}
        </button>
      </div>
    </section>

    <section class="pw-section">
      <h2>Member seamrip</h2>
      <p class="muted profile-hint">
        A copy of this quilt as you can see it, in the format a new Patchwork
        reads. It is here so a community can start again elsewhere without
        waiting for an admin. It holds the patches, events, charters,
        proposals and member lists that are already open to you. It does not
        hold email addresses, contact cards, noticeboards, or anything from a
        patch you are not in. Other people travel as a name and a picture,
        so the new quilt invites everyone back and each person sets their own
        visibility there.
      </p>
      <div class="field-actions">
        <button class="btn btn-secondary" onclick={takeMemberSeamrip} disabled={takingSeamrip}>
          {takingSeamrip ? 'Preparing...' : 'Take a copy'}
        </button>
      </div>
    </section>

    {#if steward?.steward}
      <section class="pw-section">
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

    <!-- ===== Danger zone (docs/adr/086) ===== -->
    <section class="pw-section">
      <h2 class="danger-heading">Danger Zone</h2>
      <div class="danger-card">
        <h3>Delete your account</h3>
        <PasskeyNotice show={!hasPasskey} action="delete your account" />
        <p class="danger-warning">
          This happens immediately and cannot be undone.
        </p>
        <p class="danger-warning">
          <strong>Erased:</strong> your email address, display name, bio, links,
          avatar and contact card. Your passkeys, recovery codes and sessions.
          Your personal calendar link and notification settings. Every patch
          membership you hold, the quilts you have connected or followed, and
          any claim still awaiting review.
        </p>
        <p class="danger-warning">
          <strong>Kept:</strong> what you took part in. Proposals, votes,
          comments, notices, events and recorded decisions stay so the
          community's record stays whole. They will read as
          <strong>&ldquo;Deleted account&rdquo;</strong> with no name and no
          link. Your profile page stops existing, and your username stays
          retired so nobody else can take it.
        </p>
        <p class="danger-warning">
          Copies that already federated to other servers, or that went out in an
          export taken before today, are outside this site's reach.
        </p>

        {#if blockingPatches.length > 0}
          <div class="blockers" role="status">
            <p>
              You are the only admin of
              {blockingPatches.length === 1 ? 'this patch' : 'these patches'}.
              Hand {blockingPatches.length === 1 ? 'it' : 'them'} over first,
              then come back. You can name a successor, promote another admin, or hold an election.
            </p>
            <ul>
              {#each blockingPatches as p}
                <li>
                  <a href="/patches/{p.slug}/settings" onclick={(e) => { e.preventDefault(); navigate(`/patches/${p.slug}/settings`); }}>{p.name}</a>
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        <label class="field">
          <span class="field-label">Type your username to confirm: <strong>{user.username}</strong></span>
          <input
            type="text"
            bind:value={confirmUsername}
            placeholder={user.username}
            autocomplete="off"
            spellcheck="false"
          />
        </label>
        {#if !deleteArmed}
          <button
            class="btn btn-danger"
            disabled={confirmUsername !== user.username}
            onclick={() => { deleteArmed = true; }}
          >
            Delete My Account…
          </button>
        {:else}
          <div class="delete-confirm">
            <span class="danger-warning">Really delete your account? You will be signed out.</span>
            <button class="btn btn-danger" onclick={deleteAccount} disabled={deleting}>
              {deleting ? 'Deleting…' : 'Yes, delete my account'}
            </button>
            <button class="btn btn-secondary" onclick={() => { deleteArmed = false; }} disabled={deleting}>
              Cancel
            </button>
          </div>
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
  .danger-heading {
    color: var(--color-error);
  }

  .danger-card {
    border: 1px solid var(--color-error);
    border-radius: 6px;
    padding: 1.25rem;
    max-width: var(--pw-measure-narrow);
  }

  .danger-card h3 {
    font-size: 0.95rem;
    font-weight: 600;
    margin-bottom: 0.75rem;
  }

  .danger-warning {
    font-size: 0.88rem;
    color: var(--color-text-muted);
    line-height: 1.5;
    margin-bottom: 0.75rem;
  }

  .blockers {
    border: 1px solid var(--color-warning, #b8860b);
    background: var(--color-warning-bg, rgba(184, 134, 11, 0.08));
    border-radius: var(--radius-sm, 4px);
    padding: 0.75rem 1rem;
    margin-bottom: 1rem;
    font-size: 0.88rem;
    line-height: 1.5;
  }

  .blockers ul {
    margin: 0.5rem 0 0 1.1rem;
  }

  .blockers a {
    text-decoration: underline;
  }

  .delete-confirm {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem;
  }

  h2 {
    font-size: 1.1rem;
    margin-bottom: 0.5rem;
  }

  .profile-hint {
    font-size: 0.85rem;
    margin-bottom: 1rem;
  }

  .toggle-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.9rem;
    cursor: pointer;
    margin-bottom: 0.35rem;
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

  /* Contact card items (docs/adr/083). A row is one item: what it is, its
     value, and who can already reach you by it. The sharing count sits on
     the row rather than in a header, because sharing is per item. */
  .contact-items {
    list-style: none;
    padding: 0;
    margin: 0 0 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .contact-item-row {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding-bottom: 0.75rem;
    border-bottom: 1px solid var(--border);
  }
  .contact-item-row:last-child {
    border-bottom: none;
  }
  .contact-item-fields {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
  .contact-item-fields select {
    flex: 0 0 auto;
  }
  .contact-item-fields input {
    flex: 1 1 12rem;
    min-width: 0;
  }
  .contact-item-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    flex-wrap: wrap;
    font-size: 0.85rem;
  }
  .contact-item-actions {
    display: flex;
    gap: 0.4rem;
    flex-wrap: wrap;
  }
  .contact-draft {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding-top: 0.25rem;
  }

</style>
