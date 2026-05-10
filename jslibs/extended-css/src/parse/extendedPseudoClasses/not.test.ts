import { describe, test, beforeEach, afterEach } from '@jest/globals';

import { Not } from './not';

describe(':not()', () => {
  let originalBody: string;

  beforeEach(() => {
    originalBody = document.body.innerHTML;
  });

  afterEach(() => {
    document.body.innerHTML = originalBody;
  });

  test('handles a simple selector', () => {
    document.body.innerHTML = `
        <div id="div1"></div>
        <div id="div2"></div>
      `;
    const selector = new Not('#div2');
    const input = [document.querySelector('#div1')!, document.querySelector('#div2')!];
    expect(selector.run(input)).toEqual([document.querySelector('#div1')]);
  });

  test('handles a selector list', () => {
    document.body.innerHTML = `
      <div id="div1"></div>
      <div id="div2"></div>
      <div id="div3"></div>
      <div id="div4"></div>
    `;
    const selector = new Not('#div3, #div4');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1'), document.querySelector('#div2')]);
  });

  test('matching with an extended CSS selector', () => {
    document.body.innerHTML = `
      <div id="div1"></div>
      <div id="div2">Ad</div>
      <div id="div3"><span>Ad</span></div>
      <div id="div4"><span>Advertisement</span></div>
    `;
    const selector = new Not(':has(span):contains(Ad)');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([document.querySelector('#div1'), document.querySelector('#div2')]);
  });

  test('returns an empty array on no match', () => {
    document.body.innerHTML = `
      <div data-ad></div>
      <div data-ad="123"></div>
      <div data-ad="321"></div>
      <blockquote>hey there</blockquote>
    `;
    const selector = new Not('[data-ad]');
    const input = Array.from(document.querySelectorAll('div'));
    expect(selector.run(input)).toEqual([]);
  });

  test('returns an empty array on empty input', () => {
    document.body.innerHTML = `
      <audio></audio>
      <video></video>
    `;
    const selector = new Not('.yes');
    expect(selector.run([])).toEqual([]);
  });
});
