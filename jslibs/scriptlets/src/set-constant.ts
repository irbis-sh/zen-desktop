import { isProxyable } from './helpers/isProxyable';
import { createLogger } from './helpers/logger';
import { matchStack } from './helpers/matchStack';
import { parseRegexpFromString, parseRegexpLiteral } from './helpers/parseRegexp';
import { runAt } from './helpers/runAt';

const logger = createLogger('set-constant');

const uboWrappers = new Map([
  ['function', 'asFunction'],
  ['callback', 'asCallback'],
  ['resolved', 'asResolved'],
  ['rejected', 'asRejected'],
]);

/**
 * Parses the arguments that follow the value.
 *
 * Zen passes the same argument list for both rule syntaxes, but the two engines
 * read these arguments differently. AdGuard reads a positional stack, value
 * wrapper and setProxyTrap. uBO reads name-value pairs, for example
 * `as, function`. The first argument tells them apart: uBO's pairs start with
 * `as` or `runAt`, which a stack pattern in a real rule never is. Without this,
 * `as` would become a stack pattern and the rule would apply only to calls from
 * a script whose stack contains `as`.
 */
function parseOptions(rest: string[]): {
  stack?: string;
  valueWrapper?: string;
  setProxyTrap?: string;
  when?: string;
} {
  if (rest[0] !== 'as' && rest[0] !== 'runAt') {
    const [stack, valueWrapper, setProxyTrap] = rest;
    return { stack, valueWrapper, setProxyTrap };
  }

  let valueWrapper: string | undefined;
  let when: string | undefined;
  for (let i = 0; i < rest.length; i += 2) {
    if (rest[i] === 'as') {
      // An unknown wrapper sets the plain value, as in uBO.
      valueWrapper = uboWrappers.get(rest[i + 1]);
    } else if (rest[i] === 'runAt') {
      when = rest[i + 1];
    }
  }
  return { valueWrapper, when };
}

export function setConstant(property: string, value: string, ...rest: string[]) {
  const { stack, valueWrapper, setProxyTrap, when } = parseOptions(rest);
  if (setProxyTrap !== undefined) {
    logger.warn('setProxyTrap will be ignored');
  }

  let fakeValue: any;
  switch (value) {
    case 'undefined':
      fakeValue = undefined;
      break;
    case 'false':
      fakeValue = false;
      break;
    case 'true':
      fakeValue = true;
      break;
    case 'null':
      fakeValue = null;
      break;
    case 'emptyObj':
    case '{}':
      fakeValue = {};
      break;
    case 'emptyArr':
    case '[]':
      fakeValue = [];
      break;
    case 'noopFunc':
      fakeValue = () => {};
      break;
    case 'noopCallbackFunc':
      fakeValue = () => () => {};
      break;
    case 'trueFunc':
      fakeValue = () => true;
      break;
    case 'falseFunc':
      fakeValue = () => false;
      break;
    case 'throwFunc':
      fakeValue = () => {
        throw new Error();
      };
      break;
    case 'noopPromiseResolve':
      fakeValue = () => {
        return Promise.resolve(
          new Response('', {
            status: 200,
            statusText: 'OK',
          }),
        );
      };
      break;
    case 'noopPromiseReject':
      fakeValue = () => Promise.reject();
      break;
    case '':
      fakeValue = '';
      break;
    case 'yes':
      fakeValue = 'yes';
      break;
    case 'no':
      fakeValue = 'no';
      break;
    default: {
      // uBO's range. AdGuard accepts -1 as its only negative number, but a small
      // number carries no code or data, so the wider range is harmless.
      if (/^-?\d+$/.test(value) && Math.abs(Number(value)) <= 32767) {
        fakeValue = Number(value);
        break;
      }
      throw new Error('Invalid value');
    }
  }

  const wrapped = fakeValue; // Avoid creating recursive functions by creating a temporary variable.
  switch (valueWrapper) {
    case 'asFunction':
      fakeValue = () => wrapped;
      break;
    case 'asCallback':
      fakeValue = () => () => wrapped;
      break;
    case 'asResolved':
      fakeValue = Promise.resolve(wrapped);
      break;
    case 'asRejected':
      fakeValue = Promise.reject(wrapped);
      // Without a handler, the browser logs "Uncaught (in promise)" on every page
      // the rule runs on. The page's own catch or then still sees the rejection.
      fakeValue.catch(() => {});
      break;
  }

  let stackRe: RegExp | null;
  if (stack !== undefined && stack !== '') {
    stackRe = parseRegexpLiteral(stack) || parseRegexpFromString(stack);
  }
  stackRe ??= null;

  // Only the trap waits for runAt. An invalid value still throws now, inside
  // the injected script's try/catch.
  runAt(() => trap(property, value, fakeValue, stackRe), when);
}

function trap(property: string, value: string, fakeValue: any, stackRe: RegExp | null) {
  const fakeLog = () => {
    logger.debug(`Returning fake value for property window.${property}`, { value });
  };

  if (!property.includes('.')) {
    let localValue = window[property as any];
    const odesc = Object.getOwnPropertyDescriptor(window, property);
    if (stackRe === null && isLockedWritable(odesc)) {
      window[property as any] = fakeValue;
      return;
    }
    Object.defineProperty(window, property, {
      configurable: true,
      get: () => {
        if (stackRe !== null && !matchStack(stackRe)) {
          return typeof odesc?.get === 'function' ? odesc.get.apply(window) : localValue;
        }
        fakeLog();
        return fakeValue;
      },
      set:
        typeof odesc?.set === 'function'
          ? odesc?.set.bind(window)
          : (v) => {
              localValue = v;
            },
    });
    return;
  }

  // Avoid infinite recursion in case we overwrite some sub-property of Object or Function.
  const nativeObject = Object;
  const nativeFunction = Function;
  const get = (chain: string[]) => {
    let proxyCache: { proxy: any; link: any };
    let boundFnCache: Record<any, any>;
    return (target: any, key: any) => {
      if (chain.length === 1 && chain[0] === key) {
        fakeLog();
        return fakeValue;
      }
      let link = Reflect.get(target, key, target);
      const desc = nativeObject.getOwnPropertyDescriptor(target, key);
      if (desc && 'value' in desc && !desc.configurable && !desc.writable) {
        // Get should return the original value for non-configurable, non-writable data properties.
        // https://tc39.es/ecma262/multipage/ordinary-and-exotic-objects-behaviours.html#sec-proxy-object-internal-methods-and-internal-slots-get-p-receiver
        return link;
      }

      if (
        typeof link === 'function' &&
        // This checks for native functions. The regex helps avoid false positives from functions containing the string "[native code]".
        // Function.prototype.toString is used to handle edge cases where a function has its toString method overridden.
        // Credit: https://stackoverflow.com/a/6599105
        /\{\s*\[native code\]/.test(nativeFunction.prototype.toString.call(link))
      ) {
        // Native functions frequently expect to be bounded to their original, **unproxied** object.
        // See https://stackoverflow.com/a/57580096 for more details.
        // Fixes https://github.com/irbis-sh/zen-desktop/issues/201
        if (boundFnCache !== undefined && boundFnCache[key]) {
          // Like with proxyCache, store the bound function to ensure object equality between different access operations.
          link = boundFnCache[key];
        } else {
          link = link.bind(target);
          if (boundFnCache === undefined) {
            boundFnCache = {};
          }
          boundFnCache[key] = link;
        }
      }
      if (chain[0] !== key || !isProxyable(link) || (stackRe !== null && !matchStack(stackRe))) {
        return link;
      }

      if (proxyCache?.link === link) {
        // Ensure object equality between different access operations.
        // Fixes https://github.com/irbis-sh/zen-desktop/issues/224
        return proxyCache.proxy;
      }
      const proxy = new Proxy(link, {
        get: get(chain.slice(1)),
      });
      proxyCache = { link, proxy };
      return proxy;
    };
  };

  const rootChain = property.split('.');
  const rootProp = rootChain[0] as any;
  let prev: any = window;
  let current: any = window[rootProp];
  for (let i = 1; i < rootChain.length; i++) {
    const part = rootChain[i] as any;

    if (current != undefined && i === rootChain.length - 1) {
      const odesc = Object.getOwnPropertyDescriptor(current, part);
      let localValue = current[part];
      if (stackRe === null && isLockedWritable(odesc)) {
        current[part] = fakeValue;
        return;
      }
      // Setting a getter/setter property plays more nicely with complex websites, YouTube in particular.
      Object.defineProperty(current, part, {
        configurable: true,
        get: () => {
          if (stackRe !== null && !matchStack(stackRe)) {
            return typeof odesc?.get === 'function' ? odesc.get.apply(current) : localValue;
          }
          fakeLog();
          return fakeValue;
        },
        set:
          typeof odesc?.set === 'function'
            ? odesc?.set.bind(current)
            : (v) => {
                localValue = v;
              },
      });
      return;
    }

    if (current == undefined || current[part] == undefined) {
      // Go up a level and create a proxy for the current property.
      const odesc = Object.getOwnPropertyDescriptor(prev, rootChain[i - 1]);
      if (odesc?.configurable === false && current != undefined) {
        // The parent can't take a getter, for example because it is a page's
        // top-level var, so trap the missing part on the object it holds. This
        // misses the page replacing that object later, which a trap on the
        // parent would catch.
        prev = current;
        current = current[part];
        continue;
      }
      let localValue = current;
      let proxyCache: { capturedValue: any; proxy: any };
      Object.defineProperty(prev, rootChain[i - 1], {
        configurable: true,
        get: () => {
          const capturedValue = odesc?.get ? odesc.get.apply(prev) : localValue;

          if (!isProxyable(capturedValue) || (stackRe !== null && !matchStack(stackRe))) {
            return capturedValue;
          }
          if (proxyCache?.capturedValue === capturedValue) {
            // Ensure object equality between different access operations.
            // Fixes https://github.com/irbis-sh/zen-desktop/issues/224
            return proxyCache.proxy;
          }
          const proxy = new Proxy(capturedValue, {
            get: get(rootChain.slice(i)),
          });
          proxyCache = { capturedValue, proxy };
          return proxy;
        },
        set:
          typeof odesc?.set === 'function'
            ? odesc?.set.bind(prev)
            : (v) => {
                localValue = v;
              },
      });
      return;
    }

    prev = current;
    current = current[part];
  }
  // For debugging purposes.
  logger.warn('Hit an invariant in setConstant', { property, value, stackRe });
  throw new Error('Invariant hit');
}

/**
 * Reports whether a property can't take a getter but can still be assigned.
 * A page's top-level var or function declaration makes such a property on
 * window, and a runAt rule can run after the page has declared it. uBO then
 * sets the value once, without a trap, and so does Zen. The page can
 * overwrite it later.
 *
 * Without a trap, the value can't depend on the caller's stack. A rule with a
 * stack pattern therefore skips this and fails in defineProperty, which logs
 * why, instead of applying to every caller.
 */
function isLockedWritable(desc: PropertyDescriptor | undefined): boolean {
  return desc !== undefined && !desc.configurable && desc.writable === true;
}
