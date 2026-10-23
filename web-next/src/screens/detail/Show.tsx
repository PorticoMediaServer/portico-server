import {t, locale, environment as env} from './text';
import {Fragment, useEffect, useState, useSyncExternalStore} from 'react';
import {preferenceValue} from '@core/preferences.ts';
import type {ShowWorkspaceRoute, ShowWorkspaceData} from '@core/show-workspace.ts';
import {titleFactParts} from '@core/presentation/kit-model';
import {Button, Chip, Tabs, Segmented, Switch} from '../../kit/controls';
import {AmbientArt} from '../../kit/artwork';
import {Lockup, HeroFacts} from '../../kit/layout';
import {EpisodeTile} from '../../kit/cards';
import {Menu, MenuItem} from '../../kit/overlays';
import {StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {DetailLoading, ScreenFailure, TitleActions, TitleDetails, TitleRow} from './Detail';
import {titleRecommendationRows} from '@core/recommendations.ts';
import {entryCard, titlePageFacts} from './model';
import type {ShowBindings} from './show-bindings';
import styles from './show.module.css';
import detail from './detail.module.css';
import {dependsOn, titlesWithPersonalState} from '@core/freshness-hub.ts';
import {useFreshDocument} from '../../platform/freshness';
import {EpisodeDetailsLayout, TitleProse, TitleFactsLine} from '../../kit/title/TitleParts';
import {TitlePeople} from './People';
import {TitleTabs} from './TitleTabs';
import {ScreenProblem} from './ScreenProblem';
import {runScreenCommand} from './context';

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
  const facts = titlePageFacts({year: show.year, seasonCount: data.seasonTotalCount, genres: show.genres, contentRating: show.contentRating}, env);
  const columns = phone ? 2 : 4;
  return <article className={detail.page} data-show-id={show.id} data-compact={compact || undefined}>
    {!compact ? <header className={detail.hero}>
      {preferenceValue<boolean>(preferences, 'appearance.showBackdrops') && <AmbientArt className={detail.heroArt} treatment={phone ? 'phone' : 'title'} source={art.ambient(show.backdropUrl ?? show.posterUrl)} foreground={art.foreground(show.backdropUrl ?? show.posterUrl, phone ? 800 : 1920, 16 / 9)} />}
      <div className={detail.copy}><Lockup title={show.title} platform={phone ? 'phone-web' : 'desktop'} /><TitleFactsLine parts={facts} />{show.overview && <TitleProse variant="hero" className={detail.blurb}>{show.overview}</TitleProse>}{actions}</div>
    </header> : <div className={detail.compactHeader}><h1>{show.title}</h1>{actions}</div>}
    <TitleTabs title={show.title} compact={compact}><Tabs value={tab} onValueChange={value => runScreenCommand(binding.context, () => { binding.current(data); if (value === 'episodes' || value === 'related' || value === 'details') binding.view.tab(value); })} items={[{value: 'episodes', label: t('next.controls.episodes')}, {value: 'related', label: t('next.controls.moreLikeThis')}, {value: 'details', label: t('next.controls.details')}]} /></TitleTabs>
    <div key={tab} className={detail.tabPanel}>
    {tab === 'episodes' && <section className={styles.episodes}>
      <div className={styles.toolbar}>
        <Menu open={seasonMenu} onOpenChange={setSeasonMenu} title={t('title.seasonMenu')} trigger={<Chip variant="toolbar" trailingIcon="i-down">{selectedSeason ? selectedSeason.number === 0 ? t('next.kit.special') : t('next.kit.season', {number: selectedSeason.number}) : data.groups.find(group => group.id === data.selected.group)?.title ?? t('next.controls.episodes')}</Chip>}>
          {data.seasons.map(season => <MenuItem key={season.id} selected={season.id === data.selected.seasonId} description={season.episodeCount !== undefined ? t('count.episodes', {count: season.episodeCount}) : undefined} onSelect={() => { void binding.service.selectSeason(season.id).catch(binding.context.effects.reportError); }}>{season.number === 0 ? t('next.kit.special') : t('next.kit.season', {number: season.number})}</MenuItem>)}
          {data.groups.map(group => <MenuItem key={group.id} selected={group.id === data.selected.group} onSelect={() => { void binding.service.selectGroup(group.id).catch(binding.context.effects.reportError); }}>{group.title}</MenuItem>)}
          {snapshot.seasonPagination.canPrevious && <MenuItem onSelect={() => { void binding.service.previousSeasons(); }}>{t('title.earlierSeasons')}</MenuItem>}
          {snapshot.seasonPagination.nextCursor && <MenuItem onSelect={() => { void binding.service.nextSeasons(); }}>{t('title.laterSeasons')}</MenuItem>}
        </Menu>
        {selectedSeason?.episodeCount !== undefined && selectedSeason.watchedCount !== undefined && <span className={styles.progress}>{t('title.seasonProgress', {watched: selectedSeason.watchedCount, total: selectedSeason.episodeCount})}</span>}
        <label className={styles.hide}>{t('next.controls.hideWatched')}<Switch checked={snapshot.route?.playState === 'unplayed'} onCheckedChange={checked => { const route = snapshot.route; if (route) void binding.select({...route, playState: checked ? 'unplayed' : 'all'}).catch(binding.context.effects.reportError); }} /></label>
        <Segmented value={layout} onValueChange={value => runScreenCommand(binding.context, () => { binding.current(data); if (value === 'list' || value === 'grid') binding.view.layout(value); })} aria-label={t('title.episodeLayout')} variant="view" items={[{value: 'list', label: t('title.viewList'), icon: 'i-list'}, {value: 'grid', label: t('title.viewGrid'), icon: 'i-grid'}]} />
      </div>
      {snapshot.phase === 'error' || snapshot.phase === 'refresh-required' ? <ScreenFailure retry={() => { void binding.service.retry(); }} /> : null}
      {!episodes.length && snapshot.phase === 'ready' && <StateView title={t('title.noEpisodes')} body={t('title.noEpisodesBody')} />}
      <div className={styles.tiles} data-layout={layout} style={{'--episode-columns': columns} as React.CSSProperties}>
        {episodes.map((episode, index) => {
          const expanded = snapshot.anchor.expandedEpisodeId === episode.id, rowEnd = layout === 'list' ? index : Math.min(episodes.length - 1, Math.floor(index / columns) * columns + columns - 1);
          const opened = episodes.slice(layout === 'list' ? index : Math.floor(index / columns) * columns, rowEnd + 1).find(entry => entry.id === snapshot.anchor.expandedEpisodeId);
          return <Fragment key={episode.id}><EpisodeTile model={entryCard(episode, env, 'in-show')} context="detail" artworkKind={episode.stillUrl ? 'still' : 'backdrop'} artwork={art.foreground(episode.stillUrl ?? episode.backdropUrl ?? show.backdropUrl, 632, 16 / 9)} watched={episode.watched} expanded={expanded} next={data.nextUp?.id === episode.id} description={episode.overview}
            onNavigate={() => { void binding.expand(data, episode).catch(binding.context.effects.reportError); }} />
            {rowEnd === index && opened && <div className={styles.expanded}><EpisodeBody binding={binding} data={data} /></div>}
          </Fragment>;
        })}
      </div>
      <div className={styles.paging}>{snapshot.pagination.canPrevious && <Button size="compact" onClick={() => { void binding.service.previousEpisodes(); }}>{t('title.previousPage')}</Button>}{snapshot.pagination.nextCursor && <Button size="compact" onClick={() => { void binding.service.nextEpisodes(); }}>{t('title.nextPage')}</Button>}</div>
    </section>}
    {tab === 'details' && <div className={detail.details}><section>{show.overview && <TitleProse className={detail.about}>{show.overview}</TitleProse>}<dl>{[[t('title.network'), show.network], [t('title.studio'), show.studio], [t('title.country'), show.country], [t('title.genres'), show.genres?.join(', ')]].filter(row => !!row[1]).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl></section></div>}
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
