<script>
  import {
    addRoute,
    matchRoute,
    navigate,
    replaceRoute,
    getPath,
    getQuery,
  } from './stores/router.svelte.js';
  import { checkAuth, isLoggedIn, isAdmin, getUser, logout } from './stores/auth.svelte.js';
  import { isMembershipsLoaded, getMemberships } from './stores/memberships.svelte.js';
  import { getPatchName } from './stores/patchName.svelte.js';
  import {
    setRegistryQuilts, loadMultiQuilt, clearMultiQuilt,
  } from './stores/multiQuilt.svelte.js';
  import { loadInstance, loadTags } from './stores/quilt.svelte.js';
  import { loadRegistry } from './lib/registry.js';
  import { isOnboardingDismissed } from './lib/onboarding.js';
  import { markFamiliar } from './lib/vocab.js';

  // Layout shells
  import SocialShell from './components/SocialShell.svelte';
  import AdminShell from './components/AdminShell.svelte';
  import UserSettingsShell from './components/UserSettingsShell.svelte';

  // Social mode pages
  import SocialHome from './pages/SocialHome.svelte';
  import PatchProfile from './pages/PatchProfile.svelte';
  import RemotePatch from './pages/RemotePatch.svelte';
  import UserProfile from './pages/UserProfile.svelte';
  import EventsPage from './pages/EventsPage.svelte';
  import EventDetail from './pages/EventDetail.svelte';
  import PatchShell from './components/PatchShell.svelte';
  import PatchMembers from './pages/PatchMembers.svelte';
  import PatchEvents from './pages/PatchEvents.svelte';
  import EventForm from './pages/EventForm.svelte';
  import PatchForm from './pages/PatchForm.svelte';
  import ProposalList from './pages/ProposalList.svelte';
  import ProposalForm from './pages/ProposalForm.svelte';
  import ProposalDetail from './pages/ProposalDetail.svelte';
  import GovernanceList from './pages/GovernanceList.svelte';
  import GovernanceDetail from './pages/GovernanceDetail.svelte';
  import GovernanceEdit from './pages/GovernanceEdit.svelte';
  import GovernanceVersionHistory from './pages/GovernanceVersionHistory.svelte';
  import GovernanceHub from './pages/GovernanceHub.svelte';
  import PatchSettings from './pages/PatchSettings.svelte';
  import AccountSettings from './pages/AccountSettings.svelte';
  import SecuritySettings from './pages/SecuritySettings.svelte';
  import QuiltsSettings from './pages/QuiltsSettings.svelte';
  import UserSettingsPatches from './pages/UserSettingsPatches.svelte';
  import Dashboard from './pages/Dashboard.svelte';
  import AdminDashboard from './pages/AdminDashboard.svelte';
  import AdminReports from './pages/AdminReports.svelte';
  import AdminTags from './pages/AdminTags.svelte';
  import AdminUsers from './pages/AdminUsers.svelte';
  import AdminAuditLog from './pages/AdminAuditLog.svelte';
  import AdminSubmissions from './pages/AdminSubmissions.svelte';
  import AdminEventSubmissions from './pages/AdminEventSubmissions.svelte';
  import AdminClaims from './pages/AdminClaims.svelte';
  import AdminQuiltSettings from './pages/AdminQuiltSettings.svelte';
  import AdminNeighborQuilts from './pages/AdminNeighborQuilts.svelte';
  import AdminLabel from './pages/AdminLabel.svelte';
  import Label from './pages/Label.svelte';
  import AdminLegal from './pages/AdminLegal.svelte';
  import LegalDoc from './pages/LegalDoc.svelte';
  import SubmitPatch from './pages/SubmitPatch.svelte';
  import ClaimPatch from './pages/ClaimPatch.svelte';
  import ClaimVerifyEmail from './pages/ClaimVerifyEmail.svelte';
  import AmendmentEditor from './pages/AmendmentEditor.svelte';
  import RulesProposalEditor from './pages/RulesProposalEditor.svelte';
  import Notifications from './pages/Notifications.svelte';
  import Activity from './pages/Activity.svelte';
  import NotificationPreferences from './pages/NotificationPreferences.svelte';
  import PatchSettingsNotifications from './pages/PatchSettingsNotifications.svelte';
  import Login from './pages/Login.svelte';
  import InviteLanding from './pages/InviteLanding.svelte';
  import SignupComplete from './pages/SignupComplete.svelte';
  import Welcome from './pages/Welcome.svelte';
  import Toast from './components/Toast.svelte';

  // --- Routes ---
  // Home / discovery
  addRoute('/', 'home');
  addRoute('/patches', 'patchList');
  addRoute('/map', 'map');
  addRoute('/events', 'eventList');
  // /events/new and /:id/edit must register before /events/:id — first match
  // wins, and ':id' would swallow "new".
  addRoute('/events/new', 'eventNew');
  addRoute('/events/:id/edit', 'eventEdit');
  addRoute('/events/:id', 'eventDetail');

  // Community submissions
  addRoute('/submit', 'submitPatch');

  // The Label (docs/adr/023) — public, readable logged out.
  addRoute('/label', 'label');

  // Legal documents (docs/adr/028) — public, readable logged out.
  addRoute('/privacy', 'privacy');
  addRoute('/terms', 'terms');

  // Patch routes
  addRoute('/patches/new', 'patchNew');
  addRoute('/patches/:slug/claim', 'claimPatch');

  // Email-claim link landing (docs/adr/030) — public: the token is the
  // proof, and the click may happen in a browser with no session.
  addRoute('/claims/verify-email', 'claimVerifyEmail');

  // Social profile
  addRoute('/patches/:slug', 'patchProfile');

  // A patch on another quilt, rendered read-only in-app (docs/adr/024).
  addRoute('/quilts/:host/patches/:slug', 'remotePatch');

  // User profile — public page (docs/adr/006). Also the landing target of
  // the backend's /ap/users/{id} browser redirect.
  addRoute('/users/:username', 'userProfile');

  // Patch shell — canonical URLs (ADR 003: governance is participation,
  // settings is administration; there is exactly one URL per screen).
  addRoute('/patches/:slug/governance/new', 'governanceProposalNew');
  addRoute('/patches/:slug/governance/rules/propose', 'governanceRulesPropose');
  addRoute('/patches/:slug/governance/docs/new', 'governanceDocNew');
  addRoute('/patches/:slug/governance/docs/:id/propose', 'governanceDocPropose');
  addRoute('/patches/:slug/governance/docs/:id/history', 'governanceDocHistory');
  addRoute('/patches/:slug/governance/docs/:id', 'governanceDocDetail');
  addRoute('/patches/:slug/governance/docs', 'governanceDocs');
  addRoute('/patches/:slug/governance/proposals', 'governanceProposals');
  addRoute('/patches/:slug/governance/:id', 'governanceProposal');
  addRoute('/patches/:slug/governance', 'governanceHub');
  addRoute('/patches/:slug/members', 'patchMembers');
  addRoute('/patches/:slug/events', 'patchEvents');
  addRoute('/patches/:slug/settings/info', 'patchSettingsInfo');
  addRoute('/patches/:slug/settings/appearance', 'patchSettingsAppearance');
  addRoute('/patches/:slug/settings/members', 'patchSettingsMembers');
  addRoute('/patches/:slug/settings/notifications', 'patchSettingsNotifications');
  addRoute('/patches/:slug/settings/danger', 'patchSettingsDanger');
  addRoute('/patches/:slug/settings', 'patchSettings');

  // Retired URL schemes (ADR 003) — redirect to canonical, keep old links alive.
  addRoute('/patches/:slug/manage', 'redirectManage');
  addRoute('/patches/:slug/manage/*', 'redirectManageDeep');
  addRoute('/patches/:slug/charters', 'redirectCharters');
  addRoute('/patches/:slug/charters/*', 'redirectChartersDeep');
  addRoute('/patches/:slug/proposals', 'redirectProposals');
  addRoute('/patches/:slug/proposals/new', 'redirectProposalNew');
  addRoute('/patches/:slug/proposals/:id', 'redirectProposalDetail');
  addRoute('/patches/:slug/governance-setup', 'redirectGovernanceSetup');

  // Auth + onboarding
  addRoute('/login', 'login');
  addRoute('/invite/:token', 'invite');
  addRoute('/signup/complete', 'signupComplete');
  addRoute('/welcome', 'welcome');

  // User pages
  addRoute('/settings', 'settings');
  addRoute('/settings/notifications', 'settingsNotifications');
  addRoute('/settings/security', 'settingsSecurity');
  addRoute('/settings/patches', 'settingsPatches');
  addRoute('/settings/quilts', 'quilts');
  addRoute('/notifications', 'notifications');
  addRoute('/activity', 'activity');
  addRoute('/dashboard', 'dashboard');

  // Admin
  addRoute('/admin', 'adminDashboard');
  addRoute('/admin/reports', 'adminReports');
  addRoute('/admin/tags', 'adminTags');
  addRoute('/admin/users', 'adminUsers');
  addRoute('/admin/audit', 'adminAudit');
  addRoute('/admin/submissions', 'adminSubmissions');
  addRoute('/admin/event-submissions', 'adminEventSubmissions');
  addRoute('/admin/claims', 'adminClaims');
  addRoute('/admin/quilt', 'adminQuilt');
  addRoute('/admin/neighbors', 'adminNeighbors');
  addRoute('/admin/label', 'adminLabel');
  addRoute('/admin/legal', 'adminLegal');

  // --- Derived state ---
  let path = $derived(getPath());
  let currentRoute = $derived.by(() => {
    void path;
    return matchRoute();
  });
  let routeName = $derived(currentRoute?.name || 'home');

  // Route categories
  const socialHomeRoutes = new Set([
    'home', 'patchList', 'map',
  ]);
  let isSocialHome = $derived(socialHomeRoutes.has(routeName));

  // Patch shell routes (governance participation + admin settings)
  const patchShellRoutes = new Set([
    'claimPatch',
    'governanceHub', 'governanceProposals', 'governanceProposalNew', 'governanceProposal',
    'governanceDocs', 'governanceDocNew', 'governanceDocDetail', 'governanceDocHistory', 'governanceDocPropose', 'governanceRulesPropose',
    'patchMembers', 'patchEvents',
    'patchSettings', 'patchSettingsInfo', 'patchSettingsAppearance', 'patchSettingsMembers', 'patchSettingsNotifications', 'patchSettingsDanger',
  ]);
  let isPatchShellRoute = $derived(patchShellRoutes.has(routeName));

  const settingsRoutes = new Set(['settings', 'settingsNotifications', 'settingsSecurity', 'settingsPatches', 'quilts']);
  const adminRoutes = new Set(['adminDashboard', 'adminReports', 'adminTags', 'adminUsers', 'adminAudit', 'adminSubmissions', 'adminEventSubmissions', 'adminClaims', 'adminQuilt', 'adminNeighbors', 'adminLabel', 'adminLegal']);
  let isSettingsRoute = $derived(settingsRoutes.has(routeName));
  let isAdminRoute = $derived(adminRoutes.has(routeName));

  // Standalone routes — no shell wrapper
  const standaloneRoutes = new Set(['welcome', 'login', 'invite', 'signupComplete']);
  let isStandaloneRoute = $derived(standaloneRoutes.has(routeName));

  // Social shell wraps everything except standalone pages (admin included —
  // it renders inside the shell so admins keep the global nav and identity).
  let isSocialRoute = $derived(!isStandaloneRoute);

  // Derive active tab for PatchShell
  function derivePatchTab(name) {
    if (name === 'patchMembers') return 'members';
    if (name === 'patchEvents') return 'events';
    if (name.startsWith('patchSettings')) return 'settings';
    return 'governance';
  }
  let patchTab = $derived(derivePatchTab(routeName));

  // Auth-guarded routes.
  let authRequired = $derived(
    ['settings', 'settingsNotifications', 'settingsSecurity', 'settingsPatches', 'notifications', 'activity', 'dashboard', 'submitPatch', 'claimPatch', 'patchNew', 'eventNew', 'eventEdit',
     'governanceProposalNew', 'governanceDocNew',
     'adminDashboard', 'adminReports', 'adminTags', 'adminUsers', 'adminAudit', 'adminSubmissions', 'adminEventSubmissions', 'adminClaims', 'adminQuilt', 'adminNeighbors', 'adminLabel', 'adminLegal'].includes(routeName)
  );

  let quiltScope = $state('local');
  let hasSetDefaultScope = false;

  // Scope is local or my — other quilts are doorways, never a scope
  // (objects blend, places don't — docs/adr/024).
  function changeScope(scope) {
    quiltScope = scope;
  }

  // Default to 'my' scope for authenticated users.
  $effect(() => {
    if (isLoggedIn() && !hasSetDefaultScope) {
      quiltScope = 'my';
      hasSetDefaultScope = true;
    }
    if (!isLoggedIn()) {
      quiltScope = 'local';
      hasSetDefaultScope = false;
    }
  });

  let routeParams = $derived(currentRoute?.params || {});

  // Redirects from retired URL schemes (ADR 003). History-replacing so the
  // canonical URL is what people copy and share. The setup flow is gone
  // entirely, so /governance-setup lands on the governance hub.
  const REDIRECTS = {
    redirectManage: (p) => `/patches/${p.slug}/governance`,
    redirectManageDeep: (p) => `/patches/${p.slug}/${p.rest}`,
    redirectCharters: (p) => `/patches/${p.slug}/governance/docs`,
    redirectChartersDeep: (p) => `/patches/${p.slug}/governance/docs/${p.rest}`,
    redirectProposals: (p) => `/patches/${p.slug}/governance/proposals`,
    redirectProposalNew: (p) => `/patches/${p.slug}/governance/new`,
    redirectProposalDetail: (p) => `/patches/${p.slug}/governance/${p.id}`,
    redirectGovernanceSetup: (p) => `/patches/${p.slug}/governance`,
  };
  $effect(() => {
    const target = REDIRECTS[routeName]?.(routeParams);
    if (target) replaceRoute(target);
  });

  function handleNav(e, target) {
    e.preventDefault();
    navigate(target);
  }

  // --- Init ---
  $effect(() => {
    checkAuth();
    loadInstance();
    loadTags();
    // Progressive vocabulary disclosure: after enough visits, VocabLabel
    // drops the plain-language subtitles (see lib/vocab.js).
    markFamiliar();

    // Registry links are a discovery flyer: a session-only overlay on
    // the switcher, never persisted (docs/adr/024, docs/adr/025).
    const params = new URLSearchParams(window.location.search);
    const registryUrl = params.get('registry');
    if (registryUrl) {
      loadRegistry(registryUrl)
        .then((data) => setRegistryQuilts(data.quilts))
        .catch(() => {});
    }
  });

  // Account-backed cross-quilt state: connected quilts + remote follows.
  let multiQuiltLoadedFor = null;
  $effect(() => {
    const loggedIn = isLoggedIn();
    if (loggedIn && multiQuiltLoadedFor !== getUser()?.id) {
      multiQuiltLoadedFor = getUser()?.id;
      loadMultiQuilt();
    } else if (!loggedIn && multiQuiltLoadedFor) {
      multiQuiltLoadedFor = null;
      clearMultiQuilt();
    }
  });

  // After auth, check for a persisted redirect (from owner claim flow).
  $effect(() => {
    if (isLoggedIn()) {
      const pendingRedirect = localStorage.getItem('patchwork_auth_redirect');
      if (pendingRedirect) {
        localStorage.removeItem('patchwork_auth_redirect');
        navigate(pendingRedirect);
      }
    }
  });

  // Redirect new users (zero memberships) to onboarding — unless they've
  // dismissed it (skip must genuinely exit; on an empty instance there is
  // nothing to join, so without this the redirect softlocks). patchNew is
  // exempt: creating a patch is the natural first act on a fresh instance.
  $effect(() => {
    if (isLoggedIn() && isMembershipsLoaded() && getMemberships().length === 0) {
      if (!['welcome', 'login', 'invite', 'signupComplete', 'claimPatch', 'patchNew'].includes(routeName)
          && !isOnboardingDismissed(getUser()?.id)) {
        navigate('/welcome');
      }
    }
  });
</script>

{#if isStandaloneRoute}
  <!-- ===== STANDALONE PAGES (no shell) ===== -->
  <main class="standalone-main">
    {#if routeName === 'welcome'}
      <Welcome />
    {:else if routeName === 'login'}
      <Login />
    {:else if routeName === 'invite'}
      <InviteLanding />
    {:else if routeName === 'signupComplete'}
      <SignupComplete />
    {/if}
  </main>

{:else if isAdminRoute}
  <!-- ===== ADMIN PANEL: full-screen takeover (docs/adr/005) ===== -->
  {#if authRequired && !isLoggedIn()}
    <main class="takeover-gate">
      <div class="auth-gate">
        <h2>Sign in required</h2>
        <p class="muted" style="margin-bottom: 1rem;">You need to be logged in to view this page.</p>
        <a href="/login" class="btn btn-primary" onclick={(e) => handleNav(e, '/login')}>Log In</a>
      </div>
    </main>
  {:else}
    <AdminShell>
      {#snippet children()}
        {#if routeName === 'adminDashboard'}
          <AdminDashboard />
        {:else if routeName === 'adminReports'}
          <AdminReports />
        {:else if routeName === 'adminTags'}
          <AdminTags />
        {:else if routeName === 'adminUsers'}
          <AdminUsers />
        {:else if routeName === 'adminAudit'}
          <AdminAuditLog />
        {:else if routeName === 'adminSubmissions'}
          <AdminSubmissions />
        {:else if routeName === 'adminEventSubmissions'}
          <AdminEventSubmissions />
        {:else if routeName === 'adminClaims'}
          <AdminClaims />
        {:else if routeName === 'adminLabel'}
          <AdminLabel />
        {:else if routeName === 'adminLegal'}
          <AdminLegal />
        {:else if routeName === 'adminQuilt'}
          <AdminQuiltSettings />
        {:else if routeName === 'adminNeighbors'}
          <AdminNeighborQuilts />
        {/if}
      {/snippet}
    </AdminShell>
  {/if}

{:else if isPatchShellRoute}
  <!-- ===== WORKSPACE: full-screen takeover (docs/adr/005) ===== -->
  {#if authRequired && !isLoggedIn()}
    <main class="takeover-gate">
      <div class="auth-gate">
        <h2>Sign in required</h2>
        <p class="muted" style="margin-bottom: 1rem;">You need to be logged in to view this page.</p>
        <a href="/login" class="btn btn-primary" onclick={(e) => handleNav(e, '/login')}>Log In</a>
      </div>
    </main>
  {:else}
    <PatchShell slug={routeParams.slug} activeTab={patchTab}>
      {#snippet children()}
        {#if routeName === 'governanceHub'}
          <GovernanceHub />
        {:else if routeName === 'governanceDocPropose'}
          <AmendmentEditor />
        {:else if routeName === 'governanceRulesPropose'}
          <RulesProposalEditor />
        {:else if routeName === 'governanceProposals'}
          <ProposalList />
        {:else if routeName === 'governanceProposalNew'}
          <ProposalForm />
        {:else if routeName === 'governanceProposal'}
          <ProposalDetail />
        {:else if routeName === 'governanceDocs'}
          <GovernanceList />
        {:else if routeName === 'governanceDocNew'}
          <GovernanceEdit />
        {:else if routeName === 'governanceDocDetail'}
          <GovernanceDetail />
        {:else if routeName === 'governanceDocHistory'}
          <GovernanceVersionHistory />
        {:else if routeName === 'patchMembers'}
          <PatchMembers />
        {:else if routeName === 'patchEvents'}
          <PatchEvents />
        {:else if routeName === 'claimPatch'}
          <ClaimPatch />
        {:else if routeName.startsWith('patchSettings')}
          <PatchSettings />
        {/if}
      {/snippet}
    </PatchShell>
  {/if}

{:else}
  <!-- ===== SOCIAL SHELL (discovery + personal pages) ===== -->
  <SocialShell {routeName} {quiltScope} onScopeChange={changeScope}>
    {#snippet children()}
      {#if authRequired && !isLoggedIn()}
        <div class="auth-gate">
          <h2>Sign in required</h2>
          <p class="muted" style="margin-bottom: 1rem;">You need to be logged in to view this page.</p>
          <a href="/login" class="btn btn-primary" onclick={(e) => handleNav(e, '/login')}>Log In</a>
        </div>

      <!-- ===== SOCIAL HOME: Quilt + Cards ===== -->
      {:else if isSocialHome}
        <SocialHome
          {quiltScope}
          {routeName}
          onScopeChange={changeScope}
        />

      <!-- ===== EVENTS ===== -->
      {:else if routeName === 'eventList'}
        <EventsPage {quiltScope} />
      {:else if routeName === 'eventDetail'}
        <EventDetail eventId={routeParams.id} />

      <!-- ===== PATCH PROFILE: Social view ===== -->
      {:else if routeName === 'patchProfile'}
        <PatchProfile slug={routeParams.slug} />

      <!-- ===== REMOTE PATCH: read-only view of a patch on another quilt ===== -->
      {:else if routeName === 'remotePatch'}
        <RemotePatch host={routeParams.host} slug={routeParams.slug} />

      <!-- ===== USER PROFILE ===== -->
      {:else if routeName === 'userProfile'}
        <UserProfile username={routeParams.username} />

      <!-- ===== THE LABEL (docs/adr/023) ===== -->
      {:else if routeName === 'label'}
        <Label />

      <!-- ===== LEGAL DOCUMENTS (docs/adr/028) ===== -->
      {:else if routeName === 'privacy' || routeName === 'terms'}
        <LegalDoc doc={routeName} />

      <!-- ===== EMAIL-CLAIM CONFIRMATION (docs/adr/030) ===== -->
      {:else if routeName === 'claimVerifyEmail'}
        <ClaimVerifyEmail />

      <!-- ===== USER SETTINGS ===== -->
      {:else if isSettingsRoute}
        {#if routeName === 'settingsNotifications'}
          <NotificationPreferences />
        {:else}
          <UserSettingsShell>
            {#snippet children()}
              {#if routeName === 'settings'}
                <AccountSettings />
              {:else if routeName === 'settingsSecurity'}
                <SecuritySettings />
              {:else if routeName === 'settingsPatches'}
                <UserSettingsPatches />
              {:else if routeName === 'quilts'}
                <QuiltsSettings />
              {/if}
            {/snippet}
          </UserSettingsShell>
        {/if}

      <!-- ===== STANDALONE SOCIAL PAGES ===== -->
      {:else if routeName === 'notifications'}
        <div class="page-container"><Notifications /></div>
      {:else if routeName === 'activity'}
        <div class="page-container"><Activity /></div>
      {:else if routeName === 'submitPatch'}
        <div class="page-container"><SubmitPatch /></div>
      {:else if routeName === 'dashboard'}
        <div class="page-container"><Dashboard /></div>
      {:else if routeName === 'patchNew'}
        <div class="page-container"><PatchForm /></div>
      {:else if routeName === 'eventNew'}
        <div class="page-container"><EventForm /></div>
      {:else if routeName === 'eventEdit'}
        <div class="page-container"><EventForm eventId={routeParams.id} /></div>

      {:else}
        <div class="auth-gate">
          <h2>Page not found</h2>
          <p class="muted" style="margin-bottom: 1rem;">The page you're looking for doesn't exist.</p>
          <a href="/" class="btn btn-secondary" onclick={(e) => handleNav(e, '/')}>Back to Quilt</a>
        </div>
      {/if}
    {/snippet}
  </SocialShell>
{/if}

<Toast />

<style>
  .standalone-main {
    max-width: 640px;
    margin: 0 auto;
    padding: 0 1rem 2rem;
    min-height: 100vh;
  }

  .takeover-gate {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
  }

  .auth-gate {
    text-align: center;
    padding: 3rem 0;
  }

  /* No padding here — SocialShell's .social-main container owns it (issue #17). */
  .page-container {
  }
</style>
