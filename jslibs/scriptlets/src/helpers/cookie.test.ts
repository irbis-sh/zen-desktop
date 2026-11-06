import { describe, expect, test } from '@jest/globals';

import { parseCookieOptions, validateUntrustedCookieValue } from './cookie';

describe('validateUntrustedCookieValue', () => {
  test('accepts keywords from either engine', () => {
    expect(validateUntrustedCookieValue('granted')).toBe('granted');
    expect(validateUntrustedCookieValue('emptyArr')).toBe('[]');
    expect(validateUntrustedCookieValue('emptyObj')).toBe('{}');
  });

  test('matches keywords case-insensitively and keeps the original case', () => {
    expect(validateUntrustedCookieValue('ACCEPT')).toBe('ACCEPT');
  });

  test('accepts a quoted value and keeps the quotes', () => {
    expect(validateUntrustedCookieValue('"1"')).toBe('"1"');
    expect(validateUntrustedCookieValue('"yes"')).toBe('"yes"');
  });

  test('accepts integers from -32767 to 32767', () => {
    expect(validateUntrustedCookieValue('-32767')).toBe('-32767');
    expect(validateUntrustedCookieValue('32767')).toBe('32767');
  });

  test.each(['32768', '-32768', '', '""', 'flagmajob_ads_shown', '1.5'])('rejects %p', (value) => {
    expect(() => validateUntrustedCookieValue(value)).toThrow();
  });
});

describe('parseCookieOptions', () => {
  const defaults = { reload: false, dontOverwrite: false, domain: '' };

  test.each<[string[], object]>([
    [[], defaults],
    [['reload', '1'], { ...defaults, reload: true }],
    [['reload', '0'], defaults],
    [['reload', ''], defaults],
    [['dontOverwrite', '1'], { ...defaults, dontOverwrite: true }],
    [['domain', 'example.com', 'reload', '1'], { ...defaults, domain: 'example.com', reload: true }],
    [['example.com'], { ...defaults, domain: 'example.com' }],
    [['example.com', 'reload', '1'], { ...defaults, domain: 'example.com' }],
    // The empty first argument reads as a positional domain, so the reload is
    // dropped. uBO drops it too.
    [['', 'reload', '1'], defaults],
  ])('parses %p', (rest, want) => {
    expect(parseCookieOptions(rest)).toEqual(want);
  });
});
