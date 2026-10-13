import {useMemo} from 'react';
import {playbackControlsService, type RemotePlaybackControls} from '@core/playback-controls.ts';
import {SessionsClient} from '@core/playback-v1/sessions.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import {idempotencyKey} from '@core/playback-v1/http.ts';
import {REMOTE_SKIP_MS, seekTarget, transportOffers, type TransportCommand} from '@core/server-admin/now-playing.ts';
import {isStaleAnswer} from '@core/freshness-actions.ts';
import {useAction, useRead} from '../../admin/console';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Menu, Notice, SettingsGroup, SettingsRow, Status, type IconName} from '../../ui';

/** What a remote session is doing, in a word the row can show. */
export const remoteStateId = (state: string) => (state === 'playing' ? 'remote.state.playing' : state === 'paused' ? 'remote.state.paused' : 'remote.state.starting') as 'remote.state.playing' | 'remote.state.paused' | 'remote.state.starting';

/**
 * Settings › This device › Playing on your other devices: what this profile is playing
 * elsewhere (a TV it was sent to with Play on or Cast, another browser), with Pause, Resume and
 * Stop, and Back, Forward and Start over in a menu where there is a timeline. The server says
 * which of those each one allows; a title this profile may not see is not named. Nothing is
 * shown while nothing is playing elsewhere.
 */
const seekIcons: Record<'back' | 'forward' | 'startOver', IconName> = {back: 'arrowLeft', forward: 'arrowRight', startOver: 'restart'};

export default function RemoteControlsPanel() {
  const {t} = useI18n();
  const {api} = useSession();
  const controls = useMemo(() => playbackControlsService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body)), [api]);
  const sessions = useMemo(() => new SessionsClient(localV1Http(api)), [api]);
  const read = useRead(() => controls.remote(), [controls], {every: 10000});
  const action = useAction();
  const items = read.data?.items ?? [];
  if (!items.length) return null;
  const send = (x: RemotePlaybackControls, what: TransportCommand | 'stop') => void action.run(async () => {
    if (what === 'stop') { await sessions.stop(x.sessionId); return; }
    const change = (from: {revision: string; positionMs: number}) => sessions.patch(x.sessionId, from.revision, what === 'pause' || what === 'resume' ? {state: what === 'pause' ? 'paused' : 'playing'} : {seek: {positionMs: seekTarget(what, from.positionMs) ?? 0, id: idempotencyKey()}});
    try { await change(x); }
    catch (e) {
      // The session moved since this list was read (its player reported, or changed something): send it again from where it is now.
      const current = (e as {current?: {revision?: unknown; positionMs?: unknown}} | null)?.current;
      if (!isStaleAnswer(e) || typeof current?.revision !== 'string') throw e;
      await change({revision: current.revision, positionMs: typeof current.positionMs === 'number' ? current.positionMs : x.positionMs});
    }
  }).then(() => read.reload());
  const seeks = (x: RemotePlaybackControls) => transportOffers(x, x.actions).filter((c): c is 'back' | 'forward' | 'startOver' => c !== 'pause' && c !== 'resume');
  const seconds = REMOTE_SKIP_MS / 1000;
  const seekWords = {back: t('player.skipBackSeconds', {seconds}), forward: t('player.skipForwardSeconds', {seconds}), startOver: t('action.startOver')};
  return (
    <SettingsGroup title={t('remote.title')} description={t('remote.lede')}>
      {action.error ? <div style={{padding: 12}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
      {items.map(x => (
        <SettingsRow key={x.sessionId} icon="tvDevice" label={x.device.name || t('remote.device')} help={x.item?.title ?? t('remote.hiddenTitle')}
          state={<Status tone={x.state === 'playing' ? 'healthy' : 'neutral'}>{t(remoteStateId(x.state))}</Status>}
          control={<div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
            {x.actions.includes('pause') && x.state === 'playing' ? <Button size="sm" variant="secondary" icon="pause" label={t('action.pause')} aria-label={t('remote.pauseOn', {device: x.device.name})} disabled={action.busy} onClick={() => send(x, 'pause')} /> : null}
            {x.actions.includes('resume') && x.state !== 'playing' ? <Button size="sm" variant="secondary" icon="play" label={t('entry.resume')} aria-label={t('remote.resumeOn', {device: x.device.name})} disabled={action.busy} onClick={() => send(x, 'resume')} /> : null}
            {x.actions.includes('stop') ? <Button size="sm" variant="ghost" icon="stop" label={t('action.stop')} aria-label={t('remote.stopOn', {device: x.device.name})} disabled={action.busy} onClick={() => send(x, 'stop')} /> : null}
            {seeks(x).length ? <Menu label={x.device.name || t('remote.device')} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('remote.moreOn', {device: x.device.name || t('remote.device')})} disabled={action.busy} />} items={seeks(x).map(command => ({id: command, label: seekWords[command], icon: seekIcons[command]}))} onSelect={id => send(x, id as TransportCommand)} /> : null}
          </div>} />
      ))}
    </SettingsGroup>
  );
}
