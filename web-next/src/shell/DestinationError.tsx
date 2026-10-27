import {Component, useCallback, useLayoutEffect, useMemo, useRef, useState, type ErrorInfo, type ReactNode} from 'react';
import {useRouter, useRouterState, type ErrorComponentProps} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {StateLayout} from '../kit/states';
import {useWebScreenContext} from '../route-registry-view';
import {router} from '../router';

/**
 * The destination recovery witness: the actual Router location object, its href, the actual history
 * entry key when the router supplies one, and the captured owner scope. Identity is the recovery
 * lifetime — a new location object (including a same-href fresh entry), a real entry change or a
 * scope replacement creates a new witness; a missing entry key never collapses the href.
 */
export type DestinationRecoveryWitness = Readonly<{
  location: unknown;
  href: string;
  entry?: string;
  scopeKey?: string;
}>;

const entryOf = (location: {state: {__TSR_key?: unknown}}): string | undefined => {
  const entry = location.state.__TSR_key;
  return typeof entry === 'string' && entry.length ? entry : undefined;
};

/** Immutable render facts for one error page; nothing here is authority. */
type Facts = Readonly<{
  location: {state: {__TSR_key?: unknown}; href: string};
  href: string;
  entry?: string;
  scopeKey?: string;
  error: unknown;
  matchId?: string;
  routeId?: string;
  /** The original shell currentness predicate for this render, or undefined when there is none. */
  currentness?: () => boolean;
  /** The boundary reset; the installed client CatchBoundary supplies it, SSR's direct path omits it. */
  reset?: () => void;
}>;

/** A layout-owned receipt: created fresh in layout setup, dead on cleanup. */
type Receipt = Readonly<{facts: Facts; live: {current: boolean}; pending: {current: boolean}}>;

/** The approved page failure. Both actions are receipt-bound and refuse once their receipt is dead. */
function PageFailure({onRetry, onHome}: {onRetry: () => void; onHome: () => void}) {
  const t = defaultI18n.t;
  return <StateLayout title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} icon="i-warn" tone="warning"
    primary={{id: 'retry', label: t('next.states.tryAgain'), icon: 'i-retry', onPress: onRetry}}
    secondary={{id: 'home', label: t('next.states.goHome'), onPress: onHome}} />;
}

/**
 * One child destination's error page. Render produces only immutable facts; a layout effect creates
 * and publishes a fresh receipt for those facts (and a StrictMode setup replay gets its own), while
 * cleanup kills the exact receipt and clears its pending token. Handlers act only through the
 * published receipt: they re-check its liveness, the router's actual current location object, href
 * and entry key, and the original WebScreenContext currentness predicate before acting, then repeat
 * the same checks after that predicate and again after a deferred invalidation. A stored child
 * validation/load error is reloaded only through that receipt's own failed child match, then the
 * boundary reset is called when one was supplied: the installed client CatchBoundary always passes it,
 * and only SSR's direct-error path omits it, so the port stays optional.
 */
export function ChildRouteError({error, reset}: ErrorComponentProps) {
  const current = useRouter();
  const location = useRouterState({select: state => state.location});
  const matches = useRouterState({select: state => state.matches});
  const owner = useShellBindingsScope();
  const screen = useWebScreenContext();
  const currentness = screen.context?.isCurrent;
  const resetPort: (() => void) | undefined = typeof reset === 'function' ? reset : undefined;
  const failed = matches.filter(match => match.status === 'error' && match.error === error).at(-1);
  const facts = useMemo<Facts>(() => Object.freeze({
    location, href: location.href, entry: entryOf(location), scopeKey: owner, error,
    matchId: failed?.id, routeId: failed?.routeId, currentness, reset: resetPort,
  }), [location, owner, error, failed?.id, failed?.routeId, currentness, resetPort]);
  const [published, setPublished] = useState<Receipt | undefined>(undefined);
  const live = useRef<Receipt | undefined>(undefined);
  useLayoutEffect(() => {
    const receipt: Receipt = {facts, live: {current: true}, pending: {current: false}};
    live.current = receipt;
    setPublished(receipt);
    return () => {
      // Dead and detached before its pending token is cleared: an old completion cannot publish.
      receipt.live.current = false;
      if (live.current === receipt) live.current = undefined;
      receipt.pending.current = false;
    };
  }, [facts]);
  const held = published?.facts === facts ? published : undefined;
  const holds = useCallback((receipt: Receipt, afterInvalidate = false) => {
    if (!receipt.live.current || live.current !== receipt) return false;
    const now = current.state.location;
    if (now !== receipt.facts.location || now.href !== receipt.facts.href || entryOf(now) !== receipt.facts.entry) return false;
    // The captured failed child must still be the same child with the same original error. A render
    // or effect fault had no stored match error (the router match was ready), so only the location,
    // context and liveness checks apply to it.
    if (receipt.facts.matchId !== undefined) {
      const match = current.state.matches.find(candidate => candidate.id === receipt.facts.matchId);
      if (!match || match.routeId !== receipt.facts.routeId) return false;
      if (match.status === 'error' && match.error !== receipt.facts.error) return false;
      // After this receipt's own invalidation only that same match may recover; a successor fault
      // on a different child is refused.
      if (afterInvalidate && match.status === 'error') return false;
    }
    if (receipt.facts.currentness && !receipt.facts.currentness()) return false;
    // Repeat the exact receipt and child checks after the currentness predicate.
    if (!receipt.live.current || live.current !== receipt) return false;
    const again = current.state.location;
    if (again !== receipt.facts.location || again.href !== receipt.facts.href || entryOf(again) !== receipt.facts.entry) return false;
    if (receipt.facts.matchId !== undefined) {
      const match = current.state.matches.find(candidate => candidate.id === receipt.facts.matchId);
      if (!match || match.routeId !== receipt.facts.routeId) return false;
      if (match.status === 'error' && match.error !== receipt.facts.error) return false;
    }
    return true;
  }, [current]);
  const retry = useCallback(() => {
    if (!held || !holds(held) || held.pending.current) return;
    if (held.facts.matchId === undefined) { held.facts.reset?.(); return; }
    held.pending.current = true;
    void current.invalidate({filter: match => match.id === held.facts.matchId, sync: true}).then(() => {
      if (!holds(held, true)) return;
      held.facts.reset?.();
    }).catch(() => { /* The child stays on its bounded page. */ }).finally(() => { held.pending.current = false; });
  }, [held, holds, current]);
  const home = useCallback(() => {
    if (!held || !holds(held)) return;
    void current.navigate({to: '/'}).catch(() => { /* The shell reports navigation failure. */ });
  }, [held, holds, current]);
  return <PageFailure onRetry={retry} onHome={home} />;
}

/** The captured owner scope of the current shell binding, or undefined when signed out. */
function useShellBindingsScope(): string | undefined {
  const screen = useWebScreenContext();
  return screen.binding?.scopeKey;
}

/**
 * The root route's error component: the app itself failed, so the actual error is rethrown rather
 * than drawn as a page failure. With `disableGlobalCatchBoundary` the throw leaves the router and
 * reaches the existing App last resort; its text is never logged or displayed here.
 */
export function RootErrorRethrow({error}: ErrorComponentProps): ReactNode {
  throw error;
}

export type DestinationErrorProps = Readonly<{
  witness: DestinationRecoveryWitness;
  children: ReactNode;
  /** Owner/test port for Home; the real shell passes its own navigation. */
  onHome?: () => void;
}>;

type DestinationErrorState = Readonly<{failed: boolean; failure?: {current: boolean}}>;

/**
 * The outlet backstop for a failure in the route content the boundary wraps (a child destination
 * has its own error component and catches first). Each failed render captures that render's own
 * witness, failure token and Home port in its closures; a saved old callback compares its captured
 * witness and failure before acting, so a successor fault cannot be reset or navigated by it.
 */
export class DestinationErrorBoundary extends Component<DestinationErrorProps, DestinationErrorState> {
  state: DestinationErrorState = {failed: false};
  static getDerivedStateFromError(): DestinationErrorState { return {failed: true, failure: {current: true}}; }
  componentDidCatch(_error: Error, _info: ErrorInfo) { /* The presenter is the report; nothing else is replaced. */ }
  componentDidUpdate(previous: DestinationErrorProps) {
    // A new witness is a new recovery lifetime: a same-href fresh entry, an actual entry change or
    // a scope replacement must not leave the failed destination latched.
    if (previous.witness !== this.props.witness && this.state.failed) this.setState({failed: false});
  }
  render() {
    if (!this.state.failed) return this.props.children;
    const witness = this.props.witness, failure = this.state.failure, onHome = this.props.onHome;
    const current = () => this.state.failed && this.props.witness === witness && this.state.failure === failure && failure?.current === true;
    return <PageFailure
      onRetry={() => { if (!current()) return; if (failure) failure.current = false; this.setState({failed: false}); }}
      onHome={() => {
        if (!current()) return;
        if (onHome) { onHome(); return; }
        void router.navigate({to: '/'}).catch(() => { /* The existing shell reports navigation failure. */ });
      }} />;
  }
}
