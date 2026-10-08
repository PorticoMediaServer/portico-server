import {useMemo, useState} from 'react';
import {parseEnvelope, parseStorageReport, parseTrashPage, parseUpdateReport, type StorageUsage, type TrashEntry} from '@core/administration.ts';
import {isFullTrashList} from '@core/server-admin/panel-logic.ts';
export {isFullTrashList} from '@core/server-admin/panel-logic.ts';
import {useAction, useRead} from '../../admin/console';
import {createOperationIds} from './operation-ids';
import {useCursorRead, withCursor} from '../../admin/cursor-read';
import {ListPager} from './ListPager';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {formatBytes} from '@core/presentation/index.ts';
import {Badge, Button, ConfirmDialog, Freshness, Input, Notice, SettingsGroup, SettingsRow, Status, Text} from '../../ui';

const when = (iso: string) => (iso ? new Date(iso).toLocaleString(undefined, {dateStyle: 'medium', timeStyle: 'short'}) : '—');

/** Typed calls against the administration envelope: every response names the server it came
 * from, and a response from a different server is refused rather than shown. */
function useAdmin() {
  const {api, system, session} = useSession();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  return useMemo(() => ({
    get: async <T,>(path: string, read: (raw: unknown) => T) => parseEnvelope(await api.request<unknown>(path), serverId, read).result,
    send: async <T,>(path: string, body: unknown, read: (raw: unknown) => T = raw => raw as T, method = 'POST') => parseEnvelope(await api.request<unknown>(path, method, body), serverId, read).result,
  }), [api, serverId]);
}

/** Storage & backups › Disk usage: what the server's own files take, with clean-up. Anything that destroys data asks for a typed confirmation. */
export function DiskUsagePanel() {
  const admin = useAdmin();
  const {t} = useI18n();
  const read = useRead(() => admin.get('/v1/admin/storage-usage', parseStorageReport), [admin]);
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const [chosen, setChosen] = useState<StorageUsage>();
  const [days, setDays] = useState(30);
  const report = read.data;
  return (
    <SettingsGroup title={t('settings.server.diskUsage')} description={t('web.maintenance.storageLede')}>
      {read.error ? <div style={{padding: 12}}><Notice tone={report ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {action.notice ? <div style={{padding: 12}}><Notice tone="success" compact>{action.notice}</Notice></div> : null}
      {report ? <SettingsRow label={t('web.maintenance.totalLabel')} meta={formatBytes(report.totalBytes)} help={report.truncated ? t('web.maintenance.truncatedHelp') : undefined} /> : null}
      {report?.categories.filter(c => c.present).map(c => (
        <SettingsRow key={c.id} label={c.name} help={c.regenerable ? t('web.maintenance.regenerableHelp', {description: c.description}) : c.description} meta={formatBytes(c.bytes)}
          control={c.cleanable && c.bytes > 0 ? <Button size="sm" variant="ghost" label={t('server.cleanUp')} onClick={() => { setChosen(c); setDays(30); action.clear(); }} /> : undefined} />
      ))}
      <div style={{padding: '8px 16px 16px'}}><Freshness at={read.at} refreshing={read.loading} onRefresh={read.reload} /></div>
      <ConfirmDialog open={!!chosen} onOpenChange={o => !o && setChosen(undefined)} title={t('web.maintenance.cleanUpTitle', {name: chosen?.name ?? ''})} typedConfirmation={chosen?.id} busy={action.busy} error={action.error} destructive={!chosen?.regenerable}
        body={<div style={{display: 'flex', flexDirection: 'column', gap: 12}}><span>{t('web.maintenance.cleanUpBody', {canRecreate: chosen?.regenerable ? 'yes' : 'no', id: chosen?.id ?? ''})}</span><Input label={t('web.maintenance.olderThan')} type="number" min={0} max={3650} value={days} onChange={e => setDays(Math.max(0, Number(e.target.value) || 0))} style={{width: 140}} /></div>}
        confirmLabel={t('server.cleanUp')} onConfirm={() => void action.run(async () => { if (!chosen) return; const payload = {olderThanDays: days, confirmation: chosen.id}; const key = JSON.stringify({id: chosen.id, ...payload}); const out = await admin.send<{bytesFreedText: string; filesRemoved: number}>(`/v1/admin/storage-usage/${encodeURIComponent(chosen.id)}/cleanup`, {...payload, operationId: opIds.forPayload(key)}); opIds.release(); setChosen(undefined); read.reload(); action.clear(); return out; }, 'Cleaned up.')} />
    </SettingsGroup>
  );
}

/** Storage & backups › Deleted titles: what is held before it is removed for good. */
export function DeletedTitlesPanel() {
  const admin = useAdmin();
  const {t} = useI18n();
  const read = useCursorRead(cursor => admin.get(withCursor('/v1/admin/trash?state=held&limit=100', cursor), parseTrashPage), [admin]);
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const [emptying, setEmptying] = useState<'all' | 'expired'>();
  const page = read.data;
  // CD-42: "due" is safe only when this page is the whole held listing — first page, no next
  // page, and the loaded count equals the server's held total. A last page without a next
  // cursor must NOT be assumed complete.
  const fullList = isFullTrashList(page, read.canPrevious, read.canNext);
  const dueItems = (page?.items ?? []).filter(item => item.expired);
  const dueReady = fullList && dueItems.length > 0;
  // CD-42: confirm with the server's held total, never the visible page's count.
  const emptyCount = emptying === 'expired' ? dueItems.length : (page?.heldCount ?? 0);
  const restore = (entry: TrashEntry) => void action.run(async () => {
    const key = JSON.stringify({id: entry.id});
    const out = await admin.send<{conflicts: string[]; reindexRequired: boolean}>(`/v1/admin/trash/${encodeURIComponent(entry.id)}/restore`, {operationId: opIds.forPayload(key)});
    opIds.release();
    read.reload();
    if (out.conflicts?.length) throw new Error(`${out.conflicts.length} file${out.conflicts.length === 1 ? '' : 's'} could not be put back because something else is already there.`);
  }, 'Restored.');
  return (
    <SettingsGroup title={t('web.maintenance.trash')} description={t('web.maintenance.trashLede')}>
      {read.error ? <div style={{padding: 12}}><Notice tone={page ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {action.error && !emptying ? <div style={{padding: 12}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
      {action.notice ? <div style={{padding: 12}}><Notice tone="success" compact>{action.notice}</Notice></div> : null}
      {page?.items.map(entry => (
        <SettingsRow key={entry.id} icon="trash" label={<>{entry.title} {entry.expired ? <Badge tone="warning">{t('web.maintenance.dueForRemoval')}</Badge> : null}</>} help={t('web.maintenance.trashMeta', {count: entry.fileCount, bytes: formatBytes(entry.bytes), when: when(entry.trashedAt), expires: entry.expiresAt ? when(entry.expiresAt) : 'none'})}
          control={<Button size="sm" variant="ghost" label={t('web.maintenance.putBack')} disabled={action.busy} onClick={() => restore(entry)} />} />
      ))}
      {page && !page.items.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.maintenance.nothingWaiting')}</Text></div> : null}
      {page?.items.length ? <div style={{padding: '8px 16px 16px', display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center'}}>
        <Button size="sm" variant="outline" label={t('web.maintenance.removeDue')} disabled={!dueReady} onClick={() => { action.clear(); setEmptying('expired'); }} />
        <Button size="sm" variant="danger" label={t('web.maintenance.removeAll')} onClick={() => { action.clear(); setEmptying('all'); }} />
        <Status tone="neutral">{`${page.heldCount} held`}</Status>
        {!fullList ? <Text variant="caption" tone="tertiary">{t('web.trash.dueNeedsFullList')}</Text> : null}
      </div> : null}
      <ConfirmDialog open={!!emptying} onOpenChange={o => !o && setEmptying(undefined)} title={t('web.maintenance.removeTitle')} typedConfirmation={String(emptyCount)} busy={action.busy} error={action.error}
        body={t('web.maintenance.removeBody', {count: emptyCount})}
        confirmLabel={t('web.maintenance.removeConfirm')} onConfirm={() => void action.run(async () => {
          const total = emptying === 'expired' ? (read.data?.items ?? []).filter(item => item.expired).length : (read.data?.heldCount ?? 0);
          const payload = {expiredOnly: emptying === 'expired', confirmation: String(total)};
          const key = JSON.stringify(payload);
          try {
            await admin.send('/v1/admin/trash/empty', {...payload, operationId: opIds.forPayload(key)});
          } catch (e) {
            // The held total moved under us: refetch and ask to confirm the fresh total.
            if ((e as {code?: string})?.code === 'confirmation_mismatch' || (e as {status?: number})?.status === 409) { opIds.release(); read.reload(); }
            throw e;
          }
          opIds.release(); setEmptying(undefined); read.reload();
        }, 'Removed.')} />
      <ListPager pages={read} />
    </SettingsGroup>
  );
}

/** General › Version and updates: the one place that says what is running and what is new. */
export function UpdatesPanel() {
  const {api} = useSession();
  const {t} = useI18n();
  // APL-SYS-12 (parser probe): GET /v1/admin/updates is a registry route that answers the bare
  // report, not the administration envelope, so reading it through the envelope always failed
  // with "couldn't read".
  const read = useRead(() => api.request<unknown>('/v1/admin/updates').then(parseUpdateReport), [api]);
  const report = read.data;
  const tone = report?.status === 'update-available' ? 'accent' : report?.status === 'current' ? 'healthy' : 'neutral';
  return (
    <SettingsGroup title={t('web.general.updates')} description={t('web.maintenance.updatesLede')}>
      {read.error ? <div style={{padding: 12}}><Notice tone={report ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {report ? (
        <>
          <SettingsRow label={t('web.maintenance.thisServer')} meta={report.current.version || t('web.maintenance.devBuild')} help={report.current.builtAt ? t('web.maintenance.builtAt', {when: when(report.current.builtAt), channel: report.channel}) : t('web.maintenance.channelOnly', {channel: report.channel})} />
          <SettingsRow label={report.status === 'update-available' ? t('web.maintenance.updateAvailable') : report.status === 'current' ? t('web.maintenance.upToDate') : t('web.maintenance.updateCheck')} help={report.message || (report.latest?.notes ?? undefined)}
            state={<Status tone={tone}>{report.status === 'update-available' ? report.latest?.version ?? t('web.maintenance.newVersion') : report.status === 'current' ? t('web.maintenance.currentVersion') : report.status === 'no-feed' ? t('web.maintenance.noFeed') : t('web.maintenance.updateUnavailable')}</Status>}
            control={report.latest?.url ? <Button size="sm" variant="secondary" icon="external" label={t('web.maintenance.releaseNotes')} href={report.latest.url} /> : undefined} />
        </>
      ) : null}
      <div style={{padding: '8px 16px 16px'}}><Freshness at={read.at} refreshing={read.loading} onRefresh={read.reload} /></div>
    </SettingsGroup>
  );
}
