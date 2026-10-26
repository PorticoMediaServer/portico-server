import {Component, useLayoutEffect, useState, type ReactNode} from 'react';
import {createRoot} from 'react-dom/client';
import {DestinationErrorBoundary} from '../../src/shell/DestinationError';

/** Deterministic destination fault: throws until the harness explicitly releases it. */
function ThrowWhileFaulted({fault, where}: {fault: boolean; where: 'render' | 'effect'}) {
  useLayoutEffect(() => {
    if (fault && where === 'effect') throw new Error('destination-effect-fault');
  }, [fault, where]);
  if (fault && where === 'render') throw new Error('destination-render-fault');
  return <div data-destination-recovered>{where}</div>;
}

/**
 * A generic outer boundary used ONLY to prove nesting: the destination boundary does not catch a
 * sibling that throws above it. It is not the production app last resort, and this fixture makes no
 * claim about ShellRoot/router last-resort or registration/validation behavior — those remain owner
 * UNIMPLEMENTED/UNRUN until an actual router/shell seam is exercised.
 */
class OuterNestingStandIn extends Component<{children: ReactNode}, {caught: boolean}> {
  state = {caught: false};
  static getDerivedStateFromError() { return {caught: true}; }
  render() { return this.state.caught ? <div data-outer-nesting>caught above destination</div> : this.props.children; }
}

/** A shell-level thrower, mounted outside the destination boundary. */
function ThrowShell() { throw new Error('shell-fault'); }

declare global {
  interface Window { destinationErrorHarness?: {
    where: (value: 'render' | 'effect') => void;
    release: () => void;
    rearm: () => void;
    entry: (value: string) => void;
    scope: (value: string) => void;
  }; disposeDestinationError?: () => void; disposeShellSeam?: () => void; }
}

/**
 * Real component harness: a shell marker stays mounted around the production boundary, the
 * destination fault stays deterministic until `release()`, and the recovery lifetime is built the
 * same way Root builds it — href + history entry + captured scope.
 */
export function mountDestinationErrorHarness() {
  const host = document.createElement('div');
  document.body.append(host);
  const root = createRoot(host);
  function Harness() {
    const [where, setWhere] = useState<'render' | 'effect'>('render');
    const [fault, setFault] = useState(true);
    const [entry, setEntry] = useState('entry-a'), [scope, setScope] = useState('scope-1');
    const lifetime = JSON.stringify(['/library/demo', entry, scope]);
    window.destinationErrorHarness = {
      where: value => setWhere(value), release: () => setFault(false), rearm: () => setFault(true),
      entry: value => setEntry(value), scope: value => setScope(value),
    };
    return <div data-shell-marker>
      <header data-shell-top>Portico</header>
      <DestinationErrorBoundary lifetime={lifetime}>
        <ThrowWhileFaulted fault={fault} where={where}/>
      </DestinationErrorBoundary>
    </div>;
  }
  root.render(<Harness/>);
  return () => {root.unmount(); host.remove(); window.destinationErrorHarness = undefined;};
}

/**
 * The nesting seam: a shell marker and the destination boundary are siblings, and a thrower sits
 * outside the boundary under a generic outer boundary. This proves only that the destination
 * boundary is below its siblings; it is not the production ShellRoot/router last resort.
 */
export function mountShellFailureHarness() {
  const host = document.createElement('div');
  document.body.append(host);
  const root = createRoot(host);
  root.render(<OuterNestingStandIn>
    <div data-shell-marker><header data-shell-top>Portico</header>
      <DestinationErrorBoundary lifetime="shell"><div data-destination-ok>ok</div></DestinationErrorBoundary>
      <ThrowShell/>
    </div>
  </OuterNestingStandIn>);
  return () => {root.unmount(); host.remove();};
}
