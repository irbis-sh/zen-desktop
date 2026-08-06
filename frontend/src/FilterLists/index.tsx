import { Spinner, SpinnerSize, Switch, Button, MenuItem, PopoverNext, Menu, Tag, Tooltip } from '@blueprintjs/core';
import { Select } from '@blueprintjs/select';
import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { AppToaster } from '@/common/toaster';
import { useProxyState } from '@/context/ProxyStateContext';
import { GetFilterLists, RemoveFilterList, ToggleFilterList } from 'wails/go/config/Config';
import { config } from 'wails/go/models';
import { BrowserOpenURL } from 'wails/runtime/runtime';

import { CreateFilterList } from './CreateFilterList';
import { ExportFilterList } from './ExportFilterList';
import { ImportFilterList } from './ImportFilterList';
import { FilterListType } from './types';

import './index.css';

const NO_DOOMSCROLL_URL = 'https://github.com/irbis-sh/filter-lists/blob/master/no-doomscroll/readme.md';

export function FilterLists() {
  const { t } = useTranslation();
  const [state, setState] = useState<{
    filterLists: config.FilterList[];
    loading: boolean;
  }>({
    filterLists: [],
    loading: true,
  });

  const fetchLists = async () => {
    const filterLists = await GetFilterLists();
    setState({ ...state, filterLists, loading: false });
  };

  useEffect(() => {
    (() => {
      fetchLists();
    })();
  }, []);

  const [type, setType] = useState<FilterListType>(FilterListType.GENERAL);

  return (
    <>
      <div className="filter-lists__header">
        <Select
          items={Object.values(FilterListType)}
          itemRenderer={(item) => (
            <MenuItem
              key={item}
              text={
                <>
                  {t(`filterTypes.${item}`)}
                  <span className="bp6-text-muted filter-lists__select-count">
                    ({state.filterLists.filter((filterList) => filterList.type === item && filterList.enabled).length}/
                    {state.filterLists.filter((filterList) => filterList.type === item).length})
                  </span>
                </>
              }
              onClick={() => {
                setType(item);
              }}
              active={item === type}
            />
          )}
          onItemSelect={(item) => {
            setType(item);
          }}
          popoverProps={{ minimal: true }}
          filterable={false}
        >
          <Button text={t(`filterTypes.${type}`)} endIcon="caret-down" />
        </Select>

        {type === FilterListType.DIGITAL_WELLBEING && (
          <Button
            icon="help"
            onClick={() => {
              BrowserOpenURL(NO_DOOMSCROLL_URL);
            }}
          >
            {t('filterLists.whatIsThis')}
          </Button>
        )}

        {type === FilterListType.CUSTOM && (
          <PopoverNext
            content={
              <Menu>
                <ExportFilterList />
                <ImportFilterList onAdd={fetchLists} />
              </Menu>
            }
          >
            <Button icon="more" text={t('filterLists.more')} />
          </PopoverNext>
        )}
      </div>

      {state.loading && <Spinner size={SpinnerSize.SMALL} className="filter-lists__spinner" />}

      {state.filterLists
        .filter((filterList) => filterList.type === type)
        .map((filterList) => (
          <FilterListItem
            key={filterList.url}
            filterList={filterList}
            showDelete={type === FilterListType.CUSTOM}
            onRemoved={fetchLists}
            onToggled={fetchLists}
          />
        ))}

      {type === FilterListType.CUSTOM && <CreateFilterList onAdd={fetchLists} />}
    </>
  );
}

export function FilterListItem({
  filterList,
  showDelete,
  showButtons = true,
  onRemoved,
  onToggled,
}: {
  filterList: config.FilterList;
  showDelete?: boolean;
  showButtons?: boolean;
  onRemoved?: () => void;
  onToggled?: () => void;
}) {
  const { t } = useTranslation();
  const { isProxyRunning } = useProxyState();
  const [switchLoading, setSwitchLoading] = useState(false);
  const [switchChecked, setSwitchChecked] = useState(filterList.enabled);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  return (
    <div className="filter-lists__list">
      <div className="filter-lists__list-header">
        <h3 className="filter-lists__list-name">{filterList.name}</h3>
        <Tooltip content={t('common.stopProxyToToggleFilter') as string} disabled={!isProxyRunning} placement="left">
          <Switch
            checked={switchChecked}
            disabled={switchLoading || isProxyRunning}
            onChange={async (e) => {
              setSwitchLoading(true);
              const initial = switchChecked;
              const { checked } = e.currentTarget;
              setSwitchChecked(checked);
              try {
                await ToggleFilterList(filterList.url, checked);
              } catch (err) {
                setSwitchChecked(initial);
                AppToaster.show({
                  message: t('filterLists.toggleError', { error: String(err) }),
                  intent: 'danger',
                });
              }
              setSwitchLoading(false);
              onToggled?.();
            }}
            size="large"
            className="filter-lists__list-switch"
          />
        </Tooltip>
      </div>
      {filterList.trusted ? (
        <Tag intent="success" className="filter-lists__list-trusted">
          {t('filterLists.trusted')}
        </Tag>
      ) : null}

      <div className="bp6-text-muted filter-lists__list-url">{filterList.url}</div>
      {showButtons && (
        <div className="filter-lists__list-buttons">
          <Tooltip
            content={t('filterLists.copied') as string}
            isOpen={copied}
            hoverOpenDelay={0}
            hoverCloseDelay={0}
            position="top"
            className="filter-lists__list-button"
          >
            <Button
              icon="duplicate"
              intent="none"
              className="filter-lists__list-button"
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(filterList.url);
                  setCopied(true);
                  setTimeout(() => setCopied(false), 1500);
                } catch (err) {
                  console.error('Copying error', err);
                }
              }}
            >
              {t('filterLists.copy')}
            </Button>
          </Tooltip>

          <Button
            icon="globe-network"
            intent="none"
            className="filter-lists__list-button"
            onClick={() => {
              BrowserOpenURL(filterList.url);
            }}
          >
            {t('filterLists.goTo')}
          </Button>
        </div>
      )}
      {showDelete && (
        <Tooltip content={t('common.stopProxyToDeleteFilter') as string} disabled={!isProxyRunning} placement="right">
          <Button
            icon="trash"
            intent="danger"
            size="small"
            className="filter-lists__list-delete"
            disabled={isProxyRunning}
            loading={deleteLoading}
            onClick={async () => {
              setDeleteLoading(true);
              try {
                await RemoveFilterList(filterList.url);
              } catch (err) {
                AppToaster.show({
                  message: t('filterLists.removeError', { error: String(err) }),
                  intent: 'danger',
                });
              }
              setDeleteLoading(false);
              onRemoved?.();
            }}
          >
            {t('filterLists.delete')}
          </Button>
        </Tooltip>
      )}
    </div>
  );
}
