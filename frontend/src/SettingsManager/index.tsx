import { Button, Tag } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import './index.css';

import { BrowserLink } from '@/common/BrowserLink';
import { useProxyState } from '@/context/ProxyStateContext';
import { IsNoSelfUpdate } from 'wails/go/app/App';
import { GetVersion } from 'wails/go/config/Config';
import { BrowserOpenURL } from 'wails/runtime';

import { AppRoutingSettings } from './AppRoutingSettings';
import { AssetPortInput } from './AssetPortInput';
import { AutostartSwitch } from './AutostartSwitch';
import { AutoupdateSwitch } from './AutoupdateSwitch';
import { ExportDebugDataButton } from './ExportDebugDataButton';
import { ExportLogsButton } from './ExportLogsButton';
import { IgnoredHostsInput } from './IgnoredHostsInput';
import { LocaleSelector } from './LocaleSelector';
import { PortInput } from './PortInput';
import { ThemeSelector } from './ThemeSelector';
import { UninstallCADialog } from './UninstallCADialog';

const GITHUB_URL = 'https://github.com/irbis-sh/zen-desktop';
const CHANGELOG_URL = `${GITHUB_URL}/blob/master/CHANGELOG.md`;

export function SettingsManager() {
  const { t } = useTranslation();
  const [state, setState] = useState({
    version: '',
    updatePolicy: '',
    showUpdateRadio: false,
  });
  const { proxyState } = useProxyState();

  useEffect(() => {
    (async () => {
      const [version, noSelfUpdate] = await Promise.all([GetVersion(), IsNoSelfUpdate()]);

      setState((prev) => ({
        ...prev,
        showUpdateRadio: !noSelfUpdate,
        version,
      }));
    })();
  }, []);

  return (
    <div className="settings-manager">
      <div className="settings-manager__section--app">
        <Tag size="large" intent="primary" fill className="settings-manager__section-header">
          {t('settings.sections.app')}
        </Tag>

        <div className="settings-manager__section-body">
          <LocaleSelector />
          <AutostartSwitch />
          {state.showUpdateRadio && <AutoupdateSwitch />}
          <ThemeSelector />
          <div className="settings-manager__section--links">
            <ExportLogsButton />
            <ExportDebugDataButton />
          </div>
        </div>
      </div>

      <div className="settings-manager__section--advanced">
        <Tag size="large" intent="warning" fill className="settings-manager__section-header">
          {t('settings.sections.advanced')}
        </Tag>

        <div className="settings-manager__section-body">
          <PortInput />
          <AssetPortInput />
          <IgnoredHostsInput />
          <AppRoutingSettings />
          <UninstallCADialog proxyState={proxyState} />
        </div>
      </div>

      <div className="settings-manager__about bp6-text-muted">
        <div>
          <strong>Zen</strong>
        </div>
        <div>{t('settings.about.tagline')}</div>
        <div>
          {t('settings.about.version')}: {state.version}
          <span className="settings-manager__about-changelog">
            (<BrowserLink href={CHANGELOG_URL}>{t('settings.about.changelog')}</BrowserLink>)
          </span>
        </div>
        <div>© 2026 Zen contributors</div>
        <Button
          variant="minimal"
          size="small"
          icon="git-branch"
          className="settings-manager__about-github-button"
          onClick={() => BrowserOpenURL(GITHUB_URL)}
        >
          GitHub
        </Button>
      </div>
    </div>
  );
}
