<script>
  import { WALL, nearestSwatch } from '../lib/fabricWall.js';
  import { BUNDLE_SLOTS } from '../lib/draftGeometry.js';

  // bundle: 1-6 hex fabrics; slot 0 is the identity color (docs/adr/029).
  // Fabrics come off the wall only — there is no free color picker.
  let { bundle = $bindable(), selectedSlot = $bindable(0), hint = '' } = $props();

  function setSlotFabric(hex) {
    bundle[selectedSlot] = hex;
  }

  function addSlot() {
    if (bundle.length >= BUNDLE_SLOTS) return;
    // Suggest something unlike the last fabric: walk the wall from it.
    const last = nearestSwatch(bundle[bundle.length - 1]);
    const idx = last ? WALL.findIndex((w) => w.key === last.key) : -1;
    bundle.push(WALL[(idx + 7) % WALL.length].hex);
    selectedSlot = bundle.length - 1;
  }

  function removeSlot() {
    if (bundle.length <= 1) return;
    bundle.pop();
    if (selectedSlot >= bundle.length) selectedSlot = bundle.length - 1;
  }

  function swatchName(hex) {
    const sw = nearestSwatch(hex);
    return sw && sw.hex.toLowerCase() === hex.toLowerCase() ? sw.name : hex;
  }
</script>

<div class="bundle">
  <span class="bundle-label">Bundle</span>
  <div class="bundle-slots">
    {#each bundle as hex, i}
      <button
        class="slot"
        class:selected={selectedSlot === i}
        style="background: {hex}"
        title="Fabric {i + 1}: {swatchName(hex)}{i === 0 ? ' (identity color)' : ''}"
        aria-label="Fabric {i + 1}: {swatchName(hex)}"
        aria-pressed={selectedSlot === i}
        onclick={() => { selectedSlot = i; }}
      ></button>
    {/each}
    {#if bundle.length < BUNDLE_SLOTS}
      <button class="slot slot-add" onclick={addSlot} title="Add a fabric">+</button>
    {/if}
    {#if bundle.length > 1}
      <button class="slot slot-remove" onclick={removeSlot} title="Remove the last fabric">−</button>
    {/if}
  </div>
  {#if hint}
    <p class="muted bundle-hint">{hint}</p>
  {/if}
  <div class="wall">
    {#each WALL as sw (sw.key)}
      <button
        class="swatch"
        class:selected={bundle[selectedSlot]?.toLowerCase() === sw.hex.toLowerCase()}
        style="background: {sw.hex}"
        title={sw.name}
        aria-label={sw.name}
        onclick={() => setSlotFabric(sw.hex)}
      ></button>
    {/each}
  </div>
</div>

<style>
  .bundle-label {
    display: block;
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
    margin-bottom: 0.4rem;
  }

  .bundle-slots {
    display: flex;
    gap: 0.35rem;
    align-items: center;
    margin-bottom: 0.4rem;
  }

  .slot {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    border: 2px solid var(--color-border);
    cursor: pointer;
  }

  .slot.selected {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 2px var(--color-primary);
  }

  .slot-add,
  .slot-remove {
    background: var(--color-surface);
    color: var(--color-text-muted);
    font-weight: 700;
    line-height: 1;
  }

  .bundle-hint {
    font-size: 0.72rem;
    margin: 0 0 0.4rem;
  }

  .wall {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(22px, 1fr));
    gap: 0.25rem;
    max-width: 320px;
  }

  .swatch {
    aspect-ratio: 1;
    border-radius: 3px;
    border: 1px solid rgba(0, 0, 0, 0.15);
    cursor: pointer;
  }

  .swatch.selected {
    outline: 2px solid var(--color-primary);
    outline-offset: 1px;
  }
</style>
