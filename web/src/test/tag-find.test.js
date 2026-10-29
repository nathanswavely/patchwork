import { describe, it, expect } from 'vitest';
import { findTags } from '../lib/tagFind.js';

// The one find rule the desktop filter card and the phone sheet share.
describe('findTags', () => {
  const tags = ['community', 'music', 'ceramics', 'theater', 'Gallery'];

  it('keeps the list and its order when nothing is typed', () => {
    expect(findTags(tags, '')).toEqual(tags);
    expect(findTags(tags, '   ')).toEqual(tags);
  });

  it('matches anywhere in the word, ignoring case and outer spaces', () => {
    expect(findTags(tags, ' ER ')).toEqual(['ceramics', 'theater', 'Gallery']);
  });

  it('returns nothing for a word nobody wears', () => {
    expect(findTags(tags, 'zzz')).toEqual([]);
  });
});
