import { createLogger } from './helpers/logger';
import { parseRegexpLiteral } from './helpers/parseRegexp';

const logger = createLogger('sanitize-clipboard');

function cleanURL(raw: string, params: (string | RegExp)[]): string {
  try {
    const url = new URL(raw);
    let modified = false;

    for (const pattern of params) {
      if (typeof pattern === 'string') {
        if (url.searchParams.has(pattern)) {
          url.searchParams.delete(pattern);
          modified = true;
        }
      } else {
        for (const key of Array.from(url.searchParams.keys())) {
          if (pattern.test(key)) {
            url.searchParams.delete(key);
            modified = true;
          }
        }
      }
    }

    return modified ? url.toString() : raw;
  } catch {
    return raw;
  }
}

export function sanitizeClipboard(params: string) {
  if (typeof params !== 'string' || params.length === 0) {
    logger.warn('params should be a non-empty string');
    return;
  }

  const paramsToRemove = params.split(' ').map((p) => parseRegexpLiteral(p) || p);

  if (navigator.clipboard) {
    const handler: ProxyHandler<any> = {
      async apply(target, thisArg, args) {
        const [payload] = args;

        const txt = await Promise.resolve(payload);
        const cleaned = cleanURL(String(txt), paramsToRemove);

        if (cleaned === txt) {
          return Reflect.apply(target, thisArg, args);
        }

        logger.info(`Sanitized clipboard for '${String(txt)}'`);
        return Reflect.apply(target, thisArg, [cleaned]);
      },
    };

    navigator.clipboard.writeText = new Proxy(navigator.clipboard.writeText, handler);
  }

  const legacyHandler = (ev: Event): void => {
    const e = ev as ClipboardEvent | undefined;
    let text = window.getSelection()?.toString() ?? '';

    if (!text) {
      const el = document.activeElement as HTMLInputElement | HTMLTextAreaElement | null;

      if (
        el &&
        (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') &&
        el.selectionStart !== null &&
        el.selectionEnd !== null &&
        el.selectionStart !== el.selectionEnd
      ) {
        text = el.value.slice(el.selectionStart, el.selectionEnd);
      }
    }

    if (!text) return;
    const cleaned = cleanURL(text, paramsToRemove);
    if (cleaned === text) return;

    if (e?.clipboardData) {
      e.clipboardData.setData('text/plain', cleaned);
      e.preventDefault();

      logger.info(`Sanitized clipboard for '${text}'`);
    }
  };

  document.addEventListener('copy', legacyHandler as EventListener, true);
}
