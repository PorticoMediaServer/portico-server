import {useRef, type ReactNode} from 'react';
import {Button, TextField} from '../controls';
import {BottomSheet, Popover, Toast, type OverlayChange} from '../overlays';
import {usePhone} from '../overlays/shared';
import {Icon} from '../Icon';
import {selectionText as text} from './copy';
import {missingPickerIds, retryLabel, selectionCountText} from './presentation';
import type {PickerEntry, PickerResult, SelectionCount, SelectionPickerProps} from './types';
import styles from './selection.module.css';

type Destination = 'playlist' | 'collection';
function PickerRow({entry, onSelect, busy, end}: {entry: PickerEntry; onSelect: () => void; busy: boolean; end?: ReactNode}) {
  return <Button variant="text" size="small" className={styles.pickerRow} disabled={busy} onClick={onSelect}>
    <span className={styles.pickerArtwork}>{entry.artwork}</span><span className={styles.pickerText}><b>{entry.name}</b><small>{text('next.selection.titles', {count: entry.size})}</small></span>
    {end ?? (entry.alreadyIn ? <span className={styles.pickerEnd}>{text('next.selection.alreadyIn', {count: entry.alreadyIn})}</span> : undefined)}
  </Button>;
}

/** Every action delegates to the supplied operation; entries and outcomes are bounded real data. */
function SelectionPicker({destination, ...props}: SelectionPickerProps & {destination: Destination}) {
  const {selection, entries, query, onQueryChange, hasMore, onLoadMore, mode, onModeChange, name, onNameChange, onAdd, onCreateAndAdd, onRetry, result, busy = false, error, anchorRef, ...surface} = props;
  const phone = usePhone(), nameRef = useRef<HTMLInputElement | null>(null);
  const newName = text(destination === 'playlist' ? 'next.selection.newPlaylistName' : 'next.selection.newCollectionName');
  const title = text(mode === 'new' ? 'next.selection.newPickerTitle' : 'next.selection.pickerTitle', {items: selectionCountText(selection), destination: text(destination === 'playlist' ? 'next.selection.playlistWord' : 'next.selection.collectionWord')});
  const content = <div className={styles.pickerContent} aria-busy={busy || undefined}>
    {!phone && <div className={styles.pickerHeading}>{title}</div>}
    {result && result.failures.length > 0 ? <>
      <div className={styles.pickerFailure} role="status"><b><Icon name="i-warn" size={18} />{text('next.selection.addedPartial', {added: result.added, total: result.total, name: result.destination.name})}</b>
        {result.failures.map(failure => <p key={failure.id}>{text('next.selection.couldNotAdd', {title: failure.title, reason: failure.reason})}</p>)}
      </div>
      <PickerRow entry={result.destination} busy={busy} onSelect={() => onRetry(missingPickerIds(result.failures))} end={<span className={styles.pickerEnd}>{text('next.selection.added', {count: result.added})}</span>} />
      <div className={styles.pickerButtons}><Button size="small" disabled={busy} onClick={() => surface.onOpenChange(false, 'action')}>{text('next.selection.done')}</Button><Button size="small" variant="primary" loading={busy} onClick={() => onRetry(missingPickerIds(result.failures))}>{retryLabel(result.failures)}</Button></div>
    </> : mode === 'new' ? <form className={styles.pickerNew} onSubmit={event => { event.preventDefault(); if (!busy) onCreateAndAdd(name.trim() || newName); }}>
      <TextField ref={nameRef} autoFocus label={text('next.selection.name')} aria-label={text('next.selection.name')} value={name} onChange={event => onNameChange(event.currentTarget.value)} placeholder={newName} disabled={busy} />
      <div className={styles.pickerButtons}><Button size="small" disabled={busy} onClick={() => onModeChange('choose')}>{text('next.selection.cancel')}</Button><Button size="small" type="submit" variant="primary" loading={busy}>{text('next.selection.createAndAdd', {count: selection.count})}</Button></div>
    </form> : <>
      {onQueryChange && <TextField variant="search" leadingIcon="i-search" aria-label={text(destination === 'playlist' ? 'next.selection.findPlaylist' : 'next.selection.findCollection')} placeholder={text(destination === 'playlist' ? 'next.selection.findPlaylist' : 'next.selection.findCollection')} value={query ?? ''} onChange={event => onQueryChange(event.currentTarget.value)} disabled={busy} containerClassName={styles.pickerSearch} />}
      <Button variant="text" size="small" className={styles.pickerRow} disabled={busy} onClick={() => onModeChange('new')}><span className={styles.newPickerArtwork}><Icon name="i-plus" size={20} /></span><span className={styles.pickerText}><b>{text(destination === 'playlist' ? 'next.selection.newPlaylist' : 'next.selection.newCollection')}</b></span></Button>
      <div className={styles.pickerSeparator} />
      <div className={styles.pickerEntries}>{entries.map(entry => <PickerRow key={entry.id} entry={entry} busy={busy} onSelect={() => onAdd(entry.id)} />)}</div>
      {hasMore && onLoadMore && <Button variant="text" size="small" className={styles.pickerLoadMore} disabled={busy} onClick={onLoadMore}>{text('next.selection.showMore')}</Button>}
    </>}
    {error && <div className={styles.pickerFailure} role="alert"><Icon name="i-warn" size={18} /><p>{error}</p></div>}
  </div>;
  const shared = {...surface, returnFocusRef: surface.returnFocusRef ?? anchorRef, title: phone && mode === 'new' ? newName : title, initialFocusRef: mode === 'new' ? nameRef : undefined, className: styles.picker, dismissOnOutsidePress: !busy, dismissOnBack: !busy};
  return phone ? <BottomSheet {...shared} fitContent>{content}</BottomSheet> : <Popover {...surface} anchorRef={anchorRef} returnFocusRef={surface.returnFocusRef ?? anchorRef} initialFocusRef={mode === 'new' ? nameRef : undefined} title={title} width={360} placement="bottom-start" className={styles.picker}>{content}</Popover>;
}

export function PlaylistPicker(props: SelectionPickerProps) { return <SelectionPicker {...props} destination="playlist" />; }
export function CollectionPicker(props: SelectionPickerProps) { return <SelectionPicker {...props} destination="collection" />; }

export type PickerAddedToastProps = Readonly<{open: boolean; onOpenChange: OverlayChange; selection: SelectionCount; destination: Destination; result: PickerResult; onView: (id: string) => void}>;
/** Mount in the existing ToastHost slot; the caller ends selection only after a confirmed complete result. */
export function PickerAddedToast({open, onOpenChange, selection, destination, result, onView}: PickerAddedToastProps) {
  if (result.failures.length) return null;
  return <Toast open={open} onOpenChange={onOpenChange} className={styles.pickerToast} message={<>{text('next.selection.addedToast', {items: selectionCountText(selection), name: result.destination.name})}{result.alreadyIn > 0 && <> {text('next.selection.duplicatesToast', {count: result.alreadyIn})}</>}</>} action={{label: text(destination === 'playlist' ? 'next.selection.viewPlaylist' : 'next.selection.viewCollection'), onAction: () => onView(result.destination.id)}} />;
}
