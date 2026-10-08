import {useEffect, useMemo, useState} from 'react';
import {StorageManagementService, type StorageManagementSnapshot, type StorageMount} from '@core/storage-management.ts';
import {RemoteSourceService, type DAVConfiguration, type RemoteSource} from '@core/remote-sources.ts';
import {davCommandSucceeded, davConfiguration, mountCommandSucceeded, stableStorageOperationId, type WebDavDraft} from '@core/server-admin/panel-logic.ts';
export {davCommandSucceeded, davConfiguration, mountCommandSucceeded, stableStorageOperationId, storageOperationId, type WebDavDraft} from '@core/server-admin/panel-logic.ts';
import {problem, sentence, useAction} from '../../admin/console';
import {useService} from '../../app/content';
import {formatBytes} from '@core/presentation/index.ts';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {currentI18n} from '../../app/i18n';
import {Badge, Button, Checkbox, ConfirmDialog, Dialog, Input, Loading, Menu, Notice, PasswordInput, SettingsGroup, SettingsRow, Text, TextArea, type MenuItem} from '../../ui';
import {SectionHeader} from './Server';

const mountTone = (m: StorageMount): 'healthy' | 'warning' | 'danger' | 'neutral' => (m.observedState === 'mounted' ? 'healthy' : m.observedState === 'failed' || m.observedState === 'quarantined' ? 'danger' : m.observedState === 'starting' || m.observedState === 'stopping' ? 'warning' : 'neutral');

/**
 * Storage: remote sources attached to the server. rclone mounts and WebDAV
 * shares are the same kind of thing to the owner ("a remote source"), each
 * with one Add dialog and one card style.
 */
export function RemoteSourcesPanel() {
  const {api} = useSession();
  const scope = useViewerScope();
  // CD-09: one operation ID per logical command, reused across its retries; released once the
  // outcome is definite (only an ambiguous outcome keeps it, for the receipt recovery below).
  // Declared before the services, whose factories read them during the first render.
  const [mountIds] = useState(stableStorageOperationId);
  const [davIds] = useState(stableStorageOperationId);
  const mounts = useService<StorageManagementService, StorageManagementSnapshot>(() => new StorageManagementService({api, scope, operationId: mountIds.next}), [api, scope]);
  const dav = useService<RemoteSourceService, ReturnType<RemoteSourceService['getSnapshot']>>(() => new RemoteSourceService(api, scope, davIds.next), [api, scope]);
  useEffect(() => {
    void mounts.service.load().catch(() => {});
    void dav.service.load().catch(() => {});
  }, [mounts.service, dav.service]);
  const [add, setAdd] = useState<'rclone' | 'webdav' | null>(null);
  const [editDav, setEditDav] = useState<RemoteSource | null>(null);
  const [remove, setRemove] = useState<{kind: 'mount'; mount: StorageMount} | {kind: 'webdav'; source: RemoteSource} | null>(null);
  const action = useAction();
  const runMount = (work: () => Promise<unknown>, done?: string) => action.run(async () => {
    try {
      await work();
    } finally {
      const phase = mounts.service.getSnapshot().mutation.phase;
      if (phase !== 'ambiguous' && phase !== 'pending' && phase !== 'preparing' && phase !== 'recovering') mountIds.release();
    }
    const mutation = mounts.service.getSnapshot().mutation;
    // Q1: the mutation error carries a stable code; the presenter maps it and
    // the raw message never reaches the screen.
    if (!mountCommandSucceeded(mutation)) throw Object.assign(new Error('request_failed'), {code: mutation.error?.code ?? 'request_failed'});
  }, done);
  const runDav = (work: () => Promise<unknown>, done?: string) => action.run(async () => {
    try {
      await work();
    } finally {
      if (!dav.service.getSnapshot().ambiguous) davIds.release();
    }
    const state = dav.service.getSnapshot();
    if (!davCommandSucceeded(state)) throw new Error(state.error || 'request_failed');
  }, done);
  const m = mounts.snapshot;
  const d = dav.snapshot;
  const t = currentI18n().t;
  const list = m.data?.mounts ?? [];
  const davs = d.sources.filter(x => x.kind === 'webdav');
  return (
    <>
      <SectionHeader title={t('settings.server.remoteSources')} lede={t('web.console.nav.storageLede')} actions={<Menu label={t('web.storage.addSource')} trigger={<Button variant="primary" icon="plus" label={t('web.storage.addSource')} iconAfter="chevronDown" />} items={[{id: 'webdav', label: t('web.storage.webdavShare'), icon: 'globe', meta: t('web.storage.webdavMeta')}, {id: 'rclone', label: t('web.storage.rcloneRemote'), icon: 'storage', meta: t('web.storage.rcloneMeta'), disabled: m.data ? false : undefined}]} onSelect={id => setAdd(id as 'rclone' | 'webdav')} />} />
      {m.error ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: () => void mounts.service.load()}}>{problem(m.error)}</Notice> : null}
      {d.error ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: () => void dav.service.load()}}>{d.error}</Notice> : null}
      {m.mutation.phase === 'ambiguous' ? <Notice tone="warning" action={{label: t('web.storage.checkResult'), onClick: () => void mounts.service.recover().catch(() => {})}}>{t('web.storage.mountAmbiguous')}</Notice> : null}
      {d.ambiguous ? <Notice tone="warning" action={{label: t('web.storage.checkResult'), onClick: () => void dav.service.recover().catch(() => {})}}>{t('web.storage.davAmbiguous')}</Notice> : null}
      {m.mutation.phase === 'error' || m.mutation.phase === 'conflict' ? <Notice tone="error">{m.mutation.error ? problem(m.mutation.error) : null}</Notice> : null}
      {action.error ? <Notice tone="error">{action.error}</Notice> : null}
      <div style={{display: 'flex', flexDirection: 'column', gap: 28}}>
        {(m.phase === 'loading' && !m.data) || (d.loading && !d.sources.length) ? <Loading label={t('web.storage.loading')} /> : null}
        {m.data && !list.length && !davs.length && !d.loading ? <SettingsGroup><SettingsRow label={t('web.storage.empty')} help={t('web.storage.emptyHelp')} /></SettingsGroup> : null}
        {davs.length ? (
          <SettingsGroup title={t('web.storage.webdavShares')}>
            {davs.map(x => <SettingsRow key={x.id} icon="globe" label={x.name} help={<>{x.origin ? <span style={{fontFamily: 'var(--font-mono)', fontSize: 12}}>{x.origin}</span> : null}{x.rootPath ? ` · ${x.rootPath}` : ''}{x.insecureLocal ? ' · Unencrypted (local only)' : ''}{x.credentialPresent ? ' · Password saved' : ''}</>} meta={<Badge tone={x.state === 'ready' || x.state === 'healthy' ? 'healthy' : x.errorCode ? 'danger' : 'neutral'} dot>{sentence(x.errorCode || x.state)}</Badge>} control={<Menu label={x.name} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('web.storage.actions')} />} items={[{id: 'edit', label: t('web.storage.editConnection'), icon: 'edit'}, {id: 'remove', label: t('action.removeShort'), icon: 'trash', destructive: true, separatorBefore: true}]} onSelect={id => (id === 'edit' ? setEditDav(x) : setRemove({kind: 'webdav', source: x}))} />} />)}
          </SettingsGroup>
        ) : null}
        {list.length ? (
          <SettingsGroup title={t('web.storage.rcloneRemotes')} description={m.data ? t('web.storage.rcloneLimits', {active: m.data.limits.active, definitions: m.data.limits.definitions}) : undefined}>
            {list.map(x => {
              const items: MenuItem[] = [];
              if (x.actions.includes('start')) items.push({id: 'start', label: t('web.storage.mount'), icon: 'play'});
              if (x.actions.includes('stop')) items.push({id: 'stop', label: t('web.storage.unmount'), icon: 'stop'});
              if (x.actions.includes('restart')) items.push({id: 'restart', label: t('web.storage.restart'), icon: 'refresh'});
              if (x.actions.includes('delete')) items.push({id: 'delete', label: t('action.removeShort'), icon: 'trash', destructive: true, separatorBefore: true});
              return <SettingsRow key={x.id} icon="storage" label={x.name} help={<><span style={{fontFamily: 'var(--font-mono)', fontSize: 12}}>{x.mountPath}</span>{x.cacheBytes ? ` · cache ${formatBytes(x.cacheBytes)}` : ''}{x.error ? ` · ${x.error}` : ''}{x.removalPending ? ' · Removal pending' : ''}</>} meta={<Badge tone={mountTone(x)} dot live={x.observedState === 'starting'}>{sentence(x.observedState)}</Badge>} control={items.length ? <Menu label={x.name} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('web.storage.actions')} />} items={items} onSelect={id => (id === 'delete' ? setRemove({kind: 'mount', mount: x}) : void runMount(() => mounts.service.act(x.id, id as 'start' | 'stop' | 'restart', x.controlRevision)))} /> : undefined} />;
            })}
          </SettingsGroup>
        ) : null}
        <Text as="p" variant="caption" tone="tertiary">{t('web.storage.libraryFolders')}</Text>
      </div>
      <WebDavDialog open={add === 'webdav' || !!editDav} source={editDav} onClose={() => { setAdd(null); setEditDav(null); }} onSave={(id, input) => void runDav(() => dav.service.configure(id, input), 'Saved.').then(ok => { if (ok) { setAdd(null); setEditDav(null); } })} busy={d.busy || action.busy} />
      <RcloneDialog open={add === 'rclone'} onClose={() => setAdd(null)} onSave={input => void runMount(() => mounts.service.create(input), 'Added.').then(ok => ok && setAdd(null))} busy={action.busy} />
      <ConfirmDialog open={!!remove} onOpenChange={o => !o && setRemove(null)} title={t('web.storage.removeMountTitle', {name: remove?.kind === 'mount' ? remove.mount.name : remove?.source.name ?? ''})} body={remove?.kind === 'mount' ? t('web.storage.removeMountBody') : t('web.storage.removeSourceBody')} confirmLabel={t('action.removeShort')} busy={action.busy} onConfirm={() => { if (!remove) return; void (remove.kind === 'mount' ? runMount(() => mounts.service.remove(remove.mount.id, remove.mount.controlRevision)) : runDav(() => dav.service.remove(remove.source))).then(ok => ok && setRemove(null)); }} />
    </>
  );
}

function WebDavDialog({open, source, onClose, onSave, busy}: {open: boolean; source: RemoteSource | null; onClose: () => void; onSave: (id: string | null, input: DAVConfiguration) => void; busy: boolean}) {
  const t = currentI18n().t;
  const [draft, setDraft] = useState<WebDavDraft>({name: '', root: '', username: '', password: '', insecureLocal: false});
  useEffect(() => {
    // CD-09: the address starts empty on edit too — it is never reconstructed from the server's
    // private rootPath. Empty means "keep the current connection"; typing a new address replaces it.
    if (open) setDraft({name: source?.name ?? '', root: '', username: '', password: '', insecureLocal: source?.insecureLocal ?? false});
  }, [open, source]);
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={source ? t('web.storage.editDav') : t('web.storage.addDav')} description={t('web.storage.davPasswordKept')} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={source ? t('action.save') : t('web.storage.addShare')} loading={busy} disabled={!draft.name.trim() || (!source && !draft.root.trim())} onClick={() => onSave(source?.id ?? null, davConfiguration(source, draft))} /></>}>
      <Input label={t('profile.name')} value={draft.name} onChange={e => setDraft({...draft, name: e.target.value})} maxLength={100} autoFocus />
      <Input label={t('web.claim.address')} value={draft.root} onChange={e => setDraft({...draft, root: e.target.value})} placeholder="https://cloud.example.com/remote.php/dav/files/me/Media" mono autoCapitalize="off" spellCheck={false} help={source && !draft.root.trim() && source.origin ? currentI18n().t('web.storage.webdavKeepConnection', {origin: source.origin}) : undefined} />
      <Input label={t('web.direct.username')} optional value={draft.username} onChange={e => setDraft({...draft, username: e.target.value})} autoCapitalize="off" spellCheck={false} />
      <PasswordInput label={t('web.account.password')} optional value={draft.password} onChange={e => setDraft({...draft, password: e.target.value})} help={source?.credentialPresent ? t('web.storage.keepSavedPassword') : undefined} />
      <Checkbox checked={draft.insecureLocal} onCheckedChange={v => setDraft({...draft, insecureLocal: v})} label={t('web.storage.insecureLocal')} help={t('web.storage.insecureLocalHelp')} />
    </Dialog>
  );
}

function RcloneDialog({open, onClose, onSave, busy}: {open: boolean; onClose: () => void; onSave: (input: {name: string; executable: string; remote: string; config: string}) => void; busy: boolean}) {
  const t = currentI18n().t;
  const [draft, setDraft] = useState({name: '', executable: '/opt/homebrew/bin/rclone', remote: '', config: ''});
  useEffect(() => {
    if (open) setDraft(d => ({...d, name: '', remote: '', config: ''}));
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={t('web.storage.addRclone')} description={t('web.storage.rcloneLede')} width={600} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={t('web.storage.addRemote')} loading={busy} disabled={!draft.name.trim() || !draft.remote.trim() || !draft.config.trim()} onClick={() => onSave(draft)} /></>}>
      <Input label={t('profile.name')} value={draft.name} onChange={e => setDraft({...draft, name: e.target.value})} maxLength={100} autoFocus />
      <Input label={t('web.storage.rcloneBinary')} value={draft.executable} onChange={e => setDraft({...draft, executable: e.target.value})} mono />
      <Input label={t('web.storage.rcloneRemoteName')} value={draft.remote} onChange={e => setDraft({...draft, remote: e.target.value})} placeholder="drive:Media" mono help={t('web.storage.rcloneRemoteHelp')} />
      <TextArea label={t('web.storage.rcloneConfig')} value={draft.config} onChange={e => setDraft({...draft, config: e.target.value})} placeholder={t('web.storage.rcloneConfigExample')} rows={6} style={{fontFamily: 'var(--font-mono)', fontSize: 12}} help={t('web.storage.rcloneConfigHelp')} />
    </Dialog>
  );
}
