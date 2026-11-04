/**
 * Deletes every cookie the cookie tests set. A deletion must repeat the domain
 * of the cookie it targets, and the Secure attribute that __Host- and __Secure-
 * names require. Tests run on https://www.example.com/.
 */
export function clearCookies() {
  for (const pair of document.cookie.split(';')) {
    const name = pair.split('=')[0].trim();
    if (name === '') {
      continue;
    }
    const expired = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/; Secure`;
    document.cookie = expired;
    document.cookie = `${expired}; domain=example.com`;
  }
}
