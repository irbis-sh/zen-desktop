import { FormGroup, InputGroup, Tooltip } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDebouncedCallback } from 'use-debounce';

import { AppToaster } from '@/common/toaster';
import { useProxyState } from '@/context/ProxyStateContext';
import { GetLocalResourcesSettings, SetLocalResourcesCustomDir } from 'wails/go/app/App';

export function CustomResourceDir() {
  const { t } = useTranslation();
  const { isProxyRunning } = useProxyState();
  const [state, setState] = useState({
    customDir: '',
    loading: true,
  });

  useEffect(() => {
    (async () => {
      const settings = await GetLocalResourcesSettings();
      setState({ customDir: settings.customDir ?? '', loading: false });
    })();
  }, []);

  const setCustomDir = useDebouncedCallback(async (customDir: string) => {
    try {
      await SetLocalResourcesCustomDir(customDir);
    } catch (err) {
      AppToaster.show({
        message: t('localResources.customDir.setError', { error: err }),
        intent: 'danger',
      });
    }
  }, 500);

  return (
    <FormGroup
      label={t('localResources.customDir.label')}
      labelFor="local-resources-custom-dir"
      helperText={
        <>
          {t('localResources.customDir.description')}
          <br />
          {t('localResources.customDir.helper')}
        </>
      }
    >
      <Tooltip content={t('common.stopProxyToModify') as string} disabled={!isProxyRunning} placement="top">
        <InputGroup
          id="local-resources-custom-dir"
          placeholder="/path/to/local-resources"
          value={state.customDir}
          disabled={state.loading || isProxyRunning}
          onChange={(e) => {
            const { value } = e.target;
            setState({ ...state, customDir: value });
            setCustomDir(value);
          }}
        />
      </Tooltip>
    </FormGroup>
  );
}
