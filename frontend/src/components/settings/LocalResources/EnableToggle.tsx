import { FormGroup, Switch, Tooltip } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AppToaster } from '@/common/toaster';
import { useProxyState } from '@/context/ProxyStateContext';
import { GetLocalResourcesSettings, SetLocalResourcesEnabled } from 'wails/go/app/App';

export function EnableToggle() {
  const { t } = useTranslation();
  const { isProxyRunning } = useProxyState();
  const [state, setState] = useState({
    enabled: true,
    loading: true,
  });

  useEffect(() => {
    (async () => {
      const settings = await GetLocalResourcesSettings();
      setState({ enabled: settings.enabled, loading: false });
    })();
  }, []);

  async function setEnabled(enabled: boolean) {
    setState((state) => ({ ...state, loading: true }));
    try {
      await SetLocalResourcesEnabled(enabled);
    } catch (err) {
      AppToaster.show({
        message: t('localResources.enable.setError', { error: err }),
        intent: 'danger',
      });
      setState((state) => ({ ...state, loading: false }));
      return;
    }
    setState((state) => ({ ...state, enabled, loading: false }));
  }

  return (
    <FormGroup
      label={t('localResources.enable.label')}
      labelFor="local-resources-enabled"
      helperText={t('localResources.enable.description')}
    >
      <Tooltip content={t('common.stopProxyToModify') as string} disabled={!isProxyRunning} placement="top">
        <Switch
          id="local-resources-enabled"
          checked={state.enabled}
          large
          disabled={state.loading || isProxyRunning}
          onClick={() => {
            setEnabled(!state.enabled);
          }}
        />
      </Tooltip>
    </FormGroup>
  );
}
