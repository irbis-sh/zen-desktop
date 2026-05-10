import { defineProxyChain } from './helpers/defineProxyChain';
import { createLogger } from './helpers/logger';
import { matchStack } from './helpers/matchStack';
import { parseRegexpFromString, parseRegexpLiteral } from './helpers/parseRegexp';
import { generateRandomId } from './helpers/randomId';

const logger = createLogger('abort-on-stack-trace');

export function abortOnStackTrace(property: string, stack: string): void {
  if (typeof property !== 'string' || property.length === 0) {
    logger.warn('property should be a non-empty string');
    return;
  }
  if (typeof stack !== 'string' || stack.length === 0) {
    logger.warn('stack should be a non-empty string');
    return;
  }

  const stackRe = parseRegexpLiteral(stack) || parseRegexpFromString(stack);

  const rid = generateRandomId();
  const abort = () => {
    if (stackRe !== null && matchStack(stackRe)) {
      logger.info(`Blocked script on '${stack}' stack`);
      throw new ReferenceError(`Aborted script with ID: ${rid}`);
    }
  };

  defineProxyChain(window, property, {
    onGet: abort,
    onSet: abort,
  });

  // Enhance error handling for the thrown ReferenceError
  window.addEventListener('error', (event) => {
    if (event.error instanceof ReferenceError && event.error.message.includes(rid)) {
      event.preventDefault();
    }
  });
}
