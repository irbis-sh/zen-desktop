import { Tag } from '@blueprintjs/core';
import { useTranslation } from 'react-i18next';

import './index.css';

import { BlockMissingToggle } from './BlockMissingToggle';
import { CustomResourceDir } from './CustomResourceDir';
import { EnableToggle } from './EnableToggle';
import { ImportExport } from './ImportExport';
import { InjectionCounter } from './InjectionCounter';
import { LibraryList } from './LibraryList';

export function LocalResourcesSection() {
  const { t } = useTranslation();

  return (
    <div className="settings-manager__section--local-resources">
      <Tag size="large" intent="success" fill className="settings-manager__section-header">
        {t('settings.sections.localResources')}
      </Tag>

      <div className="settings-manager__section-body">
        <EnableToggle />
        <BlockMissingToggle />
        <InjectionCounter />
        <LibraryList />
        <CustomResourceDir />
        <ImportExport />
      </div>
    </div>
  );
}
