import { createPrune } from './helpers/jsonPrune';
import { createLogger } from './helpers/logger';

const logger = createLogger('json-prune');

export function jsonPrune(propsToRemove: string, requiredProps?: string, stack?: string): void {
  if (typeof Proxy === 'undefined') {
    logger.warn('Proxy not available in this environment');
    return;
  }

  if (typeof propsToRemove !== 'string' || propsToRemove.length === 0) {
    logger.warn('propsToRemove should be a non-empty string');
    return;
  }

  const prune = createPrune(propsToRemove, requiredProps, stack);

  JSON.parse = new Proxy(JSON.parse, {
    apply: (target, thisArg, args) => {
      const obj = Reflect.apply(target, thisArg, args);
      prune(obj);
      return obj;
    },
  });

  if (typeof Response !== 'undefined') {
    Response.prototype.json = new Proxy(Response.prototype.json, {
      apply: async (target, thisArg, args) => {
        const obj = await Reflect.apply(target, thisArg, args);
        prune(obj);
        return obj;
      },
    });
  }
}
