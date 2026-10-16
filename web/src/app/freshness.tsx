import {useCallback, useEffect, useMemo, useRef, useSyncExternalStore} from 'react';
import {DocumentPool, serviceDocument, type PooledDocument, type QuietService} from '@core/freshness-pool.ts';
import {FreshnessHub, dependsOn, type FreshDocument, type FreshDocumentOptions, type FreshnessDependency, type Reachability} from '@core/freshness-hub.ts';
import type {HttpLocalApi} from '@core/index.ts';
import {useDeviceEvents} from './device-events';
import {useSession} from './session';
import {useViewerScope} from './viewer-scope';

/**
 * Open screens that stay current, quietly (the shared rules are client-core's `freshness-hub`).
 *
 * One hub and one pool of last-known documents per signed-in viewer on one server. They outlive
 * the screens: a document closed by Back is kept, and shown at once when it is opened again.
 * A different viewer or server gets a new hub and pool, and the old ones are let go with
 * everything they held.
 */
type Runtime = {api: HttpLocalApi; scope: string; hub: FreshnessHub; pool: DocumentPool};
let runtime: Runtime | undefined;

export function freshnessFor(api: HttpLocalApi, scope: {viewerId: string; serverId: string}): Runtime {
  const key = scope.serverId + '\n' + scope.viewerId;
  if (runtime && runtime.api === api && runtime.scope === key) return runtime;
  const old = runtime;
  const hub = new FreshnessHub();
  runtime = {api, scope: key, hub, pool: new DocumentPool(hub, {max: 32})};
  // Let go after this render: screens still unmounting release their documents first.
  if (old) setTimeout(() => { old.pool.dispose(); old.hub.dispose(); }, 0);
  return runtime;
}

export function useFreshness(): Runtime {
  const {api} = useSession();
  const scope = useViewerScope();
  return useMemo(() => freshnessFor(api, scope), [api, scope]);
}

const visible = () => typeof document === 'undefined' || document.visibilityState !== 'hidden';

/**
 * Mounted once in the signed-in shell. Feeds the hub what only the page knows: the device's
 * event feed, whether the tab is in front, when the person last touched something, and whether
 * the server's address answers.
 */
export function FreshnessRuntime() {
  const {api} = useSession();
  const {hub} = useFreshness();
  const events = useDeviceEvents();
  useEffect(() => { hub.attach(events); }, [hub, events]);
  useEffect(() => {
    const follow = () => hub.setActive(visible());
    follow();
    document.addEventListener('visibilitychange', follow);
    const touch = () => hub.touch();
    const options = {capture: true, passive: true} as const;
    const kinds = ['keydown', 'pointerdown', 'pointerup', 'wheel', 'touchmove'] as const;
    for (const kind of kinds) window.addEventListener(kind, touch, options);
    return () => {
      document.removeEventListener('visibilitychange', follow);
      for (const kind of kinds) window.removeEventListener(kind, touch, options);
    };
  }, [hub]);
  useEffect(() => {
    const source = {};
    const follow = () => {
      const phase = api.getRouteSnapshot()?.phase;
      // An address that proves another identity is not "can't reach": the shell says what it is.
      hub.report(source, phase !== 'offline' && phase !== 'probing');
    };
    follow();
    const off = api.subscribeRoute(follow);
    return () => { off(); hub.report(source, true); };
  }, [api, hub]);
  return null;
}

/** Whether the server has been out of reach for a while. It clears by itself. */
export function useReachability(): Reachability {
  const {hub} = useFreshness();
  return useSyncExternalStore(hub.subscribeReachability, hub.getReachability);
}

type MakeDocument<S> = () => {service: S; document: FreshDocumentOptions; watch?(document: FreshDocument): () => void};
type Store<T> = {getSnapshot(): T; subscribe(listener: () => void): () => void; dispose?(): void};

/**
 * A document kept in the viewer's pool under `key`: the same service, with what it has loaded,
 * every time the same thing is opened. `make` runs only when the pool has nothing under `key`.
 * The document counts as open while this component is mounted.
 */
export function usePooled<S extends Store<T>, T>(key: string, make: MakeDocument<S>): {service: S; snapshot: T; document: FreshDocument; memo: Record<string, unknown>} {
  const {pool} = useFreshness();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const entry = useMemo(() => pool.peek<S>(key, make), [pool, key]);
  useEffect(() => {
    let held: PooledDocument<S> | undefined = pool.acquire<S>(key, make);
    held.setOpen(true);
    return () => { held?.release(); held = undefined; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pool, key]);
  const snapshot = useSyncExternalStore(entry.service.subscribe, entry.service.getSnapshot);
  return {service: entry.service, snapshot, document: entry.document, memo: entry.memo};
}

/**
 * A failure is not shown at once. While a first load that failed is being tried again by itself
 * (the first couple of attempts, a few seconds), the screen goes on showing that it is loading;
 * after that its own error state shows, with Try again, and the attempts continue behind it.
 */
export function useRecovering(document: FreshDocument): boolean {
  const state = useSyncExternalStore(document.subscribe, document.getState);
  return (state.stale || state.revalidating) && state.failures < 2;
}

/** `usePooled` for a client-core service that reads itself again with `revalidate()`. */
export function usePooledService<S extends QuietService & Store<T>, T>(key: string, create: () => S, dependencies: readonly FreshnessDependency[] | (() => readonly FreshnessDependency[]), options?: {hasContent?(service: S): boolean}) {
  const held = usePooled<S, T>(key, () => serviceDocument(create(), dependencies, options));
  const recovering = useRecovering(held.document);
  const shown = held.snapshot as T & {phase?: string; error?: unknown};
  // An error in its first moments, while the load is being tried again: still loading as far as the person is concerned.
  const quiet = recovering && shown.phase === 'error';
  const snapshot = useMemo(() => (quiet ? {...shown, phase: 'loading', error: null} as T : held.snapshot), [quiet, shown, held.snapshot]);
  return {...held, snapshot};
}

/**
 * Keeps something that is not pooled current while this component is mounted: `revalidate` is
 * called when one of `dependencies` moved, on the hub's usual terms (gathered, paused for input,
 * retried quietly). Returns `markCurrent`, for when the screen has just read it itself.
 */
/**
 * A screen kept current by its server hints (`dependencies`) and, where something on it moves
 * with no hint (a download's progress), by a clock every `everyMs` while the tab is in front;
 * `everyMs: 0` keeps no clock. The hub decides when a read actually runs (the screen open, input
 * paused) and retries a failed read by itself; nothing is read while the tab is hidden, and one
 * read catches up when it is shown again.
 */
export function useQuietRefresh(name: string, revalidate: FreshDocumentOptions['revalidate'], hasContent: () => boolean = () => true, options: {dependencies?: readonly FreshnessDependency[]; everyMs?: number} = {}): () => void {
  const {hub} = useFreshness();
  const clock = dependsOn.clock(name);
  const everyMs = options.everyMs ?? 30000;
  const key = (options.dependencies ?? []).join('\n');
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const dependencies = useMemo(() => [clock, ...(key ? key.split('\n') : [])], [clock, key]);
  useEffect(() => {
    if (everyMs <= 0) return;
    const timer = setInterval(() => { if (hub.isActive()) hub.note(clock); }, everyMs);
    return () => clearInterval(timer);
  }, [hub, clock, everyMs]);
  return useFreshDocument(dependencies, revalidate, hasContent);
}

export function useFreshDocument(dependencies: readonly FreshnessDependency[] | (() => readonly FreshnessDependency[]), revalidate: FreshDocumentOptions['revalidate'], hasContent: () => boolean = () => true): () => void {
  const {hub} = useFreshness();
  // A function is asked again each time a hint arrives (a list's titles in view change as it scrolls).
  const dynamic = typeof dependencies === 'function';
  const key = dynamic ? '\u0000dynamic' : dependencies.join('\n');
  const latest = useRef({revalidate, hasContent, dependencies});
  latest.current = {revalidate, hasContent, dependencies};
  const held = useRef<FreshDocument | undefined>(undefined);
  useEffect(() => {
    const document = hub.document({dependencies: dynamic ? () => (latest.current.dependencies as () => readonly FreshnessDependency[])() : key ? key.split('\n') : [], revalidate: context => latest.current.revalidate(context), hasContent: () => latest.current.hasContent()});
    held.current = document;
    document.setOpen(true);
    return () => { document.dispose(); if (held.current === document) held.current = undefined; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hub, key]);
  return useCallback(() => held.current?.markCurrent(), []);
}
