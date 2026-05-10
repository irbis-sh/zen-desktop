import { describe, test, beforeEach, afterEach, expect } from '@jest/globals';

import { MatchesAttr } from './matchesAttr';

describe(':matches-attr()', () => {
  let originalBody: string;

  beforeEach(() => {
    originalBody = document.body.innerHTML;
  });

  afterEach(() => {
    document.body.innerHTML = originalBody;
  });

  test('matches by attribute name only', () => {
    document.body.innerHTML = `
      <div id="div1" data-ad></div>
      <div id="div2"></div>
      <div id="div3" ad-link></div>
    `;
    const selector = new MatchesAttr('data-ad');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1')]);
  });

  test('matches by attribute name and value', () => {
    document.body.innerHTML = `
      <div id="div1" data-ad="banner"></div>
      <div id="div2" data-ad="popup"></div>
      <div id="div3"></div>
    `;
    const selector = new MatchesAttr('data-ad=banner');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1')]);
  });

  test('handles quoted attribute names and values', () => {
    document.body.innerHTML = `
      <div id="div1" data-ad="banner"></div>
      <div id="div2" custom-attr="special value"></div>
    `;
    const selector = new MatchesAttr('"data-ad"="banner"');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1')]);
  });

  test('handles single quoted attribute names and values', () => {
    document.body.innerHTML = `
      <div id="div1" data-ad="banner"></div>
      <div id="div2" custom-attr="special value"></div>
    `;
    const selector = new MatchesAttr("'data-ad'='banner'");
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1')]);
  });

  test('matches with wildcard in attribute name', () => {
    document.body.innerHTML = `
      <div id="div1" data-ad="banner"></div>
      <div id="div2" random-ad-unit="popup"></div>
      <div id="div3" something="else"></div>
    `;
    const selector = new MatchesAttr('*ad*');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1'), document.querySelector('#div2')]);
  });

  test('matches with wildcard in attribute value', () => {
    document.body.innerHTML = `
      <div id="div1" data-type="banner-ad"></div>
      <div id="div2" data-type="ad-popup"></div>
      <div id="div3" data-type="not-ad-totally"></div>
      <div id="div4" data-type="normal"></div>
    `;
    const selector = new MatchesAttr('data-type=*ad*');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([
      document.querySelector('#div1'),
      document.querySelector('#div2'),
      document.querySelector('#div3'),
    ]);
  });

  test('matches with regexp for attribute name', () => {
    document.body.innerHTML = `
      <div id="div1" data123="value"></div>
      <div id="div2" data456="value"></div>
      <div id="div3" other="value"></div>
    `;
    const selector = new MatchesAttr('/^data\\d+$/');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1'), document.querySelector('#div2')]);
  });

  test('matches with regexp for attribute value', () => {
    document.body.innerHTML = `
      <div id="div1" data-size="240x400"></div>
      <div id="div2" data-size="300x250"></div>
      <div id="div3" data-size="text"></div>
    `;
    const selector = new MatchesAttr('data-size=/^\\d+x\\d+$/');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1'), document.querySelector('#div2')]);
  });

  test('matches with regexp for both name and value', () => {
    document.body.innerHTML = `
      <div id="div1" data-ad-unit="banner-1"></div>
      <div id="div2" ad-data-unit="popup-2"></div>
      <div id="div3" other="value-3"></div>
    `;
    const selector = new MatchesAttr('/.*ad.*unit/=/.*-\\d$/');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1'), document.querySelector('#div2')]);
  });

  test("matches examples from AdGuard's documentation", () => {
    document.body.innerHTML = `
      <div id="target1" ad-link="1random23-banner_240x400"></div>
      <div id="target2" data-1random23="adBanner"></div>
      <div id="target3" random123-unit094="click"></div>
      <div>
        <inner-random23 id="target4" nt4f5be90delay="1000"></inner-random23>
      </div>
      <div id="non-target"></div>
    `;

    const selector1 = new MatchesAttr('"ad-link"');
    const selector2 = new MatchesAttr('"data-*"="adBanner"');
    const selector3 = new MatchesAttr('*unit*=/^click$/');
    const selector4 = new MatchesAttr('"/.{5,}delay$/"="/^[0-9]*$/"');

    const input = Array.from(document.querySelectorAll('*')).filter((el) => el.id);

    expect(selector1.run(input)).toEqual([document.querySelector('#target1')]);
    expect(selector2.run(input)).toEqual([document.querySelector('#target2')]);
    expect(selector3.run(input)).toEqual([document.querySelector('#target3')]);
    expect(selector4.run(input)).toEqual([document.querySelector('#target4')]);
  });

  test('returns empty array for empty input', () => {
    const selector = new MatchesAttr('data-ad');
    expect(selector.run([])).toEqual([]);
  });

  test('returns empty array when no element matches', () => {
    document.body.innerHTML = `
      <div id="div1" data-type="banner"></div>
      <div id="div2" class="ad"></div>
    `;
    const selector = new MatchesAttr('data-ad');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([]);
  });

  test('throws on empty argument', () => {
    expect(() => new MatchesAttr('')).toThrow();
  });
});
