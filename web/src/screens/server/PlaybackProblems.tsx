import {useMemo} from 'react';
import {clearPlaybackFailureMemory, groupPlaybackFailures, readPlaybackFailureMemory, type PlaybackFailure} from '@core/playback-failure-memory.ts';
import {QUIET_READ_EVERY_MS, useAction, useRead} from '../../admin/console';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Notice, SettingsFooter, SettingsGroup, SettingsNote, SettingsRow, Text} from '../../ui';

/**
 * Streaming › Playback problems Portico remembers: ways of playing a file that failed on a
 * device and are being avoided until the time shown. Each file is one row with what failed, in
 * the server's own words underneath, and Forget, which lets the first way be tried again.
 * Nothing is shown while there are none.
 */
export function PlaybackProblemsPanel() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  // A failure a device reports, and one that expires, show and go by themselves.
  const read = useRead(() => readPlaybackFailureMemory(api), [api], {every: QUIET_READ_EVERY_MS});
  const action = useAction();
  const groups = useMemo(() => groupPlaybackFailures(read.data?.items ?? []), [read.data]);
  if (!groups.length && !action.notice) return null;
  const line = (f: PlaybackFailure) => t('server.routes.line', {route: t(`server.routes.route.${f.route}`), engine: t(`server.routes.engine.${f.engine}`)});
  return (
    <SettingsGroup title={t('server.routes.title')} description={t('server.routes.lede')}>
      {read.error ? <SettingsNote><Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.error ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
      {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
      {groups.map(group => (
        <SettingsRow key={group.sourceId} icon="warning" label={line(group.failures[0]!)}
          help={[...group.failures.slice(1).map(line), t('server.routes.when', {when: i18n.dateTime(group.reportedAtMs), until: i18n.dateTime(group.expiresAtMs)})].join(' · ')}
          state={group.failures[0]!.detail || group.failures[0]!.code ? <Text variant="mono" tone="tertiary" style={{fontSize: 12, overflowWrap: 'anywhere'}}>{group.failures[0]!.detail || group.failures[0]!.code}</Text> : undefined}
          control={<Button size="sm" variant="ghost" label={t('server.routes.forget')} aria-label={t('server.routes.forgetFor')} disabled={action.busy} onClick={() => void action.run(async () => { await clearPlaybackFailureMemory(api, group.sourceId); read.reload(); }, t('server.routes.forgotten'))} />} />
      ))}
      {read.data?.more ? <SettingsFooter><Text variant="caption" tone="tertiary">{t('server.routes.more')}</Text></SettingsFooter> : null}
    </SettingsGroup>
  );
}
