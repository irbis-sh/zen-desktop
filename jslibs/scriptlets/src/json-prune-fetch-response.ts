import { createPrune } from './helpers/jsonPrune';
import { createLogger } from './helpers/logger';
import { matchFetchArgs, ParsedPropsToMatch, parsePropsToMatch } from './helpers/request';

const logger = createLogger('json-prune-fetch-response');

export function jsonPruneFetchResponse(
  propsToRemove: string,
  requiredProps?: string,
  propsToMatch?: string,
  stack?: string,
): void {
  if (typeof Proxy === 'undefined' || typeof fetch === 'undefined' || typeof Response === 'undefined') {
    logger.warn('Either Proxy, fetch, or Response is not supported in this environment');
    return;
  }

  if (typeof propsToRemove !== 'string' || propsToRemove.length === 0) {
    logger.warn('propsToRemove cannot be empty');
    return;
  }

  let parsedProps: ParsedPropsToMatch;
  if (typeof propsToMatch === 'string') {
    try {
      parsedProps = parsePropsToMatch(propsToMatch);
    } catch (ex) {
      logger.warn('error parsing propsToMatch', ex);
      return;
    }
  }

  const prune = createPrune(propsToRemove, requiredProps, stack);

  window.fetch = new Proxy(window.fetch, {
    apply: async (target, thisArg: any, args: Parameters<typeof fetch>) => {
      if (parsedProps && !matchFetchArgs(parsedProps, args)) {
        return Reflect.apply(target, thisArg, args);
      }

      const response = await Reflect.apply(target, thisArg, args);
      const cloned = response.clone();

      let json: any;
      try {
        json = await response.json();
      } catch (ex) {
        return cloned;
      }

      prune(json);

      const prunedResponse = new Response(JSON.stringify(json), {
        status: response.status,
        statusText: response.statusText,
        headers: response.headers,
      });
      Object.defineProperties(prunedResponse, {
        url: { value: response.url },
        type: { value: response.type },
        ok: { value: response.ok },
        redirected: { value: response.redirected },
      });
      return prunedResponse;
    },
  });
}
