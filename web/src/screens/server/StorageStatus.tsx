import {useMemo, useState} from 'react';
import {storageStatusService, type UpgradeCopy} from '@core/storage-status.ts';
import {loadResourceUsage, pauseBackgroundWork, type MaintenanceDocument, type MaintenanceSettings, type RuntimeStatus} from '@core/server-admin/server-forms.ts';
import type {RuntimeSettings} from '@core/console.ts';
import {QUIET_READ_EVERY_MS, useAction, useRead} from '../../admin/console';
import {operationId} from './operation-ids';
import {dependsOn} from '@core/freshness-hub.ts';
import {useServerForm} from '../settings/ServerForms';
import {useDownloads} from '../../app/downloads';
import {useSession} from '../../app/session';
import {useI18n, type MessageId} from '../../app/i18n';
import {Button, ConfirmDialog, KeyValue, Notice, SettingsEmpty, SettingsFooter, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, Status, Text} from '../../ui';

/** Schedule › pause background work for an hour, or resume it. The pause ends by itself; work already running finishes its step. */
export function BackgroundPauseRow() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const {form, state} = useServerForm<MaintenanceSettings, MaintenanceDocument>('maintenance');
  const action = useAction();
  const until = state.status?.pausedUntilUnixMillis ?? 0;
  const paused = until > Date.now();
  if (!state.status) return null;
  return <SettingsRow label={t(paused ? 'settings.server.resumeBackground' : 'settings.server.pauseBackground')} help={action.error || t('web.maintenance.pauseHeavyWorkHelp')}
    state={paused ? <Status tone="warning">{t('settings.server.backgroundPausedUntil', {time: i18n.time(until)})}</Status> : undefined}
    control={<Button size="sm" variant="secondary" label={t(paused ? 'server.background.resume' : 'server.background.pause')} loading={action.busy} onClick={() => void action.run(async () => { await pauseBackgroundWork({api}, !paused, operationId()); await form.load(); })} />} />;
}

/** Playback › whether the owner's own FFmpeg build passed the server's checks and is the one in use. Nothing when none is named. */
export function FFmpegStatusRow() {
  const i18n = useI18n();
  const {t} = i18n;
  const checked = useServerForm<RuntimeSettings, RuntimeStatus>('runtime').state.status?.ffmpegValidation;
  if (!checked?.path) return null;
  const refused = checked.state === 'refused';
  const reason = refused ? t(checked.reasonCode && i18n.has(checked.reasonCode) ? (checked.reasonCode as MessageId) : 'server.ffmpeg.refused') : '';
  const text = refused ? t('server.ffmpeg.refusedLine', {reason}) : checked.state === 'accepted' ? t(checked.inUse ? 'server.ffmpeg.inUse' : 'server.ffmpeg.afterRestart') : t('server.ffmpeg.unchecked');
  return <SettingsRow label={t('server.ffmpeg.status')} help={refused && checked.missingFeatures.length ? t('server.ffmpeg.missing', {features: i18n.list(checked.missingFeatures.slice(0, 8))}) : undefined} control={<Status tone={refused ? 'danger' : checked.state === 'accepted' && checked.inUse ? 'healthy' : 'neutral'}>{text}</Status>} />;
}

const DOWNLOAD_MOVES = [dependsOn.list('downloads')];

/** Storage & backups › what is in use right now, beside the limits the rows below set. */
export function ResourceUsagePanel() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const read = useRead(() => loadResourceUsage(api), [api], {every: QUIET_READ_EVERY_MS});
  const u = read.data;
  const used = (part: {trackedBytes: number; limitBytes: number; measured: boolean}) => (part.measured ? t('server.usage.ofLimit', {used: i18n.bytes(part.trackedBytes), limit: i18n.bytes(part.limitBytes)}) : t('server.usage.notMeasuredYet'));
  return (
    <SettingsGroup title={t('server.usage.title')} description={t('server.usage.lede')}>
      {read.error ? <SettingsNote><Notice tone={u ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {!u && read.loading ? <SettingsLoading thing={t('server.thing.usage')} /> : null}
      {u ? <SettingsRow label={t('server.usage.streams')} meta={u.streamLimit ? t('server.usage.countOf', {used: u.streams, limit: u.streamLimit}) : i18n.number(u.streams)} /> : null}
      {u?.connections ? <SettingsRow label={t('server.usage.connections')} help={t('server.usage.connectionsHelp', {reserve: u.connections.consoleReserve})} meta={t('server.usage.countOf', {used: u.connections.ordinary, limit: u.connections.ordinaryLimit})} /> : null}
      {u ? <SettingsRow label={t('server.usage.analysis')} meta={used(u.analysis)} /> : null}
      {u ? <SettingsRow label={t('server.usage.artwork')} meta={used(u.localArtwork)} /> : null}
    </SettingsGroup>
  );
}

/**
 * Storage & backups › where the server keeps things, how much space the database can give back,
 * and the copies it made before an upgrade. The folders are chosen when the server starts, so
 * they are shown, not set, here.
 */
export function StorageStatusPanel() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const service = useMemo(() => storageStatusService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body)), [api]);
  // Space the database gives back, and a copy made by an upgrade, appear by themselves.
  const read = useRead(() => service.read(), [service], {every: QUIET_READ_EVERY_MS});
  const action = useAction();
  const [removing, setRemoving] = useState<UpgradeCopy>();
  const s = read.data;
  const path = (value: string) => <Text variant="mono" tone="secondary" style={{fontSize: '0.75rem', overflowWrap: 'anywhere'}}>{value}</Text>;
  return (
    <SettingsGroup title={t('server.places.title')} description={t('server.places.lede')}>
      {read.error ? <SettingsNote><Notice tone={s ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.error && !removing ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
      {!s && read.loading ? <SettingsLoading thing={t('server.thing.storagePlaces')} /> : null}
      {s ? <SettingsRow label={t('server.places.folders')} stack start control={<KeyValue rows={[[t('server.places.database'), path(s.databasePath)], [t('server.places.cache'), path(s.cacheDirectory)], [t('server.places.backups'), path(s.backupDirectory)]]} />} /> : null}
      {s ? <SettingsRow label={t('server.places.reclaim')} help={s.lastReclaimedAt ? t('server.places.reclaimedLast', {when: i18n.dateTime(s.lastReclaimedAt), size: i18n.bytes(s.lastReclaimedBytes)}) : t('server.places.reclaimHelp')} meta={i18n.bytes(s.reclaimableBytes)} /> : null}
      {s?.upgradeCopies.map(copy => (
        <SettingsRow key={copy.id} icon="storage" label={t('server.places.upgradeCopy', {when: i18n.dateTime(copy.createdAt)})} help={<>{i18n.bytes(copy.bytes)}{' · '}{path(copy.path)}</>}
          control={<Button size="sm" variant="ghost" icon="trash" label={t('action.delete')} onClick={() => { action.clear(); setRemoving(copy); }} />} />
      ))}
      {s && !s.upgradeCopies.length ? <SettingsRow label={t('server.places.upgradeCopies')} help={t('server.places.upgradeCopiesNone')} /> : null}
      <ConfirmDialog open={!!removing} onOpenChange={o => !o && setRemoving(undefined)} title={t('server.places.deleteCopyTitle')} body={t('server.places.deleteCopyBody')} confirmLabel={t('action.delete')} cancelLabel={t('action.cancel')} busy={action.busy} error={action.error || undefined}
        onConfirm={() => void action.run(async () => { if (!removing) return; await service.removeUpgradeCopy(removing); setRemoving(undefined); read.reload(); })} />
    </SettingsGroup>
  );
}

/** Storage & backups › what downloads take on the server, person by person, with a way to remove one person's. */
export function DownloadUsagePanel() {
  const i18n = useI18n();
  const {t} = i18n;
  const {service} = useDownloads();
  // What people have downloaded grows and shrinks as their devices come and go; the feed says when.
  // Everyone the owner has asked to see is read again: the first page and each "Show more" after it.
  const [pages, setPages] = useState(1);
  const read = useRead(async () => {
    if (!service) return null;
    const first = await service.serverUsage();
    const profiles = [...first.profiles];
    let cursor = first.profilesNextCursor ?? '';
    for (let page = 1; page < pages && cursor; page++) {
      const next = await service.serverUsage(cursor);
      profiles.push(...next.profiles);
      cursor = next.profilesNextCursor ?? '';
    }
    return {...first, profiles, profilesNextCursor: cursor};
  }, [service, pages], {on: DOWNLOAD_MOVES});
  const action = useAction();
  const [revoking, setRevoking] = useState<{key: string; name: string}>();
  const usage = read.data;
  if (!service || usage === null) return null;
  const name = (p: {accountName?: string; profileName?: string}) => (p.accountName && p.profileName && p.accountName !== p.profileName ? `${p.accountName} · ${p.profileName}` : p.profileName || p.accountName || t('server.people.removedPerson'));
  return (
    <SettingsGroup title={t('server.downloadUsage.title')} description={t('server.downloadUsage.lede')}>
      {read.error ? <SettingsNote><Notice tone={usage ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.error && !revoking ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
      {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
      {!usage && read.loading ? <SettingsLoading thing={t('server.thing.downloads')} /> : null}
      {usage ? <SettingsRow label={t('server.downloadUsage.total')} help={usage.remainingBytes !== null ? t('server.downloadUsage.remaining', {size: i18n.bytes(usage.remainingBytes)}) : undefined} meta={t('server.downloadUsage.totalLine', {size: i18n.bytes(usage.distinctArtifactBytes), count: usage.serverCount})} /> : null}
      {usage?.profiles.map(p => (
        <SettingsRow key={p.profileKey ?? p.profileId} icon="profile" label={name(p)} meta={t('server.downloadUsage.totalLine', {size: i18n.bytes(p.storedBytes ?? p.bytes), count: p.count})}
          control={p.profileKey && p.count ? <Button size="sm" variant="ghost" label={t('server.downloadUsage.remove')} onClick={() => { action.clear(); setRevoking({key: p.profileKey!, name: name(p)}); }} /> : undefined} />
      ))}
      {usage && !usage.profiles.length ? <SettingsEmpty title={t('server.downloadUsage.none')} /> : null}
      {usage?.profilesNextCursor ? <SettingsFooter><Button size="sm" variant="ghost" label={t('server.showMore')} disabled={read.loading} onClick={() => setPages(n => n + 1)} /></SettingsFooter> : null}
      <ConfirmDialog open={!!revoking} onOpenChange={o => !o && setRevoking(undefined)} title={t('server.downloadUsage.removeTitle', {name: revoking?.name ?? ''})} body={t('server.downloadUsage.removeBody')} confirmLabel={t('server.downloadUsage.remove')} busy={action.busy} error={action.error || undefined}
        onConfirm={() => void action.run(async () => {
          if (!revoking) return;
          // The server works in passes; keep going until nothing is left for this person.
          for (let pass = 0; pass < 200; pass++) { const out = await service.revokeProfile(revoking.key); if (!out.more) break; }
          setRevoking(undefined); read.reload();
        }, t('server.downloadUsage.removed'))} />
    </SettingsGroup>
  );
}
