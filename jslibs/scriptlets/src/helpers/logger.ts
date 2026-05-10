/* eslint-disable no-console */
const PRODUCT_NAME = 'Zen';

export function createLogger(scriptletName: string) {
  return {
    log(line: string, ...context: any[]) {
      console.log(`${PRODUCT_NAME} (${scriptletName}): ${line}`, ...context);
    },
    debug(line: string, ...context: any[]) {
      console.debug(`${PRODUCT_NAME} (${scriptletName}): ${line}`, ...context);
    },
    info(line: string, ...context: any[]) {
      console.info(`${PRODUCT_NAME} (${scriptletName}): ${line}`, ...context);
    },
    warn(line: string, ...context: any[]) {
      console.warn(`${PRODUCT_NAME} (${scriptletName}): ${line}`, ...context);
    },
    error(line: string, ...context: any[]) {
      console.error(`${PRODUCT_NAME} (${scriptletName}): ${line}`, ...context);
    },
  };
}
