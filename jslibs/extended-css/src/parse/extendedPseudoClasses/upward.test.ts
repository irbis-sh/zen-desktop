import { describe, test, beforeAll, afterAll } from '@jest/globals';

import { Upward } from './upward';

describe('Upward', () => {
  let originalBody: string;
  let span: Element;

  beforeAll(() => {
    originalBody = document.body.innerHTML;

    document.body.innerHTML = `
      <div id="grandparent">
        <div id="parent" data-e2e-id="ps">
          <span></span>
        </div>

        <b data-e2e-id="ps"></b>
      </div>
    `;
    span = document.querySelector('span')!;
  });

  afterAll(() => {
    document.body.innerHTML = originalBody;
  });

  test('matches ancestor by distance', () => {
    let selector = new Upward('1');
    const input = [span];
    expect(selector.run(input)).toEqual([document.querySelector('#parent')]);

    selector = new Upward('2');
    expect(selector.run(input)).toEqual([document.querySelector('#grandparent')]);
  });

  test('skips element if distance is further than the root element', () => {
    const selector = new Upward('42');
    const input = [span];
    expect(selector.run(input)).toEqual([]);
  });

  test('matches ancestor by query', () => {
    let selector = new Upward('[data-e2e-id=ps]');
    const input = [span];
    expect(selector.run(input)).toEqual([document.querySelector('div[data-e2e-id=ps]')]);

    selector = new Upward('#grandparent');
    expect(selector.run(input)).toEqual([document.querySelector('#grandparent')]);
  });
});
