import { Card, Tag } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { GetLocalResourcesStats } from 'wails/go/app/App';
import { EventsOn } from 'wails/runtime';
import './index.css';

interface LocalResourcesStats {
  totalSinceInstall: number;
  totalSinceReset: number;
  filterHits: number;
  byLibrary: Record<string, number>;
  byCDN: Record<string, number>;
}

export function LocalResourcesSummary() {
  const { t } = useTranslation();
  const [stats, setStats] = useState<LocalResourcesStats | null>(null);

  useEffect(() => {
    const load = async () => {
      setStats(await GetLocalResourcesStats());
    };
    let timer: ReturnType<typeof setTimeout> | undefined;
    // Filter-list events can arrive in bursts (block/redirect/modify), so
    // debounce the refetch to avoid hammering the Wails binding.
    const scheduleLoad = () => {
      if (timer !== undefined) {
        return;
      }
      timer = setTimeout(() => {
        timer = undefined;
        load();
      }, 300);
    };

    load();

    // Refresh on every filter action (blocks/redirects/modifies update the
    // filter tally; local serves update the CDN tally) and on resets.
    const cancelFilter = EventsOn('filter:action', () => {
      scheduleLoad();
    });
    const cancelStats = EventsOn('localcdn:stats', () => {
      if (timer !== undefined) {
        clearTimeout(timer);
        timer = undefined;
      }
      load();
    });

    return () => {
      if (timer !== undefined) {
        clearTimeout(timer);
      }
      cancelFilter();
      cancelStats();
    };
  }, []);

  const cdnEntries = stats ? Object.entries(stats.byCDN).sort((a, b) => b[1] - a[1]) : [];

  return (
    <Card className="local-resources-summary" compact>
      <div className="local-resources-summary__line">
        <span className="local-resources-summary__label">
          {t('localResources.counter.total')} {t('localResources.counter.filter')} / {t('localResources.counter.cdn')}
        </span>
        <span className="local-resources-summary__values">
          <span className="local-resources-summary__value local-resources-summary__value--filter">
            {stats?.filterHits ?? '…'}
          </span>
          <span className="local-resources-summary__divider">/</span>
          <span className="local-resources-summary__value local-resources-summary__value--cdn">
            {stats?.totalSinceReset ?? '…'}
          </span>
        </span>
      </div>
      {cdnEntries.length > 0 && (
        <div>
          <div className="bp6-text-muted">{t('localResources.counter.perCDN')}</div>
          <div className="local-resources-summary__cdn-tags">
            {cdnEntries.map(([cdn, count]) => (
              <Tag key={cdn} minimal>
                {cdn}: {count}
              </Tag>
            ))}
          </div>
        </div>
      )}
    </Card>
  );
}
