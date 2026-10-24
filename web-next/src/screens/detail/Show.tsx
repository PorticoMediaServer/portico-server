import {t, locale, environment as env} from './text';
import {Fragment, useEffect, useState, useSyncExternalStore, type ReactNode} from 'react';
import {preferenceValue} from '@core/preferences.ts';
import type {ShowWorkspaceRoute, ShowWorkspaceData, ShowSettings, WorkspaceShowCredit} from '@core/show-workspace.ts';
import {cardText, episodeLabel, titleFactParts, type FactPart} from '@core/presentation/kit-model';
import {Button, Chip, ResumeButton, Tabs, Segmented, Switch} from '../../kit/controls';
import {AmbientArt} from '../../kit/artwork';
import {Lockup} from '../../kit/layout';
import {EpisodeTile} from '../../kit/cards';
import {Menu, MenuItem, Popover} from '../../kit/overlays';
import {OwnerTag, ServerMark} from '../../kit/shell';
import {StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {DetailLoading, ScreenFailure, TitleActions, TitleRow} from './Detail';
import {titleRecommendationRows} from '@core/recommendations.ts';
import {SHOW_SETTING_ROWS} from '@core/presentation/title-page.ts';
import {episodeCode} from '@core/presentation/card-caption.ts';
import {entrySubject, titlePageFacts, calendarDate, instantDate} from './model';
import type {ShowBindings} from './show-bindings';
import {showDetailFacts} from './show-bindings';
import styles from './show.module.css';
import detail from './detail.module.css';
import {dependsOn, titlesWithPersonalState} from '@core/freshness-hub.ts';
import {useFreshDocument} from '../../platform/freshness';
import {EpisodeDetailsLayout, TitleProse, TitleFactsLine} from '../../kit/title/TitleParts';
import {TitlePeople, ShowPeople} from './People';
import {TitleTabs} from './TitleTabs';
import {ScreenProblem} from './ScreenProblem';
import {runScreenCommand} from './context';
import {GenreNames} from './GenreNames';

export type ShowScreenProps = {binding: ShowBindings; target: ShowWorkspaceRoute};
export function ShowScreen({binding, target}: ShowScreenProps) {
  const key = JSON.stringify(target);
  useEffect(() => { const route: ShowWorkspaceRoute = JSON.parse(key), request = new AbortController(); void binding.select(route, request.signal).catch(error => { if (!request.signal.aborted) binding.context.effects.reportError(error); }); return () => { request.abort(); binding.cancel(); }; }, [binding, key]);
  return <ShowBody binding={binding} />;
}

/** One body below Watch's video as well as on the title route. No independent video host. */
export function ShowBody({binding, compact = false}: {binding: ShowBindings; compact?: boolean}) {
  const snapshot = useSyncExternalStore(binding.service.subscribe, binding.service.getSnapshot, binding.service.getSnapshot);
  useFreshDocument(() => [dependsOn.libraries, ...titlesWithPersonalState(binding.service.getSnapshot().route?.libraryId), dependsOn.event('stream.resync')], refresh => binding.revalidate(refresh), () => binding.service.getSnapshot().phase === 'ready');
  const preferences = useSyncExternalStore(binding.context.preferences.subscribe, binding.context.preferences.getSnapshot, binding.context.preferences.getSnapshot);
  const personal = useSyncExternalStore(binding.personal.subscribe, binding.personal.getSnapshot, binding.personal.getSnapshot);
  const busy = useSyncExternalStore(binding.personal.subscribe, binding.personal.getBusy, binding.personal.getBusy);
  const primary = useSyncExternalStore(binding.primary.service.subscribe, binding.primary.service.getSnapshot, binding.primary.service.getSnapshot);
  const {tab, layout} = useSyncExternalStore(binding.view.subscribe, binding.view.getSnapshot, binding.view.getSnapshot), [seasonMenu, setSeasonMenu] = useState(false);
  const width = useShellWidth(), phone = width < 1024, data = snapshot.data;
  const recommendations = useSyncExternalStore(binding.recommendations.subscribe, binding.recommendations.getSnapshot, binding.recommendations.getSnapshot);
  useEffect(() => { if (data && tab === 'related') void binding.recommendations.select(data, () => binding.service.getSnapshot().data === data); }, [binding, data, tab]);
  if (!data) return snapshot.phase === 'error' ? <ScreenProblem context={binding.context} error={snapshot.error} subject={binding.subject()} retry={() => { void binding.service.retry(); }} /> : <DetailLoading key={snapshot.route?.showId ?? snapshot.route?.episodeId ?? snapshot.route?.seasonId} />;
  const {show} = data, selectedSeason = data.seasons.find(season => season.id === data.selected.seasonId);
  const episodes = data.episodes.sections[0]?.entries ?? [], art = binding.context.artwork;
  const actions = <div className={detail.actions}>
    {primary.data && <TitleActions binding={binding.primary} data={primary.data} small />}
    {personal && <Button disabled={busy} icon={personal.watchlisted ? 'i-check' : 'i-plus'} aria-pressed={personal.watchlisted} onClick={() => { void binding.setMyList(data, !personal.watchlisted).catch(binding.context.effects.reportError); }}>{t('next.controls.myList')}</Button>}
    {personal && <Button disabled={busy} icon="i-check" iconOnly aria-label={t(personal.watched ? 'title.markUnwatched' : 'title.markWatched')} aria-pressed={personal.watched} onClick={() => { void binding.setWatched(data, !personal.watched).catch(binding.context.effects.reportError); }} />}
  </div>;
  // The pinned bar carries the drawn Resume Sx Ey and My List controls once the hero is gone.
  const pinAction = primary.data?.actions.find(action => action.enabled && action.playback && action.id !== 'start_over'), nextUp = data.nextUp;
  const pinProgress = nextUp?.duration && nextUp.duration > 0 ? Math.min(1, Math.max(0, (nextUp.progressSeconds ?? 0) / nextUp.duration)) : 0;
  const pin = nextUp?.seasonNumber !== undefined && nextUp.episodeNumber !== undefined && primary.data && pinAction
    ? <ResumeButton size="small" progress={pinProgress} onClick={() => { const held = primary.data; if (held) void binding.primary.play(held, pinAction.id).catch(binding.context.effects.reportError); }}>{t('next.home.resumeEpisode', {season: nextUp.seasonNumber, episode: nextUp.episodeNumber})}</ResumeButton>
    : primary.data ? <TitleActions binding={binding.primary} data={primary.data} small /> : null;
  const owner = binding.canManage();
  const facts = titlePageFacts({year: show.year, seasonCount: data.seasonTotalCount, genres: show.genres, contentRating: show.contentRating}, env);
  // Phone frames always list episodes; retain the desktop choice across rotation.
  const episodeLayout = phone ? 'list' : layout, columns = 4;
  return <article className={detail.page} data-show-id={show.id} data-compact={compact || undefined}>
    {!compact ? <header className={detail.hero}>
      {preferenceValue<boolean>(preferences, 'appearance.showBackdrops') && <AmbientArt className={detail.heroArt} treatment={phone ? 'phone' : 'title'} source={art.ambient(show.backdropUrl ?? show.posterUrl)} foreground={art.foreground(show.backdropUrl ?? show.posterUrl, phone ? 800 : 1920, 16 / 9)} />}
      <div className={detail.copy}><Lockup title={show.title} platform={phone ? 'phone-web' : 'desktop'} /><TitleFactsLine parts={facts} genres={<GenreNames names={show.genres ?? []} onOpen={binding.context.effects.openBrowse ? genre => runScreenCommand(binding.context, () => binding.browseGenre(data, genre)) : undefined} />} />{show.overview && <TitleProse variant="hero" className={detail.blurb}>{show.overview}</TitleProse>}{actions}</div>
    </header> : <div className={detail.compactHeader}><h1>{show.title}</h1>{actions}</div>}
    <TitleTabs title={show.title} compact={compact} onBack={binding.context.effects.back ? () => runScreenCommand(binding.context, () => { binding.current(data); binding.context.effects.back?.(); }) : undefined} actions={!compact ? <div className={`${detail.actions} ${styles.pinActions}`}>{pin}{personal && <Button size="small" disabled={busy} icon={personal.watchlisted ? 'i-check' : 'i-plus'} iconOnly aria-label={t('next.controls.myList')} title={t('next.controls.myList')} aria-pressed={personal.watchlisted} onClick={() => { void binding.setMyList(data, !personal.watchlisted).catch(binding.context.effects.reportError); }} />}</div> : undefined}><Tabs value={tab} onValueChange={value => runScreenCommand(binding.context, () => { binding.current(data); if (value === 'episodes' || value === 'related' || value === 'details') binding.view.tab(value); })} items={[{value: 'episodes', label: t('next.controls.episodes')}, {value: 'related', label: t('next.controls.moreLikeThis')}, {value: 'details', label: t('next.controls.details')}]} /></TitleTabs>
    <div key={tab} className={detail.tabPanel}>
    {tab === 'episodes' && <section className={styles.episodes}>
      <div className={styles.toolbar}>
        <Menu open={seasonMenu} onOpenChange={setSeasonMenu} title={t('title.seasonMenu')} trigger={<Chip variant="toolbar" trailingIcon="i-down">{selectedSeason ? selectedSeason.number === 0 ? t('next.kit.special') : t('next.kit.season', {number: selectedSeason.number}) : data.groups.find(group => group.id === data.selected.group)?.title ?? t('next.controls.episodes')}</Chip>}>
          {data.seasons.map(season => <MenuItem key={season.id} selected={season.id === data.selected.seasonId} description={season.episodeCount !== undefined ? t('count.episodes', {count: season.episodeCount}) : undefined} onSelect={() => runScreenCommand(binding.context, () => { binding.current(data); return binding.service.selectSeason(season.id); })}>{season.number === 0 ? t('next.kit.special') : t('next.kit.season', {number: season.number})}</MenuItem>)}
          {data.groups.map(group => <MenuItem key={group.id} selected={group.id === data.selected.group} onSelect={() => runScreenCommand(binding.context, () => { binding.current(data); return binding.service.selectGroup(group.id); })}>{group.title}</MenuItem>)}
          {snapshot.seasonPagination.canPrevious && <MenuItem onSelect={() => runScreenCommand(binding.context, () => { binding.current(data); return binding.service.previousSeasons(); })}>{t('title.earlierSeasons')}</MenuItem>}
          {snapshot.seasonPagination.nextCursor && <MenuItem onSelect={() => runScreenCommand(binding.context, () => { binding.current(data); return binding.service.nextSeasons(); })}>{t('title.laterSeasons')}</MenuItem>}
        </Menu>
        {selectedSeason?.episodeCount !== undefined && selectedSeason.watchedCount !== undefined && <span className={styles.progress}>{t('title.seasonProgress', {watched: selectedSeason.watchedCount, total: selectedSeason.episodeCount})}</span>}
        <label className={styles.hide}>{t('next.controls.hideWatched')}<Switch checked={snapshot.route?.playState === 'unplayed'} onCheckedChange={checked => runScreenCommand(binding.context, () => { binding.current(data); const route = snapshot.route; if (route) return binding.select({...route, playState: checked ? 'unplayed' : 'all'}); })} /></label>
        {!phone && <Segmented value={layout} onValueChange={value => runScreenCommand(binding.context, () => { binding.current(data); if (value === 'list' || value === 'grid') binding.view.layout(value); })} aria-label={t('title.episodeLayout')} variant="view" items={[{value: 'list', label: t('title.viewList'), icon: 'i-list'}, {value: 'grid', label: t('title.viewGrid'), icon: 'i-grid'}]} />}
      </div>
      {snapshot.phase === 'error' || snapshot.phase === 'refresh-required' ? <ScreenFailure retry={() => { void binding.service.retry(); }} /> : null}
      {!episodes.length && snapshot.phase === 'ready' && <StateView title={t('title.noEpisodes')} body={t('title.noEpisodesBody')} />}
      <div className={styles.tiles} data-layout={episodeLayout} style={{'--episode-columns': columns} as React.CSSProperties}>
        {episodes.map((episode, index) => {
          const expanded = snapshot.anchor.expandedEpisodeId === episode.id, rowEnd = episodeLayout === 'list' ? index : Math.min(episodes.length - 1, Math.floor(index / columns) * columns + columns - 1);
          const opened = episodes.slice(episodeLayout === 'list' ? index : Math.floor(index / columns) * columns, rowEnd + 1).find(entry => entry.id === snapshot.anchor.expandedEpisodeId);
          const subject = entrySubject(episode), model = cardText(subject, 'in-show', env), next = data.nextUp?.id === episode.id;
          const date = subject.episode?.mode !== 'date' && subject.episode?.mode !== 'special' ? episodeLabel({mode: 'date', day: episode.airDate}, 'in-show', env) : undefined;
          const continuing = !episode.watched && (episode.progressSeconds ?? 0) > 0;
          const status: FactPart[] = continuing ? [{id: 'continue', text: t('title.continueEpisode'), role: 'status'}, ...cardText(subject, 'continuation', env).detail] : next ? [{id: 'up-next', text: t('title.nextUp'), role: 'status'}] : [];
          return <Fragment key={episode.id}><EpisodeTile className={styles.episodeRow} model={{...model, caption: [...model.caption, ...status]}} length={model.detail.find(part => part.id === 'duration')?.text} airDate={!continuing && date ? {...date, id: 'air-date'} : undefined} context="detail" artworkKind={episode.stillUrl ? 'still' : 'backdrop'} artwork={art.foreground(episode.stillUrl ?? episode.backdropUrl ?? show.backdropUrl, 632, 16 / 9)} watched={episode.watched} expanded={expanded} next={next} description={episode.overview}
            onPlay={episode.available === true && episode.playback?.itemId === episode.id ? () => runScreenCommand(binding.context, () => binding.playEpisode(data, episode)) : undefined}
            onNavigate={() => { void binding.expand(data, episode).catch(binding.context.effects.reportError); }} />
            {rowEnd === index && opened && <div className={styles.expanded}><EpisodeBody binding={binding} data={data} /></div>}
          </Fragment>;
        })}
      </div>
      <div className={styles.paging}>{snapshot.pagination.canPrevious && <Button size="compact" onClick={() => runScreenCommand(binding.context, () => { binding.current(data); return binding.service.previousEpisodes(); })}>{t('title.previousPage')}</Button>}{snapshot.pagination.nextCursor && <Button size="compact" onClick={() => runScreenCommand(binding.context, () => { binding.current(data); return binding.service.nextEpisodes(); })}>{t('title.nextPage')}</Button>}</div>
    </section>}
    {tab === 'details' && <ShowDetails binding={binding} data={data} owner={owner} />}
    {tab === 'related' && (recommendations.phase === 'error' ? <ScreenFailure retry={() => { void binding.recommendations.select(data, () => binding.service.getSnapshot().data === data); }} /> : recommendations.phase === 'loading' ? <DetailLoading /> : titleRecommendationRows(recommendations.data).map(row => <TitleRow key={row.id} binding={binding} heading={row.title} entries={row.entries} guard={() => binding.current(data)} />))}
    </div>
  </article>;
}

export function EpisodeBody({binding, data}: {binding: ShowBindings; data: ShowWorkspaceData}) {
  const snapshot = useSyncExternalStore(binding.episode.service.subscribe, binding.episode.service.getSnapshot, binding.episode.service.getSnapshot), episode = snapshot.data;
  if (!episode) return snapshot.phase === 'error' ? <ScreenFailure retry={() => { void binding.episode.service.retry(); }} /> : <DetailLoading shape="episode" />;
  const adjacent = <div className={styles.adjacent}>{(['previous', 'next'] as const).map(direction => { const target = episode.episodeDetails?.[direction]; return target && <Button key={direction} variant="text" size="small" aria-label={t(direction === 'previous' ? 'title.previousEpisode' : 'title.nextEpisode')} onClick={() => {
        runScreenCommand(binding.context, () => { binding.current(data); return binding.select({libraryId: data.scope.libraryId, showId: data.show.id, episodeId: target.id}); });
      }}>{target.title}<small>{t('next.kit.episode', {number: target.number})}</small></Button>; })}</div>;
  return <EpisodeDetailsLayout title={episode.item.title} onClose={() => runScreenCommand(binding.context, binding.closeEpisode)} context={<b>{data.show.title}</b>}
    artwork={binding.context.artwork.foreground(episode.item.stillUrl ?? data.show.backdropUrl, 880, 16 / 9)} backdrop={!episode.item.stillUrl}
    facts={<TitleFactsLine parts={titleFactParts({durationSeconds: episode.item.duration, contentRating: episode.episodeDetails?.contentRating}, env)} />}
    synopsis={episode.item.overview} actions={<TitleActions binding={binding.episode} data={episode} />} adjacent={adjacent}
    credits={<TitlePeople binding={binding.episode} data={episode} episode />} />;
}

/** The drawn About column: the show's own stored facts, never a file fact. */
export function ShowDetails({binding, data, owner}: {binding: ShowBindings; data: ShowWorkspaceData; owner: boolean}) {
  const {show} = data, facts = showDetailFacts(show, data.seasonTotalCount, locale());
  const onServer = facts.onServer ? [t('title.season', {number: facts.onServer.seasons}), facts.onServer.episodes !== undefined ? t('count.episodes', {count: facts.onServer.episodes}) : undefined].filter(Boolean).join(', ') : undefined;
  const rows: (readonly [string, ReactNode])[] = [
    [t('title.releaseDate'), calendarDate(facts.firstAired, locale())],
    [t('web.general.status'), facts.status],
    [t('title.network'), facts.network],
    [t('title.studio'), facts.studio],
    [t('title.country'), facts.country],
    [t('title.genres'), facts.genres.length ? <GenreNames names={facts.genres} onOpen={binding.context.effects.openBrowse ? genre => runScreenCommand(binding.context, () => binding.browseGenre(data, genre)) : undefined} /> : undefined],
    [t('title.createdBy', {names: ''}).trim(), facts.creators.length ? <ShowPersonLinks credits={facts.creators} binding={binding} data={data} /> : undefined],
    [t('library.column.rating'), facts.rating],
    [t('next.selection.language'), facts.language],
    [t('next.shell.server'), onServer],
    [t('title.added'), instantDate(facts.addedAt, locale())],
  ];
  return <div className={styles.detailsTab} data-show-details>
    <div className={styles.detailsGrid}>
      <section>
        <h2 className={styles.sectionHeading}>{t('filter.group.title')}</h2>
        <dl className={styles.facts}>{rows.filter(row => !!row[1]).map(([label, value]) => <Fragment key={label}><dt>{label}</dt><dd>{value}</dd></Fragment>)}</dl>
      </section>
      <div className={styles.castBlock}><ShowPeople binding={binding} data={data} /></div>
    </div>
    {owner && <ShowSettingsPanel binding={binding} data={data} />}
  </div>;
}

function ShowPersonLinks({credits, binding, data}: {credits: readonly WorkspaceShowCredit[]; binding: ShowBindings; data: ShowWorkspaceData}) {
  return <>{credits.map((credit, index) => <Fragment key={`${credit.id ?? credit.name}:${credit.ordinal}`}>{index > 0 && ', '}{credit.id
    ? <button type="button" className={styles.personLink} onClick={() => runScreenCommand(binding.context, () => {
      binding.current(data);
      binding.context.effects.openEntry({id: credit.id!, kind: 'person', title: credit.name, navigation: {view: 'person', entityId: credit.id!}});
    })}>{credit.name}</button>
    : credit.name}</Fragment>)}</>;
}

/** The owner's drawn Server panel. It carries no file facts; the four stored settings stay the source. */
export function ShowSettingsPanel({binding, data}: {binding: ShowBindings; data: ShowWorkspaceData}) {
  const [open, setOpen] = useState(false), settings = data.settings;
  const grouping = settings.ranges ? t('title.showSettings.ranges') : t('title.seasons');
  return <section className={styles.owner} aria-label={t('next.shell.ownerOnly')}>
    <div className={styles.ownerHeader}>
      <ServerMark size={34} />
      <div><b>{t('title.showSettings')}<OwnerTag /></b><small>{t('next.overlays.membersNeverSee')}</small></div>
    </div>
    <div className={styles.ownerRow}>
      <div className={styles.ownerCell} data-show-settings>
        <div className={styles.ownerLabel}>{t('title.episodeLayout')}</div>
        <div className={styles.ownerBig}><span>{grouping}</span><span>{settings.newestFirst ? t('next.overlays.newest') : t('next.overlays.oldest')}</span></div>
        <p>{t('title.showSettings.lede')}</p>
        <Popover open={open} onOpenChange={setOpen} placement="top-end" sideOffset={8} alignOffset={-18} width={540} className={styles.settingsMenu}
          title={t('title.showSettings')} trigger={<Chip className={styles.changeChip} trailingIcon="i-down">{t('web.metadataLookup.change')}</Chip>}>
          <ShowSettingsMenu binding={binding} data={data} />
        </Popover>
      </div>
    </div>
  </section>;
}

/** The drawn menu content: the four stored settings and a preview of one real episode. */
export function ShowSettingsMenu({binding, data}: {binding: ShowBindings; data: ShowWorkspaceData}) {
  const settings = data.settings, preview = data.nextUp ?? data.episodes.sections[0]?.entries[0];
  const code = preview ? episodeCode(preview, t) : undefined;
  const save = (key: typeof SHOW_SETTING_ROWS[number]['key'], checked: boolean) => {
    const next = {...settings, [key]: checked} as ShowSettings;
    return binding.setSettings(data, next);
  };
  return <>
    <div className={styles.menuHead}><b>{t('title.episodeLayout')}</b><span className={styles.menuTag}>{t('next.dashboard.everyone')}</span></div>
    <p className={styles.menuLede}>{t('title.showSettings.lede')}</p>
    {SHOW_SETTING_ROWS.map(row => <label key={row.key} className={styles.settingRow}>
      <span><b>{t(row.label)}</b><small>{t(row.help)}</small></span>
      <Switch checked={settings[row.key]} onCheckedChange={checked => runScreenCommand(binding.context, () => save(row.key, checked))} />
    </label>)}
    {preview && <div className={styles.preview}>{code && <b>{code}</b>}<span>{preview.title}</span></div>}
  </>;
}
