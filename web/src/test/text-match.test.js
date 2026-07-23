import { describe, it, expect } from 'vitest';
import { fold, textMatches } from '../lib/textMatch.js';

describe('diacritic-insensitive matching', () => {
  it('folds combining marks and case', () => {
    expect(fold('Tornādo Tornädo')).toBe('tornado tornado');
    expect(fold('Café Señor')).toBe('cafe senor');
    expect(fold('')).toBe('');
    expect(fold(null)).toBe('');
  });

  it('matches plain-keyboard queries against decorated names', () => {
    expect(textMatches('Tornādo Tornädo', 'tornado')).toBe(true);
    expect(textMatches('Zoetropolis Cinema Stillhouse', 'ZOETROP')).toBe(true);
    expect(textMatches('Mill 72', 'tornado')).toBe(false);
  });

  it('folds the query side too', () => {
    expect(textMatches('Tornado Alley', 'tornādo')).toBe(true);
  });

  it('treats an empty query as a match', () => {
    expect(textMatches('anything', '')).toBe(true);
  });
});
