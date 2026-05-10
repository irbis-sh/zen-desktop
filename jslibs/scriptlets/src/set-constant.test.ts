import { expect, test, describe, afterEach } from '@jest/globals';

import { setConstant } from './set-constant';

describe('set-constant', () => {
  let nativeObject: typeof Object;

  beforeEach(() => {
    nativeObject = window.Object;
  });

  afterEach(() => {
    delete (window as any).PROPERTY;
    delete (window as any).test;
    window.Object = nativeObject;
  });

  test('sets a non-nested property', () => {
    setConstant('PROPERTY', 'yes');

    expect((window as any).PROPERTY).toBe('yes');
  });

  test('non-nested property survives an overwrite', () => {
    setConstant('PROPERTY', 'yes');

    (window as any).PROPERTY = 'no';
    expect((window as any).PROPERTY).toBe('yes');
  });

  test('sets multiple nested properties', () => {
    (window as any).test = { prop3: {} };
    setConstant('test.prop1', '123');
    setConstant('test.prop2', '321');
    setConstant('test.prop3.prop4', '56');
    setConstant('test.prop3.prop5', '516');

    expect((window as any).test.prop1).toBe('123');
    expect((window as any).test.prop2).toBe('321');
    expect((window as any).test.prop3.prop4).toBe('56');
    expect((window as any).test.prop3.prop5).toBe('516');
  });

  test('nested properties survive an overwrite', () => {
    (window as any).test = {};
    setConstant('test.prop1', '123');
    setConstant('test.prop2', '321');

    (window as any).test.prop1 = '';
    (window as any).test.prop2 = '';

    expect((window as any).test.prop1).toBe('123');
    expect((window as any).test.prop2).toBe('321');
  });

  test('nested properties survive a root property overwrite (root property is initially undefined)', () => {
    setConstant('test.prop1', '123');
    setConstant('test.prop2', '321');

    expect((window as any).test).toBeUndefined();

    (window as any).test = {};

    expect((window as any).test).toBeDefined();
    expect((window as any).test.prop1).toBe('123');
    expect((window as any).test.prop2).toBe('321');
  });

  test("doesn't overwrite functions", () => {
    (window as any).test = () => 'meow';
    setConstant('test.prototype.prop', '123');

    expect((window as any).test()).toBe('meow');
  });

  test("doesn't modify the value if the stack doesn't match", () => {
    setConstant('PROPERTY', 'no', 'definitely-doesnt-match');

    expect((window as any).PROPERTY).toBe(undefined);
  });

  test('modifies the value if the stack does match', () => {
    setConstant('PROPERTY', 'yes', 'jest');

    expect((window as any).PROPERTY).toBe('yes');
  });

  test("doesn't modify a deeply nested value if the stack doesn't match", () => {
    (window as any).test = { prop1: { prop2: {} } };
    setConstant('test.prop1.prop2.prop3', '123', 'definitely-doesnt-match');

    expect((window as any).test.prop1.prop2.prop3).toBe(undefined);
  });

  test('modifies a deeply nested value if the stack does match', () => {
    (window as any).test = { prop1: { prop2: {} } };
    setConstant('test.prop1.prop2.prop3', '123', 'jest');

    expect((window as any).test.prop1.prop2.prop3).toBe('123');
  });

  test('wraps the value if valueWrapper is set', () => {
    setConstant('PROPERTY', 'yes', '', 'asFunction');

    expect(typeof (window as any).PROPERTY).toBe('function');
    expect((window as any).PROPERTY()).toBe('yes');
  });

  test('errors out on invalid value', () => {
    expect(() => {
      setConstant('PROPERTY', 'invalid');
    }).toThrow();
  });

  test('overwritten root property is equal to itself', () => {
    (window as any).test = {};
    setConstant('test.prop1', '123');

    expect((window as any).test === (window as any).test).toBe(true);
  });

  test('intermediate property in the chain is equal to itself', () => {
    (window as any).test = {
      prop1: {},
    };
    setConstant('test.prop1.prop2', '123');

    expect((window as any).test.prop1 === (window as any).test.prop1).toBe(true);
  });

  test('affected native function is equal to itself', () => {
    setConstant('Object.prototype.noAds', 'trueFunc');

    expect(Object.hasOwn).toBe(Object.hasOwn);
  });
});
