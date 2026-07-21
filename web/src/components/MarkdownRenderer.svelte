<script>
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';

  let { content = '' } = $props();

  marked.setOptions({ breaks: true, gfm: true });

  let html = $derived(content ? DOMPurify.sanitize(marked.parse(content)) : '');
</script>

<div class="markdown-body">
  {@html html}
</div>

<style>
  .markdown-body { font-size: 0.9rem; line-height: 1.7; color: var(--color-text); }
  .markdown-body :global(h1) { font-size: 1.4rem; font-weight: 700; margin: 1.5rem 0 0.5rem; }
  .markdown-body :global(h2) { font-size: 1.2rem; font-weight: 600; margin: 1.25rem 0 0.4rem; }
  .markdown-body :global(h3) { font-size: 1.05rem; font-weight: 600; margin: 1rem 0 0.3rem; }
  .markdown-body :global(p) { margin: 0 0 0.75rem; }
  .markdown-body :global(ul), .markdown-body :global(ol) { margin: 0 0 0.75rem; padding-left: 1.5rem; }
  .markdown-body :global(li) { margin-bottom: 0.25rem; }
  .markdown-body :global(a) { color: var(--color-primary); }
  .markdown-body :global(code) { background: var(--color-overlay); padding: 0.15rem 0.35rem; border-radius: 3px; font-size: 0.85em; }
  .markdown-body :global(pre) { background: var(--color-overlay); padding: 0.75rem 1rem; border-radius: 4px; overflow-x: auto; margin: 0 0 0.75rem; }
  .markdown-body :global(pre code) { background: none; padding: 0; }
  .markdown-body :global(blockquote) { border-left: 3px solid var(--color-border); padding-left: 1rem; color: var(--color-text-muted); margin: 0 0 0.75rem; }
  .markdown-body :global(table) { width: 100%; border-collapse: collapse; margin: 0 0 0.75rem; }
  .markdown-body :global(th), .markdown-body :global(td) { border: 1px solid var(--color-border); padding: 0.4rem 0.6rem; text-align: left; font-size: 0.85rem; }
  .markdown-body :global(th) { background: var(--color-overlay); font-weight: 600; }
  .markdown-body :global(hr) { border: none; border-top: 1px solid var(--color-border); margin: 1.5rem 0; }
</style>
