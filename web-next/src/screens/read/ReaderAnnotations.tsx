import {useEffect, useLayoutEffect, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {randomId} from '@core/random-id.ts';
import {ReadingAnnotationService, type ReadingAnnotationState} from '@core/reading-annotation-service.ts';
import type {ReadingAnnotation, ReadingAnnotationContent, ReadingAnnotationPort} from '@core/reading-annotations.ts';
import {readingLocationFromReader, readerLocationFromReading, type ReadingController, type ReaderSnapshot, type ReaderSelection} from '@portico/reader-core';
import type {ReadingPreferences} from '@portico/client-core';
import type {ReaderMountAuthority} from './mount';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays/Dialog';
import styles from './annotations.module.css';
const t = defaultI18n.t;
const blank: ReadingAnnotationState = Object.freeze({entries: [], nextCursor: '', loading: false, saving: false});
const noSubscribe = () => () => {}, noSnapshot = () => blank;

export function useReaderAnnotations(port: ReadingAnnotationPort | undefined, controller: ReadingController, authority: ReaderMountAuthority, active: boolean, ready: boolean) {
  const [owned, setOwned] = useState<Readonly<{port: ReadingAnnotationPort; service: ReadingAnnotationService}>>();
  if (owned && (owned.port !== port || !active)) owned.service.dispose();
  useLayoutEffect(() => {
    if (!port || port.manifest.canAnnotate !== true || !active) return;
    const service = new ReadingAnnotationService(port, () => {
      const state = controller.getSnapshot();
      return authority.isCurrent() && state.sourceId === port.manifest.sourceId && state.sourceRevision === port.manifest.revision && state.status === 'ready';
    });
    setOwned({port, service});
    return () => service.dispose();
  }, [port, controller, authority, active]);
  const service = owned?.port === port && active ? owned.service : undefined;
  const state = useSyncExternalStore(service?.subscribe ?? noSubscribe, service?.getSnapshot ?? noSnapshot, noSnapshot);
  useEffect(() => {if (service && ready) void service.load();}, [service, ready]);
  return {service, state};
}
const colours = ['yellow','green','blue','pink'] as const;
const colourLabels = {yellow:'next.reader.annotationYellow',green:'next.reader.annotationGreen',blue:'next.reader.annotationBlue',pink:'next.reader.annotationPink'} as const;
function selectedContent(selection: ReaderSelection, colour: typeof colours[number]): ReadingAnnotationContent | undefined {
  if (!selection.text.trim() || new TextEncoder().encode(selection.text).length > 8192 || selection.location.format === 'cbz'
    || selection.location.format === 'pdf' && !selection.pageRects?.length) return;
  return Object.freeze({id: randomId(), kind:'highlight', location:readingLocationFromReader(selection.location), text:selection.text, note:'', colour,
    ...(selection.location.format === 'pdf' ? {rects:selection.pageRects} : {})});
}
export function ReaderAnnotationActions({service, snapshot, controller, onSearch, reportError}: {
  service?: ReadingAnnotationService; snapshot: ReaderSnapshot<ReadingPreferences>; controller: ReadingController; onSearch?: (text: string) => void; reportError: (error: unknown) => void;
}) {
  const [draft, setDraft] = useState<Readonly<{owner: ReadingAnnotationService; content: ReadingAnnotationContent}>>(), [note, setNote] = useState('');
  const [colour, setColour] = useState<typeof colours[number]>('yellow');
  const state = useSyncExternalStore(service?.subscribe ?? noSubscribe, service?.getSnapshot ?? noSnapshot, noSnapshot);
  const selection = snapshot.selection;
  const shownDraft = draft?.owner === service ? draft : undefined;
  const current = () => !!service?.current() && controller.getSnapshot().sourceId === snapshot.sourceId && controller.getSnapshot().sourceRevision === snapshot.sourceRevision;
  if (!service?.current()) return null;
  const busy = state.loading || state.saving || state.revision === undefined;
  const capture = () => current() && controller.getSnapshot().selection === selection && selection ? selectedContent(selection, colour) : undefined;
  return <>
    {selection && <div className={styles.selection} role="toolbar" aria-label={t('next.reader.selectionActions')}>
      {colours.map(value => <button key={value} type="button" data-colour={value} aria-label={t(colourLabels[value])} aria-pressed={value === colour} onMouseDown={event => event.preventDefault()} onClick={() => setColour(value)}/>)}
      <Button disabled={busy} onMouseDown={event => event.preventDefault()} onClick={() => {const content = capture();if (content) void service.write(content);}}>{t('next.reader.addHighlight')}</Button>
      <Button disabled={busy} onMouseDown={event => event.preventDefault()} onClick={() => {const content = capture();if (content) {setDraft({owner:service,content});setNote('');}}}>{t('next.reader.addNote')}</Button>
      {navigator.clipboard && <Button onMouseDown={event => event.preventDefault()} onClick={() => {if (current() && controller.getSnapshot().selection === selection) void navigator.clipboard.writeText(selection.text).catch(reportError);}}>{t('next.reader.copySelection')}</Button>}
      {onSearch && <Button onMouseDown={event => event.preventDefault()} onClick={() => {if (current() && controller.getSnapshot().selection === selection) onSearch(selection.text);}}>{t('next.reader.searchSelection')}</Button>}
    </div>}
    <Dialog open={!!shownDraft && current()} onOpenChange={open => {if (!open && !state.saving) setDraft(undefined);}} title={t('next.reader.addNote')} size="sm"
      footer={<><Button disabled={state.saving} onClick={() => setDraft(undefined)}>{t('action.cancel')}</Button><Button disabled={busy || !shownDraft} onClick={() => {const captured = shownDraft;if (captured && current()) void service.write({...captured.content,note}).then(saved => {if (saved && current()) setDraft(value => value === captured ? undefined : value);});}}>{t('next.reader.annotationSave')}</Button></>}>
      <blockquote>{shownDraft?.content.text}</blockquote><label>{t('next.reader.note')}<textarea value={note} maxLength={4096} disabled={state.saving} onChange={event => setNote(event.currentTarget.value)}/></label>
      {state.error && <p role="alert">{t('next.reader.annotationFailed')}</p>}
    </Dialog>
  </>;
}
export function ReaderAnnotationList({kind, service, state, controller, reportError}: {kind: 'bookmark' | 'highlight'; service: ReadingAnnotationService; state: ReadingAnnotationState; controller: ReadingController; reportError: (error: unknown) => void}) {
  const [editing, setEditing] = useState<ReadingAnnotation>(), [note, setNote] = useState('');
  const entries = state.entries.filter(entry => entry.kind === kind), busy = state.loading || state.saving || state.revision === undefined;
  const owns = (entry: ReadingAnnotation) => service.current() && service.getSnapshot().entries.includes(entry);
  return <div className={styles.list}>
    {state.error && <p role="alert">{t('next.reader.annotationFailed')}</p>}
    {state.error && <Button disabled={state.loading || state.saving} onClick={() => {void service.load();}}>{t('next.reader.annotationReload')}</Button>}
    {state.loading && <p role="status">{t('status.loading')}</p>}
    {!state.loading && !state.error && state.revision !== undefined && !entries.length && <p>{t('next.reader.annotationEmpty')}</p>}
    {entries.map(entry => <article key={entry.id} data-colour={entry.colour}>
      <button type="button" className={styles.anchor} onClick={() => {if (owns(entry)) void controller.navigate({kind:'location',location:readerLocationFromReading(entry.location)}).catch(reportError);}}>
        {entry.text || (entry.location.format === 'epub' ? entry.location.section || t('next.reader.bookmarks') : typeof entry.location.pageIndex === 'number' ? t('next.reader.pageLabel',{page:defaultI18n.number(entry.location.pageIndex + 1)}) : t('next.reader.bookmarks'))}
      </button>
      {entry.note && <p>{entry.note}</p>}<time dateTime={new Date(entry.updatedAt).toISOString()}>{defaultI18n.dateTime(entry.updatedAt)}</time>
      <Button disabled={busy} onClick={() => {if (owns(entry)) {setEditing(entry);setNote(entry.note);}}}>{t('next.reader.annotationEdit')}</Button>
      <Button disabled={busy} onClick={() => {if (owns(entry)) void service.write({id:entry.id,remove:true});}}>{t('next.reader.annotationDelete')}</Button>
    </article>)}
    {state.nextCursor && <Button disabled={busy} onClick={() => {void service.load(true);}}>{t('next.reader.annotationMore')}</Button>}
    <Dialog open={!!editing && owns(editing)} onOpenChange={open => {if (!open && !state.saving) setEditing(undefined);}} title={t('next.reader.annotationEdit')} size="sm"
      footer={<><Button disabled={state.saving} onClick={() => setEditing(undefined)}>{t('action.cancel')}</Button><Button disabled={busy} onClick={() => {const entry=editing;if (entry && owns(entry)) void service.write({...entry,note}).then(saved => {if (saved) setEditing(value => value === entry ? undefined : value);});}}>{t('next.reader.annotationSave')}</Button></>}>
      <blockquote>{editing?.text}</blockquote><label>{t('next.reader.note')}<textarea value={note} maxLength={4096} onChange={event => setNote(event.currentTarget.value)}/></label>
    </Dialog>
  </div>;
}
