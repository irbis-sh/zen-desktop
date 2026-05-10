import { describe } from '@jest/globals';

import { jsonPruneFetchResponse } from './json-prune-fetch-response';

describe('json-prune-fetch-response', () => {
  let nativeFetch: typeof fetch;

  beforeAll(() => {
    nativeFetch = window.fetch;
  });

  afterEach(() => {
    window.fetch = nativeFetch;
  });

  test('prunes single prop', async () => {
    window.fetch = async () => new Response(JSON.stringify({ a: 123 }));

    jsonPruneFetchResponse('a');

    const json = await (await fetch('https://test.com')).json();
    expect(json).toEqual({});
  });

  test('prunes multiple props, leaves the rest intact', async () => {
    window.fetch = async () => new Response(JSON.stringify({ a: 123, b: 123, c: 123, d: 123 }));

    jsonPruneFetchResponse('a b');

    const json = await (await fetch('https://test.com')).json();
    expect(json).toEqual({ c: 123, d: 123 });
  });

  test('prunes deeply nested prop', async () => {
    window.fetch = async () => new Response(JSON.stringify({ a: { b: { c: 123 } }, d: 321 }));

    jsonPruneFetchResponse('a.b.c');

    const json = await (await fetch('https://test.com')).json();
    expect(json).toEqual({ a: { b: {} }, d: 321 });
  });

  test('multiple calls cooperate with each other', async () => {
    window.fetch = async () => new Response(JSON.stringify({ a: 123, b: 123, c: 123 }));

    jsonPruneFetchResponse('a');
    jsonPruneFetchResponse('b');

    const json = await (await fetch('https://test.com')).json();
    expect(json).toEqual({ c: 123 });
  });

  test('prunes wildcard props', async () => {
    window.fetch = async () =>
      new Response(
        JSON.stringify({
          a: {
            b: {
              c: { z: 'remove', k: 'keep' },
              d: { z: 'remove', k: 'keep' },
            },
          },
        }),
      );

    jsonPruneFetchResponse('a.*.z');

    const json = await (await fetch('https://test.com')).json();
    expect(json).toEqual({
      a: {
        b: {
          c: { k: 'keep' },
          d: { k: 'keep' },
        },
      },
    });
  });

  test("leaves body intact if rule doesn't match", async () => {
    window.fetch = async () => new Response(JSON.stringify({ a: 123 }));

    jsonPruneFetchResponse('a', 'b');

    const json = await (await fetch('https://test.com')).json();
    expect(json).toEqual({ a: 123 });
  });

  test("leaves body intact if it's not json", async () => {
    window.fetch = async () => new Response('hello');

    jsonPruneFetchResponse('a');

    const text = await (await fetch('https//test.com')).text();
    expect(text).toEqual('hello');
  });

  test('prunes response if propsToMatch match', async () => {
    window.fetch = async () => new Response(JSON.stringify({ a: 123 }));

    jsonPruneFetchResponse('a', '', 'example.org method:GET');

    const json = await (await fetch('https://example.org/test', { method: 'GET' })).json();
    expect(json).toEqual({});
  });

  test("doesn't prune response if propsToMatch don't match", async () => {
    window.fetch = async () => new Response(JSON.stringify({ a: 123 }));

    jsonPruneFetchResponse('a', '', 'example.org method:POST');

    const json = await (await fetch('https://test.org')).json();
    expect(json).toEqual({ a: 123 });
  });

  test('prunes response if stack matches', async () => {
    window.fetch = async () => new Response(JSON.stringify({ a: 123 }));

    // We use "jest" as a string to match the stack, but it doesn't appear in the call stack for async functions.
    // Instead, we match based on the file name instead, which is somewhat brittle but functional for now.
    jsonPruneFetchResponse('a', '', '', 'test');

    const json = await (await fetch('https://test.org')).json();
    expect(json).toEqual({});
  });

  test("doesn't prune response if stack doesn't match", async () => {
    window.fetch = async () => new Response(JSON.stringify({ a: 123 }));

    jsonPruneFetchResponse('a', '', '', 'shouldnt match');

    const json = await (await fetch('https://test.org')).json();
    expect(json).toEqual({ a: 123 });
  });
});
