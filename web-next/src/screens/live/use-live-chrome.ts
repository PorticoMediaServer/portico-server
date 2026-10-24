import {useEffect, useMemo, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {DVRClient, defaultRecordingOptions, type RecordingState} from '@core/dvr.ts';
import {channelTimeline} from '@core/playback/channel-timeline.ts';
import {legacyChannel, legacyChannelRow, legacyGuideSource, legacyProgramme, type GuideChannelRow} from '@core/guide/index.ts';
import type {GuideChannel, GuideProgramme} from '@core/channel-guide.ts';
import {sameFence, type ChannelPlaybackFence} from '../../../../packages/player-core/src/index.ts';
import type {WebPlaybackRuntimeSnapshot, WebShellPlaybackRuntime} from '../../platform/playback';
import {router} from '../../router';
import {screenLifetime, type ScreenContext} from '../detail/context';
import {createLiveBindings} from './bindings';
import type {LiveChromeChannel, LiveChromeGuideChannel, LiveChromeGuideTarget, LiveChromePort, LiveChromeProgramme} from './live-chrome-types';

const randomHex = async () => Array.from(crypto.getRandomValues(new Uint8Array(24)), value => value.toString(16).padStart(2, '0')).join('');
const recordingState = (programme: GuideProgramme | undefined): LiveChromeProgramme['recording'] => {
  if (!programme?.recordingId) return undefined;
  return programme.recordingState === 'recording' ? 'recording' : 'scheduled';
};
const asChromeChannel = (channel: GuideChannel): LiveChromeChannel => ({id: channel.id, sourceId: channel.sourceId, kind: channel.provenance,
  generation: channel.generation, number: channel.number, name: channel.name, ...(channel.mark ? {mark: {text: channel.mark.text, seed: channel.mark.seed}} : {}),
  favorite: channel.favorite, recordAvailable: channel.recordAvailable, tuneAvailable: channel.tuneAvailable});
const asChromeProgramme = (programme: GuideProgramme): LiveChromeProgramme => ({id: programme.id, title: programme.title,
  ...(programme.subtitle ? {subtitle: programme.subtitle} : {}), ...(programme.episode ? {episode: programme.episode} : {}),
  startMs: Date.parse(programme.start), endMs: Date.parse(programme.end), ...(recordingState(programme) ? {recording: recordingState(programme)} : {})});
const asMini = (row: GuideChannelRow, programme: GuideProgramme | undefined): LiveChromeGuideChannel | undefined => {
  const channel = legacyChannel(row);
  if (!channel) return undefined;
  return {channel: asChromeChannel(channel), ...(programme ? {programme: asChromeProgramme(programme)} : {}),
    ...(programme ? {nextStartMs: Date.parse(programme.end)} : {})};
};

type GuideFacts = Readonly<{fenceKey: string; current: GuideChannel; rows: readonly GuideChannelRow[]; programmes: ReadonlyMap<string, GuideProgramme>}>;

/**
 * Builds the drawn Live chrome's narrow port from the actual channel session and the
 * authority-local guide. Construction is inert; a mounted player starts one bounded read
 * for the current channel and its nearby rows. No second player, channel controller or
 * store is created, and no guide fact is guessed from the VOD presentation.
 */
export function useLiveChromePort(context: ScreenContext | undefined, runtime: WebShellPlaybackRuntime | undefined, snapshot: WebPlaybackRuntimeSnapshot | undefined,
  skips: Readonly<{back: number; forward: number}>): LiveChromePort | undefined {
  const fence = snapshot?.fence?.kind === 'channel' ? snapshot.fence as ChannelPlaybackFence : undefined;
  const linear = snapshot?.playback.linear, presentation = snapshot?.presentation;
  const channelRef = linear?.channel;
  const fenceKey = fence && channelRef ? JSON.stringify([fence.sessionId, fence.generation, fence.channelId, fence.channelGeneration, linear?.programme?.id ?? '']) : '';
  const life = useMemo(() => context ? screenLifetime(context) : undefined, [context]);
  const client = useMemo(() => life && context ? new DVRClient(life.api, context.scope.serverId, randomHex) : undefined, [life, context]);
  useEffect(() => () => {client?.dispose();life?.dispose();}, [client, life]);
  const [facts, setFacts] = useState<GuideFacts>();
  const [recorded, setRecorded] = useState<Readonly<{programmeId: string; state: 'recording' | 'scheduled'}>>();
  const [refresh, setRefresh] = useState(0);
  const input = useRef({fence, channelRef});input.current = {fence, channelRef};
  useEffect(() => { setRecorded(undefined); }, [fenceKey]);
  useEffect(() => {
    const fence = input.current.fence, channelRef = input.current.channelRef;
    if (!context || !life || !fence || !channelRef || !fenceKey) { setFacts(undefined); return; }
    const abort = new AbortController();
    const bindings = createLiveBindings(context, {kind: channelRef.kind, sourceId: channelRef.sourceId, timezone: defaultI18n.region.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone, search: '', sort: 'number'});
    const source = legacyGuideSource(life.api, context.scope.serverId, {kind: channelRef.kind, sourceId: channelRef.sourceId,
      timezone: defaultI18n.region.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone, search: '', sort: 'number'});
    const run = async () => {
      const current = await bindings.selectedChannel({sourceId: channelRef.sourceId, channelId: channelRef.channelId, provenance: channelRef.kind}, abort.signal);
      if (!current || current.generation !== channelRef.generation || abort.signal.aborted) return;
      const row = legacyChannelRow(current);
      const down = source.neighbor ? await source.neighbor(row, -1, abort.signal).catch(() => undefined) : undefined;
      const up: GuideChannelRow[] = [];
      let anchor: GuideChannelRow | undefined = row;
      for (let index = 0; index < 3 && anchor; index++) {
        const next: GuideChannelRow | undefined = source.neighbor ? await source.neighbor(anchor, 1, abort.signal).catch(() => undefined) : undefined;
        if (!next) break;
        up.push(next);anchor = next;
      }
      if (abort.signal.aborted) return;
      const rows = [down, row, ...up].filter((item): item is GuideChannelRow => !!item);
      const programs = await source.programs(rows.map(item => item.id), Date.now(), Date.now() + 1, abort.signal);
      if (abort.signal.aborted) return;
      const programmes = new Map<string, GuideProgramme>();
      for (const item of rows) {
        const raw = legacyChannel(item); if (!raw) continue;
        const page = programs[item.id]?.find(programme => programme.start <= Date.now() && Date.now() < programme.end);
        const rawProgramme = page ? legacyProgramme(page) : undefined;
        if (rawProgramme) programmes.set(item.id, rawProgramme);
      }
      setFacts({fenceKey, current, rows, programmes});
    };
    void run().catch(() => {});
    return () => {abort.abort();bindings.dispose();};
  }, [context, life, fenceKey, refresh]);
  if (!context || !life || !client || !runtime || !snapshot || !fence || !linear || !presentation || !fenceKey) return undefined;
  const channel = facts?.fenceKey === fenceKey ? asChromeChannel(facts.current) : undefined;
  const currentGuide = facts?.fenceKey === fenceKey ? facts.rows.find(row => row.id === fence.channelId) : undefined;
  const currentGuideProgramme = facts?.fenceKey === fenceKey ? facts.programmes.get(fence.channelId) : undefined;
  const guideProgramme = facts?.fenceKey === fenceKey && linear.programme
    ? (currentGuideProgramme?.id === linear.programme.id ? currentGuideProgramme : facts.current.programmes.find(programme => programme.id === linear.programme!.id)) : undefined;
  const authoritative = channel && currentGuide ? channel : {id: fence.channelId, sourceId: fence.sourceId, kind: fence.channelKind, generation: fence.channelGeneration,
    number: '', name: linear.name, favorite: false, recordAvailable: false, tuneAvailable: true};
  const factsRecording = recordingState(guideProgramme);
  const localRecording = recorded?.programmeId === linear.programme?.id ? recorded.state : undefined;
  const currentRecording = factsRecording ?? localRecording;
  const programme: LiveChromeProgramme | undefined = linear.programme ? {id: linear.programme.id, title: linear.programme.title,
    startMs: linear.programme.startMs, endMs: linear.programme.endMs, ...(currentRecording ? {recording: currentRecording} : {})} : undefined;
  const position = presentation.positionSeconds;
  const timeline = channelTimeline(linear, position), origin = linear.media.originMs;
  const currentMini = currentGuide && linear.programme ? asMini(currentGuide, guideProgramme) : undefined;
  const miniGuide: LiveChromeGuideChannel[] = facts?.fenceKey === fenceKey ? facts.rows.map(row => asMini(row,
    row.id === fence.channelId ? guideProgramme : facts.programmes.get(row.id))).filter((row): row is LiveChromeGuideChannel => !!row) : [];
  const currentIndex = facts?.fenceKey === fenceKey ? facts.rows.findIndex(row => row.id === fence.channelId) : -1;
  const downRow = currentIndex > 0 ? facts!.rows[currentIndex - 1] : undefined;
  const upRow = currentIndex >= 0 ? facts!.rows[currentIndex + 1] : undefined;
  const down = downRow ? asMini(downRow, facts!.programmes.get(downRow.id)) : undefined;
  const up = upRow ? asMini(upRow, facts!.programmes.get(upRow.id)) : undefined;
  const currentFence = fence, authority = snapshot.authority;
  const isCurrent = () => {
    const live = runtime.getSnapshot();
    if (live.disposed || live.authority !== authority || !live.fence || !sameFence(live.fence, currentFence) || live.fence.kind !== 'channel') return false;
    const active = live.playback.channel;
    return !!active && active.kind === currentFence.channelKind && active.sourceId === currentFence.sourceId
      && active.channelId === currentFence.channelId && active.generation === currentFence.channelGeneration
      && live.playback.linear?.channel.generation === currentFence.channelGeneration;
  };
  const guideTarget = (target: LiveChromeGuideTarget): GuideChannel | undefined => {
    const row = facts?.rows.find(item => item.id === target.channelId && item.sourceId === target.sourceId);
    const raw = row ? legacyChannel(row) : undefined;
    return raw && raw.generation === target.generation && raw.provenance === target.kind && raw.tuneAvailable ? raw : undefined;
  };
  const tuneTarget = async (target: LiveChromeGuideTarget): Promise<void> => {
    if (!isCurrent()) return;
    const raw = guideTarget(target);
    if (!raw) return;
    await runtime.playFromGuide(raw, {signal: life.signal(), isCurrent: () => isCurrent() && life.current()});
  };
  const guideRowTarget = (row: LiveChromeGuideChannel): LiveChromeGuideTarget => ({sourceId: row.channel.sourceId, channelId: row.channel.id,
    generation: row.channel.generation, kind: row.channel.kind});
  const record = async (): Promise<void> => {
    if (!isCurrent() || !client || !guideProgramme || !facts?.current) return;
    if (guideProgramme.recordingId) {
      const detail = await client.detail(guideProgramme.recordingId);
      if (!isCurrent()) return;
      await client.cancel(detail.recording);
      if (!isCurrent()) return;
      setRecorded(undefined);setRefresh(value => value + 1);return;
    }
    const scheduled = await client.schedule({channel: facts.current, programme: guideProgramme, series: false}, defaultRecordingOptions);
    if (!isCurrent()) return;
    const state: RecordingState = scheduled.state;
    if (['recording', 'preparing', 'scheduled', 'conflicted', 'waiting-source', 'waiting-guide'].includes(state)) {
      setRecorded({programmeId: guideProgramme.id, state: state === 'recording' || state === 'preparing' ? 'recording' : 'scheduled'});
    }
    setRefresh(value => value + 1);
  };
  const goLive = async (): Promise<void> => { if (isCurrent()) runtime.goLive(currentFence); };
  const port: LiveChromePort = {
    fence: currentFence, channel: authoritative, ...(programme ? {programme} : {}),
    ...(linear.next ? {nextProgramme: {title: linear.next.title, startMs: linear.next.startMs, endMs: linear.next.endMs}} : {}),
    window: {...(linear.programme ? {programmeStartMs: linear.programme.startMs, programmeEndMs: linear.programme.endMs} : {}),
      seekStartMs: origin + timeline.start * 1000, seekEndMs: origin + timeline.end * 1000, liveEdgeMs: origin + timeline.edge * 1000,
      positionMs: origin + position * 1000, behindSeconds: timeline.behind, atLive: timeline.behind <= 2},
    ...(down ? {down} : {}), ...(up ? {up} : {}), miniGuide, skipBackSeconds: skips.back, skipForwardSeconds: skips.forward,
    isCurrent, record, goLive,
    tune: async direction => { const row = direction === -1 ? down : up; if (row) await tuneTarget(guideRowTarget(row)); },
    tuneTo: tuneTarget,
    openFullGuide: () => { if (context.isCurrent()) void router.navigate({to: '/live', search: {tab: 'guide'}}).catch(() => {}); },
    openRecordings: () => { if (context.isCurrent()) void router.navigate({to: '/live', search: {tab: 'recordings', dvr: 'upcoming'}}).catch(() => {}); },
  };
  return port;
}
