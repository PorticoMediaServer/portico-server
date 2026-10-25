import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {SelectionItem} from '@core/selection-model/index.ts';
import type {HomeSelectionPicker} from '@core/home-selection/types.ts';
import type {EntryActionPorts, EntryDestination} from '../../shell/entry-actions';
import {pickerResultView} from '../../shell/selection-presentation';
import {CollectionPicker, PlaylistPicker, type PickerEntry} from '../../kit/selection';
import {LibraryPickerProducer} from './selection-picker';
export type LibraryPickerOptions = EntryActionPorts & Readonly<{signal: AbortSignal; canCreateCollection: boolean}>;
const t = defaultI18n.t;
export function LibraryPickerSurface({destination, target, ports, anchor, canCreate, onClose}: {
  destination: EntryDestination; target: readonly SelectionItem[]; ports: EntryActionPorts; anchor: HTMLElement | null; canCreate: boolean; onClose: () => void;
}) {
  const producer = useRef<LibraryPickerProducer | null>(null), anchorRef = useRef<HTMLElement | null>(anchor);
  const [picker, setPicker] = useState<HomeSelectionPicker>(), [error, setError] = useState<string>();
  const [mode, setMode] = useState<'choose' | 'new'>('choose'), [name, setName] = useState('');
  useEffect(() => {
    setPicker(undefined); setError(undefined); setMode('choose'); setName('');
    const owned = new LibraryPickerProducer(target, destination, ports, canCreate, value => {if (producer.current === owned) setPicker(value);});
    producer.current = owned;
    void owned.open().catch(() => {if (producer.current === owned && ports.isCurrent()) setError(t('next.selection.pickerFailed'));});
    return () => {owned.close(); if (producer.current === owned) producer.current = null;};
  }, [target, destination, ports, canCreate]);
  const close = () => {producer.current?.close(); producer.current = null; onClose();};
  const run = (work: (value: LibraryPickerProducer) => Promise<void>) => {
    const owned = producer.current; if (!owned || !ports.isCurrent()) return;
    setError(undefined);
    void work(owned).catch(() => {if (producer.current === owned && ports.isCurrent()) setError(t('next.selection.pickerFailed'));});
  };
  const entries: readonly PickerEntry[] = picker?.rows.map(row => ({id: row.id, name: row.name, size: row.count, canAdd: row.canAdd, alreadyIn: row.alreadyIn})) ?? [];
  const result = picker && pickerResultView(picker, () => t('next.selection.pickerFailed'));
  const Picker = destination === 'playlist' ? PlaylistPicker : CollectionPicker;
  return <Picker open onOpenChange={value => {if (!value) close();}} selection={{count: target.length}} anchorRef={anchorRef} returnFocusRef={anchorRef}
    entries={entries} query={picker?.query ?? ''} onQueryChange={value => run(owned => owned.search(value))}
    loading={!picker || picker.phase === 'loading'} busy={picker?.phase === 'adding'}
    hasNext={picker?.next} hasPrevious={picker?.previous} onNext={picker?.next ? () => run(owned => owned.page()) : undefined}
    onPrevious={picker?.previous ? () => run(owned => owned.page(true)) : undefined}
    mode={mode} onModeChange={next => {if (next === 'new' && destination === 'collection' && !canCreate) {setError(t('next.selection.collectionOwnerOnly')); return;} setMode(next);}}
    name={name} onNameChange={setName} onAdd={id => run(owned => owned.choose(id))} onCreateAndAdd={value => run(owned => owned.create(value))}
    onRetry={ids => {const failures = picker?.receipt?.failures.map(row => row.itemId) ?? []; if (picker?.phase !== 'partial' || ids.length !== failures.length || ids.some((id, index) => id !== failures[index])) return; run(owned => owned.retry());}}
    onRetryOperation={picker?.phase === 'uncertain' ? () => run(owned => owned.retry()) : undefined}
    onRetryError={picker?.phase === 'error' || error ? () => run(owned => owned.retry()) : undefined}
    result={result} error={error ?? (picker?.error ? t('next.selection.pickerFailed') : undefined)} />;
}

