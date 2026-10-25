import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {serviceI18n} from '@core/presentation/service-text.ts';
import {formatClock} from '@core/presentation/content.ts';
import type {ContentEntry, ContentProjection} from '@core/library-content.ts';
import {Menu, MenuItem} from '../../../kit/overlays';
import {Artwork, AmbientArt} from '../../../kit/artwork';
import {Button} from '../../../kit/controls';
import {dependsOn, titlesWithPersonalState} from '@core/freshness-hub.ts';
import {useFreshDocument} from '../../../platform/freshness';
import {createAudiobookBindings, type AudiobookBindings, type AudiobookTarget} from './bindings';
import {runScreenCommand, type ScreenContext} from '../context';
import {DetailLoading} from '../Loading';
import {ScreenProblem} from '../ScreenProblem';
import {t} from '../text';
import styles from './audiobooks.module.css';

type Props = {context: ScreenContext; target: AudiobookTarget; onMore?: (target: ContentEntry, anchor: HTMLElement) => void};
export function AudiobookEntry({context, target, onMore}: Props) {
  const {libraryId, view, entityId} = target;
  const binding = useMemo(() => createAudiobookBindings(context, {libraryId, view, entityId}), [context, libraryId, view, entityId]);
  const [failure, setFailure] = useState<unknown>();
  useEffect(() => {let active = true;setFailure(undefined);void binding.load().catch(error => {if (active) setFailure(error);});return () => {active = false;binding.dispose();};}, [binding]);
  return failure ? <ScreenProblem context={context} error={failure} retry={() => {setFailure(undefined);void binding.load().catch(setFailure);}}/>
    : <AudiobookBody binding={binding} onMore={onMore}/>;
}
const duration = (value: number | undefined) => value === undefined ? undefined : serviceI18n().duration(value);
function AudiobookBody({binding, onMore}: {binding: AudiobookBindings; onMore?: Props['onMore']}) {
  const state = useSyncExternalStore(binding.service.subscribe, binding.service.getSnapshot, binding.service.getSnapshot);
  const [expanded, setExpanded] = useState(false), [finished, setFinished] = useState<boolean>(), [busy, setBusy] = useState(false);
  const [more, setMore] = useState<Readonly<{entry: ContentEntry; page: ContentProjection; anchor: HTMLElement}>>();
  const page = state.projection, entity = page?.entity;
  useFreshDocument(() => [dependsOn.libraries, ...titlesWithPersonalState(page?.scope.libraryId), dependsOn.event('stream.resync')], refresh => binding.service.revalidate(refresh), () => binding.service.getSnapshot().phase === 'ready');
  useEffect(() => {
    setExpanded(false);setFinished(undefined);setMore(undefined);
    if (page?.scope.view === 'book') {let active = true;void binding.finished(page).then(value => {if (active) setFinished(value.watched);}, error => {if (active) binding.context.effects.reportError(error);});return () => {active = false;};}
  }, [binding, page]);
  if (!page || !entity) return state.phase === 'error' || state.phase === 'refresh-required'
    ? <ScreenProblem context={binding.context} error={state.error} retry={() => {void binding.load().catch(binding.context.effects.reportError);}}/> : <DetailLoading/>;
  try {binding.current(page);} catch (error) {return state.phase === 'loading' ? <DetailLoading/> : <ScreenProblem context={binding.context} error={state.error ?? error} retry={() => {void binding.load().catch(binding.context.effects.reportError);}}/>;}
  const run = (command: () => unknown) => runScreenCommand(binding.context, command);
  const book = page.scope.view === 'book', author = page.scope.view === 'author';
  const chapters = state.appended.find(section => section.id === 'chapters'), files = state.appended.find(section => section.id === 'files');
  const rows = book ? chapters?.entries.length ? chapters : files : state.appended.find(section => section.id === 'books');
  const context = state.appended.find(section => section.id === 'context')?.entries ?? [];
  const series = context.find(entry => entry.kind === 'book_series'), authorEntry = context.find(entry => entry.kind === 'author');
  const progress = entity.bookProgress, resume = page.listening?.resume;
  const ratio = progress?.positionMs !== undefined && progress.durationMs ? progress.positionMs / progress.durationMs : undefined;
  const remaining = duration(progress?.remainingMs === undefined ? undefined : progress.remainingMs / 1000);
  const primaryBook = !book ? rows?.entries.find(entry => entry.bookProgress && entry.available !== false && !entry.missing) : undefined;
  const canPlay = book ? page.listening?.actions.includes('play') && !page.listening.resumeUnavailable && (!!resume || !!files?.entries.some(entry => entry.playback)) : !!primaryBook;
  const label = book ? resume ? progress?.chapter ? t('title.resumeChapter', {chapter: t('next.kit.chapter', {number: progress.chapter.index})}) : t('entry.resume') : t('entry.play') : t('next.browse.resume', {title: primaryBook?.title ?? ''});
  const cover = entity.posterUrl ?? rows?.entries[0]?.posterUrl;
  const leafMore = (entry: ContentEntry, anchor: HTMLElement) => {
    binding.current(page);
    if (entry.kind !== 'audiobook_file' || entry.available === false || entry.missing || !state.appended.some(section => section.entries.includes(entry))) return;
    setMore({entry, page, anchor});
  };
  const localOwner = binding.context.identity?.viewer.authority === 'local' && binding.context.identity.viewer.role === 'owner';
  const effects = binding.context.effects;
  const hasLeafActions = !!(effects.addToPlaylist || effects.addToCollection || localOwner && effects.deleteTitle);
  const invoke = (effect: typeof effects.addToPlaylist) => run(() => {
    binding.current(page);
    if (!more || more.page !== page || !effect || more.entry.available === false || more.entry.missing || !binding.service.getSnapshot().appended.some(section => section.entries.includes(more.entry))) return;
    return effect({libraryId: page.scope.libraryId, itemId: more.entry.id, kind: more.entry.kind, title: more.entry.title}, more.anchor);
  });
  const open = (entry: ContentEntry) => run(() => binding.open(page, entry));
  return <article className={styles.page} data-audiobook-view={page.scope.view}>
    <AmbientArt className={styles.ambient} treatment="phone" source={binding.context.artwork.ambient(cover)}/>
    <nav className={styles.nav}>
      {binding.context.effects.back && <Button variant="ghost" icon="i-left" iconOnly aria-label={t('action.back')} onClick={() => run(() => {binding.current(page);binding.context.effects.back?.();})}/>}
      {onMore && <Button variant="ghost" icon="i-more" iconOnly aria-label={t('card.more', {title: entity.title})} onClick={event => run(() => {binding.current(page);onMore(entity, event.currentTarget);})}/>}
    </nav>
    <header className={styles.hero}>
      <div className={author ? styles.disc : styles.cover}><Artwork source={binding.context.artwork.foreground(cover, 464, 1)} alt=""/>{author && <b>{entity.title.split(/\s+/u).filter(Boolean).slice(0, 2).map(word => word[0]).join('')}</b>}</div>
      {book && series ? <button className={styles.eyebrow} onClick={() => open(series)}>{series.title}</button> : !book && <p className={styles.eyebrow}>{t(author ? 'filter.field.author' : 'library.tab.series')}</p>}
      <h1>{entity.displayTitle ?? entity.title}</h1>
      {entity.author && (authorEntry ? <button className={styles.author} onClick={() => open(authorEntry)}>{entity.author}</button> : <div className={styles.author}>{entity.author}</div>)}
      <div className={styles.meta}>
        {book && !!entity.narrators?.length && <span>{t('next.kit.readBy', {name: serviceI18n().list(entity.narrators)})}</span>}
        {!book && entity.count !== undefined && <strong>{t('next.kit.books', {count: entity.count})}</strong>}
        {entity.duration !== undefined && <span>{duration(entity.duration)}</span>}
        {book && entity.count !== undefined && <span>{t('title.chapterCount', {count: entity.count})}</span>}
      </div>
    </header>
    <div className={styles.actions}>
      {canPlay && <Button className={styles.primary} variant="primary" icon="i-play" onClick={() => run(() => binding.primary(page))}>{label}</Button>}
      {book && finished !== undefined && <Button className={styles.round} icon="i-check" iconOnly disabled={busy} aria-pressed={finished} aria-label={t(finished ? 'title.markNotFinished' : 'title.markFinished')} onClick={() => {setBusy(true);void binding.markFinished(page).then(value => setFinished(value.watched)).catch(binding.context.effects.reportError).finally(() => setBusy(false));}}/>}
      {!book && !author && primaryBook && <Button className={styles.round} icon="i-retry" iconOnly aria-label={t('action.startOver')} onClick={() => open(primaryBook)}/>}
    </div>
    {book && remaining && <div className={styles.progress}>{ratio !== undefined && <span><i style={{width: `${Math.min(1, Math.max(0, ratio)) * 100}%`}}/></span>}{t('title.resumeRemaining', {remaining})}</div>}
    {book && entity.overview && <><p className={styles.about} data-expanded={expanded || undefined}>{entity.overview}</p>{!expanded && <button className={styles.readMore} onClick={() => setExpanded(true)}>{t('web.person.readMore')}</button>}</>}
    {state.error && <ScreenProblem context={binding.context} error={state.error} retry={() => {void binding.load().catch(binding.context.effects.reportError);}}/>}
    {rows && <section><div className={styles.sectionHead}><h2>{t(book ? 'next.browse.chapters' : 'library.tab.books')}</h2><span>{rows.totalCount}</span></div>
      {rows.entries.map((entry, index) => book ? <div key={entry.id}><Chapter entry={entry} index={index} page={page} binding={binding}/>{entry.kind === 'audiobook_file' && hasLeafActions && <Button variant="ghost" icon="i-more" iconOnly aria-label={t('card.more', {title: entry.title})} onClick={event => run(() => leafMore(entry, event.currentTarget))}/>}</div>
        : <div className={styles.bookRow} key={entry.id}><button onClick={() => open(entry)}><span className={styles.thumb}><Artwork source={binding.context.artwork.foreground(entry.posterUrl, 132, 1)} alt=""/></span><span><b>{entry.displayTitle ?? entry.title}</b>{entry.narrators?.length ? <small>{t('next.kit.readBy', {name: serviceI18n().list(entry.narrators)})}</small> : null}{entry.bookProgress?.remainingMs !== undefined && <small>{t('title.resumeRemaining', {remaining: duration(entry.bookProgress.remainingMs / 1000)!})}</small>}</span></button>{onMore && <Button variant="ghost" icon="i-more" iconOnly aria-label={t('card.more', {title: entry.title})} onClick={event => run(() => {binding.current(page);onMore(entry, event.currentTarget);})}/>}</div>)}
      {rows.nextCursor && <Button size="small" onClick={() => run(() => {binding.current(page);return binding.service.next(rows.id);})}>{t('title.nextPage')}</Button>}
    </section>}
    {more?.page === page && <Menu open onOpenChange={value => {if (!value) setMore(undefined);}} title={more.entry.title} anchorRef={{current: more.anchor}}>
      {effects.addToPlaylist && <MenuItem onSelect={() => invoke(effects.addToPlaylist)}>{t('entry.addToPlaylist')}</MenuItem>}
      {effects.addToCollection && <MenuItem onSelect={() => invoke(effects.addToCollection)}>{t('entry.addToCollection')}</MenuItem>}
      {localOwner && effects.deleteTitle && <MenuItem onSelect={() => invoke(effects.deleteTitle)}>{t('entry.delete')}</MenuItem>}
    </Menu>}
    {!book && <section><div className={styles.sectionHead}><h2>{t('next.controls.details')}</h2></div><dl className={styles.details}>{entity.narrators?.length ? <><dt>{t('filter.field.narrator')}</dt><dd>{serviceI18n().list(entity.narrators)}</dd></> : null}{entity.duration !== undefined && <><dt>{t('next.kit.length')}</dt><dd>{duration(entity.duration)}</dd></>}</dl></section>}
  </article>;
}
function Chapter({entry, index, page, binding}: {entry: ContentEntry; index: number; page: ContentProjection; binding: AudiobookBindings}) {
  const resume = page.listening?.resume;
  const current = !!resume && entry.playback?.itemId === resume.itemId && resume.positionSeconds >= (entry.playback.startSeconds ?? 0) && entry.duration !== undefined && resume.positionSeconds < (entry.playback.startSeconds ?? 0) + entry.duration;
  const fraction = current && resume && entry.duration ? (resume.positionSeconds - (entry.playback?.startSeconds ?? 0)) / entry.duration : undefined;
  return <button className={styles.chapter} data-current={current || undefined} disabled={!entry.playback || entry.available === false || entry.missing} onClick={() => runScreenCommand(binding.context, () => binding.play(page, entry))}>
    <span className={styles.number}>{index + 1}</span><span className={styles.chapterTitle}>{entry.title}</span><span className={styles.duration}>{entry.duration === undefined ? undefined : formatClock(entry.duration)}</span>
    {fraction !== undefined && <span className={styles.chapterProgress}><i style={{width: `${Math.min(1, Math.max(0, fraction)) * 100}%`}}/></span>}
  </button>;
}
