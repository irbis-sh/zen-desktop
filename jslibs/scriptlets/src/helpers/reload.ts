// These live in their own module so tests can replace them: jsdom implements
// neither location.reload nor performance.getEntriesByType.

export function reloadPage(): void {
  location.reload();
}

/**
 * Reports whether the current page load is itself a reload, either by the user
 * or by a previous run of a scriptlet.
 */
export function isReloadedLoad(): boolean {
  const entry = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
  return entry?.type === 'reload';
}
