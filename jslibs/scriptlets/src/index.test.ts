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
    delete (window as any).PROPERTY;
  });

  test.each(['set-constant', 'set', 'ubo-set'])('%s', (name) => {
    scriptlet(name, 'PROPERTY', 'true');

    expect((window as any).PROPERTY).toBe(true);
  });

  // Together with the rejection test below, each case fails if the name maps to
  // a neighbouring function: only the trusted scriptlets accept a JSON value, and
  // only the -reload ones reload without a reload option.
  test.each<[string, string[], string, boolean]>([
    ['set-cookie', ['consent', 'accept'], 'consent=accept', false],
    ['ubo-set-cookie', ['GPC', '1'], 'GPC=1', false],
    ['ubo-set-cookie', ['GPC', '1', '', 'reload', '1'], 'GPC=1', true],
    ['set-cookie-reload', ['consent', 'accept'], 'consent=accept', true],
    ['trusted-set-cookie', ['consent', '{"ads":false}'], 'consent={"ads":false}', false],
    ['trusted-set-cookie-reload', ['consent', '{"ads":false}'], 'consent={"ads":false}', true],
  ])('%s', (name, args, cookie, reloads) => {
    scriptlet(name, ...args);

    expect(document.cookie).toBe(cookie);
    expect(reloadPage).toHaveBeenCalledTimes(reloads ? 1 : 0);
  });

  test.each(['set-cookie', 'ubo-set-cookie', 'set-cookie-reload'])('%s rejects a JSON value', (name) => {
    expect(() => scriptlet(name, 'consent', '{"ads":false}')).toThrow('Invalid cookie value');
    expect(document.cookie).toBe('');
  });
});
