import { Callout, Classes, FormGroup, HTMLSelect, InputGroup, Tag, TextArea, Tooltip } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDebouncedCallback } from 'use-debounce';

import { AppToaster } from '@/common/toaster';
import { useDNSStatus } from '@/common/useDNSStatus';
import { useProxyState } from '@/context/ProxyStateContext';
import { GetDNS, SetDNSCustomURL, SetDNSServer, SetDNSSystemDomains } from 'wails/go/config/Config';
import { config } from 'wails/go/models';

const { DNSServer } = config;

export function DNSSettings() {
  const { t } = useTranslation();
  const { isProxyRunning } = useProxyState();
  const [server, setServer] = useState<config.DNSServer | null>(null);
  const [customURL, setCustomURL] = useState('');
  const [customURLInvalid, setCustomURLInvalid] = useState(false);
  const [systemDomains, setSystemDomains] = useState('');

  useEffect(() => {
    (async () => {
      const dns = await GetDNS();
      // A config without "dns" has an empty server, and a hand-edited one can hold anything. The backend uses the OS
      // resolver for both, so the page shows Off.
      setServer(Object.values(DNSServer).includes(dns.server) ? dns.server : DNSServer.SYSTEM);
      setCustomURL(dns.customURL);
      setSystemDomains((dns.systemDomains ?? []).join('\n'));
    })();
  }, []);

  async function selectServer(next: config.DNSServer) {
    const previous = server;
    setServer(next);
    try {
      await SetDNSServer(next);
    } catch (err) {
      setServer(previous);
      AppToaster.show({
        message: t('dnsSettings.saveError', { error: err }),
        intent: 'danger',
      });
    }
  }

  // Both debounced saves flush on unmount, because SettingsManager unmounts on every tab switch.
  const saveCustomURL = useDebouncedCallback(
    async (url: string) => {
      // No toast and not the backend's error: this runs while the user types, so it would show one for every
      // half-typed URL.
      try {
        await SetDNSCustomURL(url);
        setCustomURLInvalid(false);
      } catch {
        setCustomURLInvalid(true);
      }
    },
    500,
    { flushOnExit: true },
  );

  const saveSystemDomains = useDebouncedCallback(
    async (domains: string) => {
      try {
        await SetDNSSystemDomains(domains.split('\n'));
      } catch (err) {
        AppToaster.show({
          message: t('dnsSettings.saveError', { error: err }),
          intent: 'danger',
        });
      }
    },
    500,
    { flushOnExit: true },
  );

  const selected = server ?? DNSServer.SYSTEM;
  const disabled = server === null || isProxyRunning;
  const stopProxyTooltip = t('common.stopProxyToModify') as string;

  return (
    <>
      {/* Blueprint puts a group's helper text below all its children, so this group holds only the select. */}
      <FormGroup
        label={t('dnsSettings.label')}
        labelFor="dnsServer"
        labelInfo={
          <Tag minimal intent="warning">
            {t('settings.caKeyStorage.experimental')}
          </Tag>
        }
        helperText={t('dnsSettings.description')}
      >
        <Tooltip content={stopProxyTooltip} disabled={!isProxyRunning} placement="top">
          <HTMLSelect
            id="dnsServer"
            value={selected}
            onChange={(e) => void selectServer(e.currentTarget.value as config.DNSServer)}
            disabled={disabled}
            // models.ts sorts the enum's members by name, so the order is set here.
            options={[
              { value: DNSServer.SYSTEM, label: t('dnsSettings.off') },
              { value: DNSServer.QUAD9, label: 'Quad9' },
              { value: DNSServer.QUAD9_ECS, label: 'Quad9 (ECS)' },
              { value: DNSServer.CLOUDFLARE, label: 'Cloudflare' },
              { value: DNSServer.GOOGLE, label: 'Google' },
              { value: DNSServer.OPENDNS, label: 'OpenDNS' },
              { value: DNSServer.CUSTOM, label: t('dnsSettings.custom') },
            ]}
          />
        </Tooltip>
      </FormGroup>

      {selected === DNSServer.CUSTOM && (
        <FormGroup
          helperText={customURLInvalid && <span id="dnsCustomURLError">{t('dnsSettings.customURLInvalid')}</span>}
          intent={customURLInvalid ? 'danger' : 'none'}
        >
          <Tooltip
            content={stopProxyTooltip}
            disabled={!isProxyRunning}
            placement="top"
            className="settings-manager__dns-tooltip"
          >
            <InputGroup
              aria-label={t('dnsSettings.customURLLabel')}
              aria-invalid={customURLInvalid}
              aria-describedby={customURLInvalid ? 'dnsCustomURLError' : undefined}
              // Disabled, a placeholder looks the same as an entered value, here and on the text area below.
              placeholder={disabled ? undefined : 'https://dns.example/dns-query'}
              value={customURL}
              intent={customURLInvalid ? 'danger' : 'none'}
              onValueChange={(value) => {
                setCustomURL(value);
                saveCustomURL(value);
              }}
              disabled={disabled}
              fill
              type="url"
            />
          </Tooltip>
        </FormGroup>
      )}

      <DNSStatusCallout />

      {selected !== DNSServer.SYSTEM && (
        <FormGroup
          label={t('dnsSettings.systemDomainsLabel')}
          labelFor="dnsSystemDomains"
          helperText={
            <>
              {t('dnsSettings.systemDomainsDescription')}
              <br />
              {t('dnsSettings.systemDomainsHelper')}
            </>
          }
        >
          <Tooltip
            content={stopProxyTooltip}
            disabled={!isProxyRunning}
            placement="top"
            className="settings-manager__dns-tooltip"
          >
            <TextArea
              id="dnsSystemDomains"
              placeholder={disabled ? undefined : 'corp.example'}
              className="settings-manager__dns-system-domains"
              value={systemDomains}
              onChange={(e) => {
                const { value } = e.target;
                setSystemDomains(value);
                saveSystemDomains(value);
              }}
              disabled={disabled}
              autoResize
              fill
            />
          </Tooltip>
        </FormGroup>
      )}
    </>
  );
}

function DNSStatusCallout() {
  const { t } = useTranslation();
  const status = useDNSStatus();

  switch (status?.mode) {
    case 'failing':
      return (
        <Callout intent="warning" className="settings-manager__dns-status" compact>
          {t('dnsSettings.statusFailing')}
          <div className={Classes.TEXT_MUTED}>{status.error}</div>
        </Callout>
      );
    case 'vpn':
      // Not a warning: this and noIPv4 are the feature working as designed, and with some VPNs they last for as long as
      // the VPN runs.
      return (
        <Callout intent="primary" className="settings-manager__dns-status" compact>
          {t('dnsSettings.statusVPN')}
        </Callout>
      );
    case 'noIPv4':
      return (
        <Callout intent="primary" className="settings-manager__dns-status" compact>
          {t('dnsSettings.statusNoIPv4')}
        </Callout>
      );
    default:
      return null;
  }
}
