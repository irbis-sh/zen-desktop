/**
 * @jest-environment-options {"url": "https://www.example.com/"}
 */
import { afterEach, beforeEach, describe, expect, jest, test } from '@jest/globals';

import { isReloadedLoad, reloadPage } from './helpers/reload';
import { setCookie, setCookieReload } from './set-cookie';
import { clearCookies } from './test/cookies';

jest.mock('./helpers/reload', () => ({
  reloadPage: jest.fn(),
  isReloadedLoad: jest.fn(() => false),
}));

let cookieSetter: jest.SpiedSetter<string>;

beforeEach(() => {
  cookieSetter = jest.spyOn(Document.prototype, 'cookie', 'set');
});

afterEach(() => {
  jest.restoreAllMocks();
  jest.clearAllMocks();
  clearCookies();
});

describe('set-cookie', () => {
  test('sets the cookie with path=/', () => {
    setCookie('consent', 'accept');

    expect(cookieSetter).toHaveBeenLastCalledWith('consent=accept; path=/');
    expect(document.cookie).toBe('consent=accept');
  });

  test('sets no path attribute for path none', () => {
    setCookie('consent', 'accept', 'none');

    expect(cookieSetter).toHaveBeenLastCalledWith('consent=accept');
  });

  test('throws for any other path', () => {
    expect(() => setCookie('consent', 'accept', '/foo')).toThrow();
    expect(cookieSetter).not.toHaveBeenCalled();
  });

  test('throws for a value outside the safelist', () => {
    expect(() => setCookie('consent', 'flagmajob_ads_shown')).toThrow();
    expect(cookieSetter).not.toHaveBeenCalled();
  });

  test('writes a name with non-token characters as given', () => {
    setCookie('tc:dismissexitintentpopup', 'true');

    expect(document.cookie).toBe('tc:dismissexitintentpopup=true');
  });

  test.each(['', ' ', '\t', 'x=any; max-age=31536000; y', 'a;b', 'a=b'])('throws for the name %p', (name) => {
    expect(() => setCookie(name, '1')).toThrow();
    expect(cookieSetter).not.toHaveBeenCalled();
  });

  test.each([' consent', 'consent ', '\tconsent'])('strips spaces and tabs around the name %p', (name) => {
    setCookie(name, '1');

    expect(cookieSetter).toHaveBeenLastCalledWith('consent=1; path=/');
  });

  test('keeps a non-breaking space in the name', () => {
    setCookie('\u00a0consent', '1');

    expect(cookieSetter).toHaveBeenLastCalledWith('\u00a0consent=1; path=/');
  });

  test('finds an existing cookie under the stripped name', () => {
    document.cookie = 'consent=reject; path=/';

    setCookie(' consent', 'accept', '', 'dontOverwrite', '1');

    expect(document.cookie).toBe('consent=reject');
  });

  test('sets a domain cookie with a uBO domain option', () => {
    setCookie('consent', '1', '', 'domain', 'example.com');

    expect(cookieSetter).toHaveBeenLastCalledWith('consent=1; path=/; domain=example.com');
    expect(document.cookie).toBe('consent=1');
  });

  test('sets a domain cookie with an AdGuard positional domain', () => {
    setCookie('consent', '1', '/', 'example.com');

    expect(cookieSetter).toHaveBeenLastCalledWith('consent=1; path=/; domain=example.com');
  });

  test.each([
    ['uBO', ['domain', 'example.com; max-age=31536000']],
    ['AdGuard', ['example.com; path=/admin']],
  ])('throws for a %s domain that is not a hostname', (_, rest) => {
    expect(() => setCookie('consent', '1', '', ...rest)).toThrow();
    expect(cookieSetter).not.toHaveBeenCalled();
  });

  test('gives a __Host- cookie Secure and path=/, and no domain', () => {
    setCookie('__Host-consent', '1', 'none', 'example.com');

    expect(cookieSetter).toHaveBeenLastCalledWith('__Host-consent=1; path=/; Secure');
    expect(document.cookie).toBe('__Host-consent=1');
  });

  test('matches the __Host- prefix case-insensitively', () => {
    setCookie('__host-consent', '1', 'none', 'example.com');

    expect(cookieSetter).toHaveBeenLastCalledWith('__host-consent=1; path=/; Secure');
  });

  test('gives a __Secure- cookie Secure and keeps its domain', () => {
    setCookie('__Secure-consent', '1', '', 'example.com');

    expect(cookieSetter).toHaveBeenLastCalledWith('__Secure-consent=1; path=/; domain=example.com; Secure');
  });

  test('keeps an existing cookie with dontOverwrite', () => {
    document.cookie = 'consent=reject; path=/';

    setCookie('consent', 'accept', '', 'dontOverwrite', '1');

    expect(document.cookie).toBe('consent=reject');
  });

  test('overwrites an existing cookie without dontOverwrite', () => {
    document.cookie = 'consent=reject; path=/';

    setCookie('consent', 'accept');

    expect(document.cookie).toBe('consent=accept');
  });

  test('does not reload without the reload option', () => {
    setCookie('consent', 'accept');

    expect(reloadPage).not.toHaveBeenCalled();
  });
});

describe('set-cookie reload', () => {
  test('reloads once when the cookie is new', () => {
    setCookie('consent', 'accept', '', 'reload', '1');

    expect(document.cookie).toBe('consent=accept');
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });

  test('neither writes nor reloads when the cookie is already set', () => {
    document.cookie = 'consent=accept; path=/';
    cookieSetter.mockClear();

    setCookie('consent', 'accept', '', 'reload', '1');

    expect(cookieSetter).not.toHaveBeenCalled();
    expect(reloadPage).not.toHaveBeenCalled();
  });

  test('reloads when the cookie has a different value', () => {
    document.cookie = 'consent=reject; path=/';

    setCookie('consent', 'accept', '', 'reload', '1');

    expect(document.cookie).toBe('consent=accept');
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });

  test('does not reload when the browser rejects the cookie', () => {
    setCookie('consent', 'accept', '', 'domain', 'other.com', 'reload', '1');

    expect(document.cookie).toBe('');
    expect(reloadPage).not.toHaveBeenCalled();
  });

  test('writes but does not reload on a reloaded load', () => {
    jest.mocked(isReloadedLoad).mockReturnValueOnce(true);

    setCookie('consent', 'accept', '', 'reload', '1');

    expect(document.cookie).toBe('consent=accept');
    expect(reloadPage).not.toHaveBeenCalled();
  });

  test('set-cookie-reload reloads without trailing options', () => {
    setCookieReload('consent', 'accept');

    expect(document.cookie).toBe('consent=accept');
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });

  test('set-cookie-reload keeps an AdGuard positional domain', () => {
    setCookieReload('consent', '2', '/', 'example.com');

    expect(cookieSetter).toHaveBeenLastCalledWith('consent=2; path=/; domain=example.com');
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });
});
