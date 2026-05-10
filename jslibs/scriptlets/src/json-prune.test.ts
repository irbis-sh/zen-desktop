import { expect, test, describe, beforeAll, afterEach } from '@jest/globals';

import { jsonPrune } from './json-prune';

describe('json-prune', () => {
  let originalJSONParse: typeof JSON.parse;

  beforeAll(() => {
    originalJSONParse = JSON.parse;
  });

  afterEach(() => {
    JSON.parse = originalJSONParse;
  });

  test('removes root object prop', () => {
    jsonPrune('a');

    const obj = JSON.parse('{"a": 123, "b": 321}');
    expect(obj).toEqual({ b: 321 });
  });

  test('removes root object prop only if required prop is matched', () => {
    jsonPrune('a', 'b');

    const obj1 = JSON.parse('{"a": 123, "b": 321}');
    expect(obj1).toEqual({ b: 321 });

    const obj2 = JSON.parse('{"a": 123, "c": 321}');
    expect(obj2).toEqual({ a: 123, c: 321 });
  });

  test('multiple json-prunes can be chained', () => {
    jsonPrune('aa');
    jsonPrune('bb');

    const obj = JSON.parse('{"aa": 123, "bb": 123, "cc": 123}');
    expect(obj).toEqual({ cc: 123 });
  });

  test('removes deeply nested properties', () => {
    jsonPrune('a.b.c.d.e.f.g');
    jsonPrune('a.b.c.d.e.f.h');

    const obj = JSON.parse('{"a":{"b":{"c":{"d":{"e":{"f":{"g": 123, "h": 321}}}}}}}');
    expect(obj).toEqual({ a: { b: { c: { d: { e: { f: {} } } } } } });
  });

  test('removes wildcard properties', () => {
    jsonPrune('a.*.z');
    jsonPrune('a.*.h');

    const obj = JSON.parse(
      '{"a":{"arr":[{"bird":{"z":"remove","k":"keep","h":"remove"}},{"cat":{"z":"remove","k":"keep","h":"remove"}}]},"b":"keep"}',
    );
    expect(obj).toEqual({ a: { arr: [{ bird: { k: 'keep' } }, { cat: { k: 'keep' } }] }, b: 'keep' });
  });

  test('matches wildcard properties', () => {
    jsonPrune('a', 'a.*.z');

    const obj = JSON.parse('{"a":{"b":{"z":1}}}');
    expect(obj).toEqual({});
  });

  test('removes properties with []', () => {
    jsonPrune('a.[].z');

    const obj = JSON.parse('{"a":[{"b":"keep","z":"remove"},{"c":"keep","z":"remove"}]}');
    expect(obj).toEqual({ a: [{ b: 'keep' }, { c: 'keep' }] });
  });

  test("doesn't modify the result if stack doesn't match", () => {
    jsonPrune('a', undefined, "this is not the stack you're looking for");

    const obj = JSON.parse('{"a": 123}');
    expect(obj).toEqual({ a: 123 });
  });

  test('modifies the result if stack does match', () => {
    jsonPrune('a', undefined, 'jest');

    const obj = JSON.parse('{"a": 123}');
    expect(obj).toEqual({});
  });

  test('"null" gets parsed to null', () => {
    jsonPrune('test');

    expect(JSON.parse('null')).toEqual(null);
  });
});
