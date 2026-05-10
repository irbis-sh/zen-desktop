import { expect, jest, test, describe, beforeEach, afterEach } from '@jest/globals';

import { preventFetch } from './prevent-fetch';

describe('prevent-fetch', () => {
  let fetchRepl: ReturnType<typeof jest.fn>;
  let originalFetch: typeof fetch;

  beforeEach(() => {
    originalFetch = window.fetch;
    fetchRepl = jest.fn();
    // @ts-ignore
    fetch = fetchRepl;
  });

  afterEach(() => {
    // @ts-ignore
    fetch = originalFetch;
  });

  test('prevents a request if called with "*"', async () => {
    preventFetch('*');

    await fetch('example.org');
    expect(fetchRepl).not.toHaveBeenCalled();
  });

  test('prevents a request on a matching domain name', async () => {
    preventFetch('example.org');

    await fetch('example.org');
    expect(fetchRepl).not.toHaveBeenCalled();
  });

  test("doesn't prevent a request on a non-matching domain name", async () => {
    preventFetch('example.org');

    await fetch('example.com');
    expect(fetchRepl).toHaveBeenCalled();
  });

  test('prevents a request on a matching method', async () => {
    preventFetch('example.org method:GET');

    await fetch('example.org', { method: 'GET' });
    expect(fetchRepl).not.toHaveBeenCalled();
  });

  test('prevents a request on a matching url segment', async () => {
    preventFetch('adsbygoogle.js');

    await fetch('https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js');
    expect(fetchRepl).not.toHaveBeenCalled();
  });

  test('generates a response body according to the passed responseBodyType', async () => {
    preventFetch('*', 'emptyObj');

    const response = await fetch('example.org');
    expect(await response.text()).toBe('{}');
    expect(response.headers.get('content-type')).toBe('application/json');
    expect(response.headers.get('content-length')).toBe('{}'.length.toString());
  });

  test('generates an opaque response if responseType is set to opaque', async () => {
    preventFetch('*', 'emptyObj', 'opaque');

    const response = await fetch('example.org');
    expect(response.type).toBe('opaque');
    expect(response.url).toBe('');
    expect(response.status).toBe(0);
    expect(response.statusText).toBe('');
    expect(response.body).toBe(null);
    expect(response.headers.get('content-type')).toBe(null);
    expect(response.headers.get('content-length')).toBe(null);
  });
});
