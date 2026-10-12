// Self-hosted fonts — bundled into dist/ and embedded in the binary, so no
// visitor traffic ever reaches Google. bnce.css carries the BNCE axis that
// --font-display headings use.
import '@fontsource-variable/shantell-sans/bnce.css';
import '@fontsource-variable/space-grotesk/index.css';

// Liquid Textile, the design system (vendored; scripts/sync-lt.sh). Before
// app.css, so Patchwork's own rules win on order while both exist, and so
// app.css can set an --lt- token over Liquid Textile's.
import './vendor/liquid-textile/foundation.css';
// Element defaults (html, links, headings, bare fields), all in :where(),
// so every component's own rule wins.
import './vendor/liquid-textile/base.css';
import './vendor/liquid-textile/liquid-textile.css';
import './vendor/liquid-textile/patterns.css';
import './vendor/liquid-textile/textures.css';
import './app.css';
import './stores/theme.svelte.js'; // Initialize theme before render.
import App from './App.svelte';
import { mount } from 'svelte';


const app = mount(App, {
  target: document.getElementById('app'),
});

export default app;
