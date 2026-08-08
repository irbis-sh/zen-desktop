import { Button, FormGroup, Tag } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AppToaster } from '@/common/toaster';
import { GetLocalResourcesStats, ResetLocalResourcesStats } from 'wails/go/app/App';
import { EventsOn } from 'wails/runtime';

interface LocalResourcesStats {
  totalSinceInstall: number;
  totalSinceReset: number;
  filterHits: number;
  byLibrary: Record<string, number>;
  byCDN: Record<string, number>;
}

export function InjectionCounter() {
  const { t } = useTranslation();
  const [stats, setStats] = useState<LocalResourcesStats | null>(null);

  useEffect(() => {
    const load = async () => {
      setStats(await GetLocalResourcesStats());
    };
    load();

    // Refresh whenever the engine serves a resource so the counter stays
    // current even when Settings is already open.
    const cancelFilter = EventsOn('filter:action', (action: { kind?: string }) => {
      if (action.kind === 'local') {
        load();
      }
    });
    const cancelStats = EventsOn('localcdn:stats', () => {
      load();
    });

    return () => {
      cancelFilter();
      cancelStats();
    };
  }, []);

  async function reset() {
    try {
      await ResetLocalResourcesStats();
    } catch (err) {
      AppToaster.show({
        message: t('localResources.counter.resetError', { error: err }),
        intent: 'danger',
      });
      return;
    }
    setStats(await GetLocalResourcesStats());
  }

  const libraryEntries = stats ? Object.entries(stats.byLibrary).sort((a, b) => b[1] - a[1]) : [];
  const cdnEntries = stats ? Object.entries(stats.byCDN).sort((a, b) => b[1] - a[1]) : [];

  return (
    <FormGroup label={t('localResources.counter.label')} helperText={t('localResources.counter.cacheNote')}>
      <div className="local-resources__counter-stats">
        <div className="local-resources__counter-row">
          <span>{t('localResources.counter.total')}</span>
          <span className="local-resources__counter-value">{stats?.totalSinceReset ?? '…'}</span>
        </div>
        {cdnEntries.length > 0 && (
          <div>
            <div className="bp6-text-muted">{t('localResources.counter.perCDN')}</div>
            <div className="local-resources__counter-library-tags">
              {cdnEntries.map(([cdn, count]) => (
                <Tag key={cdn} minimal>
                  {cdn}: {count}
                </Tag>
              ))}
            </div>
          </div>
        )}
        {libraryEntries.length > 0 && (
          <div>
            <div className="bp6-text-muted">{t('localResources.counter.perLibrary')}</div>
            <div className="local-resources__counter-library-tags">
              {libraryEntries.map(([library, count]) => (
                <Tag key={library} minimal>
                  {library}: {count}
                </Tag>
              ))}
            </div>
          </div>
        )}
      </div>
      <Button onClick={reset} disabled={!stats}>
        {t('localResources.counter.reset')}
      </Button>
    </FormGroup>
  );
}
