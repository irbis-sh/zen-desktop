import { Button, FormGroup } from '@blueprintjs/core';
import { useTranslation } from 'react-i18next';

import { AppToaster } from '@/common/toaster';
import { ExportLocalResourcesMappings, ImportLocalResourcesMappings } from 'wails/go/app/App';

export function ImportExport() {
  const { t } = useTranslation();

  async function exportMappings() {
    try {
      await ExportLocalResourcesMappings();
    } catch (err) {
      AppToaster.show({
        message: t('localResources.importExport.exportError', { error: err }),
        intent: 'danger',
      });
      return;
    }
    AppToaster.show({
      message: t('localResources.importExport.exportSuccess'),
      intent: 'success',
    });
  }

  async function importMappings() {
    try {
      await ImportLocalResourcesMappings();
    } catch (err) {
      AppToaster.show({
        message: t('localResources.importExport.importError', { error: err }),
        intent: 'danger',
      });
      return;
    }
    AppToaster.show({
      message: t('localResources.importExport.importSuccess'),
      intent: 'success',
    });
  }

  return (
    <FormGroup label={t('localResources.importExport.label')}>
      <div className="local-resources__import-export">
        <Button icon="import" onClick={importMappings}>
          {t('localResources.importExport.import')}
        </Button>
        <Button icon="export" onClick={exportMappings}>
          {t('localResources.importExport.export')}
        </Button>
      </div>
    </FormGroup>
  );
}
