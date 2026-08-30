import type { ActionableLiveTVChannel, ActionableLiveTVProgram, ActionableLiveTVSource } from '../../data/models';
import { MediaCalendarIcon, NavigationPreviousIcon, NavigationDisclosureIcon, MetadataTimeIcon, PlaybackPlayIcon, NavigationSearchIcon, ActionRateIcon, MediaVideoIcon, ActionCloseIcon } from '#portico-icons';
import { type ComponentType, type CSSProperties, type KeyboardEvent, useDeferredValue, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { productText, reviewedProductErrorText } from '../../components/ProductLanguage';
import { useLiveTVMutations, usePorticoDataSource } from '../../data/DataProvider';
import { usePlaybackSession } from '../player/PlayerSurface';
import { LiveChoiceMenu } from './LiveControls';
import { createServerClockSample, hasAction, liveActions, serverClockNow, type ServerClockSample } from './liveActions';
import { dateLabel, initialGuideStart, isConflictError, localDay, productState, timeLabel } from './liveFormat';
import { useChannelPage, useGuidePage } from './liveQueries';

export const guideHours = 3;
const guidePageSize = 50;

function sourceLogo(source: ReturnType<typeof usePorticoDataSource>, channel: ActionableLiveTVChannel | undefined) {
  return channel?.logoUrl ? source.playbackResourceUrl(channel.logoUrl) : '';
}

function programIsLive(program: ActionableLiveTVProgram | undefined, serverTime: string | undefined) {
  if (!program || !serverTime) return false;
  const now = new Date(serverTime).getTime();
  return new Date(program.startAt).getTime() <= now && new Date(program.endAt).getTime() > now;
}

export function GuideWorkspace({
  sources,
  sourceId,
  setSourceId,
  StateSurface,
}: {
  sources: ActionableLiveTVSource[];
  sourceId: string;
  setSourceId: (id: string) => void;
  StateSurface: ComponentType<{ kind: 'loading' | 'empty' | 'error' | 'permission'; title?: string; message: string; onRetry?: () => void }>;
}) {
  const source = usePorticoDataSource();
  const navigate = useNavigate();
  const player = usePlaybackSession();
  const mutations = useLiveTVMutations();
  const [windowStart, setWindowStart] = useState(() => initialGuideStart());
  const [query, setQuery] = useState('');
  const deferredQuery = useDeferredValue(query.trim());
  const [favoritesOnly, setFavoritesOnly] = useState(false);
  const [group, setGroup] = useState('all');
  const [cursors, setCursors] = useState<string[]>([]);
  const [revision, setRevision] = useState(0);
  const [selectedProgramId, setSelectedProgramId] = useState('');
  const [selectedChannelId, setSelectedChannelId] = useState('');
  const [actionError, setActionError] = useState('');
  const [actionNotice, setActionNotice] = useState('');
  const [busy, setBusy] = useState('');
  const [scheduledPrograms, setScheduledPrograms] = useState<Set<string>>(() => new Set());
  const [serverClockSample, setServerClockSample] = useState<ServerClockSample>();
  const [, setClockTick] = useState(0);
  const { query: guide, supportsPaging } = useGuidePage(sourceId, {
    from: windowStart.toISOString(),
    hours: guideHours,
    query: deferredQuery || undefined,
    favoritesOnly,
    group: group === 'all' ? undefined : group,
    order: 'asc',
    limit: guidePageSize,
    cursor: cursors.at(-1),
  }, revision);
  const { query: channelFallback, supportsPaging: fallbackSupportsPaging } = useChannelPage(sourceId, {
    query: deferredQuery || undefined,
    favoritesOnly,
    group: group === 'all' ? undefined : group,
    limit: guidePageSize,
    cursor: cursors.at(-1),
  }, revision, guide.status === 'error');

  const guideData = guide.status === 'success' ? guide.data : undefined;
  useEffect(() => {
    if (!guideData) {
      setServerClockSample(undefined);
      return;
    }
    setServerClockSample(createServerClockSample(guideData.serverTime));
  }, [guideData]);
  useEffect(() => {
    if (!guideData) return;
    const timer = window.setInterval(() => setClockTick((current) => current + 1), 1_000);
    return () => window.clearInterval(timer);
  }, [guideData]);

  useEffect(() => setCursors([]), [deferredQuery, favoritesOnly, group, sourceId, windowStart]);
  useEffect(() => {
    setGroup('all');
    setSelectedChannelId('');
    setSelectedProgramId('');
  }, [sourceId]);
  useEffect(() => {
    if (guide.status !== 'success') return;
    if (guide.data.channels.some((channel) => channel.id === selectedChannelId)) return;
    setSelectedChannelId('');
    setSelectedProgramId('');
  }, [guide, selectedChannelId]);

  const usingChannelFallback = guide.status === 'error' && channelFallback.status === 'success';
  const rawGroups = guide.status === 'success'
    ? guide.data.channelGroups ?? (!guide.data.pageInfo.hasMore ? [...new Set(guide.data.channels.flatMap((channel) => channel.groupTitle ? [channel.groupTitle] : []))].sort() : [])
    : usingChannelFallback ? channelFallback.data.groups ?? [] : [];
  const locallyFilterGroup = !supportsPaging && group !== 'all';
  const channels = guide.status === 'success'
    ? guide.data.channels.filter((channel) => !locallyFilterGroup || channel.groupTitle === group)
    : usingChannelFallback ? channelFallback.data.items : [];
  const channelIds = new Set(channels.map((channel) => channel.id));
  const programs = guide.status === 'success'
    ? guide.data.programs.filter((program) => !program.channelId || channelIds.has(program.channelId))
    : [];
  const responseServerTime = guideData ? Date.parse(guideData.serverTime) : Number.NaN;
  const responseNow = Number.isFinite(responseServerTime) ? responseServerTime : Date.now();
  const now = guideData && serverClockSample?.serverTimeMs === responseServerTime
    ? serverClockNow(serverClockSample)
    : responseNow;
  const serverTime = new Date(now).toISOString();
  const currentProgram = programs.find((program) => new Date(program.startAt).getTime() <= now && new Date(program.endAt).getTime() > now);
  const selectedProgram = programs.find((program) => program.id === selectedProgramId);
  const focusedProgram = selectedProgram ?? currentProgram;
  const focusedChannel = channels.find((channel) => channel.id === (selectedChannelId || focusedProgram?.channelId)) ?? channels[0];
  const focusedChannelProgram = focusedProgram?.channelId === focusedChannel?.id ? focusedProgram : programs.find((program) => program.channelId === focusedChannel?.id && new Date(program.startAt).getTime() <= now && new Date(program.endAt).getTime() > now);
  const capabilities = guide.status === 'success' ? guide.data.capabilities : undefined;
  const canPlay = capabilities?.canPlay ?? channels.some((channel) => hasAction(channel, liveActions.play));
  const logo = (channel: ActionableLiveTVChannel | undefined) => sourceLogo(source, channel);

  const watch = async (channel: ActionableLiveTVChannel) => {
    setActionError('');
    setActionNotice('');
    setBusy(`watch:${channel.id}`);
    try {
      const playback = await player.startLive(channel.id);
      if (playback) navigate(`/watch/${playback.media.id}`);
    } catch (reason) {
      setActionError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'open this channel' }));
    } finally {
      setBusy('');
    }
  };

  const favorite = async (channel: ActionableLiveTVChannel) => {
    setActionError('');
    setBusy(`favorite:${channel.id}`);
    try {
      await mutations.favoriteChannel(channel.id, !channel.favorite);
      setRevision((current) => current + 1);
    } catch (reason) {
      setActionError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'change this favorite' }));
    } finally {
      setBusy('');
    }
  };

  const record = async (program: ActionableLiveTVProgram, series = false) => {
    setActionError('');
    setActionNotice('');
    setBusy(`record:${program.id}`);
    try {
      if (series) {
        await mutations.recordSeries({ sourceId: program.sourceId, channelId: program.channelId, programId: program.id, title: program.title, matchType: 'series' });
      } else {
        await mutations.recordProgram({ sourceId: program.sourceId, channelId: program.channelId, programId: program.id, title: program.title, startsAt: program.startAt, endsAt: program.endAt });
      }
      setScheduledPrograms((current) => new Set(current).add(program.id));
      setActionNotice(series ? 'Series recording scheduled.' : 'Recording scheduled.');
    } catch (reason) {
      setActionError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: isConflictError(reason) ? 'resolve this recording conflict' : 'schedule this recording' }));
    } finally {
      setBusy('');
    }
  };

  const shiftWindow = (hours: number) => setWindowStart((current) => new Date(current.getTime() + hours * 60 * 60 * 1000));
  const selectDay = (day: string) => {
    const next = new Date(`${day}T00:00:00`);
    if (day === localDay()) setWindowStart(initialGuideStart());
    else setWindowStart(next);
  };
  const resetNow = () => setWindowStart(initialGuideStart());
  const selectProgram = (program: ActionableLiveTVProgram) => {
    setSelectedProgramId(program.id);
    setSelectedChannelId(program.channelId ?? '');
  };

  return <div className="live-workspace guide-workspace">
    {focusedChannel && (guide.status === 'success' || usingChannelFallback) && <section className="live-focus-strip" aria-label="Selected channel and program">
      <button type="button" className="live-focus-channel" onClick={() => setSelectedChannelId(focusedChannel.id)} aria-label={`Select ${focusedChannel.name}`}>
        <span className="channel-mark compact">{logo(focusedChannel) ? <img src={logo(focusedChannel)} alt="" /> : <span>{focusedChannel.number || focusedChannel.name.slice(0, 2)}</span>}</span>
      </button>
      <div className="live-focus-copy">
        <p><span className={programIsLive(focusedChannelProgram, serverTime) ? 'live-indicator' : 'program-time-indicator'}>{programIsLive(focusedChannelProgram, serverTime) ? 'Live' : focusedChannelProgram ? dateLabel(focusedChannelProgram.startAt) : 'Channel'}</span>{focusedChannel.number ? `Channel ${focusedChannel.number}` : focusedChannel.groupTitle}</p>
        <h2>{focusedChannelProgram?.title ?? focusedChannel.name}</h2>
        <span>{focusedChannelProgram ? `${focusedChannel.name} · ${timeLabel(focusedChannelProgram.startAt)}–${timeLabel(focusedChannelProgram.endAt)}` : 'No schedule data is available for this channel.'}</span>
        {focusedChannelProgram?.description && <p className="live-focus-description">{focusedChannelProgram.description}</p>}
      </div>
      <div className="live-focus-actions">
        {canPlay && hasAction(focusedChannel, liveActions.play) && <PrimaryButton disabled={busy === `watch:${focusedChannel.id}`} onClick={() => void watch(focusedChannel)}><PlaybackPlayIcon fill="currentColor" /> {busy === `watch:${focusedChannel.id}` ? productText('state.opening', { destination: productText('destination.live-tv') }) : programIsLive(focusedChannelProgram, serverTime) ? productText('action.watch-live') : 'Watch channel'}</PrimaryButton>}
        {capabilities?.canFavoriteChannels && (hasAction(focusedChannel, liveActions.favoriteAdd) || hasAction(focusedChannel, liveActions.favoriteRemove)) && <IconButton disabled={busy === `favorite:${focusedChannel.id}`} label={`${focusedChannel.favorite ? 'Remove' : 'Add'} ${focusedChannel.name} ${focusedChannel.favorite ? 'from' : 'to'} favorites`} className={focusedChannel.favorite ? 'selected' : ''} onClick={() => void favorite(focusedChannel)}><ActionRateIcon fill={focusedChannel.favorite ? 'currentColor' : 'none'} /></IconButton>}
        {focusedChannelProgram && capabilities?.canScheduleRecordings && hasAction(focusedChannelProgram, liveActions.record) && !scheduledPrograms.has(focusedChannelProgram.id) && <SecondaryButton disabled={busy === `record:${focusedChannelProgram.id}`} onClick={() => void record(focusedChannelProgram)}><MediaVideoIcon /> {productText('action.record-once')}</SecondaryButton>}
        {focusedChannelProgram && capabilities?.canScheduleRecordings && hasAction(focusedChannelProgram, liveActions.recordSeries) && !scheduledPrograms.has(focusedChannelProgram.id) && <SecondaryButton disabled={busy === `record:${focusedChannelProgram.id}`} onClick={() => void record(focusedChannelProgram, true)}><MediaCalendarIcon /> {productText('action.record-series')}</SecondaryButton>}
        {scheduledPrograms.has(focusedChannelProgram?.id ?? '') && <span className="recording-confirmed"><MediaVideoIcon /> Scheduled</span>}
        {selectedProgramId && <IconButton label="Return to current program" onClick={() => { setSelectedProgramId(''); setSelectedChannelId(''); }}><ActionCloseIcon /></IconButton>}
      </div>
    </section>}

    <div className="live-toolbar guide-toolbar">
      <LiveChoiceMenu className="source-choice" label="Source" value={sourceId} choices={sources.map((candidate) => ({ id: candidate.id, label: candidate.name, detail: `${candidate.channelCount} channels` }))} onChange={setSourceId} />
      <div className="guide-window-control">
        <IconButton label="Earlier programs" onClick={() => shiftWindow(-guideHours)}><NavigationPreviousIcon /></IconButton>
        <label className="guide-day-field"><span>Day</span><input type="date" value={localDay(windowStart)} onChange={(event) => selectDay(event.target.value)} /></label>
        <button type="button" className="guide-now-button" onClick={resetNow}><MetadataTimeIcon /> Now</button>
        <IconButton label="Later programs" onClick={() => shiftWindow(guideHours)}><NavigationDisclosureIcon /></IconButton>
      </div>
      <label className="guide-search"><NavigationSearchIcon /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search guide" aria-label="Search guide" /></label>
      {rawGroups.length > 1 && <LiveChoiceMenu className="group-choice" label="Group" value={group} choices={[{ id: 'all', label: 'All channels' }, ...rawGroups.map((name) => ({ id: name, label: name }))]} onChange={setGroup} />}
      <SecondaryButton selected={favoritesOnly} onClick={() => setFavoritesOnly((current) => !current)}><ActionRateIcon fill={favoritesOnly ? 'currentColor' : 'none'} /> Favorites</SecondaryButton>
    </div>

    {(actionError || actionNotice) && <p className={actionError ? 'live-action-message error' : 'live-action-message'} role={actionError ? 'alert' : 'status'}>{actionError || actionNotice}</p>}
    {guide.status === 'loading' && <StateSurface kind="loading" {...productState('live-tv.loading')} />}
    {guide.status === 'error' && channelFallback.status === 'loading' && <StateSurface kind="loading" {...productState('live-tv.loading')} />}
    {guide.status === 'error' && channelFallback.status === 'error' && (() => {
      const restricted = (guide.error as Error & { status?: number }).status === 401 || (guide.error as Error & { status?: number }).status === 403;
      const state = productState(restricted ? 'live-tv.restricted' : 'live-tv.guide-unavailable');
      return <StateSurface kind={restricted ? 'permission' : 'error'} title={state.title} message={reviewedProductErrorText(guide.error, restricted ? 'live-tv.restricted' : 'live-tv.guide-unavailable')} onRetry={() => setRevision((current) => current + 1)} />;
    })()}
    {guide.status === 'success' && channels.length === 0 && <StateSurface kind="empty" {...productState(query || favoritesOnly || group !== 'all' ? 'live-tv.filter-empty' : 'live-tv.empty')} />}
    {guide.status === 'success' && channels.length > 0 && <GuideGrid channels={channels} programs={programs} from={new Date(guide.data.from)} to={new Date(guide.data.to)} serverTime={new Date(serverTime)} selectedProgramId={selectedProgramId} scheduledPrograms={scheduledPrograms} onSelectProgram={selectProgram} onSelectChannel={(channel) => { setSelectedChannelId(channel.id); setSelectedProgramId(''); }} logo={logo} />}
    {usingChannelFallback && channels.length === 0 && <StateSurface kind="empty" {...productState(query || favoritesOnly || group !== 'all' ? 'live-tv.filter-empty' : 'live-tv.empty')} />}
    {usingChannelFallback && channels.length > 0 && <GuideGrid channels={channels} programs={[]} from={windowStart} to={new Date(windowStart.getTime() + guideHours * 60 * 60 * 1000)} serverTime={new Date(serverTime)} selectedProgramId="" scheduledPrograms={scheduledPrograms} onSelectProgram={() => undefined} onSelectChannel={(channel) => { setSelectedChannelId(channel.id); setSelectedProgramId(''); }} logo={logo} />}
    {guide.status === 'success' && supportsPaging && (cursors.length > 0 || guide.data.pageInfo.hasMore) && <div className="guide-page-controls">
      <SecondaryButton disabled={cursors.length === 0} onClick={() => setCursors((current) => current.slice(0, -1))}><NavigationPreviousIcon /> Previous channels</SecondaryButton>
      <span>Page {cursors.length + 1}{guide.data.pageInfo.total != null ? ` · ${guide.data.pageInfo.total} channels` : ''}</span>
      <SecondaryButton disabled={!guide.data.pageInfo.hasMore || !guide.data.pageInfo.nextCursor} onClick={() => guide.data.pageInfo.nextCursor && setCursors((current) => [...current, guide.data.pageInfo.nextCursor!])}>Next channels <NavigationDisclosureIcon /></SecondaryButton>
    </div>}
    {usingChannelFallback && fallbackSupportsPaging && (cursors.length > 0 || channelFallback.data.pageInfo.hasMore) && <div className="guide-page-controls">
      <SecondaryButton disabled={cursors.length === 0} onClick={() => setCursors((current) => current.slice(0, -1))}><NavigationPreviousIcon /> Previous channels</SecondaryButton>
      <span>Page {cursors.length + 1}{channelFallback.data.pageInfo.total != null ? ` · ${channelFallback.data.pageInfo.total} channels` : ''}</span>
      <SecondaryButton disabled={!channelFallback.data.pageInfo.hasMore || !channelFallback.data.pageInfo.nextCursor} onClick={() => channelFallback.data.pageInfo.nextCursor && setCursors((current) => [...current, channelFallback.data.pageInfo.nextCursor!])}>Next channels <NavigationDisclosureIcon /></SecondaryButton>
    </div>}
  </div>;
}

export type SharedGuideChannel = {
  id: string;
  name: string;
  number?: string;
  groupTitle?: string;
  favorite?: boolean;
};

export type SharedGuideProgram = {
  id: string;
  channelId?: string;
  channelRef?: string;
  startAt: string;
  endAt: string;
  title: string;
  subtitle?: string;
  category?: string;
};

export function GuideGrid<C extends SharedGuideChannel, P extends SharedGuideProgram>({
  channels,
  programs,
  from,
  to,
  serverTime,
  selectedProgramId,
  scheduledPrograms,
  onSelectProgram,
  onSelectChannel,
  logo,
}: {
  channels: C[];
  programs: P[];
  from: Date;
  to: Date;
  serverTime: Date;
  selectedProgramId: string;
  scheduledPrograms: Set<string>;
  onSelectProgram: (program: P) => void;
  onSelectChannel: (channel: C) => void;
  logo: (channel: C) => string;
}) {
  const range = Math.max(1, to.getTime() - from.getTime());
  const ticks = Array.from({ length: guideHours * 2 + 1 }, (_, index) => new Date(from.getTime() + index * 30 * 60 * 1000));
  const trackWidth = guideHours * 220;
  const nowPosition = ((serverTime.getTime() - from.getTime()) / range) * 100;
  const showNow = nowPosition >= 0 && nowPosition <= 100;
  const byChannel = new Map<string, P[]>();
  programs.forEach((program) => {
    const channelId = program.channelId ?? program.channelRef ?? '';
    byChannel.set(channelId, [...(byChannel.get(channelId) ?? []), program]);
  });
  const moveFocus = (event: KeyboardEvent<HTMLButtonElement>, channelIndex: number, programIndex: number, startAt: string) => {
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return;
    event.preventDefault();
    if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
      const next = Math.max(0, programIndex + (event.key === 'ArrowRight' ? 1 : -1));
      document.querySelector<HTMLButtonElement>(`[data-guide-cell="${channelIndex}:${next}"]`)?.focus();
      return;
    }
    const nextChannel = channelIndex + (event.key === 'ArrowDown' ? 1 : -1);
    const candidates = Array.from(document.querySelectorAll<HTMLButtonElement>(`[data-guide-channel-index="${nextChannel}"]`));
    const start = new Date(startAt).getTime();
    candidates.sort((left, right) => Math.abs(Number(left.dataset.guideStart) - start) - Math.abs(Number(right.dataset.guideStart) - start))[0]?.focus();
  };
  const gridStyle = { '--guide-track-width': `${trackWidth}px` } as CSSProperties;
  return <div className="guide-scroll" aria-label="Program guide" tabIndex={0}>
    <div className="live-guide-grid" style={gridStyle}>
      <div className="live-guide-corner">Channel</div>
      <div className="live-guide-times">{ticks.map((tick) => <time key={tick.toISOString()}>{timeLabel(tick)}</time>)}</div>
      {channels.map((channel, channelIndex) => {
        const channelPrograms = (byChannel.get(channel.id) ?? []).sort((left, right) => left.startAt.localeCompare(right.startAt));
        return <div className="live-guide-row" key={channel.id}>
          <button type="button" className="live-guide-channel" onClick={() => onSelectChannel(channel)}>
            <span className="guide-channel-logo">{logo(channel) ? <img src={logo(channel)} alt="" /> : <span>{channel.number || '—'}</span>}</span>
            <span><strong>{channel.name}</strong><span>{channel.groupTitle || 'Live channel'}</span></span>
            {channel.favorite && <ActionRateIcon fill="currentColor" aria-label="Favorite" />}
          </button>
          <div className="live-guide-program-track">
            {showNow && <span className="guide-now-line" style={{ left: `${nowPosition}%` }} aria-hidden="true" />}
            {channelPrograms.map((program, programIndex) => {
              const start = Math.max(from.getTime(), new Date(program.startAt).getTime());
              const end = Math.min(to.getTime(), new Date(program.endAt).getTime());
              if (end <= start) return null;
              const isCurrent = new Date(program.startAt).getTime() <= serverTime.getTime() && new Date(program.endAt).getTime() > serverTime.getTime();
              const style = {
                '--program-left': `${((start - from.getTime()) / range) * 100}%`,
                '--program-width': `${Math.max(4, ((end - start) / range) * 100)}%`,
              } as CSSProperties;
              return <button
                key={program.id}
                type="button"
                data-guide-cell={`${channelIndex}:${programIndex}`}
                data-guide-channel-index={channelIndex}
                data-guide-start={new Date(program.startAt).getTime()}
                className={`${selectedProgramId === program.id ? 'selected' : ''} ${isCurrent ? 'current' : ''} ${scheduledPrograms.has(program.id) ? 'scheduled' : ''}`}
                style={style}
                aria-label={`${program.title}, ${timeLabel(program.startAt)} to ${timeLabel(program.endAt)}`}
                onClick={() => onSelectProgram(program)}
                onKeyDown={(event) => moveFocus(event, channelIndex, programIndex, program.startAt)}
              ><strong>{program.title}</strong><span>{timeLabel(program.startAt)} · {program.subtitle || program.category || 'Live'}</span>{scheduledPrograms.has(program.id) && <MediaVideoIcon aria-label="Scheduled to record" />}</button>;
            })}
            {channelPrograms.length === 0 && <div className="live-guide-gap">No schedule data</div>}
          </div>
        </div>;
      })}
    </div>
  </div>;
}
