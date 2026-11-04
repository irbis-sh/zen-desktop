/**
 * @jest-environment-options {"url": "https://www.example.com/"}
 */
import { afterEach, describe, expect, jest, test } from '@jest/globals';

import { reloadPage } from './helpers/reload';
import { clearCookies } from './test/cookies';

import scriptlet from './index';

jest.mock('./helpers/reload', () => ({
  reloadPage: jest.fn(),
  isReloadedLoad: jest.fn(() => false),
}));

describe('scriptlet names', () => {
  afterEach(() => {
    jest.clearAllMocks();
    clearCookies();
  });

  // Each case fails if the name maps to a neighbouring function: only the -reload
  // ones reload without a reload option.
  test.each<[string, string[], string, boolean]>([
    ['set-cookie', ['consent', 'accept'], 'consent=accept', false],
    ['ubo-set-cookie', ['GPC', '1', '', 'reload', '1'], 'GPC=1', true],
    ['set-cookie-reload', ['consent', 'accept'], 'consent=accept', true],
  ])('%s', (name, args, cookie, reloads) => {
    scriptlet(name, ...args);

    expect(document.cookie).toBe(cookie);
    expect(reloadPage).toHaveBeenCalledTimes(reloads ? 1 : 0);
  });
});
