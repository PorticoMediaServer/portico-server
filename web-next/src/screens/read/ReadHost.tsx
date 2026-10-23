import {useEffect, useState} from 'react';
import {useMatch, useRouterState} from '@tanstack/react-router';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {ReadEntry} from './ReadEntry';
import {readSearch, retainedReadTarget, type ReadTarget} from './route-search';
import styles from './entry.module.css';

/** The registry owns one admitted host. This module owns its one current book;
 * Reading Settings can cover it without closing the engine or its overrides. */
export function ReadHost() {
  const captured = useProtectedWebScreenContext();
  const match = useMatch({from: '/read/$itemId', shouldThrow: false});
  const pathname = useRouterState({select: state => state.location.pathname});
  const itemId = match?.params.itemId, sourceId = readSearch(match?.search ?? {}).sourceId;
  const [retained, setRetained] = useState<ReadTarget>();
  const settings = pathname === '/settings';
  useEffect(() => {setRetained(previous => retainedReadTarget(previous, itemId ? {itemId, sourceId} : undefined, settings));}, [itemId, sourceId, settings]);
  if (!captured || !retained) return null;
  const visible = !!match && retained.itemId === itemId && retained.sourceId === sourceId;
  return <div className={styles.host} hidden={!visible} inert={!visible} aria-hidden={!visible}>
    <ReadEntry key={JSON.stringify([retained.itemId, retained.sourceId])} captured={captured} itemId={retained.itemId} sourceId={retained.sourceId}/>
  </div>;
}

/** The real screen is painted once by the admitted retained host. */
export function ReadRouteMarker() {return null;}
