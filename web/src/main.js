// Self-hosted fonts — bundled into dist/ and embedded in the binary, so no
// visitor traffic ever reaches Google. bnce.css carries the BNCE axis that
// --font-display headings use.
import '@fontsource-variable/shantell-sans/bnce.css';
import '@fontsource-variable/space-grotesk/index.css';

import './app.css';
import './lib/textile.css';
import './stores/theme.svelte.js'; // Initialize theme before render.
import App from './App.svelte';
import { mount } from 'svelte';


const app = mount(App, {
  target: document.getElementById('app'),
});

export default app;
