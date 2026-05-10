export function matchStack(stackToMatch: RegExp): boolean {
  const { stack } = new Error();
  if (stack === undefined) {
    return false;
  }

  return stack
    .split('\n')
    .slice(2) // Remove internal functions from the stacktrace.
    .map((l) => l.trim())
    .some((l) => stackToMatch.test(l));
}
