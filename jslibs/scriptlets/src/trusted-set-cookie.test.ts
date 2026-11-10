/**
 * @jest-environment-options {"url": "https://www.example.com/"}
 */
import { afterEach, beforeEach, describe, expect, jest, test } from '@jest/globals';

import { reloadPage } from './helpers/reload';
import { clearCookies } from './test/cookies';
import { trustedSetCookie, trustedSetCookieReload } from './trusted-set-cookie';

jest.mock('./helpers/reload', () => ({
  reloadPage: jest.fn(),
  isReloadedLoad: jest.fn(() => false),
}));

let cookieSetter: jest.SpiedSetter<string>;

beforeEach(() => {
  cookieSetter = jest.spyOn(Document.prototype, 'cookie', 'set');
});

afterEach(() => {
  jest.useRealTimers();
  jest.restoreAllMocks();
  jest.clearAllMocks();
  clearCookies();
});

describe('trusted-set-cookie', () => {
  test('sets an arbitrary JSON value as written', () => {
    trustedSetCookie('consent', '{"necessary":true,"ads":false}');

    expect(document.cookie).toBe('consent={"necessary":true,"ads":false}');
  });

  test('sets an empty value', () => {
    trustedSetCookie('consent', '');

    expect(cookieSetter).toHaveBeenLastCalledWith('consent=; path=/');
  });

  test.each(['', 'a;b', 'a=b'])('throws for the name %p', (name) => {
    expect(() => trustedSetCookie(name, '1')).toThrow();
    expect(cookieSetter).not.toHaveBeenCalled();
  });

  test('writes a name with non-token characters as given', () => {
    trustedSetCookie('tc:cookies', '1');

    expect(document.cookie).toBe('tc:cookies=1');
  });

  test('replaces $now$ inside the value', () => {
    const before = Date.now();
    trustedSetCookie('consent', '{"timestamp":$now$}');

    const match = /^consent=\{"timestamp":(\d+)\}$/.exec(document.cookie);
    expect(match).not.toBeNull();
    expect(Number(match![1])).toBeGreaterThanOrEqual(before);
  });

  test('replaces $currentDate$ with Date.prototype.toString()', () => {
    // Decoding undoes the encoding of a localised time zone name, such as `ä`.
    const now = new Date('2026-11-03T12:00:00Z');
    jest.useFakeTimers({ now });

    trustedSetCookie('consent', '$currentDate$');

    expect(decodeURIComponent(document.cookie)).toBe(`consent=${now.toString()}`);
  });

  test('replaces $currentISODate$ inside the value', () => {
    trustedSetCookie('consent', 'at=$currentISODate$');

    expect(document.cookie).toMatch(/^consent=at=\d{4}-\d\d-\d\dT[\d:.]+Z$/);
  });

  test('sets an expiry one calendar day ahead for 1day', () => {
    // The day before a clock change in many time zones. A calendar day, as in
    // uBO, is then 23 or 25 hours, so the expected date can't be a fixed offset.
    const now = new Date('2026-03-28T12:00:00Z');
    jest.useFakeTimers({ now });

    trustedSetCookie('consent', '1', '1day');

    const expected = new Date(now);
    expected.setDate(expected.getDate() + 1);
    expect(cookieSetter).toHaveBeenLastCalledWith(`consent=1; expires=${expected.toUTCString()}; path=/`);
  });

  test('sets an expiry a number of seconds ahead', () => {
    // 02:10 in Berlin, in the hour that repeats when clocks go back. Adding the
    // seconds in local time would give 00:11 UTC, already in the past.
    jest.useFakeTimers({ now: new Date('2026-10-25T01:10:00Z') });

    trustedSetCookie('consent', '1', '60');

    expect(cookieSetter).toHaveBeenLastCalledWith('consent=1; expires=Sun, 25 Oct 2026 01:11:00 GMT; path=/');
  });

  test('throws for an expiry with a unit', () => {
    expect(() => trustedSetCookie('consent', '1', '10s')).toThrow();
    expect(cookieSetter).not.toHaveBeenCalled();
  });

  test('encodes a semicolon instead of splitting the value', () => {
    trustedSetCookie('consent', 'a;b');

    expect(document.cookie).toBe('consent=a%3Bb');
  });

  test('encodes only the characters that need it', () => {
    trustedSetCookie('consent', '{"tz":"Mitteleuropäische Zeit"}');

    expect(document.cookie).toBe('consent={"tz":"Mitteleurop%C3%A4ische Zeit"}');
  });

  test('leaves a pre-encoded value unchanged', () => {
    trustedSetCookie('consent', 'groups=2%3A0%2C4%3A0');

    expect(document.cookie).toBe('consent=groups=2%3A0%2C4%3A0');
  });

  test('does not add Secure to an unprefixed cookie', () => {
    trustedSetCookie('consent', '1', '', '', 'domain', 'example.com');

    expect(cookieSetter).toHaveBeenLastCalledWith('consent=1; path=/; domain=example.com');
  });
});

describe('trusted-set-cookie-reload', () => {
  test('reloads once when the cookie is new', () => {
    trustedSetCookieReload('consent', '{"timestamp":$now$}');

    expect(document.cookie).toMatch(/^consent=\{"timestamp":\d+\}$/);
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });

  test('does not reload when a cookie of the same shape has an old timestamp', () => {
    document.cookie = 'consent={"timestamp":1}; path=/';

    trustedSetCookieReload('consent', '{"timestamp":$now$}');

    expect(document.cookie).toBe('consent={"timestamp":1}');
    expect(reloadPage).not.toHaveBeenCalled();
  });

  test('reloads when a cookie of a different shape exists', () => {
    document.cookie = 'consent={"other":1}; path=/';

    trustedSetCookieReload('consent', '{"timestamp":$now$}');

    expect(document.cookie).toMatch(/^consent=\{"timestamp":\d+\}$/);
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });

  test('matches the shape of a value with encoded characters', () => {
    document.cookie = 'consent=%C3%A4%3B1; path=/';

    trustedSetCookieReload('consent', 'ä;$now$');

    expect(reloadPage).not.toHaveBeenCalled();
  });

  test('keeps an AdGuard positional domain', () => {
    trustedSetCookieReload('consent', 'false_false_true', '1day', '/', 'example.com');

    expect(cookieSetter.mock.calls[0][0]).toMatch(
      /^consent=false_false_true; expires=[^;]+; path=\/; domain=example\.com$/,
    );
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });
});
