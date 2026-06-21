import { FormGroup, NumericInput, Tooltip } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDebouncedCallback } from 'use-debounce';

import { AppToaster } from '@/common/toaster';
import { useProxyState } from '@/context/ProxyStateContext';
import { GetPACPort, SetPACPort } from 'wails/go/config/Config';

export function PACPortInput() {
  const { t } = useTranslation();
  const { isProxyRunning } = useProxyState();
  const [state, setState] = useState({
    port: 0,
    loading: true,
  });

  useEffect(() => {
    (async () => {
      const port = await GetPACPort();
      setState({ ...state, port, loading: false });
    })();
  }, []);

  const setPort = useDebouncedCallback(async (port: number) => {
    try {
      await SetPACPort(port);
    } catch (ex) {
      AppToaster.show({
        message: t('pacPortInput.setError', { error: ex }),
        intent: 'danger',
      });
    }
  }, 500);

  return (
    <FormGroup
      label={t('pacPortInput.label')}
      labelFor="pac-port"
      helperText={
        <>
          {t('pacPortInput.description')}
          <br />
          {t('pacPortInput.helper')}
        </>
      }
    >
      <Tooltip content={t('common.stopProxyToModify') as string} disabled={!isProxyRunning} placement="top">
        <NumericInput
          id="pac-port"
          min={0}
          max={65535}
          value={state.port}
          onValueChange={(port) => {
            if (Number.isNaN(port)) {
              return;
            }
            setState({ ...state, port });
            setPort(port);
          }}
          disabled={state.loading || isProxyRunning}
        />
      </Tooltip>
    </FormGroup>
  );
}
