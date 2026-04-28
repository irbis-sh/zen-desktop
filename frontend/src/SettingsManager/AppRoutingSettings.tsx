import { Button, Callout, FormGroup, Radio, RadioGroup, Tooltip } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { AppToaster } from '@/common/toaster';
import { useProxyState } from '@/context/ProxyStateContext';
import { SelectAppForRouting } from 'wails/go/app/App';
import { GetRouting, SetRouting } from 'wails/go/config/Config';
import { config } from 'wails/go/models';

export function AppRoutingSettings() {
  const { t } = useTranslation();
  const { isProxyRunning } = useProxyState();
  const [routing, setRoutingState] = useState<config.RoutingConfig | null>(null);

  useEffect(() => {
    (async () => {
      setRoutingState(await GetRouting());
    })();
  }, []);

  const loading = routing === null;
  const disabled = loading || isProxyRunning;
  const showAllowlistEmptyWarning = routing?.mode === config.RoutingMode.ALLOWLIST && routing.appPaths.length === 0;

  async function saveRouting(nextRouting: config.RoutingConfig) {
    const previousRouting = routing;
    setRoutingState(nextRouting);
    try {
      await SetRouting(nextRouting);
    } catch (err) {
      setRoutingState(previousRouting);
      AppToaster.show({
        message: t('appRoutingSettings.saveError', { error: err }),
        intent: 'danger',
      });
    }
  }

  async function addApp() {
    if (!routing) {
      return;
    }

    let path: string;
    try {
      path = await SelectAppForRouting();
    } catch (err) {
      AppToaster.show({
        message: t('appRoutingSettings.selectError', { error: err }),
        intent: 'danger',
      });
      return;
    }

    if (!path || routing.appPaths.includes(path)) {
      return;
    }

    await saveRouting({ ...routing, appPaths: [...routing.appPaths, path] });
  }

  async function removeApp(path: string) {
    if (!routing) {
      return;
    }

    await saveRouting({ ...routing, appPaths: routing.appPaths.filter((app) => app !== path) });
  }

  return (
    <FormGroup label={t('appRoutingSettings.label')} helperText={t('appRoutingSettings.description')}>
      <Tooltip content={t('common.stopProxyToModify') as string} disabled={!isProxyRunning} placement="top">
        <div className="settings-manager__app-routing">
          <RadioGroup
            onChange={(event) => {
              if (!routing) {
                return;
              }
              const mode = event.currentTarget.value as config.RoutingMode;
              void saveRouting({ ...routing, mode });
            }}
            selectedValue={routing?.mode ?? config.RoutingMode.BLOCKLIST}
          >
            <Radio
              label={t('appRoutingSettings.blocklistMode') as string}
              value={config.RoutingMode.BLOCKLIST}
              disabled={disabled}
            />
            <Radio
              label={t('appRoutingSettings.allowlistMode') as string}
              value={config.RoutingMode.ALLOWLIST}
              disabled={disabled}
            />
          </RadioGroup>

          {showAllowlistEmptyWarning && (
            <Callout intent="warning" className="settings-manager__app-routing-warning" compact>
              {t('appRoutingSettings.emptyAllowlistWarning')}
            </Callout>
          )}

          <div className="settings-manager__app-routing-list">
            {(routing?.appPaths ?? []).map((path) => (
              <div className="settings-manager__app-routing-app" key={path}>
                <div className="settings-manager__app-routing-app-name">{appDisplayName(path)}</div>
                <div className="settings-manager__app-routing-app-path bp6-text-muted">{path}</div>
                <Button
                  icon="cross"
                  variant="minimal"
                  size="small"
                  aria-label={t('appRoutingSettings.removeApp') as string}
                  disabled={disabled}
                  onClick={() => {
                    void removeApp(path);
                  }}
                />
              </div>
            ))}
          </div>

          <Button
            icon="plus"
            text={t('appRoutingSettings.addApp')}
            size="small"
            disabled={disabled}
            onClick={() => {
              void addApp();
            }}
          />
        </div>
      </Tooltip>
    </FormGroup>
  );
}

function appDisplayName(path: string): string {
  const normalizedPath = path.replace(/\\/g, '/');
  return normalizedPath.split('/').filter(Boolean).pop() ?? path;
}
