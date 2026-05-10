import { defineProxyChain } from './helpers/defineProxyChain';
import { createLogger } from './helpers/logger';
import { parseRegexpFromString, parseRegexpLiteral } from './helpers/parseRegexp';
import { generateRandomId } from './helpers/randomId';

const logger = createLogger('abort-current-inline-script');

export function abortCurrentInlineScript(property: string, search?: string | null): void {
  if (typeof property !== 'string' || property.length === 0) {
    logger.warn('property should be a non-empty string');
    return;
  }

  let searchRe: RegExp | null;
  if (typeof search === 'string' && search.length > 0) {
    searchRe = parseRegexpLiteral(search) || parseRegexpFromString(search);
  }
  const rid = generateRandomId();
  const currentScript = document.currentScript;

  const abort = () => {
    const element = document.currentScript;
    if (
      element instanceof HTMLScriptElement &&
      element !== currentScript &&
      (!searchRe || searchRe.test(element.textContent || ''))
    ) {
      logger.info(`Blocked ${property} in currentScript`);
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
