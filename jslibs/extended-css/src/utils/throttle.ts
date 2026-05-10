export function throttle<T extends (...args: any[]) => any>(fn: T, delayMs: number): (...args: Parameters<T>) => void {
  let lastCallAt: number | undefined = undefined;
  let timerId: number | undefined = undefined;

  return (...args: Parameters<T>) => {
    if (timerId !== undefined) return;

    const current = performance.now();

    if (lastCallAt !== undefined) {
      const elapsed = current - lastCallAt;
      if (elapsed < delayMs) {
        const remaining = delayMs - elapsed;
        timerId = window.setTimeout(() => {
          timerId = undefined;
          lastCallAt = performance.now();
          fn(...args);
        }, remaining);
        return;
      }
    }

    lastCallAt = current;
    fn(...args);
  };
}
