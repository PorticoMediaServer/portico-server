import {useEffect, useMemo, useRef, useState} from 'react';
import {getBackups, isAuthenticationFailure, restartServer, waitForServerAnswer} from '@core/backups.ts';
import {serverChanged} from '../../app/server-changes';
import {dependsOn} from '@core/freshness-hub.ts';
import {useServerForm} from '../settings/ServerForms';
import {parseEnvelope, parseStorageReport, parseUpdateReport, type StorageUsage} from '@core/administration.ts';
import {emptyMissingTitles, listMissingTitles} from '@core/title-identity.ts';
import {problem, useAction, useConsole, useRead} from '../../admin/console';
import {createOperationIds} from './operation-ids';
import {useCursorRead, withCursor} from '../../admin/cursor-read';
import {ListPager} from './ListPager';
import {useSession} from '../../app/session';
import {useI18n, type I18n} from '../../app/i18n';
import {Button, ConfirmDialog, Freshness, Input, Notice, SettingsEmpty, SettingsFooter, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, Status, Switch} from '../../ui';
import {storageCategoryWords} from '@core/presentation/index.ts';

// A scan that finds files gone, or back, moves this list.
const MISSING_MOVES = [dependsOn.content()];
const when = (i18n: I18n, iso: string) => (iso ? i18n.dateTime(iso) : '—');

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
  const i18n = useI18n();
  const {t} = i18n;
  const read = useRead(() => admin.get('/v1/admin/storage-usage', parseStorageReport), [admin]);
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const [chosen, setChosen] = useState<StorageUsage>();
  const [days, setDays] = useState(30);
  const report = read.data;
  const chosenName = chosen ? storageCategoryWords(i18n, chosen).name : '';
  return (
    <SettingsGroup title={t('settings.server.diskUsage')} description={t('web.maintenance.storageLede')} action={<Freshness at={read.at} refreshing={read.loading} onRefresh={read.reload} />}>
      {read.error ? <SettingsNote><Notice tone={report ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
      {!report && read.loading ? <SettingsLoading thing={t('server.thing.diskUsage')} /> : null}
      {report ? <SettingsRow label={t('web.maintenance.totalLabel')} meta={i18n.bytes(report.totalBytes)} help={report.truncated ? t('web.maintenance.truncatedHelp') : undefined} /> : null}
      {report?.categories.filter(c => c.present).map(c => {
        const words = storageCategoryWords(i18n, c);
        return <SettingsRow key={c.id} label={words.name} help={c.regenerable ? t('web.maintenance.regenerableHelp', {description: words.description}) : words.description} meta={i18n.bytes(c.bytes)}
          control={c.cleanable && c.bytes > 0 ? <Button size="sm" variant="ghost" label={t('server.cleanUp')} onClick={() => { setChosen(c); setDays(30); action.clear(); }} /> : undefined} />;
      })}
      {/* Tier 2: files go. The word to type is the folder's name as shown; the server is sent its id. */}
      <ConfirmDialog open={!!chosen} onOpenChange={o => !o && setChosen(undefined)} title={t('web.maintenance.cleanUpTitle', {name: chosenName})} typedConfirmation={chosen ? chosenName : undefined} busy={action.busy} error={action.error || undefined} destructive={!chosen?.regenerable}
        body={<div style={{display: 'flex', flexDirection: 'column', gap: 12}}><span>{t('server.cleanup.body', {canRecreate: chosen?.regenerable ? 'yes' : 'no'})}</span><Input label={t('web.maintenance.olderThan')} type="number" min={0} max={3650} value={days} onChange={e => setDays(Math.max(0, Number(e.target.value) || 0))} style={{width: 140}} /></div>}
        confirmLabel={t('server.cleanUp')} onConfirm={() => void action.run(async () => { if (!chosen) return; const payload = {olderThanDays: days, confirmation: chosen.id}; const key = JSON.stringify({id: chosen.id, ...payload}); await admin.send<{bytesFreedText: string; filesRemoved: number}>(`/v1/admin/storage-usage/${encodeURIComponent(chosen.id)}/cleanup`, {...payload, operationId: opIds.forPayload(key)}); opIds.release(); setChosen(undefined); read.reload(); }, t('server.cleanup.done'))} />
    </SettingsGroup>
  );
}

/** Storage & backups › Missing titles: titles whose files are no longer on disk. They stay in
 * the library until the owner's rule (the rows above this list) removes them, and come back on
 * their own if the files return. "Remove them now" does not wait for the rule. */
export function MissingTitlesPanel() {
  const {api} = useSession();
  const i18n = useI18n();
  const {t} = i18n;
  const read = useCursorRead(cursor => listMissingTitles(api, {cursor: cursor || undefined, limit: 100}), [api], {on: MISSING_MOVES});
  const action = useAction();
  const [asking, setAsking] = useState(false);
  const [removed, setRemoved] = useState<number>();
  const page = read.data;
  const total = page?.total ?? 0;
  return (
    <>
      {read.error ? <SettingsNote><Notice tone={page ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.error && !asking ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
      {removed !== undefined ? <SettingsNote><Notice tone="success" compact>{t('web.missingTitles.removed', {count: removed})}</Notice></SettingsNote> : null}
      {!page && read.loading ? <SettingsLoading thing={t('server.thing.missingTitles')} /> : null}
      {page?.items.map(entry => (
        <SettingsRow key={entry.id} icon="film" label={entry.parentTitle ? `${entry.parentTitle} · ${entry.title}` : entry.title}
          help={entry.removedAt ? t('web.missingTitles.sinceRemoved', {when: when(i18n, entry.missingSince), removes: when(i18n, entry.removedAt)}) : t('web.missingTitles.since', {when: when(i18n, entry.missingSince)})} />
      ))}
      {page && !page.items.length ? <SettingsEmpty title={t('web.missingTitles.none')} /> : null}
      {total ? <SettingsFooter>
        <Button size="sm" variant="secondary" label={t('web.missingTitles.removeNow')} onClick={() => { action.clear(); setRemoved(undefined); setAsking(true); }} />
        <Status tone="neutral">{t('web.missingTitles.count', {count: total})}</Status>
      </SettingsFooter> : null}
      <ConfirmDialog open={asking} onOpenChange={o => !o && setAsking(false)} title={t('web.missingTitles.removeTitle')} typedConfirmation={String(total)} busy={action.busy} error={action.error || undefined}
        body={t('server.missing.removeBody', {count: total})}
        confirmLabel={t('web.missingTitles.removeConfirm')} onConfirm={() => void action.run(async () => {
          const out = await emptyMissingTitles(api);
          setRemoved(out.titles);
          setAsking(false);
          read.reload();
        })} />
      <ListPager pages={read} />
    </>
  );
}

/** The address the server checks for new versions when the owner has not changed it, from the settings registry; nothing when it cannot be read. */
function useDefaultUpdateFeed(): string | undefined {
  const client = useConsole();
  const read = useRead(async () => { const fields = ((await client.registry()) as {fields?: readonly {id?: unknown; default?: unknown}[]} | null)?.fields; const field = Array.isArray(fields) ? fields.find(f => f?.id === 'updates.feedUrl') : undefined; return typeof field?.default === 'string' ? field.default : ''; }, [client]);
  return read.data || undefined;
}

/**
 * General › "Check for new versions": the update notice's on/off. Off clears the address the
 * server checks; on puts back the usual one. An address the owner set by hand is left as it is.
 * Part of the page's one Save.
 */
function UpdateCheckRow() {
  const {t} = useI18n();
  const {form, state} = useServerForm<Record<string, unknown>>('runtime');
  const usual = useDefaultUpdateFeed();
  const feed = state.draft?.['updates.feedUrl'];
  if (typeof feed !== 'string') return null;
  return <SettingsRow label={t('server.updates.check')} help={t('server.updates.checkHelp')} control={<Switch checked={feed !== ''} disabled={feed === '' && !usual} label={t('server.updates.check')} onCheckedChange={on => form.set(draft => ({...draft, 'updates.feedUrl': on ? usual ?? '' : ''}))} />} />;
}

/**
 * General › Restart: an owner action, asked first. The page waits for the server to answer again
 * and says so; an ended sign-in goes to the sign-in screen through the session as usual.
 */
export function RestartPanel() {
  const {t} = useI18n();
  const {api, system, session} = useSession();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  const [asking, setAsking] = useState(false);
  const [phase, setPhase] = useState<'idle' | 'restarting' | 'back'>('idle');
  const [error, setError] = useState('');
  const stop = useRef<AbortController | null>(null);
  useEffect(() => () => stop.current?.abort(), []);
  const restart = () => {
    const controller = stop.current = new AbortController();
    setAsking(false); setError(''); setPhase('restarting');
    restartServer(api, controller.signal)
      // The server may already be going down when it answers; either way, wait for it to come back.
      .catch(e => { if ((e as {status?: number} | null)?.status) throw e; })
      .then(() => waitForServerAnswer(() => getBackups(api, serverId, controller.signal), {signal: controller.signal}))
      .then(() => { if (!controller.signal.aborted) { setPhase('back'); serverChanged(); } }, e => { if (controller.signal.aborted || isAuthenticationFailure(e)) return; setPhase('idle'); setError(problem(e, 'action') || t('server.restart.failed')); });
  };
  return (
    <SettingsGroup title={t('web.maintenance.serverRestart')}>
      {error ? <SettingsNote><Notice tone="error" compact>{error}</Notice></SettingsNote> : null}
      {phase === 'back' ? <SettingsNote><Notice tone="success" compact>{t('server.restart.back')}</Notice></SettingsNote> : null}
      <SettingsRow label={t('web.maintenance.serverRestart')} help={t('web.maintenance.serverRestartHelp')} state={phase === 'restarting' ? <span role="status">{t('server.restart.waiting')}</span> : undefined}
        control={<Button size="sm" variant="secondary" icon="refresh" label={t('web.maintenance.serverRestart')} loading={phase === 'restarting'} onClick={() => setAsking(true)} />} />
      <ConfirmDialog open={asking} onOpenChange={setAsking} title={t('server.restart.title')} body={t('server.restart.body')} confirmLabel={t('web.maintenance.serverRestart')} cancelLabel={t('action.cancel')} destructive={false} onConfirm={restart} />
    </SettingsGroup>
  );
}

/** General › Version and updates: the one place that says what is running and what is new. */
export function UpdatesPanel() {
  const {api} = useSession();
  const i18n = useI18n();
  const {t} = i18n;
  // APL-SYS-12 (parser probe): GET /v1/admin/updates is a registry route that answers the bare
  // report, not the administration envelope, so reading it through the envelope always failed
  // with "couldn't read".
  const read = useRead(() => api.request<unknown>('/v1/admin/updates').then(parseUpdateReport), [api]);
  const report = read.data;
  const tone = report?.status === 'update-available' ? 'accent' : report?.status === 'current' ? 'healthy' : 'neutral';
  return (
    <SettingsGroup title={t('web.general.updates')} description={t('web.maintenance.updatesLede')} action={<Freshness at={read.at} refreshing={read.loading} onRefresh={read.reload} />}>
      {read.error ? <SettingsNote><Notice tone={report ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {report ? (
        <>
          <SettingsRow label={t('web.maintenance.thisServer')} meta={report.current.version || t('web.maintenance.devBuild')} help={report.current.builtAt ? t('web.maintenance.builtAt', {when: when(i18n, report.current.builtAt), channel: report.channel}) : t('web.maintenance.channelOnly', {channel: report.channel})} />
          <SettingsRow label={report.status === 'update-available' ? t('web.maintenance.updateAvailable') : report.status === 'current' ? t('web.maintenance.upToDate') : t('web.maintenance.updateCheck')} help={report.message || (report.latest?.notes ?? undefined)}
            state={<Status tone={tone}>{report.status === 'update-available' ? report.latest?.version ?? t('web.maintenance.newVersion') : report.status === 'current' ? t('web.maintenance.currentVersion') : report.status === 'no-feed' ? t('web.maintenance.noFeed') : t('web.maintenance.updateUnavailable')}</Status>}
            control={report.latest?.url ? <Button size="sm" variant="secondary" icon="external" label={t('web.maintenance.releaseNotes')} href={report.latest.url} /> : undefined} />
        </>
      ) : null}
      <UpdateCheckRow />
    </SettingsGroup>
  );
}
