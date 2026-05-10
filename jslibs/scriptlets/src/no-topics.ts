import { createLogger } from './helpers/logger';

const logger = createLogger('no-topics');

export function noTopics() {
  const browsingTopicsKey = 'browsingTopics';

  if (typeof Document !== 'function' || typeof Document.prototype !== 'object') {
    return;
  }

  const descriptor = Object.getOwnPropertyDescriptor(Document.prototype, browsingTopicsKey);
  if (!descriptor || !descriptor.configurable || typeof descriptor.value !== 'function') {
    return;
  }

  const original = descriptor.value;
  const fakeFn = new Proxy(original, {
    apply: () => {
      logger.info('Preventing Topics API usage');

      return Promise.resolve(
        new Response('', {
          status: 200,
          statusText: 'OK',
        }),
      );
    },
  });

  Object.defineProperty(Document.prototype, browsingTopicsKey, {
    configurable: true,
    get: () => {
      return fakeFn;
    },
    set: () => {},
  });
}
