import { Button, Icon } from '@blueprintjs/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useDNSStatus } from '@/common/useDNSStatus';

import './index.css';

interface DNSStatusBarProps {
  onOpen: () => void;
  onClose: () => void;
}

export function DNSStatusBar({ onOpen, onClose }: DNSStatusBarProps) {
  const { t } = useTranslation();
  const status = useDNSStatus();
  // Only "failing": the VPN and no-IPv4 modes are the feature working as designed, and the settings page shows them.
  const failing = status?.mode === 'failing';

  // Closing the bar hides it until encrypted DNS works again or the proxy stops. On a network that blocks DoH it would
  // otherwise stay for as long as the user is connected, and the settings page still shows the status. The VPN and
  // no-IPv4 modes don't count: a reconnect on such a network passes through no-IPv4 and would bring the bar back.
  const [dismissed, setDismissed] = useState(false);
  if (dismissed && (status?.mode === 'active' || status?.mode === '')) {
    setDismissed(false);
  }

  // The live region stays mounted, because screen readers announce content added to an existing region, but often not
  // a region that is added along with its content.
  return (
    <div role="status">
      {failing && !dismissed && (
        <div className="dns-status-bar">
          <button type="button" className="dns-status-bar__open" title={t('dnsStatusBar.open')} onClick={onOpen}>
            <Icon icon="warning-sign" className="dns-status-bar__icon" />
            {t('dnsStatusBar.failing')}
          </button>
          <Button
            variant="minimal"
            size="small"
            intent="warning"
            icon="cross"
            aria-label={t('dnsStatusBar.close')}
            title={t('dnsStatusBar.close')}
            className="dns-status-bar__close"
            onClick={() => {
              setDismissed(true);
              onClose();
            }}
          />
        </div>
      )}
    </div>
  );
}
