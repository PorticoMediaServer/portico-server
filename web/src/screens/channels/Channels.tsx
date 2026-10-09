import React, {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import {channelOperationId, saveChannelPreference, stripDays, dayPrimeTime} from '@core/channel-guide.ts';
import {defaultRecordingOptions, seriesRuleConfig, type RecordingDraft} from '@core/dvr.ts';
import {GuideWindowStore, legacyGuideCatalog, legacyChannel, legacyProgramme, BLOCK_MS, SLOT_MS, normalizePrograms, HOUR_MS, DAY_MS, MINUTE_MS, dayStarts, focusTarget, focusTimeOf, layoutRow, localDayStart, neighbor, nowNext, nowX, pxPerMsFor, rulerTicks, spanAt, type ChannelSource, type GuideCell, type GuideChannelRow, type GuideDataSource, type GuideProgram} from '@core/guide/index.ts';
import {ALL, LIBRARY, RECORDINGS, channelEntries, defaultEntry, hasReminder, readReminders, removeReminder, sourceName, setChannelSurf, forgetGuideReads, channelReason, serverWideOf, type ServerWide, subscribeReminders, toggleReminder, useChannelMemory, useChannelSources, channelGuideSource, localTimezone, type ChannelSort, type ChannelView, type GuideQuery, type Reminder} from '../../app/channels';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {ErrorState, errorText} from '../../app/errors';
import {usePlayerActions} from '../../player/PlayerContext';
import {useDVRClient} from '../live/guide';
import {createNowStore, type NowStore} from '../live/now-store';
import {DVRView} from '../live/DVR';
import {Artwork, Badge, Button, Chip, Chips, Dialog, Icon, IconButton, Menu, Notice, Page, PageHeader, Segmented, Select, Skeleton, StateView, Text, cx, useArtworkUrl, useCompact, useHoverCapable, type MenuItem} from '../../ui';
import s from './Channels.module.css';
import {dvrPaddings, dvrLimits} from '../live/dvr-groups';

/* Geometry (Spec §3.1, web). */
const COL = 240;
const CH = 220;
const ROW = 72;
const RULER = 40;
const GAP = 2;
const LIST_ROW = 88;
const LIST_ROW_COMPACT = 108;
const PX_PER_MS = pxPerMsFor(COL);
const OVERSCAN = 3;
const WIDE = 900;

type Selection = {channel: GuideChannelRow; program?: GuideProgram};

/** This device's time zone and labels in it. */
export function useGuideFormat() {
  const i18n = useI18n();
  const zone = localTimezone();
  return useMemo(() => {
    let dayKeyFormat: Intl.DateTimeFormat | undefined;
    try { dayKeyFormat = new Intl.DateTimeFormat('en-CA', {year: 'numeric', month: '2-digit', day: '2-digit', timeZone: zone}); } catch {}
    const dayKey = (ms: number) => (dayKeyFormat ? dayKeyFormat.format(new Date(ms)) : new Date(ms).toISOString().slice(0, 10));
    const clock = (ms: number) => i18n.time(ms);
    const dayLabel = (ms: number, now = Date.now()) => {
      const key = dayKey(ms);
      if (key === dayKey(now)) return i18n.t('guide.today');
      if (key === dayKey(now + DAY_MS)) return i18n.t('guide.tomorrow');
      if (key === dayKey(now - DAY_MS)) return i18n.t('guide.yesterday');
      return i18n.date(ms, 'weekday');
    };
    const range = (a: number, b: number) => i18n.t('guide.range', {start: clock(a), end: clock(b)});
    return {zone, clock, dayLabel, range, t: i18n.t, duration: (seconds: number) => i18n.duration(seconds, 'short')};
  }, [i18n, zone]);
}
type Format = ReturnType<typeof useGuideFormat>;

/** Kept for the single-channel timeline (§6): one channel's programs, few rows. */
function useNow(stepMs = 30_000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => { const id = setInterval(() => setNow(Date.now()), stepMs); return () => clearInterval(id); }, [stepMs]);
  return now;
}

function useWidth(ref: React.RefObject<HTMLElement | null>): number {
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setWidth(el.clientWidth);
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);
  return width;
}

/** The height from an element's top to the bottom of the window (the grid fills the screen). */
function useFillHeight(ref: React.RefObject<HTMLElement | null>, bottom = 16, min = 320): number {
  const [height, setHeight] = useState(560);
  useLayoutEffect(() => {
    const measure = () => { const el = ref.current; if (el) setHeight(Math.max(min, Math.round(window.innerHeight - el.getBoundingClientRect().top - bottom))); };
    measure();
    window.addEventListener('resize', measure);
    const id = setTimeout(measure, 300);
    return () => { window.removeEventListener('resize', measure); clearTimeout(id); };
  }, [ref, bottom, min]);
  return height;
}

const minutes = (ms: number) => Math.max(1, Math.round(ms / MINUTE_MS));
/** "22 min left", or "2h 7m left" past an hour. */
const timeLeft = (format: {t: ReturnType<typeof useI18n>['t']; duration: (s: number) => string}, ms: number) => (ms < 60 * MINUTE_MS ? format.t('guide.minutesLeft', {count: minutes(ms)}) : format.t('guide.timeLeft', {time: format.duration(Math.round(ms / MINUTE_MS) * 60)}));

/**
 * Channels (Spec — Channels and Guide §2, §3, §7, §10). One route per entry: All channels, one
 * source, or Recordings. Guide or List, remembered per source.
 */
export function ChannelsScreen() {
  const {entry} = useParams({strict: false}) as {entry?: string};
  const search = useSearch({strict: false}) as {view?: ChannelView; group?: string; favorites?: boolean};
  const navigate = useNavigate();
  const {sources, loading, failed, refresh} = useChannelSources();
  const format = useGuideFormat();
  const t = format.t;
  const compact = useCompact();
  const entries = useMemo(() => channelEntries(sources), [sources]);
  const current = entry && entries.some(e => e.id === entry) ? entry : undefined;
  // /channels opens All channels (2+ sources), else the only source.
  useEffect(() => {
    if (!current && sources.length && !loading) void navigate({to: '/channels/$entry', params: {entry: defaultEntry(sources)}, replace: true});
  }, [current, sources, loading, navigate]);
  if (!current) {
    if (failed && !sources.length) return <Page><StateView icon="warning" title={t('channels.title')} body={errorText(new Error('sources'), 'live', 'load')} action={{label: t('action.tryAgain'), onClick: refresh}} /></Page>;
    if (!loading && !sources.length) return <Page><PageHeader title={t('channels.title')} /><StateView icon="channels" title={t('web.live.empty.noSourcesTitle')} body={t('web.live.empty.noSourcesMember')} /></Page>;
    return <Page><PageHeader title={t('channels.title')} /><GuideSkeleton /></Page>;
  }
  const info = entries.find(e => e.id === current)!;
  const picker = compact && entries.length > 1 ? <SourcePicker entries={entries} current={current} /> : null;
  if (current === RECORDINGS) {
    return (
      <Page>
        <PageHeader eyebrow={t('channels.title')} title={info.name} />
        {picker}
        <DVRView />
      </Page>
    );
  }
  return <GuideScreen key={current} entry={current} name={info.name} library={info.library} sources={sources} picker={picker} search={search} />;
}

function SourcePicker({entries, current}: {entries: ReturnType<typeof channelEntries>; current: string}) {
  const navigate = useNavigate();
  const t = useI18n().t;
  const go = (id: string) => void navigate({to: '/channels/$entry', params: {entry: id}});
  // §2.0 phone: chips for 3 or fewer entries, otherwise a dropdown titled with the current source.
  if (entries.length <= 3) {
    return <div className={s.picker}><Chips>{entries.map(e => <Chip key={e.id} label={e.name} icon={e.icon} pressed={e.id === current} onClick={() => go(e.id)} />)}</Chips></div>;
  }
  const now = entries.find(e => e.id === current);
  return (
    <div className={s.picker}>
      <Menu label={t('channels.source')} align="start" trigger={<Button variant="secondary" size="sm" icon={now?.icon} iconAfter="chevronDown" label={now?.name ?? ''} />} items={entries.map(e => ({id: e.id, label: e.name, icon: e.icon, selected: e.id === current}))} onSelect={go} />
    </div>
  );
}

function GuideScreen({entry, name, library, sources, picker, search}: {entry: string; name: string; library: boolean; sources: readonly ChannelSource[]; picker: React.ReactNode; search: {view?: ChannelView; group?: string; favorites?: boolean}}) {
  const {api, session, owner} = useSession();
  const serverId = session?.viewer.serverId ?? '';
  const navigate = useNavigate();
  const format = useGuideFormat();
  const t = format.t;
  const compact = useCompact();
  const memory = useChannelMemory(entry);
  const pageRef = useRef<HTMLDivElement>(null);
  const width = useWidth(pageRef);
  const view: ChannelView = search.view ?? memory.view ?? (compact ? 'list' : 'guide');
  const gridFits = !compact && (width === 0 || width >= WIDE);
  const setView = (v: ChannelView) => { memory.setView(v); void navigate({to: '/channels/$entry', params: {entry}, search: {...search, view: v}, replace: true}); };
  const setFilter = (next: {group?: string; favorites?: boolean}) => void navigate({to: '/channels/$entry', params: {entry}, search: {view: search.view, ...(next.group ? {group: next.group} : {}), ...(next.favorites ? {favorites: true} : {})}, replace: true});
  const sort = memory.sort;
  const [showHidden, setShowHidden] = useState(false);

  const makeQuery = useCallback((): GuideQuery => ({entry, timezone: format.zone, sort, ...(search.group ? {group: search.group} : {}), ...(search.favorites ? {favorites: true} : {}), ...(showHidden ? {includeHidden: true} : {})}), [entry, format.zone, sort, search.group, search.favorites, showHidden]);
  // The current data source is kept for the per-channel timeline (§6), which reads one channel by time.
  const sourceRef = useRef<GuideDataSource | null>(null);
  const newSource = () => (sourceRef.current = channelGuideSource(api, serverId, makeQuery()));
  const store = useMemo(() => new GuideWindowStore({source: newSource()}), [api, serverId]); // eslint-disable-line react-hooks/exhaustive-deps
  const queryKey = JSON.stringify(makeQuery());
  const firstQuery = useRef(queryKey);
  useEffect(() => {
    if (firstQuery.current === queryKey) return;
    firstQuery.current = queryKey;
    forgetGuideReads();
    store.reset(newSource());
  }, [queryKey]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => store.dispose(), [store]);
  const snapshot = useSyncExternalStore(store.subscribe, store.getSnapshot);

  // Groups and favorites of this source, unfiltered (chips): the top groups by count, plus All.
  const [catalog, setCatalog] = useState<{groups: string[]; favorites: boolean}>({groups: [], favorites: false});
  useEffect(() => {
    const controller = new AbortController();
    legacyGuideCatalog(api,serverId,{kind:entry===ALL?'all':entry===LIBRARY?'library-channel':'live-source',sourceId:entry===ALL||entry===LIBRARY?'':entry,timezone:format.zone,sort:'number'},controller.signal).then(summary => {
      if(!controller.signal.aborted)setCatalog({groups:[...summary.groups],favorites:summary.favorites});
    }, () => {});
    return () => controller.abort();
  }, [api, serverId, entry, format.zone, snapshot.total === undefined]); // eslint-disable-line react-hooks/exhaustive-deps

  const [selection, setSelection] = useState<Selection | null>(null);
  const [toast, setToast] = useState<{text: string; action?: {label: string; onClick: () => void}} | null>(null);
  useEffect(() => { if (!toast) return; const id = setTimeout(() => setToast(null), 5000); return () => clearTimeout(id); }, [toast]);
  const player = usePlayerActions();
  const dvr = useDVRClient();
  // FEAT-01: when every loaded channel is blocked by the server itself, say so once.
  const wide = useMemo(() => {
    const rows: GuideChannelRow[] = [];
    for (let r = 0; r < Math.min(snapshot.total ?? 0, 30); r++) { const c = store.channelAt(r); if (c) rows.push(c); }
    return serverWideOf(rows);
  }, [store, snapshot]);
  const wideNotice = wide ? <div style={{padding: '0 var(--page-gutter) 12px'}}><ServerWideNotice reason={wide} library={library} /></div> : null;
  const viewport = useRef({firstRow: 0, lastRow: 0});
  const watch = useCallback((channel: GuideChannelRow) => {
    const rawChannel = legacyChannel(channel);
    if (!rawChannel || !channel.tuneAvailable) return;
    // FEAT-07: remember where this channel sits in the guide, so the player can change channel.
    const {firstRow, lastRow} = viewport.current;
    for (let row = Math.max(0, firstRow - 10); row <= lastRow + 10; row++) {
      if (store.channelAt(row)?.id === channel.id) { setChannelSurf({row, channelAt: r => store.channelAt(r)}); break; }
    }
    player.tune(rawChannel);
  }, [player, store]);
  const recordNow = useCallback(async (channel: GuideChannelRow, program: GuideProgram) => {
    const c = legacyChannel(channel), p = legacyProgramme(program);
    if (!dvr || !c || !p || !channel.recordAvailable || program.end <= Date.now()) { setToast({text: t('guide.cantRecord')}); return; }
    try {
      await dvr.schedule({channel: c, programme: p, series: false}, defaultRecordingOptions);
      setToast({text: t('guide.recordedShortcut', {title: program.title})});
      (forgetGuideReads(), store.invalidate());
    } catch (e) { setToast({text: errorText(e, 'live', 'save')}); }
  }, [dvr, store, t]);
  const setPreference = useCallback(async (channel: GuideChannelRow, values: {favorite: boolean; hidden: boolean}) => {
    const c = legacyChannel(channel);
    if (!c) return;
    try {
      await saveChannelPreference(api, serverId, c, await channelOperationId(() => Promise.resolve(crypto.randomUUID())), values);
      forgetGuideReads(); store.reset(newSource());
    } catch (e) { setToast({text: errorText(e, 'live', 'save')}); }
  }, [api, serverId, store, makeQuery]);

  const range = useMemo(() => {
    const scoped = entry === ALL ? sources : sources.filter(x => x.id === entry);
    const starts = scoped.map(x => x.availableStart).filter((v): v is number => !!v);
    const ends = scoped.map(x => x.availableEnd).filter((v): v is number => !!v);
    const now = Date.now();
    return {start: Math.min(now - DAY_MS, ...(starts.length ? [Math.min(...starts)] : [])), end: Math.max(now + 3 * HOUR_MS, ...(ends.length ? [Math.max(...ends)] : [now + 7 * DAY_MS]))};
  }, [sources, entry]);

  const filterName = search.favorites ? t('channels.favorites') : search.group ?? '';
  const channelMenu = useCallback((channel: GuideChannelRow): MenuItem[] => [
    {id: 'fav', label: channel.favorite ? t('channels.removeFavorite') : t('channels.addFavorite'), icon: channel.favorite ? 'starFilled' : 'star'},
    ...(owner ? [{id: 'hide', label: legacyChannel(channel)?.hidden ? t('channels.unhide') : t('channels.hide'), icon: 'eyeOff' as const}] : []),
    ...(owner && channel.kind === 'library' ? [{id: 'edit', label: t('channels.editChannel'), icon: 'edit' as const}] : []),
  ], [t, owner]);
  const onChannelMenu = useCallback((channel: GuideChannelRow, id: string) => {
    const hidden = legacyChannel(channel)?.hidden ?? false;
    if (id === 'fav') void setPreference(channel, {favorite: !channel.favorite, hidden});
    else if (id === 'hide') void setPreference(channel, {favorite: channel.favorite, hidden: !hidden});
    else if (id === 'edit') void navigate({to: '/settings/$section', params: {section: 'server-live'}, search: {}});
  }, [setPreference, navigate]);
  const [timeline, setTimeline] = useState<GuideChannelRow | null>(null);
  const loadPrograms = useCallback((channelId: string, start: number, end: number, signal: AbortSignal) => sourceRef.current!.programs([channelId], start, end, signal).then(r => r[channelId] ?? []), []);
  // PERF-07: one stable object, so memoised rows keep identical props across unrelated renders
  // (selection, toasts). Rows re-render only when the store notifies their page.
  const shared: Shared = useMemo(() => ({store, format, sources, entry, onOpen: setSelection, onWatch: watch, onRecord: recordNow, channelMenu, onChannelMenu, onChannel: setTimeline, loadPrograms, viewport, serverWide: !!wide}), [store, format, sources, entry, watch, recordNow, channelMenu, onChannelMenu, loadPrograms, wide]); // eslint-disable-line react-hooks/exhaustive-deps

  // §10: a source whose guide refresh is behind says so above the guide (owners get a link to it).
  const stale = (entry === ALL ? sources : sources.filter(x => x.id === entry)).some(x => x.guideState === 'stale');
  const staleNotice = stale ? <div style={{padding: '0 var(--page-gutter) 12px'}}><Notice tone="warning" compact action={owner ? {label: t('guide.openSource'), onClick: () => void navigate({to: '/settings/$section', params: {section: 'server-live'}, search: {}})} : undefined}>{t('guide.stale')}</Notice></div> : null;
  let body: React.ReactNode;
  if (snapshot.total === undefined && snapshot.channelsError) body = <ErrorState error={new Error('channels')} context="live" retry={() => store.retry()} />;
  else if (snapshot.total === 0) {
    body = filterName
      ? <StateView icon="filter" title={t('channels.filterEmptyTitle', {filter: filterName})} action={{label: t('channels.showAll'), onClick: () => setFilter({})}} />
      : <StateView icon={library ? 'library' : 'channels'} title={t('channels.emptyTitle', {source: name})} body={owner ? t('channels.emptyOwner') : t('channels.emptyMember')} action={owner ? {label: t('channels.refreshNow'), onClick: () => store.reset(newSource())} : undefined} />;
  } else if (view === 'guide' && gridFits) body = <GuideGrid {...shared} range={range} />;
  else body = <ChannelList {...shared} />;

  return (
    <Page>
      <div ref={pageRef} className={s.page}>
        <PageHeader
          eyebrow={t('channels.title')}
          title={name}
          actions={<Segmented label={t('channels.view')} size="sm" options={[{id: 'guide' as const, icon: 'calendar' as const, label: t('channels.viewGuide')}, {id: 'list' as const, icon: 'list' as const, label: t('channels.viewList')}]} value={view} onChange={setView} />}
        />
        {picker}
        <div className={s.controls}>
          <div className={s.chipScroll} role="group" aria-label={t('channels.filters')}>
            <Chips>
              <Chip label={t('channels.filterAll')} pressed={!search.group && !search.favorites} onClick={() => setFilter({})} />
              {catalog.favorites || search.favorites ? <Chip label={t('channels.favorites')} icon="starFilled" pressed={!!search.favorites} onClick={() => setFilter({favorites: !search.favorites})} /> : null}
              {catalog.groups.map(g => <Chip key={g} label={g} pressed={search.group === g} onClick={() => setFilter({group: search.group === g ? undefined : g})} />)}
            </Chips>
          </div>
          <span className={s.spacer} />
          <Menu label={t('channels.sort')} trigger={<Button variant="ghost" size="sm" icon="sort" label={sort === 'name' ? t('channels.sortName') : t('channels.sortNumber')} />} items={[
            {id: 'number', label: t('channels.sortNumber'), selected: sort === 'number'},
            {id: 'name', label: t('channels.sortName'), selected: sort === 'name'},
            ...(owner ? [{id: 'hidden', label: t('channels.showHidden'), selected: showHidden, separatorBefore: true}] : []),
          ]} onSelect={id => { if (id === 'hidden') setShowHidden(v => !v); else memory.setSort(id as ChannelSort); }} />
        </div>
        {staleNotice}{wideNotice}
        {body}
      </div>
      {timeline ? <ChannelTimeline channel={timeline} format={format} range={range} loadPrograms={loadPrograms} showSource={entry === ALL} sources={sources} onOpen={setSelection} onWatch={watch} onClose={() => setTimeline(null)} /> : null}
      {selection ? <ProgramSheet selection={selection} format={format} sources={sources} showSource={entry === ALL} onClose={() => setSelection(null)} onWatch={watch} onChanged={() => (forgetGuideReads(), store.invalidate())} onFavorite={c => void setPreference(c, {favorite: !c.favorite, hidden: legacyChannel(c)?.hidden ?? false})} /> : null}
      {toast ? <div className={s.toast} role="status">{toast.text}{toast.action ? <Button size="sm" variant="secondary" label={toast.action.label} onClick={toast.action.onClick} /> : null}</div> : null}
    </Page>
  );
}

export type Shared = {
  store: GuideWindowStore; format: Format; sources: readonly ChannelSource[]; entry: string;
  onOpen: (s: Selection) => void; onWatch: (c: GuideChannelRow) => void; onRecord: (c: GuideChannelRow, p: GuideProgram) => void;
  channelMenu: (c: GuideChannelRow) => MenuItem[]; onChannelMenu: (c: GuideChannelRow, id: string) => void;
  /** Opens a channel's own timeline (§6): its programs by day, from an hour ago. */
  onChannel: (c: GuideChannelRow) => void;
  loadPrograms: (channelId: string, start: number, end: number, signal: AbortSignal) => Promise<readonly GuideProgram[]>;
  /** The rows on screen, so Watch knows where the channel sits for channel up/down (FEAT-07). */
  viewport?: {current: {firstRow: number; lastRow: number}};
  /** Every channel is blocked by the server itself (FEAT-01): explained once, rows not dimmed. */
  serverWide?: boolean;
};

function GuideSkeleton() {
  return <div className={s.grid} aria-hidden>{Array.from({length: 7}).map((_, i) => <div key={i} style={{display: 'flex', height: ROW, borderBottom: '1px solid var(--line-soft)'}}><div style={{width: CH, padding: 16}}><Skeleton height={40} /></div><div style={{flex: 1, padding: 8}}><Skeleton height={56} /></div></div>)}</div>;
}

// ── Channel identity (§2.1) ──────────────────────────────────────

const TINTS = ['blue', 'coral', 'gold', 'mint', 'rose', 'sky', 'slate', 'violet'];
function tintFor(id: string): string {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0;
  return `var(--profile-art-${TINTS[Math.abs(h) % TINTS.length]})`;
}
function initials(name: string): string {
  const words = name.replace(/[^\p{L}\p{N} ]/gu, ' ').split(/\s+/).filter(Boolean);
  return (words.length > 1 ? words[0]![0]! + words[1]![0]! : (words[0] ?? '?').slice(0, 2)).toUpperCase();
}
function ChannelLogo({channel}: {channel: GuideChannelRow}) {
  const {url} = useArtworkUrl(channel.logoUrl);
  if (url) return <span className={s.logo}><img src={url} alt="" /></span>;
  return <span className={s.logo} style={{background: tintFor(channel.id)}} aria-hidden>{initials(channel.name)}</span>;
}
function LibraryMark() {
  const t = useI18n().t;
  return <span className={s.libraryMark} title={t('channels.custom.name')} aria-label={t('channels.custom.name')} role="img"><Icon name="library" size={12} /></span>;
}

function ChannelHeader({channel, shared, showSource}: {channel: GuideChannelRow; shared: Shared; showSource: boolean}) {
  const t = shared.format.t;
  const reason = channel.tuneAvailable ? '' : channelReason(channel.tuneUnavailableReason);
  return (
    <div className={s.channel} role="rowheader">
      <button type="button" className={s.channelMain} tabIndex={-1} title={reason ? `${channel.name} · ${reason}` : channel.name} aria-label={[channel.number ? t('channels.channel', {number: channel.number}) : '', channel.name, reason].filter(Boolean).join(', ')} onClick={() => shared.onChannel(channel)}>
        <ChannelLogo channel={channel} />
        <span className={s.channelCopy}>
          <span className={s.channelName}><span>{channel.name}</span>{channel.favorite ? <Icon name="starFilled" size={12} className={s.star} /> : null}{channel.kind === 'library' ? <LibraryMark /> : null}</span>
          <span className={s.channelMeta}>{channel.number ? <span>{channel.number}</span> : null}{showSource ? <span>{sourceName(shared.sources, channel.sourceId)}</span> : null}</span>
        </span>
      </button>
      <Menu label={t('channels.moreFor', {channel: channel.name})} align="start" trigger={<IconButton name="moreVertical" label={t('channels.moreFor', {channel: channel.name})} variant="ghost" size="sm" className={s.channelMore} tabIndex={-1} />} items={shared.channelMenu(channel)} onSelect={id => shared.onChannelMenu(channel, id)} />
    </div>
  );
}

// ── The guide grid (§3, §7) ──────────────────────────────────────

export function GuideGrid({range, ...shared}: Shared & {range: {start: number; end: number}}) {
  const {store, format} = shared;
  const hover = useHoverCapable();
  const t = format.t;
  const snapshot = useSyncExternalStore(store.subscribe, store.getSnapshot);
  const total = snapshot.total ?? 8;
  const outer = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const width = useWidth(outer);
  const height = useFillHeight(outer);
  const laneWidth = Math.max(COL, width - CH - 2);
  const spanMs = laneWidth / PX_PER_MS;
  const clamp = useCallback((start: number) => Math.max(range.start, Math.min(start, range.end - spanMs)), [range, spanMs]);
  const [viewStart, setViewStart] = useState(() => spanAt(Date.now(), 3 * HOUR_MS).start);
  const viewEnd = viewStart + spanMs;
  // PERF-07: `now` lives in a tiny store. This grid never subscribes: only the now-line
  // (and its ruler tab) and one `LiveProgress` child per on-air block read it, so a 30 s
  // tick never re-renders rows. Layout uses a non-reactive read (`get()`) at render time.
  const nowStore = useMemo(() => createNowStore(30_000), []);
  useEffect(() => () => nowStore.dispose(), [nowStore]);
  const [scrollTop, setScrollTop] = useState(0);
  const bodyHeight = Math.max(0, height - RULER);
  const firstRow = Math.max(0, Math.floor(scrollTop / ROW) - OVERSCAN);
  const lastRow = Math.min(total - 1, Math.ceil((scrollTop + bodyHeight) / ROW) + OVERSCAN);
  useEffect(() => { store.setViewport({firstRow, lastRow, start: viewStart, end: viewEnd}); if (shared.viewport) shared.viewport.current = {firstRow, lastRow}; }, [store, firstRow, lastRow, viewStart, viewEnd]); // eslint-disable-line react-hooks/exhaustive-deps

  const jumpTo = useCallback((anchor: number, lead = 0.25) => setViewStart(clamp(spanAt(anchor, spanMs, lead).start)), [clamp, spanMs]);
  const shift = useCallback((ms: number) => setViewStart(v => clamp(v + ms)), [clamp]);
  // Width known → put now at 25% (§3.4).
  const placed = useRef(false);
  useEffect(() => { if (width && !placed.current) { placed.current = true; jumpTo(Date.now()); } }, [width, jumpTo]);

  // Shift + wheel, or a horizontal trackpad gesture, moves time; the plain wheel scrolls channels (§7).
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      const dx = e.shiftKey && Math.abs(e.deltaX) < Math.abs(e.deltaY) ? e.deltaY : e.deltaX;
      if (Math.abs(dx) < 1 || (!e.shiftKey && Math.abs(e.deltaX) <= Math.abs(e.deltaY))) return;
      e.preventDefault();
      shift(dx / PX_PER_MS);
    };
    el.addEventListener('wheel', onWheel, {passive: false});
    return () => el.removeEventListener('wheel', onWheel);
  }, [shift]);

  // Focus (§4, §7): a row and a focus time; the cell containing the time has the one tab stop.
  // Layout reads come from a non-reactive `get()` so keyboard moves stay fresh without ticking.
  const [focus, setFocus] = useState<{row: number; time: number}>({row: 0, time: Date.now()});
  const cellsOf = useCallback((row: number): GuideCell[] => rowCells(store, row, viewStart, viewEnd, nowStore.get()), [store, viewStart, viewEnd, nowStore]);
  const wantFocus = useRef(false);
  useEffect(() => {
    if (!wantFocus.current) return;
    wantFocus.current = false;
    scroller.current?.querySelector<HTMLElement>('[data-focused="true"]')?.focus({preventScroll: true});
  });
  const scrollRowIntoView = (row: number) => {
    const el = scroller.current;
    if (!el) return;
    const top = row * ROW, bottom = top + ROW;
    if (top < el.scrollTop) el.scrollTop = top;
    else if (bottom > el.scrollTop + bodyHeight) el.scrollTop = bottom - bodyHeight;
  };
  const moveTo = (row: number, time: number) => {
    const r = Math.max(0, Math.min(total - 1, row));
    wantFocus.current = true;
    setFocus({row: r, time});
    scrollRowIntoView(r);
  };
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.target instanceof HTMLElement && !e.target.closest('[role="gridcell"]')) return;
    const cells = cellsOf(focus.row);
    const index = focusTarget(cells, focus.time);
    const cell = cells[index];
    const channel = store.channelAt(focus.row);
    const visibleRows = Math.max(1, Math.floor(bodyHeight / ROW) - 1);
    switch (e.key) {
      case 'ArrowDown': e.preventDefault(); moveTo(focus.row + 1, focus.time); break;
      case 'ArrowUp': e.preventDefault(); moveTo(focus.row - 1, focus.time); break;
      case 'PageDown': e.preventDefault(); moveTo(focus.row + visibleRows, focus.time); break;
      case 'PageUp': e.preventDefault(); moveTo(focus.row - visibleRows, focus.time); break;
      case 'Home': e.preventDefault(); moveTo(0, focus.time); break;
      case 'End': e.preventDefault(); moveTo(total - 1, focus.time); break;
      case 'ArrowRight':
      case 'ArrowLeft': {
        e.preventDefault();
        const dir = e.key === 'ArrowRight' ? 1 : -1;
        const next = index < 0 ? -1 : neighbor(cells, index, dir);
        if (next >= 0) {
          const target = cells[next]!;
          if (target.start < viewStart || target.start >= viewEnd) setViewStart(clamp(target.start - spanMs * 0.25));
          moveTo(focus.row, dir > 0 ? Math.max(target.start, viewStart) : focusTimeOf(target, viewStart, viewEnd));
        } else {
          // No trap: past the edge, time moves by two hours (within the published range).
          const step = dir * 2 * HOUR_MS;
          shift(step);
          moveTo(focus.row, focus.time + step);
        }
        break;
      }
      case 'Enter': case ' ': if (channel) { e.preventDefault(); shared.onOpen({channel, program: cell?.program}); } break;
      case 'w': case 'W': if (channel && !e.metaKey && !e.ctrlKey) { e.preventDefault(); shared.onWatch(channel); } break;
      case 'r': case 'R': if (channel && cell?.program && !e.metaKey && !e.ctrlKey) { e.preventDefault(); shared.onRecord(channel, cell.program); } break;
      case 'n': case 'N': if (!e.metaKey && !e.ctrlKey) { e.preventDefault(); jumpTo(Date.now()); moveTo(focus.row, Date.now()); } break;
    }
  };

  const canEarlier = viewStart > range.start, canLater = viewEnd < range.end;
  // Ticks are laid out without `now`; the subscribing ruler below hides labels near the line.
  // Guide.days (today counts as 1, at most 31) offers exactly those days when the server sends
  // it; otherwise the picker falls back to the available range (7 days).
  const guideDays = useMemo(() => {
    const scoped = shared.entry === ALL ? shared.sources : shared.sources.filter(x => x.id === shared.entry);
    const present = scoped.map(x => x.guideDays).filter((d): d is number => d !== undefined);
    return present.length ? Math.max(...present) : undefined;
  }, [shared.sources, shared.entry]);
  const days = useMemo(() => {
    if (guideDays !== undefined && guideDays >= 1) return stripDays(Date.now(), format.zone, guideDays);
    return dayStarts(range.start, range.end, format.zone);
  }, [guideDays, range, format.zone]);
  const currentDay = localDayStart(viewStart + spanMs * 0.25, format.zone);
  // PERF-07: one stable focus callback (row, time); the row binds its own row inside, so the
  // prop identity never invalidates the memoised row.
  const focusCell = useCallback((row: number, time: number) => setFocus({row, time}), []);
  const rows: React.ReactNode[] = [];
  for (let row = firstRow; row <= lastRow; row++) rows.push(<GuideRow key={row} row={row} shared={shared} viewStart={viewStart} viewEnd={viewEnd} nowStore={nowStore} focus={focus} onFocusCell={focusCell} />);

  return (
    <>
      <div className={s.controls} style={{paddingTop: 0}}>
        <GuideNowButton nowStore={nowStore} viewStart={viewStart} viewEnd={viewEnd} onJump={() => jumpTo(Date.now())} />
        <Menu label={t('guide.day')} align="start" trigger={<Button variant="ghost" size="sm" icon="calendar" iconAfter="chevronDown" label={format.dayLabel(currentDay, nowStore.get())} />} items={days.map(d => ({id: String(d), label: format.dayLabel(d, nowStore.get()), selected: d === currentDay}))} onSelect={id => {
          const day = Number(id);
          // Today → now; another day → 6:00 PM (prime time), §3.4.
          if (day === localDayStart(Date.now(), format.zone)) jumpTo(Date.now());
          else setViewStart(clamp(day + 18 * HOUR_MS - spanMs * 0.25));
        }} />
        {/* FEAT-04: the day strip — Today, then the next 6 days, each to 6:00 PM local that day. */}
        <div className={s.chipScroll} role="group" aria-label={t('guide.day')}>
          <Chips>
            {days.slice(0, 7).map(d => <Chip key={d} label={format.dayLabel(d, nowStore.get())} pressed={d === currentDay} onClick={() => setViewStart(clamp(dayPrimeTime(d, format.zone) - spanMs * 0.25))} />)}
          </Chips>
        </div>
        <span className={s.spacer} />
        {hover ? <Text variant="caption" tone="tertiary">{t('guide.keyboardHint')}</Text> : null}
      </div>
      <div ref={outer} className={s.grid}>
        <div ref={scroller} className={s.scroller} style={{height}} role="grid" aria-label={t('guide.label')} aria-rowcount={total} onKeyDown={onKeyDown} onScroll={e => setScrollTop(e.currentTarget.scrollTop)}>
          <GuideRuler format={format} viewStart={viewStart} viewEnd={viewEnd} pxPerMs={PX_PER_MS} laneWidth={laneWidth} nowStore={nowStore} canEarlier={canEarlier} canLater={canLater} onShift={shift} />
          <div className={s.body} style={{height: total * ROW}}>
            {rows}
            <GuideNowLine nowStore={nowStore} viewStart={viewStart} viewEnd={viewEnd} pxPerMs={PX_PER_MS} />
          </div>
        </div>
      </div>
    </>
  );
}

/**
 * PERF-07: the only grid-level readers of `now`. The Now button's arrow, the ruler's Now tab
 * (with its tick hiding) and the vertical now-line each subscribe to the store; the grid and
 * its rows never do, so a 30 s tick re-renders only these children plus each on-air
 * `LiveProgress` below.
 */
const GuideNowButton = React.memo(function GuideNowButton({nowStore, viewStart, viewEnd, onJump}: {nowStore: NowStore; viewStart: number; viewEnd: number; onJump: () => void}) {
  const {t} = useI18n();
  // Subscribe: the arrow flips only when `now` enters or leaves the span.
  const now = useSyncExternalStore(nowStore.subscribe, nowStore.getSnapshot);
  const off = now < viewStart || now >= viewEnd;
  return <Button variant="outline" size="sm" icon={off ? (now < viewStart ? 'back' : 'forward') : undefined} label={t('guide.now')} aria-label={off ? (now < viewStart ? t('guide.nowEarlier') : t('guide.nowLater')) : t('guide.now')} onClick={onJump} />;
});

function GuideRuler({format, viewStart, viewEnd, pxPerMs, laneWidth, nowStore, canEarlier, canLater, onShift}: {format: Format; viewStart: number; viewEnd: number; pxPerMs: number; laneWidth: number; nowStore: NowStore; canEarlier: boolean; canLater: boolean; onShift: (ms: number) => void}) {
  const t = format.t;
  const now = useSyncExternalStore(nowStore.subscribe, nowStore.getSnapshot);
  const ticks = useMemo(() => rulerTicks(viewStart, viewEnd, pxPerMs, format.zone), [viewStart, viewEnd, pxPerMs, format.zone]);
  const lineX = nowX(now, viewStart, viewEnd, pxPerMs);
  return (
    <div className={s.ruler} role="row">
      <div className={s.rulerCorner} role="columnheader">{t('guide.channelColumn')}</div>
      <div className={s.rulerLane} aria-hidden>
        {/* A label under the Now tab steps aside (the tick line stays). A day tick names the day. */}
        {ticks.map(tick => <span key={tick.atMs} className={cx(s.tick, tick.kind === 'hour' && s.tickHour, tick.kind === 'day' && s.tickDay)} style={{left: tick.x}}>{(lineX !== undefined && tick.x > lineX - 72 && tick.x < lineX + 40) || (canLater && tick.x > laneWidth - 76) || (canEarlier && tick.x < 60) ? '' : tick.kind === 'day' ? format.dayLabel(tick.atMs, now) : format.clock(tick.atMs)}</span>)}
        {lineX !== undefined ? <span className={s.nowTab} style={{left: lineX}}>{t('guide.now')}</span> : null}
        {canEarlier ? <button type="button" className={cx(s.edge, s.edgeLeft)} aria-label={t('guide.earlier')} onClick={() => onShift(-2 * HOUR_MS)}><Icon name="back" size={12} />{t('guide.twoHours')}</button> : null}
        {canLater ? <button type="button" className={cx(s.edge, s.edgeRight)} aria-label={t('guide.later')} onClick={() => onShift(2 * HOUR_MS)}>{t('guide.twoHours')}<Icon name="forward" size={12} /></button> : null}
      </div>
    </div>
  );
}

function GuideNowLine({nowStore, viewStart, viewEnd, pxPerMs}: {nowStore: NowStore; viewStart: number; viewEnd: number; pxPerMs: number}) {
  const now = useSyncExternalStore(nowStore.subscribe, nowStore.getSnapshot);
  const lineX = nowX(now, viewStart, viewEnd, pxPerMs);
  if (lineX === undefined) return null;
  return <div className={s.nowLine} style={{left: CH + lineX}} aria-hidden />;
}

function rowCells(store: GuideWindowStore, row: number, viewStart: number, viewEnd: number, now: number): GuideCell[] {
  const channel = store.channelAt(row);
  if (!channel) return [];
  const opts = {viewStart, viewEnd, pxPerMs: PX_PER_MS, now, gapPx: GAP};
  if (channel.guide === 'none') return layoutRow([], opts);
  const {programs, complete} = store.programsFor(row, viewStart, viewEnd);
  const cells = layoutRow(programs, opts);
  // While blocks are still loading, only real programs are drawn (a hole isn't "No information" yet).
  return complete ? cells : cells.filter(c => c.kind === 'program');
}

const GuideRow = React.memo(function GuideRow({row, shared, viewStart, viewEnd, nowStore, focus, onFocusCell}: {row: number; shared: Shared; viewStart: number; viewEnd: number; nowStore: NowStore; focus: {row: number; time: number}; onFocusCell: (row: number, time: number) => void}) {
  const {store, format} = shared;
  const t = format.t;
  useSyncExternalStore(useCallback(fn => store.subscribePage(store.pageOf(row), fn), [store, row]), () => store.getSnapshot().version);
  // Every hook runs before the placeholder return: a row renders first as a placeholder, then with
  // its channel, and the hook order must not change between those renders.
  const handleFocusCell = useCallback((time: number) => onFocusCell(row, time), [onFocusCell, row]);
  const channel = store.channelAt(row);
  if (!channel) {
    return <div className={s.row} style={{top: row * ROW}} role="row" aria-rowindex={row + 1}><div className={s.channel}><Skeleton width={40} height={40} /><Skeleton width="60%" height={14} /></div><div className={s.lane}><div className={s.loadingBar}><Skeleton height="100%" /></div></div></div>;
  }
  const {complete, failed} = channel.guide === 'none' ? {complete: true, failed: false} : store.programsFor(row, viewStart, viewEnd);
  // PERF-07: layout reads `now` once per row render (no subscription), so a tick never
  // re-renders this row. The on-air progress below subscribes on its own.
  const cells = rowCells(store, row, viewStart, viewEnd, nowStore.get());
  const focusIndex = focus.row === row ? focusTarget(cells, focus.time) : -1;
  return (
    <div className={cx(s.row, !channel.tuneAvailable && !shared.serverWide && s.rowDim)} style={{top: row * ROW}} role="row" aria-rowindex={row + 1}>
      <ChannelHeader channel={channel} shared={shared} showSource={shared.entry === ALL} />
      <div className={s.lane}>
        {!complete && !cells.length && !failed ? <div className={s.loadingBar}><Skeleton height="100%" /></div> : null}
        {failed && !cells.length ? <div className={s.failedBand}>{t('guide.tileFailed')}<Button size="sm" variant="ghost" label={t('action.tryAgain')} onClick={() => store.retry()} /></div> : null}
        {cells.map((cell, i) => <GuideCellView key={cell.program?.id ?? `gap:${cell.start}`} cell={cell} channel={channel} format={format} nowStore={nowStore} focused={i === focusIndex} tabStop={i === focusIndex || (focus.row !== row && row === 0 && i === 0 && focus.row === 0)} viewStart={viewStart} onOpen={shared.onOpen} onFocusCell={handleFocusCell} />)}
      </div>
    </div>
  );
});

const GuideCellView = React.memo(function GuideCellView({cell, channel, format, nowStore, focused, tabStop, viewStart, onOpen, onFocusCell}: {cell: GuideCell; channel: GuideChannelRow; format: Format; nowStore: NowStore; focused: boolean; tabStop: boolean; viewStart: number; onOpen: Shared['onOpen']; onFocusCell: (time: number) => void}) {
  const t = format.t;
  // PERF-07: stable callbacks built inside the memoised block (no inline arrows from the row,
  // so a parent render with identical cells keeps every block's props identical).
  const handleOpen = useCallback(() => onOpen({channel, program: cell.program}), [onOpen, channel, cell.program]);
  const handleFocus = useCallback(() => { if (!focused) onFocusCell(Math.max(cell.start, viewStart)); }, [focused, onFocusCell, cell.start, viewStart]);
  const label = useMemo(() => {
    const now = nowStore.get();
    const p = cell.program;
    const time = format.range(cell.start, cell.end);
    const flags = p?.flags ?? {};
    const flagList = [flags.live ? t('guide.live') : '', flags.new ? t('guide.new') : '', flags.premiere ? t('guide.premiere') : ''].filter(Boolean);
    const episode = p?.episode ? (p.episode.season && p.episode.number ? `S${p.episode.season} E${p.episode.number}` : p.episode.display ?? '') : '';
    const episodeLine = [episode, p?.subtitle].filter(Boolean).join(' · ');
    return [t('guide.cellTime', {start: format.clock(cell.start), end: format.clock(cell.end)}), p ? p.title : t('guide.noInformation'), channel.name, ...flagList, cell.state === 'now' && p ? t('guide.onNowLeft', {count: minutes(cell.end - now)}) : '', p?.recording ? t('guide.recordingScheduled') : ''].filter(Boolean).join(', ');
  }, [format, cell, channel, nowStore, t]);
  const p = cell.program;
  const time = format.range(cell.start, cell.end);
  const flags = p?.flags ?? {};
  const flagList = [flags.live ? t('guide.live') : '', flags.new ? t('guide.new') : '', flags.premiere ? t('guide.premiere') : ''].filter(Boolean);
  const episode = p?.episode ? (p.episode.season && p.episode.number ? `S${p.episode.season} E${p.episode.number}` : p.episode.display ?? '') : '';
  const episodeLine = [episode, p?.subtitle].filter(Boolean).join(' · ');
  const onNow = cell.state === 'now' && cell.kind === 'program';
  // Programme facts are static per programme (PERF-07: safe as props). Blocks ≥ 180 px wide
  // show the episode line even below the full tier (§3.3 text tiers start full at 200 px).
  const wide = cell.width >= 180;
  return (
    <div
      role="gridcell"
      tabIndex={tabStop ? 0 : -1}
      data-focused={focused ? 'true' : undefined}
      aria-label={label}
      title={p && cell.tier !== 'full' ? `${p.title} · ${time}` : undefined}
      className={cx(s.cell, cell.kind === 'gap' && s.gap, onNow && s.cellNow, cell.state === 'past' && s.cellPast, cell.tier === 'sliver' && s.cellSliver)}
      style={{left: cell.x, width: cell.width}}
      onClick={handleOpen}
      onFocus={handleFocus}
    >
      {cell.tier === 'sliver' ? null : (
        <>
          <span className={s.cellTitle}>
            {cell.clippedStart ? <span className={s.clip} aria-hidden>‹</span> : null}
            {p?.recording ? <span className={s.recDot} aria-hidden /> : null}
            <span>{p ? p.title : t('guide.noInformation')}</span>
            {cell.tier === 'title2' && flagList.length ? <span className={s.flagDot} aria-hidden /> : null}
            {cell.clippedEnd && cell.tier !== 'full' ? <span className={s.clip} aria-hidden>›</span> : null}
          </span>
          {cell.tier === 'full' && p ? (
            <span className={s.cellSub}>
              <span>{onNow ? t('guide.ends', {time: format.clock(cell.end)}) : time}</span>
              {flags.live ? <span className={cx(s.flag, s.flagLive)}>{t('guide.live')}</span> : null}
              {flags.new ? <span className={s.flag}>{t('guide.new')}</span> : null}
              {flags.premiere ? <span className={s.flag}>{t('guide.premiere')}</span> : null}
              {episodeLine ? <span>{episodeLine}</span> : null}
              {cell.clippedEnd ? <span className={s.clip} aria-hidden>›</span> : null}
            </span>
          ) : null}
          {cell.tier !== 'full' && wide && p && episodeLine ? (
            <span className={s.cellSub}>
              <span>{episodeLine}</span>
              {flags.live ? <span className={cx(s.flag, s.flagLive)}>{t('guide.live')}</span> : null}
              {flags.new ? <span className={s.flag}>{t('guide.new')}</span> : null}
              {flags.premiere ? <span className={s.flag}>{t('guide.premiere')}</span> : null}
            </span>
          ) : null}
          {cell.tier === 'full' && !p && !cell.clippedStart && !cell.clippedEnd ? <span className={s.cellSub}>{time}</span> : null}
        </>
      )}
      {onNow ? <LiveProgress cell={cell} nowStore={nowStore} /> : null}
    </div>
  );
});

/** PERF-07: the progress of the one on-air block per row. The only `now` reader in rows. */
const LiveProgress = React.memo(function LiveProgress({cell, nowStore}: {cell: GuideCell; nowStore: NowStore}) {
  const now = useSyncExternalStore(nowStore.subscribe, nowStore.getSnapshot);
  const progress = Math.max(0, Math.min(1, (now - cell.start) / Math.max(1, cell.end - cell.start)));
  if (progress <= 0) return null;
  return <span className={s.progress} style={{width: `${progress * 100}%`}} aria-hidden />;
});

// ── The list view: now / next with progress (§6, §7) ─────────────

export function ChannelList(shared: Shared) {
  const {store, format} = shared;
  const t = format.t;
  const rowHeight = useCompact() ? LIST_ROW_COMPACT : LIST_ROW;
  const snapshot = useSyncExternalStore(store.subscribe, store.getSnapshot);
  const total = snapshot.total ?? 8;
  const outer = useRef<HTMLDivElement>(null);
  const height = useFillHeight(outer);
  // PERF-07: same store as the grid. This list never subscribes: only each on-air row's
  // `ListLiveMeta` below reads `now`, so a 30 s tick never re-renders rows.
  const nowStore = useMemo(() => createNowStore(30_000), []);
  useEffect(() => () => nowStore.dispose(), [nowStore]);
  const [scrollTop, setScrollTop] = useState(0);
  const firstRow = Math.max(0, Math.floor(scrollTop / rowHeight) - OVERSCAN);
  const lastRow = Math.min(total - 1, Math.ceil((scrollTop + height) / rowHeight) + OVERSCAN);
  // Window from a non-reactive read: it moves on scroll/viewport renders, not on tick.
  const anchor = nowStore.get();
  const start = Math.floor(anchor / SLOT_MS) * SLOT_MS - HOUR_MS, end = start + 4 * HOUR_MS;
  useEffect(() => { store.setViewport({firstRow, lastRow, start, end}); if (shared.viewport) shared.viewport.current = {firstRow, lastRow}; }, [store, firstRow, lastRow, start, end]); // eslint-disable-line react-hooks/exhaustive-deps
  const rows: React.ReactNode[] = [];
  for (let row = firstRow; row <= lastRow; row++) rows.push(<ListRowView key={row} row={row} shared={shared} nowStore={nowStore} start={start} end={end} height={rowHeight} />);
  return (
    <div ref={outer} className={s.list}>
      <div className={s.listScroller} style={{height}} onScroll={e => setScrollTop(e.currentTarget.scrollTop)} role="list" aria-label={t('channels.title')}>
        <div style={{position: 'relative', height: total * rowHeight}}>{rows}</div>
      </div>
    </div>
  );
}

const ListRowView = React.memo(function ListRowView({row, shared, nowStore, start, end, height}: {row: number; shared: Shared; nowStore: NowStore; start: number; end: number; height: number}) {
  const {store, format} = shared;
  const t = format.t;
  useSyncExternalStore(useCallback(fn => store.subscribePage(store.pageOf(row), fn), [store, row]), () => store.getSnapshot().version);
  const channel = store.channelAt(row);
  // Every hook before the placeholder return (same rule as GuideRow above).
  const openChannel = useCallback(() => { if (channel) shared.onChannel(channel); }, [shared, channel]);
  const watchChannel = useCallback(() => { if (channel) shared.onWatch(channel); }, [shared, channel]);
  const openMenu = useCallback((id: string) => { if (channel) shared.onChannelMenu(channel, id); }, [shared, channel]);
  if (!channel) return <div className={s.listRow} style={{top: row * height, height}} role="listitem"><Skeleton width={40} height={40} /><Skeleton width="50%" height={14} /></div>;
  const {programs, complete} = channel.guide === 'none' ? {programs: [], complete: true} : store.programsFor(row, start, end);
  // Layout from a non-reactive read; the live progress + "left" line below subscribes on its own.
  const nn = nowNext(programs, nowStore.get());
  const current = nn.now, next = nn.next;
  const reason = channel.tuneAvailable ? '' : channelReason(channel.tuneUnavailableReason);
  return (
    <div className={cx(s.listRow)} style={{top: row * height, height, opacity: channel.tuneAvailable || shared.serverWide ? 1 : 0.6}} role="listitem">
      <button type="button" className={s.listMain} onClick={openChannel} aria-label={[channel.number, channel.name, current ? current.title : t('guide.noGuide'), reason].filter(Boolean).join(', ')} title={reason || undefined}>
        <ChannelLogo channel={channel} />
        <span className={s.listCopy}>
          <span className={s.channelName}><span>{[channel.number, channel.name].filter(Boolean).join(' ')}</span>{channel.favorite ? <Icon name="starFilled" size={12} className={s.star} /> : null}{channel.kind === 'library' ? <LibraryMark /> : null}{shared.entry === ALL ? <span className={s.channelMeta}>{sourceName(shared.sources, channel.sourceId)}</span> : null}</span>
          {current ? (
            <span className={s.listNow}>
              <span>{current.title} · {format.range(current.start, current.end)}</span>
              <ListLiveMeta nowStore={nowStore} format={format} program={current} />
            </span>
          ) : <span className={s.listNow}><span>{complete ? t('guide.noGuide') : ' '}</span></span>}
          {next ? <span className={s.listNext}>{t('guide.next', {title: next.title, time: format.clock(next.start)})}</span> : null}
        </span>
      </button>
      {channel.tuneAvailable ? <IconButton name="play" label={t('channels.watchChannel', {channel: channel.name})} variant="ghost" onClick={watchChannel} /> : null}
      <Menu label={t('channels.moreFor', {channel: channel.name})} trigger={<IconButton name="moreVertical" label={t('channels.moreFor', {channel: channel.name})} variant="ghost" size="sm" />} items={shared.channelMenu(channel)} onSelect={openMenu} />
    </div>
  );
});

/** PERF-07: the one `now` reader in a list row: progress + "left" for the on-air program. */
const ListLiveMeta = React.memo(function ListLiveMeta({nowStore, format, program}: {nowStore: NowStore; format: Format; program: GuideProgram}) {
  const now = useSyncExternalStore(nowStore.subscribe, nowStore.getSnapshot);
  const value = Math.max(0, Math.min(1, (now - program.start) / Math.max(1, program.end - program.start)));
  return (
    <>
      <span className={s.listProgress} aria-hidden><i style={{width: `${value * 100}%`}} /></span>
      <span className={s.listLeft}>{timeLeft(format, program.end - now)}</span>
    </>
  );
});

// ── A channel's timeline (§6): its programs from an hour ago, by day, windowed by time ──

const TIMELINE_BLOCKS = 2;
export function ChannelTimeline({channel, format, range, loadPrograms, showSource, sources, onOpen, onWatch, onClose}: {channel: GuideChannelRow; format: Format; range: {start: number; end: number}; loadPrograms: Shared['loadPrograms']; showSource: boolean; sources: readonly ChannelSource[]; onOpen: (s: Selection) => void; onWatch: (c: GuideChannelRow) => void; onClose: () => void}) {
  const t = format.t;
  const now = useNow();
  const first = Math.max(range.start, Math.floor((Date.now() - HOUR_MS) / BLOCK_MS) * BLOCK_MS);
  const [loadedTo, setLoadedTo] = useState(first);
  const [blocks, setBlocks] = useState<Map<number, readonly GuideProgram[]>>(new Map());
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);
  const sentinel = useRef<HTMLDivElement>(null);
  const placed = useRef(false);
  const more = useCallback(() => {
    if (loading || loadedTo >= range.end) return;
    const controller = new AbortController();
    const from = loadedTo, to = Math.min(range.end, from + TIMELINE_BLOCKS * BLOCK_MS);
    setLoading(true); setFailed(false);
    loadPrograms(channel.id, from, to, controller.signal).then(list => {
      setBlocks(prev => new Map(prev).set(from, list));
      setLoadedTo(to);
    }, () => setFailed(true)).finally(() => setLoading(false));
    return () => controller.abort();
  }, [loading, loadedTo, range.end, loadPrograms, channel.id]);
  useEffect(() => { if (!blocks.size) more(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  // Windowed by time: the next blocks load as the end of the list comes into view.
  useEffect(() => {
    const el = sentinel.current, root = scroller.current;
    if (!el || !root) return;
    const io = new IntersectionObserver(entries => { if (entries.some(e => e.isIntersecting)) more(); }, {root, rootMargin: '400px'});
    io.observe(el);
    return () => io.disconnect();
  }, [more]);
  const programs = useMemo(() => normalizePrograms([...blocks.values()].flat()).filter(p => p.end > first), [blocks, first]);
  // Opens on the program on now.
  useEffect(() => {
    if (placed.current || !programs.length) return;
    placed.current = true;
    requestAnimationFrame(() => scroller.current?.querySelector<HTMLElement>('[data-now="true"]')?.scrollIntoView({block: 'center'}));
  }, [programs.length]);
  const days: {day: number; items: GuideProgram[]}[] = [];
  for (const p of programs) {
    const day = localDayStart(p.start, format.zone);
    if (days.at(-1)?.day !== day) days.push({day, items: []});
    days.at(-1)!.items.push(p);
  }
  return (
    <Dialog open onOpenChange={o => !o && onClose()} placement="side" width={460} title={[channel.number, channel.name].filter(Boolean).join(' · ')} description={showSource ? sourceName(sources, channel.sourceId) : undefined}
      actions={<Button variant="primary" icon="play" label={channel.kind === 'library' ? t('channels.watch') : t('guide.watchLive')} disabled={!channel.tuneAvailable} onClick={() => { onWatch(channel); onClose(); }} />}>
      <div ref={scroller} className={s.timeline}>
        {channel.guide === 'none' || (!loading && !programs.length && loadedTo >= Math.min(range.end, first + TIMELINE_BLOCKS * BLOCK_MS) && !failed) ? <Text as="p" variant="body" tone="tertiary">{t('guide.noGuide')}</Text> : null}
        {days.map(d => (
          <section key={d.day} className={s.timelineDay} aria-label={format.dayLabel(d.day, now)}>
            <h3 className={s.timelineHeader}>{format.dayLabel(d.day, now)}</h3>
            {d.items.map(p => {
              const onNow = p.start <= now && p.end > now, past = p.end <= now;
              const episode = p.episode ? (p.episode.season && p.episode.number ? `S${p.episode.season} E${p.episode.number}` : p.episode.display ?? '') : '';
              return (
                <button key={p.id} type="button" className={cx(s.timelineRow, onNow && s.timelineNow, past && s.cellPast)} data-now={onNow ? 'true' : undefined} onClick={() => onOpen({channel, program: p})} aria-label={[t('guide.cellTime', {start: format.clock(p.start), end: format.clock(p.end)}), p.title, onNow ? t('guide.onNowLeft', {count: minutes(p.end - now)}) : ''].filter(Boolean).join(', ')}>
                  <span className={s.timelineTime}>{format.clock(p.start)}</span>
                  <span className={s.timelineCopy}>
                    <span className={s.cellTitle}>{p.recording ? <span className={s.recDot} aria-hidden /> : null}<span>{p.title}</span>{p.flags?.new ? <span className={s.flag}>{t('guide.new')}</span> : null}{p.flags?.live ? <span className={cx(s.flag, s.flagLive)}>{t('guide.live')}</span> : null}</span>
                    <span className={s.cellSub}>{[format.range(p.start, p.end), episode, p.subtitle].filter(Boolean).join(' · ')}</span>
                    {onNow ? <span className={s.listProgress} style={{width: '100%', maxWidth: 'none'}} aria-hidden><i style={{width: `${((now - p.start) / (p.end - p.start)) * 100}%`}} /></span> : null}
                  </span>
                </button>
              );
            })}
          </section>
        ))}
        {failed ? <Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: () => void more()}}>{t('guide.tileFailed')}</Notice> : null}
        {loading ? <div className={s.timelineLoading}><Skeleton height={48} /><Skeleton height={48} /></div> : null}
        <div ref={sentinel} aria-hidden style={{height: 1}} />
      </div>
    </Dialog>
  );
}

// ── The program sheet (§3.5, §7) ─────────────────────────────────

export function ProgramSheet({selection, format, sources, showSource, onClose, onWatch, onChanged, onFavorite}: {selection: Selection; format: Format; sources: readonly ChannelSource[]; showSource: boolean; onClose: () => void; onWatch: (c: GuideChannelRow) => void; onChanged: () => void; onFavorite: (c: GuideChannelRow) => void}) {
  const t = format.t;
  const {channel, program} = selection;
  const navigate = useNavigate();
  const dvr = useDVRClient();
  const now = Date.now();
  const rawChannel = legacyChannel(channel);
  const rawProgram = program ? legacyProgramme(program) : undefined;
  const airing = !!program && program.start <= now && program.end > now;
  const ended = !!program && program.end <= now;
  const future = !!program && program.start > now;
  const [busy, setBusy] = useState<'one' | 'series' | null>(null);
  const [error, setError] = useState('');
  const [done, setDone] = useState<'one' | 'series' | null>(null);
  const [recordAsk, setRecordAsk] = useState(false);
  const [seriesAsk, setSeriesAsk] = useState(false);
  const [episodes, setEpisodes] = useState<'new' | 'all'>('new');
  const [anyChannel, setAnyChannel] = useState(false);
  // FEAT-02: padding for a one-time record and for a series rule, from the same bounded
  // choices as the DVR rule editor (`dvrPaddings`, within `validRecordingOptions`).
  const [beforeSeconds, setBeforeSeconds] = useState(0);
  const [afterSeconds, setAfterSeconds] = useState(0);
  // FEAT-02: how many episodes a series rule keeps (0 keeps every episode).
  const [keep, setKeep] = useState(0);
  const minutes = (seconds: number) => (seconds ? t('web.dvr.minutes', {count: seconds / 60}) : t('web.dvr.none'));
  const reminders = useSyncExternalStore(subscribeReminders, () => JSON.stringify(readReminders().map(r => r.programId)));
  const reminded = !!program && reminders.includes(`"${program.id}"`);
  const canRecord = !!dvr && !!rawChannel && !!rawProgram && channel.kind === 'live' && channel.recordAvailable && !ended;
  const draft: RecordingDraft | undefined = rawChannel && rawProgram ? {channel: rawChannel, programme: rawProgram, series: false} : undefined;
  const scheduled = !!program?.recording || done === 'one';
  const record = async (kind: 'one' | 'series') => {
    if (!dvr || !draft || busy) return;
    setBusy(kind); setError('');
    try {
      const padding = {beforeSeconds, afterSeconds};
      if (kind === 'one') await dvr.schedule(draft, {...defaultRecordingOptions, ...padding});
      else {
        // The series rule editor's state (new/all, keep, padding, channel scope) maps through
        // the shared `seriesRuleConfig`, as the DVR rule editor does.
        const {config, anchor} = seriesRuleConfig({...draft, series: true}, {episodes, keep, beforeSeconds, afterSeconds, anyChannel});
        await dvr.saveRule(await dvr.newRuleID(), 0, config, anchor);
      }
      setDone(kind); setRecordAsk(false); setSeriesAsk(false);
      onChanged();
    } catch (e) { setError(errorText(e, 'live', 'save')); } finally { setBusy(null); }
  };
  const reason = channel.tuneAvailable ? '' : channelReason(channel.tuneUnavailableReason);
  const description = program?.description ?? '';
  const when = program ? `${format.dayLabel(program.start, now)}, ${format.range(program.start, program.end)}` : '';
  const flags = program?.flags ?? {};
  const episode = program?.episode ? (program.episode.season && program.episode.number ? `S${program.episode.season} E${program.episode.number}` : program.episode.display ?? '') : '';
  const episodeLine = [episode, program?.subtitle].filter(Boolean).join(' · ');
  // Programme facts are static per programme (PERF-07: safe as props). Year, rating and the
  // first 3 categories read as one line; the star rating keeps its own badge when present.
  const facts = program ? [program.year ? String(program.year) : '', program.rating ?? '', (program.categories ?? []).slice(0, 3).join(', ')].filter(Boolean).join(' · ') : '';
  const viewRecordings = {label: t('guide.viewRecordings'), onClick: () => { onClose(); void navigate({to: '/channels/$entry', params: {entry: RECORDINGS}}); }};
  const watchLabel = channel.kind === 'library' ? t('channels.watch') : !program || program.start > now || ended ? t('guide.watchChannel') : t('guide.watchLive');
  const showWatch = !program || airing || ended || !channel.tuneAvailable;
  const reminder: Reminder | undefined = program && rawChannel ? {programId: program.id, channelId: channel.id, title: program.title, channelName: channel.name, start: program.start} : undefined;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} placement="side" width={440} title={program?.title ?? channel.name} description={
      <span className={s.sheetHead}>
        <ChannelLogo channel={channel} />
        <span style={{display: 'flex', flexDirection: 'column', minWidth: 0}}>
          <span className={s.channelName} style={{color: 'var(--text-primary)'}}><span>{[channel.number, channel.name].filter(Boolean).join(' · ')}</span>{channel.kind === 'library' ? <LibraryMark /> : null}</span>
          {showSource ? <span className={s.channelMeta}>{sourceName(sources, channel.sourceId)}</span> : null}
        </span>
      </span>
    }>
      <div className={s.sheetBody}>
        {program?.image ? <div style={{maxWidth: 160}}><Artwork path={program.image} shape="poster" alt={program.title} /></div> : null}
        {program ? (
          <>
            {episodeLine ? <Text as="p" variant="bodyStrong">{episodeLine}</Text> : null}
            <Text as="p" variant="body" tone="secondary">{when}</Text>
            {facts ? <Text as="p" variant="body" tone="secondary">{facts}</Text> : null}
            <div className={s.sheetChips}>
              {airing ? <Badge tone="record" live>{t('guide.onNow')}</Badge> : null}
              {flags.live ? <Badge tone="record">{t('guide.live')}</Badge> : null}
              {flags.new ? <Badge tone="accent">{t('guide.new')}</Badge> : null}
              {flags.premiere ? <Badge tone="accent">{t('guide.premiere')}</Badge> : null}
              {program.rating ? <Badge tone="neutral">{program.rating}</Badge> : null}
              {(program.categories ?? []).slice(0, 3).map(c => <Badge key={c} tone="neutral">{c}</Badge>)}
              {program.starRating ? <Badge tone="neutral">{t('guide.starRating', {value: program.starRating})}</Badge> : null}
              {scheduled ? <Badge tone="record" dot>{t('guide.recordingScheduled')}</Badge> : null}
            </div>
            {airing ? <Text as="p" variant="caption" tone="secondary">{t('guide.startedLeft', {started: minutes(now - program.start), left: minutes(program.end - now)})}</Text> : null}
          </>
        ) : <Text as="p" variant="body" tone="secondary">{t('guide.noInformation')}</Text>}
        {reason ? <Notice tone="warning" compact>{reason}</Notice> : null}
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        {done === 'one' ? <Notice tone="success" compact action={viewRecordings}>{t('guide.scheduledBody')}</Notice> : null}
        {done === 'series' ? <Notice tone="success" compact action={viewRecordings}>{t('guide.seriesSet')}</Notice> : null}
        <div className={s.sheetActions}>
          {showWatch ? <Button variant="primary" icon="play" label={watchLabel} disabled={!channel.tuneAvailable} onClick={() => { onWatch(channel); onClose(); }} /> : null}
          {canRecord && !scheduled ? <Button variant={future ? 'primary' : 'secondary'} icon="dvr" label={t('guide.record')} disabled={!!busy} onClick={() => setRecordAsk(true)} /> : null}
          {canRecord && program?.seriesId && done !== 'series' ? <Button variant="secondary" icon="repeat" label={t('guide.recordSeries')} disabled={!!busy} onClick={() => setSeriesAsk(true)} /> : null}
          {future && reminder ? <Button variant="secondary" icon={reminded ? 'close' : 'bell'} label={reminded ? t('guide.stopRemind') : t('guide.remind')} onClick={() => toggleReminder(reminder)} /> : null}
          <Button variant="ghost" icon={channel.favorite ? 'starFilled' : 'star'} label={channel.favorite ? t('channels.removeFavorite') : t('channels.addFavorite')} onClick={() => { onFavorite(channel); onClose(); }} />
        </div>
        {reminded ? <Text as="p" variant="caption" tone="tertiary">{t('guide.reminderSet')}</Text> : null}
        {program ? <Text as="p" variant="body" tone={description ? 'secondary' : 'tertiary'} style={{whiteSpace: 'pre-wrap'}}>{description || t('guide.noDescription')}</Text> : null}
      </div>
      {recordAsk ? (
        <Dialog open onOpenChange={setRecordAsk} title={t('guide.record')} description={program?.title} width={400} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => setRecordAsk(false)} /><Button variant="primary" icon="dvr" label={t('guide.record')} loading={busy === 'one'} onClick={() => void record('one')} /></>}>
          <div className={s.radioRow} role="group" aria-label={t('guide.record')}>
            <Select label={t('web.dvr.startEarly')} value={String(beforeSeconds)} onChange={e => setBeforeSeconds(Number(e.target.value))} options={dvrPaddings.map(n => ({value: String(n), label: minutes(n)}))} />
            <Select label={t('web.dvr.endLate')} value={String(afterSeconds)} onChange={e => setAfterSeconds(Number(e.target.value))} options={dvrPaddings.map(n => ({value: String(n), label: minutes(n)}))} />
          </div>
        </Dialog>
      ) : null}
      {seriesAsk ? (
        <Dialog open onOpenChange={setSeriesAsk} title={t('guide.recordSeriesTitle')} description={program?.title} width={400} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => setSeriesAsk(false)} /><Button variant="primary" label={t('guide.recordSeriesTitle')} loading={busy === 'series'} onClick={() => void record('series')} /></>}>
          <div className={s.radioRow} role="radiogroup" aria-label={t('guide.recordSeriesTitle')}>
            <Segmented label={t('guide.recordSeriesTitle')} options={[{id: 'new' as const, label: t('guide.newOnly')}, {id: 'all' as const, label: t('guide.allEpisodes')}]} value={episodes} onChange={setEpisodes} />
            <Segmented label={t('guide.recordSeriesTitle')} options={[{id: 'this' as const, label: t('guide.thisChannel')}, {id: 'any' as const, label: t('guide.anyChannel')}]} value={anyChannel ? 'any' : 'this'} onChange={v => setAnyChannel(v === 'any')} />
            <Select label={t('web.dvr.keepLabel')} value={String(keep)} onChange={e => setKeep(Number(e.target.value))} options={dvrLimits.map(n => ({value: String(n), label: n ? t('web.dvr.keepLatest', {count: n}) : t('web.dvr.keepAll')}))} />
            <Select label={t('web.dvr.startEarly')} value={String(beforeSeconds)} onChange={e => setBeforeSeconds(Number(e.target.value))} options={dvrPaddings.map(n => ({value: String(n), label: minutes(n)}))} />
            <Select label={t('web.dvr.endLate')} value={String(afterSeconds)} onChange={e => setAfterSeconds(Number(e.target.value))} options={dvrPaddings.map(n => ({value: String(n), label: minutes(n)}))} />
          </div>
        </Dialog>
      ) : null}
    </Dialog>
  );
}

/**
 * "Remind me" (§3.5, decision 3): while a Portico tab is open, a toast 2 minutes before a reminded
 * program starts, with Watch. Mounted once in the shell.
 */
export function ChannelReminders() {
  const t = useI18n().t;
  const navigate = useNavigate();
  const list = useSyncExternalStore(subscribeReminders, () => JSON.stringify(readReminders()));
  const [due, setDue] = useState<Reminder | null>(null);
  useEffect(() => {
    const reminders = JSON.parse(list) as Reminder[];
    const timers = reminders.map(r => {
      const at = r.start - 2 * MINUTE_MS - Date.now();
      if (at > 2 ** 31 - 1) return undefined;
      return setTimeout(() => { setDue(r); removeReminder(r.programId); }, Math.max(0, at));
    });
    return () => timers.forEach(id => id !== undefined && clearTimeout(id));
  }, [list]);
  useEffect(() => { if (!due) return; const id = setTimeout(() => setDue(null), 60_000); return () => clearTimeout(id); }, [due]);
  if (!due) return null;
  return (
    <div className={s.toast} role="status">
      <Icon name="bell" size={16} />
      {t('guide.reminder', {title: due.title, channel: due.channelName})}
      <Button size="sm" variant="primary" label={t('channels.watch')} onClick={() => { setDue(null); void navigate({to: '/channels'}); }} />
      <IconButton name="close" label={t('action.dismiss')} variant="ghost" size="sm" onClick={() => setDue(null)} />
    </div>
  );
}

export {hasReminder};

function ServerWideNotice({reason, library}: {reason: ServerWide; library: boolean}) {
  const {owner} = useSession();
  const navigate = useNavigate();
  const {t} = useI18n();
  const body = !owner ? t('web.live.serverWide.memberBody') : reason === 'platform' ? t('web.live.serverWide.platformBody') : reason === 'tools' ? t('web.live.serverWide.toolsBody') : t('web.live.serverWide.deliveryBody');
  return <Notice tone="info" compact title={t('web.live.serverWide.title')} action={owner ? {label: library ? t('web.live.openChannelSettings') : t('web.live.openLiveSettings'), onClick: () => void navigate({to: '/settings/$section', params: {section: 'server-live'}, search: {}})} : undefined}>{body}</Notice>;
}
