import { preventAddEventListener } from './prevent-add-event-listener';

describe('prevent-addEventListener', () => {
  let original: typeof window.EventTarget.prototype.addEventListener;

  beforeEach(() => {
    original = window.EventTarget.prototype.addEventListener;
    preventAddEventListener('click', 'console.log');
  });

  afterEach(() => {
    window.EventTarget.prototype.addEventListener = original;
  });

  it('should not block unrelated event types', () => {
    const btn = document.createElement('button');
    const fn = jest.fn();

    btn.addEventListener('mouseover', fn);
    btn.dispatchEvent(new Event('mouseover'));

    expect(fn).toHaveBeenCalled();
  });

  it('should allow handlers that do not match the function body', () => {
    const div = document.createElement('div');
    const fn = jest.fn();

    div.addEventListener('click', fn);
    div.dispatchEvent(new Event('click'));

    expect(fn).toHaveBeenCalled();
  });

  it('should prevent handlers that match both event type and function content', () => {
    const span = document.createElement('span');
    const spy = jest.fn();

    span.addEventListener('click', () => console.log('Click block'));
    span.dispatchEvent(new Event('click'));

    expect(spy).not.toHaveBeenCalled();
  });

  it('should not interfere if no filtering is configured', () => {
    window.EventTarget.prototype.addEventListener = original;
    preventAddEventListener('', '');

    const box = document.createElement('div');
    const handler = jest.fn();

    box.addEventListener('click', handler);
    box.dispatchEvent(new Event('click'));

    expect(handler).toHaveBeenCalled();
  });

  it('should block handlers when only function body matches the search pattern', () => {
    window.EventTarget.prototype.addEventListener = original;
    preventAddEventListener('', 'console.log');

    const div = document.createElement('div');
    const handler = () => console.log('logging something');

    div.addEventListener('submit', handler);
    div.dispatchEvent(new Event('submit'));

    expect(typeof handler).toBe('function');
  });
});
