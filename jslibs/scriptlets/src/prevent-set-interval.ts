import { createLogger } from './helpers/logger';
import { shouldPrevent } from './helpers/shouldPrevent';

const logger = createLogger('prevent-set-interval');

export function preventSetInterval(search = '', delay = '') {
  if (typeof Proxy === 'undefined') {
    logger.warn('Proxy not available in this environment');
    return;
  }

  window.setInterval = new Proxy(window.setInterval, {
    apply: (target: typeof setInterval, thisArg: typeof window, args: Parameters<typeof setInterval>) => {
      const [callback, timer] = args;

      if (
        shouldPrevent({
          callback,
          delay: timer,
          matchCallback: search,
          matchDelay: delay,
        })
      ) {
        logger.info(`Prevented setInterval(${String(callback)}, ${timer})"`);
        return 0;
      }

      return Reflect.apply(target, thisArg, args);
    },
  });
}
