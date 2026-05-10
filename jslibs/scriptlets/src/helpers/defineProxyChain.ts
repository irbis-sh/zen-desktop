import { isProxyable } from './isProxyable';

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
    const chainSoFar = parts.slice(0, i + 1);

    // final property in chain
    if (isLast) {
      const propName = chainSoFar[chainSoFar.length - 1];
      const originalDescriptor = Object.getOwnPropertyDescriptor(current, propName) || {};
      const originalGetter = originalDescriptor.get;
      const originalSetter = originalDescriptor.set;
      let originalValue = originalDescriptor.value;

      Object.defineProperty(current, parts[parts.length - 1], {
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
      const isConfigurable = isPropertyConfigurable(current, part);
      const propExists = Object.prototype.hasOwnProperty.call(current, part);

      if (isConfigurable && !propExists) {
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
    }
  }
}

function isPropertyConfigurable(o: AnyObject, prop: string): boolean {
  if (!o) {
    return true;
  }

  const descriptor = Object.getOwnPropertyDescriptor(o, prop);
  if (!descriptor) {
    return true;
  }

  return Boolean(descriptor.configurable);
}
