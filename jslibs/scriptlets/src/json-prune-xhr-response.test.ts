import { expect, test, beforeAll, afterEach } from '@jest/globals';

import { jsonPruneXHRResponse } from './json-prune-xhr-response';

describe('json-prune-xhr-response', () => {
  let originalOpen: typeof XMLHttpRequest.prototype.open;
  let originalSend: typeof XMLHttpRequest.prototype.send;
  let originalGetResponseHeader: typeof XMLHttpRequest.prototype.getResponseHeader;
  let originalGetAllResponseHeaders: typeof XMLHttpRequest.prototype.getAllResponseHeaders;

  beforeAll(() => {
    originalOpen = XMLHttpRequest.prototype.open;
    originalSend = XMLHttpRequest.prototype.send;
    originalGetResponseHeader = XMLHttpRequest.prototype.getResponseHeader;
    originalGetAllResponseHeaders = XMLHttpRequest.prototype.getAllResponseHeaders;
  });

  afterEach(() => {
    XMLHttpRequest.prototype.open = originalOpen;
    XMLHttpRequest.prototype.send = originalSend;
    XMLHttpRequest.prototype.getResponseHeader = originalGetResponseHeader;
    XMLHttpRequest.prototype.getAllResponseHeaders = originalGetAllResponseHeaders;
  });

  test('prunes text response', (done) => {
    setupFakeSend(JSON.stringify({ a: 123, b: 123 }), 'text');

    jsonPruneXHRResponse('b');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'test');
    xhr.send();

    xhr.onreadystatechange = () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      const expectedResponse = JSON.stringify({ a: 123 });
      expect(xhr.response).toBe(expectedResponse);
      expect(xhr.responseText).toBe(expectedResponse);
      done();
    };
  });

  test('prunes arraybuffer response', (done) => {
    setupFakeSend(new TextEncoder().encode(JSON.stringify({ a: 123, b: 123 })), 'arraybuffer');

    jsonPruneXHRResponse('b');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'test');
    xhr.send();

    xhr.addEventListener('readystatechange', () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      const expectedResponse = JSON.stringify({ a: 123 });
      const decodedResponse = new TextDecoder().decode(xhr.response);
      expect(decodedResponse).toBe(expectedResponse);
      done();
    });
  });

  test('leaves non-JSON arraybuffer response intact', (done) => {
    const expectedResponse = 'hey there';

    setupFakeSend(new TextEncoder().encode(expectedResponse), 'arraybuffer');

    jsonPruneXHRResponse('hey');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'test');
    xhr.send();

    xhr.addEventListener('readystatechange', () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      const decoded = new TextDecoder().decode(xhr.response);
      expect(decoded).toBe(expectedResponse);
      done();
    });
  });

  test('prunes blob response', (done) => {
    setupFakeSend(new Blob([JSON.stringify({ a: 123, b: 123 })]), 'blob');

    jsonPruneXHRResponse('b');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'test');
    xhr.send();

    xhr.addEventListener('readystatechange', async () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      const decoded = await xhr.response.text();
      expect(decoded).toBe(JSON.stringify({ a: 123 }));
      done();
    });
  });

  test('leaves non-JSON blob response intact', (done) => {
    const expectedResponse = 'hey there';

    setupFakeSend(new Blob([expectedResponse]), 'blob');

    jsonPruneXHRResponse('hey');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'test');
    xhr.send();

    xhr.addEventListener('readystatechange', async () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      const decoded = await xhr.response.text();
      expect(decoded).toBe(expectedResponse);
      done();
    });
  });

  test('prunes json response', (done) => {
    setupFakeSend({ a: 123, b: 123 }, 'json');

    jsonPruneXHRResponse('b');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'test');
    xhr.send();

    xhr.addEventListener('readystatechange', () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      expect(xhr.response).toEqual({ a: 123 });
      done();
    });
  });

  test("doesn't prune response on a non-matching domain name", (done) => {
    setupFakeSend({ a: 123, b: 123 }, 'json');

    jsonPruneXHRResponse('b', '', 'example.org');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'https://example.net');
    xhr.send();

    xhr.addEventListener('readystatechange', () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      expect(xhr.response).toEqual({ a: 123, b: 123 });
      done();
    });
  });

  test('prunes response on a matching domain name', (done) => {
    setupFakeSend({ a: 123, b: 123 }, 'json');

    jsonPruneXHRResponse('b', '', 'example.org');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'https://example.org');
    xhr.send();

    xhr.addEventListener('readystatechange', () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      expect(xhr.response).toEqual({ a: 123 });
      done();
    });
  });

  test('prunes deeply nested property', (done) => {
    setupFakeSend(
      JSON.stringify({
        a: {
          b: {
            c: {
              d: [{ remove: 123, keep: 321 }],
            },
          },
        },
      }),
      'text',
    );

    jsonPruneXHRResponse('a.*.remove');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'test');
    xhr.send();

    xhr.addEventListener('readystatechange', () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      expect(xhr.response).toBe(
        JSON.stringify({
          a: {
            b: {
              c: {
                d: [{ keep: 321 }],
              },
            },
          },
        }),
      );
      done();
    });
  });

  test("doesn't prune on a missing required property", (done) => {
    setupFakeSend({ a: 123, b: 123 }, 'json');

    jsonPruneXHRResponse('b', 'c');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'test');
    xhr.send();

    xhr.addEventListener('readystatechange', () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      expect(xhr.response).toEqual({ a: 123, b: 123 });
      done();
    });
  });

  test('prunes multiple properties', (done) => {
    setupFakeSend({ a: 123, b: 123, c: 123 }, 'json');

    jsonPruneXHRResponse('a b');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'test');
    xhr.send();

    xhr.addEventListener('readystatechange', () => {
      if (xhr.readyState !== XMLHttpRequest.DONE) {
        return;
      }

      expect(xhr.response).toEqual({ c: 123 });
      done();
    });
  });
});

function setupFakeSend(response: any, responseType: XMLHttpRequestResponseType): void {
  XMLHttpRequest.prototype.send = new Proxy(XMLHttpRequest.prototype.send, {
    apply: (target, thisArg) => {
      Object.defineProperties(thisArg, {
        readyState: { value: XMLHttpRequest.DONE, writable: false },
        response: { value: response, writable: false },
        responseText: { value: response, writable: false },
        responseType: { value: responseType, writable: false },
      });

      setTimeout(() => {
        thisArg.dispatchEvent(new Event('readystatechange'));
        thisArg.dispatchEvent(new Event('load'));
        thisArg.dispatchEvent(new Event('loadend'));
      }, 1);
    },
  });
}
