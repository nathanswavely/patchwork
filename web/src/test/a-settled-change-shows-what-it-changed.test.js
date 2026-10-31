/**
 * A settled proposal's Changes tab diffs against the text it was made
 * against.
 *
 * The trap: once a change applies, the live document is the proposed body,
 * so diffing `current_doc_content` against `proposed_body` compares a text
 * with itself. An applied rules change read "No changes to governance rules."
 * The server sends `prior_doc_content` (from the charter's history) for a
 * settled proposal, or `prior_doc_unavailable` when the history is gone, and
 * the page must diff against that, never fall back to the live text.
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

describe('ProposalDetail — the Changes tab of a settled proposal', () => {
  const src = source('pages/ProposalDetail.svelte');
  const changes = src.slice(src.indexOf("activeTab === 'changes'"), src.indexOf("activeTab === 'discussion'"));

  it('picks the prior text whenever the server sent one', () => {
    expect(src).toMatch(
      /settledDiff = \$derived\(proposal\?\.prior_doc_content != null \|\| proposal\?\.prior_doc_unavailable === true\)/
    );
    expect(src).toMatch(
      /diffBase = \$derived\(settledDiff \? proposal\?\.prior_doc_content : proposal\?\.current_doc_content\)/
    );
  });

  it('diffs both the rules and a charter against that base, not the live text', () => {
    expect(changes).toContain('currentRules={JSON.parse(diffBase)}');
    expect(changes).toContain('oldText={diffBase}');
    expect(changes).not.toContain('current_doc_content');
  });

  it('says the history is gone rather than showing a diff against the live text', () => {
    expect(changes).toMatch(/\{#if diffBase == null\}\s*<p class="lt-muted">The text from before this proposal is not in this patch's history/);
  });

  it('labels an applied change as what changed, not what is proposed', () => {
    expect(changes).toContain("heading={changeApplied ? 'Rule changes' : undefined}");
    expect(changes).toContain("oldLabel={changeApplied ? 'Before' : settledDiff ? 'When proposed' : 'Current'}");
    expect(changes).toContain("newLabel={changeApplied ? 'After' : 'Proposed'}");
  });
});

describe('StructuredRulesDiff — the heading is the caller\'s', () => {
  const src = source('components/StructuredRulesDiff.svelte');

  it('defaults to the editor\'s heading and renders whatever it is given', () => {
    expect(src).toContain("heading = 'Proposed Rule Changes'");
    expect(src).toContain('<h3>{heading}</h3>');
  });
});
