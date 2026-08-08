import { FormGroup, Switch, Tooltip } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AppToaster } from '@/common/toaster';
import { useProxyState } from '@/context/ProxyStateContext';
import { GetLocalResourcesLibraries, SetLocalResourcesLibraryEnabled } from 'wails/go/app/App';

interface LibraryInfo {
  key: string;
  name: string;
  license: string;
  version: string;
  enabled: boolean;
  resourceCount: number;
}

export function LibraryList() {
  const { t } = useTranslation();
  const { isProxyRunning } = useProxyState();
  const [state, setState] = useState({
    libraries: [] as LibraryInfo[],
    loading: true,
  });

  useEffect(() => {
    (async () => {
      const libraries = await GetLocalResourcesLibraries();
      setState({ libraries: libraries ?? [], loading: false });
    })();
  }, []);

  async function setLibraryEnabled(key: string, enabled: boolean) {
    setState((state) => ({ ...state, loading: true }));
    try {
      await SetLocalResourcesLibraryEnabled(key, enabled);
    } catch (err) {
      AppToaster.show({
        message: t('localResources.libraries.toggleError', { error: err }),
        intent: 'danger',
      });
      setState((state) => ({ ...state, loading: false }));
      return;
    }
    setState((state) => ({
      ...state,
      loading: false,
      libraries: state.libraries.map((library) => (library.key === key ? { ...library, enabled } : library)),
    }));
  }

  return (
    <FormGroup label={t('localResources.libraries.label')} helperText={t('localResources.libraries.description')}>
      <Tooltip content={t('common.stopProxyToModify') as string} disabled={!isProxyRunning} placement="top">
        <div className="local-resources__library-list">
          {state.libraries.map((library) => (
            <Switch
              key={library.key}
              checked={library.enabled}
              disabled={state.loading || isProxyRunning}
              label={`${library.name} (${library.version})`}
              title={library.license}
              onChange={() => {
                setLibraryEnabled(library.key, !library.enabled);
              }}
            />
          ))}
        </div>
      </Tooltip>
    </FormGroup>
  );
}
