<script>
  /**
   * The one rendering of a person outside their own page (CONTEXT.md,
   * docs/adr/083). Same card in the Members room, on a notice's replies, and
   * beside an event's organizer.
   *
   * It follows the patch card's gesture rule rather than inventing a second
   * one: where there is a pointer, pointing at a person previews them; where
   * there is not, the first tap opens the card, and the card is how the
   * profile is reached. Keyboard focus opens it too — a card reachable only
   * by hover is reachable only by mouse.
   *
   * The contact section is the one part that differs by who is looking, and
   * it is absent far more often than it is present, so everything above it
   * has to be worth opening on its own.
   */
  import { Heart, UsersThree, Wrench, Phone, EnvelopeSimple, At, Note } from 'phosphor-svelte';
  import { navigate } from '../stores/router.svelte.js';

  let { person, role = '' } = $props();

  let open = $state(false);
  let anchor = $state(null);
  let pos = $state({ top: 0, left: 0 });
  const hasPointer =
    typeof window !== 'undefined' &&
    window.matchMedia('(hover: hover) and (pointer: fine)').matches;

  // CONTEXT.md role marks: heart = follower, three users = member,
  // wrench = admin. Paired with the word, never standing alone.
  const ROLE_MARK = { follower: Heart, member: UsersThree, admin: Wrench };
  const KIND_MARK = { phone: Phone, email: EnvelopeSimple, handle: At, note: Note };
  const KIND_WORD = { phone: 'Phone', email: 'Email', handle: 'Handle', note: 'Note' };

  let items = $derived(person?.contact || []);
  let initial = $derived((person?.display_name || person?.username || '?')[0].toUpperCase());

  function place() {
    if (!anchor) return;
    const r = anchor.getBoundingClientRect();
    // Flip above when the card would fall off the fold; the card is at most
    // 320px tall in practice and clamping beats a card half off-screen.
    const below = window.innerHeight - r.bottom;
    pos = {
      top: below < 260 ? Math.max(8, r.top - 268) : r.bottom + 6,
      left: Math.min(Math.max(8, r.left), Math.max(8, window.innerWidth - 308)),
    };
  }

  function show() {
    place();
    open = true;
  }

  function hide() {
    open = false;
  }

  // The patch card's rule, exactly (CONTEXT.md): where there is a pointer,
  // pointing previews and *clicking opens the person*. Where there is none
  // there is only one gesture, so the first tap opens the card and the card
  // is how the profile is reached. Toggling on click would close, on a
  // pointer device, the card that hovering just opened.
  function activate(e) {
    if (hasPointer) {
      openProfile(e);
      return;
    }
    if (open) hide();
    else show();
  }

  function href(item) {
    if (item.kind === 'phone') return `tel:${item.value.replace(/[^+\d]/g, '')}`;
    if (item.kind === 'email') return `mailto:${item.value}`;
    return null;
  }

  function openProfile(e) {
    e.preventDefault();
    hide();
    navigate(`/users/${person.username}`);
  }
</script>

<svelte:window onscroll={() => open && place()} onresize={() => open && place()} />

<span
  class="person-anchor"
  bind:this={anchor}
  onmouseenter={() => hasPointer && show()}
  onmouseleave={() => hasPointer && hide()}
  role="presentation"
>
  <button
    type="button"
    class="person-trigger"
    aria-expanded={open}
    onclick={activate}
    onfocus={() => hasPointer && show()}
    onblur={() => hasPointer && hide()}
    onkeydown={(e) => { if (e.key === 'Escape' && open) { hide(); e.stopPropagation(); } }}
  >
    {person.display_name || person.username}
  </button>

  {#if open}
    <div class="person-card" style="top: {pos.top}px; left: {pos.left}px;" role="dialog" aria-label="About {person.display_name || person.username}">
      <div class="person-head">
        <span class="person-avatar" aria-hidden="true">
          {#if person.avatar_url}
            <img src={person.avatar_url} alt="" />
          {:else}
            {initial}
          {/if}
        </span>
        <span class="person-names">
          <span class="person-display">{person.display_name || person.username}</span>
          <span class="muted person-handle">@{person.username}</span>
        </span>
      </div>

      {#if role}
        <p class="person-standing muted">
          {#if ROLE_MARK[role]}
            {@const Mark = ROLE_MARK[role]}
            <Mark size={14} weight={role === 'follower' ? 'fill' : 'regular'} />
          {/if}
          <span>{role}</span>
        </p>
      {/if}

      {#if items.length > 0}
        <!-- Shown only because the server sent it: this viewer is in a patch
             these items were shared into (docs/adr/083). The card never says
             which patch — that membership may be private or hidden. -->
        <ul class="person-contact">
          {#each items as item (item.id)}
            {@const Mark = KIND_MARK[item.kind]}
            <li>
              {#if Mark}<Mark size={14} aria-hidden="true" />{/if}
              <span class="sr-only">{KIND_WORD[item.kind] || item.kind}</span>
              {#if href(item)}
                <a href={href(item)}>{item.value}</a>
              {:else}
                <span>{item.value}</span>
              {/if}
              {#if item.label}<span class="muted">{' · '}{item.label}</span>{/if}
            </li>
          {/each}
        </ul>
      {/if}

      <a class="person-profile-link" href="/users/{person.username}" onclick={openProfile}>View profile</a>
    </div>
  {/if}
</span>

<style>
  .person-anchor {
    position: relative;
    display: inline-block;
  }

  .person-trigger {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: var(--color-text);
    cursor: pointer;
    text-align: left;
  }
  .person-trigger:hover,
  .person-trigger:focus-visible {
    text-decoration: underline;
  }

  .person-card {
    position: fixed;
    z-index: 60;
    width: 300px;
    max-width: calc(100vw - 16px);
    background: var(--color-surface, #fff);
    border: 1px solid var(--color-border);
    border-radius: 6px;
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.12);
    padding: 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .person-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .person-avatar {
    width: 2rem;
    height: 2rem;
    flex: 0 0 auto;
    border-radius: 50%;
    background: var(--color-border);
    display: grid;
    place-items: center;
    font-weight: 600;
    overflow: hidden;
  }
  .person-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .person-names {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .person-display {
    font-weight: 600;
  }
  .person-handle {
    font-size: 0.8rem;
  }

  .person-standing {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin: 0;
    font-size: 0.8rem;
  }

  .person-contact {
    list-style: none;
    margin: 0;
    padding: 0.5rem 0 0;
    border-top: 1px solid var(--color-border);
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.9rem;
  }
  .person-contact li {
    display: flex;
    align-items: baseline;
    gap: 0.4rem;
    word-break: break-word;
  }

  .person-profile-link {
    font-size: 0.85rem;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
</style>
