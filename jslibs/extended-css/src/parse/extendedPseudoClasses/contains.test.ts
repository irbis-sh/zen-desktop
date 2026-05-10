import { describe, test, beforeAll, afterAll, expect } from '@jest/globals';

import { Contains } from './contains';

describe('Contains', () => {
  let originalBody: string;
  let divs: Element[];

  beforeAll(() => {
    originalBody = document.body.innerHTML;
    document.body.innerHTML = `
      <div id="one">Hello world</div>
      <div id="two">Foo <span>bar</span></div>
      <div id="three">Special chars: [abc]</div>
      <div id="four">Regex test 123</div>
      <div id="five"></div>
      <div id="six">Bye world</div>
    `;
    divs = Array.from(document.querySelectorAll('div'));
  });

  afterAll(() => {
    document.body.innerHTML = originalBody;
  });

  test('matches elements containing a plain string', () => {
    let selector = new Contains('Hello');
    expect(selector.run(divs)).toEqual([document.querySelector('#one')]);

    selector = new Contains('world');
    const res = selector.run(divs);
    expect(res).toContain(document.querySelector('#one'));
    expect(res).toContain(document.querySelector('#six'));
  });

  test('matches elements containing a substring', () => {
    const selector = new Contains('bar');
    expect(selector.run(divs)).toEqual([document.querySelector('#two')]);
  });

  test('matches elements containing special characters', () => {
    const selector = new Contains('[abc]');
    expect(selector.run(divs)).toEqual([document.querySelector('#three')]);
  });

  test('matches elements using a regexp literal', () => {
    const selector = new Contains('/test \\d+/');
    expect(selector.run(divs)).toEqual([document.querySelector('#four')]);
  });

  test('matches elements using a regexp with flags', () => {
    const selector = new Contains('/hello/i');
    expect(selector.run(divs)).toEqual([document.querySelector('#one')]);
  });

  test('returns empty array if no match', () => {
    const selector = new Contains('notfound');
    expect(selector.run(divs)).toEqual([]);
  });

  test('returns empty array for empty elements', () => {
    const selector = new Contains('anything');
    expect(selector.run([document.querySelector('#five')!])).toEqual([]);
  });
});
