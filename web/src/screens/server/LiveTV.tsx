import {importSkipped, liveSourceState} from '@core/server-admin/live-words.ts';
import {rebaseDraft, isStaleAnswer} from '@core/freshness-actions.ts';
import {useI18n} from '../../app/i18n';
import {useEffect, useState} from 'react';
import {lanConfirmationRoots, previewRemoteSource, remoteSourceDraft, withConfirmedLanRoots, type RemoteSourceDraft, type RemoteSourcePreview, type RefreshStatus, type SourceKind} from '@core/linear-api.ts';
import {channelRequestID, newLiveSourceInput, type LiveSourceInput} from '@core/live-source-draft.ts';
import type {RecordingStorage} from '@core/dvr.ts';
import {canRefreshSource, normalizeHdHomerunLocator} from '@core/server-admin/panel-logic.ts';
export {canRefreshSource, normalizeHdHomerunLocator} from '@core/server-admin/panel-logic.ts';
import {problem, sendFresh, useAction, useRead} from '../../admin/console';
import {useInlineForm} from '../settings/ServerForms';
import {createOperationIds} from './operation-ids';
import {usePollWhile} from '../../admin/poll-while';
import {useSession} from '../../app/session';
import {Badge, Button, ConfirmDialog, Dialog, Freshness, Input, KeyValue, Menu, Notice, PasswordInput, Segmented, Select, SettingsEmpty, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, type MenuItem} from '../../ui';
import {ChannelMapDialog, DiscoverTunersDialog, RecordingRulesGroup, SourceSettingsDialog} from './LiveSourceExtras';
import {dependsOn} from '@core/freshness-hub.ts';

const SOURCE_MOVES = [dependsOn.list('liveSources')];
const RECORDING_SPACE_MOVES = [dependsOn.list('recordingStorage')];

type LiveSource = {id: string; name: string; revision: number; state: string; tunerCount: number; tunerCountMode: string; generation: string; publishedAt: string; channels: number; programmes: number; deliveryValidation: string; capacityKnown: boolean; planningEstimate: number};
type SourceList = {sources: LiveSource[]; refresh: RefreshStatus[]};
/**
 * Live TV & DVR administration: sources feeding the guide (tuners,
 * playlists) and the space reserved for recordings. One Add flow with a
 * kind chooser; every row has Refresh, Pause and Remove in one menu.
 */
export function LiveSourcesPanel() {
  const {api} = useSession();
  const i18n = useI18n();
  const {t} = i18n;
  // A scheduled guide refresh, a source that starts failing, or one added from another device shows by itself.
  const read = useRead<SourceList>(() => api.request<SourceList>('/v1/admin/live-sources'), [api], {on: SOURCE_MOVES});
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
  // The source as the server has it now: read before a switch, a refresh or a removal is sent again.
  const again = (id: string) => async () => (await api.request<SourceList>('/v1/admin/live-sources')).sources.find(x => x.id === id);
  const path = (src: LiveSource, what: string) => `/v1/admin/live-sources/${encodeURIComponent(src.id)}/${what}`;
  const refresh = (src: LiveSource) => void action.run(() => sendFresh(src, x => api.request(path(x, 'refresh'), 'POST', {expectedRevision: x.revision}), again(src.id), now => now.state !== 'disabled'), t('web.liveTv.refreshStarted')).then(ok => ok && read.reload());
  const toggle = (src: LiveSource) => {
    const enabled = src.state === 'disabled';
    void action.run(async () => {
      await sendFresh(src, x => api.request(path(x, 'enabled'), 'POST', {expectedRevision: x.revision, enabled, requestId: opIds.forPayload(JSON.stringify({id: x.id, revision: x.revision, enabled}))}), again(src.id), now => ((now.state !== 'disabled') === enabled ? 'done' : true));
      opIds.release();
    }).then(ok => ok && read.reload());
  };
  const removeSource = (src: LiveSource) => void action.run(async () => {
    await sendFresh(src, x => api.request(path(x, 'remove'), 'POST', {expectedRevision: x.revision, requestId: opIds.forPayload(JSON.stringify({id: x.id, revision: x.revision}))}), again(src.id));
    opIds.release();
  }).then(ok => { if (ok) { setRemove(null); read.reload(); } });
  return (
    <>
      <SettingsGroup title={t('web.liveTv.sources')} action={<div style={{display: 'flex', gap: 12, alignItems: 'center'}}><Freshness at={read.at} onRefresh={read.reload} refreshing={read.loading} /><Menu label={t('web.liveTv.addSource')} trigger={<Button size="sm" variant="secondary" icon="plus" label={t('web.liveTv.addSource')} iconAfter="chevronDown" />} items={[{id: 'remote', label: t('web.liveTv.remoteSource'), icon: 'live', meta: t('web.liveTv.remoteSourceMeta')}, {id: 'discover', label: t('web.liveTv.discover'), icon: 'search', meta: t('web.liveTv.discoverMeta')}, {id: 'upload', label: t('web.liveTv.upload'), icon: 'upload', meta: t('web.liveTv.uploadMeta')}]} onSelect={id => { action.clear(); if (id === 'discover') setDiscover(true); else { setFound(undefined); setAdd(id as 'remote' | 'upload'); } }} /></div>}>
        {read.error ? <SettingsNote><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
        {action.error && !remove ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
        {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
        {!read.data && read.loading ? <SettingsLoading thing={t('server.thing.liveSources')} /> : null}
        {read.data && !sources.length ? <SettingsEmpty title={t('live.noSources')} body={t('web.liveTv.noSourcesHelp')} /> : null}
        {sources.map(src => <ImportSkippedNotice key={'skipped:' + src.id} name={src.name} diagnostics={(src as {diagnostics?: unknown}).diagnostics} />)}
        {sources.map(src => {
          const r = status(src.id);
          const state = liveSourceState(src, r);
          const refreshable = canRefreshSource(src, read.data?.refresh);
          const items: MenuItem[] = [{id: 'settings', label: t('web.liveTv.sourceSettings'), icon: 'settings'}, {id: 'channels', label: t('web.liveTv.channelsAndNumbers'), icon: 'list'}, ...(refreshable ? [{id: 'refresh', label: t('web.liveTv.refreshGuideNow'), icon: 'refresh', separatorBefore: true} as MenuItem] : []), {id: 'toggle', label: src.state === 'disabled' ? t('web.liveTv.enableSource') : t('web.liveTv.pauseSource'), icon: src.state === 'disabled' ? 'play' : 'pause'}, {id: 'remove', label: t('web.liveTv.removeSource'), icon: 'trash', destructive: true, separatorBefore: true}];
          return <SettingsRow key={src.id} icon="live" label={src.name} help={[t('server.live.channelCount', {count: src.channels}), t('server.live.programCount', {count: src.programmes}), src.tunerCount ? t(src.tunerCountMode === 'discovered' ? 'web.liveTv.tunersDetected' : 'server.live.tunerCount', {count: src.tunerCount}) : '', src.publishedAt ? t('server.live.guideUpdated', {when: i18n.dateTime(src.publishedAt)}) : '', r?.errorCode ? t('web.console.refreshFailed') : ''].filter(Boolean).join(' · ')} meta={<Badge tone={state.tone} dot live={state.live}>{t(state.id)}</Badge>} control={<Menu label={src.name} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('server.row.actions', {name: src.name})} />} items={items} onSelect={id => {
            action.clear();
            if (id === 'settings') setConfigure(src);
            else if (id === 'channels') setMapping(src);
            else if (id === 'refresh') { if (canRefreshSource(src, read.data?.refresh)) refresh(src); }
            else if (id === 'toggle') toggle(src);
            else setRemove(src);
          }} />} />;
        })}
      </SettingsGroup>
      <RemoteSourceDialog open={add === 'remote'} tuner={found} onClose={() => setAdd(null)} onSaved={() => { setAdd(null); read.reload(); }} />
      {configure ? <SourceSettingsDialog src={configure} guides={sources} onClose={() => setConfigure(null)} onChanged={read.reload} /> : null}
      {mapping ? <ChannelMapDialog src={mapping} onClose={() => setMapping(null)} /> : null}
      {discover ? <DiscoverTunersDialog onClose={() => setDiscover(false)} onUse={t => { setDiscover(false); setFound(t.baseUrl || t.address); setAdd('remote'); }} /> : null}
      <UploadSourceDialog open={add === 'upload'} onClose={() => setAdd(null)} onSaved={() => { setAdd(null); read.reload(); }} />
      <ConfirmDialog open={!!remove} onOpenChange={o => !o && setRemove(null)} title={t('web.liveTv.removeTitle', {name: remove?.name ?? ''})} body={t('web.liveTv.removeBody')} confirmLabel={t('web.liveTv.removeSource')} busy={action.busy} error={action.error || undefined} onConfirm={() => { if (remove) removeSource(remove); }} />
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
      setDraft({...remoteSourceDraft(channelRequestID()), viewerAccess: 'server-members', ...(tuner ? {kind: 'hdhomerun' as SourceKind, locator: tuner, name: i18n.t('server.live.kind.hdhomerun'), useDiscoveredCapacity: true} : {})});
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
      <Segmented label={i18n.t('web.libraries.kind')} options={(['m3u', 'xtream', 'hdhomerun'] as SourceKind[]).map(k => ({id: k, label: i18n.t(`server.live.kind.${k}`)}))} value={draft.kind} onChange={kind => { setDraft({...draft, kind}); setPreview(undefined); }} />
      <Input label={i18n.t('profile.name')} value={draft.name} onChange={e => setDraft({...draft, name: e.target.value})} maxLength={100} autoFocus />
      <Input label={draft.kind === 'hdhomerun' ? i18n.t('web.liveTv.tunerAddress') : draft.kind === 'xtream' ? i18n.t('web.liveTv.serverAddress') : i18n.t('web.liveTv.playlistUrl')} value={draft.locator} onChange={e => { setDraft({...draft, locator: e.target.value}); setPreview(undefined); }} placeholder={draft.kind === 'hdhomerun' ? '192.168.1.50' : draft.kind === 'xtream' ? 'http://provider.example.com:8080' : 'https://…/playlist.m3u'} mono autoCapitalize="off" spellCheck={false} /> {/* lint-strings-allow: address format examples, not copy */}
      <Input label={i18n.t('web.liveTv.guideUrl')} optional help={i18n.t(draft.kind === 'hdhomerun' ? 'server.live.guideUrlTunerHelp' : 'server.live.guideUrlHelp')} value={draft.guideUrl} onChange={e => { setDraft({...draft, guideUrl: e.target.value}); setPreview(undefined); }} mono autoCapitalize="off" spellCheck={false} />
      {draft.kind === 'xtream' ? <><Input label={i18n.t('web.direct.username')} value={draft.username} onChange={e => setDraft({...draft, username: e.target.value})} autoCapitalize="off" spellCheck={false} /><PasswordInput label={i18n.t('web.account.password')} value={draft.password} onChange={e => setDraft({...draft, password: e.target.value})} /></> : null}
      <Select label={i18n.t('web.liveTv.whoCanWatch')} value={draft.viewerAccess} options={[{value: 'server-members', label: i18n.t('web.liveTv.everyoneOnServer')}, {value: 'owner-only', label: i18n.t('web.together.onlyMe')}]} onChange={e => setDraft({...draft, viewerAccess: e.target.value as RemoteSourceDraft['viewerAccess']})} />
      <Select label={i18n.t('web.liveTv.refreshGuide')} value={String(draft.refreshSeconds)} options={[{value: '3600', label: i18n.t('web.libraries.everyHour')}, {value: '21600', label: i18n.t('web.libraries.every6h')}, {value: '43200', label: i18n.t('web.liveTv.every12h')}, {value: '86400', label: i18n.t('web.libraries.daily')}]} onChange={e => setDraft({...draft, refreshSeconds: Number(e.target.value)})} />
      {preview ? <Notice tone="success" title={[i18n.t('server.live.channelCount', {count: preview.preview.channels}), i18n.t('server.live.programCount', {count: preview.preview.programmes})].join(' · ')}>{preview.preview.names.slice(0, 6).join(', ')}{preview.preview.names.length > 6 ? '…' : ''}{preview.discoveredCapacity ? ` · ${i18n.t('web.liveTv.capacityTuners', {count: preview.discoveredCapacity})}` : ''}</Notice> : null}
      {preview ? <ImportSkippedNotice diagnostics={preview.preview.diagnostics} /> : null}
      {confirm ? (
        <Notice tone="warning" title={i18n.t('web.liveTv.lanTitle')}>
          <span>{i18n.t('web.liveTv.lanBody')}</span>
          <ul style={{margin: '8px 0 0', paddingLeft: 20, fontFamily: 'var(--font-mono)', fontSize: 12.5}}>{confirm.map(root => <li key={root}>{root}</li>)}</ul>
        </Notice>
      ) : null}
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
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
      {preview ? <Notice tone="success" title={[t('server.live.channelCount', {count: preview.channels}), t('server.live.programCount', {count: preview.programmes})].join(' · ')}>{preview.names?.slice(0, 6).join(', ')}</Notice> : null}
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
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
  const i18n = useI18n();
  const {t} = i18n;
  // What recordings use and what is free are measured by the server as it records: followed quietly.
  const readStorage = () => api.request<{result?: RecordingStorage} & Partial<RecordingStorage>>('/v1/dvr/storage');
  const read = useRead(readStorage, [api], {on: RECORDING_SPACE_MOVES});
  // BE-API-10: one ID per logical save; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const data = (read.data?.result ?? read.data) as RecordingStorage | undefined;
  const [draft, setDraft] = useState<RecordingStorage['policy']>();
  // What the draft was last compared with: a newer policy is taken under the owner's edits, never over them.
  const [basis, setBasis] = useState<RecordingStorage['policy']>();
  const policyKey = data?.policy ? JSON.stringify(data.policy) : '';
  useEffect(() => {
    if (!data?.policy) return;
    const next = data.policy;
    setDraft(current => (current && basis ? rebaseDraft(basis, current, next).merged : next));
    setBasis(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [policyKey]);
  // The policy is this panel's own draft; it is saved by the page's one Save.
  const dirty = !!basis && !!draft && JSON.stringify({...draft, revision: 0}) !== JSON.stringify({...basis, revision: 0});
  const inline = useInlineForm('recording-storage', {
    dirty,
    discard: () => setDraft(basis),
    save: async () => {
      if (!basis || !draft) return;
      const put = (policy: RecordingStorage['policy']) => api.request('/v1/dvr/storage', 'PUT', {requestId: opIds.forPayload(JSON.stringify({revision: policy.revision, policy})), expectedRevision: policy.revision, policy});
      try { await put({...draft, revision: basis.revision}); }
      catch (e) {
        if (!isStaleAnswer(e)) throw e;
        // The policy moved on the server: carry these edits onto it and save again, unless the same limit was changed there too.
        const raw = await readStorage().catch(() => { throw e; });
        const fresh = ((raw.result ?? raw) as RecordingStorage).policy;
        const rebased = rebaseDraft({...basis, revision: 0}, {...draft, revision: 0}, {...fresh, revision: 0});
        setBasis(fresh); setDraft({...rebased.merged, revision: fresh.revision});
        if (rebased.conflict) throw e;
        await put({...rebased.merged, revision: fresh.revision});
      }
      opIds.release(); read.reload();
    },
  });
  if (read.error && !data) return <SettingsGroup title={t('channels.recordings')}><SettingsNote><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote></SettingsGroup>;
  if (!data || !draft) return null;
  const gb = (b: number) => Math.round(b / 1e9);
  return (
    <SettingsGroup title={t('channels.recordings')} description={t('web.liveTv.recordingsLede')} action={<Freshness at={data.measurement?.measuredAt} onRefresh={read.reload} refreshing={read.loading} verb="Measured" />}>
      <SettingsRow label={t('web.liveTv.space')} stack control={<KeyValue rows={[[t('server.live.usedByRecordings'), i18n.bytes(data.measurement.usedBytes)], [t('server.live.freeOnVolume'), i18n.bytes(data.measurement.freeBytes)], [t('web.liveTv.space.reserved'), i18n.bytes(data.measurement.reservedBytes)], [t('web.liveTv.space.waitingDelete'), i18n.bytes(data.pendingDeleteBytes)], [t('web.liveTv.space.forecast'), data.forecastDescription || t('web.liveTv.space.forecastOver', {size: i18n.bytes(data.forecastBytes), hours: data.forecastHours})]]} />} start />
      {!data.measurement.writeHealthy ? <SettingsNote><Notice tone="warning" compact>{t('web.liveTv.notWritable')}</Notice></SettingsNote> : null}
      <SettingsRow label={t('web.liveTv.keepRecordings')} help={t('web.liveTv.keepRecordingsHelp')} control={<Input hideLabel label={t('web.liveTv.keepRecordings')} type="number" min={0} max={3650} inputMode="numeric" value={draft.retentionDays} onChange={e => setDraft({...draft, retentionDays: Number(e.target.value) || 0})} style={{width: 110}} />} meta={t('settings.unit.days')} />
      <SettingsRow label={t('web.liveTv.episodesPerSeries')} help={t('web.liveTv.episodesPerSeriesHelp')} control={<Input hideLabel label={t('web.liveTv.episodesPerSeriesLabel')} type="number" min={0} max={100000} inputMode="numeric" value={draft.episodeLimit} onChange={e => setDraft({...draft, episodeLimit: Number(e.target.value) || 0})} style={{width: 110}} />} />
      <SettingsRow label={t('web.liveTv.leaveFree')} help={t('web.liveTv.leaveFreeHelp')} control={<Input hideLabel label={t('web.liveTv.leaveFree')} type="number" min={0} inputMode="numeric" value={gb(draft.floorBytes)} onChange={e => setDraft({...draft, floorBytes: (Number(e.target.value) || 0) * 1e9})} style={{width: 110}} />} meta={t('web.liveTv.gbUnit')} />
      <SettingsRow label={t('web.liveTv.capUpTo')} help={t('web.liveTv.capUpToHelp')} control={<Input hideLabel label={t('web.transcoding.capLabel')} type="number" min={0} inputMode="numeric" value={gb(draft.capBytes)} onChange={e => setDraft({...draft, capBytes: (Number(e.target.value) || 0) * 1e9})} style={{width: 110}} />} meta={t('web.liveTv.gbUnit')} />
      {inline.error ? <SettingsNote><Notice tone="error" compact>{problem(inline.error, 'action')}</Notice></SettingsNote> : null}
    </SettingsGroup>
  );
}

/** What a guide or playlist import left out and why; nothing when it left nothing out. The rest of the import is in use either way. */
function ImportSkippedNotice({name, diagnostics}: {name?: string; diagnostics: unknown}) {
  const i18n = useI18n();
  const skipped = importSkipped(i18n.t, id => i18n.has(id), diagnostics);
  if (!skipped) return null;
  const notice = <Notice tone="warning" compact title={name}>{[skipped.summary, ...skipped.reasons].join(' ')}</Notice>;
  return name ? <SettingsNote>{notice}</SettingsNote> : notice;
}
