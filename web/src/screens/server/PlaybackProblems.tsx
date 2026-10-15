import {useMemo} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {clearPlaybackFailureMemory, groupPlaybackFailures, readPlaybackFailureMemory, type PlaybackFailure} from '@core/playback-failure-memory.ts';
import {useAction, useRead} from '../../admin/console';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Notice, SettingsFooter, SettingsGroup, SettingsNote, SettingsRow, Text} from '../../ui';
import {dependsOn} from '@core/freshness-hub.ts';

const PROBLEM_MOVES = [dependsOn.list('playbackProblems')];

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
  const read = useRead(() => readPlaybackFailureMemory(api), [api], {on: PROBLEM_MOVES});
  const action = useAction();
  const groups = useMemo(() => groupPlaybackFailures(read.data?.items ?? []), [read.data]);
  const navigate = useNavigate();
  if (!groups.length && !action.notice) return null;
  const line = (f: PlaybackFailure) => t('server.routes.line', {route: t(`server.routes.route.${f.route}`), engine: t(`server.routes.engine.${f.engine}`)});
  return (
    <SettingsGroup title={t('server.routes.title')} description={t('server.routes.lede')}>
      {read.error ? <SettingsNote><Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.error ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
      {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
      {groups.map(group => (
        // The title the file belongs to leads; what failed, and when, is underneath.
        <SettingsRow key={group.sourceId} icon="warning" label={group.item?.title || t('server.routes.unknownTitle')}
          help={[...group.failures.map(line), t('server.routes.when', {when: i18n.dateTime(group.reportedAtMs), until: i18n.dateTime(group.expiresAtMs)})].join(' · ')}
          state={group.failures[0]!.detail || group.failures[0]!.code ? <Text variant="mono" tone="tertiary" style={{fontSize: 12, overflowWrap: 'anywhere'}}>{group.failures[0]!.detail || group.failures[0]!.code}</Text> : undefined}
          control={<>
            {group.item ? <Button size="sm" variant="ghost" label={t('action.details')} aria-label={t('server.routes.openTitle')} onClick={() => void navigate({to: '/media/$itemId', params: {itemId: group.item!.id}, search: {}})} /> : null}
            <Button size="sm" variant="ghost" label={t('server.routes.forget')} aria-label={t('server.routes.forgetFor')} disabled={action.busy} onClick={() => void action.run(async () => { await clearPlaybackFailureMemory(api, group.sourceId); read.reload(); }, t('server.routes.forgotten'))} />
          </>} />
      ))}
      {read.data?.more ? <SettingsFooter><Text variant="caption" tone="tertiary">{t('server.routes.more')}</Text></SettingsFooter> : null}
    </SettingsGroup>
  );
}
