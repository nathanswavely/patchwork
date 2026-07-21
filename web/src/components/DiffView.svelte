<script>
  import { diffLines } from 'diff';

  let {
    oldText = '',
    newText = '',
    oldLabel = 'Current',
    newLabel = 'Proposed',
    defaultMode = 'auto',
  } = $props();

  // View mode: 'split' or 'unified'. Auto-detects from viewport.
  let viewMode = $state('unified');

  $effect(() => {
    if (defaultMode !== 'auto') {
      viewMode = defaultMode;
      return;
    }
    const mq = window.matchMedia('(min-width: 769px)');
    viewMode = mq.matches ? 'split' : 'unified';
    const handler = (e) => { viewMode = e.matches ? 'split' : 'unified'; };
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  });

  // Collapsed sections: set of indices into the collapsible ranges.
  let expandedSections = $state(new Set());

  function toggleExpand(key) {
    const next = new Set(expandedSections);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    expandedSections = next;
  }

  // Compute line-level diff.
  let hunks = $derived.by(() => {
    if (oldText === newText) return [];
    return diffLines(oldText || '', newText || '');
  });

  // Stats.
  let stats = $derived.by(() => {
    let added = 0, removed = 0;
    for (const h of hunks) {
      const count = h.value.split('\n').filter((l, i, arr) => !(i === arr.length - 1 && l === '')).length;
      if (h.added) added += count;
      if (h.removed) removed += count;
    }
    return { added, removed };
  });

  // Build line arrays for rendering.
  // Each entry: { oldNo, newNo, text, type: 'added'|'removed'|'unchanged' }
  let diffData = $derived.by(() => {
    if (hunks.length === 0) return { unified: [], left: [], right: [] };

    // Unified lines.
    const unified = [];
    let oldNo = 1, newNo = 1;
    for (const hunk of hunks) {
      const lines = hunk.value.split('\n');
      // diffLines includes trailing empty string from final newline — skip it.
      const clean = lines.length > 1 && lines[lines.length - 1] === '' ? lines.slice(0, -1) : lines;

      for (const line of clean) {
        if (hunk.added) {
          unified.push({ oldNo: null, newNo, text: line, type: 'added' });
          newNo++;
        } else if (hunk.removed) {
          unified.push({ oldNo, newNo: null, text: line, type: 'removed' });
          oldNo++;
        } else {
          unified.push({ oldNo, newNo, text: line, type: 'unchanged' });
          oldNo++;
          newNo++;
        }
      }
    }

    // Split lines: pair removed/added hunks side-by-side.
    const left = [];
    const right = [];
    let lOld = 1, lNew = 1;
    let i = 0;
    while (i < hunks.length) {
      const hunk = hunks[i];
      const lines = hunk.value.split('\n');
      const clean = lines.length > 1 && lines[lines.length - 1] === '' ? lines.slice(0, -1) : lines;

      if (!hunk.added && !hunk.removed) {
        for (const line of clean) {
          left.push({ no: lOld++, text: line, type: 'unchanged' });
          right.push({ no: lNew++, text: line, type: 'unchanged' });
        }
        i++;
      } else if (hunk.removed && i + 1 < hunks.length && hunks[i + 1].added) {
        // Paired change: removed on left, added on right.
        const remLines = clean;
        const addLines = hunks[i + 1].value.split('\n');
        const addClean = addLines.length > 1 && addLines[addLines.length - 1] === '' ? addLines.slice(0, -1) : addLines;
        const maxLen = Math.max(remLines.length, addClean.length);
        for (let j = 0; j < maxLen; j++) {
          if (j < remLines.length) {
            left.push({ no: lOld++, text: remLines[j], type: 'removed' });
          } else {
            left.push({ no: null, text: '', type: 'empty' });
          }
          if (j < addClean.length) {
            right.push({ no: lNew++, text: addClean[j], type: 'added' });
          } else {
            right.push({ no: null, text: '', type: 'empty' });
          }
        }
        i += 2;
      } else if (hunk.removed) {
        for (const line of clean) {
          left.push({ no: lOld++, text: line, type: 'removed' });
          right.push({ no: null, text: '', type: 'empty' });
        }
        i++;
      } else {
        // added only
        for (const line of clean) {
          left.push({ no: null, text: '', type: 'empty' });
          right.push({ no: lNew++, text: line, type: 'added' });
        }
        i++;
      }
    }

    return { unified, left, right };
  });

  // Collapsible ranges: find runs of >6 unchanged lines.
  const CONTEXT_LINES = 3;

  function getCollapsibleRanges(lines) {
    const ranges = [];
    let runStart = -1;
    for (let i = 0; i < lines.length; i++) {
      const isUnchanged = lines[i].type === 'unchanged';
      if (isUnchanged) {
        if (runStart === -1) runStart = i;
      } else {
        if (runStart !== -1 && i - runStart > CONTEXT_LINES * 2 + 1) {
          ranges.push({ start: runStart + CONTEXT_LINES, end: i - CONTEXT_LINES });
        }
        runStart = -1;
      }
    }
    // Handle trailing unchanged run.
    if (runStart !== -1 && lines.length - runStart > CONTEXT_LINES * 2 + 1) {
      ranges.push({ start: runStart + CONTEXT_LINES, end: lines.length - CONTEXT_LINES });
    }
    return ranges;
  }

  let unifiedRanges = $derived(getCollapsibleRanges(diffData.unified));
  // For split, use left lines as the reference (they're the same length as right).
  let splitRanges = $derived(getCollapsibleRanges(diffData.left));

  function isCollapsed(ranges, lineIdx) {
    for (const r of ranges) {
      if (lineIdx >= r.start && lineIdx < r.end && !expandedSections.has(r.start)) {
        return r;
      }
    }
    return null;
  }

  function isCollapseStart(ranges, lineIdx) {
    for (const r of ranges) {
      if (lineIdx === r.start && !expandedSections.has(r.start)) {
        return r;
      }
    }
    return null;
  }
</script>

{#if oldText === newText}
  <div class="diff-view diff-empty">
    <p class="muted" style="padding: 1rem; text-align: center; font-size: 0.85rem;">No changes</p>
  </div>
{:else}

<div class="diff-view">
  <div class="diff-header">
    <div class="diff-labels">
      <span class="diff-label">
        <span class="diff-dot dot-removed"></span> {oldLabel}
      </span>
      <span class="diff-label">
        <span class="diff-dot dot-added"></span> {newLabel}
      </span>
    </div>
    <div class="diff-controls">
      <span class="diff-stats">
        {#if stats.added > 0}<span class="stat-added">+{stats.added}</span>{/if}
        {#if stats.removed > 0}<span class="stat-removed">-{stats.removed}</span>{/if}
      </span>
      <div class="mode-toggle">
        <button class:active={viewMode === 'split'} onclick={() => viewMode = 'split'}>Split</button>
        <button class:active={viewMode === 'unified'} onclick={() => viewMode = 'unified'}>Unified</button>
      </div>
    </div>
  </div>

  {#if viewMode === 'unified'}
    <div class="diff-body unified">
      {#each diffData.unified as line, idx}
        {@const collapse = isCollapseStart(unifiedRanges, idx)}
        {@const hidden = !collapse && isCollapsed(unifiedRanges, idx)}
        {#if collapse}
          <button class="collapse-row" onclick={() => toggleExpand(collapse.start)}>
            {collapse.end - collapse.start} unchanged lines
          </button>
        {:else if !hidden}
          <div class="diff-line {line.type}">
            <span class="line-no old-no">{line.oldNo ?? ''}</span>
            <span class="line-no new-no">{line.newNo ?? ''}</span>
            <span class="line-prefix">{line.type === 'added' ? '+' : line.type === 'removed' ? '-' : ' '}</span>
            <span class="line-text">{line.text || '\u00A0'}</span>
          </div>
        {/if}
      {/each}
    </div>

  {:else}
    <div class="diff-body split">
      <div class="split-side left">
        {#each diffData.left as line, idx}
          {@const collapse = isCollapseStart(splitRanges, idx)}
          {@const hidden = !collapse && isCollapsed(splitRanges, idx)}
          {#if collapse}
            <button class="collapse-row" onclick={() => toggleExpand(collapse.start)}>
              {collapse.end - collapse.start} lines
            </button>
          {:else if !hidden}
            <div class="diff-line {line.type}">
              <span class="line-no">{line.no ?? ''}</span>
              <span class="line-text">{line.text || '\u00A0'}</span>
            </div>
          {/if}
        {/each}
      </div>
      <div class="split-side right">
        {#each diffData.right as line, idx}
          {@const collapse = isCollapseStart(splitRanges, idx)}
          {@const hidden = !collapse && isCollapsed(splitRanges, idx)}
          {#if collapse}
            <button class="collapse-row" onclick={() => toggleExpand(collapse.start)}>
              {collapse.end - collapse.start} lines
            </button>
          {:else if !hidden}
            <div class="diff-line {line.type}">
              <span class="line-no">{line.no ?? ''}</span>
              <span class="line-text">{line.text || '\u00A0'}</span>
            </div>
          {/if}
        {/each}
      </div>
    </div>
  {/if}
</div>

{/if}

<style>
  .diff-view {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
    font-family: 'SF Mono', 'Fira Code', 'Fira Mono', Menlo, Consolas, monospace;
    font-size: 0.82rem;
    line-height: 1.5;
  }

  .diff-empty {
    background: var(--color-surface);
  }

  .diff-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.4rem 0.75rem;
    background: var(--color-overlay);
    border-bottom: 1px solid var(--color-border);
    font-size: 0.78rem;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .diff-labels {
    display: flex;
    gap: 1rem;
    align-items: center;
  }

  .diff-label {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--color-text-muted);
  }

  .diff-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .dot-removed { background: var(--color-error); }
  .dot-added { background: var(--color-success); }

  .diff-controls {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .diff-stats {
    display: flex;
    gap: 0.4rem;
    font-weight: 500;
  }

  .stat-added { color: var(--color-success); }
  .stat-removed { color: var(--color-error); }

  .mode-toggle {
    display: flex;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .mode-toggle button {
    padding: 0.15rem 0.5rem;
    border: none;
    background: var(--color-surface);
    font-size: 0.72rem;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .mode-toggle button.active {
    background: var(--color-primary);
    color: var(--color-btn-on-primary);
  }

  /* Unified view */
  .diff-body.unified {
    overflow-x: auto;
  }

  .diff-body.unified .diff-line {
    display: flex;
    align-items: baseline;
    min-height: 1.5em;
    padding: 0 0.5rem;
  }

  .diff-body.unified .line-no {
    width: 3ch;
    text-align: right;
    color: var(--color-text-muted);
    flex-shrink: 0;
    user-select: none;
  }

  .diff-body.unified .old-no { margin-right: 0.25rem; }
  .diff-body.unified .new-no { margin-right: 0.5rem; }

  .diff-body.unified .line-prefix {
    width: 1.5ch;
    flex-shrink: 0;
    user-select: none;
    font-weight: 600;
  }

  .diff-body.unified .line-text {
    white-space: pre-wrap;
    word-break: break-word;
    flex: 1;
    min-width: 0;
  }

  /* Split view */
  .diff-body.split {
    display: grid;
    grid-template-columns: 1fr 1fr;
    overflow-x: auto;
  }

  .split-side {
    overflow: hidden;
  }

  .split-side.left {
    border-right: 1px solid var(--color-border);
  }

  .split-side .diff-line {
    display: flex;
    align-items: baseline;
    min-height: 1.5em;
    padding: 0 0.5rem;
  }

  .split-side .line-no {
    width: 3ch;
    text-align: right;
    margin-right: 0.5rem;
    color: var(--color-text-muted);
    flex-shrink: 0;
    user-select: none;
  }

  .split-side .line-text {
    white-space: pre-wrap;
    word-break: break-word;
    flex: 1;
    min-width: 0;
  }

  /* Line type colors */
  .diff-line.added {
    background: color-mix(in srgb, var(--color-success) 10%, var(--color-surface));
  }

  .diff-line.added .line-prefix { color: var(--color-success); }

  .diff-line.removed {
    background: color-mix(in srgb, var(--color-error) 10%, var(--color-surface));
  }

  .diff-line.removed .line-prefix { color: var(--color-error); }

  .diff-line.empty {
    background: var(--color-overlay);
  }

  /* Collapse row */
  .collapse-row {
    display: block;
    width: 100%;
    padding: 0.2rem 0.75rem;
    border: none;
    border-top: 1px solid var(--color-border);
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg);
    color: var(--color-primary);
    font-size: 0.75rem;
    cursor: pointer;
    text-align: center;
    font-family: inherit;
  }

  .collapse-row:hover {
    background: color-mix(in srgb, var(--color-primary) 5%, var(--color-bg));
  }

  @media (max-width: 768px) {
    .mode-toggle {
      display: none;
    }
  }
</style>
