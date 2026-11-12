import { afterEach, beforeEach, describe, expect, jest, test } from '@jest/globals';

import { runAt } from './runAt';

describe('runAt', () => {
  let readyState: DocumentReadyState;
  const advance = (state: DocumentReadyState) => {
    readyState = state;
    document.dispatchEvent(new Event('readystatechange'));
  };

  beforeEach(() => {
    readyState = 'loading';
    // jsdom has finished loading the document by the time tests run.
    Object.defineProperty(document, 'readyState', { configurable: true, get: () => readyState });
  });

  afterEach(() => {
    delete (document as any).readyState;
    jest.restoreAllMocks();
  });

  test.each([undefined, 'loading', 'asap', 'unknown'])('runs straight away for %p', (when) => {
    const fn = jest.fn();
    runAt(fn, when);

    expect(fn).toHaveBeenCalledTimes(1);
  });

  test('runs straight away when the document is already past the state', () => {
    readyState = 'complete';
    const fn = jest.fn();
    runAt(fn, 'interactive');

    expect(fn).toHaveBeenCalledTimes(1);
  });

  test.each([
    ['interactive', 'interactive'],
    ['end', 'interactive'],
    ['2', 'interactive'],
    ['complete', 'complete'],
    ['idle', 'complete'],
    ['3', 'complete'],
  ] as const)('%p waits for the %p state, then runs once', (when, state) => {
    const fn = jest.fn();
    runAt(fn, when);
    expect(fn).not.toHaveBeenCalled();

    if (state === 'complete') {
      advance('interactive');
      expect(fn).not.toHaveBeenCalled();
    }
    advance(state);
    expect(fn).toHaveBeenCalledTimes(1);

    advance('complete');
    expect(fn).toHaveBeenCalledTimes(1);
  });

  test('logs an error thrown by a deferred call', () => {
    const consoleError = jest.spyOn(console, 'error').mockImplementation(() => {});
    const err = new Error('boom');
    runAt(() => {
      throw err;
    }, 'idle');

    advance('complete');
    expect(consoleError).toHaveBeenCalledWith(expect.stringContaining('Deferred call failed'), err);
  });
});
