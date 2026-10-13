import React, {useEffect, useMemo, useState} from 'react';
import {parseEnvelope} from '@core/administration.ts';
import {parseChannelGuide, type GuideRoute} from '@core/channel-guide.ts';
import {parseRecordingGrants, parseRecordingOwners, recordingAllowed, recordingOwnerKey, recordingOwnerLabel, recordingSeries, recordingGuideRoute, recordingRulePayload, type RecordingOwner, type RecordingAnchor, type RecordingOwnerChoice} from '../../admin/recording-policy';
import {RecordingPermissionsGroup} from './DVRSettings';
import {guideOnlyPatch} from '@core/server-admin/panel-logic.ts';
export {guideOnlyPatch} from '@core/server-admin/panel-logic.ts';
import {QUIET_READ_EVERY_MS, problem, sendFresh, useAction, useRead} from '../../admin/console';
import {isStaleAnswer, rebaseDraft} from '@core/freshness-actions.ts';
import {createOperationIds} from './operation-ids';
import {useSession} from '../../app/session';
import {liveSourceChoicesService, type LiveSourceChoicePage} from '@core/administration.ts';
import {useI18n} from '../../app/i18n';
import {Badge, Button, Checkbox, ConfirmDialog, Dialog, Input, Loading, Menu, Notice, Segmented, Select, SettingsEmpty, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, Switch, Text, type MenuItem} from '../../ui';
import type {MessageId} from '../../app/i18n';

function useServerId() { const {system, session} = useSession(); return system?.id ?? session?.viewer.serverId ?? ''; }
/** These documents come in the shared admin envelope; the page reads fields it names and no others. */
function useAdmin() {
  const {api} = useSession();
  const serverId = useServerId();
  return useMemo(() => ({
    get: async <T,>(path: string) => parseEnvelope(await api.request<unknown>(path), serverId, r => r as T).result,
    send: async <T,>(path: string, method: string, body?: unknown) => parseEnvelope(await api.request<unknown>(path, method, body), serverId, r => r as T).result,
  }), [api, serverId]);
}
const source = (id: string, rest: string) => `/v1/admin/live-sources/${encodeURIComponent(id)}${rest}`;
const list = (text: string) => text.split(',').map(v => v.trim()).filter(Boolean);
/** CD-49: channel-map edits go in sequential batches of at most 200 (the
 * server caps overrides at MaxPageSize). Batches run one at a time with the
 * fresh revision chained forward, each with its own stable operation ID. */
export const channelMapBatchLimit = 200;
export function chunkChannelMapEntries<T>(entries: readonly T[]): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < entries.length; i += channelMapBatchLimit) out.push(entries.slice(i, i + channelMapBatchLimit));
  return out;
}

type FilterRule = {mode: 'include' | 'exclude'; values: string[]};
type SourceSettings = {enabledGroups: readonly string[]; enabledChannels: readonly string[]; kind: string; streamBufferSeconds: number | null; retryWindowSeconds: number | null; userAgent: string; filters: {categories: FilterRule; countries: FilterRule; keywords: FilterRule}; logoImport: {enabled: boolean; overwriteExisting: boolean}; channelNumbering: {mode: 'source' | 'sequential'; startAt: number; step: number}; guideSourceId: string};
type SourceDocument = {sourceId: string; sourceName: string; revision: number; settings: SourceSettings; effective: {streamBufferSeconds: number; retryWindowSeconds: number}; availableKinds: string[]};
const kindName: Record<string, MessageId> = {playlist: 'server.live.sourceKind.playlist', 'xmltv-guide': 'server.live.sourceKind.guide', hdhomerun: 'server.live.sourceKind.tuner'};
const filterName = {categories: 'server.live.filter.categories', countries: 'server.live.filter.countries', keywords: 'server.live.filter.keywords'} as const satisfies Record<string, MessageId>;

/** One source's own settings: what it is, where its guide comes from, which of its channels
 * are wanted, how they are numbered, and whether its logos are brought in. */
export function SourceSettingsDialog({src, guides, onClose, onChanged}: {src: {id: string; name: string}; guides: readonly {id: string; name: string}[]; onClose: () => void; onChanged: () => void}) {
  const admin = useAdmin();
  const {t} = useI18n();
  const read = useRead(() => admin.get<SourceDocument>(source(src.id, '/configuration')), [admin, src.id]);
  const action = useAction();
  const logos = useAction();
  const [draft, setDraft] = useState<SourceSettings>();
  const [logoResult, setLogoResult] = useState('');
  useEffect(() => { if (read.data) setDraft(read.data.settings); }, [read.data]);
  const doc = read.data;
  const dirty = !!draft && !!doc && JSON.stringify(draft) !== JSON.stringify(doc.settings);
  const guideOnly = draft?.kind === 'xmltv-guide';
  const patch = (p: Partial<SourceSettings>) => setDraft(d => (d ? {...d, ...p} : d));
  // BE-API-10: one ID per logical save; retries reuse it, edits mint a new one.
  const [opIds] = useState(createOperationIds);
  const [logoOpIds] = useState(createOperationIds);
  const save = () => void action.run(async () => {
    if (!draft || !doc) return;
    const put = (revision: number, settings: SourceSettings) => admin.send(source(src.id, '/configuration'), 'PUT', {expectedRevision: revision, operationId: opIds.forPayload(JSON.stringify({revision, settings})), settings});
    try { await put(doc.revision, draft); }
    catch (e) {
      if (!isStaleAnswer(e)) throw e;
      // The source's settings moved (a channel was renumbered, or they were saved elsewhere): carry these edits onto them and save again.
      const fresh = await admin.get<SourceDocument>(source(src.id, '/configuration')).catch(() => { throw e; });
      const rebased = rebaseDraft(doc.settings, draft, fresh.settings);
      if (rebased.conflict) { read.reload(); throw e; }
      await put(fresh.revision, rebased.merged);
    }
    opIds.release(); read.reload(); onChanged();
  }, t('server.live.settingsSaved'));
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={src.name} description={t('web.liveSource.settingsLede')} width={620} actions={<><Button variant="ghost" label={t('action.close')} onClick={onClose} /><Button variant="primary" label={t('action.save')} loading={action.busy} disabled={!dirty} onClick={save} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
        {read.error ? <Notice tone={doc ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice> : null}
        {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
        {action.notice ? <Notice tone="success" compact>{action.notice}</Notice> : null}
        {!draft && !read.error ? <Loading label={t('web.librarySettings.loading')} /> : null}
        {draft && doc ? (
          <>
            <Select label={t('web.liveSource.provides')} value={draft.kind} onChange={e => (kind => patch({...guideOnlyPatch(kind)}))(e.target.value)} options={doc.availableKinds.map(k => ({value: k, label: kindName[k] ? t(kindName[k]!) : k}))} />
            {guideOnly ? <Text variant="caption" tone="tertiary">{t('web.liveSource.guideOnlyNote')}</Text> : (
              <Select label={t('web.liveSource.guideFrom')} help={t('web.liveSource.guideFromHelp')} value={draft.guideSourceId || 'own'} onChange={e => patch({guideSourceId: e.target.value === 'own' ? '' : e.target.value})} options={[{value: 'own', label: t('web.liveSource.ownGuide')}, ...guides.filter(g => g.id !== src.id).map(g => ({value: g.id, label: g.name}))]} />
            )}
            {/* Bringing in only part of a large line-up: ticked from what the source really has. Nothing ticked means all of it. */}
            {!guideOnly ? (
              <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
                <SourceChoicePicker sourceId={src.id} kind="groups" label={t('server.live.onlyGroups')} help={t('server.live.pickGroupsHelp')} value={draft.enabledGroups} onChange={enabledGroups => patch({enabledGroups})} />
                <SourceChoicePicker sourceId={src.id} kind="channels" label={t('server.live.onlyChannels')} help={t('server.live.pickChannelsHelp')} value={draft.enabledChannels} onChange={enabledChannels => patch({enabledChannels})} />
              </div>
            ) : null}
            {!guideOnly ? (
              <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
                <Text variant="label" tone="secondary">{t('web.liveSource.keepChannels')}</Text>
                {(Object.keys(filterName) as (keyof typeof filterName)[]).map(key => (
                  <div key={key} style={{display: 'flex', gap: 8, alignItems: 'flex-end', flexWrap: 'wrap'}}>
                    <div style={{flex: '1 1 260px'}}><Input label={t(filterName[key])} placeholder={t('web.liveSource.filterPlaceholder')} value={draft.filters[key].values.join(', ')} onChange={e => patch({filters: {...draft.filters, [key]: {...draft.filters[key], values: list(e.target.value)}}})} /></div>
                    <Segmented size="sm" label={t('web.liveSource.filterMode', {filter: t(filterName[key])})} value={draft.filters[key].mode} onChange={mode => patch({filters: {...draft.filters, [key]: {...draft.filters[key], mode}}})} options={[{id: 'include', label: t('web.liveSource.onlyThese')}, {id: 'exclude', label: t('web.liveSource.allButThese')}]} />
                  </div>
                ))}
              </div>
            ) : null}
            {!guideOnly ? (
              <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
                <Text variant="label" tone="secondary">{t('web.liveSource.channelNumbers')}</Text>
                <Segmented label={t('web.liveSource.numbering')} value={draft.channelNumbering.mode} onChange={mode => patch({channelNumbering: {...draft.channelNumbering, mode}})} options={[{id: 'source', label: t('web.liveSource.asSent')}, {id: 'sequential', label: t('web.liveSource.inOrder')}]} />
                {draft.channelNumbering.mode === 'sequential' ? (
                  <div style={{display: 'flex', gap: 8}}>
                    <Input label={t('web.liveSource.startAt')} type="number" min={1} value={String(draft.channelNumbering.startAt)} onChange={e => patch({channelNumbering: {...draft.channelNumbering, startAt: Math.max(1, Number(e.target.value) || 1)}})} style={{width: 120}} />
                    <Input label={t('web.liveSource.countBy')} type="number" min={1} value={String(draft.channelNumbering.step)} onChange={e => patch({channelNumbering: {...draft.channelNumbering, step: Math.max(1, Number(e.target.value) || 1)}})} style={{width: 120}} />
                  </div>
                ) : null}
              </div>
            ) : null}
            {!guideOnly ? (
              <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
                <Text variant="label" tone="secondary">{t('web.liveSource.channelLogos')}</Text>
                <Checkbox checked={draft.logoImport.enabled} onCheckedChange={enabled => patch({logoImport: {...draft.logoImport, enabled}})} label={t('web.liveSource.bringLogos')} />
                <Checkbox checked={draft.logoImport.overwriteExisting} disabled={!draft.logoImport.enabled} onCheckedChange={overwriteExisting => patch({logoImport: {...draft.logoImport, overwriteExisting}})} label={t('web.liveSource.replaceLogos')} />
                <div style={{display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap'}}>
                  <Button size="sm" variant="secondary" icon="download" label={t('web.liveSource.importLogos')} loading={logos.busy} disabled={dirty || !doc.settings.logoImport.enabled} onClick={() => void logos.run(async () => { const key = `logos:${src.id}`; const r = await admin.send<{requested: number; imported: number; skipped: number}>(source(src.id, '/logos'), 'POST', {operationId: logoOpIds.forPayload(key)}); logoOpIds.release(); setLogoResult(t('web.liveSource.logoResult', {imported: r.imported, skipped: r.skipped, requested: r.requested})); })} />
                  {logos.error || logoResult ? <Text variant="caption" tone={logos.error ? 'danger' : 'secondary'}>{logos.error || logoResult}</Text> : null}
                </div>
              </div>
            ) : null}
            <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
              <Input label={t('web.liveSource.streamBuffer')} type="number" min={0} placeholder={t('web.liveSource.serverDefault', {value: doc.effective.streamBufferSeconds})} value={draft.streamBufferSeconds === null ? '' : String(draft.streamBufferSeconds)} onChange={e => patch({streamBufferSeconds: e.target.value === '' ? null : Math.max(0, Number(e.target.value) || 0)})} style={{width: 220}} />
              <Input label={t('web.liveSource.retryFor')} type="number" min={0} placeholder={t('web.liveSource.serverDefault', {value: doc.effective.retryWindowSeconds})} value={draft.retryWindowSeconds === null ? '' : String(draft.retryWindowSeconds)} onChange={e => patch({retryWindowSeconds: e.target.value === '' ? null : Math.max(0, Number(e.target.value) || 0)})} style={{width: 220}} />
            </div>
          </>
        ) : null}
      </div>
    </Dialog>
  );
}

type MapEntry = {channelId: string; name: string; sourceNumber: string; number: string; group: string; guideChannelId: string; hidden: boolean; overridden: boolean};
type MapPage = {sourceId: string; items: MapEntry[]; nextCursor: string};
type Override = {number: string; guideChannelId: string; hidden: boolean};

/** Renumber, hide, and point individual channels at a guide listing. Only rows that were
 * touched are sent, and the write is fenced on the source's settings revision. */
export function ChannelMapDialog({src, onClose}: {src: {id: string; name: string}; onClose: () => void}) {
  const admin = useAdmin();
  const {t} = useI18n();
  const action = useAction();
  const [pages, setPages] = useState<MapEntry[]>([]);
  const [cursor, setCursor] = useState<string | null>('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [edits, setEdits] = useState<Record<string, Override>>({});
  const [filter, setFilter] = useState('');
  const [partial, setPartial] = useState<{saved: number; total: number} | null>(null);
  const more = async (from: string) => {
    setLoading(true); setError('');
    try { const page = await admin.get<MapPage>(source(src.id, `/channel-map?limit=100${from ? '&cursor=' + encodeURIComponent(from) : ''}`)); setPages(prev => (from ? [...prev, ...page.items] : page.items)); setCursor(page.nextCursor || null); }
    catch (e) { setError(problem(e)); } finally { setLoading(false); }
  };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void more(''); }, [src.id]);
  const value = (c: MapEntry): Override => edits[c.channelId] ?? {number: c.overridden ? c.number : '', guideChannelId: c.guideChannelId, hidden: c.hidden};
  const edit = (c: MapEntry, p: Partial<Override>) => setEdits(prev => ({...prev, [c.channelId]: {...value(c), ...p}}));
  const changed = Object.keys(edits).length;
  const shown = filter.trim() ? pages.filter(c => (c.name + ' ' + c.number + ' ' + c.group).toLowerCase().includes(filter.trim().toLowerCase())) : pages;
  // BE-API-10: each batch keeps its own stable ID across retries of the same edits.
  const [mapOpIds] = useState(createOperationIds);
  const save = () => void action.run(async () => {
    setPartial(null);
    const all = Object.entries(edits).map(([channelId, o]) => ({channelId, number: o.number.trim(), guideChannelId: o.guideChannelId.trim(), hidden: o.hidden}));
    const batches = chunkChannelMapEntries(all);
    // CD-49: start from a freshly read revision; each batch moves it forward by
    // one, with its own stable operation ID. Batches run sequentially — never
    // in parallel with the same revision — and stop on conflict or failure.
    const start = await admin.get<{revision: number}>(source(src.id, '/configuration'));
    let revision = start.revision;
    let saved = 0;
    for (let i = 0; i < batches.length; i++) {
      const batch = batches[i]!;
      const batchKey = JSON.stringify({revision, batch});
      try {
        await admin.send(source(src.id, '/channel-map'), 'PUT', {expectedRevision: revision, operationId: mapOpIds.forPayload(batchKey), overrides: batch});
        saved += batch.length;
        revision += 1;
      } catch (e) {
        // Retain the unsaved edits (this batch onwards) and report how far it got.
        const remaining = all.slice(saved);
        const keep: Record<string, Override> = {};
        for (const o of remaining) {
          const prior = edits[o.channelId];
          if (prior) keep[o.channelId] = prior;
        }
        setEdits(keep);
        setPartial({saved, total: all.length});
        await more('');
        throw e;
      }
    }
    setEdits({}); mapOpIds.release(); await more('');
  }, t('server.live.saved'));
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('web.liveSource.channelMapTitle', {name: src.name})} description={t('web.liveSource.channelMapLede')} width={760} actions={<><Button variant="ghost" label={t('action.close')} onClick={onClose} /><Button variant="primary" label={changed ? t('web.liveSource.saveMapChanges', {changed}) : t('action.save')} loading={action.busy} disabled={!changed} onClick={save} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: () => void more('')}}>{error}</Notice> : null}
        {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
        {partial && partial.saved < partial.total ? <Notice tone="warning" compact>{t('web.liveSource.channelMapPartial', {saved: partial.saved, total: partial.total})}</Notice> : null}
        {action.notice ? <Notice tone="success" compact>{action.notice}</Notice> : null}
        <Input label={t('web.live.findChannel')} hideLabel placeholder={t('web.live.findChannel')} value={filter} onChange={e => setFilter(e.target.value)} />
        <div style={{maxHeight: '52vh', overflow: 'auto', border: '1px solid var(--line-soft)', borderRadius: 10}}>
          <table style={{width: '100%', borderCollapse: 'collapse', fontSize: 14}}>
            <thead><tr style={{textAlign: 'left', position: 'sticky', top: 0, background: 'var(--surface-raised)'}}>{(['channel', 'fromSource', 'number', 'guideId', 'hidden'] as const).map(h => <th key={h} style={{padding: '8px 12px', fontWeight: 600}}><Text variant="label" tone="tertiary">{t(`server.live.map.${h}`)}</Text></th>)}</tr></thead>
            <tbody>
              {shown.map(c => {
                const v = value(c);
                return (
                  <tr key={c.channelId} style={{borderTop: '1px solid var(--line-soft)', opacity: v.hidden ? 0.55 : 1}}>
                    <td style={{padding: '8px 12px'}}><Text variant="bodyStrong" clamp={1}>{c.name}</Text>{c.group ? <Text variant="caption" tone="tertiary" clamp={1}>{c.group}</Text> : null}</td>
                    <td style={{padding: '8px 12px'}}><Text variant="caption" tone="tertiary">{c.sourceNumber || '—'}</Text></td>
                    <td style={{padding: '8px 12px', width: 110}}><Input label={t('web.liveSource.numberFor', {name: c.name})} hideLabel placeholder={c.number || '—'} value={v.number} maxLength={12} onChange={e => edit(c, {number: e.target.value})} /></td>
                    <td style={{padding: '8px 12px', width: 200}}><Input label={t('web.liveSource.guideFor', {name: c.name})} hideLabel placeholder={t('player.quality.automatic')} value={v.guideChannelId} maxLength={200} onChange={e => edit(c, {guideChannelId: e.target.value})} /></td>
                    <td style={{padding: '8px 12px', width: 70}}><Switch checked={v.hidden} onCheckedChange={hidden => edit(c, {hidden})} label={t('web.liveSource.hideChannel', {name: c.name})} /></td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          {loading ? <div style={{padding: 12}}><Loading label={t('web.liveSource.loadingChannels')} /></div> : null}
          {!loading && !pages.length && !error ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.liveSource.noChannels')}</Text></div> : null}
        </div>
        {cursor ? <div><Button size="sm" variant="ghost" label={t('action.showMoreChannels')} disabled={loading} onClick={() => void more(cursor)} /></div> : null}
      </div>
    </Dialog>
  );
}

type Tuner = {deviceId: string; model: string; friendlyName: string; address: string; baseUrl: string; tunerCount: number; configured: boolean};
type Discovery = {devices: Tuner[]; discoveryEnabled: boolean; message?: string};

/** Looks for network tuners on the server's own network. Nothing is added by looking. */
export function DiscoverTunersDialog({onClose, onUse}: {onClose: () => void; onUse: (tuner: Tuner) => void}) {
  const admin = useAdmin();
  const {t} = useI18n();
  const [result, setResult] = useState<Discovery>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const search = async () => {
    setBusy(true); setError('');
    try { setResult(await admin.send<Discovery>('/v1/admin/live-sources/discover', 'POST', {windowSeconds: 5})); } catch (e) { setError(problem(e)); } finally { setBusy(false); }
  };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void search(); }, []);
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('web.liveSource.discoverTitle')} description={t('web.liveSource.discoverLede')} width={520} actions={<><Button variant="ghost" label={t('action.close')} onClick={onClose} /><Button variant="secondary" icon="refresh" label={t('web.liveSource.searchAgain')} loading={busy} onClick={() => void search()} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        {busy && !result ? <Loading label={t('web.liveSource.searching')} /> : null}
        {result && !result.discoveryEnabled ? <Notice tone="info" compact>{t('web.liveSource.searchOff')}</Notice> : null}
        {result && result.discoveryEnabled && !result.devices.length ? <Notice tone="info" compact>{t('web.liveSource.noTuners')}</Notice> : null}
        {result?.devices.map(found => <SettingsRow key={found.deviceId || found.address} icon="live" label={found.friendlyName || found.model || t('web.liveSource.tunerFallback')} help={[found.model, found.address, found.tunerCount ? t('server.live.tunerCount', {count: found.tunerCount}) : ''].filter(Boolean).join(' · ')} control={found.configured ? <Badge tone="neutral">{t('web.liveSource.alreadyAdded')}</Badge> : <Button size="sm" variant="secondary" label={t('web.liveSource.useThis')} onClick={() => onUse(found)} />} />)}
      </div>
    </Dialog>
  );
}

type Keep = {mode: 'keep-all' | 'keep-count' | 'keep-days'; keepCount: number; keepDays: number; keepUntilWatched: boolean};
type Group = {owner: RecordingOwner; anchor?: RecordingAnchor; status?: string; id: string; kind: 'series' | 'keyword'; name: string; match: string; sourceId: string; enabled: boolean; revision: number; options: {prePaddingSeconds: number | null; postPaddingSeconds: number | null; keepPolicy: Keep | null; newEpisodesOnly: boolean; priority: number; folderTemplate: string}};
const blank: Omit<Group, 'id' | 'revision'> = {owner: {authority: 'local', accountId: '', profileId: ''}, kind: 'series', name: '', match: '', sourceId: '', enabled: true, options: {prePaddingSeconds: null, postPaddingSeconds: null, keepPolicy: null, newEpisodesOnly: false, priority: 0, folderTemplate: ''}};

/** Standing instructions for an explicitly owned series selected from the real guide. */
export function RecordingRulesGroup({sources}: {sources: readonly {id: string; name: string}[]}) {
  const admin = useAdmin();
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const serverId = useServerId();
  const policy = useRead(async () => {
    const grants = parseRecordingGrants(await admin.get<unknown>('/v1/admin/dvr/recording-permissions'));
    const owners: RecordingOwnerChoice[] = [];
    let cursor = '';
    const seen = new Set<string>();
    do {
      const page = parseRecordingOwners(await admin.get<unknown>('/v1/admin/dvr/recording-owners?limit=200' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : '')));
      owners.push(...page.items);
      cursor = page.nextCursor;
      if (cursor && seen.has(cursor)) throw Object.assign(new Error('invalid_response'), {code: 'invalid_response'});
      seen.add(cursor);
    } while (cursor);
    return {owners, grants};
  }, [admin]);
  // A rule a member makes from the guide, or pauses, shows here by itself.
  const read = useRead(() => admin.get<{items: Group[]}>('/v1/admin/dvr/recording-groups?limit=100'), [admin], {every: QUIET_READ_EVERY_MS});
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [ruleOpIds] = useState(createOperationIds);
  const [edit, setEdit] = useState<(Omit<Group, 'id' | 'revision'> & {id?: string; revision?: number}) | null>(null);
  const [remove, setRemove] = useState<Group | null>(null);
  const groups = read.data?.items ?? [];
  // A rule as the server has it now: read before pausing, resuming or removing it is sent again.
  const again = (id: string) => async () => (await admin.get<{items: Group[]}>('/v1/admin/dvr/recording-groups?limit=100')).items.find(x => x.id === id);
  const [guideDate, setGuideDate] = useState(() => new Date().toISOString().slice(0, 10));
  const [guideSource, setGuideSource] = useState('');
  const [guideCursor, setGuideCursor] = useState('');
  const guideRoute = useMemo(() => recordingGuideRoute(guideDate, guideSource), [guideDate, guideSource]);
  const guide = useRead(async () => {
    if (!guideRoute) throw new Error('Choose a valid guide date.');
    const q = new URLSearchParams({kind: guideRoute.kind, start: guideRoute.start, end: guideRoute.end, timezone: guideRoute.timezone, search: guideRoute.search, sourceId: guideRoute.sourceId, limit: '30', cursor: guideCursor});
    return parseChannelGuide(await api.request('/v1/guide?' + q), serverId, guideRoute);
  }, [api, serverId, guideRoute, guideCursor, !!edit], {auto: !!edit && !!guideRoute});
  const series = guide.data ? recordingSeries(guide.data) : [];
  const chosenSeries = series.find(s => s.anchor.programmeId === edit?.anchor?.programmeId && s.anchor.generation === edit?.anchor?.generation);
  const owners = policy.data?.owners ?? [];
  const grants = policy.data?.grants ?? [];
  const allowedOwners = owners.filter(o => recordingAllowed(o.owner, grants));
  const legacy = (g: Group) => g.status === 'needs-owner-and-guide-selection' || !g.owner?.accountId || !g.owner?.profileId || g.kind !== 'series';
  const valid = !!edit?.name.trim() && !!edit.match && !!edit.sourceId && !!edit.owner.accountId && recordingAllowed(edit.owner, grants) && (!!edit.id || !!chosenSeries && !!guideRoute && !guide.loading && !guide.error);
  const openEdit = (g?: Group) => { setGuideCursor(''); setGuideSource(g?.sourceId ?? ''); setEdit(g && !legacy(g) ? {...g} : {...blank, name: g?.name ?? ''}); };

  const body = (g: NonNullable<typeof edit>) => {
    const key = JSON.stringify({id: g.id ?? null, revision: g.revision ?? null, owner: g.owner, anchor: g.anchor, kind: g.kind, name: g.name, match: g.match, sourceId: g.sourceId, enabled: g.enabled, options: g.options});
    return recordingRulePayload(g, grants, ruleOpIds.forPayload(key));
  };
  const save = () => void action.run(async () => { if (!edit || !valid) return; await admin.send(edit.id ? `/v1/admin/dvr/recording-groups/${encodeURIComponent(edit.id)}` : '/v1/admin/dvr/recording-groups', edit.id ? 'PUT' : 'POST', body(edit)); ruleOpIds.release(); setEdit(null); read.reload(); }, t('server.live.saved'));
  const items = (g: Group): MenuItem[] => [{id: 'edit', label: legacy(g) ? t('web.recordingRules.replaceOwned') : t('web.recordingRules.editRule'), icon: 'edit'}, {id: 'toggle', label: g.enabled ? t('web.recordingRules.pauseRule') : t('web.recordingRules.resumeRule'), icon: 'pause', disabled: legacy(g) || !recordingAllowed(g.owner, grants)}, {id: 'remove', label: t('web.recordingRules.remove'), icon: 'trash', destructive: true, separatorBefore: true}];
  return (
    <>
    {policy.error ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: policy.reload}}>{policy.error}</Notice> : null}
    {policy.data ? <RecordingPermissionsGroup owners={owners} grants={grants} onSaved={policy.reload} /> : null}
    <SettingsGroup title={t('web.recordingRules.title')} description={t('web.recordingRules.lede')} action={<Button size="sm" variant="secondary" icon="plus" label={t('web.recordingRules.newRule')} disabled={!policy.data} onClick={() => openEdit()} />}>
      {read.error ? <SettingsNote><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.error && !edit && !remove ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
      {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
      {!read.data && read.loading ? <SettingsLoading thing={t('server.thing.recordingRules')} /> : null}
      {read.data && !groups.length ? <SettingsEmpty title={t('web.recordingRules.noRules')} body={t('web.recordingRules.noRulesHelp')} /> : null}
      {groups.map(g => <SettingsRow key={g.id} icon="dvr" label={g.name} help={[g.kind === 'series' ? t('web.recordingRules.seriesKind') : t('web.recordingRules.matchTitles', {match: g.match}), g.options.newEpisodesOnly ? t('web.recordingRules.newOnly') : '', g.sourceId ? sources.find(s => s.id === g.sourceId)?.name ?? '' : t('web.recordingRules.anySource')].filter(Boolean).join(' · ')} meta={legacy(g) ? t('web.recordingRules.chooseOwner') : g.enabled ? undefined : t('web.recordingRules.pausedRule')} control={<Menu label={t('web.liveSource.ruleOptions', {name: g.name})} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('web.liveSource.ruleOptions', {name: g.name})} />} items={items(g)} onSelect={id => {
        if (id === 'edit') openEdit(g);
        else if (id === 'toggle' && !legacy(g) && recordingAllowed(g.owner, grants)) { const enabled = !g.enabled; void action.run(async () => { await sendFresh(g, x => admin.send(`/v1/admin/dvr/recording-groups/${encodeURIComponent(x.id)}`, 'PUT', body({...x, enabled})), again(g.id), now => (now.enabled === enabled ? 'done' : true)); ruleOpIds.release(); read.reload(); }); }
        else setRemove(g);
      }} />} />)}
      <Dialog open={!!edit} onOpenChange={o => !o && setEdit(null)} title={edit?.id ? t('web.recordingRules.editTitle') : t('web.recordingRules.newTitle')} width={520} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => setEdit(null)} /><Button variant="primary" label={t('action.save')} loading={action.busy} disabled={!valid} onClick={save} /></>}>
        {edit ? (
          <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
            {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
            <Input label={t('web.recordingRules.ruleName')} value={edit.name} maxLength={160} onChange={e => setEdit({...edit, name: e.target.value})} />
            <Select label={t('web.recordingRules.recordFor')} disabled={!!edit.id} value={edit.owner.accountId ? recordingOwnerKey(edit.owner) : ''} onChange={e => { const choice = allowedOwners.find(o => recordingOwnerKey(o.owner) === e.target.value); if (choice) setEdit({...edit, owner: choice.owner}); }} options={[{value: '', label: t('web.recordingRules.chooseProfile')}, ...allowedOwners.map(o => ({value: recordingOwnerKey(o.owner), label: recordingOwnerLabel(o)}))]} />
            {!allowedOwners.length ? <Notice tone="info" compact>{t('web.recordingRules.allowFirst')}</Notice> : null}
            {!edit.id ? <>
              <Select label={t('live.guideSource')} value={guideSource} onChange={e => { setGuideSource(e.target.value); setGuideCursor(''); setEdit({...edit, anchor: undefined, match: '', sourceId: ''}); }} options={[{value: '', label: t('web.live.allSources')}, ...sources.map(s => ({value: s.id, label: s.name}))]} />
              <Input label={t('web.recordingRules.guideDate')} type="date" value={guideDate} onChange={e => { setGuideDate(e.target.value); setGuideCursor(''); setEdit({...edit, anchor: undefined, match: '', sourceId: ''}); }} />
              {!guideRoute ? <Notice tone="info" compact>{t('web.recordingRules.validDate')}</Notice> : null}
              {guide.error ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: guide.reload}}>{guide.error}</Notice> : null}
              {guide.loading ? <Loading label={t('web.recordingRules.readingGuide')} /> : null}
              <Select label={t('web.recordingRules.seriesFromGuide')} value={chosenSeries?.key ?? ''} onChange={e => { const picked = series.find(s => s.key === e.target.value); if (picked) setEdit({...edit, name: edit.name || picked.title, kind: 'series', match: picked.seriesId, sourceId: picked.anchor.sourceId, anchor: picked.anchor, enabled: picked.recordAvailable, options: {...edit.options, newEpisodesOnly: false}}); }} options={[{value: '', label: t('web.recordingRules.chooseProgram')}, ...series.map(s => ({value: s.key, label: t('web.recordingRules.seriesOption', {title: s.title, channel: s.channelName, time: i18n.time(s.startsAt), available: s.recordAvailable ? 'yes' : 'no'})}))]} />
              {!guide.loading && guide.data && !series.length ? <Text variant="caption" tone="secondary">{t('web.recordingRules.noSeries')}</Text> : null}
              <div style={{display: 'flex', gap: 8}}>{guideCursor ? <Button size="sm" variant="ghost" label={t('web.recordingRules.firstChannels')} onClick={() => setGuideCursor('')} /> : null}{guide.data?.nextCursor ? <Button size="sm" variant="ghost" label={t('web.live.empty.moreTitle')} onClick={() => setGuideCursor(guide.data!.nextCursor)} /> : null}</div>
              {chosenSeries && !chosenSeries.recordAvailable ? <Notice tone="info" compact>{t('web.recordingRules.captureUnavailable')}</Notice> : null}
            </> : <Text variant="caption" tone="secondary">{t('web.recordingRules.fixedNote')}</Text>}
            <Checkbox checked={edit.options.newEpisodesOnly} disabled={!edit.options.newEpisodesOnly && !(chosenSeries ? ['new', 'repeat'].includes(chosenSeries.newEvidence) : series.some(s => s.seriesId === edit.match && ['new', 'repeat'].includes(s.newEvidence)))} onCheckedChange={newEpisodesOnly => setEdit({...edit, options: {...edit.options, newEpisodesOnly}})} label={t('guide.newOnly')} />
            <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
              <Input label={t('web.recordingRules.startEarlySeconds')} type="number" min={0} placeholder={t('web.recordingRules.default')} value={edit.options.prePaddingSeconds === null ? '' : String(edit.options.prePaddingSeconds)} onChange={e => setEdit({...edit, options: {...edit.options, prePaddingSeconds: e.target.value === '' ? null : Math.max(0, Number(e.target.value) || 0)}})} style={{width: 170}} />
              <Input label={t('web.recordingRules.endLateSeconds')} type="number" min={0} placeholder={t('web.recordingRules.default')} value={edit.options.postPaddingSeconds === null ? '' : String(edit.options.postPaddingSeconds)} onChange={e => setEdit({...edit, options: {...edit.options, postPaddingSeconds: e.target.value === '' ? null : Math.max(0, Number(e.target.value) || 0)}})} style={{width: 170}} />
              <Input label={t('web.recordingRules.priority')} help={t('web.recordingRules.priorityHelp')} type="number" value={String(edit.options.priority)} onChange={e => setEdit({...edit, options: {...edit.options, priority: Math.round(Number(e.target.value) || 0)}})} style={{width: 130}} />
            </div>
            <Select label={t('web.dvr.keepLabel')} value={edit.options.keepPolicy?.mode ?? 'default'} onChange={e => setEdit({...edit, options: {...edit.options, keepPolicy: e.target.value === 'default' ? null : {mode: e.target.value as Keep['mode'], keepCount: edit.options.keepPolicy?.keepCount || 5, keepDays: edit.options.keepPolicy?.keepDays || 30, keepUntilWatched: edit.options.keepPolicy?.keepUntilWatched ?? false}}})} options={[{value: 'default', label: t('web.recordingRules.useDefaults')}, {value: 'keep-all', label: t('web.logs.everything')}, {value: 'keep-count', label: t('web.recordingRules.latestFew')}, {value: 'keep-days', label: t('web.dvrSettings.keepDays')}]} />
            {edit.options.keepPolicy?.mode === 'keep-count' ? <Input label={t('web.dvrSettings.howMany')} type="number" min={1} value={String(edit.options.keepPolicy.keepCount)} onChange={e => setEdit({...edit, options: {...edit.options, keepPolicy: {...edit.options.keepPolicy!, keepCount: Math.max(1, Number(e.target.value) || 1)}}})} style={{width: 140}} /> : null}
            {edit.options.keepPolicy?.mode === 'keep-days' ? <Input label={t('web.maintenance.days')} type="number" min={1} value={String(edit.options.keepPolicy.keepDays)} onChange={e => setEdit({...edit, options: {...edit.options, keepPolicy: {...edit.options.keepPolicy!, keepDays: Math.max(1, Number(e.target.value) || 1)}}})} style={{width: 140}} /> : null}
          </div>
        ) : null}
      </Dialog>
      <ConfirmDialog open={!!remove} onOpenChange={o => !o && setRemove(null)} title={t('web.recordingRules.removeTitle', {name: remove?.name ?? ''})} body={t('web.recordingRules.removeBody')} confirmLabel={t('web.recordingRules.remove')} busy={action.busy} error={action.error || undefined} onConfirm={() => { const g = remove; if (g) { const key = JSON.stringify({id: g.id, revision: g.revision}); void action.run(async () => { await sendFresh(g, x => admin.send(`/v1/admin/dvr/recording-groups/${encodeURIComponent(x.id)}?expectedRevision=${x.revision}&operationId=${ruleOpIds.forPayload(JSON.stringify({id: x.id, revision: x.revision}))}`, 'DELETE'), again(g.id)); ruleOpIds.release(); setRemove(null); read.reload(); }); } }} />
    </SettingsGroup>
    </>
  );
}

type SourceChoice = LiveSourceChoicePage['items'][number];

/**
 * Choose some of a source's groups or channels from what it really has: a search field, the
 * matches as a checklist a page at a time, and what is chosen so far with a way to clear it.
 * Something chosen earlier that the source no longer lists stays chosen until it is unticked.
 */
function SourceChoicePicker({sourceId, kind, label, help, value, onChange}: {sourceId: string; kind: 'groups' | 'channels'; label: string; help: string; value: readonly string[]; onChange: (next: readonly string[]) => void}) {
  const {t} = useI18n();
  const {api} = useSession();
  const [open, setOpen] = useState(value.length > 0);
  const [query, setQuery] = useState('');
  const [items, setItems] = useState<readonly SourceChoice[]>([]);
  const [total, setTotal] = useState(0);
  const [cursor, setCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState('');
  useEffect(() => {
    if (!open) return;
    let live = true;
    // What is typed is searched once typing pauses; a new search starts the list again.
    const timer = setTimeout(() => {
      setLoading(true); setFailed('');
      liveSourceChoicesService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body), sourceId).list(kind, '', 50, query.trim())
        .then(page => { if (live) { setItems(page.items); setTotal(page.total); setCursor(page.nextCursor); } }, e => { if (live) setFailed(problem(e)); })
        .finally(() => { if (live) setLoading(false); });
    }, query ? 300 : 0);
    return () => { live = false; clearTimeout(timer); };
  }, [open, api, sourceId, kind, query]);
  const more = () => {
    setLoading(true);
    liveSourceChoicesService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body), sourceId).list(kind, cursor, 50, query.trim())
      .then(page => { setItems(prev => [...prev, ...page.items.filter(x => !prev.some(p => p.id === x.id))]); setCursor(page.nextCursor); }, e => setFailed(problem(e)))
      .finally(() => setLoading(false));
  };
  const toggle = (id: string, on: boolean) => onChange(on ? [...value.filter(x => x !== id), id] : value.filter(x => x !== id));
  const gone = value.filter(id => !items.some(x => x.id === id));
  return (
    <fieldset style={{border: 0, padding: 0, margin: 0, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 8}}>
      <legend style={{padding: 0, marginBottom: 8, display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap'}}>
        <Text variant="label" tone="secondary">{label}</Text>
        <Text variant="caption" tone="tertiary">{value.length ? t('server.live.chosenCount', {count: value.length}) : t('server.live.chosenAll')}</Text>
        {value.length ? <Button size="sm" variant="link" label={t('server.live.chooseAll')} onClick={() => onChange([])} /> : null}
        {!open ? <Button size="sm" variant="link" label={t('server.live.choose')} onClick={() => setOpen(true)} /> : null}
      </legend>
      {open ? (
        <>
          <Input hideLabel type="search" label={t(kind === 'groups' ? 'server.live.searchGroups' : 'server.live.searchChannels')} placeholder={t(kind === 'groups' ? 'server.live.searchGroups' : 'server.live.searchChannels')} value={query} onChange={e => setQuery(e.target.value)} />
          {failed ? <Notice tone="error" compact>{failed}</Notice> : null}
          <div style={{display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 240, overflowY: 'auto'}}>
            {items.map(x => <Checkbox key={x.id} checked={value.includes(x.id)} onCheckedChange={on => toggle(x.id, on)} label={x.name || x.id} help={kind === 'groups' ? t('server.live.channelCount', {count: x.channels}) : x.group || undefined} />)}
            {/* Chosen before and not in this list (another search, or no longer in the source): still shown, so it can be unticked. */}
            {gone.map(id => <Checkbox key={'chosen:' + id} checked onCheckedChange={() => toggle(id, false)} label={id} />)}
            {!items.length && !gone.length && !loading && !failed ? <Text variant="caption" tone="tertiary">{t('server.live.noMatches')}</Text> : null}
          </div>
          {cursor ? <div><Button size="sm" variant="ghost" label={t('action.showMore')} loading={loading} onClick={more} /></div> : null}
          <Text variant="caption" tone="tertiary">{total ? `${t('server.live.choiceTotal', {count: total})} ${help}` : help}</Text>
        </>
      ) : null}
    </fieldset>
  );
}
