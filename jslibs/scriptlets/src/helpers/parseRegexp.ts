const regexpLiteralPattern = /^\/((?:[^/\\\r\n]|\\.)+)\/([gimsuy]*)$/;

/**
 * Parses a regular expression string into a RegExp object.
 * @param input The string to parse.
 * @returns The RegExp if the input is a valid RegExp string, null otherwise.
 */
export function parseRegexpLiteral(input: string): RegExp | null {
  const match = input.match(regexpLiteralPattern);
  if (match === null) {
    return null;
  }

  try {
    return new RegExp(match[1], match[2]);
  } catch {
    return null;
  }
}

/**
 * Parses a string into a RegExp. This is a more lenient version of parseRegexp, as it does not require the input to be surrounded by slashes.
 * @param input The string to parse.
 * @param flags Optional flags to pass to the RegExp constructor.
 * @returns The RegExp if the input is a valid RegExp string, null otherwise.
 */
export function parseRegexpFromString(input: string, flags?: string): RegExp | null {
  try {
    return new RegExp(escapeRegexp(input), flags);
  } catch {
    return null;
  }
}

function escapeRegexp(input: string): string {
  return input.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
