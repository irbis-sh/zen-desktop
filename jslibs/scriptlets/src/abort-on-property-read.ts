import { defineProxyChain } from './helpers/defineProxyChain';
import { createLogger } from './helpers/logger';
import { generateRandomId } from './helpers/randomId';

const logger = createLogger('abort-on-property-read');

export function abortOnPropertyRead(property: string): void {
  if (typeof property !== 'string' || property.length === 0) {
    logger.warn('property should be a non-empty string');
    return;
  }

  const rid = generateRandomId();
  const abort = () => {
    logger.info(`Blocked ${property} read`);
    throw new ReferenceError(`Aborted script with ID: ${rid}`);
  };

  defineProxyChain(window, property, {
    onGet: abort,
  });

  // Enhance error handling for the thrown ReferenceError
  window.addEventListener('error', (event) => {
    if (event.error instanceof ReferenceError && event.error.message.includes(rid)) {
      event.preventDefault();
    }
  });
}
