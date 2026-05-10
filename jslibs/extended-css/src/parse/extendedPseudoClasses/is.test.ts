import { describe, test, beforeEach, afterEach, expect } from '@jest/globals';

import { Is } from './is';

describe(':is()', () => {
  let originalBody: string;

  beforeEach(() => {
    originalBody = document.body.innerHTML;
  });

  afterEach(() => {
    document.body.innerHTML = originalBody;
  });

  test('matches elements by single selector', () => {
    document.body.innerHTML = `
      <div id="target1" class="inner">Match</div>
      <div id="target2" class="outer">No match</div>
      <span id="target3" class="inner">Match</span>
    `;

    const selector = new Is('.inner');
    const input = [
      document.querySelector('#target1')!,
      document.querySelector('#target2')!,
      document.querySelector('#target3')!,
    ];
    expect(selector.run(input)).toEqual([document.querySelector('#target1'), document.querySelector('#target3')]);
  });

  test('matches elements by multiple selectors (OR semantics)', () => {
    document.body.innerHTML = `
      <div id="target1" class="inner">Match by class</div>
      <div id="target2" class="outer">No match</div>
      <div id="target3">Match by id</div>
      <span id="target4" class="footer">Match by class</span>
    `;

    const selector = new Is('.inner, #target3, .footer');
    const input = [
      document.querySelector('#target1')!,
      document.querySelector('#target2')!,
      document.querySelector('#target3')!,
      document.querySelector('#target4')!,
    ];
    expect(selector.run(input)).toEqual([
      document.querySelector('#target1'),
      document.querySelector('#target3'),
      document.querySelector('#target4'),
    ]);
  });

  test('matches elements by tag selectors', () => {
    document.body.innerHTML = `
      <div id="target1">Match</div>
      <span id="target2">Match</span>
      <p id="target3">No match</p>
    `;

    const selector = new Is('div, span');
    const input = [
      document.querySelector('#target1')!,
      document.querySelector('#target2')!,
      document.querySelector('#target3')!,
    ];
    expect(selector.run(input)).toEqual([document.querySelector('#target1'), document.querySelector('#target2')]);
  });

  test('matches elements by attribute selectors', () => {
    document.body.innerHTML = `
      <div id="target1" data-test="true">Match</div>
      <div id="target2" aria-label="button">Match</div>
      <div id="target3">No match</div>
    `;

    const selector = new Is('[data-test], [aria-label]');
    const input = [
      document.querySelector('#target1')!,
      document.querySelector('#target2')!,
      document.querySelector('#target3')!,
    ];
    expect(selector.run(input)).toEqual([document.querySelector('#target1'), document.querySelector('#target2')]);
  });

  test('handles complex selectors with pseudo-classes', () => {
    document.body.innerHTML = `
      <div id="target1" class="first">Match</div>
      <div id="target2" class="second">No match</div>
      <div id="target3" class="first">Match</div>
    `;

    const selector = new Is('.first:first-child, .first:last-child');
    const input = [
      document.querySelector('#target1')!,
      document.querySelector('#target2')!,
      document.querySelector('#target3')!,
    ];
    expect(selector.run(input)).toEqual([document.querySelector('#target1'), document.querySelector('#target3')]);
  });

  test('returns empty array if no match', () => {
    document.body.innerHTML = `
      <div id="target1" class="nomatch">No match</div>
      <span id="target2">No match</span>
    `;

    const selector = new Is('.does-not-exist, #missing');
    const input = [document.querySelector('#target1')!, document.querySelector('#target2')!];
    expect(selector.run(input)).toEqual([]);
  });

  test('returns empty array for empty input', () => {
    document.body.innerHTML = `<div class="test">Test</div>`;

    const selector = new Is('.test');
    expect(selector.run([])).toEqual([]);
  });

  test('handles whitespace in selector list', () => {
    document.body.innerHTML = `
      <div id="target1" class="inner">Match</div>
      <div id="target2" class="footer">Match</div>
      <div id="target3" class="other">No match</div>
    `;

    const selector = new Is(' .inner , .footer ');
    const input = [
      document.querySelector('#target1')!,
      document.querySelector('#target2')!,
      document.querySelector('#target3')!,
    ];
    expect(selector.run(input)).toEqual([document.querySelector('#target1'), document.querySelector('#target2')]);
  });

  test('gracefully handles invalid selectors', () => {
    document.body.innerHTML = `
      <div id="target1" class="valid">Match</div>
      <div id="target2" class="other">No match</div>
    `;

    // Invalid selector syntax should be skipped
    const selector = new Is('.valid, :invalid-pseudo');
    const input = [document.querySelector('#target1')!, document.querySelector('#target2')!];
    expect(selector.run(input)).toEqual([document.querySelector('#target1')]);
  });

  test('handles nested container scenario from documentation', () => {
    document.body.innerHTML = `
      <div id="container">
        <div data="true">
          <div>
            <div id="target1" class="inner"></div>
          </div>
        </div>
        <div class="footer">Not selected</div>
        <span class="inner">Not in container path</span>
      </div>
    `;

    const selector = new Is('.inner, .footer');
    const containerElements = Array.from(document.querySelectorAll('#container *'));
    const result = selector.run(containerElements);

    expect(result).toContain(document.querySelector('#target1'));
    expect(result).toContain(document.querySelector('.footer'));
    expect(result).toContain(document.querySelector('span.inner'));
  });

  test('matches elements with compound selectors', () => {
    document.body.innerHTML = `
      <div id="target1" class="box primary">Match</div>
      <div id="target2" class="box">No match</div>
      <div id="target3" class="primary">No match</div>
      <span id="target4" class="box primary">Match</span>
    `;

    const selector = new Is('.box.primary, span.box');
    const input = [
      document.querySelector('#target1')!,
      document.querySelector('#target2')!,
      document.querySelector('#target3')!,
      document.querySelector('#target4')!,
    ];
    expect(selector.run(input)).toEqual([document.querySelector('#target1'), document.querySelector('#target4')]);
  });

  test('handles universal selector', () => {
    document.body.innerHTML = `
      <div id="target1">Match</div>
      <span id="target2">Match</span>
      <p id="target3">Match</p>
    `;

    const selector = new Is('*');
    const input = [
      document.querySelector('#target1')!,
      document.querySelector('#target2')!,
      document.querySelector('#target3')!,
    ];
    expect(selector.run(input)).toEqual(input);
  });
});
