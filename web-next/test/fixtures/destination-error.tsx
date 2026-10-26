import {useLayoutEffect, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {DestinationErrorBoundary} from '../../src/shell/DestinationError';

/** A destination that throws on its first render, then renders normally (bounded retry). */
let renders = 0;
function ThrowOnceRender() {
  renders += 1;
  if (renders === 1) throw new Error('destination-render-fault');
  return <div data-destination-recovered>recovered</div>;
}

/** A destination that throws from its layout effect on the first commit. */
let effects = 0;
function ThrowOnceEffect() {
  useLayoutEffect(() => {
    effects += 1;
    if (effects === 1) throw new Error('destination-effect-fault');
  }, []);
  return <div data-destination-effect>effect-ok</div>;
}

/** A shell-level component that throws, mounted OUTSIDE the destination boundary. */
function ThrowShell() {
  throw new Error('shell-fault');
}

declare global {
  interface Window { destinationErrorHarness?: {mode: (value: string) => void; routeKey: (value: string) => void; reset: () => void; bump: () => void}; disposeDestinationError?: () => void; disposeShellFailure?: () => void; }
}

/**
 * Real component harness: a shell marker stays mounted around the production boundary, and the
 * destination child throws from render or from a layout effect. `home` is left to the production
 * router port so Home is the real navigation.
 */
export function mountDestinationErrorHarness() {
  const host = document.createElement('div');
  document.body.append(host);
  const root = createRoot(host);
  let setMode: (value: string) => void = () => {}, setRouteKey: (value: string) => void = () => {};
  function Harness() {
    const [mode, set] = useState('render'), [routeKey, setKey] = useState('entry-a'), [cycle, setCycle] = useState(0);
    setMode = set; setRouteKey = setKey;
    window.destinationErrorHarness = {mode: value => set(value), routeKey: value => setKey(value), reset: () => {renders = 0; effects = 0;}, bump: () => setCycle(value => value + 1)};
    return <div data-shell-marker>
      <header data-shell-top>Portico</header>
      <DestinationErrorBoundary routeKey={routeKey}>
        {mode === 'render' ? <ThrowOnceRender key={cycle}/> : mode === 'effect' ? <ThrowOnceEffect key={cycle}/> : <div data-destination-ok>ok</div>}
      </DestinationErrorBoundary>
    </div>;
  }
  root.render(<Harness/>);
  return () => {root.unmount(); host.remove(); window.destinationErrorHarness = undefined;};
}

/** A deliberate shell-level failure: the thrower sits outside the destination boundary. */
export function mountShellFailureHarness() {
  const host = document.createElement('div');
  document.body.append(host);
  const root = createRoot(host);
  root.render(<div data-shell-marker><header data-shell-top>Portico</header><DestinationErrorBoundary routeKey="shell"><div data-destination-ok>ok</div></DestinationErrorBoundary><ThrowShell/></div>);
  return () => {root.unmount(); host.remove();};
}
