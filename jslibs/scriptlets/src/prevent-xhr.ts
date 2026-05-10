import { createLogger } from './helpers/logger';
import { genRandomResponse, matchXhrArgs, ParsedPropsToMatch, parsePropsToMatch } from './helpers/request';

const logger = createLogger('prevent-xhr');

// Use Symbols to avoid interference with any other scriptlets or libraries.
const prevent = Symbol('prevent');
const url = Symbol('url');
const responseHeaders = Symbol('responseHeaders');

interface ExtendedXHR extends XMLHttpRequest {
  [prevent]?: boolean;
  [url]?: string;
  [responseHeaders]?: Record<string, string>;
}

export function preventXHR(propsToMatch: string, randomizeResponseTextPattern?: string): void {
  if (typeof Proxy === 'undefined') {
    logger.warn('Proxy is not supported in this environment');
    return;
  }
  if (typeof propsToMatch !== 'string') {
    logger.warn('propsToMatch is required');
    return;
  }

  let parsedProps: ParsedPropsToMatch;
  try {
    parsedProps = parsePropsToMatch(propsToMatch);
  } catch (ex) {
    logger.warn('error parsing props', ex);
    return;
  }

  const openOverride: ProxyHandler<typeof XMLHttpRequest.prototype.open>['apply'] = (
    target,
    thisArg: ExtendedXHR,
    args: Parameters<typeof XMLHttpRequest.prototype.open>,
  ) => {
    if (!thisArg[prevent] && !matchXhrArgs(parsedProps, ...args)) {
      thisArg[prevent] = false;
      return Reflect.apply(target, thisArg, args);
    }

    logger.debug('Preventing XHR request', args);
    thisArg[prevent] = true;
    thisArg[url] = args[1].toString();
    return Reflect.apply(target, thisArg, args);
  };

  const sendOverride: ProxyHandler<typeof XMLHttpRequest.prototype.send>['apply'] = (
    target,
    thisArg: ExtendedXHR,
    args: Parameters<typeof XMLHttpRequest.prototype.send>,
  ) => {
    if (!thisArg[prevent]) {
      return Reflect.apply(target, thisArg, args);
    }

    setTimeout(() => {
      const props = {
        readyState: { value: thisArg.DONE, writable: false },
        statusText: { value: 'OK', writable: false },
        response: { value: '' as string | Blob | ArrayBuffer | Document | object, writable: false },
        responseText: { value: '', writable: false },
        responseURL: { value: thisArg[url], writable: false },
        responseXML: { value: null as null | Document, writable: false },
        status: { value: 200, writable: false },
      };
      thisArg[responseHeaders] = {
        date: new Date().toUTCString(),
        'content-length': '0',
      };
      switch (thisArg.responseType) {
        case 'arraybuffer':
          props.response.value = new ArrayBuffer(0);
          thisArg[responseHeaders]['content-type'] = 'application/octet-stream';
          break;
        case 'blob':
          props.response.value = new Blob([]);
          thisArg[responseHeaders]['content-type'] = 'application/octet-stream';
          break;
        case 'document': {
          const doc = new DOMParser().parseFromString('', 'text/html');
          props.response.value = doc;
          props.responseXML.value = doc;
          thisArg[responseHeaders]['content-type'] = 'text/html';
          thisArg[responseHeaders]['content-length'] = doc.documentElement.outerHTML.length.toString();
          break;
        }
        case 'json':
          props.response.value = {};
          props.responseText.value = '{}';
          thisArg[responseHeaders]['content-type'] = 'application/json';
          thisArg[responseHeaders]['content-length'] = '2';
          break;
        default:
          thisArg[responseHeaders]['content-type'] = 'text/plain';
          if (typeof randomizeResponseTextPattern !== 'string' || randomizeResponseTextPattern === '') {
            break;
          }
          try {
            const responseText = genRandomResponse(randomizeResponseTextPattern);
            props.response.value = responseText;
            props.responseText.value = responseText;
            thisArg[responseHeaders]['content-length'] = responseText.length.toString();
          } catch (ex) {
            logger.error('Generating random response text', ex);
          }
      }
      Object.defineProperties(thisArg, props);

      thisArg.dispatchEvent(new Event('readystatechange'));
      thisArg.dispatchEvent(new Event('load'));
      thisArg.dispatchEvent(new Event('loadend'));
    }, 1);
  };

  const getResponseHeaderOverride: ProxyHandler<typeof XMLHttpRequest.prototype.getResponseHeader>['apply'] = (
    target,
    thisArg: ExtendedXHR,
    args: Parameters<typeof XMLHttpRequest.prototype.getResponseHeader>,
  ) => {
    if (!thisArg[prevent]) {
      return Reflect.apply(target, thisArg, args);
    }

    if (thisArg.readyState !== thisArg.DONE) {
      return null;
    }
    return thisArg[responseHeaders]![args[0].toLowerCase()] ?? null;
  };

  const getAllResponseHeadersOverride: ProxyHandler<typeof XMLHttpRequest.prototype.getAllResponseHeaders>['apply'] = (
    target,
    thisArg: ExtendedXHR,
    args: Parameters<typeof XMLHttpRequest.prototype.getAllResponseHeaders>,
  ) => {
    if (!thisArg[prevent]) {
      return Reflect.apply(target, thisArg, args);
    }

    if (thisArg.readyState !== thisArg.DONE) {
      return null;
    }

    let result = '';
    for (const [name, value] of Object.entries(thisArg[responseHeaders]!)) {
      result += `${name}: ${value}\r\n`;
    }
    return result;
  };

  XMLHttpRequest.prototype.open = new Proxy(XMLHttpRequest.prototype.open, {
    apply: openOverride,
  });
  XMLHttpRequest.prototype.send = new Proxy(XMLHttpRequest.prototype.send, {
    apply: sendOverride,
  });
  XMLHttpRequest.prototype.getResponseHeader = new Proxy(XMLHttpRequest.prototype.getResponseHeader, {
    apply: getResponseHeaderOverride,
  });
  XMLHttpRequest.prototype.getAllResponseHeaders = new Proxy(XMLHttpRequest.prototype.getAllResponseHeaders, {
    apply: getAllResponseHeadersOverride,
  });
}
