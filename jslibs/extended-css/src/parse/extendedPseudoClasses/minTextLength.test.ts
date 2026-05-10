import { describe, test, beforeEach, afterEach, expect } from '@jest/globals';

import { MinTextLength } from './minTextLength';

describe(':MinTextLength()', () => {
  let originalBody: string;

  beforeEach(() => {
    originalBody = document.body.innerHTML;
  });

  afterEach(() => {
    document.body.innerHTML = originalBody;
  });

  test('filters elements by minimum text length', () => {
    document.body.innerHTML = `
      <div id="t1">hi</div>
      <div id="t2">hello</div>
      <div id="t3"></div>
    `;

    const selector = new MinTextLength('3');
    const input = [document.querySelector('#t1')!, document.querySelector('#t2')!, document.querySelector('#t3')!];

    expect(selector.run(input)).toEqual([document.querySelector('#t2')]);
  });

  test('includes elements at boundary length and excludes below', () => {
    document.body.innerHTML = `
      <div id="t1">a</div>
      <div id="t2">ab</div>
      <div id="t3"></div>
    `;

    const selector = new MinTextLength('1');
    const input = [document.querySelector('#t1')!, document.querySelector('#t2')!, document.querySelector('#t3')!];

    expect(selector.run(input)).toEqual([document.querySelector('#t1'), document.querySelector('#t2')]);
  });

  test('includes nested text content when computing length', () => {
    document.body.innerHTML = `
      <div id="t4">a<span>bc</span>d</div>
      <div id="t5"><span>123</span></div>
      <div id="t6">ab</div>
    `;

    const selector = new MinTextLength('3');
    const input = [document.querySelector('#t4')!, document.querySelector('#t5')!, document.querySelector('#t6')!];

    expect(selector.run(input)).toEqual([document.querySelector('#t4'), document.querySelector('#t5')]);
  });

  test('counts whitespace literally', () => {
    document.body.innerHTML = `
      <div id="ws1">  </div>
      <div id="ws2"> x </div>
      <div id="ws3">y</div>
    `;

    const selector = new MinTextLength('2');
    const input = [document.querySelector('#ws1')!, document.querySelector('#ws2')!, document.querySelector('#ws3')!];

    expect(selector.run(input)).toEqual([document.querySelector('#ws1'), document.querySelector('#ws2')]);
  });

  test('returns empty array for empty input', () => {
    const selector = new MinTextLength('1');
    expect(selector.run([])).toEqual([]);
  });

  test('returns empty array when no element meets the threshold', () => {
    document.body.innerHTML = `
      <div id="a">a</div>
      <div id="b"></div>
    `;
    const selector = new MinTextLength('5');
    const input = [document.querySelector('#a')!, document.querySelector('#b')!];
    expect(selector.run(input)).toEqual([]);
  });

  test('throws on invalid arguments', () => {
    expect(() => new MinTextLength('abc')).toThrow();
    expect(() => new MinTextLength('')).toThrow();
    expect(() => new MinTextLength('0')).toThrow();
    expect(() => new MinTextLength('-5')).toThrow();
  });
});
