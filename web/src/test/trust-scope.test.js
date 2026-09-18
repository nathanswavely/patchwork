/**
 * docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar
 *
 * Decision 2 gives the trusted-contributor grant a scope, decision 3 offers
 * the per-patch grant at suggestion approval, decision 6 lets a suggestion
 * carry a feed, decision 7 has an instance admin answer a trust request from
 * Admin → Users at whatever scope they judge right, and decision 8 makes the
 * rejection note something the suggester reads.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('TrustScopePicker — one control for the whole quilt or a named set', () => {
  const src = source('components/TrustScopePicker.svelte');

  it('keeps the interface its consumers were built against', () => {
    expect(src).toMatch(/all = \$bindable\(false\)/);
    expect(src).toMatch(/selected = \$bindable\(\[\]\)/);
    expect(src).toMatch(/disabled = false/);
    expect(src).toMatch(/label = 'Where'/);
    // The event form asks for a scope that may be the whole quilt; the
    // per-patch grant on a user row may not, so the parent checkbox is a
    // prop rather than a fact about the component.
    expect(src).toMatch(/allowAll = true/);
  });

  it('puts "Every unclaimed patch" at the top, as a real checkbox bound to `all`', () => {
    expect(src).toMatch(/<input type="checkbox" bind:checked=\{all\}/);
    expect(src).toContain('Every unclaimed patch');
  });

  it('dims and disables the per-patch list under `all` without clearing the picks', () => {
    expect(src).toMatch(/class:dimmed=\{all\}/);
    expect(src).toMatch(/disabled=\{disabled \|\| all\}/);
    // Nothing anywhere empties `selected` when `all` is checked.
    expect(src).not.toMatch(/all[\s\S]{0,40}selected = \[\]/);
  });

  it('loads unclaimed patches once, from the ordinary node list', () => {
    expect(src).toContain('nodes?status=unclaimed');
    expect(src).toMatch(/onfocus=\{ensureLoaded\}/);
    expect(src).toMatch(/if \(loadState === 'loading' \|\| loadState === 'loaded'\) return;/);
  });

  it('filters in the browser, debounced, with no server-side query parameter', () => {
    expect(src).toMatch(/setTimeout\(\(\) => \{ filter = query; \}, 200\)/);
    expect(src).toMatch(/\.toLowerCase\(\)\.includes\(filter\.trim\(\)\.toLowerCase\(\)\)/);
    expect(src).not.toMatch(/nodes\?[^`'"]*\bq=/);
  });

  it('offers at most eight matches and never one that is already picked', () => {
    expect(src).toMatch(/\.slice\(0, 8\)/);
    expect(src).toMatch(/\.filter\(\(n\) => !selectedIds\.has\(n\.id\)\)/);
  });

  it('says so plainly when nothing matches', () => {
    expect(src).toContain('No unclaimed patch by that name.');
  });

  it('collapses behind a summary, and starts open only when there is nothing to summarize', () => {
    expect(src).toMatch(/let expanded = \$state\(selected\.length === 0 && !all\)/);
    expect(src).toMatch(/aria-expanded=\{expanded\}/);
    expect(src).toMatch(/aria-controls=\{bodyId\}/);
    expect(src).toContain("if (everywhere) return 'Every unclaimed patch';");
  });

  it('has no reorder controls: selection order is the order', () => {
    expect(src).not.toMatch(/move ?(up|down|earlier|later)/i);
    expect(src).not.toMatch(/ArrowUp|ArrowDown/);
    // No index arithmetic on the picks, which is what a reorder control
    // would need and nothing else here does.
    expect(src).not.toMatch(/splice\(|\[i - 1\]|\[i \+ 1\]/);
  });
});

describe('AdminSubmissions — the approval carries two second judgements', () => {
  const src = source('pages/AdminSubmissions.svelte');

  it('shows the feed the suggestion carried, and its upcoming count', () => {
    expect(src).toMatch(/\{#if sub\.feed_url\}/);
    expect(src).toMatch(/<a href=\{sub\.feed_url\} target="_blank" rel="noopener">/);
    expect(src).toContain('found when it was suggested');
    expect(src).toContain('{sub.feed_upcoming_count}');
    expect(src).toContain("sub.feed_upcoming_count === 1 ? 'upcoming event' : 'upcoming events'");
  });

  it('offers the per-patch grant in the submitter’s name, checked by default', () => {
    expect(src).toContain(
      'Let {submitterName(sub)} add events here without review until it is claimed'
    );
    expect(src).toContain('A per-patch trusted-contributor grant. Revocable from Users.');
    expect(src).toMatch(/trust\[sub\.id\] = true;/);
  });

  it('offers the feed only when there is one, checked by default', () => {
    expect(src).toContain('Attach the events feed');
    expect(src).toMatch(/feeds\[sub\.id\] = true;/);
  });

  it('sends grant_trust and attach_feed on approve, and never a feed there is none of', () => {
    expect(src).toMatch(/body\.grant_trust = grantTrustInputs\[id\] !== false;/);
    expect(src).toMatch(
      /body\.attach_feed = Boolean\(sub\.feed_url\) && attachFeedInputs\[id\] !== false;/
    );
  });

  it('labels the note as something the suggester reads, and sends it on reject', () => {
    expect(src).toContain('Note to the suggester (optional)');
    expect(src).toMatch(/body\.note = \(noteInputs\[id\] \|\| ''\)\.trim\(\);/);
  });
});

describe('AdminUsers — a trust request is answered here', () => {
  const src = source('pages/AdminUsers.svelte');

  it('loads the pending requests and hides the section entirely when there are none', () => {
    expect(src).toMatch(/await api\('admin\/trust-requests'\)/);
    expect(src).toMatch(/\{#if trustRequests\.length > 0\}/);
    expect(src).toContain('<h2>Trust requests</h2>');
  });

  it('opens the picker on the scope that was asked for', () => {
    expect(src).toMatch(/all: req\.scope === 'all'/);
    expect(src).toMatch(/selected: \(req\.nodes \|\| \[\]\)\.map/);
    expect(src).toMatch(/bind:all=\{requestScopes\[req\.id\]\.all\}/);
    expect(src).toMatch(/bind:selected=\{requestScopes\[req\.id\]\.selected\}/);
  });

  it('refuses an empty scope before it reaches the server', () => {
    expect(src).toContain('Pick at least one patch, or every unclaimed patch.');
  });

  it('sends the approve and decline bodies the route takes', () => {
    expect(src).toMatch(/const body = \{ action: 'approve', scope: scope\.all \? 'all' : 'patches' \};/);
    expect(src).toMatch(/if \(!scope\.all\) body\.node_ids = scope\.selected\.map\(\(n\) => n\.id\);/);
    expect(src).toMatch(/body: \{ action: 'decline', note: \(requestNotes\[req\.id\] \|\| ''\)\.trim\(\) \}/);
    expect(src).toMatch(/api\(`admin\/trust-requests\/\$\{req\.id\}`, \{ method: 'PATCH'/);
  });

  it('names the person and links to their profile', () => {
    expect(src).toContain('href="/users/{req.user.username}"');
    expect(src).toContain('{req.user.display_name || req.user.username}');
  });
});

describe('AdminUsers — the per-patch grants sit beside the quilt-wide one', () => {
  const src = source('pages/AdminUsers.svelte');

  it('lists the grants as chips that say where the trust reaches', () => {
    expect(src).toContain('Trusted on {node.name}');
    expect(src).toMatch(/\{#each u\.trusted_nodes as node \(node\.id\)\}/);
  });

  it('grants and revokes through the trusted-patches routes', () => {
    expect(src).toMatch(
      /api\(`admin\/users\/\$\{user\.id\}\/trusted-patches`, \{\s*method: 'POST',\s*body: \{ node_id: node\.id \},/
    );
    expect(src).toMatch(
      /api\(`admin\/users\/\$\{user\.id\}\/trusted-patches\/\$\{node\.id\}`, \{ method: 'DELETE' \}\)/
    );
  });

  it('uses the shared picker in per-patch mode, with no quilt-wide checkbox', () => {
    expect(src).toContain("import TrustScopePicker from '../components/TrustScopePicker.svelte'");
    expect(src).toMatch(/bind:selected=\{addTrustSelected\}\s*\n\s*allowAll=\{false\}/);
  });

  it('keeps the quilt-wide toggle it already had', () => {
    expect(src).toMatch(/body: \{ trusted_contributor: !user\.trusted_contributor \}/);
  });
});
