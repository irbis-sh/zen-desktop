import { Button, Icon } from '@blueprintjs/core';
import { useTranslation } from 'react-i18next';

import { BrowserOpenURL } from 'wails/runtime/runtime';

import './index.css';

const LINK = 'https://opencollective.com/irbis-sh';

export function DonateButton() {
  const { t } = useTranslation();

  return (
    <Button
      icon={<Icon icon="heart" className="donate-button__icon" />}
      variant="outlined"
      onClick={() => BrowserOpenURL(LINK)}
    >
      {t('donate')}
    </Button>
  );
}
