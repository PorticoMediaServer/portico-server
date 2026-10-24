import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {SavedService} from '@core/saved.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../kit/controls';
import {Dialog} from '../kit/overlays';
import {CollectionPicker, DeleteMediaDialog, PlaylistPicker, type PickerEntry, type PickerResult} from '../kit/selection';
import {entryActions, type EntryActionPorts, type EntryActionTarget, type EntryDestination} from './entry-actions';
import {deleteSurfaceFor, EntryDeleteProducer} from './entry-delete';

const t = defaultI18n.t;
function errorText(code: string | undefined): string | undefined {
  if (!code) return undefined;
  return t(code === 'delete_not_ready' ? 'next.selection.deleteNotReady'
    : code === 'delete_uncertain' ? 'next.selection.deleteUncertain'
      : 'next.selection.deleteFailed');
}

function EntryPickerSurface({destination, target, ports, anchor, canCreate, onClose}: {
  destination: EntryDestination; target: EntryActionTarget; ports: EntryActionPorts; anchor: HTMLElement | null; canCreate: boolean; onClose: () => void;
}) {
  const [mode, setMode] = useState<'choose' | 'new'>('choose'), [name, setName] = useState(''), [query, setQuery] = useState('');
  const [busy, setBusy] = useState(false), [error, setError] = useState<string | undefined>(undefined), [result, setResult] = useState<PickerResult | undefined>(undefined);
  const service = useMemo(() => new SavedService({api: ports.api, scope: {serverId: ports.serverId, viewerId: ports.viewerId}, requestId: ports.requestId}), [ports]);
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const anchorRef = useRef<HTMLElement | null>(anchor);
  useEffect(() => {
    void service.select({view: destination === 'playlist' ? 'playlists' : 'collections'}).catch(() => setError(t('next.selection.pickerFailed')));
    return () => service.dispose();
  }, [service, destination]);
  const entries = useMemo<readonly PickerEntry[]>(() => {
    const rows = snapshot.projection?.sections.flatMap(section => section.entries) ?? [];
    const wanted = rows.flatMap(entry => destination === 'playlist'
      ? entry.kind === 'playlist' ? [{id: entry.id, name: entry.title, size: entry.count ?? 0, canAdd: true}] : []
      : entry.kind === 'collection' ? [{id: entry.id, name: entry.title, size: 0, canAdd: true}] : []);
    const needle = query.trim().toLowerCase();
    return Object.freeze(wanted.filter(row => !needle || row.name.toLowerCase().includes(needle)));
  }, [snapshot.projection, destination, query]);
  const finish = (id: string, label: string) => setResult({destination: {id, name: label}, inserted: 1, processed: 1, total: 1, complete: true, failures: []});
  const add = async (id: string, label: string) => {
    if (!ports.isCurrent()) return;
    setBusy(true); setError(undefined);
    try {
      const operationId = await ports.requestId();
      if (!ports.isCurrent()) return;
      if (destination === 'playlist') await ports.api.request(routePath('post_playlists_id_entries').replace('{id}', encodeURIComponent(id)), 'POST', {operationId, itemIds: [target.itemId]});
      else await ports.api.request(routePath('post_collections_id_items_batch').replace('{id}', encodeURIComponent(id)), 'POST', {addItemIds: [target.itemId], removeItemIds: []});
      if (!ports.isCurrent()) return;
      finish(id, label);
    } catch { if (ports.isCurrent()) setError(t('next.selection.pickerFailed')); }
    finally { if (ports.isCurrent()) setBusy(false); }
  };
  const create = async (value: string) => {
    if (!value.trim() || !ports.isCurrent()) return;
    if (destination === 'collection' && !canCreate) { setError(t('next.selection.collectionOwnerOnly')); return; }
    setBusy(true); setError(undefined);
    try {
      const operationId = await ports.requestId();
      if (!ports.isCurrent()) return;
      if (destination === 'playlist') {
        const made = await ports.api.request<{playlistId: string}>(routePath('post_playlists'), 'POST', {name: value, itemIds: [target.itemId], operationId});
        if (ports.isCurrent()) finish(made.playlistId, value);
      } else {
        const made = await ports.api.request<{id: string}>(`/v1/libraries/${encodeURIComponent(target.libraryId)}/collections`, 'POST', {name: value});
        await ports.api.request(routePath('post_collections_id_items_batch').replace('{id}', encodeURIComponent(made.id)), 'POST', {addItemIds: [target.itemId], removeItemIds: []});
        if (ports.isCurrent()) finish(made.id, value);
      }
    } catch { if (ports.isCurrent()) setError(t('next.selection.pickerFailed')); }
    finally { if (ports.isCurrent()) setBusy(false); }
  };
  const Picker = destination === 'playlist' ? PlaylistPicker : CollectionPicker;
  return <Picker open onOpenChange={value => { if (!value) onClose(); }} selection={{count: 1}} anchorRef={anchorRef}
    entries={entries} query={query} onQueryChange={setQuery} loading={snapshot.phase === 'idle' || snapshot.phase === 'loading'}
    mode={mode} onModeChange={next => { if (next === 'new' && destination === 'collection' && !canCreate) { setError(t('next.selection.collectionOwnerOnly')); return; } setMode(next); }}
    name={name} onNameChange={setName} onAdd={id => { const entry = entries.find(row => row.id === id); void add(id, entry?.name ?? ''); }}
    onCreateAndAdd={value => void create(value)} onRetry={() => { setError(undefined); setResult(undefined); void service.select({view: destination === 'playlist' ? 'playlists' : 'collections'}).catch(() => setError(t('next.selection.pickerFailed'))); }}
    onRetryOperation={() => setResult(undefined)} onRetryError={() => { setError(undefined); setResult(undefined); }}
    result={result} busy={busy} error={error} />;
}

/** Mounted once by the shell. It renders nothing until a screen effect opens a surface. */
export function EntryActionHost() {
  const open = useSyncExternalStore(entryActions.subscribe, entryActions.getSnapshot, () => null);
  const producer = useRef(new EntryDeleteProducer());
  const state = useSyncExternalStore(producer.current.subscribe, producer.current.getSnapshot, producer.current.getSnapshot);
  const anchor = useRef<HTMLElement | null>(null);
  const stale = !!open && !open.ports.isCurrent();
  useEffect(() => {
    if (stale) { entryActions.close(); return; }
    const current = producer.current;
    if (!open) { current.close(); return; }
    anchor.current = open.anchor;
    current.open(open.target, open.ports);
    return () => current.close();
  }, [open, stale]);
  if (!open || stale) return null;
  if (open.kind === 'picker') return <EntryPickerSurface destination={open.destination} target={open.target} ports={open.ports} anchor={open.anchor} canCreate={open.canCreate} onClose={() => entryActions.close()} />;
  const deleteNow = () => { void (state.phase === 'uncertain' ? producer.current.retry() : producer.current.confirm()); };
  const surface = deleteSurfaceFor(state.phase);
  if (surface === 'uncertain') {
    return <Dialog open onOpenChange={value => { if (!value) entryActions.close(); }} title={t('next.selection.delete')}
      returnFocusRef={anchor} footer={<Button variant="primary" onClick={deleteNow}>{t('next.states.tryAgain')}</Button>}>
      <p role="alert">{errorText(state.errorCode)}</p>
    </Dialog>;
  }
  return <DeleteMediaDialog owner open onOpenChange={value => { if (!value && state.phase !== 'deleting' && state.phase !== 'confirming') entryActions.close(); }}
    returnFocusRef={anchor} selectionCount={1} preview={state.preview ?? null}
    confirmation={state.confirmation} onConfirmationChange={value => producer.current.setConfirmation(value)}
    removeFiles={state.removeFiles} onRemoveFilesChange={value => producer.current.setRemoveFiles(value)}
    onDelete={deleteNow} busy={state.phase === 'loading' || state.phase === 'confirming' || state.phase === 'deleting'}
    error={errorText(state.errorCode)} result={state.result}
    ownerTag={<span>{t('next.selection.owner')}</span>} />;
}
