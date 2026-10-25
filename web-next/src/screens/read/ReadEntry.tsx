import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {ReadingSession} from '@portico/client-core';
import {createComicAdapter, createEpubAdapter, createPdfAdapter, createReadingPublication,
  type ReadingController, type ReadingPublication, type ReaderLocation, type ReaderContentsTarget} from '@portico/reader-core';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {Skeleton, StateView} from '../../kit/states';
import {Button} from '../../kit/controls';
import {createReadBindings} from './bindings';
import {bindReadPreferences} from './preferences';
import {bindReadingProgress} from './progress';
import {loadReadFonts, type ReadFonts} from './fonts';
import {readerPdfAdapterOptions} from './pdf-worker';
import type {ReaderMountAuthority} from './mount';
import styles from './entry.module.css';
import {takeContentsTarget, retireContentsTarget} from './contents-handoff';
import {ReaderBook} from './ReaderBook';

const {t} = defaultI18n;
type Captured = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;
type OpenBook = Readonly<{session: ReadingSession; publication: ReadingPublication; controller: ReadingController;
  authority: ReaderMountAuthority; location?: ReaderLocation; contentsTarget?: ReaderContentsTarget; annotations?: ReadingSession}>;

/** Real publication session; services start only inside the captured route lifetime. */
export function ReadEntry({captured, itemId, sourceId, active = true, onClose}: {captured: Captured; itemId: string; sourceId?: string; active?: boolean; onClose?: () => void}) {
  const [state, setState] = useState<Readonly<{book?: OpenBook; failed?: boolean}>>({});
  const [contentsTarget] = useState(() => takeContentsTarget(captured.context, itemId, sourceId));
  const [retry, setRetry] = useState(0);
  const reportRef = useRef(captured.binding.home.effects.reportError); reportRef.current = captured.binding.home.effects.reportError;
  const reportError = useRef((error: unknown) => {if (captured.context.isCurrent()) reportRef.current(error);}).current;
  useEffect(() => {
    retireContentsTarget(captured.context, contentsTarget);
    const life = new AbortController(), bindings = createReadBindings(captured.context);
    let alive = true, fonts: ReadFonts | undefined, publication: ReadingPublication | undefined;
    let reader: ReturnType<typeof bindReadPreferences> | undefined, progress: ReturnType<typeof bindReadingProgress> | undefined;
    const offRetired = bindings.authority.subscribe(() => {if (alive) setState({failed: true});});
    let disposal: Promise<void> | undefined;
    const dispose = () => {
      if (disposal) return disposal;
      alive = false; offRetired(); life.abort();
      disposal = (async () => {
        try {await reader?.dispose();}
        finally {
          try {await progress?.stop();}
          finally {
            try {publication?.dispose();}
            finally {try {fonts?.dispose();} finally {await bindings.dispose();}}
          }
        }
      })();
      return disposal;
    };
    setState({});
    void (async () => {
      const versions = (await bindings.client.versions(itemId, life.signal)).versions.filter(version => version.inventoryAvailable);
      if (!alive) return;
      const selected = sourceId ? versions.find(version => version.sourceId === sourceId) : versions.length === 1 ? versions[0] : undefined;
      if (!selected) {
        if (versions.length > 1 && !sourceId) await router.navigate({to: '/item/$itemId', params: {itemId}, replace: true});
        else setState({failed: true});
        return;
      }
      const session = await bindings.open({itemId, sourceId: selected.sourceId});
      publication = await createReadingPublication(session, life.signal);
      if (contentsTarget) {
        const current = publication.contentsTarget(contentsTarget.index);
        if (current.sourceId !== contentsTarget.sourceId || current.sourceRevision !== contentsTarget.sourceRevision) throw new Error('source_changed');
      }
      const saved = await session.progress(life.signal), location = publication.resumeLocation(saved) ?? undefined;
      if (publication.format === 'epub') fonts = await loadReadFonts({document, signal: life.signal});
      if (!alive) return;
      const adapters = publication.format === 'epub' ? [createEpubAdapter({fonts: fonts!.fonts})]
        : publication.format === 'pdf' ? [createPdfAdapter(readerPdfAdapterOptions)] : [createComicAdapter({pages: publication.comicPages!})];
      reader = bindReadPreferences({adapters, preferences: bindings.preferences, authority: bindings.authority, reportError: code => reportError({code})});
      progress = bindReadingProgress({controller: reader.controller, session, initial: saved, authority: bindings.authority, reportError});
      setState({book: {session, publication, controller: reader.controller, authority: bindings.authority, location, contentsTarget, ...(session.manifest.canAnnotate === true ? {annotations:session} : {})}});
    })().catch(error => {
      // A rejected bootstrap must never leave the reader on its loading skeleton. This attempt is
      // still the live one unless it was retired, so show the failure state; report only while the
      // captured authority still holds, since a retired capture owns no error surface.
      if (alive && !life.signal.aborted) {if (bindings.authority.isCurrent()) reportError(error); setState({failed: true});}
      void dispose().catch(() => {});
    });
    return () => {void dispose().catch(() => {});};
  }, [captured.context, itemId, sourceId, contentsTarget, reportError, retry]);
  if (state.book) return <ReaderBook key={state.book.session.manifest.sessionId} book={state.book} active={active} reportError={reportError} onSearchAll={query => {if (!captured.context.isCurrent()) return; void router.navigate({to: '/search', search: {q: query}}).catch(reportError);}} onOpenSettings={() => {void router.navigate({to: '/settings', search: {scope: 'account', section: 'reading'}}).catch(reportError);}} onBack={onClose ?? (() => router.history.back())}/>;
  /** The drawn way back stays available while a book is opening and when it cannot open. */
  const leave = onClose ?? (() => router.history.back());
  if (state.failed) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setRetry(value => value + 1)}}
    secondary={{id: 'back', label: t('next.reader.back'), onPress: leave}}/>;
  return <div className={styles.loading}><span className={styles.escape}><Button variant="glass" icon="i-left" iconOnly aria-label={t('next.reader.back')} onClick={leave}/></span><Skeleton height="100dvh"/></div>;
}
