/**
 * Regression tests for the passkey naming flow.
 *
 * The first cut of #44 put a "Name this passkey" field above the Add Passkey
 * button. It read as an editor for the credential list above it rather than a
 * setting for a credential that did not exist yet — reported from production
 * as "a rename form sitting on the page with no connected context".
 *
 * Naming now happens after enrollment, as a plain rename against a credential
 * that already exists. That ordering is not cosmetic: collecting the name
 * between the ceremony and register/finish would put a person's typing inside
 * the challenge's TTL window, so a slow typist could lose the enrollment.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against the source text — enough to catch this regression, which
 * is entirely a question of what is rendered when.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// Vitest runs with the web/ project root as cwd.
function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('passkey naming happens after enrollment', () => {
  const src = source('pages/SecuritySettings.svelte');

  it('renders no standalone name field before the passkey exists', () => {
    // The orphaned field was <input id="passkey-name"> sitting in the
    // add-passkey block. Nothing may bind a name input outside the modal.
    expect(src).not.toContain('id="passkey-name"');
    expect(src).not.toContain('bind:value={newName}');
  });

  it('collects the name in a modal, not inline', () => {
    expect(src).toContain('<Modal');
    expect(src).toContain('open={namingId !== null}');
  });

  it('opens the naming modal only once the credential has an id', () => {
    // The modal renames a real credential, so it must not open unless the
    // server actually returned one.
    expect(src).toMatch(/if \(created\?\.id\)/);
    expect(src).toContain('namingId = created.id');
  });

  it('enrolls with the guessed name so the ceremony never waits on typing', () => {
    // register/finish must carry a name computed on the spot — not a value
    // read out of an input the person is still editing.
    expect(src).toMatch(/register\/finish[\s\S]{0,200}name: suggestName\(\)/);
  });

  it('saves the chosen name via the rename endpoint, not re-enrollment', () => {
    expect(src).toMatch(/auth\/credentials\/\$\{namingId\}/);
    expect(src).toMatch(/method: 'PATCH'/);
  });

  it('dismissing keeps the stored name, not the half-typed one', () => {
    // namingDefault is what the credential is actually stored under.
    // The dismiss label must read from it, or the button promises a name
    // that dismissing will not save.
    expect(src).toContain('Keep "{namingDefault}"');
    expect(src).not.toContain("Keep \"{namingValue || 'Passkey'}\"");
  });

  it('treats a failed rename as a rename failure, not a lost passkey', () => {
    // The credential is already enrolled and usable at this point; the copy
    // must not imply otherwise.
    expect(src).toContain('your passkey still works');
  });
});
