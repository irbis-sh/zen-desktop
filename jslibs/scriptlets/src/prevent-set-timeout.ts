import { createLogger } from './helpers/logger';
import { shouldPrevent } from './helpers/shouldPrevent';

const logger = createLogger('prevent-set-timeout');

export function preventSetTimeout(search = '', delay = '') {
  if (typeof Proxy === 'undefined') {
    logger.warn('Proxy not available in this environment');
    return;
  }

  window.setTimeout = new Proxy(window.setTimeout, {
    apply: (target: typeof setTimeout, thisArg: typeof window, args: Parameters<typeof setTimeout>) => {
      const [callback, timer] = args;

      if (
        shouldPrevent({
          callback,
          delay: timer,
          matchCallback: search,
          matchDelay: delay,
        })
      ) {
        logger.info(`Prevented setTimeout(${String(callback)}, ${timer})"`);
        return 0;
      }

      return Reflect.apply(target, thisArg, args);
    },
  });
}
