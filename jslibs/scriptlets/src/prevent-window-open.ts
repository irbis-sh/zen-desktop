import { createLogger } from './helpers/logger';
import { parseRegexpFromString, parseRegexpLiteral } from './helpers/parseRegexp';
import { parseValidInt } from './helpers/parseValidInt';

const logger = createLogger('prevent-window-open');

type Handler = ProxyHandler<typeof window.open>['apply'];

export function preventWindowOpen(match?: string, delayOrSearch?: string, replacement?: string) {
  let handler: Handler;

  try {
    if (match === '1' || match === '0') {
      handler = makeOldSyntaxHandler(match, delayOrSearch, replacement);
    } else {
      handler = makeNewSyntaxHandler(match, delayOrSearch, replacement);
    }
  } catch (error) {
    logger.warn('Error while making handler', { ex: error });
    return;
  }

  window.open = new Proxy(window.open, { apply: handler });
}

function makeOldSyntaxHandler(match?: string, search?: string, replacement?: string): Handler {
  let invertMatch = false;
  if (match === '0') {
    invertMatch = true;
  }

  let matchRe: RegExp | undefined;
  if (typeof search === 'string' && search.length > 0) {
    matchRe = (parseRegexpLiteral(search) || parseRegexpFromString(search)) ?? undefined;
    if (matchRe === undefined) {
      throw new Error('Could not parse search');
    }
  }

  let returnValue: (() => void) | (() => true) | Record<string, () => void> = () => {};
  if (replacement === 'trueFunc') {
    returnValue = () => true;
  } else if (typeof replacement === 'string' && replacement.length > 0) {
    if (!replacement.startsWith('{') || !replacement.endsWith('}')) {
      throw new Error(`Invalid replacement ${replacement}`);
    }
    const content = replacement.slice(1, -1);
    const parts = content.split('=');
    if (parts.length !== 2 || parts[0].length === 0 || parts[1] !== 'noopFunc') {
      throw new Error(`Invalid replacement ${replacement}`);
    }
    returnValue = { [parts[0]]: () => {} };
  }

  return (target, thisArg, args: Parameters<typeof window.open>) => {
    let url: string;
    if (args.length === 0 || args[0] == undefined) {
      // This is a valid case.
      // https://developer.mozilla.org/en-US/docs/Web/API/Window/open#url
      url = '';
    } else if (typeof args[0] === 'string') {
      url = args[0];
    } else if (args[0] instanceof URL) {
      url = args[0].toString();
    } else {
      // Bad input, let the original function handle it.
      return Reflect.apply(target, thisArg, args);
    }

    if (matchRe !== undefined) {
      let prevent = matchRe.test(url);
      if (invertMatch) {
        prevent = !prevent;
      }
      if (!prevent) {
        return Reflect.apply(target, thisArg, args);
      }
    }

    logger.info('Preventing window.open', { args });

    return returnValue;
  };
}

function makeNewSyntaxHandler(match?: string, delay?: string, replacement?: string): Handler {
  let invertMatch = false;
  let matchRe: RegExp | undefined;
  if (typeof match === 'string' && match.length > 0) {
    invertMatch = match[0] === '!';
    if (invertMatch) {
      match = match.slice(1);
    }

    matchRe = (parseRegexpLiteral(match) || parseRegexpFromString(match)) ?? undefined;
    if (matchRe === undefined) {
      throw new Error('Could not parse match');
    }
  }

  let parsedDelaySeconds: number;
  if (typeof delay === 'string' && delay.length > 0) {
    parsedDelaySeconds = parseValidInt(delay);
  }

  if (typeof replacement === 'string' && replacement !== 'obj' && replacement !== 'blank') {
    throw new Error(`Replacement type ${replacement} not supported`);
  }

  return (target, thisArg, args: Parameters<typeof window.open>) => {
    let url: string;
    if (args.length === 0 || args[0] == undefined) {
      // This is a valid case.
      // https://developer.mozilla.org/en-US/docs/Web/API/Window/open#url
      url = '';
    } else if (typeof args[0] === 'string') {
      url = args[0];
    } else if (args[0] instanceof URL) {
      url = args[0].toString();
    } else {
      // Bad input, let the original function handle it.
      return Reflect.apply(target, thisArg, args);
    }

    if (matchRe !== undefined) {
      let prevent = matchRe.test(url);
      if (invertMatch) {
        prevent = !prevent;
      }
      if (!prevent) {
        return Reflect.apply(target, thisArg, args);
      }
    }

    logger.info('Preventing window.open', { args });

    if (replacement === 'blank') {
      return Reflect.apply(target, thisArg, ['about:blank', ...args.slice(1)]);
    }

    let decoy: HTMLObjectElement | HTMLIFrameElement;
    switch (replacement) {
      case 'obj':
        decoy = document.createElement('object');
        decoy.data = url;
        break;
      default:
        decoy = document.createElement('iframe');
        decoy.src = url;
    }
    // Move the element far off-screen.
    decoy.style.setProperty('height', '1px', 'important');
    decoy.style.setProperty('width', '1px', 'important');
    decoy.style.setProperty('position', 'absolute', 'important');
    decoy.style.setProperty('top', '-9999px', 'important');
    document.body.appendChild(decoy);
    if (parsedDelaySeconds !== undefined) {
      setTimeout(() => {
        decoy.remove();
      }, parsedDelaySeconds * 1000);
    }

    let fakeWindow: WindowProxy | null;
    switch (replacement) {
      case 'obj':
        fakeWindow = decoy.contentWindow;
        if (fakeWindow === null || typeof fakeWindow !== 'object') {
          return null;
        }
        Object.defineProperties(fakeWindow, {
          closed: { value: false },
          opener: { value: window },
          frameElement: { value: null },
        });
        break;
      default:
        // We do not end up using the decoy here, which replicates the behavior of uBo and AdGuard.
        // Creating an iframe is likely still essential, either because triggering the URL
        // has some significance in the application's logic or because it helps bypass anti-adblock detections.
        //
        // Below we follow uBo's approach of creating a fake WindowProxy, with a slight modification
        // to ignore property assignments:
        // - https://github.com/gorhill/uBlock/blob/8629f07138749e7c6088fbfda84a381f2cd3bc66/src/js/resources/scriptlets.js#L2048-L2058
        // Also, for reference, see AdGuard's implementation:
        // - https://github.com/AdguardTeam/Scriptlets/blob/1324cfab78b9366010e1d9bfe8070dd11dd8421b/src/scriptlets/prevent-window-open.js#L161
        fakeWindow = new Proxy(window, {
          get: (target, prop, receiver) => {
            if (prop === 'closed') {
              return false;
            }
            const r = Reflect.get(target, prop, receiver);
            if (typeof r === 'function') {
              return () => {};
            }
            return r;
          },
          set: () => {
            return true;
          },
        });
    }

    return fakeWindow;
  };
}
