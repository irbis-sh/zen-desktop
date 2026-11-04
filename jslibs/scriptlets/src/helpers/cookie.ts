import { escapeRegexp } from './parseRegexp';
import { isReloadedLoad, reloadPage } from './reload';

// uBO's safelist. AdGuard's is a subset of it, apart from emptyArr and emptyObj,
// which validateUntrustedCookieValue handles separately.
const safeCookieValues = new Set([
  'accept',
  'reject',
  'accepted',
  'rejected',
  'notaccepted',
  'allow',
  'disallow',
  'deny',
  'allowed',
  'denied',
  'approved',
  'disapproved',
  'checked',
  'unchecked',
  'dismiss',
  'dismissed',
  'enable',
  'disable',
  'enabled',
  'disabled',
  'essential',
  'nonessential',
  'forbidden',
  'forever',
  'hide',
  'hidden',
  'necessary',
  'required',
  'ok',
  'on',
  'off',
  'true',
  't',
  'false',
  'f',
  'yes',
  'y',
  'no',
  'n',
  'all',
  'none',
  'functional',
  'granted',
  'done',
  'decline',
  'declined',
  'closed',
  'next',
  'mandatory',
  'disagree',
  'agree',
  'set',
  'unset',
  'given',
]);

/**
 * Checks whether an untrusted set-cookie scriptlet may set the value, and maps
 * it to the value to store.
 * @throws If the value is not allowed.
 */
export function validateUntrustedCookieValue(value: string): string {
  if (value === 'emptyArr') {
    return '[]';
  } else if (value === 'emptyObj') {
    return '{}';
  }

  // uBO accepts a quoted keyword or integer and stores it with the quotes.
  const unquoted = /^"(.+)"$/.exec(value)?.[1] ?? value;
  if (safeCookieValues.has(unquoted.toLowerCase())) {
    return value;
  }
  if (/^-?\d+$/.test(unquoted) && Math.abs(parseInt(unquoted, 10)) <= 32767) {
    return value;
  }

  throw new Error(`Invalid cookie value: ${value}`);
}

interface CookieOptions {
  reload: boolean;
  dontOverwrite: boolean;
  domain: string;
}

const namedOptions = new Set(['reload', 'dontOverwrite', 'domain']);

/**
 * Parses the arguments that follow the path.
 *
 * Zen passes the same argument list for both rule syntaxes, but the two engines
 * read these arguments differently. uBO reads name-value pairs, for example
 * `reload, 1`. AdGuard reads a single positional domain. The first argument
 * tells them apart: it is never one of uBO's option names in an AdGuard rule,
 * since a domain contains a dot.
 */
export function parseCookieOptions(rest: string[]): CookieOptions {
  const opts: CookieOptions = { reload: false, dontOverwrite: false, domain: '' };
  if (!namedOptions.has(rest[0])) {
    opts.domain = rest[0] ?? '';
    return opts;
  }

  for (let i = 0; i < rest.length; i += 2) {
    const value = rest[i + 1] ?? '';
    switch (rest[i]) {
      case 'reload':
        opts.reload = isFlagOn(value);
        break;
      case 'dontOverwrite':
        opts.dontOverwrite = isFlagOn(value);
        break;
      case 'domain':
        opts.domain = value;
        break;
    }
  }
  return opts;
}

/**
 * Sets a cookie, then reloads the page if opts.reload is set and the cookie is
 * new.
 *
 * value may contain time placeholders. Only trusted scriptlets can pass them:
 * untrusted values come from a safelist that has no `$`.
 * @param name Stripped of surrounding spaces and tabs, as browsers do, but never
 * percent-encoded. uBO percent-encodes characters outside the RFC 2616 token set
 * in untrusted names, but browsers accept names such as `tc:cookies`, and the
 * site would never read a cookie under the encoded name.
 * @param expires An expiry date in `toUTCString()` form, or empty for a session cookie.
 * @param path `''` or `/` for `path=/`, or `none` for no path attribute.
 * @throws If the name is empty or contains `;` or `=`, the path is invalid, or
 * the domain is not a hostname.
 */
export function writeCookie(name: string, value: string, expires: string, path: string, opts: CookieOptions): void {
  // Untrimmed, the checks below would look for a name the browser never stores,
  // and a __Host- or __Secure- cookie would be dropped for lacking Secure.
  name = stripCookieName(name);
  // `;` and `=` would change how the browser splits the cookie string. In an
  // untrusted rule, the name `x=any; max-age=31536000; y` would get past the value
  // safelist and set a persistent cookie.
  if (name === '' || /[;=]/.test(name)) {
    throw new Error(`Invalid cookie name: ${name}`);
  }
  if (path !== '' && path !== '/' && path !== 'none') {
    throw new Error(`Invalid cookie path: ${path}`);
  }
  // The domain is pasted into the cookie string, so anything but a hostname
  // could add attributes: `example.com; max-age=31536000` would let an
  // untrusted list set a persistent cookie.
  if (opts.domain !== '' && !/^\.?[a-z0-9-]+(?:\.[a-z0-9-]+)*$/i.test(opts.domain)) {
    throw new Error(`Invalid cookie domain: ${opts.domain}`);
  }

  const existing = readCookieValues(name);
  if (opts.dontOverwrite && existing.length > 0) {
    return;
  }
  // Without reload the cookie is rewritten on every load, as in both engines.
  // That keeps a trusted cookie's expiry and timestamps current.
  if (opts.reload && existing.some((stored) => matchesTemplate(stored, value))) {
    return;
  }

  const encodedValue = encodeCookieValue(replaceTimePlaceholders(value));
  let cookie = `${name}=${encodedValue}`;
  if (expires !== '') {
    cookie += `; expires=${expires}`;
  }
  // Browsers reject a prefixed cookie that lacks these attributes, and a
  // __Host- cookie that has a domain. The prefix check is case-insensitive in
  // current browsers.
  if (/^__Host-/i.test(name)) {
    cookie += '; path=/; Secure';
  } else {
    if (path !== 'none') {
      cookie += '; path=/';
    }
    if (opts.domain !== '') {
      cookie += `; domain=${opts.domain}`;
    }
    if (/^__Secure-/i.test(name)) {
      cookie += '; Secure';
    }
  }
  document.cookie = cookie;

  // The browser silently drops a cookie for a domain the page does not belong
  // to, one that would replace an HttpOnly cookie, or a Secure one on plain
  // HTTP. Reading it back catches all of these, so there is no reload for a
  // cookie that was never set.
  const done = readCookieValues(name).includes(encodedValue);
  // Without the reloaded-load check, a site whose server deletes the cookie on
  // every response would reload forever. If the user reloaded by hand, the page
  // doesn't reload again, so the new cookie takes effect on the next load.
  if (done && opts.reload && !isReloadedLoad()) {
    reloadPage();
  }
}

function isFlagOn(value: string): boolean {
  // uBO reads the value as a number, so `reload, 0` is off.
  return value !== '' && value !== '0';
}

// Characters outside the RFC 6265 cookie-octet set, except space, `"` and `,`,
// which browsers accept. uBO makes the same exceptions. Without the u flag, an
// emoji would match as two halves, and encodeURIComponent throws on half of one.
const unsafeValueChars = /[^ -:<-[\]-~]/gu;

/**
 * Percent-encodes only the characters that need it. uBO encodes the whole
 * string when any character needs it, so a single `ä` turns a JSON value into
 * something its reader cannot parse. That happens with `$currentDate$` in
 * non-English locales, where `Date.prototype.toString()` can include a
 * localised time zone name.
 *
 * matchesTemplate relies on this too: encoding a template's literal parts
 * separately gives the same text as encoding the whole value.
 */
function encodeCookieValue(value: string): string {
  return value.replace(unsafeValueChars, (c) => encodeURIComponent(c));
}

/**
 * Returns the values of every cookie with this name. There can be more than
 * one: cookies with the same name and different domains or paths are all
 * visible to the page.
 */
function readCookieValues(name: string): string[] {
  const values: string[] = [];
  for (const pair of document.cookie.split(';')) {
    const eq = pair.indexOf('=');
    if (eq !== -1 && stripCookieName(pair.slice(0, eq)) === name) {
      values.push(pair.slice(eq + 1).trim());
    }
  }
  return values;
}

/**
 * Browsers strip spaces and tabs around a cookie name, and nothing else. They
 * keep a non-breaking space, so trim() would change the name.
 */
function stripCookieName(name: string): string {
  return name.replace(/^[ \t]+|[ \t]+$/g, '');
}

const timePlaceholders = /\$(?:now|currentDate|currentISODate)\$/g;

function replaceTimePlaceholders(value: string): string {
  const now = new Date();
  return value.replace(timePlaceholders, (placeholder) => {
    switch (placeholder) {
      case '$now$':
        return String(now.getTime());
      case '$currentDate$':
        // toString() follows AdGuard, as every rule using this placeholder in the
        // lists Zen offers comes from an AdGuard list. uBO uses toUTCString().
        return now.toString();
      default:
        return now.toISOString();
    }
  });
}

/**
 * Reports whether a stored cookie value counts as already set.
 *
 * With time placeholders, any stored value of the same shape counts, however old
 * its timestamps. Comparing exactly would never match, as the value changes on
 * every load, and the page would reload on every visit.
 */
function matchesTemplate(stored: string, template: string): boolean {
  const pattern = template
    .split(timePlaceholders)
    .map((literal) => escapeRegexp(encodeCookieValue(literal)))
    .join('.+');
  return new RegExp(`^${pattern}$`).test(stored);
}
