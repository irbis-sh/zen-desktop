export function isProxyable(o: any): boolean {
  return o !== null && (typeof o === 'function' || typeof o === 'object');
}
