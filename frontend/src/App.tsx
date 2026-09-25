import { Button, ButtonGroup, FocusStyleManager, NonIdealState } from '@blueprintjs/core';
import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import './App.css';

import { RestartApplication } from 'wails/go/app/App';
import { GetFirstLaunch } from 'wails/go/config/Config';
import { EventsOn } from 'wails/runtime/runtime';

import { ThemeType, useTheme } from './common/ThemeManager';
import { AppToaster } from './common/toaster';
import { AppHeader } from './components/AppHeader';
import { StartStopButton } from './components/StartStopButton';
import { useProxyState } from './context/ProxyStateContext';
import { FilterLists } from './FilterLists';
import { Intro } from './Intro';
import { useProxyHotkey } from './ProxyHotkey';
import { RequestLog } from './RequestLog';
import { Rules } from './Rules';
import { SettingsManager } from './SettingsManager';

function App() {
  const { t } = useTranslation();
  const { effectiveTheme } = useTheme();

  useEffect(() => {
    FocusStyleManager.onlyShowFocusOnTabs();
  }, []);

  useEffect(() => {
    const cancel = EventsOn('app:update', (action: { kind: string }) => {
      if (action.kind === 'updateAvailable') {
        AppToaster.show({
          message: t('app.update.updateAvailable'),
          intent: 'primary',
          timeout: 0,
          action: {
            text: t('app.update.restart'),
            onClick: () => {
              const restartingToast = AppToaster.show({
                message: t('app.update.restarting'),
                intent: 'primary',
                timeout: 0,
              });
              RestartApplication().catch((error) => {
                AppToaster.dismiss(restartingToast);
                AppToaster.show({
                  message: t('app.update.restartFailed', { error }),
                  intent: 'danger',
                });
              });
            },
          },
        });
      }
    });

    return cancel;
  }, [t]);

  const { proxyState } = useProxyState();
  const [activeTab, setActiveTab] = useState<'home' | 'filterLists' | 'rules' | 'settings'>('home');
  const [showIntro, setShowIntro] = useState(false);

  useEffect(() => {
    GetFirstLaunch().then(setShowIntro);
  }, []);

  useProxyHotkey(showIntro);

  return (
    <div id="app" className={effectiveTheme === ThemeType.DARK ? 'bp6-dark' : ''}>
      <AppHeader />

      {showIntro ? (
        <Intro
          onClose={() => {
            setShowIntro(false);
          }}
        />
      ) : (
        <>
          <ButtonGroup fill variant="minimal" className="tabs">
            <Button icon="circle" active={activeTab === 'home'} onClick={() => setActiveTab('home')}>
              {t('app.tabs.home')}
            </Button>
            <Button icon="filter" active={activeTab === 'filterLists'} onClick={() => setActiveTab('filterLists')}>
              {t('app.tabs.filterLists')}
            </Button>
            <Button icon="code" active={activeTab === 'rules'} onClick={() => setActiveTab('rules')}>
              {t('app.tabs.rules')}
            </Button>
            <Button icon="settings" active={activeTab === 'settings'} onClick={() => setActiveTab('settings')}>
              {t('app.tabs.settings')}
            </Button>
          </ButtonGroup>

          <div className="content">
            <div style={{ display: activeTab === 'home' ? 'block' : 'none' }}>
              {proxyState === 'off' ? (
                <NonIdealState
                  icon="lightning"
                  title={t('app.proxy.inactive')}
                  description={t('app.proxy.description') as string}
                  className="request-log__non-ideal-state"
                />
              ) : (
                <RequestLog />
              )}
            </div>
            {activeTab === 'filterLists' && <FilterLists />}
            {activeTab === 'rules' && <Rules />}
            {activeTab === 'settings' && <SettingsManager />}
          </div>
          <StartStopButton />
        </>
      )}
    </div>
  );
}

export default App;
