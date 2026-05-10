import { afterAll, beforeAll, describe, test, expect } from '@jest/globals';

import { parseRawSelectorList } from '.';

function plan(input: string): string {
  return parseRawSelectorList(input)
    .map((t) => t.map((t) => t.toString()).join(' '))
    .join(', ');
}

describe('parseSelectorList', () => {
  test.each<[string, string]>([
    ['div', 'RawQuery(div)'],
    ['div span', 'RawQuery(div span)'],
    ['a[href^="http"]', 'RawQuery(a[href^="http"])'],

    // Pure CSS with combinators is bridged into a single Raw
    ['div>.x+span~a', 'RawQuery(div>.x+span~a)'],

    // Extended pseudo classes split into steps
    ['div:contains(ad)', 'RawQuery(div) :Contains(ad)'],
    ['div.banner:matches-css(color: red)', 'RawQuery(div.banner) :MatchesCSS(color: red)'],
    [':matches-path(/^\\/shop/) .card', ':MatchesPath(/^\\/shop/) RawQuery(.card)'],
    ['div:upward(3)', 'RawQuery(div) :Upward(3)'],

    // Imperative bridging when a combinator is adjacent to an extended step
    ['div:upward(3)~:contains(ad)', 'RawQuery(div) :Upward(3) SubsSiblComb :Contains(ad)'],

    // Leading combinator in raw followed by extended
    ['.x:contains(y)', 'RawQuery(.x) :Contains(y)'],

    // Context bootstrap for leading extended step and imperative bridge
    [':upward(1)+:upward(2)', 'RawQuery(*) :Upward(1) NextSiblComb :Upward(2)'],

    // Non-extended pseudo remains in raw
    ['section:where(.x, .y)', 'RawQuery(section:where(.x, .y))'],

    // Relative selector
    ['> .x + p', 'ChildComb RawMatches(.x) NextSiblComb RawMatches(p)'],

    // Pseudo-class aliases (uBO and ABP compat)
    [
      'span:has-text(Promoted):-abp-contains(AD):-abp-has(.banner)',
      'RawQuery(span) :Contains(Promoted) :Contains(AD) :Has(...)',
    ],

    // Selector lists
    ['.banner, .ad', 'RawQuery(.banner), RawQuery(.ad)'],

    // Combinators between a mix of raw and extended tokens
    ['#parent:min-text-length(2) *', 'RawQuery(#parent) :MinTextLength(2) RawQuery(*)'],
    [':min-text-length(2) > div', 'RawQuery(*) :MinTextLength(2) RawQuery(:scope>div)'],
    [':min-text-length(2) + div', 'RawQuery(*) :MinTextLength(2) NextSiblComb RawMatches(div)'],
  ])('parse %j', (input, expected) => {
    expect(plan(input)).toEqual(expected);
  });

  test('throws on dangling combinator', () => {
    expect(() => parseRawSelectorList('div >')).toThrow(/dangling combinator/i);
  });

  describe('with native :has/:is/:not support ON', () => {
    let origCSS: typeof window.CSS;

    beforeAll(() => {
      origCSS = window.CSS;
      window.CSS = {
        supports: () => true,
        escape: (s: string) => s,
      } as unknown as typeof CSS;
    });

    afterAll(() => {
      window.CSS = origCSS;
    });

    test.each<[string, string]>([
      // :not with standard-only args -> single RawQuery
      ['div:not(.ad)', 'RawQuery(div:not(.ad))'],

      // :has with standard-only args -> merged into raw
      [
        'div:contains(ad) span:has(.banner), > .x + p, code',
        'RawQuery(div) :Contains(ad) RawQuery(span:has(.banner)), ChildComb RawMatches(.x) NextSiblComb RawMatches(p), RawQuery(code)',
      ],

      // :is with standard-only args -> stays in raw run
      [
        ':min-text-length(2) + div > div:is(.class) + span',
        'RawQuery(*) :MinTextLength(2) NextSiblComb RawMatches(div) RawQuery(:scope>div:is(.class)+span)',
      ],

      // :has with non-standard arg -> extended
      [':has(:min-text-length(42))', 'RawQuery(*) :Has(...)'],
    ])('parse %j', (input, expected) => {
      expect(plan(input)).toEqual(expected);
    });
  });

  describe('with native support OFF', () => {
    let origCSS: typeof window.CSS;

    beforeAll(() => {
      origCSS = window.CSS;
      window.CSS = {
        supports: () => false,
        escape: (s: string) => s,
      } as unknown as typeof CSS;
    });

    afterAll(() => {
      window.CSS = origCSS;
    });

    test.each<[string, string]>([
      // :not with standard-only args -> extended path
      ['div:not(.ad)', 'RawQuery(div) :Not(...)'],

      // :has with standard-only args -> extended path
      [
        'div:contains(ad) span:has(.banner), > .x + p, code',
        'RawQuery(div) :Contains(ad) RawQuery(span) :Has(...), ChildComb RawMatches(.x) NextSiblComb RawMatches(p), RawQuery(code)',
      ],

      // :is with standard-only args -> extended path
      [
        ':min-text-length(2) + div > div:is(.class) + span',
        'RawQuery(*) :MinTextLength(2) NextSiblComb RawMatches(div) RawQuery(:scope>div) :Is(...) NextSiblComb RawMatches(span)',
      ],
    ])('parse %j', (input, expected) => {
      expect(plan(input)).toEqual(expected);
    });
  });
});
