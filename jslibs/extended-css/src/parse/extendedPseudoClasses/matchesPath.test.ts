import { describe, test, beforeAll, afterAll } from '@jest/globals';

import { MatchesPath } from './matchesPath';

describe('MatchesPath', () => {
  let originalLocation: Location;

  beforeAll(() => {
    originalLocation = window.location;
    // @ts-ignore
    delete window.location;
    // @ts-ignore
    window.location = { pathname: '/foo/bar/baz' };
  });

  afterAll(() => {
    // @ts-ignore
    window.location = originalLocation;
  });

  test('returns input if pathname includes the search string', () => {
    const selector = new MatchesPath('foo');
    const input = [{}, {}] as Element[];
    expect(selector.run(input)).toBe(input);
  });

  test('returns empty array if pathname does not include the search string', () => {
    const selector = new MatchesPath('notfound');
    const input = [{}, {}] as Element[];
    expect(selector.run(input)).toEqual([]);
  });

  test('returns input if pathname matches the regexp', () => {
    const selector = new MatchesPath('/bar/');
    const input = [{}, {}] as Element[];
    expect(selector.run(input)).toBe(input);
  });

  test('returns empty array if pathname does not match the regexp', () => {
    const selector = new MatchesPath('/qux/');
    const input = [{}, {}] as Element[];
    expect(selector.run(input)).toEqual([]);
  });

  test('returns empty array if input is empty', () => {
    const selector = new MatchesPath('foo');
    expect(selector.run([])).toEqual([]);
  });
});
