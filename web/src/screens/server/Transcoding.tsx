import {useEffect} from 'react';
import {PlaybackSettingsService, type PlaybackSettingsSnapshot} from '@core/playback-settings.ts';
import {deliveryStatusRows} from '@core/server-admin/status-words.ts';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {useService} from '../../app/content';
import {Freshness, KeyValue, Notice, Surface, Text} from '../../ui';
import {useI18n} from '../../app/i18n';
import s from './Server.module.css';

/** CD-46: capacity caps are null for unlimited; an explicit value is 1–1000000. Zero is never
 * sent (the server refuses it). The hardware/session settings where 0 means unlimited are a
 * different control and stay untouched. */
export const clampCap = (value: string): number | null => {
  if (value.trim() === '') return null;
  return Math.min(1000000, Math.max(1, Math.floor(Number(value)) || 1));
};
/** Playback › Right now: what the server reports about delivery, above the settings. */
export function TranscodeStatusPanel() {
  const {api} = useSession();
  const {t} = useI18n();
  const scope = useViewerScope();
  const {service, snapshot} = useService<PlaybackSettingsService, PlaybackSettingsSnapshot>(() => new PlaybackSettingsService({api: {requestBounded: (path, _max, signal) => api.request(path, 'GET', undefined, signal)}, scope}), [api, scope]);
  useEffect(() => {
    void service.refresh();
  }, [service]);
  const d = snapshot.observation?.diagnostics;
  return (
    <Surface>
      <div className={s.panelHead}>
        <span className={s.panelTitle}>{t('settings.server.transcodeStatus')}</span>
        <Freshness at={snapshot.observation?.observedAt} onRefresh={() => void service.refresh()} refreshing={snapshot.phase === 'loading'} verb="Observed" />
      </div>
      {snapshot.phase === 'access-denied' ? <Text variant="caption" tone="tertiary">{t('web.transcoding.ownerOnly')}</Text> : null}
      {snapshot.error ? <Notice tone="error" compact>{snapshot.error}</Notice> : null}
      {d ? <KeyValue rows={deliveryStatusRows(t, d).map(([k, v]) => [k, v] as [string, string])} /> : null}
    </Surface>
  );
}
