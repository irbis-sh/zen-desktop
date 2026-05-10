import { parseRegexpFromString, parseRegexpLiteral } from './parseRegexp';

export function shouldPrevent({
  callback,
  delay,
  matchCallback,
  matchDelay,
}: {
  callback: unknown;
  delay: unknown;
  matchCallback: string;
  matchDelay: string;
}): boolean {
  if (typeof callback !== 'function' && typeof callback !== 'string') {
    return false;
  }

  if (typeof matchCallback !== 'string' || (matchDelay && !isValidDelayNumber(matchDelay))) {
    return false;
  }

  const { isInverted: isInvertedMatch, regexp: matchRegexp } = createRegExpForSearch(matchCallback);
  const { isInverted: isInvertedDelayMatch, match: delayMatch } = parseDelayMatch(matchDelay);

  const parsedDelay = parseDelay(delay);

  const callbackStr = String(callback);
  const callbackMatches = matchRegexp?.test(callbackStr) !== isInvertedMatch;
  const delayMatches = delayMatch !== null && (parsedDelay === delayMatch) !== isInvertedDelayMatch;

  if (delayMatch === null) {
    return callbackMatches;
  }
  if (!matchCallback) {
    return delayMatches;
  }

  return callbackMatches && delayMatches;
}

const parseReverse = (str: string): { isReverse: boolean; value: string } => {
  let isReverse = false;
  if (str.startsWith('!')) {
    str = str.slice(1);
    isReverse = true;
  }

  return {
    isReverse,
    value: str,
  };
};

const createRegExpForSearch = (str: string) => {
  const { isReverse, value } = parseReverse(str);

  return {
    isInverted: isReverse,
    regexp: parseRegexpLiteral(value) || parseRegexpFromString(value),
  };
};

const parseDelayMatch = (str: string) => {
  const { isReverse, value } = parseReverse(str);
  const delayValue = parseInt(value, 10);

  const delayRegexp = Number.isNaN(delayValue) ? null : delayValue;

  return {
    isInverted: isReverse,
    match: delayRegexp,
  };
};

const isValidDelayNumber = (value: unknown): boolean => {
  if (typeof value !== 'string') return false;

  const isNumber = (value: unknown) => {
    return typeof value === 'number' && !Number.isNaN(value) && Number.isFinite(value);
  };

  return value.startsWith('!') ? isNumber(+value.slice(1)) : isNumber(+value);
};

const parseDelay = <T>(delay: T): number | T => {
  const parsedDelay = Math.floor(parseInt(delay as string, 10));
  return typeof parsedDelay === 'number' && !Number.isNaN(parsedDelay) ? parsedDelay : delay;
};
