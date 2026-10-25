import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {DetailService, type DetailProjection} from '@core/detail.ts';
import {defaultI18n} from '@i18n';
import {Dialog} from '../kit/overlays';
import {MetadataEditor} from '../screens/metadata/MetadataEditor';
import styles from '../screens/metadata/metadata.module.css';
import type {EntryActionPorts, EntryActionTarget} from './entry-actions';

export type EntryMetadataEffect = (target: EntryActionTarget, anchor: HTMLElement) => Promise<void>;
type Open = Readonly<{id: number; target: EntryActionTarget; anchor: HTMLElement; ports: EntryActionPorts; lifetime: AbortController; ownerCurrent: () => boolean}>;
let opened: Open | null = null;
let serial = 0;
const listeners = new Set<() => void>();
const emit = () => {for (const listener of listeners) listener();};
const close = () => {opened?.lifetime.abort(); opened = null; emit();};
const current = (value: Open) => opened === value && !value.lifetime.signal.aborted && value.ports.isCurrent() && value.ownerCurrent();

/** The caller's local owner epoch is rechecked for every read and write. */
export function createEntryMetadataEffect(ports: EntryActionPorts, ownerCurrent: () => boolean): EntryMetadataEffect {
  return async (target, anchor) => {
    if (!ports.isCurrent() || !ownerCurrent()) throw new DOMException('Metadata owner changed', 'AbortError');
    close();
    opened = Object.freeze({id: ++serial, target: Object.freeze({...target}), anchor, ports, ownerCurrent, lifetime: new AbortController()});
    emit();
  };
}
export function retireEntryMetadata(ports: EntryActionPorts) {if (opened?.ports === ports) close();}

function CheckedMetadata({value}: {value: Open}) {
  const anchor = useRef<HTMLElement | null>(value.anchor);
  const [data, setData] = useState<DetailProjection>();
  const [failed, setFailed] = useState(false);
  const api = useMemo(() => ({async request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> {
    const check = () => {if (!current(value)) throw new DOMException('Metadata authority changed', 'AbortError');};
    check();
    const result = await value.ports.api.request<T>(path, method, body, signal ? AbortSignal.any([signal, value.lifetime.signal]) : value.lifetime.signal);
    check(); return result;
  }}), [value]);
  useEffect(() => {
    const service = new DetailService({api, scope: {serverId: value.ports.serverId, viewerId: value.ports.viewerId}, requestId: value.ports.requestId});
    void service.select({libraryId: value.target.libraryId, itemId: value.target.itemId}).then(() => {
      if (!current(value)) return;
      const snapshot = service.getSnapshot(), detail = snapshot.data;
      if (snapshot.phase !== 'ready' || !detail || detail.item.id !== value.target.itemId || detail.scope.libraryId !== value.target.libraryId
        || detail.item.kind !== value.target.kind || !['movie', 'episode', 'song', 'audiobook_file'].includes(detail.item.kind) || !detail.actions.some(action => action.id === 'metadata_edit' && action.enabled)) {setFailed(true); return;}
      setData(detail);
    }).catch(() => {if (current(value)) setFailed(true);});
    return () => {service.dispose(); value.lifetime.abort();};
  }, [api, value]);
  const metadataApi = useMemo(() => ({async request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> {
    if (!data || !current(value)) throw new DOMException('Metadata grant retired', 'AbortError');
    const result = await api.request<T>(path, method, body, signal);
    if (!current(value)) throw new DOMException('Metadata grant retired', 'AbortError');
    // Every full editing document must retain the fresh Detail viewer fence.
    if (result && typeof result === 'object' && 'viewerFence' in result && result.viewerFence !== data.scope.viewerFence) throw new Error('metadata-fence-mismatch');
    return result;
  }}), [api, data, value]);
  const scope = useMemo(() => ({serverId: value.ports.serverId, viewerId: value.ports.viewerId}), [value]);
  const target = useMemo(() => ({kind: 'item' as const, id: value.target.itemId, libraryId: value.target.libraryId}), [value]);
  return <Dialog open onOpenChange={open => {if (!open) close();}} title={defaultI18n.t('next.home.editSelection')}
    returnFocusRef={anchor} placement="right" width={720} showCloseButton={!data} headerClassName={data ? styles.hostTitle : undefined} bodyClassName={data ? styles.hostBody : undefined}>
    {data ? <MetadataEditor api={metadataApi} scope={scope} target={target} onClose={close}/>
      : <p role={failed ? 'alert' : 'status'}>{defaultI18n.t(failed ? 'next.states.crashed' : 'next.states.loading')}</p>}
  </Dialog>;
}

export function EntryMetadataHost() {
  const value = useSyncExternalStore(listener => {listeners.add(listener); return () => {listeners.delete(listener);};}, () => opened, () => null);
  return value && current(value) ? <CheckedMetadata key={value.id} value={value}/> : null;
}
