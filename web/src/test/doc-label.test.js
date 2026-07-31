import { describe, it, expect } from 'vitest';
import { docLabel } from '../lib/docLabel.js';

describe('governance doc labels', () => {
  it('strips the extension whichever one it is', () => {
    // The badge is text-transform: capitalize, so a surviving `.md` read as
    // part of the name: "To Tool Library Rules.Md".
    expect(docLabel('tool-library-rules.md')).toBe('tool library rules');
    expect(docLabel('governance-rules.json')).toBe('governance rules');
  });

  it('leaves a bare title alone', () => {
    expect(docLabel('Governance Rules')).toBe('Governance Rules');
  });

  it('only strips a trailing extension', () => {
    expect(docLabel('v1.0-notes.md')).toBe('v1.0 notes');
  });

  it('survives a missing name', () => {
    expect(docLabel(null)).toBe('');
    expect(docLabel(undefined)).toBe('');
  });
});
