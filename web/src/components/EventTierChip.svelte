<script>
  // What an event's visibility says, wherever an event is drawn
  // (docs/adr/2026-09-19-an-event-says-who-it-is-for-within-what-the-patch-allows.md).
  //
  // A public event wears nothing: it is the ordinary case, and a chip on
  // every row says nothing at all. The other two always wear one, because a
  // members-only event that looks identical to a public one is the mistake
  // the tiers exist to prevent. Words, not an icon: "who can see this" is
  // not something a glyph says unambiguously.
  //
  // The chip states what the event says about itself. Where the patch has
  // turned Follower Permissions off, the server reads a followers event
  // down to members-only without rewriting the row, and the event form is
  // where that is explained — one place, not on every card.
  let { visibility = 'public' } = $props();

  const LABELS = { followers: 'Followers', members: 'Members only' };
  let label = $derived(LABELS[visibility] || '');
</script>

{#if label}
  <span class="badge">{label}</span>
{/if}
