/**
 * This file is used to add fetch globals to the JSDOM environment in Jest.
 * https://github.com/jsdom/jsdom/issues/1724#issuecomment-1446858041
 */
import { Blob } from 'node:buffer';
import { TextEncoder, TextDecoder } from 'node:util';

import JSDOMEnvironment from 'jest-environment-jsdom';

// https://github.com/facebook/jest/blob/v29.4.3/website/versioned_docs/version-29.4/Configuration.md#testenvironment-string
export default class FixJSDOMEnvironment extends JSDOMEnvironment {
  constructor(...args: ConstructorParameters<typeof JSDOMEnvironment>) {
    super(...args);

    this.global.fetch = fetch;
    this.global.Headers = Headers;
    this.global.Request = Request;
    this.global.Response = Response;
    this.global.TextEncoder = TextEncoder;
    this.global.TextDecoder = TextDecoder as any;
    this.global.Blob = Blob as any;
  }
}
