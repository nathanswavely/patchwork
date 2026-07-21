<script>
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';
  import ReportButton from '../components/ReportButton.svelte';

  let { username = '' } = $props();

  let profile = $state(null);
  let loading = $state(true);
  let error = $state('');

  let isSelf = $derived(getUser()?.username === username);
  let adminOf = $derived((profile?.memberships || []).filter((m) => m.role === 'admin'));
  let memberOf = $derived((profile?.memberships || []).filter((m) => m.role === 'member'));

  $effect(() => {
    if (username) loadProfile();
  });

  async function loadProfile() {
    loading = true;
    error = '';
    try {
      profile = await api(`users/${encodeURIComponent(username)}`);
    } catch (e) {
      error = e.message || 'Failed to load profile';
      profile = null;
    } finally {
      loading = false;
    }
  }

  function extractDomain(url) {
    try { return new URL(url).hostname.replace(/^www\./, ''); }
    catch { return url; }
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
  }
</script>

<div class="profile">
  {#if loading}
    <div class="profile-loading">
      <div class="skel skel-avatar"></div>
      <div class="skel" style="width: 200px; height: 28px; margin: 12px auto 0;"></div>
      <div class="skel" style="width: 160px; height: 16px; margin: 8px auto 0;"></div>
    </div>
  {:else if error}
    <div class="profile-error">
      <h2>Person not found</h2>
      <p class="muted">{error}</p>
      <a href="/" class="btn btn-secondary" onclick={(e) => { e.preventDefault(); navigate('/'); }}>Back to Quilt</a>
    </div>
  {:else if profile}
    <!-- Header -->
    <div class="profile-header">
      <div class="profile-avatar">
        {#if profile.avatar_url}
          <img src={profile.avatar_url} alt="" />
        {:else}
          {(profile.display_name || profile.username || '?')[0].toUpperCase()}
        {/if}
      </div>
      <h1 class="profile-name">{profile.display_name || profile.username}</h1>
      <p class="profile-username muted">@{profile.username} &middot; here since {formatDate(profile.created_at)}</p>
      {#if profile.bio}
        <p class="profile-bio">{profile.bio}</p>
      {/if}
      {#if isSelf}
        <a href="/settings" class="btn btn-secondary btn-edit" onclick={(e) => { e.preventDefault(); navigate('/settings'); }}>
          Edit profile
        </a>
      {:else if profile.id}
        <div class="profile-report">
          <ReportButton entityType="user" entityId={profile.id} entityName={profile.display_name || profile.username} />
        </div>
      {/if}
    </div>

    <!-- Links -->
    {#if profile.links && profile.links.length > 0}
      <section class="profile-section">
        <h3 class="section-title">Links</h3>
        <div class="link-list">
          {#each profile.links as link}
            <a href={link.url} class="about-link" target="_blank" rel="noopener">
              {link.label || extractDomain(link.url)}
            </a>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Patches: the contributor ladder, legible (docs/adr/006) -->
    {#if adminOf.length > 0}
      <section class="profile-section">
        <h3 class="section-title">Runs</h3>
        <div class="patch-list">
          {#each adminOf as m (m.node_id)}
            <a href="/patches/{m.node_slug}" class="patch-item" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
              <span class="patch-item-name">{m.node_name}</span>
              <span class="badge badge-admin">admin</span>
            </a>
          {/each}
        </div>
      </section>
    {/if}

    {#if memberOf.length > 0}
      <section class="profile-section">
        <h3 class="section-title">Member of</h3>
        <div class="patch-list">
          {#each memberOf as m (m.node_id)}
            <a href="/patches/{m.node_slug}" class="patch-item" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
              <span class="patch-item-name">{m.node_name}</span>
              <span class="badge">member</span>
            </a>
          {/each}
        </div>
      </section>
    {/if}

    {#if adminOf.length === 0 && memberOf.length === 0 && !profile.bio && (!profile.links || profile.links.length === 0)}
      <div class="profile-empty">
        <p class="muted">Nothing here yet.</p>
        {#if isSelf}
          <p class="muted">Memberships you've hidden don't appear on your profile. Manage them in <a href="/settings/patches" onclick={(e) => { e.preventDefault(); navigate('/settings/patches'); }}>Settings</a>.</p>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .profile {
    max-width: 560px;
    margin: 0 auto;
    /* Padding comes from SocialShell's .social-main container (issue #17). */
  }

  .profile-loading {
    padding: 3rem 0;
    text-align: center;
  }

  .skel {
    background: var(--color-overlay);
    border-radius: 4px;
  }

  .skel-avatar {
    width: 72px;
    height: 72px;
    border-radius: 50%;
    margin: 0 auto;
  }

  .profile-error {
    text-align: center;
    padding: 3rem 0;
  }

  .profile-error h2 {
    margin-bottom: 0.5rem;
  }

  /* Header */
  .profile-header {
    text-align: center;
    margin-bottom: 1.5rem;
  }

  .profile-avatar {
    width: 72px;
    height: 72px;
    border-radius: 50%;
    margin: 0 auto 0.75rem;
    background: var(--color-overlay);
    color: var(--color-text);
    font-size: 1.6rem;
    font-weight: 700;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
  }

  .profile-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .profile-name {
    font-size: 1.75rem;
    font-weight: 700;
    margin-bottom: 0.25rem;
  }

  .profile-username {
    font-size: 0.85rem;
    margin-bottom: 0.75rem;
  }

  .profile-bio {
    font-size: 0.9rem;
    color: var(--color-text-muted);
    line-height: 1.6;
    max-width: 440px;
    margin: 0 auto;
  }

  .btn-edit {
    margin-top: 1rem;
  }

  .profile-report {
    margin-top: 0.75rem;
  }

  /* Sections */
  .profile-section {
    border-top: 1px solid var(--color-border);
    padding: 1.25rem 0;
  }

  .section-title {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
    margin-bottom: 0.75rem;
    text-align: center;
  }

  .link-list {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .about-link {
    display: block;
    font-size: 0.88rem;
    color: var(--color-primary);
    text-decoration: none;
    padding: 0.2rem 0;
    text-align: center;
  }

  .about-link:hover {
    text-decoration: underline;
  }

  /* Patch memberships */
  .patch-list {
    display: flex;
    flex-direction: column;
  }

  .patch-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.5rem;
    text-decoration: none;
    color: var(--color-text);
    border-radius: var(--radius);
    transition: background 100ms ease;
  }

  .patch-item:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  .patch-item-name {
    font-size: 0.88rem;
    font-weight: 500;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .badge-admin {
    color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 12%, transparent);
  }

  .profile-empty {
    text-align: center;
    padding: 2rem 0;
    border-top: 1px solid var(--color-border);
  }
</style>
