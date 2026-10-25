import React, { Suspense } from 'react';
import { ConfigProvider, Spin, theme } from 'antd';
import ReactDOM from 'react-dom/client';
import { ErrorBoundary } from 'react-error-boundary';
import { HelmetProvider } from 'react-helmet-async';
import { RouterProvider } from 'react-router-dom';

import { MainError } from './components/main-error.tsx';
import { useResolvedTheme } from './lib/theme.ts';
import { router } from './router';

import './i18n';
import './assets/styles/index.css';

const darkSlider = {
  railBg: 'rgba(255, 255, 255, 0.28)',
  railHoverBg: 'rgba(255, 255, 255, 0.4)',
  trackBg: '#4096ff',
  trackHoverBg: '#69b1ff',
  handleColor: '#91caff',
  handleActiveColor: '#e6f4ff',
  dotBorderColor: 'rgba(255, 255, 255, 0.4)'
};

const lightSlider = {
  railBg: 'rgba(0, 0, 0, 0.18)',
  railHoverBg: 'rgba(0, 0, 0, 0.28)',
  trackBg: '#1677ff',
  trackHoverBg: '#4096ff',
  handleColor: '#1677ff',
  handleActiveColor: '#0958d9',
  dotBorderColor: 'rgba(0, 0, 0, 0.28)'
};

const ThemedApp = () => {
  const resolved = useResolvedTheme();
  const isDark = resolved === 'dark';

  const themeConfig = {
    algorithm: isDark ? theme.darkAlgorithm : theme.defaultAlgorithm,
    components: {
      Slider: isDark ? darkSlider : lightSlider,
      Collapse: {
        headerPadding: 0,
        contentPadding: 0
      }
    }
  };

  return (
    <ConfigProvider theme={themeConfig}>
      <RouterProvider router={router} />
    </ConfigProvider>
  );
};

const renderApp = () => {
  return ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <Suspense
        fallback={
          <div className="flex h-screen w-screen items-center justify-center">
            <Spin size="large" />
          </div>
        }
      >
        <ErrorBoundary FallbackComponent={MainError}>
          <HelmetProvider>
            <ThemedApp />
          </HelmetProvider>
        </ErrorBoundary>
      </Suspense>
    </React.StrictMode>
  );
};

if (import.meta.env.MODE === 'mocked') {
  const { worker } = await import('./mocks/browser');
  worker.start().then(() => {
    return renderApp();
  });
}

renderApp();
