import { isProxyable } from './isProxyable';
import { createLogger } from './logger';

const logger = createLogger('defineProxyChain');

type AnyObject = { [key: string]: any };

interface ProxyCallbacks {
  onGet?: () => void;
  onSet?: () => void;
}

export function defineProxyChain(root: AnyObject, chain: string, callbacks: ProxyCallbacks): void {
  const parts = chain.split('.');
  let current = root;

  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    const isLast = i === parts.length - 1;

    // Final property in the chain.
    if (isLast) {
      // Methods like document.createElement live on the prototype (Document.prototype),
      // not on the instance, so walk the prototype chain to find the descriptor.
      // An own-property-only lookup would capture undefined and shadow the real method.
      let holder: AnyObject | null = current;
      let originalDescriptor: PropertyDescriptor | undefined;
      while (holder !== null && (originalDescriptor = Object.getOwnPropertyDescriptor(holder, part)) === undefined) {
        holder = Object.getPrototypeOf(holder);
      }
      const originalGetter = originalDescriptor?.get;
      const originalSetter = originalDescriptor?.set;
      let originalValue = originalDescriptor?.value;

      Object.defineProperty(current, part, {
        configurable: true,
        enumerable: true,
        get() {
          if (callbacks.onGet) {
            callbacks.onGet();
          }

          return originalGetter ? originalGetter.call(this) : originalValue;
        },
        set(newValue) {
          if (callbacks.onSet) {
            callbacks.onSet();
          }

          if (originalSetter) {
            originalSetter.call(this, newValue);
          } else {
            originalValue = newValue;
          }
        },
      });
    } else {
      // `in` checks the whole prototype chain: inherited intermediates like
      // document.defaultView or navigator.serviceWorker must be descended into,
      // not shadowed by the deferred trap below (which would make them read
      // back as undefined).
      if (!(part in current)) {
        let internalValue: any;

        const createProxy = (target: AnyObject, chainParts: string[]) => {
          return new Proxy(target, {
            get(target, prop) {
              const value = Reflect.get(target, prop, target);
              if (chainParts.length === 1 && prop === chainParts[0]) {
                if (callbacks.onGet) {
                  callbacks.onGet();
                }

                return value;
              }
              if (prop === chainParts[0] && isProxyable(value)) {
                return createProxy(value, chainParts.slice(1));
              }
              return value;
            },
            set(target, prop, value) {
              if (chainParts.length === 1 && prop === chainParts[0]) {
                if (callbacks.onSet) {
                  callbacks.onSet();
                }
              }
              return Reflect.set(target, prop, value);
            },
          });
        };

        Object.defineProperty(current, part, {
          configurable: true,
          enumerable: true,
          get() {
            if (isProxyable(internalValue)) {
              return createProxy(internalValue, parts.slice(i + 1));
            } else {
              return internalValue;
            }
          },
          set(newValue) {
            internalValue = newValue;
          },
        });
        return;
      }

      // Move into the next level of the chain.
      current = current[part];
      if (!isProxyable(current)) {
        // E.g. an inherited accessor that currently returns null, like
        // document.body while parsing is still inside <head>. Trapping deeper
        // would mean shadowing a live accessor, so give up instead. This
        // deliberately also gives up on inherited data properties holding
        // undefined (where a deferred trap could catch a later instance
        // assignment): the trap would hide later prototype assignments, and
        // not breaking the page outweighs supporting that rare pattern.
        logger.warn(`Giving up on "${chain}": "${part}" is not an object`);
        return;
      }
    }
  }
}
