/**
 * The Hosting Patch field on the event form.
 *
 * The select is fed membership rows (node_id, node_name) from me/nodes. The
 * suggest-an-event door (docs/adr/026) loads a patch instead — GET nodes/{slug}
 * returns a node (id, name) — and handing that straight to the select rendered
 * an option with neither value nor label: the field read blank and disabled
 * over a patch the form had already chosen. Submission worked; the control lied.
 *
 * Asserted against source text — there is no Svelte render library in this
 * project (see patch-profile-window.test.js).
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const source = readFileSync(
  resolve(process.cwd(), 'src/pages/EventForm.svelte'),
  'utf8'
);

describe('the hosting patch select is fed one shape', () => {
  it('renders options from node_id and node_name', () => {
    expect(source).toContain('<option value={node.node_id}>{node.node_name}</option>');
  });

  it('normalises the locked patch into that shape instead of passing a node', () => {
    // The lock path must not put a raw patch into the list the select reads.
    expect(source).not.toMatch(/myNodes\s*=\s*\[lockedNode\]/);
    expect(source).toMatch(
      /myNodes\s*=\s*\[\{\s*node_id:\s*lockedNode\.id,\s*node_name:\s*lockedNode\.name\s*\}\]/
    );
  });

  it('still submits the locked patch id', () => {
    expect(source).toMatch(/nodeId\s*=\s*lockedNode\.id/);
  });
});
