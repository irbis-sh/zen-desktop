import { expect, jest, test, describe, beforeEach, afterEach } from '@jest/globals';

import { genRandomResponse } from './helpers/request/randomResponse';
import { preventXHR } from './prevent-xhr';

jest.mock('./helpers/request/randomResponse', () => ({
  genRandomResponse: jest.fn(),
}));

describe('preventXHR', () => {
  let originalOpen: typeof XMLHttpRequest.prototype.open;
  let originalSend: typeof XMLHttpRequest.prototype.send;
  let originalGetResponseHeader: typeof XMLHttpRequest.prototype.getResponseHeader;
  let originalGetAllResponseHeaders: typeof XMLHttpRequest.prototype.getAllResponseHeaders;
  let send: ReturnType<typeof jest.fn>;

  beforeEach(() => {
    originalOpen = XMLHttpRequest.prototype.open;
    originalSend = XMLHttpRequest.prototype.send;
    originalGetResponseHeader = XMLHttpRequest.prototype.getResponseHeader;
    originalGetAllResponseHeaders = XMLHttpRequest.prototype.getAllResponseHeaders;
    send = jest.fn();
    XMLHttpRequest.prototype.send = new Proxy(XMLHttpRequest.prototype.send, { apply: send as any });
  });

  afterEach(() => {
    XMLHttpRequest.prototype.open = originalOpen;
    XMLHttpRequest.prototype.send = originalSend;
    XMLHttpRequest.prototype.getResponseHeader = originalGetResponseHeader;
    XMLHttpRequest.prototype.getAllResponseHeaders = originalGetAllResponseHeaders;
    jest.clearAllMocks();
  });

  test("prevents a request if called with '*'", () => {
    preventXHR('*');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'example.org');
    xhr.send();

    expect(send).not.toHaveBeenCalled();
  });

  test("prevents a request if called with ''", () => {
    preventXHR('');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'example.org');
    xhr.send();

    expect(send).not.toHaveBeenCalled();
  });

  test('prevents a request on matching a domain name', () => {
    preventXHR('example.org');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'example.org/test');
    xhr.send();

    expect(send).not.toHaveBeenCalled();
  });

  test('prevents a request on a matching domain name if the url is fully qualified and contains a port', () => {
    preventXHR('example.org');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'https://example.org:443/test');
    xhr.send();

    expect(send).not.toHaveBeenCalled();
  });

  test("doesn't prevent a request with a non-matching domain name", () => {
    preventXHR('example.org');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'example.net');
    xhr.send();

    expect(send).toHaveBeenCalled();
  });

  test('prevents a request on a matching method', () => {
    preventXHR('method:GET');

    const xhr1 = new XMLHttpRequest();
    xhr1.open('GET', 'example.org');
    xhr1.send();

    expect(send).not.toHaveBeenCalled();
  });

  test("doesn't prevent a request on a non-matching method", () => {
    preventXHR('method:GET');

    const xhr = new XMLHttpRequest();
    xhr.open('POST', 'example.org');
    xhr.send();

    expect(send).toHaveBeenCalled();
  });

  test('prevents a request on a matching domain name and method', () => {
    preventXHR('example.org method:GET');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'example.org/');
    xhr.send();

    expect(send).not.toHaveBeenCalled();
  });

  test('prevents a request on a matching url segment', () => {
    preventXHR('adsbygoogle.js');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js?client=test');
    xhr.send();

    expect(send).not.toHaveBeenCalled();
  });

  test("doesn't prevent a request if only url gets matched", () => {
    preventXHR('example.org method:GET');

    const xhr = new XMLHttpRequest();
    xhr.open('POST', 'example.org');
    xhr.send();

    expect(send).toHaveBeenCalled();
  });

  test('generates empty json response if responseType is set to json', (done) => {
    preventXHR('*');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'example.org');
    xhr.responseType = 'json';
    xhr.onload = () => {
      try {
        expect(xhr.readyState).toBe(4);
        expect(xhr.response).toEqual({});
        expect(xhr.responseText).toBe('{}');
        expect(xhr.getResponseHeader('Content-Type')).toBe('application/json');
        done();
      } catch (ex) {
        done(ex as Error);
      }
    };
    xhr.send();
  });

  test('generates proper fake response headers', (done) => {
    preventXHR('*');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'example.org');
    xhr.onload = () => {
      try {
        expect(xhr.getResponseHeader('Content-Type')).toBe('text/plain');
        expect(xhr.getResponseHeader('Content-Length')).toBe('0');
        done();
      } catch (ex) {
        done(ex as Error);
      }
    };
    xhr.send();
  });

  test('generates random response', (done) => {
    (genRandomResponse as jest.Mock).mockReturnValue('wow-much-random');

    preventXHR('*', 'length:100-100');

    const xhr = new XMLHttpRequest();
    xhr.open('GET', 'example.org');
    xhr.onload = () => {
      try {
        expect(genRandomResponse).toHaveBeenCalled();
        expect(xhr.response).toBe('wow-much-random');
        expect(xhr.getResponseHeader('Content-Length')).toBe('wow-much-random'.length.toString());
        done();
      } catch (ex) {
        done(ex as Error);
      }
    };
    xhr.send();
  });
});
