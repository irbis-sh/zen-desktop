import { afterAll, beforeAll, describe, test } from '@jest/globals';
import * as CSSTree from 'css-tree';

import { tokenize } from './tokenize';

function tok(input: string): string {
  const ast = CSSTree.parse(input, { context: 'selectorList', positions: true }) as CSSTree.SelectorList;
  return tokenize(ast, input)
    .map((t) => t.map((t) => t.toString()).join(' '))
    .join(', ');
}

describe('tokenize', () => {
  test.each<[string, string]>([
    ['div', 'RawTok(div)'],
    ['*', 'RawTok(*)'],
    ['a[href^="http"]', 'RawTok(a[href^="http"])'],

    ['div>.x+span~a', 'RawTok(div) CombTok(>) RawTok(.x) CombTok(+) RawTok(span) CombTok(~) RawTok(a)'],

    ['div:contains(ad)', 'RawTok(div) ExtTok(:contains(ad))'],
    ['div.banner:matches-css(color: red)', 'RawTok(div.banner) ExtTok(:matches-css(color: red))'],
    [':matches-path(/^\\/shop/) .card', 'ExtTok(:matches-path(/^\\/shop/)) CombTok( ) RawTok(.card)'],
    ['div:upward(3)', 'RawTok(div) ExtTok(:upward(3))'],

    ['div:upward(3)~:contains(ad)', 'RawTok(div) ExtTok(:upward(3)) CombTok(~) ExtTok(:contains(ad))'],

    ['> .x:contains(y)', 'CombTok(>) RawTok(.x) ExtTok(:contains(y))'],

    ['div >', 'RawTok(div) CombTok(>)'],

    [':upward(1)+:upward(2)', 'ExtTok(:upward(1)) CombTok(+) ExtTok(:upward(2))'],

    // Selector lists in classes/pseudo-classes
    ['section:where(.x, .y)', 'RawTok(section:where(.x, .y))'],

    ['div, .banner', 'RawTok(div), RawTok(.banner)'],

    // Extended args always stay extended regardless of native support
    ['div:not(:contains(ad))', 'RawTok(div) ExtTok(:not(:contains(ad)))'],
    ['div:has(:upward(2))', 'RawTok(div) ExtTok(:has(:upward(2)))'],

    // -abp-has always stays extended (alias not natively optimizable)
    ['div:-abp-has(.ad)', 'RawTok(div) ExtTok(:-abp-has(.ad))'],
  ])('tokenize selector %j', (input, expected) => {
    expect(tok(input)).toEqual(expected);
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
      // :not/:has/:is with standard-only args -> merged into raw
      ['div:not(.ad)', 'RawTok(div:not(.ad))'],
      ['div :not(.ad)', 'RawTok(div) CombTok( ) RawTok(:not(.ad))'],
      ['div:has(span, strong)', 'RawTok(div:has(span, strong))'],
      ['div:is(.ad, .promo)', 'RawTok(div:is(.ad, .promo))'],

      // Nested optimizable: both :not and :has have standard-only args -> all native
      ['div:not(:has(.ad))', 'RawTok(div:not(:has(.ad)))'],

      // Nested non-optimizable: :not contains :has-text
      [':not(:has-text(test))', 'ExtTok(:not(:has-text(test)))'],
    ])('tokenize selector %j', (input, expected) => {
      expect(tok(input)).toEqual(expected);
    });
  });

  describe('with native :has/:is/:not support OFF', () => {
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
      // :not/:has/:is with standard-only args -> stays extended (no native support)
      ['div:not(.ad)', 'RawTok(div) ExtTok(:not(.ad))'],
      ['div :not(.ad)', 'RawTok(div) CombTok( ) ExtTok(:not(.ad))'],
      ['div:has(span, strong)', 'RawTok(div) ExtTok(:has(span, strong))'],
      ['div:is(.ad, .promo)', 'RawTok(div) ExtTok(:is(.ad, .promo))'],

      // Nested: falls back to extended
      ['div:not(:has(.ad))', 'RawTok(div) ExtTok(:not(:has(.ad)))'],
    ])('tokenize selector %j', (input, expected) => {
      expect(tok(input)).toEqual(expected);
    });
  });
});
