import { afterEach, describe, expect, test } from '@jest/globals';

import { isReloadedLoad } from './reload';

describe('isReloadedLoad', () => {
  afterEach(() => {
    delete (performance as any).getEntriesByType;
  });

  test.each<[object[], boolean]>([
    [[{ type: 'reload' }], true],
    [[{ type: 'navigate' }], false],
    [[{ type: 'back_forward' }], false],
    [[], false],
  ])('for navigation entries %p returns %p', (entries, want) => {
    // jsdom does not implement getEntriesByType.
    (performance as any).getEntriesByType = (type: string) => (type === 'navigation' ? entries : []);

    expect(isReloadedLoad()).toBe(want);
  });
});
