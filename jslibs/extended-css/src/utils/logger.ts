/* eslint-disable no-console */
const PRODUCT_NAME = 'Zen';

export function createLogger(namespace: string) {
  return {
    log(line: string, ...context: any[]) {
      console.log(`${PRODUCT_NAME}/extended-css/${namespace}: ${line}`, ...context);
    },
    debug(line: string, ...context: any[]) {
      console.debug(`${PRODUCT_NAME}/extended-css/${namespace}: ${line}`, ...context);
    },
    info(line: string, ...context: any[]) {
      console.info(`${PRODUCT_NAME}/extended-css/${namespace}: ${line}`, ...context);
    },
    warn(line: string, ...context: any[]) {
      console.warn(`${PRODUCT_NAME}/extended-css/${namespace}: ${line}`, ...context);
    },
    error(line: string, ...context: any[]) {
      console.error(`${PRODUCT_NAME}/extended-css/${namespace}: ${line}`, ...context);
    },
  };
}
