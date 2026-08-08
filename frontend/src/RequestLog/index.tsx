import { CardList, Card, Tag, Collapse, Intent, CompoundTag, HTMLTable } from '@blueprintjs/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { getCurrentLocale } from '@/i18n';
import { EventsOn } from 'wails/runtime';
import './index.css';

interface Rule {
  filterName: string;
  rawRule: string;
}

interface Process {
  id: number;
  name: string;
  diskPath: string;
}

enum FilterActionKind {
  Block = 'block',
  Redirect = 'redirect',
  Modify = 'modify',
  Local = 'local',
}

interface FilterAction {
  id: string;
  kind: FilterActionKind;
  method: string;
  url: string;
  to: string;
  referer: string;
  rules: Rule[];
  process: Process;
  resource?: string;
  blocked?: boolean;
  requestedVersion?: string;
  servedVersion?: string;
  versionDelta?: string;
  createdAt: Date;
}

export function RequestLog() {
  const { t } = useTranslation();
  const [logs, setLogs] = useState<FilterAction[]>([]);

  useEffect(() => {
    const cancel = EventsOn('filter:action', (action: Omit<FilterAction, 'id' | 'createdAt'>) => {
      setLogs((logs) =>
        [
          {
            ...action,
            id: id(),
            createdAt: new Date(),
          },
          ...logs,
        ].slice(0, 200),
      );
    });

    return () => {
      cancel();
    };
  }, []);

  return (
    <div className="request-log">
      {logs.length === 0 ? (
        <p className="request-log__empty">{t('requestLog.emptyState')}</p>
      ) : (
        <CardList compact>
          {logs.map((log) => (
            <RequestLogCard log={log} key={log.id} />
          ))}
        </CardList>
      )}
    </div>
  );
}

function RequestLogCard({ log }: { log: FilterAction }) {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);

  const { hostname } = new URL(log.url, 'http://foo'); // Setting the base url somehow helps with parsing //hostname:port URLs

  let tagIntent: Intent;
  switch (log.kind) {
    case FilterActionKind.Block:
      tagIntent = Intent.DANGER;
      break;
    case FilterActionKind.Local:
      tagIntent = log.blocked ? Intent.DANGER : Intent.SUCCESS;
      break;
    case FilterActionKind.Modify:
      tagIntent = Intent.WARNING;
      break;
    case FilterActionKind.Redirect:
      tagIntent = Intent.WARNING;
      break;
    default:
      tagIntent = Intent.NONE;
  }

  const hasProcessId = log.process.id > 0;
  const hasProcessPath = Boolean(log.process.diskPath);

  return (
    <>
      <Card key={log.id} className="request-log__card" interactive onClick={() => setIsOpen(!isOpen)}>
        <div className="request-log__card__summary">
          {log.kind === FilterActionKind.Local && log.versionDelta && (
            <span
              className="request-log__version-delta"
              title={
                log.versionDelta === 'upgrade' ? t('requestLog.versionUpgraded') : t('requestLog.versionDowngraded')
              }
              aria-label={
                log.versionDelta === 'upgrade' ? t('requestLog.versionUpgraded') : t('requestLog.versionDowngraded')
              }
            >
              {log.versionDelta === 'upgrade' ? '↑' : '↓'}
            </span>
          )}
          <Tag minimal intent={tagIntent}>
            {hostname}
          </Tag>
          {log.kind === FilterActionKind.Local && (
            <Tag minimal intent={tagIntent}>
              {log.blocked ? t('requestLog.localBlocked') : t('requestLog.local')}
              {log.resource ? ` · ${log.resource}` : ''}
            </Tag>
          )}
          <Tag minimal className="request-log__card__process-tag" title={log.process.name}>
            {log.process.name}
          </Tag>
        </div>
        <div className="bp6-text-muted">
          {log.createdAt.toLocaleTimeString(getCurrentLocale(), { timeStyle: 'short' })}
        </div>
      </Card>

      <Collapse isOpen={isOpen}>
        <Card className="request-log__card__details" compact>
          <div className="request-log__card__details__section">
            <div className="request-log__card__details__section-header">
              <div className="request-log__card__details__section-title">{t('requestLog.request')}</div>
            </div>
            <div className="request-log__card__details__group">
              <div className="request-log__card__details__field request-log__card__details__field--tag">
                <div className="request-log__card__details__value">
                  <CompoundTag leftContent={t('requestLog.method')} minimal>
                    {log.method}
                  </CompoundTag>
                </div>
              </div>
              <div className="request-log__card__details__field request-log__card__details__field--text">
                <div className="request-log__card__details__label">{t('requestLog.url')}:</div>
                <div className="request-log__card__details__value">{log.url}</div>
              </div>
              {log.kind === FilterActionKind.Redirect && (
                <div className="request-log__card__details__field request-log__card__details__field--text">
                  <div className="request-log__card__details__label">{t('requestLog.redirectedTo')}:</div>
                  <div className="request-log__card__details__value">{log.to}</div>
                </div>
              )}
              {log.referer && (
                <div className="request-log__card__details__field request-log__card__details__field--text">
                  <div className="request-log__card__details__label">{t('requestLog.referer')}:</div>
                  <div className="request-log__card__details__value">{log.referer}</div>
                </div>
              )}
              {log.kind === FilterActionKind.Local && log.requestedVersion && log.servedVersion && (
                <div className="request-log__card__details__field request-log__card__details__field--text">
                  <div className="request-log__card__details__label">{t('requestLog.version')}:</div>
                  <div className="request-log__card__details__value">
                    {log.requestedVersion} → {log.servedVersion}
                  </div>
                </div>
              )}
            </div>
          </div>
          <div className="request-log__card__details__section">
            <div className="request-log__card__details__section-header">
              <div className="request-log__card__details__section-title">{t('requestLog.process')}</div>
            </div>
            <div className="request-log__card__details__group">
              <div className="request-log__card__details__field request-log__card__details__field--tag request-log__card__details__field--process-summary">
                <div className="request-log__card__details__value">
                  {hasProcessId && (
                    <CompoundTag leftContent="PID" minimal>
                      {log.process.id}
                    </CompoundTag>
                  )}
                  <CompoundTag
                    leftContent={t('requestLog.processName')}
                    minimal
                    className="request-log__card__details__process-name-tag"
                    title={log.process.name}
                  >
                    {log.process.name}
                  </CompoundTag>
                </div>
              </div>
              {hasProcessPath && (
                <div className="request-log__card__details__field request-log__card__details__field--text">
                  <div className="request-log__card__details__label">{t('requestLog.processPath')}:</div>
                  <div className="request-log__card__details__value">{log.process.diskPath}</div>
                </div>
              )}
            </div>
          </div>
          <div className="request-log__card__details__section">
            <div className="request-log__card__details__section-header">
              <div className="request-log__card__details__section-title">{t('requestLog.rules')}</div>
              <Tag minimal>{log.rules.length}</Tag>
            </div>
            <HTMLTable bordered compact striped className="request-log__card__details__rules">
              <thead>
                <tr>
                  <th>{t('requestLog.filterName')}</th>
                  <th>{t('requestLog.rule')}</th>
                </tr>
              </thead>
              <tbody>
                {log.rules.map((rule) => (
                  <tr key={rule.rawRule}>
                    <td>{rule.filterName}</td>
                    <td>{rule.rawRule}</td>
                  </tr>
                ))}
              </tbody>
            </HTMLTable>
          </div>
        </Card>
      </Collapse>
    </>
  );
}

function id(): string {
  return Math.random().toString(36).slice(2, 9);
}
