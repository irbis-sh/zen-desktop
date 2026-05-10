/**
 * Parses a string into a valid integer.
 * @param input The string to parse
 * @returns The parsed integer
 * @throws If the input is NaN or Infinity
 */
export function parseValidInt(input: string): number {
  const parsed = parseInt(input, 10);
  if (isNaN(parsed)) {
    throw new Error('input is NaN');
  } else if (!Number.isFinite(parsed)) {
    throw new Error('input is Infinite');
  }

  return parsed;
}
