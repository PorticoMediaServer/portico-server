import { lazy, StrictMode, Suspense, useEffect } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { DataProvider, useAuthSession } from './data/DataProvider';
import { useRuntime } from './runtime/RuntimeContext';
import { RuntimeProvider } from './runtime/RuntimeProvider';
import { RuntimeSurface } from './runtime/RuntimeSurface';
import { RuntimeProductFrame } from './runtime/RuntimeProductFrame';
import { runtimeUsesProductFrame } from './runtime/runtimeFramePolicy';
import { markWebTiming, measureWebTiming } from './runtime/performance';
import { registerHostedServiceWorker } from './runtime/hostedServiceWorker';
import { UnexpectedErrorBoundary } from './runtime/UnexpectedErrorBoundary';
import './styles/tokens.css';
import './styles/base.css';
import './styles/auth.css';
import './styles/runtime.css';
import './styles/app.css';
import './styles/quality.css';

const App = lazy(() => import('./App').then((module) => ({ default: module.App })));

function ConnectedRuntimeEntry() {
  const auth = useAuthSession();
  if (auth.status === 'loading') return <div className="runtime-content-reservation" aria-busy="true" />;
  return <Suspense fallback={<div className="runtime-content-reservation" aria-busy="true" />}><App /></Suspense>;
}

function RuntimeEntry() {
  const runtime = useRuntime();
  useEffect(() => {
    markWebTiming('first-frame');
  }, []);
  useEffect(() => {
    markWebTiming(`runtime:${runtime.state.id}`);
    if (runtime.state.id === 'server-ready') {
      markWebTiming('server-ready');
      measureWebTiming('runtime-to-server-ready', 'first-frame', 'server-ready');
    }
  }, [runtime.state.id]);

  if (runtime.state.id !== 'server-ready' || !runtime.source) {
    const surface = <RuntimeSurface embedded={runtimeUsesProductFrame(runtime.state)} />;
    return runtimeUsesProductFrame(runtime.state) ? <RuntimeProductFrame>{surface}</RuntimeProductFrame> : surface;
  }
	return <RuntimeProductFrame connected><DataProvider source={runtime.source} initialViewer={runtime.initialViewer} expectedViewerScope={runtime.expectedViewerScope} browserAccountsEnabled={false} localSessionQuarantineEnabled={runtime.config.mode === 'bundled'} viewerRuntime={runtime.viewerRuntime}>
      <ConnectedRuntimeEntry />
    </DataProvider></RuntimeProductFrame>;
}

const fixtureSourceLoader = import.meta.env.DEV
  ? async () => {
      const { FixturePorticoDataSource } = await import('./data/fixtureSource');
      return new FixturePorticoDataSource();
    }
  : undefined;

const rootHost = globalThis as typeof globalThis & { __PORTICO_WEB_ROOT__?: Root };
const reactRoot = rootHost.__PORTICO_WEB_ROOT__ ?? createRoot(document.getElementById('root')!);
rootHost.__PORTICO_WEB_ROOT__ = reactRoot;
const router = createBrowserRouter([{ path: '*', element: <RuntimeEntry /> }]);

if (import.meta.env.PROD) {
  void registerHostedServiceWorker(window.__PORTICO_CONFIG__?.mode);
}

reactRoot.render(
  <StrictMode>
    <UnexpectedErrorBoundary>
      <RuntimeProvider fixtureSourceLoader={fixtureSourceLoader}>
        <RouterProvider router={router} />
      </RuntimeProvider>
    </UnexpectedErrorBoundary>
  </StrictMode>,
);
