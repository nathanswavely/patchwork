/**
 * Where a new word typed into the tag picker goes (docs/adr/114).
 *
 * In Patch Settings, Suggest only added a local chip and nothing was sent
 * until Save tags, so a suggestion made and left behind never reached the
 * admin queue while its chip already looked like it was waiting there. The
 * submission pages showed the same field and sent nothing at all. And an
 * instance admin had to suggest a word to themselves and approve it.
 *
 * Asserted against source text, since nothing here renders.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const picker = source('components/TagPicker.svelte');

describe('TagPicker: the new-word field', () => {
  it('appears only where the word goes somewhere', () => {
    expect(picker).toMatch(/showNewWord = \$derived\(creates \|\| !!onSuggest \|\| deferSuggestions\)/);
    expect(picker).toContain('{#if showNewWord}');
  });

  it('creates the tag outright for an instance admin and picks it', () => {
    expect(picker).toMatch(/creates = \$derived\(isAdmin\(\)\)/);
    expect(picker).toMatch(/if \(creates\) \{[\s\S]{0,400}api\('admin\/tags', \{ method: 'POST'[\s\S]{0,120}await loadTags\(\);\s*add\(/);
    expect(picker).toContain("creates ? 'Add tag' : 'Suggest'");
  });

  it('sends a suggestion at once when the caller provides onSuggest', () => {
    expect(picker).toMatch(/if \(onSuggest\) \{[\s\S]{0,120}await onSuggest\(name\)/);
  });
});

describe('Patch Settings sends a suggestion when it is made', () => {
  const settings = source('pages/PatchSettingsInfo.svelte');

  it('wires onSuggest rather than collecting chips for Save tags', () => {
    expect(settings).toContain('onSuggest={suggestTag}');
    expect(settings).not.toContain('bind:suggested');
    expect(settings).toMatch(/async function suggestTag\(name\) \{\s*await api\(`nodes\/\$\{slug\}`, \{ method: 'PATCH', body: \{ suggest_tags: \[name\] \} \}\)/);
  });

  it('does not send suggestions with Save tags any more', () => {
    const save = settings.slice(settings.indexOf('async function saveTags'), settings.indexOf('function resetTags'));
    expect(save).not.toContain('suggest_tags');
  });
});

describe('the other pickers', () => {
  it('patch creation still sends suggestions with the form', () => {
    const form = source('pages/PatchForm.svelte');
    expect(form).toContain('bind:suggested={suggestTags} deferSuggestions');
    expect(form).toMatch(/suggest_tags: suggestTags\.length > 0 \? suggestTags : undefined/);
  });

  it('listing submission and its review never offer a suggestion they would drop', () => {
    for (const page of ['pages/SubmitPatch.svelte', 'pages/AdminSubmissions.svelte']) {
      const src = source(page);
      expect(src).not.toContain('deferSuggestions');
      expect(src).not.toContain('onSuggest');
    }
  });
});
