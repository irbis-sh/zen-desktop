import { t } from 'i18next';
import { Component, ErrorInfo, ReactNode } from 'react';

import { AppToaster } from './common/toaster';

interface Props {
  children: ReactNode;
}

class ErrorBoundary extends Component<Props> {
  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    AppToaster.show({
      message: t('errorBoundary.errorMessage', { error }),
      intent: 'danger',
    });
    console.error('ErrorBoundary', error, errorInfo);
  }

  render() {
    return this.props.children;
  }
}

export default ErrorBoundary;
