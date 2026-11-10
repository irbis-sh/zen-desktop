import { parseCookieOptions, validateUntrustedCookieValue, writeCookie } from './helpers/cookie';

export function setCookie(name = '', value = '', path = '', ...rest: string[]) {
  writeCookie(name, validateUntrustedCookieValue(value), '', path, parseCookieOptions(rest));
}

// Sets reload after parsing. uBO puts `reload, 1` before the remaining arguments,
// which here would read them as name-value pairs and lose an AdGuard positional
// domain.
export function setCookieReload(name = '', value = '', path = '', ...rest: string[]) {
  writeCookie(name, validateUntrustedCookieValue(value), '', path, {
    ...parseCookieOptions(rest),
    reload: true,
  });
}
