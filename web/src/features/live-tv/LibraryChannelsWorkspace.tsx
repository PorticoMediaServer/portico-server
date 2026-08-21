import type { LibraryChannelsGuide } from '@portico/client-core';
import { AlertTriangle, ChevronLeft, ChevronRight, Clock3, Play, Search, TvMinimalPlay } from '#portico-icons';
import { type ComponentType, useDeferredValue, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { productText } from '../../components/ProductLanguage';
import { useLibraryChannelsGuide, usePorticoDataSource } from '../../data/DataProvider';
import { usePlaybackSession } from '../player/PlayerSurface';
import { GuideGrid, guideHours, type SharedGuideChannel, type SharedGuideProgram } from './LiveGuide';
import { dateLabel, initialGuideStart, localDay, productState, requestError, timeLabel } from './liveFormat';

type StateSurfaceType = ComponentType<{ kind: 'loading' | 'empty' | 'error' | 'permission'; title?: string; message: string; onRetry?: () => void }>;
type LibrarySummary = LibraryChannelsGuide['channels'][number];
type LibraryEntry = LibraryChannelsGuide['programs'][number];
type LibraryGridChannel = SharedGuideChannel & LibrarySummary;
type LibraryGridProgram = SharedGuideProgram & { entry: LibraryEntry };

function gridChannel(channel: LibrarySummary): LibraryGridChannel {
  return { ...channel, number: '', groupTitle: 'Library Channel', favorite: false };
}

function gridProgram(entry: LibraryEntry): LibraryGridProgram {
  return {
    id: entry.id,
    channelId: entry.channelId,
    startAt: entry.startsAt,
    endAt: entry.endsAt,
    title: entry.title || productState(entry.kind === 'slate' ? 'library-channel.slate' : 'library-channel.program-unavailable').title,
    subtitle: entry.subtitle || entry.availability,
    category: entry.kind,
    entry,
  };
}

function entryIsNow(entry: LibraryEntry | undefined, now: number) {
  return Boolean(entry && new Date(entry.startsAt).valueOf() <= now && now < new Date(entry.endsAt).valueOf());
}

export function LibraryChannelsWorkspace({ StateSurface }: { StateSurface: StateSurfaceType }) {
  const source = usePorticoDataSource();
  const player = usePlaybackSession();
  const navigate = useNavigate();
  const [windowStart, setWindowStart] = useState(() => initialGuideStart());
  const [query, setQuery] = useState('');
  const deferredQuery = useDeferredValue(query.trim().toLocaleLowerCase());
  const [onlyAvailable, setOnlyAvailable] = useState(false);
  const [selectedProgramId, setSelectedProgramId] = useState('');
  const [selectedChannelId, setSelectedChannelId] = useState('');
  const [revision, setRevision] = useState(0);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const range = useMemo(() => ({
    from: windowStart.toISOString(),
    to: new Date(windowStart.getTime() + guideHours * 3_600_000).toISOString(),
    limit: 1000,
  }), [windowStart]);
  const guide = useLibraryChannelsGuide(range, revision);
  const data = guide.status === 'success' ? guide.data : undefined;
  const rawChannels = data?.channels ?? [];
  const rawEntries = data?.programs ?? [];
  const matchingProgramChannelIds = new Set(rawEntries.filter((program) => `${program.title ?? ''} ${program.subtitle ?? ''} ${program.summary ?? ''}`.toLocaleLowerCase().includes(deferredQuery)).map((program) => program.channelId));
  const visibleChannels = rawChannels.filter((channel) => !deferredQuery || `${channel.name} ${channel.description}`.toLocaleLowerCase().includes(deferredQuery) || matchingProgramChannelIds.has(channel.id));
  const visibleChannelIds = new Set(visibleChannels.map((channel) => channel.id));
  const rawPrograms = rawEntries.filter((program) => visibleChannelIds.has(program.channelId) && (!onlyAvailable || program.availability === 'available'));
  const channels = visibleChannels.map(gridChannel);
  const programs = rawPrograms.map(gridProgram);
  const now = data ? new Date(data.serverTime).valueOf() : Date.now();
  const selected = rawPrograms.find((program) => program.id === selectedProgramId);
  const current = rawPrograms.find((program) => program.channelId === (selectedChannelId || visibleChannels[0]?.id) && entryIsNow(program, now));
  const focused = selected ?? current ?? rawPrograms.find((program) => program.channelId === (selectedChannelId || visibleChannels[0]?.id)) ?? rawPrograms[0];
  const focusedChannel = visibleChannels.find((channel) => channel.id === (selectedChannelId || focused?.channelId)) ?? visibleChannels[0];
  const logo = (channel: LibraryGridChannel) => channel.logoUrl ? source.playbackResourceUrl(channel.logoUrl) : '';

  const tune = async () => {
    if (!focusedChannel || focused?.availability !== 'available') return;
    setError('');
    setBusy(true);
    try {
      const playback = await player.startLibraryChannel(focusedChannel.id);
      if (playback) navigate(`/watch/${playback.media.id}`);
    } catch (reason) {
      setError(requestError(reason, 'problem.request-failed'));
    } finally {
      setBusy(false);
    }
  };

  const today = initialGuideStart();
  const horizonEnd = new Date(today.getTime() + 6 * 86_400_000);
  const latestWindowStart = new Date(today.getTime() + 7 * 86_400_000 - guideHours * 3_600_000);
  const shiftWindow = (hours: number) => setWindowStart((currentStart) => new Date(Math.min(latestWindowStart.getTime(), Math.max(today.getTime(), currentStart.getTime() + hours * 3_600_000))));
  const selectDay = (day: string) => {
    const requested = day === localDay() ? initialGuideStart() : new Date(`${day}T00:00:00`);
    setWindowStart(new Date(Math.min(latestWindowStart.getTime(), Math.max(today.getTime(), requested.getTime()))));
  };
  const canShiftEarlier = windowStart.getTime() > today.getTime();
  const canShiftLater = windowStart.getTime() < latestWindowStart.getTime();
  const explainer = productState('library-channel.explainer');

  return <div className="live-workspace guide-workspace library-channel-workspace">
    <div className="library-channel-source-note"><TvMinimalPlay /><span><strong>{explainer.title}</strong><span>{explainer.message}</span></span></div>
    {focusedChannel && focused && <section className="live-focus-strip" aria-label="Selected Library Channel and program">
      <span className="live-focus-channel"><span className="channel-mark compact">{focusedChannel.logoUrl ? <img src={source.playbackResourceUrl(focusedChannel.logoUrl)} alt="" /> : <TvMinimalPlay />}</span></span>
      <div className="live-focus-copy"><p><span className={entryIsNow(focused, now) ? 'live-indicator' : 'program-time-indicator'}>{entryIsNow(focused, now) ? 'Live' : dateLabel(focused.startsAt)}</span>Library Channel</p><h2>{focused.title || productState('library-channel.program-unavailable').title}</h2><span>{focusedChannel.name} · {timeLabel(focused.startsAt)}–{timeLabel(focused.endsAt)}</span>{(focused.summary || focused.subtitle) && <p className="live-focus-description">{focused.summary || focused.subtitle}</p>}</div>
      <div className="live-focus-actions"><PrimaryButton disabled={busy || focused.availability !== 'available' || !focusedChannel.actions.includes('live.play')} onClick={() => void tune()}><Play fill="currentColor" /> {busy ? productText('state.opening', { destination: productText('destination.live-tv') }) : productText('action.watch-live')}</PrimaryButton></div>
    </section>}
    <div className="live-toolbar guide-toolbar">
      <div className="guide-window-control"><IconButton label="Earlier programs" disabled={!canShiftEarlier} onClick={() => shiftWindow(-guideHours)}><ChevronLeft /></IconButton><label className="guide-day-field"><span>Day</span><input type="date" min={localDay(today)} max={localDay(horizonEnd)} value={localDay(windowStart)} onChange={(event) => selectDay(event.target.value)} /></label><button type="button" className="guide-now-button" onClick={() => setWindowStart(initialGuideStart())}><Clock3 /> Now</button><IconButton label="Later programs" disabled={!canShiftLater} onClick={() => shiftWindow(guideHours)}><ChevronRight /></IconButton></div>
      <label className="guide-search"><Search /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search Library Channels" aria-label="Search Library Channels" /></label>
      <SecondaryButton selected={onlyAvailable} onClick={() => setOnlyAvailable((currentOnly) => !currentOnly)}>Available only</SecondaryButton>
    </div>
    {error && <p className="live-action-message error" role="alert"><AlertTriangle /> {error}</p>}
    {guide.status === 'loading' && <StateSurface kind="loading" {...productState('library-channel.loading')} />}
    {guide.status === 'error' && <StateSurface kind={(guide.error as Error & { status?: number }).status === 403 ? 'permission' : 'error'} title={productState('library-channel.load-failed').title} message={requestError(guide.error, 'library-channel.load-failed')} onRetry={() => setRevision((value) => value + 1)} />}
    {guide.status === 'success' && !rawChannels.length && <StateSurface kind="empty" {...productState('library-channel.empty')} />}
    {guide.status === 'success' && rawChannels.length > 0 && !channels.length && <StateSurface kind="empty" {...productState('library-channel.filter-empty')} />}
    {guide.status === 'success' && channels.length > 0 && <GuideGrid channels={channels} programs={programs} from={new Date(guide.data.from)} to={new Date(guide.data.to)} serverTime={new Date(guide.data.serverTime)} selectedProgramId={selectedProgramId} scheduledPrograms={new Set()} onSelectProgram={(program) => { setSelectedProgramId(program.id); setSelectedChannelId(program.channelId ?? ''); }} onSelectChannel={(channel) => { setSelectedChannelId(channel.id); setSelectedProgramId(''); }} logo={logo} />}
  </div>;
}
