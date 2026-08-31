/**
 * The events surface never got the scope parameter the quilt and the map
 * both have, so `/events/my` rendered the whole instance's calendar and only
 * *added* remote-followed quilts' events on top of it.
 *
 * There is no Svelte render library in this project, so the wiring is
 * asserted against source text — which is enough here, because the bug was
 * exactly a missing param in the request. The behaviour behind the param is
 * covered in internal/handler/events_scope_test.go.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('the events page honours the quilt scope', () => {
  const src = source('pages/EventsPage.svelte');

  it('builds a scoped param for the local list', () => {
    expect(src).toMatch(
      /const localParams = params \+ \(quiltScope === 'my' \? '&scope=my' : ''\)/
    );
    expect(src).toContain('api(`events${localParams}`)');
  });

  it('never sends the first page unscoped', () => {
    // Inside loadData the plain `params` string is the shared half and
    // travels to other quilts, so the local fetch must not use it directly.
    // (loadMore builds its own `params` with the scope already appended.)
    const loadData = src.match(/async function loadData\(\)[\s\S]*?\n  \}/);
    expect(loadData, 'loadData not found').toBeTruthy();
    expect(loadData[0]).not.toContain('api(`events${params}`)');
    expect(loadData[0]).toContain('api(`events${localParams}`)');
  });

  it('does not leak the scope to other quilts', () => {
    // A remote quilt has no idea who is asking (CORS is `*`, so no cookie
    // rides along) and would answer scope=my with an empty list. What scopes
    // a remote quilt's events is the follow, not this parameter.
    const remoteFetch = src.match(/fetch\(`\$\{url\}\/api\/v1\/events[^`]*`\)/);
    expect(remoteFetch, 'remote events fetch not found').toBeTruthy();
    expect(remoteFetch[0]).toContain('${params}');
    expect(remoteFetch[0]).not.toContain('localParams');
  });

  it('carries the scope onto the next page', () => {
    // loadMore pages the local list; without the scope, page two was the
    // whole quilt appended to a scoped page one.
    const loadMore = src.match(/async function loadMore\(\)[\s\S]*?\n  \}/);
    expect(loadMore, 'loadMore not found').toBeTruthy();
    expect(loadMore[0]).toContain("&scope=my");
  });

  it('draws the filter map at the same scope as the list', () => {
    // The tag/search filter resolves an event to its patch through this map.
    // Fetched unscoped, a private patch you belong to is absent from it, and
    // filtering silently dropped that patch's events.
    expect(src).toMatch(
      /api\(`nodes\/tree\$\{quiltScope === 'my' \? '\?scope=my' : ''\}`\)/
    );
    expect(src).toContain('patchMapScope !== quiltScope');
  });

  it('only tracks remote follows where they are an ingredient', () => {
    // The load effect read the remote-follows store on both scopes, so the
    // whole-quilt list refetched itself the moment that store resolved.
    // They merge into the feed on My Quilt only.
    expect(src).toMatch(
      /if \(quiltScope === 'my'\) void getRemoteFollows\(\)\.length;/
    );
  });

  it('keeps the tree cache out of the effect that refetches', () => {
    // patchMap is $state read inside loadData, which runs from an $effect
    // that also writes it — the read is what made this page fetch its list
    // three times per view. The cache key is a plain variable on purpose.
    expect(src).toMatch(/\n  let patchMapScope = null;/);
    expect(src).not.toContain('if (patchMap.size === 0)');
  });
});
