import { Button, Dialog, DialogBody, DialogFooter, FormGroup, Switch, Tag, Tooltip } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import './index.css';

import { AppToaster } from '@/common/toaster';
import { ProxyState } from '@/types';
import { HardwareKeyUnavailableReason, SetCAKeyStorage } from 'wails/go/app/App';
import { GetKeyStorage } from 'wails/go/config/Config';
import { config } from 'wails/go/models';
import { Environment } from 'wails/runtime';

export interface CAKeyStorageSwitchProps {
  proxyState: ProxyState;
}

export function CAKeyStorageSwitch({ proxyState }: CAKeyStorageSwitchProps) {
  const { t } = useTranslation();
  const [state, setState] = useState({
    platform: '',
    storage: config.KeyStorageType.DISK,
    unavailableReason: '',
    isOpen: false,
    loading: false,
  });

  useEffect(() => {
    (async () => {
      const [{ platform }, storage] = await Promise.all([Environment(), GetKeyStorage()]);
      setState((state) => ({ ...state, platform, storage }));
    })();
    // The first probe can take a second on Windows, so the switch renders without waiting for it.
    // Enabling hardware storage probes again in the backend, so acting before this returns is safe.
    (async () => {
      const unavailableReason = await HardwareKeyUnavailableReason();
      setState((state) => ({ ...state, unavailableReason }));
    })();
  }, []);

  // Linux has no hardware key support yet.
  if (state.platform !== 'darwin' && state.platform !== 'windows') {
    return null;
  }

  const hardware = state.storage === config.KeyStorageType.HARDWARE;
  let disabledReason: string | undefined;
  if (proxyState !== 'off') {
    disabledReason = t('settings.caKeyStorage.stopProxyTooltip');
  } else if (!hardware && state.unavailableReason) {
    // Switching back to disk must stay possible even if the hardware has gone away.
    disabledReason = t('settings.caKeyStorage.unavailableTooltip', { reason: state.unavailableReason });
  }

  async function regenerate() {
    setState((state) => ({ ...state, loading: true }));
    try {
      await SetCAKeyStorage(hardware ? config.KeyStorageType.DISK : config.KeyStorageType.HARDWARE);
    } catch (err) {
      AppToaster.show({
        message: t('settings.caKeyStorage.errorMessage', { error: err }),
        intent: 'danger',
      });
    }
    // Reflect the saved value, so the switch stays put if any step failed.
    const storage = await GetKeyStorage();
    setState((state) => ({ ...state, storage, isOpen: false, loading: false }));
  }

  return (
    <>
      <FormGroup
        label={t('settings.caKeyStorage.label')}
        labelFor="caKeyStorage"
        labelInfo={
          <Tag minimal intent="warning">
            {t('settings.caKeyStorage.experimental')}
          </Tag>
        }
        helperText={
          state.platform === 'darwin'
            ? t('settings.caKeyStorage.descriptionMacOS')
            : t('settings.caKeyStorage.descriptionWindows')
        }
      >
        <Tooltip content={disabledReason}>
          <Switch
            id="caKeyStorage"
            checked={hardware}
            size="large"
            disabled={disabledReason !== undefined || state.loading}
            onChange={() => setState((state) => ({ ...state, isOpen: true }))}
          />
        </Tooltip>
      </FormGroup>

      <Dialog
        isOpen={state.isOpen}
        onClose={() => setState((state) => ({ ...state, isOpen: false }))}
        title={t('settings.caKeyStorage.confirmTitle')}
        isCloseButtonShown={!state.loading}
        className="ca-key-storage-dialog"
      >
        <DialogBody>
          <p>{t('settings.caKeyStorage.confirmBody')}</p>
        </DialogBody>
        <DialogFooter
          actions={
            <>
              <Button disabled={state.loading} onClick={() => setState((state) => ({ ...state, isOpen: false }))}>
                {t('settings.caKeyStorage.cancel')}
              </Button>
              <Button intent="primary" loading={state.loading} onClick={regenerate}>
                {t('settings.caKeyStorage.confirm')}
              </Button>
            </>
          }
        />
      </Dialog>
    </>
  );
}
