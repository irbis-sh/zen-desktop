import { createLogger } from './helpers/logger';
import { matchFetchArgs, ParsedPropsToMatch, parsePropsToMatch } from './helpers/request';

type ResponseBodyType = 'emptyObj' | 'emptyArr' | 'emptyStr' | '';
type ResponseType = 'basic' | 'cors' | 'opaque';

const logger = createLogger('prevent-fetch');

export function preventFetch(
  propsToMatch: string,
  responseBodyType: ResponseBodyType = 'emptyObj',
  responseType?: ResponseType,
) {
  if (typeof fetch === 'undefined' || typeof Proxy === 'undefined' || typeof Response === 'undefined') {
    logger.warn('Either fetch, Proxy, or Response is not supported in this environment');
    return;
  }

  let responseBody;
  switch (responseBodyType) {
    case '':
    case 'emptyObj':
      responseBody = '{}';
      break;
    case 'emptyArr':
      responseBody = '[]';
      break;
    case 'emptyStr':
      responseBody = '';
      break;
    default:
      logger.warn(`Invalid responseBody: ${responseBodyType}`);
      return;
  }

  if (typeof responseType === 'string') {
    if (responseType !== 'basic' && responseType !== 'cors' && responseType !== 'opaque') {
      logger.warn(`Invalid responseType: ${responseType}`);
      return;
    }
  }

  let parsedProps: ParsedPropsToMatch;
  try {
    parsedProps = parsePropsToMatch(propsToMatch);
  } catch (ex) {
    logger.warn('Error parsing props', ex);
  }

  // @ts-ignore
  fetch = new Proxy(fetch, {
    apply: async (target, thisArg, args: Parameters<typeof fetch>) => {
      if (!matchFetchArgs(parsedProps, args)) {
        return Reflect.apply(target, thisArg, args);
      }

      const response = new Response(responseBody, {
        status: 200,
        statusText: 'OK',
        headers: new Headers({
          'Content-Length': responseBody.length.toString(),
          'Content-Type': 'application/json',
          Date: new Date().toUTCString(),
        }),
      });

      if (responseType === 'opaque') {
        // https://fetch.spec.whatwg.org/#concept-filtered-response-opaque
        Object.defineProperties(response, {
          url: { value: '' },
          status: { value: 0 },
          statusText: { value: '' },
          body: { value: null },
          type: { value: 'opaque' },
          headers: { value: new Headers() },
        });
      } else {
        let url: string;
        if (args[0] instanceof URL) {
          url = args[0].toString();
        } else if (args[0] instanceof Request) {
          url = args[0].url;
        } else {
          url = args[0];
        }

        Object.defineProperties(response, {
          url: { value: url },
          type: { value: responseType || 'basic' },
        });
      }

      return response;
    },
  });
}
