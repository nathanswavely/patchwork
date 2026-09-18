<script>
  import * as d3 from 'd3';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { loadMemberships } from '../stores/memberships.svelte.js';
  import { getSubmissionsEnabled } from '../stores/quilt.svelte.js';
  import TemplatePreviewDrawer from '../components/TemplatePreviewDrawer.svelte';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import TagPicker from '../components/TagPicker.svelte';
  import MapLocationPicker from '../components/MapLocationPicker.svelte';
  import { suggestPlace, worthLookingUp } from '../lib/placeSuggestion.js';
  import { hasMapLocation } from '../lib/mapLocation.js';
  import { MOTIFS, MOTIF_KEYS } from '../lib/patchIcons.js';
  import { PALETTES, PALETTE_KEYS, paletteForPatch } from '../lib/quiltTheme.js';
  import { BLOCKS, getBlockIndex, getRotation } from '../lib/quiltBlocks.js';
  import { templateMembershipPolicy } from '../lib/governanceTemplates.js';

  // Patch setup (docs/adr/039) reuses this exact form: a claim is creation
  // with prepopulated fields, not a handoff. mode='setup' prepopulates
  // from the existing unclaimed listing (`initial`) instead of starting
  // blank, locks the slug, and posts to the claim's setup endpoint before
  // saving the (possibly edited) fields — everything else about the form,
  // including the lining presentation below, is identical to creation.
  let {
    mode = 'create',
    slug: setupSlug = '',
    claimId = '',
    expiresAt = '',
    initial = null,
  } = $props();

  // The fork (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar):
  // ordinary creation opens on "Is this your patch to run?" before any
  // field, so nothing promises admin of a place nobody has admitted the
  // visitor to. Claim setup skips it — a claimant has already answered by
  // claiming. Answered in component state only, never persisted: it is a
  // question about this visit, not a standing preference.
  let readyToCreate = $state(mode === 'setup');

  let name = $state(initial?.name || '');
  let description = $state(initial?.description || '');
  let address = $state(initial?.address || '');
  let website = $state(initial?.website || '');

  // Map location, offered here rather than only in settings (docs/adr/082).
  // The address and the marker stay separate acts: leaving the address field
  // opens the picker and may propose a marker, and the patch is created with
  // no coordinates unless somebody confirms one. Accepting by submitting the
  // form would make the point derived rather than placed, which is the thing
  // the ADR exists to prevent.
  let latitude = $state(initial?.latitude ?? null);
  let longitude = $state(initial?.longitude ?? null);
  let showPicker = $state(hasMapLocation(initial?.latitude, initial?.longitude));
  let suggestion = $state(null);
  let lookingUp = $state(false);
  let lastLookedUp = '';
  let mapCenter = $state(null);
  let placed = $derived(hasMapLocation(latitude, longitude));

  api('instance')
    .then((inst) => {
      if (inst?.geography?.latitude != null && inst?.geography?.longitude != null) {
        mapCenter = { lat: inst.geography.latitude, lng: inst.geography.longitude };
      }
    })
    .catch(() => {});

  // Fired when the address field loses focus, not on every keystroke: a map
  // that materializes mid-typing shoves every control below it down the page
  // while somebody is still using one.
  //
  // The same shove happens on blur when the blur *is* a press: mousedown on
  // Create Patch blurs the address field, the picker (320px of map) lands
  // above the button, and mouseup arrives on the map instead — the press is
  // swallowed. So while a pointer is down, the reveal waits for it to come
  // up, plus a tick so the click has dispatched against the layout it
  // started on. The suggestion itself is unchanged: it is still only a
  // proposal until somebody confirms it (docs/adr/082).
  let pointerHeld = false;

  $effect(() => {
    const down = () => { pointerHeld = true; };
    const up = () => { pointerHeld = false; };
    // Capture phase, so the flag is set before the press blurs anything.
    window.addEventListener('pointerdown', down, true);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
    return () => {
      window.removeEventListener('pointerdown', down, true);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
    };
  });

  function pointerReleased() {
    return new Promise((resolve) => {
      const done = () => {
        window.removeEventListener('pointerup', done);
        window.removeEventListener('pointercancel', done);
        setTimeout(resolve, 0);
      };
      window.addEventListener('pointerup', done);
      window.addEventListener('pointercancel', done);
    });
  }

  async function addressSettled() {
    const q = address.trim();
    if (!worthLookingUp(q)) return;
    if (q === lastLookedUp) return;
    lastLookedUp = q;
    if (pointerHeld) await pointerReleased();
    showPicker = true;
    // A marker the person already placed is theirs; never propose over it.
    if (placed) return;
    lookingUp = true;
    try {
      suggestion = await suggestPlace(q);
    } finally {
      lookingUp = false;
    }
  }

  function confirmPlacement(lat, lng) {
    latitude = lat;
    longitude = lng;
    suggestion = null;
  }

  function clearPlacement() {
    latitude = null;
    longitude = null;
    suggestion = null;
    showPicker = false;
  }
  let visibility = $state(initial?.visibility || 'public');
  // The membership policy, preselected to the closed option.
  //
  // This has been wrong twice in opposite directions. First the create form
  // never asked at all, and the API's default made the patch open. F-008
  // answered that by starting unset, so the person had to choose. That fixed
  // the right bug the wrong way round: unset weights the three options
  // equally and the first card reads as the ordinary pick, while the template
  // list below *is* preselected, at Minimal, whose own rules say invite_only.
  // So the form pre-answered one question and left the other looking like a
  // choice between equals. On the reference instance a five-minute-old
  // account took the first card and got a patch anyone could join.
  //
  // Preselected is not the same as unasked. The question is still on the
  // screen, still required, and one click changes it. What moved is which way
  // it fails when nobody engages with it, and closed is the safe direction: a
  // patch that should have been open is a setting away, and a patch that
  // should have been closed has already admitted people.
  //
  // Setup (docs/adr/039) asks the same question, seeded from the chosen
  // template rather than fixed here, because a claimant picking a template is
  // already saying what kind of patch this is. See policySeeded below.
  let membershipPolicy = $state('invite_only');
  const membershipPolicies = [
    { id: 'invite_only', name: 'Invite only', desc: 'Only people an admin invites can join.' },
    { id: 'approval_required', name: 'Approval required', desc: 'Anyone can ask to join; an admin approves each request.' },
    { id: 'open', name: 'Open', desc: 'Anyone can join without approval.' },
  ];
  // Minimal is the default (docs/adr/041): the typical new patch is one
  // person running a listing; ceremony is opted into, not inherited.
  let template = $state('minimal');

  // In setup mode the policy follows the template until the claimant answers
  // the question themselves, after which it is theirs and nothing moves it.
  // Each template's rules file already states a membership policy, so the
  // seed is that template's own answer rather than a fourth opinion — and
  // the server falls back to the same value when a client sends no policy,
  // so the form and the API agree about what Minimal means. Creation is
  // untouched: it starts blank and refuses to submit unanswered.
  let policyAnswered = $state(false);
  let seededPolicy = $derived(templateMembershipPolicy(template));
  let policySeeded = $derived(mode === 'setup' && !policyAnswered && !!seededPolicy);
  $effect(() => {
    if (policySeeded) membershipPolicy = seededPolicy;
  });
  // Tags, in priority order — the first motif-bearing tag derives the
  // motif, and shared tags place new patches near their kind on the quilt.
  let tags = $state(Array.isArray(initial?.tags) ? [...initial.tags] : []);
  // Words that are not in the vocabulary yet (docs/adr/114). They travel in
  // their own field so an unknown name in `tags` stays an error.
  let suggestTags = $state([]);

  // Tile appearance. At creation the form always shows a concrete pick —
  // seeded randomly so every new patch starts somewhere real — and
  // creation pins it. In setup mode it instead seeds from the listing's
  // current effective appearance (chosen or hash-assigned) so the
  // claimant sees what's already there rather than a reroll. Motif stays
  // '' (auto-derived from tags) unless the listing has an explicit pick.
  // Drafting your own block and swapping fabrics live in Patch Settings →
  // Appearance after creation/setup.
  const seededAppearance = (() => {
    if (mode !== 'setup' || !initial) {
      return {
        palette: PALETTE_KEYS[Math.floor(Math.random() * PALETTE_KEYS.length)],
        blockKey: BLOCKS[Math.floor(Math.random() * BLOCKS.length)].key,
        rotation: [0, 90, 180, 270][Math.floor(Math.random() * 4)],
        motif: '',
      };
    }
    const ap = initial.appearance || null;
    // raw: see PatchSettingsAppearance — a fabric picker shows real fabric,
    // whatever register the viewer reads the quilt in (docs/adr/112).
    const pal = paletteForPatch(initial.id, ap, { raw: true });
    return {
      palette: pal.paletteKey || PALETTE_KEYS[0],
      blockKey: BLOCKS[getBlockIndex(initial.id, ap)].key,
      rotation: getRotation(initial.id, ap),
      motif: ap?.icon && MOTIFS[ap.icon] ? ap.icon : '',
    };
  })();
  // Motif is optional: '' means it is derived from the first motif-bearing
  // tag, falling back to the quilt mark.
  let motif = $state(seededAppearance.motif);
  let palette = $state(seededAppearance.palette);
  let blockKey = $state(seededAppearance.blockKey);
  let rotation = $state(seededAppearance.rotation);

  const PREVIEW_SIZE = 84;
  const THUMB_SIZE = 40;
  let previewEl = $state(null);
  let thumbEls = $state({});

  function drawBlock(svgEl, size, key, pal, rot) {
    const svg = d3.select(svgEl);
    svg.selectAll('*').remove();
    const g = svg.append('g');
    if (rot) g.attr('transform', `rotate(${rot}, ${size / 2}, ${size / 2})`);
    const block = BLOCKS.find(b => b.key === key) || BLOCKS[0];
    block.render(g, size, { primary: pal.primary, secondary: pal.secondary, bg: pal.bg });
  }

  $effect(() => {
    if (previewEl) drawBlock(previewEl, PREVIEW_SIZE, blockKey, PALETTES[palette], rotation);
  });

  $effect(() => {
    const pal = PALETTES[palette];
    for (const b of BLOCKS) {
      const el = thumbEls[b.key];
      if (el) drawBlock(el, THUMB_SIZE, b.key, pal, 0);
    }
  });

  let submitting = $state(false);
  // The lining (docs/adr/037): shown at creation because adoption should
  // never be a surprise. Text fetched lazily when the drawer opens.
  let liningOpen = $state(false);
  let lining = $state(null);

  function toggleLining() {
    liningOpen = !liningOpen;
    if (liningOpen && !lining) {
      api('instance/lining').then((l) => { lining = l; }).catch(() => {});
    }
  }
  let error = $state('');
  let previewTemplate = $state('');

  const templates = [
    { id: 'minimal', name: 'Minimal', desc: 'No overhead. You\'re the only one running it.', leadership: 'Maintainer', bestFor: 'Bands, solo artists, pop-up projects' },
    { id: 'casual', name: 'Casual', desc: 'Small group. Majority rules.', leadership: 'Maintainer', bestFor: 'Small collectives, meetups, studios (5\u201320 people)' },
    { id: 'collaborative', name: 'Collaborative', desc: 'Open community, structured process. Trust builds through contribution.', leadership: 'Meritocratic', bestFor: 'Venues, co-ops, makerspaces (20\u2013100 people)' },
    { id: 'formal', name: 'Formal', desc: 'Coalition-scale governance. Elected council, term limits, full accountability.', leadership: 'Elected Council', bestFor: 'Arts districts, coalitions (100+ people)' },
  ];

  function validate() {
    if (!name.trim()) return 'Name is required';
    if (!membershipPolicy) return 'Choose a membership policy';
    return '';
  }

  function formatExpiry(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }

  async function handleSubmit() {
    const validationError = validate();
    if (validationError) {
      error = validationError;
      return;
    }

    error = '';
    submitting = true;

    const appearance = {
      palette,
      block: blockKey,
      rotation,
      ...(motif ? { icon: motif } : {}),
    };

    if (mode === 'setup') {
      // Setup submit is the creation moment (docs/adr/039): the claim's
      // setup endpoint activates the patch, forks governance from the
      // chosen template, and adopts the lining; the (possibly edited)
      // fields save separately. A PATCH failure after activation still
      // lands the claimant on their new patch — the edits just wait in
      // Settings.
      try {
        // The policy travels with the template, not in the PATCH below:
        // setup is where governance is forked, and the rules file is written
        // from this answer. PATCH /nodes does not accept a membership policy
        // at all — it is governance, and governance moves through the rules.
        await api(`claims/${claimId}/setup`, {
          method: 'POST',
          body: { template, membership_policy: membershipPolicy },
        });
      } catch (e) {
        if (e.status === 410) {
          showToast(e.message || 'Your setup window has expired. The patch is claimable again.', 'error');
          navigate(`/patches/${setupSlug}`);
        } else if (e.status === 409) {
          showToast('This patch is no longer claimable.', 'error');
          navigate(`/patches/${setupSlug}`);
        } else {
          error = e.message || 'Failed to complete setup';
          showToast('Something went wrong. Please try again.', 'error');
        }
        submitting = false;
        return;
      }

      try {
        await api(`nodes/${setupSlug}`, {
          method: 'PATCH',
          body: {
            name: name.trim(),
            description: description.trim() || undefined,
            address: address.trim() || undefined,
            website: website.trim() || undefined,
            latitude: placed ? latitude : undefined,
            longitude: placed ? longitude : undefined,
            visibility,
            appearance,
            tags: tags.length > 0 ? tags : undefined,
            suggest_tags: suggestTags.length > 0 ? suggestTags : undefined,
          },
        });
        showToast('This patch is yours', 'success');
      } catch (e) {
        showToast('Patch set up. Finish edits in Patch Settings.', 'info');
      }
      // The claimant is this patch's admin now, but the memberships store
      // still says they belong nowhere, and the zero-membership redirect in
      // App.svelte reads that store. Refresh it before landing.
      await loadMemberships();
      navigate(`/patches/${setupSlug}`);
      submitting = false;
      return;
    }

    try {
      const body = {
        name: name.trim(),
        description: description.trim() || undefined,
        address: address.trim() || undefined,
        website: website.trim() || undefined,
        latitude: placed ? latitude : undefined,
        longitude: placed ? longitude : undefined,
        visibility,
        membership_policy: membershipPolicy,
        template,
        appearance,
        tags: tags.length > 0 ? tags : undefined,
        suggest_tags: suggestTags.length > 0 ? suggestTags : undefined,
      };
      const result = await api('nodes', { method: 'POST', body });
      // A word an admin already declined does not fail the creation; the
      // patch exists and the person is told why the chip is missing.
      if (result?.tag_warning) showToast(result.tag_warning, 'info');
      // Creating a patch makes you its admin, server-side. The memberships
      // store loaded before that row existed, and App.svelte's onboarding
      // redirect sends anyone with zero memberships to /welcome — which is
      // where a founder landed instead of on their patch. Refresh first.
      await loadMemberships();
      showToast('Patch created', 'success');
      navigate(`/patches/${result.slug}`);
    } catch (e) {
      error = e.message || 'Failed to create patch';
      showToast('Something went wrong. Please try again.', 'error');
    } finally {
      submitting = false;
    }
  }
</script>

<div class="page-fade">
  <div class="container-narrow">
    <div>
      {#if mode === 'create' && !readyToCreate}
        <h1>Add a patch</h1>
        <p class="fork-question">Is this your patch to run?</p>
        <div class="fork-cards">
          <button type="button" class="fork-card" onclick={() => { readyToCreate = true; }}>
            <strong>I run this patch</strong>
            <span>You become its admin. Members, events and settings are yours from the start.</span>
          </button>
          <button type="button" class="fork-card" onclick={() => navigate('/submit')}>
            <strong>Someone else runs it</strong>
            <span>It joins the quilt as an unclaimed patch. The people who run it can claim it later, and you can add its events once it's approved.</span>
          </button>
        </div>
      {:else}
      {#if mode === 'setup'}
        <h1>Set up your patch</h1>
        <p class="muted" style="margin-bottom: 0.35rem;">Complete this patch's details to activate it.</p>
        {#if expiresAt}
          <p class="muted setup-expiry" style="margin-bottom: 1.5rem;">Approval expires {formatExpiry(expiresAt)}.</p>
        {/if}
      {:else}
        <h1>Create Patch</h1>
        <p class="muted" style="margin-bottom: 1.5rem;">Start a new community, collective, venue, or group.</p>
        {#if getSubmissionsEnabled()}
          <p class="muted" style="margin-bottom: 1.5rem;">Not yours to run? <a href="/submit" class="suggest-link" onclick={(e) => { e.preventDefault(); navigate('/submit'); }}>Suggest it instead</a></p>
        {/if}
      {/if}

      <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
        {#if mode === 'setup'}
          <div class="field">
            <label for="setup-slug">Address</label>
            <input id="setup-slug" type="text" value={setupSlug} disabled readonly />
            <span class="field-hint muted">This is the patch's existing address.</span>
          </div>
        {/if}

        <div class="field">
          <label for="name">Name <span class="required">*</span></label>
          <input id="name" type="text" bind:value={name} disabled={submitting} required placeholder="e.g. Gallery Row, Lancaster Beats Lab" />
        </div>

        <div class="field">
          <label for="description">Description</label>
          <textarea id="description" bind:value={description} rows="4" disabled={submitting} placeholder="What is this patch about?"></textarea>
        </div>

        <div class="field">
          <!-- "Location" names an event's venue and never this field
               (CONTEXT.md, docs/adr/046). The patch's own prose field is
               the Address, which is what Patch Settings calls it too. -->
          <label for="address">Address</label>
          <input
            id="address"
            type="text"
            bind:value={address}
            disabled={submitting}
            placeholder="Where is this based?"
            onblur={addressSettled}
          />

          {#if placed}
            <p class="place-state">
              On the map. <button type="button" class="link-btn" onclick={clearPlacement}>Remove</button>
            </p>
          {:else if lookingUp}
            <p class="place-state muted">Looking for this address...</p>
          {/if}

          {#if showPicker && !placed}
            <MapLocationPicker
              lat={latitude}
              lng={longitude}
              center={mapCenter}
              {suggestion}
              onSave={confirmPlacement}
              onCancel={clearPlacement}
            />
          {/if}
        </div>

        <div class="field">
          <label for="website">Website</label>
          <input id="website" type="url" bind:value={website} disabled={submitting} placeholder="https://..." />
        </div>

        <div class="field">
          <label>Tags</label>
          <p class="field-hint muted">
            What kind of patch is this? Tags help people find you, and new
            patches are placed near others with the same tags on the quilt.
            The first tag decides your default motif. Missing a word? Suggest
            it, and an admin decides whether it joins the quilt's tags.
          </p>
          <TagPicker bind:selected={tags} bind:suggested={suggestTags} disabled={submitting} />
        </div>

        <div class="field">
          <label>Tile</label>
          <p class="field-hint muted">
            How this patch looks on the quilt. You can change it anytime
            in Patch Settings → Appearance, and draft your own block there too.
          </p>
          <div class="tile-picker">
            <div class="tile-preview-col">
              <svg
                bind:this={previewEl}
                class="tile-preview"
                viewBox="0 0 {PREVIEW_SIZE} {PREVIEW_SIZE}"
                width={PREVIEW_SIZE}
                height={PREVIEW_SIZE}
                role="img"
                aria-label="Tile preview"
              ></svg>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                onclick={() => { rotation = (rotation + 90) % 360; }}
                disabled={submitting}
                title="Rotate 90°"
              >
                Rotate
              </button>
            </div>
            <div class="tile-choices">
              <div class="palette-row" role="group" aria-label="Palette">
                {#each PALETTE_KEYS as key (key)}
                  {@const p = PALETTES[key]}
                  <button
                    type="button"
                    class="palette-chip"
                    class:selected={palette === key}
                    onclick={() => { palette = key; }}
                    disabled={submitting}
                    title="{p.name}: {p.subtitle}"
                    aria-label={p.name}
                    aria-pressed={palette === key}
                  >
                    <span style="background: {p.primary}"></span>
                    <span style="background: {p.secondary}"></span>
                    <span style="background: {p.bg}"></span>
                  </button>
                {/each}
              </div>
              <div class="block-row" role="group" aria-label="Block">
                {#each BLOCKS as block (block.key)}
                  <button
                    type="button"
                    class="block-chip"
                    class:selected={blockKey === block.key}
                    onclick={() => { blockKey = block.key; }}
                    disabled={submitting}
                    title={block.name}
                    aria-label={block.name}
                    aria-pressed={blockKey === block.key}
                  >
                    <svg
                      bind:this={thumbEls[block.key]}
                      viewBox="0 0 {THUMB_SIZE} {THUMB_SIZE}"
                      width={THUMB_SIZE}
                      height={THUMB_SIZE}
                      role="img"
                      aria-label={block.name}
                    ></svg>
                  </button>
                {/each}
              </div>
            </div>
          </div>
        </div>

        <div class="field">
          <label>Motif</label>
          <p class="field-hint muted">
            Optional. The mark that appears beside the name on the quilt.
            Unset, it follows your first tag. You can change it later in
            Patch Settings.
          </p>
          <div class="motif-grid">
            {#each MOTIF_KEYS as key (key)}
              {@const m = MOTIFS[key]}
              {@const MotifIcon = m.component}
              <button
                type="button"
                class="motif-swatch"
                class:selected={motif === key}
                onclick={() => { motif = motif === key ? '' : key; }}
                disabled={submitting}
                title={m.name}
                aria-pressed={motif === key}
                aria-label={m.name}
              >
                <MotifIcon size={18} weight="fill" />
              </button>
            {/each}
          </div>
        </div>

        <div class="field">
          <label>The lining</label>
          <p class="field-hint muted">
            Every patch starts with a shared community standards called the lining. It is always public, and if your patch amends it, the changes are public and the patch is marked as having amended the lining.
          </p>
          <button type="button" class="lining-toggle" onclick={toggleLining}>
            {liningOpen ? 'Hide the lining' : 'Read the lining'}
          </button>
          {#if liningOpen}
            <div class="lining-text">
              {#if lining}
                <MarkdownRenderer content={lining.body} />
              {:else}
                <span class="muted">Loading...</span>
              {/if}
            </div>
          {/if}
        </div>

        <fieldset class="field policy-field">
          <legend>Membership Policy <span class="required">*</span></legend>
          <!-- The distinction the form otherwise never mentions. Joining and
               following are different relationships, and only one of them is
               what this setting governs — so somebody can pick Open reasoning
               that people need it to see the patch at all, which is the one
               thing it has nothing to do with. -->
          <p class="field-hint muted">
            Members vote on proposals and appear in the patch's member list.
            Following is separate and always open: anyone can follow a public
            patch and see its events.
          </p>
          {#if policySeeded}
            <!-- What the control cannot show: that this answer came from the
                 template below and will keep following it until it is
                 answered here. Without the line, picking a template silently
                 moves an answer further up the page. -->
            <p class="field-hint muted">Set by the {templates.find((t) => t.id === template)?.name || template} template. Change it if that is not this patch.</p>
          {/if}
          <div class="policy-grid">
            {#each membershipPolicies as p (p.id)}
              <label class="policy-card" class:selected={membershipPolicy === p.id}>
                <input type="radio" name="membership_policy" value={p.id} bind:group={membershipPolicy} onchange={() => policyAnswered = true} disabled={submitting} required />
                <span class="policy-info">
                  <strong>{p.name}</strong>
                  <span class="policy-desc">{p.desc}</span>
                </span>
              </label>
            {/each}
          </div>
        </fieldset>

        <div class="field">
          <label>Governance Template</label>
          <p class="field-hint muted">How should this patch be organized? You can change this later.</p>
          <div class="template-grid">
            {#each templates as t}
              <label class="template-card" class:selected={template === t.id}>
                <input type="radio" name="template" value={t.id} bind:group={template} disabled={submitting} />
                <div class="template-info">
                  <div class="template-top">
                    <strong>{t.name}</strong>
                    <button type="button" class="preview-link" onclick={(e) => { e.preventDefault(); e.stopPropagation(); previewTemplate = t.id; }}>Preview</button>
                  </div>
                  <span class="template-desc">{t.desc}</span>
                  <span class="template-meta">{t.leadership} · {t.bestFor}</span>
                </div>
              </label>
            {/each}
          </div>
        </div>

        {#if error}
          <p class="error-text">{error}</p>
        {/if}

        <div class="field-actions">
          <button type="submit" class="btn btn-primary" disabled={submitting}>
            {#if mode === 'setup'}
              {submitting ? 'Finishing...' : 'Finish setup'}
            {:else}
              {submitting ? 'Creating...' : 'Create Patch'}
            {/if}
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            onclick={() => navigate(mode === 'setup' ? `/patches/${setupSlug}` : '/dashboard')}
          >
            Cancel
          </button>
        </div>
      </form>
      {/if}
    </div>
  </div>
</div>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape' && previewTemplate) previewTemplate = ''; }} />

{#if previewTemplate}
  <TemplatePreviewDrawer templateId={previewTemplate} onClose={() => previewTemplate = ''} />
{/if}

<style>
  h1 {
    margin-bottom: 0.25rem;
  }

  .fork-question {
    color: var(--color-text-muted);
    margin-bottom: 1.25rem;
  }

  .fork-cards {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .fork-card {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    align-items: flex-start;
    text-align: left;
    padding: 1rem 1.1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .fork-card:hover,
  .fork-card:focus-visible {
    border-color: var(--color-primary);
  }

  .fork-card strong {
    font-size: 0.95rem;
  }

  .fork-card span {
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  @media (min-width: 640px) {
    .fork-cards {
      flex-direction: row;
    }

    .fork-card {
      flex: 1;
    }
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
    flex: 1;
  }

  .place-state {
    font-size: 0.82rem;
    margin: 0.4rem 0 0;
  }

  .link-btn {
    border: none;
    background: none;
    padding: 0;
    font: inherit;
    color: var(--color-primary);
    cursor: pointer;
    text-decoration: underline;
  }

  .field label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .required {
    color: var(--color-error);
  }

  textarea {
    resize: vertical;
    min-height: 80px;
  }

  .field-row {
    display: flex;
    gap: 1rem;
  }

  .field-hint {
    font-size: 0.8rem;
    margin-bottom: 0.5rem;
  }

  .tile-picker {
    display: flex;
    gap: 0.85rem;
    align-items: flex-start;
  }

  .tile-preview-col {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.35rem;
    flex-shrink: 0;
  }

  .tile-preview {
    border-radius: 4px;
    border: 2px solid var(--lt-thread, var(--color-border));
    display: block;
  }

  .tile-choices {
    min-width: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .palette-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .palette-chip {
    display: flex;
    width: 44px;
    height: 26px;
    border: 2px solid var(--color-border);
    border-radius: 4px;
    overflow: hidden;
    padding: 0;
    cursor: pointer;
    transition: border-color 120ms ease;
  }

  .palette-chip span {
    flex: 1;
  }

  .palette-chip:hover {
    border-color: var(--color-text-muted);
  }

  .palette-chip.selected {
    border-color: var(--color-primary);
  }

  .block-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .block-chip {
    display: flex;
    padding: 2px;
    border: 2px solid var(--color-border);
    border-radius: 4px;
    background: var(--color-surface);
    cursor: pointer;
    transition: border-color 120ms ease;
  }

  .block-chip svg {
    border-radius: 2px;
    display: block;
  }

  .block-chip:hover {
    border-color: var(--color-text-muted);
  }

  .block-chip.selected {
    border-color: var(--color-primary);
  }

  @media (max-width: 640px) {
    .tile-picker {
      flex-direction: column;
    }
  }

  .motif-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(40px, 1fr));
    gap: 0.35rem;
  }

  .motif-swatch {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.45rem;
    border: 2px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    color: var(--color-text);
    cursor: pointer;
    transition: border-color 120ms ease;
  }

  .motif-swatch:hover {
    border-color: var(--color-text-muted);
  }

  .motif-swatch.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 10%, var(--color-surface));
  }

  .policy-field {
    border: none;
    padding: 0;
    margin: 0;
    min-width: 0;
  }

  .policy-field legend {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
    padding: 0;
    margin-bottom: 0.25rem;
  }

  .policy-grid {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .policy-card {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .policy-card:hover {
    border-color: var(--color-primary);
  }

  .policy-card.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 5%, var(--color-surface));
  }

  .policy-card input[type="radio"] {
    margin-top: 0.15rem;
    flex-shrink: 0;
  }

  .policy-info {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
  }

  .policy-info strong {
    font-size: 0.9rem;
  }

  .policy-desc {
    font-size: 0.82rem;
    color: var(--color-text-muted);
  }

  .template-grid {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .template-card {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    padding: 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease;
  }

  .template-card:hover {
    border-color: var(--color-primary);
  }

  .template-card.selected {
    border-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 5%, var(--color-surface));
  }

  .template-card input[type="radio"] {
    margin-top: 0.15rem;
    flex-shrink: 0;
  }

  .template-info {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    /* Fill the card so .template-top's space-between actually reaches the
       right edge — without this the column shrink-wraps and Preview hugs
       the title. */
    flex: 1;
    min-width: 0;
  }

  .template-top {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .template-info strong {
    font-size: 0.9rem;
  }

  .preview-link {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: 0.78rem;
    cursor: pointer;
    padding: 0;
    text-decoration: none;
  }

  .preview-link:hover {
    text-decoration: underline;
  }

  .template-desc {
    font-size: 0.82rem;
    color: var(--color-text-muted);
  }

  .template-meta {
    font-size: 0.72rem;
    color: var(--color-text-muted);
    margin-top: 0.15rem;
  }

  .field-actions {
    display: flex;
    gap: 0.75rem;
    padding-top: 0.5rem;
  }

  .suggest-link {
    font-weight: 600;
    color: var(--color-primary);
    text-decoration: none;
  }

  .suggest-link:hover {
    text-decoration: underline;
  }

  @media (max-width: 640px) {
    .field-row {
      flex-direction: column;
    }
  }
  .lining-toggle {
    align-self: flex-start;
    border: none;
    background: none;
    padding: 0;
    font-size: 0.85rem;
    color: var(--color-primary);
    text-decoration: underline;
    cursor: pointer;
  }

  .lining-text {
    margin-top: 0.6rem;
    padding: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-bg);
    font-size: 0.88rem;
    line-height: 1.7;
    max-height: 320px;
    overflow-y: auto;
  }

</style>
