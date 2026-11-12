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

    expect((window as any).test.prop1).toBe(123);
    expect((window as any).test.prop2).toBe(321);
    expect((window as any).test.prop3.prop4).toBe(56);
    expect((window as any).test.prop3.prop5).toBe(516);
  });

  test('nested properties survive an overwrite', () => {
    (window as any).test = {};
    setConstant('test.prop1', '123');
    setConstant('test.prop2', '321');

    (window as any).test.prop1 = '';
    (window as any).test.prop2 = '';

    expect((window as any).test.prop1).toBe(123);
    expect((window as any).test.prop2).toBe(321);
  });

  test('nested properties survive a root property overwrite (root property is initially undefined)', () => {
    setConstant('test.prop1', '123');
    setConstant('test.prop2', '321');

    expect((window as any).test).toBeUndefined();

    (window as any).test = {};

    expect((window as any).test).toBeDefined();
    expect((window as any).test.prop1).toBe(123);
    expect((window as any).test.prop2).toBe(321);
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

    expect((window as any).test.prop1.prop2.prop3).toBe(123);
  });

  test('wraps the value if valueWrapper is set', () => {
    setConstant('PROPERTY', 'yes', '', 'asFunction');

    expect(typeof (window as any).PROPERTY).toBe('function');
    expect((window as any).PROPERTY()).toBe('yes');
  });

  test.each([
    ['0', 0],
    ['32767', 32767],
    ['-32767', -32767],
    ['-1', -1],
  ])('sets %s as a number', (value, expected) => {
    setConstant('PROPERTY', value);

    expect((window as any).PROPERTY).toBe(expected);
  });

  test.each(['32768', '-32768', '1abc', '1.5', '+1', ' 1'])('rejects the number-like value %p', (value) => {
    expect(() => setConstant('PROPERTY', value)).toThrow('Invalid value');
  });

  test('wraps a number with asFunction', () => {
    setConstant('PROPERTY', '5', '', 'asFunction');

    expect((window as any).PROPERTY()).toBe(5);
  });

  test('sets a resolved promise with asResolved', async () => {
    setConstant('PROPERTY', 'true', '', 'asResolved');

    // .resolves calls a function and awaits its result, so only this line fails
    // if the wrapper sets a function again.
    expect((window as any).PROPERTY).toBeInstanceOf(Promise);
    await expect((window as any).PROPERTY).resolves.toBe(true);
  });

  test('sets a rejected promise with asRejected', async () => {
    setConstant('PROPERTY', '5', '', 'asRejected');
    // Jest fails the test if the rejection is still unhandled after a macrotask,
    // so this checks that setConstant handles it.
    await new Promise((r) => setTimeout(r));

    expect((window as any).PROPERTY).toBeInstanceOf(Promise);
    await expect((window as any).PROPERTY).rejects.toBe(5);
  });

  test.each([
    ['{}', {}],
    ['[]', []],
  ])("accepts uBO's %s", (value, expected) => {
    setConstant('PROPERTY', value);

    expect((window as any).PROPERTY).toEqual(expected);
  });

  describe("uBO's name-value pairs", () => {
    test('as, function wraps the value in a function', () => {
      setConstant('PROPERTY', '1', 'as', 'function');

      expect((window as any).PROPERTY()).toBe(1);
    });

    test('as, resolved sets a promise', async () => {
      setConstant('PROPERTY', '1', 'as', 'resolved');

      expect((window as any).PROPERTY).toBeInstanceOf(Promise);
      await expect((window as any).PROPERTY).resolves.toBe(1);
    });

    test('as after runAt still applies', () => {
      setConstant('PROPERTY', '1', 'runAt', 'idle', 'as', 'callback');

      expect((window as any).PROPERTY()()).toBe(1);
    });

    test('runAt alone sets the plain value', () => {
      setConstant('PROPERTY', 'true', 'runAt', 'idle');

      expect((window as any).PROPERTY).toBe(true);
    });

    test('an unknown wrapper sets the plain value', () => {
      setConstant('PROPERTY', '1', 'as', 'asFunction');

      expect((window as any).PROPERTY).toBe(1);
    });

    describe('runAt', () => {
      beforeEach(() => {
        // jsdom has finished loading the document by the time tests run.
        Object.defineProperty(document, 'readyState', { configurable: true, value: 'loading' });
      });

      afterEach(() => {
        delete (document as any).readyState;
      });

      test('idle sets the value once the page has loaded', () => {
        (window as any).PROPERTY = 'original';
        setConstant('PROPERTY', 'true', 'runAt', 'idle');
        expect((window as any).PROPERTY).toBe('original');

        Object.defineProperty(document, 'readyState', { configurable: true, value: 'complete' });
        document.dispatchEvent(new Event('readystatechange'));
        expect((window as any).PROPERTY).toBe(true);
      });

      test("sets a page's top-level var once the page has loaded", () => {
        // A top-level var is non-configurable. jsdom doesn't make it so, so
        // define one by hand. It can't be deleted, hence the unique name.
        Object.defineProperty(window, 'LOCKED_VAR', { value: 'original', writable: true, configurable: false });
        setConstant('LOCKED_VAR', 'true', 'runAt', 'idle');

        Object.defineProperty(document, 'readyState', { configurable: true, value: 'complete' });
        document.dispatchEvent(new Event('readystatechange'));
        expect((window as any).LOCKED_VAR).toBe(true);
      });

      test('an invalid value throws without waiting', () => {
        expect(() => {
          setConstant('PROPERTY', 'invalid', 'runAt', 'idle');
        }).toThrow();
      });
    });
  });

  test('sets a non-configurable nested property', () => {
    (window as any).test = {};
    Object.defineProperty((window as any).test, 'prop', { value: 'original', writable: true, configurable: false });
    setConstant('test.prop', 'true');

    expect((window as any).test.prop).toBe(true);
  });

  test('leaves a non-configurable property alone when the rule has a stack pattern', () => {
    (window as any).test = {};
    Object.defineProperty((window as any).test, 'prop', { value: 'original', writable: true, configurable: false });
    // Can't be deleted, hence the unique name.
    Object.defineProperty(window, 'LOCKED_STACK', { value: 'original', writable: true, configurable: false });

    expect(() => setConstant('test.prop', 'true', 'doesnt-match')).toThrow();
    expect(() => setConstant('LOCKED_STACK', 'true', 'doesnt-match')).toThrow();
    expect((window as any).test.prop).toBe('original');
    expect((window as any).LOCKED_STACK).toBe('original');
  });

  test('traps a missing part below a non-configurable root', () => {
    // Can't be deleted, hence the unique name.
    Object.defineProperty(window, 'LOCKED_ROOT', { value: {}, writable: true, configurable: false });
    setConstant('LOCKED_ROOT.ads.enabled', 'false');

    (window as any).LOCKED_ROOT.ads = { enabled: true };
    expect((window as any).LOCKED_ROOT.ads.enabled).toBe(false);
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
