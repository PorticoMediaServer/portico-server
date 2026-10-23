import {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type CSSProperties} from 'react';
import {useSearch} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {dayStarts, localDayStart, layoutRow, legacyChannel, pxPerMsFor, rulerTicks, type GuideChannelRow, type GuideProgram} from '@core/guide/index.ts';
import {dayPrimeTime} from '@core/channel-guide.ts';
import type {GuideSourceSummaries} from '@core/guide/directory.ts';
import type {LegacyGuideOptions} from '@core/guide/legacy-source.ts';
import {programmeFactParts} from '@portico/client-core/kit-model';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {Artwork, ChannelMark} from '../../kit/artwork';
import {Button, Chip, Segmented, Select, TextField} from '../../kit/controls';
import {BottomSheet, SidePanel} from '../../kit/overlays';
import {Skeleton, StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {ScreenProblem} from '../detail/ScreenProblem';
import {createLiveBindings, type LiveBindings} from './bindings';
import {parseLiveScreenSearch, type LiveScreenSearch} from './route-search';
import styles from './live.module.css';

const t = defaultI18n.t, halfHour = 30 * 60_000;
const nowStart = () => Math.floor(Date.now() / halfHour) * halfHour;
type Selected = Readonly<{owner: LiveBindings; channel: GuideChannelRow; programme: GuideProgram}>;
function BoundLive({captured}: {captured: ProtectedLibraryScreen}) {
  const context = useLibraryScreenContext(captured), search = parseLiveScreenSearch(useSearch({from: '/live'})), width = useShellWidth();
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
  const navigate = useCallback((change: Partial<LiveScreenSearch>) => {if (context.isCurrent()) void router.navigate({to: '/live', search: {...JSON.parse(searchKey), ...change}, replace: true, resetScroll: false}).catch(context.effects.reportError);}, [context, searchKey]);
  const [draft, setDraft] = useState(search.q ?? '');
  useEffect(() => setDraft(search.q ?? ''), [search.q]);
  useEffect(() => {if (draft === (search.q ?? '')) return;const timer = setTimeout(() => navigate({q: draft.trim() || undefined}), 180);return () => clearTimeout(timer);}, [draft, search.q, navigate]);
  const [start, setStart] = useState(nowStart), [now, setNow] = useState(Date.now), [selected, setSelected] = useState<Selected>();
  useEffect(() => {const timer = setInterval(() => setNow(Date.now()), 30_000);return () => clearInterval(timer);}, []);
  const viewport = useRef<HTMLDivElement>(null), [windowSize, setWindowSize] = useState({width: 1, height: 480}), [scrollTop, setScrollTop] = useState(0);
  useLayoutEffect(() => {
    const element = viewport.current;if (!element) return;
    const measure = () => setWindowSize({width: element.clientWidth, height: element.clientHeight});
    const observer = new ResizeObserver(measure);observer.observe(element);measure();return () => observer.disconnect();
  }, [owner, sources?.length]);
  const phone = width < 600, column = phone ? 60 : width < 1024 ? 180 : 284, rowHeight = phone ? 72 : width < 1024 ? 62 : 76;
  const scale = pxPerMsFor(width < 1024 ? 90 : 240), end = start + Math.max(1, windowSize.width - column) / scale;
  const first = Math.max(0, Math.floor(scrollTop / rowHeight) - 1), count = Math.ceil(windowSize.height / rowHeight) + 3;
  const last = Math.max(first, Math.min(first + count - 1, (snapshot.total ?? count) - 1));
  useEffect(() => owner.store.setViewport({firstRow: first, lastRow: last, start, end}), [owner, first, last, start, end]);
  useEffect(() => {
    if (!search.channelId || selected?.owner === owner) return;
    for (let index = first; index <= last; index++) {
      const channel = owner.store.channelAt(index);
      if (channel?.id !== search.channelId) continue;
      const programme = owner.store.programsFor(index, start, end).programs.find(programme => programme.start <= now && now < programme.end);
      if (programme) setSelected({owner, channel, programme});
      return;
    }
  }, [owner, snapshot.version, search.channelId, selected, first, last, start, end, now]);
  const drag = useRef<Readonly<{x: number; y: number; start: number}> | undefined>(undefined), moved = useRef(false);
  const currentSelection = selected?.owner === owner ? selected : undefined;
  const facts = (programme: GuideProgram) => programmeFactParts({startMs: programme.start, endMs: programme.end,
    episode: programme.episode ? {mode: 'seasonal', season: programme.episode.season, episode: programme.episode.number} : undefined,
    subtitle: programme.subtitle, categories: programme.categories, year: programme.year, rating: programme.rating, flags: programme.flags, recording: programme.recording?.state === 'recording'},
  {t, locale: defaultI18n.region.formatLocale, timeZone: timezone});
  const mark = (channel: GuideChannelRow) => {
    const native = legacyChannel(channel);
    const actual = native && captured.context.home.getSnapshot().document?.rows.flatMap(row => row.live?.channels ?? []).find(home => home.channel.id === native.id && home.channel.sourceId === native.sourceId && home.channel.generation === native.generation)?.channel.mark;
    return actual && native ? <ChannelMark mark={{sourceId: native.sourceId, id: native.id, ...actual}} size={phone ? 36 : 34}/>
      : <span className={styles.mark} aria-hidden="true"/>;
  };
  const play = (channel: GuideChannelRow) => {
    const actual = legacyChannel(channel), playback = captured.binding.playback;
    if (!actual || !playback || !owner.life.current()) return;
    void playback.runtime.playFromGuide(actual, {signal: owner.life.signal(), isCurrent: owner.life.current}).catch(context.effects.reportError);
  };
  const ends = selectedSources.map(source => Date.parse(source.availableEnd));
  const through = ends.length && ends.every(value => Number.isFinite(value)) ? Math.min(...ends) : undefined;
  const days = through === undefined ? [] : dayStarts(now, through, timezone);
  const view = search.view ?? 'guide', hasSources = !!sources?.length;
  const body = currentSelection && <>
    <p className={styles.facts}>{facts(currentSelection.programme).map(part => <span key={part.id}>{part.text}</span>)}</p>
    {currentSelection.programme.description && <p>{currentSelection.programme.description}</p>}
    {currentSelection.programme.image && <Artwork source={context.artwork.foreground(currentSelection.programme.image, 640, 16 / 9)} alt=""/>}
  </>;
  const footer = currentSelection && currentSelection.channel.tuneAvailable && captured.binding.playback
    ? <Button variant="primary" icon="i-play" onClick={() => play(currentSelection.channel)}>{t(currentSelection.programme.start > now ? 'guide.watchChannel' : 'guide.watchLive')}</Button> : undefined;
  if (authorityFailure) return <ScreenProblem context={context} error={authorityFailure} retry={() => setAttempt(value => value + 1)}/>;
  return <section className={styles.live}>
    <header className={styles.header}><h1>{t('next.home.shell.live')}</h1>
      {sources && <Select aria-label={t('channels.source')} value={search.sourceId ? JSON.stringify(['source', selectedSources[0]?.provenance, search.sourceId]) : 'all'} options={[{value: 'all', label: t('channels.all')}, ...sources.map(source => ({value: JSON.stringify(['source', source.provenance, source.id]), label: source.provenance === 'library-channel' ? t('channels.custom.name') : source.name}))]}
        onValueChange={value => {const source = sources.find(source => JSON.stringify(['source', source.provenance, source.id]) === value);
          if (source) navigate({sourceId: source.id, kind: source.provenance, channelId: undefined, group: undefined});
          else if (value === 'all') navigate({sourceId: undefined, kind: undefined, channelId: undefined, group: undefined});}}/>}
      {hasSources && <p className={styles.sourceFacts}><strong>{t('next.live.sources', {count: selectedSources.length})}</strong>{through !== undefined && <span>{t('next.live.guideThrough', {day: new Intl.DateTimeFormat(defaultI18n.region.formatLocale, {weekday: 'long', timeZone: timezone}).format(through - 1)})}</span>}</p>}
    </header>
    {failed === owner ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setAttempt(value => value + 1)}}/>
      : sources && !sources.length ? <StateView title={t('channels.noSourcesTitle')} body={t(context.identity?.viewer.role === 'owner' ? 'channels.noSourcesOwner' : 'channels.noSourcesMember')}/>
        : <>
          <div className={styles.chips}><Chip selected={!search.group && !search.favorites} onClick={() => navigate({group: undefined, favorites: undefined})}>{t('channels.filterAll')}</Chip>
            <Chip icon="i-star" selected={search.favorites === true} count={selectedSources.reduce((sum, source) => sum + source.favoriteCount, 0)} onClick={() => navigate({favorites: !search.favorites || undefined, group: undefined})}>{t('channels.favorites')}</Chip>
            {groups.slice(0, 6).map(([group, count]) => <Chip key={group} selected={search.group === group} count={count} onClick={() => navigate({group, favorites: undefined})}>{group}</Chip>)}
            {groups.length > 6 && <Select aria-label={t('next.live.moreGroups')} placeholder={t('next.live.moreGroups')} value={groups.slice(6).some(([group]) => group === search.group) ? search.group : undefined}
              options={groups.slice(6).map(([group, count]) => ({value: group, label: group, description: defaultI18n.number(count)}))} onValueChange={group => navigate({group, favorites: undefined})}/>}
          </div>
          <div className={styles.controls}><div className={styles.stepper}><Button variant="icon" size="small" icon="i-left" iconOnly aria-label={t('guide.earlier')} onClick={() => setStart(value => value - 2 * 60 * 60_000)}/><Button variant="text" size="small" onClick={() => setStart(nowStart())}>{t('guide.now')}</Button><Button variant="icon" size="small" icon="i-right" iconOnly aria-label={t('guide.later')} onClick={() => setStart(value => value + 2 * 60 * 60_000)}/></div>
            {days.length > 0 && <Select aria-label={t('guide.day')} value={String(localDayStart(start, timezone))}
              options={days.map(day => ({value: String(day), label: day === localDayStart(now, timezone) ? t('guide.today') : new Intl.DateTimeFormat(defaultI18n.region.formatLocale, {weekday: 'long', timeZone: timezone}).format(day)}))}
              onValueChange={value => {const day = days.find(day => String(day) === value);if (day !== undefined) setStart(dayPrimeTime(day, timezone));}}/>}
            <TextField aria-label={t('web.live.findChannel')} placeholder={t('web.live.findChannel')} value={draft} onChange={event => setDraft(event.currentTarget.value)}/>
            <div className={styles.end}>{snapshot.total !== undefined && <span>{t('player.info.channels', {count: snapshot.total})}</span>}
              <Select aria-label={t('channels.sort')} value={search.sort ?? 'number'} options={[{value: 'number', label: t('channels.sortNumber')}, {value: 'name', label: t('channels.sortName')}, {value: 'favorites', label: t('channels.favorites')}]}
                onValueChange={sort => {if (sort === 'number' || sort === 'name' || sort === 'favorites') navigate({sort});}}/>
              <Segmented variant="view" aria-label={t('channels.view')} value={view} onValueChange={view => {if (view === 'guide' || view === 'list') navigate({view});}}
                items={[{value: 'guide', label: t('channels.viewGuide'), icon: 'i-grid'}, {value: 'list', label: t('channels.viewList'), icon: 'i-list'}]}/></div>
          </div>
          {snapshot.channelsError && <StateView title={t('guide.tileFailed')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => owner.store.retry()}}/>}
          {snapshot.total === 0 && <StateView title={t('channels.filterEmptyTitle', {filter: search.group ?? (search.favorites ? t('channels.favorites') : search.q ?? t('channels.all'))})}
            primary={{id: 'all', label: t('channels.showAll'), onPress: () => navigate({group: undefined, favorites: undefined, q: undefined})}}/>}
          <div className={styles.guide} style={{'--live-channel': `${column}px`, '--live-row': `${rowHeight}px`} as CSSProperties}>
            {view === 'guide' && <div className={styles.ruler}><span>{phone ? <Button className={styles.dockedNow} variant="text" size="xs" onClick={() => setStart(nowStart())}>{t('guide.now')}</Button> : t('guide.channelColumn')}</span><div className={styles.lane}>{rulerTicks(start, end, scale, timezone).map(tick => <span key={tick.atMs} className={styles.tick} style={{left: tick.x}}>{defaultI18n.time(tick.atMs)}</span>)}</div></div>}
            <div className={styles.viewport} ref={viewport} onScroll={event => setScrollTop(event.currentTarget.scrollTop)}
              onPointerDown={event => {if (event.pointerType === 'touch') {drag.current = {x: event.clientX, y: event.clientY, start};moved.current = false;}}}
              onPointerMove={event => {const initial = drag.current;if (!initial || Math.abs(event.clientX - initial.x) < 8 || Math.abs(event.clientY - initial.y) > Math.abs(event.clientX - initial.x)) return;
                moved.current = true;event.currentTarget.setPointerCapture(event.pointerId);setStart(initial.start - (event.clientX - initial.x) / scale);}}
              onPointerUp={() => {drag.current = undefined;}} onPointerCancel={() => {drag.current = undefined;}}
              onClickCapture={event => {if (moved.current) {event.preventDefault();event.stopPropagation();moved.current = false;}}}>
              <div className={styles.rows} style={{height: Math.max(0, snapshot.total ?? count) * rowHeight}}>
                {Array.from({length: Math.max(0, last - first + 1)}, (_, offset) => {
                  const index = first + offset, channel = owner.store.channelAt(index), page = owner.store.programsFor(index, start, end);
                  const programs = page.programs, cells = page.complete ? layoutRow(programs, {viewStart: start, viewEnd: end, pxPerMs: scale, now, gapPx: 2, ...(width < 1024 ? {tiers: {full: 50, title2: 50, title: 24}} : {})}) : [];
                  return <div key={index} className={styles.row} style={{top: index * rowHeight}} data-live-channel-id={channel?.id}>
                    <div className={styles.channel}>{channel ? <>{mark(channel)}<span><b>{channel.name}</b><small>{channel.number}{!phone && channel.group && <span>{channel.group}</span>}</small></span></> : <Skeleton width={phone ? 36 : 180} height={34}/>}</div>
                    <div className={styles.lane}>{!channel || !page.complete ? <Skeleton width="100%" height={rowHeight - 8}/> : view === 'guide' ? <>
                      {now >= start && now < end && <i className={styles.nowLine} style={{left: (now - start) * scale}}/>}
                      {cells.map(cell => <button key={`${cell.start}:${cell.end}`} className={styles.cell} data-state={cell.state} data-gap={cell.kind === 'gap' || undefined} data-tier={cell.tier}
                        style={{left: cell.x, width: cell.width}} type="button" aria-label={cell.program?.title ?? t('guide.noInformation')}
                        onClick={() => {if (cell.program && owner.life.current()) setSelected({owner, channel, programme: cell.program});}}>
                        <b>{cell.program?.title ?? t('guide.noInformation')}</b>{cell.tier === 'full' && cell.program && <small>{facts(cell.program).filter(part => ['episode-label', 'programme-subtitle', 'live', 'new', 'premiere', 'recording'].includes(part.id)).map(part => <span key={part.id}>{part.text}</span>)}</small>}
                      </button>)}
                    </> : <div className={styles.listPrograms}>{programs.filter(programme => programme.end > now).slice(0, 3).map(programme => <button key={programme.id} type="button" onClick={() => {if (owner.life.current()) setSelected({owner, channel, programme});}}><small>{defaultI18n.time(programme.start)}</small><b>{programme.title}</b><span>{facts(programme).filter(part => ['episode-label', 'programme-subtitle', 'live', 'new'].includes(part.id)).map(part => <span key={part.id}>{part.text}</span>)}</span></button>)}</div>}</div>
                  </div>;
                })}
              </div>
            </div>
          </div>
        </>}
    {phone ? <BottomSheet open={!!currentSelection} onOpenChange={open => {if (!open) setSelected(undefined);}} title={currentSelection?.programme.title} description={currentSelection?.channel.name} footer={footer}>{body}</BottomSheet>
      : <SidePanel open={!!currentSelection} onOpenChange={open => {if (!open) setSelected(undefined);}} title={currentSelection?.programme.title} description={currentSelection?.channel.name} footer={footer}>{body}</SidePanel>}
  </section>;
}
function LiveRouteScreen() {
  const captured = useProtectedWebScreenContext();
  return captured ? <BoundLive captured={captured}/> : null;
}
export default defineWebScreen({id: 'live', component: LiveRouteScreen, validateSearch: parseLiveScreenSearch});
