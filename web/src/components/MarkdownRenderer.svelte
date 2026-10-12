<script>
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';

  let { content = '' } = $props();

  marked.setOptions({ breaks: true, gfm: true });

  let html = $derived(content ? DOMPurify.sanitize(marked.parse(content)) : '');
</script>

<!-- Governance prose (the charter, proposal bodies, legal documents) at
     full body size, read at length at a proper measure (docs/adr/038).
     Liquid Textile's .lt-prose was lifted from the rules this used to
     carry; its rules sit at zero specificity, so a surface that tightens
     its own paragraphs (a comment, a preview pane) still wins. -->
<div class="lt-prose">
  {@html html}
</div>
