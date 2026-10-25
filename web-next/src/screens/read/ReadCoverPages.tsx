import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {ReadingSession} from '@portico/client-core';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays';
import {Skeleton, StateView} from '../../kit/states';
import type {ScreenContext} from '../detail/context';
import {createReadBindings, type ReadBindingContext} from './bindings';
import styles from './cover-pages.module.css';

const {t} = defaultI18n;
/** CBZ pages are actual admitted archive images. No PDF page count or EPUB page is inferred. */
export function ReadCoverPages({reading, context, itemId, sourceId, upload, busy}: {reading: ReadBindingContext; context: ScreenContext; itemId: string; sourceId: string; upload: (file: Blob) => Promise<boolean>; busy: boolean}) {
  const [open, setOpen] = useState(false), [session, setSession] = useState<ReadingSession>(), [failed, setFailed] = useState(false), [retry, setRetry] = useState(0);
  const trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open) return;
    const bindings = createReadBindings(reading); let active = true;
    setSession(undefined); setFailed(false);
    const off = bindings.authority.subscribe(() => {if (active) setFailed(true);});
    void bindings.open({itemId, sourceId}).then(value => {
      if (!active || !context.isCurrent() || !bindings.authority.isCurrent()) return;
      if (value.manifest.format !== 'cbz' || !value.manifest.pageNames?.length) throw new Error('invalid_cover_source');
      setSession(value);
    }).catch(error => {if (active && context.isCurrent()) {setFailed(true); context.effects.reportError(error);}});
    return () => {active = false; off(); void bindings.dispose().catch(() => {});};
  }, [open, reading, context, itemId, sourceId, retry]);
  return <><Button ref={trigger} size="small" disabled={busy} onClick={() => {if (context.isCurrent()) setOpen(true);}}>{t('next.reader.chooseCoverPage')}</Button>
    {open && <Dialog open title={t('next.reader.chooseCoverPage')} returnFocusRef={trigger} onOpenChange={setOpen}>
      {failed ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setRetry(value => value + 1)}}/>
        : session ? <Pages key={session.manifest.sessionId} session={session} context={context} upload={upload} busy={busy} onChosen={() => setOpen(false)}/> : <Skeleton height={280}/>}
    </Dialog>}
  </>;
}
function Pages({session, context, upload, busy, onChosen}: {session: ReadingSession; context: ScreenContext; upload: (file: Blob) => Promise<boolean>; busy: boolean; onChosen: () => void}) {
  const [start, setStart] = useState(0), [pages, setPages] = useState<readonly Readonly<{index: number; url: string}>[]>([]), [failed, setFailed] = useState(false), [choosing, setChoosing] = useState(false), [retry, setRetry] = useState(0);
  const choice = useRef<AbortController | undefined>(undefined), requests = useRef(new Set<AbortController>()), pending = useRef(false);
  const count = session.manifest.pageNames!.length, end = Math.min(start + 6, count);
  useEffect(() => {const owned = requests.current; return () => {for (const request of owned) request.abort(); owned.clear();};}, []);
  useEffect(() => {
    const operation = new AbortController(), urls: string[] = []; setPages([]); setFailed(false);
    void (async () => {
      for (let index = start; index < end; index++) {
        const image = await session.page(index, {width: 180, signal: operation.signal});
        if (operation.signal.aborted || !context.isCurrent()) return;
        const url = URL.createObjectURL(new Blob([image.bytes.slice() as Uint8Array<ArrayBuffer>], {type: image.mime})); urls.push(url);
        setPages(previous => [...previous, {index, url}]);
      }
    })().catch(error => {if (!operation.signal.aborted && context.isCurrent()) {setFailed(true); context.effects.reportError(error);}});
    return () => {operation.abort(); for (const url of urls) URL.revokeObjectURL(url);};
  }, [session, context, start, end, retry]);
  const choose = async (index: number) => {
    if (busy || pending.current || !context.isCurrent() || session.signal.aborted) return;
    const operation = new AbortController(); choice.current = operation; requests.current.add(operation); pending.current = true; setChoosing(true); setFailed(false);
    try {
      const image = await session.page(index, {width: 1280, signal: operation.signal});
      if (operation.signal.aborted || !context.isCurrent()) return;
      const blob = new Blob([image.bytes.slice() as Uint8Array<ArrayBuffer>], {type: image.mime});
      if (blob.size > 10 * 1024 * 1024) throw new Error(t('web.metadata.uploadImageLimit'));
      const saved = await upload(blob);
      if (!operation.signal.aborted && context.isCurrent()) {if (saved) onChosen(); else setFailed(true);}
    } catch (error) {if (!operation.signal.aborted && context.isCurrent()) {setFailed(true); context.effects.reportError(error);}}
    finally {requests.current.delete(operation); if (choice.current === operation) {pending.current = false; if (!operation.signal.aborted && context.isCurrent()) setChoosing(false);}}
  };
  return <><div className={styles.pages}>{pages.map(page => <button key={page.index} type="button" disabled={busy || choosing} onClick={() => {void choose(page.index);}}>
    <img src={page.url} alt=""/><span>{t('next.reader.pageLabel', {page: defaultI18n.number(page.index + 1)})}</span></button>)}</div>
    {failed && <Button disabled={busy || choosing} onClick={() => setRetry(value => value + 1)}>{t('next.states.tryAgain')}</Button>}
    <div className={styles.navigation}><Button size="small" disabled={start === 0 || busy || choosing} onClick={() => setStart(value => Math.max(0, value - 6))}>{t('next.reader.previous')}</Button>
      <Button size="small" disabled={end >= count || busy || choosing} onClick={() => setStart(end)}>{t('next.reader.next')}</Button></div></>;
}
