/**
 * Co-hosts read as equals, and the owner can share the pen
 * (docs/adr/2026-10-19-co-hosts-read-as-equals-and-the-owner-can-share-the-pen.md).
 *
 * Three changes to how event links (docs/adr/032) are made and shown: the
 * event form chooses co-hosts at creation and when editing, the event page
 * names every confirmed co-host on its "Hosted by" line, and the owner may
 * grant a co-host edit rights that stop short of visibility and deletion.
 * The server enforces all of it (event_link_edit_rights_test.go); these pin
 * that the UI offers only what the server will accept.
 *
 * Source text only — there is no Svelte render library in this project.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('EventForm chooses co-hosts', () => {
  const src = source('pages/EventForm.svelte');

  it('mounts the picker only for someone who speaks for the hosting patch', () => {
    expect(src).toMatch(/import CoHostPicker from '\.\.\/components\/CoHostPicker\.svelte'/);
    expect(src).toMatch(/\{#if canChooseCoHosts\}[\s\S]*?<CoHostPicker bind:rows=\{coHosts\}/);
    const gate = src.match(/let canChooseCoHosts = \$derived\(([\s\S]*?)\n  \);/)[1];
    expect(gate).toMatch(/!coEditing/);
    expect(gate).toMatch(/speaksFor\(hostingPatch\.slug, hostingPatch\.status, hostingViewerTrusted\)/);
    // A suggestion is not an active event, and only active events take links.
    expect(gate).toMatch(/isEdit \? eventStatus === 'active' : !willReview/);
  });

  it('mirrors the handshake standing, status check before any trust', () => {
    const fn = src.match(/function speaksFor\(slug, status, viewerTrusted = false\) \{([\s\S]*?)\n  \}/)[1];
    expect(fn).toMatch(/activeRoles\.get\(slug\) === 'admin'/);
    expect(fn.indexOf("if (status !== 'unclaimed') return false;")).toBeLessThan(
      fn.indexOf('viewerTrusted || isTrustedContributor()')
    );
  });

  it('hides who-can-see-this from a co-host editor, whom the server refuses', () => {
    expect(src).toMatch(/let showTierControl = \$derived\(!!hostingPatch && !willReview && !coEditing\);/);
    expect(src).toMatch(/coEditing = event\.viewer_co_edits === true;/);
  });

  it('turns rows into requests, rights changes and removals only after the save', () => {
    const fn = src.match(/async function applyCoHosts\(id\) \{([\s\S]*?)\n  \}/)[1];
    expect(fn).toMatch(/method: 'DELETE'/);
    expect(fn).toMatch(/method: 'POST', body: \{ target: r\.slug, can_edit: r\.can_edit \}/);
    expect(fn).toMatch(/method: 'PATCH', body: \{ can_edit: r\.can_edit \}/);
    // Created: only once the event exists and published.
    expect(src).toMatch(/canChooseCoHosts && coHosts\.length \? await applyCoHosts\(result\.id\) : \[\]/);
    expect(src).toMatch(/canChooseCoHosts \? await applyCoHosts\(eventId\) : \[\]/);
  });

  it('drops a co-host that becomes the hosting patch', () => {
    expect(src).toMatch(/coHosts = coHosts\.filter\(\(r\) => r\.slug !== item\.slug\);/);
  });
});

describe('CoHostPicker writes nothing itself', () => {
  const src = source('components/CoHostPicker.svelte');

  it('never posts a link; the form does, on save', () => {
    expect(src).not.toMatch(/method: 'POST'/);
    expect(src).not.toMatch(/\/links/);
  });

  it('leaves the hosting patch out and refuses a patch chosen twice', () => {
    expect(src).toMatch(/if \(n\.slug === hostSlug\) return null;/);
    expect(src).toMatch(/if \(!item\?\.slug \|\| item\.slug === hostSlug \|\| chosen\.has\(item\.slug\)\) return;/);
  });

  it('sends a patch on another quilt to the event page, where mentions are made', () => {
    expect(src).toMatch(/if \(parsed\.host !== location\.host\)/);
  });

  it('offers edit rights as a checkbox per row, off by default', () => {
    expect(src).toMatch(/can_edit: false, link: null/);
    expect(src).toMatch(/type="checkbox"[\s\S]*?Can edit/);
  });
});

describe('EventLinks names every co-host as a host', () => {
  const src = source('components/EventLinks.svelte');

  it('draws one Hosted by line, owner first, then confirmed links, then mentions', () => {
    expect(src).toMatch(/<span class="hosts">Hosted by\{#each hosts as h, i \(h\.key\)\}\{separator\(i, hosts\.length\)\}/);
    const hosts = src.match(/let hosts = \$derived\(\[([\s\S]*?)\]\);/)[1];
    expect(hosts.indexOf("kind: 'owner'")).toBeLessThan(hosts.indexOf("kind: 'link'"));
    expect(hosts.indexOf("kind: 'link'")).toBeLessThan(hosts.indexOf("kind: 'mention'"));
    expect(hosts).toMatch(/confirmed\.map/);
  });

  it('no longer sets co-hosts apart on a "with" line', () => {
    expect(src).not.toMatch(/class="with-label"/);
  });

  it('uses the serial comma, since band names contain "and"', () => {
    const fn = src.match(/function separator\(i, n\) \{([\s\S]*?)\n  \}/)[1];
    expect(fn).toMatch(/n === 2 \? ' and ' : ', and '/);
  });

  it('writes the line with no whitespace inside a name, which would space the commas', () => {
    expect(src).not.toMatch(/<span class="host">\s*\n/);
  });

  it('carries the page pills on the same line', () => {
    expect(src).toMatch(/let \{ event, onChanged, trailing \} = \$props\(\);/);
    expect(src).toMatch(/\{@render trailing\?\.\(\)\}/);
  });

  it('tells each side of a pending request where it stands', () => {
    expect(src).toMatch(/asked <strong>\{l\.node_name\}<\/strong> to co-host this event/);
    expect(src).toMatch(/Waiting for <strong>\{event\.node_name\}<\/strong> to confirm/);
  });
});

describe('EventDetail: a co-host edits but never deletes', () => {
  const src = source('pages/EventDetail.svelte');

  it('reads the server’s answer for the co-host door', () => {
    const canEdit = src.match(/let canEdit = \$derived\(([\s\S]*?)\n  \);/)[1];
    expect(canEdit).toMatch(/event\.viewer_co_edits === true/);
    expect(canEdit).toMatch(/!imported/);
  });

  it('gates Delete on DeleteEvent’s own rule, which edit rights do not reach', () => {
    const canDelete = src.match(/let canDelete = \$derived\(([\s\S]*?)\n  \);/)[1];
    expect(canDelete).not.toMatch(/viewer_co_edits/);
    expect(src).toMatch(/\{#if canDelete\}\s*<ConfirmAction/);
  });

  it('hands the pills to EventLinks instead of drawing its own host link', () => {
    expect(src).toMatch(/\{#snippet trailing\(\)\}/);
    expect(src).not.toMatch(/>Hosted by \{event\.node_name\}<\/a>/);
  });
});

describe('WorkspaceSearch: a picker keeps Enter out of its form', () => {
  const src = source('components/WorkspaceSearch.svelte');

  // Found by driving the co-host picker: after one pick the finder closed,
  // typing did not reopen it, and the next Enter saved the event.
  it('reopens when typed into after a pick', () => {
    const fn = src.match(/function onInput\(\) \{([\s\S]*?)\n  \}/)[1];
    expect(fn).toMatch(/if \(!open\) \{\n\s*open = true;\n\s*ensureLoaded\(\);/);
  });

  it('swallows Enter in a closed picker', () => {
    expect(src).toMatch(/if \(!open\) \{\n\s*if \(isPicker\) e\.preventDefault\(\);\n\s*return;\n\s*\}/);
  });
});
