import { createLogger } from './logger';

const logger = createLogger('run-at');

// uBO's names for the document's ready states, in the order the states occur.
const readyStates = new Map([
  ['loading', 1],
  ['asap', 1],
  ['interactive', 2],
  ['end', 2],
  ['2', 2],
  ['complete', 3],
  ['idle', 3],
  ['3', 3],
]);

/**
 * Calls fn once the document reaches the ready state that `when` names, as
 * uBO's runAt option does. If the document is already there, or `when` is
 * missing or a name uBO doesn't know, fn runs straight away.
 */
export function runAt(fn: () => void, when: string | undefined): void {
  const target = readyStates.get(when ?? '') ?? 0;
  const reached = () => (readyStates.get(document.readyState) ?? 0) >= target;
  if (reached()) {
    fn();
    return;
  }

  const onChange = () => {
    if (!reached()) {
      return;
    }
    document.removeEventListener('readystatechange', onChange, { capture: true });
    try {
      fn();
    } catch (ex) {
      // The injected script's try/catch around the scriptlet has returned by
      // now. Without this, the error would reach the page's own error handlers.
      logger.error('Deferred call failed', ex);
    }
  };
  document.addEventListener('readystatechange', onChange, { capture: true });
}
