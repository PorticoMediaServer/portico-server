import {currentI18n, useI18n} from '../../app/i18n';
import {useEffect, useState} from 'react';
import {lanConfirmationRoots, linearError, previewRemoteSource, remoteSourceDraft, withConfirmedLanRoots, type RemoteSourceDraft, type RemoteSourcePreview, type RefreshStatus, type SourceKind} from '@core/linear-api.ts';
import {channelRequestID, newLiveSourceInput, type LiveSourceInput} from '@core/live-source-draft.ts';
import type {RecordingStorage} from '@core/dvr.ts';
import {canRefreshSource, normalizeHdHomerunLocator} from '@core/server-admin/panel-logic.ts';
export {canRefreshSource, normalizeHdHomerunLocator} from '@core/server-admin/panel-logic.ts';
import {problem, sentence, useAction, useRead} from '../../admin/console';
import {useInlineForm} from '../settings/ServerForms';
import {createOperationIds} from './operation-ids';
import {usePollWhile} from '../../admin/poll-while';
import {formatBytes} from '@core/presentation/index.ts';
import {useSession} from '../../app/session';
import {Badge, Button, ConfirmDialog, Dialog, Freshness, Input, KeyValue, Menu, Notice, PasswordInput, Segmented, Select, SettingsGroup, SettingsRow, StateView, Text, TextArea, type MenuItem} from '../../ui';
import {ChannelMapDialog, DiscoverTunersDialog, RecordingRulesGroup, SourceSettingsDialog} from './LiveSourceExtras';

type LiveSource = {id: string; name: string; revision: number; state: string; tunerCount: number; tunerCountMode: string; generation: string; publishedAt: string; channels: number; programmes: number; deliveryValidation: string; capacityKnown: boolean; planningEstimate: number};
type SourceList = {sources: LiveSource[]; refresh: RefreshStatus[]};
const kindLabel: Record<SourceKind, string> = {m3u: 'M3U playlist URL', xtream: 'Xtream Codes', hdhomerun: 'HDHomeRun'};
/**
 * Live TV & DVR administration: sources feeding the guide (tuners,
 * playlists) and the space reserved for recordings. One Add flow with a
 * kind chooser; every row has Refresh, Pause and Remove in one menu.
 */
export function LiveSourcesPanel() {
  const {api} = useSession();
  const {t} = useI18n();
  const read = useRead<SourceList>(() => api.request<SourceList>('/v1/admin/live-sources'), [api]);
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const [add, setAdd] = useState<'remote' | 'upload' | null>(null);
  const [remove, setRemove] = useState<LiveSource | null>(null);
  const [configure, setConfigure] = useState<LiveSource | null>(null);
  const [mapping, setMapping] = useState<LiveSource | null>(null);
  const [discover, setDiscover] = useState(false);
  const [found, setFound] = useState<string>();
  const sources = read.data?.sources ?? [];
  const status = (id: string) => read.data?.refresh.find(r => r.sourceId === id);
  // While a source refreshes, check again with backoff (2 → 30 s) until it settles; paused while hidden.
  usePollWhile(!!read.data?.refresh.some(r => r.state === 'refreshing'), read.reload);
  const tone = (src: LiveSource): 'healthy' | 'warning' | 'danger' | 'neutral' => {
    const r = status(src.id);
    if (src.state === 'disabled') return 'neutral';
    if (r?.state === 'credentials-required' || r?.state === 'degraded' || src.state === 'failed') return 'danger';
    if (r?.state === 'refreshing') return 'warning';
    return 'healthy';
  };
  return (
    <>
      {read.error && !read.data ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice> : null}
      {action.error ? <Notice tone="error">{action.error}</Notice> : null}
        <SettingsGroup title={t('web.liveTv.sources')} action={<div style={{display: 'flex', gap: 12, alignItems: 'center'}}><Freshness at={read.at} onRefresh={read.reload} refreshing={read.loading} />{<Menu label={t('web.liveTv.addSource')} trigger={<Button variant="primary" icon="plus" label={t('web.liveTv.addSource')} iconAfter="chevronDown" />} items={[{id: 'remote', label: t('web.liveTv.remoteSource'), icon: 'live', meta: t('web.liveTv.remoteSourceMeta')}, {id: 'discover', label: t('web.liveTv.discover'), icon: 'search', meta: t('web.liveTv.discoverMeta')}, {id: 'upload', label: t('web.liveTv.upload'), icon: 'upload', meta: t('web.liveTv.uploadMeta')}]} onSelect={id => { if (id === 'discover') setDiscover(true); else { setFound(undefined); setAdd(id as 'remote' | 'upload'); } }} />}</div>}>
          {read.data && !sources.length ? <SettingsRow label={t('live.noSources')} help={t('web.liveTv.noSourcesHelp')} /> : null}
          {sources.map(src => {
            const r = status(src.id);
            const refreshable = canRefreshSource(src, read.data?.refresh);
            const items: MenuItem[] = [{id: 'settings', label: t('web.liveTv.sourceSettings'), icon: 'settings'}, {id: 'channels', label: t('web.liveTv.channelsAndNumbers'), icon: 'list'}, ...(refreshable ? [{id: 'refresh', label: t('web.liveTv.refreshGuideNow'), icon: 'refresh', separatorBefore: true} as MenuItem] : []), {id: 'toggle', label: src.state === 'disabled' ? t('web.liveTv.enableSource') : t('web.liveTv.pauseSource'), icon: src.state === 'disabled' ? 'play' : 'pause'}, {id: 'remove', label: t('web.liveTv.removeSource'), icon: 'trash', destructive: true, separatorBefore: true}];
            return <SettingsRow key={src.id} icon="live" label={src.name} help={`${src.channels} channels · ${src.programmes.toLocaleString()} programs${src.tunerCount ? ` · ${src.tunerCount} tuners${src.tunerCountMode === 'discovered' ? ' (detected)' : ''}` : ''}${src.publishedAt ? ` · guide updated ${new Date(src.publishedAt).toLocaleString()}` : ''}${r?.errorCode ? ` · ${currentI18n().t('web.console.refreshFailed')}` : ''}`} meta={<Badge tone={tone(src)} dot live={r?.state === 'refreshing'}>{src.state === 'disabled' ? t('web.liveTv.pausedBadge') : r ? sentence(r.state) : sentence(src.state)}</Badge>} control={<Menu label={src.name} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('web.storage.actions')} />} items={items} onSelect={id => {
              if (id === 'settings') setConfigure(src);
              else if (id === 'channels') setMapping(src);
              else if (id === 'refresh') { if (!canRefreshSource(src, read.data?.refresh)) return; void action.run(() => api.request(`/v1/admin/live-sources/${encodeURIComponent(src.id)}/refresh`, 'POST', {expectedRevision: src.revision}), t('web.liveTv.refreshStarted')).then(ok => ok && read.reload()); }
              else if (id === 'toggle') void action.run(async () => { const payload = {expectedRevision: src.revision, enabled: src.state === 'disabled'}; const key = JSON.stringify({id: src.id, ...payload}); await api.request(`/v1/admin/live-sources/${encodeURIComponent(src.id)}/enabled`, 'POST', {...payload, requestId: opIds.forPayload(key)}); opIds.release(); }).then(ok => ok && read.reload());
              else setRemove(src);
            }} />} />;
          })}
        </SettingsGroup>
      <RemoteSourceDialog open={add === 'remote'} tuner={found} onClose={() => setAdd(null)} onSaved={() => { setAdd(null); read.reload(); }} />
      {configure ? <SourceSettingsDialog src={configure} guides={sources} onClose={() => setConfigure(null)} onChanged={read.reload} /> : null}
      {mapping ? <ChannelMapDialog src={mapping} onClose={() => setMapping(null)} /> : null}
      {discover ? <DiscoverTunersDialog onClose={() => setDiscover(false)} onUse={t => { setDiscover(false); setFound(t.baseUrl || t.address); setAdd('remote'); }} /> : null}
      <UploadSourceDialog open={add === 'upload'} onClose={() => setAdd(null)} onSaved={() => { setAdd(null); read.reload(); }} />
      <ConfirmDialog open={!!remove} onOpenChange={o => !o && setRemove(null)} title={t('web.liveTv.removeTitle', {name: remove?.name ?? ''})} body={t('web.liveTv.removeBody')} confirmLabel={t('web.liveTv.removeSource')} busy={action.busy} onConfirm={() => { if (remove) { const payload = {expectedRevision: remove.revision}; const key = JSON.stringify({id: remove.id, ...payload}); void action.run(async () => { await api.request(`/v1/admin/live-sources/${encodeURIComponent(remove.id)}/remove`, 'POST', {...payload, requestId: opIds.forPayload(key)}); opIds.release(); }).then(ok => { if (ok) { setRemove(null); read.reload(); } }); } }} />
    </>
  );
}

function RemoteSourceDialog({open, tuner, onClose, onSaved}: {open: boolean; tuner?: string; onClose: () => void; onSaved: () => void}) {
  const {api, session} = useSession();
  const i18n = useI18n();
  const serverId = session?.viewer.serverId ?? '';
  const action = useAction();
  // BE-API-10: one ID per logical save; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const [draft, setDraft] = useState<RemoteSourceDraft>(() => ({...remoteSourceDraft(channelRequestID()), viewerAccess: 'server-members'}));
  const [preview, setPreview] = useState<RemoteSourcePreview>();
  // CD-06: local-network addresses (the tuner, the guide, every stream) the owner must confirm.
  const [confirm, setConfirm] = useState<readonly string[]>();
  useEffect(() => {
    if (open) {
      // Arriving from a network search: the tuner's address is already known.
      setDraft({...remoteSourceDraft(channelRequestID()), viewerAccess: 'server-members', ...(tuner ? {kind: 'hdhomerun' as SourceKind, locator: tuner, name: 'HDHomeRun', useDiscoveredCapacity: true} : {})});
      setPreview(undefined);
      setConfirm(undefined);
      action.clear();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  const ready = draft.name.trim() && draft.locator.trim();
  // A preview may ask for the local-network roots it found; the owner confirms them and it runs
  // again with every root confirmed so far (a second device can be reported on the next pass).
  // CD-49: a bare HDHomeRun host/IP is normalized to an explicit http:// URL
  // before preview; unsupported schemes are refused without previewing.
  const check = (next: RemoteSourceDraft) => void action.run(async () => {
    setConfirm(undefined);
    const normalized = next.kind === 'hdhomerun' ? {...next, locator: normalizeHdHomerunLocator(next.locator)} : next;
    if (normalized.locator !== next.locator) setDraft(normalized);
    try { setPreview(await previewRemoteSource(api, serverId, normalized)); }
    catch (e) { const roots = lanConfirmationRoots(e); if (roots) { setConfirm(roots); return; } throw e; }
  });
  const confirmAndCheck = () => { if (!confirm) return; const next = withConfirmedLanRoots(draft, confirm); setDraft(next); check(next); };
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={i18n.t('web.liveTv.addDialog')} description={i18n.t('web.liveTv.addDialogLede')} width={560} actions={<><Button variant="ghost" label={i18n.t('action.cancel')} onClick={onClose} />{preview ? <Button variant="primary" label={i18n.t('web.liveTv.addSource')} loading={action.busy} onClick={() => void action.run(async () => { const key = JSON.stringify({previewId: preview.previewId}); await api.request('/v1/admin/live-sources/remote/save', 'POST', {previewId: preview.previewId, requestId: opIds.forPayload(key)}); opIds.release(); }).then(ok => ok && onSaved())} /> : confirm ? <Button variant="primary" label={i18n.t('web.liveTv.lanConfirm')} loading={action.busy} onClick={confirmAndCheck} /> : <Button variant="primary" label={i18n.t('web.liveTv.checkSource')} loading={action.busy} disabled={!ready} onClick={() => check(draft)} />}</>}>
      <Segmented label={i18n.t('web.libraries.kind')} options={(['m3u', 'xtream', 'hdhomerun'] as SourceKind[]).map(k => ({id: k, label: kindLabel[k]}))} value={draft.kind} onChange={kind => { setDraft({...draft, kind}); setPreview(undefined); }} />
      <Input label={i18n.t('profile.name')} value={draft.name} onChange={e => setDraft({...draft, name: e.target.value})} maxLength={100} autoFocus />
      <Input label={draft.kind === 'hdhomerun' ? i18n.t('web.liveTv.tunerAddress') : draft.kind === 'xtream' ? i18n.t('web.liveTv.serverAddress') : i18n.t('web.liveTv.playlistUrl')} value={draft.locator} onChange={e => { setDraft({...draft, locator: e.target.value}); setPreview(undefined); }} placeholder={draft.kind === 'hdhomerun' ? '192.168.1.50' : draft.kind === 'xtream' ? 'http://provider.example.com:8080' : 'https://…/playlist.m3u'} mono autoCapitalize="off" spellCheck={false} /> {/* lint-strings-allow: address format examples, not copy */}
      {draft.kind !== 'hdhomerun' ? <Input label={i18n.t('web.liveTv.guideUrl')} optional value={draft.guideUrl} onChange={e => setDraft({...draft, guideUrl: e.target.value})} mono autoCapitalize="off" spellCheck={false} /> : null}
      {draft.kind === 'xtream' ? <><Input label={i18n.t('web.direct.username')} value={draft.username} onChange={e => setDraft({...draft, username: e.target.value})} autoCapitalize="off" spellCheck={false} /><PasswordInput label={i18n.t('web.account.password')} value={draft.password} onChange={e => setDraft({...draft, password: e.target.value})} /></> : null}
      <Select label={i18n.t('web.liveTv.whoCanWatch')} value={draft.viewerAccess} options={[{value: 'server-members', label: i18n.t('web.liveTv.everyoneOnServer')}, {value: 'owner-only', label: i18n.t('web.together.onlyMe')}]} onChange={e => setDraft({...draft, viewerAccess: e.target.value as RemoteSourceDraft['viewerAccess']})} />
      <Select label={i18n.t('web.liveTv.refreshGuide')} value={String(draft.refreshSeconds)} options={[{value: '3600', label: i18n.t('web.libraries.everyHour')}, {value: '21600', label: i18n.t('web.libraries.every6h')}, {value: '43200', label: i18n.t('web.liveTv.every12h')}, {value: '86400', label: i18n.t('web.libraries.daily')}]} onChange={e => setDraft({...draft, refreshSeconds: Number(e.target.value)})} />
      {preview ? <Notice tone="success" title={i18n.t('web.liveTv.previewMeta', {channels: preview.preview.channels, programs: preview.preview.programmes.toLocaleString()})}>{preview.preview.names.slice(0, 6).join(', ')}{preview.preview.names.length > 6 ? '…' : ''}{preview.discoveredCapacity ? ` · ${i18n.t('web.liveTv.capacityTuners', {count: preview.discoveredCapacity})}` : ''}</Notice> : null}
      {confirm ? (
        <Notice tone="warning" title={i18n.t('web.liveTv.lanTitle')}>
          <span>{i18n.t('web.liveTv.lanBody')}</span>
          <ul style={{margin: '8px 0 0', paddingLeft: 20, fontFamily: 'var(--font-mono)', fontSize: 12.5}}>{confirm.map(root => <li key={root}>{root}</li>)}</ul>
        </Notice>
      ) : null}
      {action.error ? <Notice tone="error" compact>{linearError(action.error)}</Notice> : null}
    </Dialog>
  );
}

function UploadSourceDialog({open, onClose, onSaved}: {open: boolean; onClose: () => void; onSaved: () => void}) {
  const {api} = useSession();
  const {t} = useI18n();
  const action = useAction();
  const [draft, setDraft] = useState<LiveSourceInput>(newLiveSourceInput);
  const [preview, setPreview] = useState<{channels: number; programmes: number; names?: string[]}>();
  useEffect(() => {
    if (open) {
      setDraft(newLiveSourceInput());
      setPreview(undefined);
      action.clear();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  const readFile = (f: File | undefined, key: 'playlist' | 'guide') => f && f.text().then(text => { setDraft(d => ({...d, [key]: text})); setPreview(undefined); });
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={t('web.liveTv.upload')} description={t('web.liveTv.uploadLede')} width={560} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} />{preview ? <Button variant="primary" label={t('web.liveTv.addSource')} loading={action.busy} onClick={() => void action.run(() => api.request('/v1/admin/live-sources', 'POST', draft)).then(ok => ok && onSaved())} /> : <Button variant="primary" label={t('web.liveTv.checkFiles')} loading={action.busy} disabled={!draft.name.trim() || !draft.playlist} onClick={() => void action.run(async () => { const r = await api.request<{preview: {channels: number; programmes: number; names?: string[]}}>('/v1/admin/live-sources/preview', 'POST', draft); setPreview(r.preview); })} />}</>}>
      <Input label={t('profile.name')} value={draft.name} onChange={e => setDraft({...draft, name: e.target.value})} maxLength={100} autoFocus />
      <Input label={t('web.liveTv.playlistFile')} type="file" accept=".m3u,.m3u8,text/plain" onChange={e => readFile(e.target.files?.[0], 'playlist')} />
      <Input label={t('web.liveTv.guideFile')} optional type="file" accept=".xml,text/xml,application/xml" onChange={e => readFile(e.target.files?.[0], 'guide')} />
      <Input label={t('web.dvrSettings.tuners')} type="number" min={0} max={64} inputMode="numeric" value={draft.tunerCount} onChange={e => setDraft({...draft, tunerCount: Number(e.target.value) || 0})} help={t('web.liveTv.tunersHelp')} style={{width: 120}} />
      {preview ? <Notice tone="success" title={t('web.liveTv.previewMeta', {channels: preview.channels, programs: preview.programmes.toLocaleString()})}>{preview.names?.slice(0, 6).join(', ')}</Notice> : null}
      {action.error ? <Notice tone="error" compact>{linearError(action.error)}</Notice> : null}
    </Dialog>
  );
}

/** Recording › who may record and the standing rules, with the sources a rule can name. */
export function RecordingRulesPanel() {
  const {api} = useSession();
  const read = useRead<SourceList>(() => api.request<SourceList>('/v1/admin/live-sources'), [api]);
  return <RecordingRulesGroup sources={read.data?.sources ?? []} />;
}

/** Recording › space: what recordings use, and the limits that keep the disk from filling. Saved with the page. */
export function RecordingStoragePanel() {
  const {api} = useSession();
  const {t} = useI18n();
  const read = useRead<{result?: RecordingStorage} & Partial<RecordingStorage>>(() => api.request('/v1/dvr/storage'), [api]);
  const action = useAction();
  // BE-API-10: one ID per logical save; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const data = (read.data?.result ?? read.data) as RecordingStorage | undefined;
  const [draft, setDraft] = useState<RecordingStorage['policy']>();
  useEffect(() => {
    if (data?.policy) setDraft(data.policy);
  }, [data?.policy]);
  // The policy is this panel's own draft; it is saved by the page's one Save.
  const dirty = !!data && !!draft && JSON.stringify(draft) !== JSON.stringify(data.policy);
  const inline = useInlineForm('recording-storage', {
    dirty,
    discard: () => setDraft(data?.policy),
    save: async () => { if (!data || !draft) return; const key = JSON.stringify({revision: data.policy.revision, policy: draft}); await api.request('/v1/dvr/storage', 'PUT', {requestId: opIds.forPayload(key), expectedRevision: data.policy.revision, policy: draft}); opIds.release(); read.reload(); },
  });
  if (read.error && !data) return <SettingsGroup title={t('channels.recordings')}><div style={{padding: 12}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div></SettingsGroup>;
  if (!data || !draft) return null;
  const gb = (b: number) => Math.round(b / 1e9);
  return (
    <SettingsGroup title={t('channels.recordings')} description={t('web.liveTv.recordingsLede')} action={<Freshness at={data.measurement?.measuredAt} onRefresh={read.reload} refreshing={read.loading} verb="Measured" />}>
      <SettingsRow label={t('web.liveTv.space')} stack control={<KeyValue rows={[['Used by recordings', formatBytes(data.measurement.usedBytes)], ['Free on the volume', formatBytes(data.measurement.freeBytes)], ['Reserved', formatBytes(data.measurement.reservedBytes)], ['Waiting for deletion', formatBytes(data.pendingDeleteBytes)], ['Forecast', data.forecastDescription || `${formatBytes(data.forecastBytes)} over ${data.forecastHours} hours`]]} />} start />
      {!data.measurement.writeHealthy ? <div style={{padding: '0 16px 12px'}}><Notice tone="warning" compact>{t('web.liveTv.notWritable')}</Notice></div> : null}
      <SettingsRow label={t('web.liveTv.keepRecordings')} help={t('web.liveTv.keepRecordingsHelp')} control={<Input hideLabel label={t('web.liveTv.keepRecordings')} type="number" min={0} max={3650} inputMode="numeric" value={draft.retentionDays} onChange={e => setDraft({...draft, retentionDays: Number(e.target.value) || 0})} style={{width: 110}} />} meta={t('settings.unit.days')} />
      <SettingsRow label={t('web.liveTv.episodesPerSeries')} help={t('web.liveTv.episodesPerSeriesHelp')} control={<Input hideLabel label={t('web.liveTv.episodesPerSeriesLabel')} type="number" min={0} max={100000} inputMode="numeric" value={draft.episodeLimit} onChange={e => setDraft({...draft, episodeLimit: Number(e.target.value) || 0})} style={{width: 110}} />} />
      <SettingsRow label={t('web.liveTv.leaveFree')} help={t('web.liveTv.leaveFreeHelp')} control={<Input hideLabel label={t('web.liveTv.leaveFree')} type="number" min={0} inputMode="numeric" value={gb(draft.floorBytes)} onChange={e => setDraft({...draft, floorBytes: (Number(e.target.value) || 0) * 1e9})} style={{width: 110}} />} meta={t('web.liveTv.gbUnit')} />
      <SettingsRow label={t('web.liveTv.capUpTo')} help={t('web.liveTv.capUpToHelp')} control={<Input hideLabel label={t('web.transcoding.capLabel')} type="number" min={0} inputMode="numeric" value={gb(draft.capBytes)} onChange={e => setDraft({...draft, capBytes: (Number(e.target.value) || 0) * 1e9})} style={{width: 110}} />} meta={t('web.liveTv.gbUnit')} />
      {inline.error ? <div style={{padding: '0 16px 12px'}}><Notice tone="error" compact>{problem(inline.error, 'action')}</Notice></div> : null}
    </SettingsGroup>
  );
}
