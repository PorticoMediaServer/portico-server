import {useCallback, useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {useSearch} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {dayStarts, localDayStart, legacyChannel, legacyChannelRow, legacyProgram, pxPerMsFor, type GuideChannelRow, type GuideProgram} from '@core/guide/index.ts';
import {dayPrimeTime} from '@core/channel-guide.ts';
import type {GuideSourceSummaries} from '@core/guide/directory.ts';
import type {LegacyGuideOptions} from '@core/guide/legacy-source.ts';
import {dependsOn} from '@core/freshness-hub.ts';
import {useQuietRefresh} from '../../platform/freshness';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {useScreenAppearance} from '../browse/screen-preferences';
import {SeriesRulesEntry} from './SeriesRulesEntry';
import {ScreenProblem} from '../detail/ScreenProblem';
import {ProgrammePanel} from './ProgrammePanel';
import {RecordingsScreen} from './RecordingsScreen';
import {createLiveBindings, type LiveBindings} from './bindings';
import {parseLiveScreenSearch, type LiveScreenSearch} from './route-search';
import {liveProgramWindow} from './program-window';
import {useGuideViewPreference} from './guide-view-preference';
import {LiveOnNow} from './OnNow';
import {GuideScreen, type GuideRowData} from './GuideScreen';
import styles from './live.module.css';

const t = defaultI18n.t, halfHour = 30 * 60_000;
const liveDependencies = [dependsOn.list('guide'), dependsOn.list('liveSources'), dependsOn.list('libraryChannels'), dependsOn.list('myRecordings'), dependsOn.personal(), dependsOn.event('stream.resync')];
const nowStart = () => Math.floor(Date.now() / halfHour) * halfHour;
type Selected = Readonly<{owner: LiveBindings; channel: GuideChannelRow; programme?: GuideProgram}>;
function BoundLive({captured}: {captured: ProtectedLibraryScreen}) {
  const context = useLibraryScreenContext(captured), search = parseLiveScreenSearch(useSearch({from: '/live'})), width = useShellWidth();
  const appearance = useScreenAppearance(captured.context)!;
  const timezone = defaultI18n.region.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone;
  const key = JSON.stringify({kind: search.kind ?? 'all', sourceId: search.sourceId ?? '', timezone, group: search.group, favorites: search.favorites,
    search: search.q, sort: search.sort ?? 'number'} satisfies LegacyGuideOptions);
  const owner = useMemo(() => createLiveBindings(context, JSON.parse(key)), [context, key]);
  useEffect(() => () => owner.dispose(), [owner]);
  const snapshot = useSyncExternalStore(owner.store.subscribe, owner.store.getSnapshot, owner.store.getSnapshot);
  const authorityFailure = useSyncExternalStore(owner.failure.subscribe, owner.failure.getSnapshot, owner.failure.getSnapshot);
  const [summary, setSummary] = useState<{owner: LiveBindings; value: GuideSourceSummaries}>(), [failed, setFailed] = useState<LiveBindings>(), [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let current = true;
    void owner.sources().then(value => {if (current) {setSummary({owner, value});setFailed(undefined);}}).catch(() => {if (current) {setSummary(undefined);setFailed(owner);}});
    return () => {current = false;};
  }, [owner, attempt]);
  const sources = summary?.owner === owner ? summary.value.sources : undefined;
  const selectedSources = sources?.filter(source => (!search.sourceId || source.id === search.sourceId) && (!search.kind || search.kind === 'all' || source.provenance === search.kind)) ?? [];
  const groupCounts = new Map<string, number>();
  // Source summaries partition channels. Their published counts are combined,
  // never calculated from the loaded guide page or a lineup scan.
  for (const source of selectedSources) for (const group of source.groupCounts) groupCounts.set(group.name, (groupCounts.get(group.name) ?? 0) + group.count);
  const groups = [...groupCounts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  const searchKey = JSON.stringify(search);
  const navigate = useCallback((change: Partial<LiveScreenSearch>) => {
    const filtersChanged = ['sourceId', 'kind', 'group', 'favorites', 'q'].some(key => Object.hasOwn(change, key));
    if (context.isCurrent()) void router.navigate({to: '/live', search: {...JSON.parse(searchKey), ...(filtersChanged ? {channelId: undefined} : {}), ...change}, replace: true, resetScroll: false}).catch(context.effects.reportError);
  }, [context, searchKey]);
  const [start, setStart] = useState(nowStart), [now, setNow] = useState(Date.now), [selected, setSelected] = useState<Selected>();
  useEffect(() => {const timer = setInterval(() => setNow(Date.now()), 30_000);return () => clearInterval(timer);}, []);
  useEffect(() => {
    if (!search.sourceId || !search.channelId || search.kind !== 'live-source' && search.kind !== 'library-channel') return;
    const request = new AbortController();
    void owner.selectedChannel({sourceId: search.sourceId, channelId: search.channelId, provenance: search.kind}, request.signal).then(channel => {
      if (!channel || request.signal.aborted || !owner.life.current()) return;
      const time = Date.now(), programme = channel.programmes.find(programme => Date.parse(programme.start) <= time && time < Date.parse(programme.end)) ?? channel.programmes[0];
      setSelected({owner, channel: legacyChannelRow(channel), ...(programme ? {programme: legacyProgram(programme)} : {})});
    }).catch(error => {if (!request.signal.aborted) context.effects.reportError(error);});
    return () => request.abort();
  }, [context, owner, search.sourceId, search.channelId, search.kind]);
  const phone = width < 600, column = phone ? 60 : width < 1024 ? 180 : 284, rowHeight = phone ? 72 : width < 1024 ? 62 : 76;
  const scale = pxPerMsFor(width < 1024 ? 90 : 240), end = start + Math.max(1, width - column) / scale;
  const {view, remember} = useGuideViewPreference({...context.scope, kind: search.kind ?? 'all', sourceId: search.sourceId ?? ''}, search.view, context.isCurrent);
  const {start: readStart, end: readEnd} = liveProgramWindow(view, start, end, now);
  // The guide body scrolls with the page; the loaded window is the visible rows.
  const [visible, setVisible] = useState({first: 0, last: Math.max(0, Math.ceil(640 / rowHeight))});
  const onVisibleWindow = useCallback((first: number, last: number) => setVisible(old => old.first === first && old.last === last ? old : {first, last}), []);
  const first = visible.first, last = visible.last;
  useEffect(() => owner.store.setViewport({firstRow: first, lastRow: last, start: readStart, end: readEnd}), [owner, first, last, readStart, readEnd]);
  useEffect(() => {
    if (!search.channelId || selected?.owner === owner) return;
    for (let index = first; index <= last; index++) {
      const channel = owner.store.channelAt(index);
      if (channel?.id !== search.channelId) continue;
      const programme = owner.store.programsFor(index, readStart, readEnd).programs.find(programme => programme.start <= now && now < programme.end);
      if (programme) setSelected({owner, channel, programme});
      return;
    }
  }, [owner, snapshot.version, search.channelId, selected, first, last, readStart, readEnd, now]);
  const currentSelection = selected?.owner === owner ? selected : undefined;
  useQuietRefresh('live-guide', async refresh => {
    await refresh.settle();
    if (refresh.signal.aborted || !owner.life.current()) return;
    const value = await owner.refresh(refresh.signal);
    if (!refresh.signal.aborted && owner.life.current()) {setSelected(undefined);setSummary({owner, value});setFailed(undefined);}
  }, () => summary?.owner === owner, {dependencies: liveDependencies, everyMs: 0});
  const play = (channel: GuideChannelRow) => {
    const actual = legacyChannel(channel), playback = captured.binding.playback;
    if (!actual || !playback || !owner.life.current()) return;
    void playback.runtime.playFromGuide(actual, {signal: owner.life.signal(), isCurrent: owner.life.current}).catch(context.effects.reportError);
  };
  const ends = selectedSources.map(source => Date.parse(source.availableEnd));
  const through = ends.length && ends.every(value => Number.isFinite(value)) ? Math.min(...ends) : undefined;
  const days = through === undefined ? [] : dayStarts(now, through, timezone);
  const throughDay = through === undefined ? undefined : new Intl.DateTimeFormat(defaultI18n.region.formatLocale, {weekday: 'long', timeZone: timezone}).format(through - 1);
  const hasSources = !!sources?.length;
  const total = snapshot.total ?? Math.max(1, last + 1);
  const rows: GuideRowData[] = [];
  for (let index = first; index <= last; index++) {
    const channel = owner.store.channelAt(index), page = owner.store.programsFor(index, readStart, readEnd);
    rows.push({index, ...(channel ? {channel} : {}), programs: page.programs, complete: page.complete, failed: page.failed});
  }
  const favoriteCount = selectedSources.reduce((sum, source) => sum + source.favoriteCount, 0);
  const chips = [
    {id: 'all', label: t('channels.filterAll'), count: snapshot.total, selected: !search.group && !search.favorites, onSelect: () => navigate({group: undefined, favorites: undefined})},
    {id: 'favorites', label: t('channels.favorites'), favorites: true, count: favoriteCount, selected: search.favorites === true, onSelect: () => navigate({favorites: !search.favorites || undefined, group: undefined})},
    ...groups.map(([group, count]) => ({id: `group:${group}`, label: group, count, selected: search.group === group, onSelect: () => navigate({group, favorites: undefined})})),
  ];
  const sourceOptions = [{value: 'all', label: t('channels.all')},
    ...(sources ?? []).map(source => ({value: JSON.stringify(['source', source.provenance, source.id]), label: source.provenance === 'library-channel' ? t('channels.custom.name') : source.name}))];
  const switchSource = (value: string) => {
    const source = sources?.find(source => JSON.stringify(['source', source.provenance, source.id]) === value);
    if (source) navigate({sourceId: source.id, kind: source.provenance, channelId: undefined, group: undefined});
    else if (value === 'all') navigate({sourceId: undefined, kind: undefined, channelId: undefined, group: undefined});
  };
  const sourceName = search.sourceId ? selectedSources[0]?.provenance === 'library-channel' ? t('channels.custom.name') : selectedSources[0]?.name : t('channels.all');
  const onChanged = useCallback(() => { owner.store.invalidate(); setAttempt(value => value + 1); }, [owner]);
  const openRecordings = useCallback(() => {
    setSelected(undefined);
    if (context.isCurrent()) void router.navigate({to: '/live', search: {...JSON.parse(searchKey), tab: 'recordings', dvr: 'upcoming'}, replace: true, resetScroll: false}).catch(context.effects.reportError);
  }, [context, searchKey]);
  if (authorityFailure) return <ScreenProblem context={context} error={authorityFailure} retry={() => setAttempt(value => value + 1)}/>;
  return <>
    {failed === owner ? <div className={styles.guideState}><StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setAttempt(value => value + 1)}}/></div>
      : sources && !sources.length ? <div className={styles.guideState}><StateView title={t('channels.noSourcesTitle')} body={t(context.identity?.viewer.role === 'owner' ? 'channels.noSourcesOwner' : 'channels.noSourcesMember')}/></div>
        : <GuideScreen title={t('next.home.shell.live')}
          switcher={hasSources ? {value: search.sourceId ? JSON.stringify(['source', selectedSources[0]?.provenance, search.sourceId]) : 'all', options: sourceOptions, onChange: switchSource} : undefined}
          summary={hasSources ? {name: sourceName ?? t('channels.all'), ...(snapshot.total !== undefined ? {channels: snapshot.total} : {}), ...(throughDay ? {through: t('next.live.guideThrough', {day: throughDay})} : {})} : undefined}
          onTabChange={() => navigate({tab: 'recordings'})}
          chips={chips}
          onNow={width >= 1024 && hasSources ? <LiveOnNow owner={owner} now={now} timezone={timezone} artwork={context.artwork} reducedMotion={appearance.reducedMotion}
            onOpen={(channel, programme) => {if (owner.life.current()) setSelected({owner, channel, programme});}}
            onPlay={captured.binding.playback ? play : undefined}/> : undefined}
          day={{label: t('guide.day'), value: String(localDayStart(start, timezone)),
            options: days.map(day => ({value: String(day), label: day === localDayStart(now, timezone) ? t('guide.today') : new Intl.DateTimeFormat(defaultI18n.region.formatLocale, {weekday: 'long', timeZone: timezone}).format(day)})),
            onChange: value => {const day = days.find(day => String(day) === value);if (day !== undefined) setStart(dayPrimeTime(day, timezone));}}}
          sort={{value: search.sort ?? 'number', options: [{value: 'number', label: t('channels.sortNumber')}, {value: 'name', label: t('channels.sortName')}, {value: 'favorites', label: t('channels.favorites')}],
            onChange: sort => {if (sort === 'number' || sort === 'name' || sort === 'favorites') navigate({sort});}}}
          problem={snapshot.channelsError ? <StateView title={t('guide.tileFailed')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => owner.store.retry()}}/> : undefined}
          empty={snapshot.total === 0 ? <StateView title={t('channels.filterEmptyTitle', {filter: search.group ?? (search.favorites ? t('channels.favorites') : search.q ?? t('channels.all'))})}
            primary={{id: 'all', label: t('channels.showAll'), onPress: () => navigate({group: undefined, favorites: undefined, q: undefined})}}/> : undefined}
          view={view} onViewChange={view => {if (context.isCurrent()) {remember(view);navigate({view});}}}
          time={{earlier: t('guide.earlier'), now: t('guide.now'), later: t('guide.later'), onEarlier: () => setStart(value => value - 2 * 60 * 60_000), onNow: () => setStart(nowStart()), onLater: () => setStart(value => value + 2 * 60 * 60_000)}}
          span={{start, end, now, scale, column, rowHeight, timezone}}
          rows={rows} total={total}
          selected={currentSelection ? {channelId: currentSelection.channel.id, ...(currentSelection.programme ? {programmeId: currentSelection.programme.id} : {})} : undefined}
          onSelect={(channel, programme) => {if (owner.life.current()) setSelected({owner, channel, ...(programme ? {programme} : {})});}}
          onPlay={captured.binding.playback ? play : undefined}
          onVisibleWindow={onVisibleWindow} onRetry={() => owner.store.retry()}/>}
    {currentSelection && <ProgrammePanel captured={captured} channel={currentSelection.channel} programme={currentSelection.programme}
      now={now} timezone={timezone} open onClose={() => setSelected(undefined)}
      onWatch={captured.binding.playback ? () => play(currentSelection.channel) : undefined} onChanged={onChanged} onViewRecordings={openRecordings}/>}
  </>;
}
function LiveRouteScreen() {
  const captured = useProtectedWebScreenContext();
  const search = parseLiveScreenSearch(useSearch({from: '/live'}));
  const openView = (dvr: 'upcoming' | 'recorded' | 'rules' | 'history' | 'storage') => {
    if (captured) void router.navigate({to: '/live', search: {...search, tab: 'recordings', dvr}, replace: true, resetScroll: false}).catch(() => {});
  };
  const openGuide = () => {if (captured) void router.navigate({to: '/live', search: {...search, tab: 'guide'}, replace: true, resetScroll: false}).catch(() => {});};
  if (!captured) return null;
  if (search.tab !== 'recordings') return <BoundLive captured={captured}/>;
  const view = search.dvr ?? 'upcoming';
  if (view === 'rules') return <SeriesRulesEntry captured={captured} onView={openView}/>;
  return <RecordingsScreen captured={captured} view={view === 'recorded' || view === 'history' || view === 'storage' ? view : 'upcoming'} onView={openView} onGuide={openGuide}/>;
}
export default defineWebScreen({id: 'live', component: LiveRouteScreen, validateSearch: parseLiveScreenSearch});
