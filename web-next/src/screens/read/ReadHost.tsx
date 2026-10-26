import {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {useLocation, useMatch} from '@tanstack/react-router';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {ReadEntry} from './ReadEntry';
import {readSearch, retainedReadTarget, type ReadTarget} from './route-search';
import styles from './entry.module.css';

type Captured = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;
type Location = ReturnType<typeof useLocation>;
type CloseReceipt = {context: Captured['context']; binding: Captured['binding']; target: ReadTarget; location: Location; itemId: string; sourceId?: string; live: boolean; claimed: boolean};

/** The registry owns one admitted host. This module owns its one current book;
 * Reading Settings can cover it without closing the engine or its overrides. */
export function ReadHost() {
  const captured = useProtectedWebScreenContext();
  const match = useMatch({from: '/read/$itemId', shouldThrow: false});
  const location = useLocation({select: value => value});
  const itemId = match?.params.itemId, sourceId = readSearch(match?.search ?? {}).sourceId;
  const [retained, setRetained] = useState<ReadTarget>();
  const committed = useRef<CloseReceipt | null>(null), [published, setPublished] = useState<CloseReceipt>();
  const context = captured?.context, binding = captured?.binding;
  const settings = location.pathname === '/settings';
  useEffect(() => {setRetained(previous => retainedReadTarget(previous, itemId ? {itemId, sourceId} : undefined, settings));}, [itemId, sourceId, settings]);
  const visible = !!match && !!retained && retained.itemId === itemId && retained.sourceId === sourceId;
  useLayoutEffect(() => {
    if (!context || !binding || !retained || !visible || !itemId || readSearch(location.search).sourceId !== sourceId) return;
    const original: CloseReceipt = {context, binding, target: retained, location, itemId, sourceId, live: true, claimed: false};
    committed.current = original; setPublished(original);
    return () => {original.live = false; if (committed.current === original) committed.current = null;};
  }, [context, binding, retained, visible, itemId, sourceId, location]);
  if (!captured || !retained) return null;
  const original = visible && published?.live && published.context === context && published.binding === binding && published.target === retained
    && published.location === location && published.itemId === itemId && published.sourceId === sourceId ? published : undefined;
  const onClose = () => {
    if (!original || !original.live || original.claimed || committed.current !== original || router.state.location !== original.location
      || original.location.pathname !== location.pathname || readSearch(original.location.search).sourceId !== original.sourceId) return;
    if (!original.context.isCurrent()) return;
    if (!original.live || original.claimed || committed.current !== original || router.state.location !== original.location) return;
    original.claimed = true;
    void router.navigate({to: '/item/$itemId', params: {itemId: original.itemId}, replace: true}).catch(error => {
      if (original.context.isCurrent()) original.binding.home.effects.reportError(error);
    });
    setRetained(previous => committed.current === original && original.live && previous === original.target ? undefined : previous);
  };
  return <div className={styles.host} hidden={!visible} inert={!visible} aria-hidden={!visible}>
    <ReadEntry key={JSON.stringify([retained.itemId, retained.sourceId])} captured={captured} itemId={retained.itemId} sourceId={retained.sourceId} active={visible} onClose={onClose}/>
  </div>;
}

/** The real screen is painted once by the admitted retained host. */
export function ReadRouteMarker() {return null;}
