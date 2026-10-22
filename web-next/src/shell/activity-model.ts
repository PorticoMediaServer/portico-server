import {playbackIdentity, type CardSubject, type PlaybackSubject, type TimeEnvironment} from '@core/presentation/kit-model/index.ts';
import type {ActivitySummarySession, ActivitySummarySnapshot, OwnerActivitySummary} from '@core/admin/activity-summary/index.ts';
import {jobName, jobStateName, scanStepName} from '@core/server-admin/console-catalogue.ts';
import {deliveryId} from '@core/server-admin/now-playing.ts';
import {parseAdminSession} from '@core/playback-v1/types.ts';
import {heightText} from '@core/presentation/kit-model/technical.ts';
import {defaultI18n} from '@i18n';
import type {ActivityPanelView} from '../kit/shell';

/** Cold, refreshing and failed reads do not establish normal health or zero work. */
export function activityPanelView(snapshot: ActivitySummarySnapshot, serverName: string, time: TimeEnvironment, actions: Pick<ActivityPanelView, 'onDashboard' | 'onRetry'> & {openUpdate: () => void}): ActivityPanelView {
  if (snapshot.phase === 'ready' && snapshot.document) return activityView(snapshot.document, time, actions);
  return {serverName, streams: [], scans: [], tasks: [], phase: snapshot.phase === 'error' || snapshot.phase === 'unavailable' ? 'error' : 'loading', onRetry: actions.onRetry, onDashboard: actions.onDashboard};
}

/** Semantic facts only. The canonical formatter owns episode identity and fact order. */
export function activitySubject(stream: ActivitySummarySession): PlaybackSubject | undefined {
  const item = stream.item;
  if (!item || !['movie', 'episode', 'extra', 'song', 'audiobook_file'].includes(item.kind)) return undefined;
  const subject: CardSubject = {id: item.id, kind: item.kind as CardSubject['kind'], title: item.title, year: item.year, artist: item.artist, album: item.album,
    ...(item.showId && item.showTitle ? {show: {id: item.showId, title: item.showTitle}} : {}),
    ...(item.kind === 'episode' ? {episode: item.episodeNumbering === 'date' ? {mode: 'date' as const, day: item.airDate}
      : item.episodeNumbering === 'absolute' ? {mode: 'absolute' as const, number: item.episodeNumber}
      : item.seasonNumber === 0 ? {mode: 'special' as const} : {mode: 'seasonal' as const, season: item.seasonNumber, episode: item.episodeNumber}} : {}),
  };
  return {kind: item.kind === 'song' ? 'music' : item.kind === 'audiobook_file' ? 'audiobook' : 'video', item: subject};
}
export function activityView(document: OwnerActivitySummary, time: TimeEnvironment, actions: Pick<ActivityPanelView, 'onDashboard' | 'onRetry'> & {openUpdate: () => void}): ActivityPanelView {
  const {t} = defaultI18n;
  return {serverName: document.serverName, health: document.health.degraded ? 'attention' : 'normal', streamCount: document.streams.count, taskCount: document.tasks.count,
    streams: document.streams.items.flatMap(stream => {
      const subject = activitySubject(stream); if (!subject) return [];
      const delivery = deliveryId(parseAdminSession(stream).decision);
      const height = heightText(stream.videoHeight, time.t);
      const conversion = delivery === 'web.nowPlaying.delivery.convertVideo' && height
        ? {id: 'delivery', text: t('next.home.shell.convertingTo', {quality: height}), role: 'status' as const}
        : delivery === 'web.nowPlaying.delivery.convertVideo' || delivery === 'web.nowPlaying.delivery.convertAudio'
          ? {id: 'delivery', text: t(delivery), role: 'status' as const} : undefined;
      return [{id: stream.id, identity: playbackIdentity(subject, 'dock', time), name: stream.profile.name, device: stream.device.name,
        initial: Array.from(stream.profile.name.trim())[0] ?? '', positionMs: stream.positionMs, durationMs: stream.durationMs, paused: stream.state === 'paused',
        conversion, onOpen: () => actions.onDashboard('streams')}];
    }),
    scans: document.scans.map(scan => ({id: scan.libraryId, title: scan.libraryName, detail: scanStepName(t, defaultI18n.has, scan.phase), current: scan.current,
      icon: 'i-refresh', processed: scan.analyzed, discovered: scan.discovered, total: scan.total, onOpen: () => actions.onDashboard('activity')})),
    tasks: document.tasks.items.map(task => ({id: task.id, title: jobName(t, defaultI18n.has, task.kind), detail: jobStateName(t, defaultI18n.has, task.state),
      current: task.libraryName, icon: 'i-act', processed: task.processed, total: task.total, onOpen: () => actions.onDashboard('activity')})),
    update: {...document.update, onOpen: actions.openUpdate},
    ...(document.lastScanFinishedAt ? {lastScan: defaultI18n.relativeTime(document.lastScanFinishedAt, Date.parse(document.observedAt))} : {}),
    phase: 'ready', onRetry: actions.onRetry, onDashboard: actions.onDashboard,
  };
}
