import { parseValidInt } from '../parseValidInt';

const RANDOM_RESPONSE_PATTERN_REGEXP = /length:(\d+)-(\d+)/;
const RANDOM_STR_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()_+=~';

export function genRandomResponse(pattern: string): string {
  if (pattern === 'false') {
    return '';
  }

  let minLength: number;
  let maxLength: number;
  const regexpMatch = pattern.match(RANDOM_RESPONSE_PATTERN_REGEXP);
  if (pattern === 'true') {
    minLength = 10;
    maxLength = 10;
  } else if (regexpMatch) {
    minLength = parseValidInt(regexpMatch[1]);
    maxLength = parseValidInt(regexpMatch[2]);
    if (maxLength > 500000) {
      throw new Error('maxLength exceeds limit');
    }
    if (minLength > maxLength) {
      throw new Error('minLength exceeds maxLength');
    }
  } else {
    throw new Error('Invalid pattern');
  }

  const length = getRandomIntInclusive(minLength, maxLength);
  return genRandomStrWithLength(length);
}

function getRandomIntInclusive(min: number, max: number): number {
  min = Math.ceil(min);
  max = Math.floor(max);
  return Math.floor(Math.random() * (max - min + 1) + min);
}

function genRandomStrWithLength(length: number): string {
  let result = '';
  for (let i = 0; i < length; i++) {
    result += RANDOM_STR_ALPHABET.charAt(Math.floor(Math.random() * RANDOM_STR_ALPHABET.length));
  }
  return result;
}
