import { describe, test, beforeEach, afterEach } from '@jest/globals';

import { Has } from './has';

describe(':has()', () => {
  let originalBody: string;

  beforeEach(() => {
    originalBody = document.body.innerHTML;
  });

  afterEach(() => {
    document.body.innerHTML = originalBody;
  });

  test('matches descendant by simple selector', () => {
    document.body.innerHTML = `
      <div id="desc1">Not selected</div>
      <div id="desc2">Selected
        <span class="banner">inner element</span>
      </div>
    `;

    const selector = new Has('.banner');
    const input = [document.querySelector('#desc1')!, document.querySelector('#desc2')!];
    expect(selector.run(input)).toEqual([document.querySelector('#desc2')]);
  });

  test('matches direct child using ">" combinator', () => {
    document.body.innerHTML = `
      <div id="child1">
        <div>
          <p class="banner">Has a nested .banner</p>
        </div>
      </div>
      <div id="child2">
        <p class="banner">child element</p>
      </div>
    `;

    const selector = new Has('> .banner');
    const input = [document.querySelector('#child1')!, document.querySelector('#child2')!];
    expect(selector.run(input)).toEqual([document.querySelector('#child2')]);
  });

  test('matches next sibling using "+" combinator', () => {
    document.body.innerHTML = `
      <div id="adj1">Not selected</div>
      <div id="adj2">Selected</div>
      <p class="banner">adjacent sibling</p>
      <span>Not selected</span>
    `;

    const selector = new Has('+ .banner');
    const input = [document.querySelector('#adj1')!, document.querySelector('#adj2')!];
    expect(selector.run(input)).toEqual([document.querySelector('#adj2')]);
  });

  test('matches subsequent sibling using "~" combinator', () => {
    document.body.innerHTML = `
      <div id="gen1">Not selected</div>
      <div id="gen2">Selected</div>
      <p class="banner" id="gen-banner">general sibling</p>
    `;

    const selector = new Has('~ .banner');
    const input = [document.querySelector('#gen1')!, document.querySelector('#gen2')!];
    expect(selector.run(input)).toEqual(input);
  });

  test('supports selector list separated by commas (OR semantics)', () => {
    document.body.innerHTML = `
      <div id="either1"><span>child span</span></div>
      <div id="either2"><p class="banner">child .banner</p></div>
      <div id="either3">No match</div>
      <div id="eitherBoth">
        <span>child span</span>
        <p class="banner">child .banner</p>
      </div>
    `;

    const selector = new Has('span, .banner');
    const input = [
      document.querySelector('#either1')!,
      document.querySelector('#either2')!,
      document.querySelector('#either3')!,
      document.querySelector('#eitherBoth')!,
    ];
    expect(selector.run(input)).toEqual([
      document.querySelector('#either1'),
      document.querySelector('#either2'),
      document.querySelector('#eitherBoth'),
    ]);
  });

  test('returns empty array if no match', () => {
    document.body.innerHTML = `
      <div id="either1">No matching element</div>
    `;

    const selector = new Has('.does-not-exist');
    const input = [document.querySelector('#either1')!];
    expect(selector.run(input)).toEqual([]);
  });

  test('does not match elements without scoped match', () => {
    document.body.innerHTML = `
      <div id="outside">Outside</div>
      <p class="banner" id="outside-banner">outside banner</p>
    `;

    const selector = new Has('.banner');
    const input = [document.querySelector('#outside')!];
    expect(selector.run(input)).toEqual([]);
  });
});
