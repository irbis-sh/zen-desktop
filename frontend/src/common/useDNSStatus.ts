import { useEffect, useState } from 'react';

import { GetDNSStatus } from 'wails/go/app/App';
import { resolver } from 'wails/go/models';
import { EventsOn } from 'wails/runtime';

/**
 * The modes are the values of resolver.StatusMode in internal/dns/resolver. An empty mode means no resolver runs.
 */
export function useDNSStatus() {
  const [status, setStatus] = useState<resolver.Status | null>(null);

  useEffect(() => {
    let gotEvent = false;
    const cancel = EventsOn('dns:status', (status: resolver.Status) => {
      gotEvent = true;
      setStatus(status);
    });
    (async () => {
      const status = await GetDNSStatus();
      // An event that arrived while this call was on its way is newer.
      if (!gotEvent) {
        setStatus(status);
      }
    })();
    return cancel;
  }, []);

  return status;
}
