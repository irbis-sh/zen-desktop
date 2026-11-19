import { FormGroup, TextArea, Tooltip } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDebouncedCallback } from 'use-debounce';

import { useProxyState } from '@/context/ProxyStateContext';
import { GetIgnoredHosts, SetIgnoredHosts } from 'wails/go/config/Config';

export function IgnoredHostsInput() {
  const { t } = useTranslation();
  const { isProxyRunning } = useProxyState();
  const [state, setState] = useState({
    ignoredHosts: '',
    loading: true,
  });

  useEffect(() => {
    (async () => {
      const ignoredHosts = await GetIgnoredHosts();
      setState({ ignoredHosts: (ignoredHosts ?? []).join('\n'), loading: false });
    })();
  }, []);

  const setIgnoredHosts = useDebouncedCallback(async (ignoredHosts: string) => {
    await SetIgnoredHosts(
      ignoredHosts
        .split('\n')
        .map((host) => host.trim())
        .filter((host) => host.length > 0),
    );
  }, 500);

  return (
    <FormGroup
      label={t('ignoredHostsInput.label')}
      labelFor="ignoredHosts"
      helperText={
        <>
          {t('ignoredHostsInput.description')}
          <br />
          {t('ignoredHostsInput.helper')}
        </>
      }
    >
      <Tooltip
        content={t('common.stopProxyToModify') as string}
        disabled={!isProxyRunning}
        placement="top"
        className="settings-manager__ignored-hosts-tooltip"
      >
        <TextArea
          id="ignoredHosts"
          // Disabled, a placeholder looks the same as an entered host.
          placeholder={state.loading || isProxyRunning ? undefined : 'example.com'}
          className="settings-manager__ignored-hosts-input"
          value={state.ignoredHosts}
          onChange={(e) => {
            const { value } = e.target;
            setState({ ...state, ignoredHosts: value });
            setIgnoredHosts(value);
          }}
          disabled={state.loading || isProxyRunning}
          autoResize
          fill
        />
      </Tooltip>
    </FormGroup>
  );
}
