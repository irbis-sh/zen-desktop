import { createLogger } from './helpers/logger';

const logger = createLogger('no-protected-audience');

export function noProtectedAudience() {
  if (typeof Navigator !== 'function' || typeof Navigator.prototype !== 'object') {
    return;
  }

  const methodsToPatch = {
    joinAdInterestGroup: () => Promise.resolve(),
    runAdAuction: () => Promise.resolve(null),
    leaveAdInterestGroup: () => Promise.resolve(),
    clearOriginJoinedAdInterestGroups: () => Promise.resolve(),
    createAuctionNonce: () => '',
    updateAdInterestGroups: () => {},
  };

  for (const key of Object.keys(methodsToPatch) as (keyof typeof methodsToPatch)[]) {
    const descriptor = Object.getOwnPropertyDescriptor(Navigator.prototype, key);

    if (!descriptor || !descriptor.configurable || typeof descriptor.value !== 'function') {
      continue;
    }

    const original = descriptor.value;
    const fakeFn = new Proxy(original, {
      apply: () => {
        logger.info(`Preventing usage of Protected Audience API: ${key}`);
        return methodsToPatch[key]();
      },
    });

    Object.defineProperty(Navigator.prototype, key, {
      configurable: true,
      get: () => fakeFn,
      set: () => {},
    });
  }
}
