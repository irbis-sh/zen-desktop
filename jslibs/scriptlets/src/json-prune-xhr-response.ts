import { createPrune } from './helpers/jsonPrune';
import { createLogger } from './helpers/logger';
import { matchXhrArgs, ParsedPropsToMatch, parsePropsToMatch } from './helpers/request';

const logger = createLogger('json-prune-xhr-response');

// Use Symbols to avoid interference with any other scriptlets or libraries.
const requestHeaders = Symbol('requestHeaders');
const shouldPrune = Symbol('shouldPrune');
const openArgs = Symbol('openArgs');
const responseHeaders = Symbol('responseHeaders');

interface Uninitialized extends XMLHttpRequest {
  [shouldPrune]?: boolean;
}

interface ToPrune extends XMLHttpRequest {
  [shouldPrune]: true;
  [requestHeaders]: [string, string][];
  [openArgs]: Parameters<typeof XMLHttpRequest.prototype.open>;
  // The reason for using an array instead of a map here is that response headers get rarely accessed,
  // so it's better to prioritize lower memory footprint and initialization cost over access speed.
  [responseHeaders]: [string, string][];
}

type ExtendedXHR = Uninitialized & ToPrune;

export function jsonPruneXHRResponse(
  propsToRemove: string,
  requiredProps?: string,
  propsToMatch?: string,
  stack?: string,
): void {
  if (typeof Proxy === 'undefined') {
    logger.warn('Proxy is not supported in this environment');
    return;
  }
  if (typeof propsToRemove !== 'string' || propsToRemove.length === 0) {
    logger.warn('propsToMatch cannot be empty');
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

  const { open: nativeOpen, send: nativeSend } = window.XMLHttpRequest.prototype;

  XMLHttpRequest.prototype.open = new Proxy(XMLHttpRequest.prototype.open, {
    apply: (target, thisArg: ExtendedXHR, args: Parameters<typeof XMLHttpRequest.prototype.open>) => {
      if (parsedProps !== undefined && !matchXhrArgs(parsedProps, ...args)) {
        return Reflect.apply(target, thisArg, args);
      }

      thisArg[shouldPrune] = true;
      thisArg[requestHeaders] = [];
      thisArg[openArgs] = args;
      thisArg[responseHeaders] = [];

      return Reflect.apply(target, thisArg, args);
    },
  });

  XMLHttpRequest.prototype.setRequestHeader = new Proxy(XMLHttpRequest.prototype.setRequestHeader, {
    apply: (target, thisArg: ExtendedXHR, args: Parameters<typeof XMLHttpRequest.prototype.setRequestHeader>) => {
      if (!thisArg[shouldPrune]) {
        return Reflect.apply(target, thisArg, args);
      }

      thisArg[requestHeaders].push(args);
      return Reflect.apply(target, thisArg, args);
    },
  });

  XMLHttpRequest.prototype.send = new Proxy(XMLHttpRequest.prototype.send, {
    apply: (target, thisArg: ExtendedXHR, args: Parameters<typeof XMLHttpRequest.prototype.send>) => {
      if (!thisArg[shouldPrune]) {
        return Reflect.apply(target, thisArg, args);
      }

      // Create a substitute request to capture the response.
      const subsReq = new XMLHttpRequest();
      subsReq.addEventListener('readystatechange', async () => {
        if (subsReq.readyState !== XMLHttpRequest.DONE) {
          return;
        }

        const newProps: PropertyDescriptorMap & ThisType<typeof thisArg> = {
          readyState: { value: subsReq.readyState, writable: false },
          responseURL: { value: subsReq.responseURL, writable: false },
          status: { value: subsReq.status, writable: false },
          statusText: { value: subsReq.statusText, writable: false },
          response: { value: subsReq.response, writable: false },
        };
        try {
          // responseXML might throw when accessed:
          // https://developer.mozilla.org/en-US/docs/Web/API/XMLHttpRequest/responseXML#exceptions
          newProps.responseXML = { value: subsReq.responseXML, writable: false };
        } catch {
          /* intentionally left empty */
        }
        try {
          // responseText might throw when accessed:
          // https://developer.mozilla.org/en-US/docs/Web/API/XMLHttpRequest/responseText#exceptions
          newProps.responseText = { value: subsReq.responseText, writable: false };
        } catch {
          /* intentionally left empty */
        }

        try {
          if (subsReq.responseType === '' || subsReq.responseType === 'text') {
            const obj = JSON.parse(subsReq.responseText);
            prune(obj);
            const stringified = JSON.stringify(obj);

            newProps.response = { value: stringified, writable: false };
            newProps.responseText = { value: stringified, writable: false };
          } else if (subsReq.responseType === 'arraybuffer') {
            // Assume UTF-8. JSON.parse will throw an error if our assumption is incorrect.
            const decoded = new TextDecoder().decode(subsReq.response);
            const obj = JSON.parse(decoded);
            prune(obj);
            const reencoded = new TextEncoder().encode(JSON.stringify(obj));

            newProps.response = { value: reencoded, writable: false };
          } else if (subsReq.responseType === 'blob') {
            // Assume UTF-8.
            const decoded = await subsReq.response.text();
            const obj = JSON.parse(decoded);
            prune(obj);
            const reencoded = new Blob([JSON.stringify(obj)]);

            newProps.response = { value: reencoded, writable: false };
          } else if (subsReq.responseType === 'json') {
            prune(subsReq.response);
            newProps.response = { value: subsReq.response, writable: false };
          } else {
            throw new Error(`Unsupported type: ${subsReq.responseType}`);
          }
        } catch (ex) {
          logger.error('Error parsing/pruning response', ex);
        }

        Object.defineProperties(thisArg, newProps);

        const headers = subsReq.getAllResponseHeaders();
        for (const header of headers.trim().split(/[\r\n]+/)) {
          const [name, value] = header.split(': ');
          thisArg[responseHeaders].push([name, value]);
        }

        setTimeout(() => {
          // Executing this from within setTimeout to accommodate clients that set event handlers after calling send.
          thisArg.dispatchEvent(new Event('readystatechange'));
          thisArg.dispatchEvent(new Event('load'));
          thisArg.dispatchEvent(new Event('loadend'));
        }, 1);
      });

      nativeOpen.apply(subsReq, thisArg[openArgs]);

      for (const [name, value] of thisArg[requestHeaders]) {
        subsReq.setRequestHeader(name, value);
      }

      try {
        nativeSend.apply(subsReq, args);
      } catch (ex) {
        logger.error('Error sending substitute request', ex);
        return Reflect.apply(target, thisArg, args);
      }
    },
  });

  XMLHttpRequest.prototype.getResponseHeader = new Proxy(XMLHttpRequest.prototype.getResponseHeader, {
    apply: (target, thisArg: ExtendedXHR, args: Parameters<typeof XMLHttpRequest.prototype.getResponseHeader>) => {
      if (!thisArg[shouldPrune]) {
        return Reflect.apply(target, thisArg, args);
      }

      let res = null;
      for (const [name, value] of thisArg[responseHeaders]) {
        if (name === args[0]) {
          res = value;
          break;
        }
      }
      return res;
    },
  });

  XMLHttpRequest.prototype.getAllResponseHeaders = new Proxy(XMLHttpRequest.prototype.getAllResponseHeaders, {
    apply: (target, thisArg: ExtendedXHR, args: Parameters<typeof XMLHttpRequest.prototype.getAllResponseHeaders>) => {
      if (!thisArg[shouldPrune]) {
        return Reflect.apply(target, thisArg, args);
      }

      return thisArg[responseHeaders].map(([name, value]) => `${name}: ${value}`).join('\r\n');
    },
  });
}
