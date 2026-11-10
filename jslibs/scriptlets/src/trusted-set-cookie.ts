import { parseCookieOptions, writeCookie } from './helpers/cookie';

export function trustedSetCookie(name = '', value = '', offsetExpiresSec = '', path = '', ...rest: string[]) {
  writeCookie(name, value, parseExpiry(offsetExpiresSec), path, parseCookieOptions(rest));
}

// Sets reload after parsing. uBO puts `reload, 1` before the remaining arguments,
// which here would read them as name-value pairs and lose an AdGuard positional
// domain.
export function trustedSetCookieReload(name = '', value = '', offsetExpiresSec = '', path = '', ...rest: string[]) {
  writeCookie(name, value, parseExpiry(offsetExpiresSec), path, { ...parseCookieOptions(rest), reload: true });
}

/**
 * Turns an offset into an expiry date for the cookie.
 * @param offset `1day`, `1year`, a number of seconds, or empty for a session cookie.
 * @throws If the offset is none of these.
 */
function parseExpiry(offset: string): string {
  if (offset === '') {
    return '';
  }

  const date = new Date();
  if (offset === '1day') {
    date.setDate(date.getDate() + 1);
  } else if (offset === '1year') {
    date.setFullYear(date.getFullYear() + 1);
  } else if (/^\d+$/.test(offset)) {
    // Not setSeconds, which adds in local time: in the hour that repeats when
    // clocks go back, it can give an expiry in the past, and the browser then
    // deletes the cookie at once.
    date.setTime(date.getTime() + parseInt(offset, 10) * 1000);
  } else {
    throw new Error(`Invalid cookie expiry: ${offset}`);
  }
  return date.toUTCString();
}
