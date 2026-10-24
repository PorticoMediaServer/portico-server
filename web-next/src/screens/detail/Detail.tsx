import {t, locale, environment as env} from './text';
import {useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
import type {DetailProjection, FileInfoProjection} from '@core/detail.ts';
import type {ContentEntry} from '@core/library-content.ts';
import type {PlaybackOptions} from '@core/playback-v1/types.ts';
import {detailActions} from '@core/presentation/detail-actions.ts';
import {titleRecommendationRows, type ItemRecommendations} from '@core/recommendations.ts';
import {serverTextLabel} from '@core/presentation/server-text.ts';
import {serviceI18n} from '@core/presentation/service-text.ts';
import {fileInfoModel, titleFactParts} from '@core/presentation/kit-model';
import {preferenceValue} from '@core/preferences.ts';
import {AmbientArt, PersonCircle} from '../../kit/artwork';
import {Button, ResumeButton, Tabs} from '../../kit/controls';
import {Lockup, Shelf} from '../../kit/layout';
import {PosterCard, SquareCard} from '../../kit/cards';
import {Menu, MenuItem, FileInfo, Dialog, type FileInfoAction} from '../../kit/overlays';
import {StateView} from '../../kit/states';
import {Icon} from '../../kit/Icon';
import {useShellWidth} from '../../shell/viewport';
import {entryCard, fileFacts, titlePageFacts, calendarDate, instantDate} from './model';
import type {DetailBindings} from './bindings';
import styles from './detail.module.css';
import {dependsOn, titlesWithPersonalState} from '@core/freshness-hub.ts';
import {useFreshDocument} from '../../platform/freshness';
import {TitleProse, TitleFactsLine} from '../../kit/title/TitleParts';
import {TitlePeople, CreditLine, CreditNames} from './People';
import {FileManagement} from './FileManagement';
import {TitleTabs} from './TitleTabs';
import {ScreenProblem} from './ScreenProblem';
import {runScreenCommand} from './context';
import {DetailLoading} from './Loading';
import {GenreNames} from './GenreNames';
export {DetailLoading} from './Loading';

export type DetailScreenProps = {binding: DetailBindings; itemId: string; libraryId?: string};
export function DetailScreen({binding, itemId, libraryId}: DetailScreenProps) {
  const [failure, setFailure] = useState<unknown>(), [attempt, setAttempt] = useState(0);
  useEffect(() => { const request = new AbortController(); setFailure(undefined); void binding.select(libraryId ? {itemId, libraryId} : {itemId}, request.signal).catch(error => { if (!request.signal.aborted) setFailure(error); }); return () => { request.abort(); binding.cancel(); }; }, [binding, itemId, libraryId, attempt]);
  return failure ? <ScreenProblem context={binding.context} error={failure} retry={() => setAttempt(value => value + 1)} /> : <TitleBody binding={binding} />;
}

/** Also mounted under the retained phone/tablet Watch surface. It owns no route or video. */
export function TitleBody({binding, compact = false}: {binding: DetailBindings; compact?: boolean}) {
  const snapshot = useSyncExternalStore(binding.service.subscribe, binding.service.getSnapshot, binding.service.getSnapshot);
  useFreshDocument(() => [dependsOn.libraries, ...titlesWithPersonalState(binding.service.getSnapshot().target?.libraryId), dependsOn.event('stream.resync')], refresh => binding.service.revalidate(refresh), () => binding.service.getSnapshot().phase === 'ready');
  const preferences = useSyncExternalStore(binding.context.preferences.subscribe, binding.context.preferences.getSnapshot, binding.context.preferences.getSnapshot);
  const data = snapshot.data, {tab} = useSyncExternalStore(binding.view.subscribe, binding.view.getSnapshot, binding.view.getSnapshot);
  const recommendations = useSyncExternalStore(binding.recommendations.subscribe, binding.recommendations.getSnapshot, binding.recommendations.getSnapshot);
  useEffect(() => { if (data?.item.kind === 'movie') void binding.recommendations.select(data, () => binding.service.getSnapshot().data === data); }, [binding, data]);
  if (!data) return snapshot.phase === 'error' ? <ScreenProblem context={binding.context} error={snapshot.error} subject={binding.subject()} retry={() => { void binding.service.retry(); }} /> : <DetailLoading key={snapshot.target?.itemId} />;
  const tabs = [{value: 'related', label: t('next.controls.moreLikeThis')}, {value: 'details', label: t('next.controls.details')}, ...(data.extras?.some(extra => extra.items.length) ? [{value: 'extras', label: t('next.controls.extras')}] : [])];
  return <article className={styles.page} data-compact={compact || undefined} data-title-id={data.scope.itemId}>
    {!compact && <TitleHero binding={binding} data={data} showBackdrops={preferenceValue<boolean>(preferences, 'appearance.showBackdrops')} />}
    {compact && <div className={styles.compactHeader}><h1>{data.item.title}</h1><TitleActions binding={binding} data={data} /></div>}
    <TitleTabs title={data.item.title} compact={compact} onBack={binding.context.effects.back ? () => runScreenCommand(binding.context, () => { binding.current(data); binding.context.effects.back?.(); }) : undefined} actions={!compact && <TitleActions binding={binding} data={data} small />}><Tabs className={styles.tabStrip} aria-label={t('next.controls.details')} value={tab} onValueChange={value => runScreenCommand(binding.context, () => { binding.current(data); if (value === 'related' || value === 'details' || value === 'extras') binding.view.tab(value); })} items={tabs} /></TitleTabs>
    {snapshot.phase === 'error' && <ScreenFailure retry={() => { void binding.service.retry(); }} />}
    {snapshot.mutationError && <ScreenFailure retry={() => { void binding.service.retryMutation(); }} />}
    <div key={tab} className={tab === 'details' ? styles.tabPanel : `${styles.tabPanel} ${styles.rowsPanel}`}>{tab === 'details' ? <TitleDetails binding={binding} data={data} /> : tab === 'extras' ? data.extras?.map(extra => <TitleRow key={extra.type} heading={extra.label} entries={extra.items} binding={binding} />)
      : recommendations.data ? titleRecommendationRows(recommendations.data).map(row => <TitleRecommendationRow key={row.id} row={row} binding={binding} subtitleVisible={!compact} guard={() => binding.current(data)} />)
        : data.related?.rows.map(row => <TitleRow key={row.id} heading={row.heading} entries={row.entries} binding={binding} guard={() => binding.current(data)} />)}</div>
  </article>;
}

function TitleHero({binding, data, showBackdrops}: {binding: DetailBindings; data: DetailProjection; showBackdrops: boolean}) {
  const width = useShellWidth(), phone = width < 1024, {item, facts, metadata} = data, art = binding.context.artwork;
  const directed = metadata.credits.filter(credit => credit.role.toLowerCase() === 'director');
  const starring = metadata.credits.filter(credit => credit.department.toLowerCase() === 'acting' || credit.role.toLowerCase() === 'cast').slice(0, 4);
  const parts = titlePageFacts({year: item.year, durationSeconds: item.duration, genres: metadata.genres.map(genre => genre.name), contentRating: facts?.contentRating}, env);
  const genres = <GenreNames names={metadata.genres.map(value => value.name)} onOpen={binding.context.effects.openBrowse ? genre => runScreenCommand(binding.context, () => binding.browseGenre(data, genre)) : undefined} />;
  if (phone) {
    const phoneParts = [...parts.filter(part => part.id === 'year' || part.id === 'genres'), ...parts.filter(part => part.id !== 'year' && part.id !== 'genres')];
    return <header className={styles.phoneHero}>
      <div className={styles.phonePicture}>
        {showBackdrops && <AmbientArt className={styles.phoneHeroArt} treatment="phone" source={art.ambient(item.backdropUrl ?? item.posterUrl)} foreground={art.foreground(item.backdropUrl ?? item.posterUrl, 800, 16 / 9)} position="22% 30%" />}
        <div className={styles.phoneCopy}><Lockup title={item.title} long platform="phone-web" /><TitleFactsLine parts={phoneParts} genres={genres} /></div>
      </div>
      <TitleActions binding={binding} data={data} />
      {item.overview && <TitleProse variant="hero" className={styles.phoneSynopsis}>{item.overview}</TitleProse>}
    </header>;
  }
  return <header className={styles.hero}>
    {showBackdrops && <AmbientArt className={styles.heroArt} treatment={phone ? 'phone' : 'title'} source={art.ambient(item.backdropUrl ?? item.posterUrl)} foreground={art.foreground(item.backdropUrl ?? item.posterUrl, phone ? 800 : 1920, 16 / 9)} />}
    <div className={styles.copy}>
      {data.collections?.[0] && <CollectionLink collection={data.collections[0]} onOpen={() => runScreenCommand(binding.context, () => { binding.current(data); binding.context.effects.openEntry({id: data.collections![0].id, libraryId: data.scope.libraryId, kind: 'collection', title: data.collections![0].name, navigation: {view: 'collection', entityId: data.collections![0].id}}); })} />}
      <Lockup title={item.title} platform={phone ? 'phone-web' : 'desktop'} />
      <TitleFactsLine parts={parts} genres={genres} />
      {facts?.tagline && <p className={styles.tagline}>{facts.tagline}</p>}
      {item.overview && <TitleProse variant="hero" className={styles.blurb}>{item.overview}</TitleProse>}
      {(directed.length > 0 || starring.length > 0) && <div className={styles.credits}>{directed.length > 0 && <CreditLine message="title.directedBy" credits={directed} binding={binding} data={data} />}{starring.length > 0 && <CreditLine message="title.starring" credits={starring} binding={binding} data={data} />}</div>}
      <TitleActions binding={binding} data={data} />
    </div>
  </header>;
}

/** The hero's collection link. Its total is every visible member the server counted, not
    only films, so the count keeps that real total and names no member kind: the catalogue's
    neutral collection noun already used for a collection's own entry count. */
export function CollectionLink({collection, onOpen}: {collection: NonNullable<DetailProjection['collections']>[number]; onOpen: () => void}) {
  return <button className={styles.collection} type="button" onClick={onOpen}><Icon name="i-layers" size={18} />{collection.name}<span className={styles.collectionCount}>{t('lib.count', {count: collection.memberCount, pivot: 'other'})}</span></button>;
}

export function TitleActions({binding, data, small = false, topResult = false}: {binding: DetailBindings; data: DetailProjection; small?: boolean; topResult?: boolean}) {
  const phone = useShellWidth() < 1024 && !topResult;
  const model = detailActions(data, {platform: 'web'}), primary = model.primary;
  const primaryAction = data.actions.find(action => action.enabled && action.playback && action.id !== 'start_over');
  const restart = data.actions.find(action => action.id === 'start_over' && action.enabled && action.playback);
  const run = (task: Promise<unknown>) => { void task.catch(binding.context.effects.reportError); };
  const size = small ? 'compact' as const : 'hero' as const;
  return <><div className={styles.actions} data-phone-title={phone && !small || undefined}>
    {primary && primaryAction && (primary.id === 'resume' && !phone ? <ResumeButton size={size} progress={data.item.duration > 0 ? data.personal.progressSeconds / data.item.duration : 0} remaining={primary.caption} onClick={() => run(binding.play(data, primaryAction.id))}>{t('next.controls.resume')}</ResumeButton>
      : <Button variant="primary" icon="i-play" size={size} onClick={() => run(binding.play(data, primaryAction.id))}>{t(primary.id === 'resume' ? 'next.controls.resume' : 'next.controls.play')}</Button>)}
    {!small && <>
      {!phone && !topResult && restart && primary?.id === 'resume' && <Button icon="i-retry" size={size} onClick={() => run(binding.play(data, restart.id))}>{t('next.controls.startOver')}</Button>}
      {data.actions.some(action => action.id === 'watchlist' && action.enabled) && <Button icon={data.personal.watchlisted ? 'i-check' : 'i-plus'} size={size} aria-pressed={data.personal.watchlisted} onClick={() => run(binding.mutate(data, {action: 'watchlist', value: !data.personal.watchlisted}))}>{t('next.controls.myList')}</Button>}
      {!phone && !topResult && data.actions.some(action => action.id === 'watched' && action.enabled) && <Button icon="i-check" iconOnly aria-label={t(data.personal.watched ? 'title.markUnwatched' : 'title.markWatched')} aria-pressed={data.personal.watched} size={size} onClick={() => run(binding.mutate(data, {action: 'watched', value: !data.personal.watched}))} />}
      <TitleMore binding={binding} data={data} />
    </>}
  </div>{phone && !small && primary?.id === 'resume' && <div className={styles.phoneResume}>
    <div className={styles.phoneProgress} role="progressbar" aria-label={t('next.layout.progress')} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(Math.min(1, Math.max(0, data.item.duration > 0 ? data.personal.progressSeconds / data.item.duration : 0)) * 100)}><span style={{transform: `scaleX(${Math.min(1, Math.max(0, data.item.duration > 0 ? data.personal.progressSeconds / data.item.duration : 0))})`}} /></div>
    <div className={styles.phoneRemaining}>{primary.caption}{restart && <Button variant="text" icon="i-retry" size="small" onClick={() => run(binding.play(data, restart.id))}>{t('next.controls.startOver')}</Button>}</div>
  </div>}</>;
}

function TitleMore({binding, data}: {binding: DetailBindings; data: DetailProjection}) {
  const [open, setOpen] = useState(false), [fileOpen, setFileOpen] = useState(false);
  const [files, setFiles] = useState<FileInfoProjection>(), [failed, setFailed] = useState(false);
  const [manage, setManage] = useState<FileInfoAction>();
  const trigger = useRef<HTMLButtonElement>(null), fileRead = useRef<AbortController | undefined>(undefined), width = useShellWidth(), phone = width < 1024;
  const versionRead = useRef<AbortController | undefined>(undefined), versionChoice = useRef<AbortController | undefined>(undefined);
  const [versions, setVersions] = useState<Readonly<{data: DetailProjection; options: PlaybackOptions; request: AbortController}>>();
  useEffect(() => { setOpen(false); setFileOpen(false); setFiles(undefined); setManage(undefined); }, [data]);
  useEffect(() => () => {fileRead.current?.abort();versionRead.current?.abort();versionChoice.current?.abort();}, [data]);
  useEffect(() => {
    if (!open || !binding.context.effects.chooseVersion || !data.actions.some(action => action.enabled && action.playback?.itemId === data.item.id && action.id !== 'start_over')) return;
    const request = new AbortController();versionRead.current = request;setVersions(undefined);
    void binding.readVersions(data, request.signal).then(options => {if (!request.signal.aborted) setVersions({data, options, request});}, error => {
      if (!request.signal.aborted) binding.context.effects.reportError(error);
    });
    return () => {if (versionChoice.current !== request) request.abort();};
  }, [binding, data, open]);
  const chooseVersion = () => {
    if (!versions || versions.data !== data || versions.request.signal.aborted) return;
    versionChoice.current?.abort();versionChoice.current = versions.request;
    void binding.chooseVersion(data, versions.options, versions.request.signal, trigger).catch(binding.context.effects.reportError).finally(() => {
      versions.request.abort();if (versionChoice.current === versions.request) versionChoice.current = undefined;
    });
  };
  const loadFiles = () => {
    fileRead.current?.abort(); const controller = new AbortController(); fileRead.current = controller;
    setFileOpen(true); setFailed(false);
    void binding.readFileInfo(data, undefined, controller.signal).then(value => { if (!controller.signal.aborted) setFiles(value); }, error => { if (!controller.signal.aborted) { setFailed(true); binding.context.effects.reportError(error); } });
  };
  const closeFiles = (value: boolean) => { setFileOpen(value); if (!value) fileRead.current?.abort(); };
  const model = useMemo(() => files && fileInfoModel(files.files.map(fileFacts), files.manage ? {audience: 'owner', manageSurface: phone ? 'phone' : 'desktop', ownerFacts: {locations: files.files.map(file => ({id: file.id, path: file.path, canDelete: files.manage?.delete})), canSplit: files.manage.split && files.files.length >= 2, canMerge: phone && files.manage.merge}} : {audience: 'viewer'}, env, phone ? 'phone' : 'desktop'), [files, phone]);
  return <>
    <Menu open={open} onOpenChange={setOpen} title={data.item.title} trigger={<Button ref={trigger} icon="i-more" iconOnly aria-label={t('next.controls.more')} />}>
      {phone && data.actions.some(action => action.id === 'watched' && action.enabled) && <MenuItem icon="i-check" selected={data.personal.watched} onSelect={() => { void binding.mutate(data, {action: 'watched', value: !data.personal.watched}).catch(binding.context.effects.reportError); }}>{t(data.personal.watched ? 'title.markUnwatched' : 'title.markWatched')}</MenuItem>}
      {data.actions.some(action => action.id === 'favorite' && action.enabled) && <MenuItem icon="i-heart" selected={data.personal.favorite} onSelect={() => { void binding.mutate(data, {action: 'favorite', value: !data.personal.favorite}).catch(binding.context.effects.reportError); }}>{t(data.personal.favorite ? 'entry.removeFromFavorites' : 'entry.addToFavorites')}</MenuItem>}
      {versions?.data === data && !versions.request.signal.aborted && versions.options.versions.length > 1 && <MenuItem icon="i-film" onSelect={chooseVersion}>{t('next.overlays.differentVersion')}</MenuItem>}
      <MenuItem icon="i-info" onSelect={loadFiles}>{t('next.overlays.fileInfo')}</MenuItem>
    </Menu>
    {fileOpen && model && <FileInfo model={model} subject={data.item.title} layout={phone ? 'phone' : 'desktop'} open onOpenChange={closeFiles} returnFocusRef={trigger} onManage={action => runScreenCommand(binding.context, () => { binding.current(data); closeFiles(false); setManage(action); })} />}
    {fileOpen && !model && <Dialog open onOpenChange={closeFiles} title={t('next.overlays.fileInfo')} returnFocusRef={trigger}>{failed ? <ScreenFailure retry={loadFiles} /> : <DetailLoading shape="panel" />}</Dialog>}
    {manage && files && <FileManagement binding={binding} data={data} files={files} action={manage} onClose={() => setManage(undefined)} returnFocusRef={trigger} />}
  </>;
}

export function TitleDetails({binding, data}: {binding: DetailBindings; data: DetailProjection}) {
  const {item, facts, metadata} = data;
  const language = (code: string) => { try { return new Intl.DisplayNames(locale(), {type: 'language'}).of(code) ?? code; } catch { return code; } };
  const directors = metadata.credits.filter(credit => credit.role.toLowerCase() === 'director'), writers = metadata.credits.filter(credit => /writer|writing|screenplay|teleplay|novel/i.test(credit.role));
  const rows: (readonly [string, ReactNode])[] = [
    [t('title.releaseDate'), calendarDate(facts?.releaseDate, locale())],
    [t('library.column.runtime'), titleFactParts({durationSeconds: item.duration}, env).find(part => part.id === 'duration')?.text],
    [t('title.genres'), metadata.genres.length ? <GenreNames names={metadata.genres.map(value => value.name)} onOpen={binding.context.effects.openBrowse ? genre => runScreenCommand(binding.context, () => binding.browseGenre(data, genre)) : undefined} /> : undefined],
    [t('title.directedByLabel'), directors.length ? <CreditNames binding={binding} data={data} credits={directors} /> : undefined],
    [t('title.writtenByLabel'), writers.length ? <CreditNames binding={binding} data={data} credits={writers} /> : undefined],
    [t('title.studio'), facts?.studio], [t('title.network'), facts?.network], [t('title.country'), facts?.country], [t('title.edition'), facts?.edition],
    [t('next.selection.language'), facts?.originalLanguage ? language(facts.originalLanguage) : undefined],
    [t('title.audio'), data.languages.audio.map(language).join(', ')], [t('title.subtitles'), data.languages.subtitles.map(language).join(', ')],
    [t('title.added'), instantDate(item.addedAt, locale())],
  ];
  return <div className={styles.details}><section><dl>{rows.filter(row => !!row[1]).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl></section>
    {metadata.credits.length > 0 && <TitlePeople binding={binding} data={data} />}
  </div>;
}

export function TitleRecommendationRow({row, binding, guard, subtitleVisible = true}: {row: ItemRecommendations['rows'][number]; binding: Pick<DetailBindings, 'context'>; guard?: () => void; subtitleVisible?: boolean}) {
  const i18n = serviceI18n();
  return <TitleRow heading={serverTextLabel(i18n, row.titleText, row.title)} subtitle={subtitleVisible ? serverTextLabel(i18n, row.subtitleText) : undefined} entries={row.entries} binding={binding} guard={guard} />;
}

export function TitleRow({heading, subtitle, entries, binding, guard}: {heading: ReactNode; subtitle?: ReactNode; entries: readonly ContentEntry[]; binding: Pick<DetailBindings, 'context'>; guard?: () => void}) {
  const phone = useShellWidth() < 1024;
  return <Shelf heading={heading} subtitle={phone ? undefined : subtitle} items={entries} getKey={entry => entry.id} renderItem={entry => {
    const props = {model: entryCard(entry, env), artwork: binding.context.artwork.foreground(entry.posterUrl, 380, 2 / 3), onNavigate: () => runScreenCommand(binding.context, () => { if (binding.context.isCurrent()) { guard?.(); binding.context.effects.openEntry(entry); } }), watched: entry.watched};
    return ['album', 'book'].includes(entry.kind) ? <SquareCard {...props} /> : <PosterCard {...props} />;
  }} />;
}
export function ScreenFailure({retry}: {retry: () => void}) { return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: retry}} />; }
