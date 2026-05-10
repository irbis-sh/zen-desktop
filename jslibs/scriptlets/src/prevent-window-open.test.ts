import { expect, jest, test, describe, beforeEach, afterEach } from '@jest/globals';

import { preventWindowOpen } from './prevent-window-open';

describe('prevent-window-open', () => {
  let openRepl: ReturnType<typeof jest.fn>;
  let originalOpen: typeof open;

  beforeEach(() => {
    originalOpen = window.open;
    openRepl = jest.fn();
    window.open = openRepl;
  });

  afterEach(() => {
    window.open = originalOpen;
  });

  test('new syntax: prevents all calls to window.open when called with no arguments', () => {
    preventWindowOpen();

    window.open();
    window.open('https://test.com');
    window.open(new URL('https://test.com'));

    expect(openRepl).not.toHaveBeenCalled();
  });

  test('new syntax: correctly handles string "match"', () => {
    preventWindowOpen('test');

    window.open('https://test.com');
    window.open(new URL('https://test.com'));
    expect(openRepl).not.toHaveBeenCalled();

    window.open('https://example.com');
    window.open(new URL('https://example.com'));
    expect(openRepl).toHaveBeenCalledTimes(2);
  });

  test('new syntax: correctly handles regular expression "match"', () => {
    preventWindowOpen('/a.+c/');

    window.open('https://abc.com');
    window.open(new URL('https://abc.com'));
    expect(openRepl).not.toHaveBeenCalled();

    window.open('https://cba.net');
    window.open(new URL('https://cba.net'));
    expect(openRepl).toHaveBeenCalledTimes(2);
  });

  test('new syntax: inverts prevention when "match" is prepended with !', () => {
    preventWindowOpen('!test');

    window.open('https://example.com');
    window.open(new URL('https://example.com'));
    expect(openRepl).not.toHaveBeenCalled();

    window.open('https://test.com');
    window.open(new URL('https://test.com'));
    expect(openRepl).toHaveBeenCalledTimes(2);
  });

  test('new syntax: returns a fake window when called with no "replacement"', () => {
    preventWindowOpen();

    const w = window.open();
    expect(openRepl).not.toHaveBeenCalled();
    expect(w).not.toBeNull();

    expect(w!.document).toBeInstanceOf(Document);
  });

  test('new syntax: calls window.open with "about:blank" when "replacement" is "blank"', () => {
    preventWindowOpen('', '', 'blank');

    window.open('https://test.com', '_self');
    expect(openRepl).toHaveBeenCalledWith('about:blank', '_self');
  });

  test('old syntax: prevents all calls to window.open when called only with "match"', () => {
    preventWindowOpen('1');

    window.open();
    window.open('https://test.com');
    window.open(new URL('https://test.com'));

    expect(openRepl).not.toHaveBeenCalled();
  });

  test('old syntax: correctly handles string "search"', () => {
    preventWindowOpen('1', 'test');

    window.open('https://test.com');
    window.open(new URL('https://test.com'));
    expect(openRepl).not.toHaveBeenCalled();

    window.open('https://example.com');
    window.open(new URL('https://example.com'));
    expect(openRepl).toHaveBeenCalledTimes(2);
  });

  test('old syntax: correctly handles regular expression "search"', () => {
    preventWindowOpen('1', '/a.+c/');

    window.open('https://abc.com');
    window.open(new URL('https://abc.com'));
    expect(openRepl).not.toHaveBeenCalled();

    window.open('https://cba.net');
    window.open(new URL('https://cba.net'));
    expect(openRepl).toHaveBeenCalledTimes(2);
  });

  test('old syntax: inverts prevention when "match" is prepended with 0', () => {
    preventWindowOpen('0', 'test');

    window.open('https://example.com');
    window.open(new URL('https://example.com'));
    expect(openRepl).not.toHaveBeenCalled();

    window.open('https://test.com');
    window.open(new URL('https://test.com'));
    expect(openRepl).toHaveBeenCalledTimes(2);
  });

  test('old syntax: returns a noop function when called with no "replacement"', () => {
    preventWindowOpen('1', 'test');

    const w = window.open('https://test.com') as unknown as Function;
    expect(openRepl).not.toHaveBeenCalled();
    expect(w).toBeInstanceOf(Function);
    expect(w()).toBeUndefined();
  });

  test('old syntax: returns a true function when "replacement" is "trueFunc"', () => {
    preventWindowOpen('1', 'test', 'trueFunc');

    const w = window.open('https://test.com') as unknown as Function;
    expect(openRepl).not.toHaveBeenCalled();
    expect(w).toBeInstanceOf(Function);
    expect(w()).toBe(true);
  });

  test('old syntax: returns a noop function in a property when "replacement" is "{prop=noopFunc}"', () => {
    preventWindowOpen('1', 'test', '{prop=noopFunc}');

    const w = window.open('https://test.com') as unknown as { prop: Function };
    expect(openRepl).not.toHaveBeenCalled();
    expect(w).toEqual({ prop: expect.any(Function) });
    expect(w.prop()).toBeUndefined();
  });
});
