import React, {useEffect, useMemo, useState} from 'react';
import {parseEnvelope} from '@core/administration.ts';
import {parseChannelGuide, type GuideRoute} from '@core/channel-guide.ts';
import {parseRecordingGrants, parseRecordingOwners, recordingAllowed, recordingOwnerKey, recordingOwnerLabel, recordingSeries, recordingGuideRoute, recordingRulePayload, type RecordingOwner, type RecordingAnchor, type RecordingOwnerChoice} from '../../admin/recording-policy';
import {RecordingPermissionsGroup} from './DVRSettings';
import {guideOnlyPatch} from '@core/server-admin/panel-logic.ts';
export {guideOnlyPatch} from '@core/server-admin/panel-logic.ts';
import {problem, useAction, useRead} from '../../admin/console';
import {createOperationIds} from './operation-ids';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Badge, Button, Checkbox, ConfirmDialog, Dialog, Input, Loading, Menu, Notice, Segmented, Select, SettingsGroup, SettingsRow, Switch, Text, type MenuItem} from '../../ui';

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
type SourceSettings = {kind: string; streamBufferSeconds: number | null; retryWindowSeconds: number | null; userAgent: string; filters: {categories: FilterRule; countries: FilterRule; keywords: FilterRule}; logoImport: {enabled: boolean; overwriteExisting: boolean}; channelNumbering: {mode: 'source' | 'sequential'; startAt: number; step: number}; guideSourceId: string};
type SourceDocument = {sourceId: string; sourceName: string; revision: number; settings: SourceSettings; effective: {streamBufferSeconds: number; retryWindowSeconds: number}; availableKinds: string[]};
const kindName: Record<string, string> = {playlist: 'Channels (playlist or tuner)', 'xmltv-guide': 'Guide only (XMLTV)', hdhomerun: 'HDHomeRun tuner'};
const filterName = {categories: 'Categories', countries: 'Countries', keywords: 'Words in the channel name'} as const;

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
  const save = () => void action.run(async () => { if (!draft || !doc) return; const key = JSON.stringify({revision: doc.revision, settings: draft}); await admin.send(source(src.id, '/configuration'), 'PUT', {expectedRevision: doc.revision, operationId: opIds.forPayload(key), settings: draft}); opIds.release(); read.reload(); onChanged(); }, 'Saved. Changes to filters and numbering show after the next guide refresh.');
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={src.name} description={t('web.liveSource.settingsLede')} width={620} actions={<><Button variant="ghost" label={t('action.close')} onClick={onClose} /><Button variant="primary" label={t('action.save')} loading={action.busy} disabled={!dirty} onClick={save} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
        {read.error ? <Notice tone={doc ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice> : null}
        {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
        {action.notice ? <Notice tone="success" compact>{action.notice}</Notice> : null}
        {!draft && !read.error ? <Loading label={t('web.librarySettings.loading')} /> : null}
        {draft && doc ? (
          <>
            <Select label={t('web.liveSource.provides')} value={draft.kind} onChange={e => (kind => patch({...guideOnlyPatch(kind)}))(e.target.value)} options={doc.availableKinds.map(k => ({value: k, label: kindName[k] ?? k}))} />
            {guideOnly ? <Text variant="caption" tone="tertiary">{t('web.liveSource.guideOnlyNote')}</Text> : (
              <Select label={t('web.liveSource.guideFrom')} help={t('web.liveSource.guideFromHelp')} value={draft.guideSourceId || 'own'} onChange={e => patch({guideSourceId: e.target.value === 'own' ? '' : e.target.value})} options={[{value: 'own', label: t('web.liveSource.ownGuide')}, ...guides.filter(g => g.id !== src.id).map(g => ({value: g.id, label: g.name}))]} />
            )}
            {!guideOnly ? (
              <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
                <Text variant="label" tone="secondary">{t('web.liveSource.keepChannels')}</Text>
                {(Object.keys(filterName) as (keyof typeof filterName)[]).map(key => (
                  <div key={key} style={{display: 'flex', gap: 8, alignItems: 'flex-end', flexWrap: 'wrap'}}>
                    <div style={{flex: '1 1 260px'}}><Input label={filterName[key]} placeholder={t('web.liveSource.filterPlaceholder')} value={draft.filters[key].values.join(', ')} onChange={e => patch({filters: {...draft.filters, [key]: {...draft.filters[key], values: list(e.target.value)}}})} /></div>
                    <Segmented size="sm" label={t('web.liveSource.filterMode', {filter: key})} value={draft.filters[key].mode} onChange={mode => patch({filters: {...draft.filters, [key]: {...draft.filters[key], mode}}})} options={[{id: 'include', label: t('web.liveSource.onlyThese')}, {id: 'exclude', label: t('web.liveSource.allButThese')}]} />
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
  }, 'Saved.');
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
            <thead><tr style={{textAlign: 'left', position: 'sticky', top: 0, background: 'var(--surface-raised)'}}>{['Channel', 'From source', 'Number', 'Guide listing ID', 'Hidden'].map(h => <th key={h} style={{padding: '8px 12px', fontWeight: 600}}><Text variant="label" tone="tertiary">{h}</Text></th>)}</tr></thead>
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
        {result?.devices.map(found => <SettingsRow key={found.deviceId || found.address} icon="live" label={found.friendlyName || found.model || t('web.liveSource.tunerFallback')} help={[found.model, found.address, found.tunerCount ? `${found.tunerCount} tuners` : ''].filter(Boolean).join(' · ')} control={found.configured ? <Badge tone="neutral">{t('web.liveSource.alreadyAdded')}</Badge> : <Button size="sm" variant="primary" label={t('web.liveSource.useThis')} onClick={() => onUse(found)} />} />)}
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
  const {t} = useI18n();
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
      if (cursor && seen.has(cursor)) throw new Error('Recording profiles could not be read. Refresh and try again.');
      seen.add(cursor);
    } while (cursor);
    return {owners, grants};
  }, [admin]);
  const read = useRead(() => admin.get<{items: Group[]}>('/v1/admin/dvr/recording-groups?limit=100'), [admin]);
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [ruleOpIds] = useState(createOperationIds);
  const [edit, setEdit] = useState<(Omit<Group, 'id' | 'revision'> & {id?: string; revision?: number}) | null>(null);
  const [remove, setRemove] = useState<Group | null>(null);
  const groups = read.data?.items ?? [];
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
  const save = () => void action.run(async () => { if (!edit || !valid) return; await admin.send(edit.id ? `/v1/admin/dvr/recording-groups/${encodeURIComponent(edit.id)}` : '/v1/admin/dvr/recording-groups', edit.id ? 'PUT' : 'POST', body(edit)); ruleOpIds.release(); setEdit(null); read.reload(); }, 'Saved.');
  const items = (g: Group): MenuItem[] => [{id: 'edit', label: legacy(g) ? t('web.recordingRules.replaceOwned') : t('web.recordingRules.editRule'), icon: 'edit'}, {id: 'toggle', label: g.enabled ? t('web.recordingRules.pauseRule') : t('web.recordingRules.resumeRule'), icon: 'pause', disabled: legacy(g) || !recordingAllowed(g.owner, grants)}, {id: 'remove', label: t('web.recordingRules.remove'), icon: 'trash', destructive: true, separatorBefore: true}];
  return (
    <>
    {policy.error ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: policy.reload}}>{policy.error}</Notice> : null}
    {policy.data ? <RecordingPermissionsGroup owners={owners} grants={grants} onSaved={policy.reload} /> : null}
    <SettingsGroup title={t('web.recordingRules.title')} description={t('web.recordingRules.lede')} action={<Button size="sm" variant="secondary" icon="plus" label={t('web.recordingRules.newRule')} disabled={!policy.data} onClick={() => openEdit()} />}>
      {read.error ? <div style={{padding: 12}}><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {action.error && !edit ? <div style={{padding: 12}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
      {read.data && !groups.length ? <SettingsRow label={t('web.recordingRules.noRules')} help={t('web.recordingRules.noRulesHelp')} /> : null}
      {groups.map(g => <SettingsRow key={g.id} icon="dvr" label={g.name} help={[g.kind === 'series' ? t('web.recordingRules.seriesKind') : t('web.recordingRules.matchTitles', {match: g.match}), g.options.newEpisodesOnly ? t('web.recordingRules.newOnly') : '', g.sourceId ? sources.find(s => s.id === g.sourceId)?.name ?? '' : t('web.recordingRules.anySource')].filter(Boolean).join(' · ')} meta={legacy(g) ? t('web.recordingRules.chooseOwner') : g.enabled ? undefined : t('web.recordingRules.pausedRule')} control={<Menu label={t('web.liveSource.ruleOptions', {name: g.name})} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('web.liveSource.ruleOptions', {name: g.name})} />} items={items(g)} onSelect={id => {
        if (id === 'edit') openEdit(g);
        else if (id === 'toggle' && !legacy(g) && recordingAllowed(g.owner, grants)) void action.run(async () => { await admin.send(`/v1/admin/dvr/recording-groups/${encodeURIComponent(g.id)}`, 'PUT', body({...g, enabled: !g.enabled})); ruleOpIds.release(); read.reload(); });
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
              <Select label={t('web.recordingRules.seriesFromGuide')} value={chosenSeries?.key ?? ''} onChange={e => { const picked = series.find(s => s.key === e.target.value); if (picked) setEdit({...edit, name: edit.name || picked.title, kind: 'series', match: picked.seriesId, sourceId: picked.anchor.sourceId, anchor: picked.anchor, enabled: picked.recordAvailable, options: {...edit.options, newEpisodesOnly: false}}); }} options={[{value: '', label: t('web.recordingRules.chooseProgram')}, ...series.map(s => ({value: s.key, label: t('web.recordingRules.seriesOption', {title: s.title, channel: s.channelName, time: new Date(s.startsAt).toLocaleTimeString(), available: s.recordAvailable ? 'yes' : 'no'})}))]} />
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
      <ConfirmDialog open={!!remove} onOpenChange={o => !o && setRemove(null)} title={t('web.recordingRules.removeTitle', {name: remove?.name ?? ''})} body={t('web.recordingRules.removeBody')} confirmLabel={t('web.recordingRules.remove')} busy={action.busy} onConfirm={() => { const g = remove; if (g) { const key = JSON.stringify({id: g.id, revision: g.revision}); void action.run(async () => { await admin.send(`/v1/admin/dvr/recording-groups/${encodeURIComponent(g.id)}?expectedRevision=${g.revision}&operationId=${ruleOpIds.forPayload(key)}`, 'DELETE'); ruleOpIds.release(); setRemove(null); read.reload(); }); } }} />
    </SettingsGroup>
    </>
  );
}
