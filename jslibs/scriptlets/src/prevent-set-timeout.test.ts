import { preventSetTimeout } from './prevent-set-timeout';

describe('prevent-set-timeout', () => {
  let originalSetTimeout: typeof setTimeout;
  let context: Record<string, any>;

  beforeEach(() => {
    originalSetTimeout = global.setTimeout;
    jest.useFakeTimers();

    context = {
      test: undefined,
      value: undefined,
    };
  });

  afterEach(() => {
    global.setTimeout = originalSetTimeout;
    jest.useRealTimers();
  });

  test('should prevent matching inverted value', () => {
    preventSetTimeout('!value');

    setTimeout(function () {
      context.test = 'test';
    }, 300);

    jest.runAllTimers();
    expect(context.test).toBeUndefined();
  });

  test('should not prevent non-matching inverted value', () => {
    preventSetTimeout('!value');

    setTimeout(function () {
      context.test = 'value';
    }, 400);

    jest.runAllTimers();
    expect(context.test).toBe('value');
  });

  test('should prevent matching non-integer delay', () => {
    preventSetTimeout('', '28.2');

    setTimeout(function () {
      context.test = 'value 1';
    }, 28.2);

    jest.runAllTimers();
    expect(context.test).toBeUndefined();
  });

  test('should not prevent callback execution when search parameter is not a string', () => {
    preventSetTimeout({} as any, '100');

    setTimeout(function () {
      context.test = 'value 1';
    }, 100);

    jest.runAllTimers();
    expect(context.test).toBe('value 1');
  });

  test('should not prevent non-matching inverted delay with matching search', () => {
    preventSetTimeout('value', '!300');

    setTimeout(function () {
      context.test = 'value 1';
    }, 300);

    jest.runAllTimers();
    expect(context.test).toBe('value 1');
  });

  test('should prevent matching search and matching inverted delay', () => {
    preventSetTimeout('value', '!300');

    setTimeout(function () {
      context.test = 'value 2';
    }, 400);

    jest.runAllTimers();
    expect(context.test).toBeUndefined();
  });

  test('should prevent when both values are matching (inverted)', () => {
    preventSetTimeout('!value', '!300');

    setTimeout(function () {
      context.test = 'test';
    }, 300);

    jest.runAllTimers();
    expect(context.test).toBe('test');
  });

  test('should prevent when delay matches blocked value', () => {
    preventSetTimeout('!value', '!300');

    setTimeout(function () {
      context.test = 'test';
    }, 400);

    jest.runAllTimers();
    expect(context.test).toBeUndefined();
  });

  test('should not prevent when value does not match blocked one', () => {
    preventSetTimeout('!value', '!300');

    setTimeout(() => {
      context.test = 'value';
    }, 400);

    jest.runAllTimers();
    expect(context.test).toBe('value');
  });
});
