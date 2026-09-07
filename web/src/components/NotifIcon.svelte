<script>
  import { ClipboardText, Scroll, UsersThree, MapPin, Wrench, Bell, ChatCircle, Heart, ShieldWarning } from 'phosphor-svelte';

  let { type = '', size = 18 } = $props();

  const ICONS = [
    ['proposal.', ClipboardText],
    ['governance.', Scroll],
    ['membership.', UsersThree],
    ['event.', MapPin],
    ['admin.', Wrench],
    ['comment.', ChatCircle],
    // Cross-quilt: a followed remote patch said something (docs/adr/024).
    ['remote.', Heart],
    // Moderation: a warning, a suspension, or a report of yours reviewed.
    // These carried the default bell, which made a warning look like any
    // other notice in a list.
    ['account.', ShieldWarning],
    ['report.', ShieldWarning],
  ];

  let Icon = $derived.by(() => {
    for (const [prefix, component] of ICONS) {
      if (type?.startsWith(prefix)) return component;
    }
    return Bell;
  });
</script>

<Icon {size} weight="duotone" />
