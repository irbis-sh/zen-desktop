import { abortCurrentInlineScript } from './abort-current-inline-script';
import { abortOnPropertyRead } from './abort-on-property-read';
import { abortOnPropertyWrite } from './abort-on-property-write';
import { abortOnStackTrace } from './abort-on-stack-trace';
import { createLogger } from './helpers/logger';
import { jsonPrune } from './json-prune';
import { jsonPruneFetchResponse } from './json-prune-fetch-response';
import { jsonPruneXHRResponse } from './json-prune-xhr-response';
import { noProtectedAudience } from './no-protected-audience';
import { noTopics } from './no-topics';
import { nowebrtc } from './nowebrtc';
import { preventAddEventListener } from './prevent-add-event-listener';
import { preventFetch } from './prevent-fetch';
import { preventSetInterval } from './prevent-set-interval';
import { preventSetTimeout } from './prevent-set-timeout';
import { preventWindowOpen } from './prevent-window-open';
import { preventXHR } from './prevent-xhr';
import { sanitizeClipboard } from './sanitize-clipboard';
import { setConstant } from './set-constant';
import { setLocalStorageItem } from './set-local-storage-item';
import { setSessionStorageItem } from './set-session-storage-item';

const logger = createLogger('index');

const scriptletNameToFunction = new Map<string, Function>([
  ['abort-current-inline-script', abortCurrentInlineScript],
  ['abort-on-property-read', abortOnPropertyRead],
  ['aopr', abortOnPropertyRead],
  ['abort-on-property-write', abortOnPropertyWrite],
  ['aopw', abortOnPropertyWrite],
  ['abort-on-stack-trace', abortOnStackTrace],
  ['aost', abortOnStackTrace],
  ['json-prune', jsonPrune],
  ['nowebrtc', nowebrtc],
  ['prevent-fetch', preventFetch],
  ['no-fetch-if', preventFetch],
  ['prevent-xhr', preventXHR],
  ['no-xhr-if', preventXHR],
  ['set-local-storage-item', setLocalStorageItem],
  ['set-session-storage-item', setSessionStorageItem],
  ['set-constant', setConstant],
  ['json-prune-fetch-response', jsonPruneFetchResponse],
  ['json-prune-xhr-response', jsonPruneXHRResponse],
  ['prevent-window-open', preventWindowOpen],
  ['nowoif', preventWindowOpen],
  ['prevent-setTimeout', preventSetTimeout],
  ['prevent-setInterval', preventSetInterval],
  ['prevent-addEventListener', preventAddEventListener],
  ['no-topics', noTopics],
  ['no-protected-audience', noProtectedAudience],
  ['sanitize-clipboard', sanitizeClipboard],
]);

export default function (name: string, ...args: string[]): void {
  if (!scriptletNameToFunction.has(name)) {
    logger.debug(`Scriptlet ${name} does not exist or is not yet implemented.`);
    return;
  }

  scriptletNameToFunction.get(name)!(...args);
}
