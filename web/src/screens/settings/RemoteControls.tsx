import {useMemo} from 'react';
import {playbackControlsService, type RemotePlaybackControls} from '@core/playback-controls.ts';
import {SessionsClient} from '@core/playback-v1/sessions.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import {useAction, useRead} from '../../admin/console';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Notice, SettingsGroup, SettingsRow, Status} from '../../ui';

/** What a remote session is doing, in a word the row can show. */
export const remoteStateId = (state: string) => (state === 'playing' ? 'remote.state.playing' : state === 'paused' ? 'remote.state.paused' : 'remote.state.starting') as 'remote.state.playing' | 'remote.state.paused' | 'remote.state.starting';

/**
 * Settings › This device › Playing on your other devices: what this profile is playing
 * elsewhere (a TV it was sent to with Play on or Cast, another browser), with Pause, Resume and
 * Stop. The server says which of those each one allows; a title this profile may not see is not
 * named. Nothing is shown while nothing is playing elsewhere.
 */
export default function RemoteControlsPanel() {
  const {t} = useI18n();
  const {api} = useSession();
  const controls = useMemo(() => playbackControlsService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body)), [api]);
  const sessions = useMemo(() => new SessionsClient(localV1Http(api)), [api]);
  const read = useRead(() => controls.remote(), [controls], {every: 10000});
  const action = useAction();
  const items = read.data?.items ?? [];
  if (!items.length) return null;
  const send = (x: RemotePlaybackControls, what: 'pause' | 'resume' | 'stop') => void action.run(async () => {
    if (what === 'stop') await sessions.stop(x.sessionId);
    else await sessions.patch(x.sessionId, x.revision, {state: what === 'pause' ? 'paused' : 'playing'});
  }).then(() => read.reload());
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
          </div>} />
      ))}
    </SettingsGroup>
  );
}
