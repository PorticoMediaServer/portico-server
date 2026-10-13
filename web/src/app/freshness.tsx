import {useCallback, useEffect, useMemo, useRef, useSyncExternalStore} from 'react';
import {DocumentPool, serviceDocument, type PooledDocument, type QuietService} from '@core/freshness-pool.ts';
import {FreshnessHub, type FreshDocument, type FreshDocumentOptions, type FreshnessDependency, type Reachability} from '@core/freshness-hub.ts';
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

type Made<S> = {service: S; document: FreshDocumentOptions; watch?(document: FreshDocument): () => void};
type Store<T> = {getSnapshot(): T; subscribe(listener: () => void): () => void; dispose?(): void};

/**
 * A document kept in the viewer's pool under `key`: the same service, with what it has loaded,
 * every time the same thing is opened. `make` runs only when the pool has nothing under `key`.
 * The document counts as open while this component is mounted.
 */
export function usePooled<S extends Store<T>, T>(key: string, make: () => Made<S>): {service: S; snapshot: T; document: FreshDocument; memo: Record<string, unknown>} {
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

/** `usePooled` for a client-core service that reads itself again with `revalidate()`. */
export function usePooledService<S extends QuietService & Store<T>, T>(key: string, create: () => S, dependencies: readonly FreshnessDependency[] | (() => readonly FreshnessDependency[]), options?: {hasContent?(service: S): boolean}) {
  return usePooled<S, T>(key, () => serviceDocument(create(), dependencies, options));
}

/**
 * Keeps something that is not pooled current while this component is mounted: `revalidate` is
 * called when one of `dependencies` moved, on the hub's usual terms (gathered, paused for input,
 * retried quietly). Returns `markCurrent`, for when the screen has just read it itself.
 */
export function useFreshDocument(dependencies: readonly FreshnessDependency[], revalidate: FreshDocumentOptions['revalidate'], hasContent: () => boolean = () => true): () => void {
  const {hub} = useFreshness();
  const key = dependencies.join('\n');
  const latest = useRef({revalidate, hasContent});
  latest.current = {revalidate, hasContent};
  const held = useRef<FreshDocument | undefined>(undefined);
  useEffect(() => {
    const document = hub.document({dependencies: key ? key.split('\n') : [], revalidate: context => latest.current.revalidate(context), hasContent: () => latest.current.hasContent()});
    held.current = document;
    document.setOpen(true);
    return () => { document.dispose(); if (held.current === document) held.current = undefined; };
  }, [hub, key]);
  return useCallback(() => held.current?.markCurrent(), []);
}
