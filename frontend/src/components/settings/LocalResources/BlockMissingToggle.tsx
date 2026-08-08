import { FormGroup, Switch, Tooltip } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AppToaster } from '@/common/toaster';
import { useProxyState } from '@/context/ProxyStateContext';
import { GetLocalResourcesSettings, SetLocalResourcesBlockMissing } from 'wails/go/app/App';

export function BlockMissingToggle() {
  const { t } = useTranslation();
  const { isProxyRunning } = useProxyState();
  const [state, setState] = useState({
    blockMissing: false,
    loading: true,
  });

  useEffect(() => {
    (async () => {
      const settings = await GetLocalResourcesSettings();
      setState({ blockMissing: settings.blockMissing, loading: false });
    })();
  }, []);

  async function setBlockMissing(blockMissing: boolean) {
    setState((state) => ({ ...state, loading: true }));
    try {
      await SetLocalResourcesBlockMissing(blockMissing);
    } catch (err) {
      AppToaster.show({
        message: t('localResources.blockMissing.setError', { error: err }),
        intent: 'danger',
      });
      setState((state) => ({ ...state, loading: false }));
      return;
    }
    setState((state) => ({ ...state, blockMissing, loading: false }));
  }

  return (
    <FormGroup
      label={t('localResources.blockMissing.label')}
      labelFor="local-resources-block-missing"
      helperText={t('localResources.blockMissing.description')}
    >
      <Tooltip content={t('common.stopProxyToModify') as string} disabled={!isProxyRunning} placement="top">
        <Switch
          id="local-resources-block-missing"
          checked={state.blockMissing}
          large
          disabled={state.loading || isProxyRunning}
          onClick={() => {
            setBlockMissing(!state.blockMissing);
          }}
        />
      </Tooltip>
    </FormGroup>
  );
}
