import { preventSetInterval } from './prevent-set-interval';

describe('prevent-set-interval', () => {
  let originalSetInterval: typeof setInterval;
  let context: Record<string, any>;

  beforeEach(() => {
    originalSetInterval = global.setInterval;
    jest.useFakeTimers();
    context = {
      test: undefined,
      value: undefined,
    };
  });

  afterEach(() => {
    global.setInterval = originalSetInterval;
    jest.useRealTimers();
  });

  test('should prevent when condition matches "!value"', () => {
    preventSetInterval('!value');

    const timer = setInterval(() => {
      context.test = 'test';
    }, 300);

    jest.advanceTimersByTime(400);
    clearInterval(timer);
    expect(context.test).toBeUndefined();
  });

  test('should not prevent when condition does not match "!value"', () => {
    preventSetInterval('!value');

    const timer = setInterval(() => {
      context.test = 'value';
    }, 400);

    jest.advanceTimersByTime(500);
    clearInterval(timer);
    expect(context.test).toBe('value');
  });

  test('should not prevent with different property when condition is "!value"', () => {
    preventSetInterval('!value');

    const timer = setInterval(() => {
      context.value = 'test';
    }, 500);

    jest.advanceTimersByTime(600);
    clearInterval(timer);
    expect(context.value).toBe('test');
  });

  test('should not prevent with allowed delay when delay condition is "!300"', () => {
    preventSetInterval('value', '!300');

    const timer = setInterval(() => {
      context.test = 'value';
    }, 300);

    jest.advanceTimersByTime(400);
    clearInterval(timer);
    expect(context.test).toBe('value');
  });

  test('should prevent with blocked delay when delay condition is "!300"', () => {
    preventSetInterval('value', '!300');

    const timer = setInterval(() => {
      context.test = 'value';
    }, 400);

    jest.advanceTimersByTime(500);
    clearInterval(timer);
    expect(context.test).toBeUndefined();
  });

  test('should prevent with another blocked delay when delay condition is "!300"', () => {
    preventSetInterval('value', '!300');

    const timer = setInterval(() => {
      context.test = 'value';
    }, 500);

    jest.advanceTimersByTime(600);
    clearInterval(timer);
    expect(context.test).toBeUndefined();
  });

  test('should not prevent when both pattern and delay are allowed', () => {
    preventSetInterval('!value', '!300');

    const timer = setInterval(() => {
      context.test = 'test';
    }, 300);

    jest.advanceTimersByTime(400);
    clearInterval(timer);
    expect(context.test).toBe('test');
  });

  test('should prevent when both pattern and delay match block conditions', () => {
    preventSetInterval('!value', '!300');

    const timer = setInterval(() => {
      context.test = 'test';
    }, 400);

    jest.advanceTimersByTime(500);
    clearInterval(timer);
    expect(context.test).toBeUndefined();
  });

  test('should not prevent when pattern is excluded but delay is allowed', () => {
    preventSetInterval('!value', '!300');

    const timer = setInterval(() => {
      context.test = 'value';
    }, 400);

    jest.advanceTimersByTime(500);
    clearInterval(timer);
    expect(context.test).toBe('value');
  });

  test('should not prevent on different property with allowed delay', () => {
    preventSetInterval('!value', '!300');

    const timer = setInterval(() => {
      context.value = 'test';
    }, 500);

    jest.advanceTimersByTime(600);
    clearInterval(timer);
    expect(context.value).toBe('test');
  });
});
