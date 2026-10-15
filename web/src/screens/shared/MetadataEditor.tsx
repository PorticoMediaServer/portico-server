import React, {useEffect, useMemo, useState} from 'react';
import {MetadataRepairService, type RepairData, type RepairRelationship, type RepairTarget, type RepairView} from '@core/metadata-repair.ts';
import {bulkTargetLimit, type BulkFieldEdit} from '@core/metadata-bulk.ts';
import {orderTakesOffset, readTmdbOrder, tmdbAbsoluteOrder} from '@core/screen-metadata.ts';
import {groupMetadataTargets, runBulkRequests} from './bulk-job';
import {useService} from '../../app/content';
import {useMetadataEditor} from '../../app/metadata-editor';
import {useSession} from '../../app/session';
import {Artwork, Button, Dialog, Icon, Input, Loading, Notice, Select, Spinner, Text, TextArea, cx, useCompact, type IconName} from '../../ui';
import s from './MetadataEditor.module.css';
import {currentI18n, useI18n, type I18n, type MessageId} from '../../app/i18n';
import {errorText} from '../../app/errors';
import {useViewerScope} from '../../app/viewer-scope';

/**
 * The metadata editor. Reads the owner "repair" projection for one entity and
 * turns every change into a revision-fenced command: field edits with locks,
 * artwork choice, genres and credits, provider matching, and undo. Editing a
 * field locks it (a provider refresh will not overwrite it) unless the viewer
 * unlocked it on purpose in this session, mirroring the old editor. Bulk mode
 * applies the same field edits to every selected entity, one fenced command
 * each, and reports every outcome.
 */
type FieldType = 'text' | 'multiline' | 'integer' | 'number' | 'date' | 'enum' | 'list';
export type FieldSpec = {field: string; label: string; group?: string; type: FieldType; bulk: boolean; maxLength?: number; min?: number; max?: number; allowed?: readonly string[]};
type Data = RepairData & {schema?: readonly FieldSpec[]; artworkRoles?: readonly string[]};
type Tab = 'general' | 'artwork' | 'tags' | 'people' | 'matching' | 'history';

/** The editor's own name for a field the server sent without a label (its labels win whenever they are there). */
const fallbackLabels: Readonly<Record<string, MessageId>> = {title: 'web.metadata.field.title', description: 'web.metadata.field.description', year: 'web.metadata.field.year', author: 'web.metadata.field.author', narrator: 'web.metadata.field.narrator', number: 'web.metadata.field.number', sortTitle: 'web.metadata.field.sortTitle', originalTitle: 'web.metadata.field.originalTitle', edition: 'web.metadata.field.edition', tagline: 'web.metadata.field.tagline', releaseDate: 'web.metadata.field.releaseDate', contentRating: 'web.metadata.field.contentRating', studio: 'web.metadata.field.studio', network: 'web.metadata.field.network', country: 'web.metadata.field.country', seasonNumber: 'web.metadata.field.seasonNumber', episodeNumber: 'web.metadata.field.episodeNumber', trackNumber: 'web.metadata.field.trackNumber', discNumber: 'web.metadata.field.discNumber', partNumber: 'web.metadata.field.partNumber', series: 'web.metadata.field.series', seriesIndex: 'web.metadata.field.seriesIndex', label: 'web.metadata.field.label', tags: 'web.metadata.field.tags', labels: 'web.metadata.field.labels'};
const fallbackOrder = ['title', 'sortTitle', 'originalTitle', 'edition', 'year', 'releaseDate', 'contentRating', 'studio', 'network', 'country', 'seasonNumber', 'episodeNumber', 'trackNumber', 'discNumber', 'partNumber', 'author', 'narrator', 'series', 'seriesIndex', 'label', 'number', 'tagline', 'description', 'tags', 'labels'];
function fallbackSchema(data: RepairData, t: I18n['t']): FieldSpec[] {
  const keys = Object.keys(data.snapshot.fields).sort((a, b) => (fallbackOrder.indexOf(a) + 1 || 99) - (fallbackOrder.indexOf(b) + 1 || 99));
  return keys.map(field => ({field, label: fallbackLabels[field] ? t(fallbackLabels[field]) : field.replace(/([A-Z])/g, ' $1').replace(/^./, c => c.toUpperCase()), type: field === 'description' || field === 'tagline' ? 'multiline' : /number|year|index/i.test(field) ? 'integer' : field === 'releaseDate' ? 'date' : field === 'tags' || field === 'labels' ? 'list' : 'text', bulk: !['title', 'sortTitle', 'originalTitle', 'edition', 'tagline', 'seasonNumber', 'episodeNumber', 'trackNumber', 'discNumber', 'partNumber', 'number', 'seriesIndex'].includes(field)}));
}
const tabs: readonly {id: Tab; label: MessageId; icon: IconName}[] = [
  {id: 'general', label: 'web.metadata.tabGeneral', icon: 'edit'}, {id: 'artwork', label: 'web.metadata.tabArtwork', icon: 'film'}, {id: 'tags', label: 'web.metadata.tabTags', icon: 'category'}, {id: 'people', label: 'web.metadata.tabPeople', icon: 'people'}, {id: 'matching', label: 'web.librarySettings.matching', icon: 'link'}, {id: 'history', label: 'web.metadata.tabHistory', icon: 'clock'},
];

export function MetadataEditorDialog() {
  const {t} = useI18n();
  const editor = useMetadataEditor();
  const request = editor?.request;
  const single = request?.targets.length === 1;
  return (
    <Dialog open={!!request} onOpenChange={o => !o && editor?.close()} title={request ? (single ? (request.titles[0] ? t('web.metadata.editTitle', {title: request.titles[0]}) : t('web.metadata.editFallbackTitle')) : t('web.metadata.editCount', {count: request.targets.length})) : ''} description={request ? (single ? t('web.metadata.kindOnServer', {kind: request.targets[0].kind}) : t('web.metadata.bulkDescription')) : undefined} width={960}>
      {request ? (single ? <SingleEditor key={request.targets[0].kind + request.targets[0].id} target={request.targets[0]} onSaved={request.onSaved} onClose={editor!.close} /> : <BulkEditor key={request.targets.map(t => t.id).join(',')} targets={request.targets} onSaved={request.onSaved} onClose={editor!.close} />) : null}
    </Dialog>
  );
}

/* ---------- Single entity ---------- */

function SingleEditor({target, onSaved, onClose}: {target: RepairTarget; onSaved?: () => void; onClose: () => void}) {
  const {api} = useSession();
  const scope = useViewerScope();
  const {service, snapshot} = useService<MetadataRepairService, RepairView>(() => new MetadataRepairService({api, scope}), [api, scope]);
  useEffect(() => { void service.load(target); }, [service, target]);
  const [tab, setTab] = useState<Tab>('general');
  const compact = useCompact();
  const data = snapshot.data as Data | null;
  const busy = snapshot.phase === 'saving' || snapshot.phase === 'loading';
  const run = async (command: Parameters<MetadataRepairService['command']>[0]) => {
    const ok = await service.command(command);
    if (ok) onSaved?.();
    return ok;
  };
  // M25-1a: a search queues async provider work; the POST returns while the
  // status is still transient (searching/pending). Reload with bounded backoff
  // until it settles, so the tab shows the answer (e.g. unmatched) without
  // reopening the dialog.
  const runMatching = async (command: Parameters<MetadataRepairService['command']>[0]) => {
    const ok = await run(command);
    if (ok && (command.action === 'search' || command.action === 'retry')) {
      for (const delay of [1000, 2000, 4000, 8000]) {
        const status = (service.getSnapshot().data as Data | null)?.snapshot.identity?.status;
        if (!isTransientMatchingStatus(status)) break;
        await new Promise(r => setTimeout(r, delay));
        try { await service.load(target); } catch { break; }
      }
    }
    return ok;
  };
  const counts: Partial<Record<Tab, number>> = data ? {artwork: data.artwork.candidates.length, tags: data.snapshot.relationships.filter(r => r.kind === 'genre').length, people: data.snapshot.relationships.filter(r => r.kind === 'credit').length, matching: data.candidates.length, history: data.history.length} : {};
  const i18n = useI18n();
  const t = i18n.t;
  return (
    <div className={s.frame}>
      <nav className={s.rail} aria-label={t('web.metadata.sections')} role="tablist" aria-orientation={compact ? 'horizontal' : 'vertical'}>
        {tabs.map(item => <button key={item.id} type="button" role="tab" aria-selected={tab === item.id} className={s.railButton} onClick={() => setTab(item.id)}><Icon name={item.icon} size={16} />{t(item.label)}{counts[item.id] ? <span className={s.railCount}>{counts[item.id]}</span> : null}</button>)}
      </nav>
      <div className={s.content} role="tabpanel">
        {snapshot.phase === 'denied' ? <Notice tone="error">{snapshot.error}</Notice> : null}
        {snapshot.phase === 'conflict' || snapshot.phase === 'uncertain' || snapshot.phase === 'error' ? <Notice tone={snapshot.phase === 'error' ? 'error' : 'warning'} action={{label: t('action.refresh'), onClick: () => void service.load(target)}}>{snapshot.error}</Notice> : null}
        {!data && snapshot.phase === 'loading' ? <Loading label={t('web.metadata.loading')} /> : null}
        {data && tab === 'general' ? <GeneralTab data={data} busy={busy} onSave={fields => run({action: 'edit', fields})} onClose={onClose} /> : null}
        {data && tab === 'artwork' ? <ArtworkTab data={data} busy={busy} run={run} /> : null}
        {data && tab === 'tags' ? <GenresTab data={data} busy={busy} run={run} /> : null}
        {data && tab === 'people' ? <CreditsTab data={data} busy={busy} run={run} /> : null}
        {data && tab === 'matching' ? <MatchingTab data={data} busy={busy} run={runMatching} /> : null}
        {data && tab === 'history' ? <HistoryTab data={data} busy={busy} run={run} /> : null}
      </div>
    </div>
  );
}

type FieldEdit = {value?: string; values?: readonly string[]; locked?: boolean; useAutomatic?: boolean};
const splitList = (v: string) => v.split(',').map(x => x.trim()).filter(Boolean);
const joinList = (v: readonly string[] | undefined, fallback: string) => (v ? v.join(', ') : fallback);

/* ---------- CD-46 bounds (exactly the server's repair_schema.go rules) ---------- */

export type FieldProblemKey = 'web.metadata.titleRequired' | 'web.metadata.wholeNumber' | 'web.metadata.numberRange' | 'web.metadata.dateError' | 'web.metadata.enumError' | 'web.metadata.textTooLong' | 'web.metadata.listEntryTooLong' | 'web.metadata.tooManyEntries';
export type FieldProblem = {key: FieldProblemKey; params?: Record<string, string | number>};
/** Canonical non-negative decimal integers, like the server's Atoi + Itoa round-trip: no leading zeros, signs, fractions or exponents. */
const CANONICAL_INT = /^(0|[1-9][0-9]*)$/;
const runes = (v: string) => Array.from(v).length;
/** Real YYYY-MM-DD calendar validity, like the server's time.Parse("2006-01-02"). */
export function isRealISODate(value: string): boolean {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!m) return false;
  const year = Number(m[1]), month = Number(m[2]), day = Number(m[3]);
  if (month < 1 || month > 12 || day < 1 || day > 31) return false; // lint-strings-allow: linter false positive on `> 12 || day <` (date math, not copy)
  return day <= new Date(Date.UTC(year, month, 0)).getUTCDate();
}
/**
 * CD-46: one edited text value against its published schema bounds. An empty
 * string clears the field (except the required title), exactly as the server
 * treats it. List values go through validateRepairList instead.
 */
export function validateRepairValue(spec: FieldSpec, value: string, kind: string): FieldProblem | undefined {
  if (value === '') return spec.field === 'title' && kind !== 'season' ? {key: 'web.metadata.titleRequired'} : undefined;
  if (spec.field === 'title' && !value.trim()) return {key: 'web.metadata.titleRequired'};
  switch (spec.type) {
    case 'integer':
    case 'number': {
      if (!CANONICAL_INT.test(value)) return {key: 'web.metadata.wholeNumber', params: {label: spec.label}};
      const n = Number(value);
      if (!Number.isSafeInteger(n)) return {key: 'web.metadata.wholeNumber', params: {label: spec.label}};
      if ((spec.min != null && n < spec.min) || (spec.max != null && n > spec.max)) {
        if (spec.min != null && spec.max != null) return {key: 'web.metadata.numberRange', params: {label: spec.label, min: spec.min, max: spec.max}};
        return {key: 'web.metadata.wholeNumber', params: {label: spec.label}};
      }
      return undefined;
    }
    case 'date':
      return isRealISODate(value) ? undefined : {key: 'web.metadata.dateError', params: {label: spec.label}};
    case 'enum':
      return !spec.allowed?.length || spec.allowed.includes(value) ? undefined : {key: 'web.metadata.enumError', params: {label: spec.label}};
    case 'multiline':
    case 'text': {
      const max = spec.maxLength ?? 300;
      return runes(value) > max ? {key: 'web.metadata.textTooLong', params: {label: spec.label, max}} : undefined;
    }
    default:
      return undefined;
  }
}
/** CD-46: list entries are trimmed (empties dropped, like the server), each at most maxLength runes, at most max entries. */
export function validateRepairList(spec: FieldSpec, values: readonly string[]): FieldProblem | undefined {
  const entries = values.map(v => v.trim()).filter(Boolean);
  const entryMax = spec.maxLength ?? 128;
  if (entries.some(v => runes(v) > entryMax)) return {key: 'web.metadata.listEntryTooLong', params: {label: spec.label, max: entryMax}};
  const max = spec.max ?? 64;
  if (entries.length > max) return {key: 'web.metadata.tooManyEntries', params: {label: spec.label, max}};
  return undefined;
}

export function GeneralTab({data, busy, onSave, onClose}: {data: Data; busy: boolean; onSave: (fields: Record<string, FieldEdit>) => Promise<boolean>; onClose: () => void}) {
  const i18n = useI18n();
  const t = i18n.t;
  const schema = data.schema ?? fallbackSchema(data, t);
  const [values, setValues] = useState<Record<string, string>>({});
  const [locks, setLocks] = useState<Record<string, boolean>>({});
  const [automatic, setAutomatic] = useState<Set<string>>(new Set());
  const [unlockedOnPurpose, setUnlockedOnPurpose] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string>();
  // A new revision resets the draft: the saved values are now the truth.
  useEffect(() => { setValues({}); setLocks({}); setAutomatic(new Set()); setUnlockedOnPurpose(new Set()); setError(undefined); }, [data.revision]);
  const listField = (f: string) => schema.find(x => x.field === f)?.type === 'list';
  const saved = (f: string) => (listField(f) ? joinList(data.snapshot.fields[f]?.values, data.snapshot.fields[f]?.value ?? '') : data.snapshot.fields[f]?.value ?? '');
  const current = (f: string) => values[f] ?? saved(f);
  const isLocked = (f: string) => locks[f] ?? data.snapshot.fields[f]?.locked ?? false;
  const edit = (f: string, v: string) => {
    setValues(prev => ({...prev, [f]: v}));
    setAutomatic(prev => { if (!prev.has(f)) return prev; const n = new Set(prev); n.delete(f); return n; });
    // Editing locks the field so the next refresh keeps it, unless the viewer unlocked it on purpose.
    if (!unlockedOnPurpose.has(f)) setLocks(prev => ({...prev, [f]: true}));
  };
  const toggleLock = (f: string) => {
    const next = !isLocked(f);
    setLocks(prev => ({...prev, [f]: next}));
    setUnlockedOnPurpose(prev => { const n = new Set(prev); if (next) n.delete(f); else n.add(f); return n; });
  };
  const restoreAutomatic = (f: string) => { setValues(prev => { const n = {...prev}; delete n[f]; return n; }); setLocks(prev => ({...prev, [f]: false})); setAutomatic(prev => new Set(prev).add(f)); };
  const changes: Record<string, FieldEdit> = {};
  for (const spec of schema) {
    const f = spec.field;
    if (automatic.has(f)) { changes[f] = {useAutomatic: true}; continue; }
    const edit: FieldEdit = {};
    if (values[f] !== undefined && values[f] !== saved(f)) { if (spec.type === 'list') edit.values = splitList(values[f]); else edit.value = values[f]; }
    if (locks[f] !== undefined && locks[f] !== data.snapshot.fields[f]?.locked) edit.locked = locks[f];
    // A list's edit is carried by its entries; once the field is locked that is all that changed.
    if (edit.value !== undefined || edit.values !== undefined || edit.locked !== undefined) changes[f] = edit;
  }
  const pending = Object.keys(changes).length;
  const validate = (): string | undefined => {
    const t = currentI18n().t;
    for (const spec of schema) {
      const edit = changes[spec.field];
      if (!edit) continue;
      // List fields edit through values, never a raw value.
      if (edit.value !== undefined && spec.type !== 'list') {
        const problem = validateRepairValue(spec, edit.value, data.target.kind);
        if (problem) return t(problem.key, problem.params);
      }
      if (edit.values !== undefined) {
        const problem = validateRepairList(spec, edit.values);
        if (problem) return t(problem.key, problem.params);
      }
    }
    return undefined;
  };
  const save = async () => {
    const problem = validate();
    if (problem) { setError(problem); return; }
    setError(undefined);
    // A successful save is the end of the task: the sheet closes, like the old editor.
    if (await onSave(changes)) onClose();
  };
  return (
    <>
      {error ? <Notice tone="error">{error}</Notice> : null}
      <div className={s.fields}>
        {schema.map(spec => {
          const f = spec.field;
          const field = data.snapshot.fields[f];
          const value = current(f);
          const auto = field?.automaticValue ?? '';
          const differs = field && auto !== value && !(spec.type === 'list' && !auto && !value);
          const dirty = changes[f] !== undefined;
          const wide = spec.type === 'multiline' || spec.type === 'list';
          const source = field?.source && field.source !== 'automatic' && field.source !== 'manual' ? providerName(field.source.replace('provider:', ''), t) : undefined;
          return (
            <div key={f} className={cx(s.field, wide && s.wide, dirty && s.dirty)} style={dirty ? {paddingLeft: 8} : undefined}>
              <div className={s.fieldHead}><span className={s.fieldLabel}>{spec.label}</span></div>
              {/* The lock lives inside the control, as in the original editor: it is part of the value, not a separate setting. */}
              {(() => { const lock = <button type="button" className={cx(s.lock, isLocked(f) && s.lockOn)} aria-pressed={isLocked(f)} aria-label={t(isLocked(f) ? 'web.metadata.unlockField' : 'web.metadata.lockField', {label: spec.label})} title={isLocked(f) ? t('web.metadata.lockedTip') : t('web.metadata.unlockedTip')} onClick={() => toggleLock(f)} disabled={busy}><Icon name={isLocked(f) ? 'lock' : 'unlock'} size={15} /></button>;
                return spec.type === 'multiline' ? <div className={s.areaWrap}><TextArea label={spec.label} hideLabel rows={5} value={value} onChange={e => edit(f, e.target.value)} disabled={busy} className={s.area} /><span className={s.areaLock}>{lock}</span></div>
                  : spec.type === 'enum' && spec.allowed ? <div className={s.areaWrap}><Select label={spec.label} hideLabel value={value} options={[{value: '', label: '—'}, ...spec.allowed.map(a => ({value: a, label: a}))]} onChange={e => edit(f, e.target.value)} disabled={busy} /><span className={s.selectLock}>{lock}</span></div>
                  : <Input label={spec.label} hideLabel type={spec.type === 'date' ? 'date' : 'text'} inputMode={spec.type === 'integer' ? 'numeric' : undefined} value={value} onChange={e => edit(f, e.target.value)} disabled={busy} placeholder={spec.type === 'list' ? t('web.metadata.commaSeparated') : undefined} trailing={lock} />; })()}
              <div className={s.fieldHint}>
                {automatic.has(f) ? <span>{t('web.metadata.resetAutomatic')}</span> : differs ? <><span title={auto}>{source ? t('web.metadata.automaticValueFrom', {source, value: auto || '—'}) : t('web.metadata.automaticValue', {value: auto || '—'})}</span><Button variant="link" size="sm" label={t('web.metadata.useIt')} onClick={() => restoreAutomatic(f)} disabled={busy} /></> : source ? <span>{t('web.metadata.fromSource', {source})}</span> : field?.source === 'manual' ? <span>{t('web.metadata.editedHere')}</span> : null}
              </div>
            </div>
          );
        })}
      </div>
      <Footer copy={pending ? t('web.metadata.pendingCount', {count: pending}) : t('web.metadata.noChanges')} busy={busy} primary={pending ? {label: t('web.metadata.saveChanges'), onClick: () => void save()} : undefined} onClose={onClose} />
    </>
  );
}

function Footer({copy, busy, primary, onClose}: {copy: React.ReactNode; busy: boolean; primary?: {label: string; onClick: () => void}; onClose: () => void}) {
  const i18n = useI18n();
  const t = i18n.t;
  return (
    <div className={s.footer}>
      <span className={s.footerCopy}>{copy}</span>
      <div style={{display: 'flex', gap: 8}}>
        <Button variant="ghost" label={primary ? t('action.cancel') : t('action.close')} onClick={onClose} />
        {primary ? <Button variant="primary" label={primary.label} loading={busy} disabled={busy} onClick={primary.onClick} /> : null}
      </div>
    </div>
  );
}

/* ---------- Artwork ---------- */

/** "tmdb" → "TMDB", "fanart" → "Fanart" (WEB-MENU-03: provider names as people write them). */
function providerName(p: string, t: I18n['t']): string {
  const id = p.toLowerCase();
  if (id === 'local') return t('web.metadata.sourceLocal');
  if (id === 'embedded') return t('web.metadata.sourceEmbedded');
  const known: Record<string, string> = {tmdb: 'TMDB', tvdb: 'TVDB', imdb: 'IMDb', fanart: 'Fanart.tv', musicbrainz: 'MusicBrainz', audible: 'Audible', openlibrary: 'Open Library'}; // lint-strings-allow: provider names are proper nouns
  return known[id] ?? (p.length <= 4 ? p.toUpperCase() : p.charAt(0).toUpperCase() + p.slice(1));
}

const ART_ROLES: ReadonlySet<string> = new Set(['poster', 'backdrop', 'cover', 'square', 'thumbnail', 'banner', 'logo', 'still', 'portrait']);
/** A picture's place on the page, in words; a place this app has no word for is called a picture. */
const roleLabel = (role: string, t: I18n['t']): string => t('web.metadata.artRole', {role: ART_ROLES.has(role) ? role : 'other'});
const roleShape = (role: string): 'poster' | 'landscape' | 'square' | 'circle' => (role === 'poster' ? 'poster' : role === 'cover' || role === 'square' ? 'square' : role === 'portrait' ? 'circle' : 'landscape');

function ArtworkTab({data, busy, run}: {data: Data; busy: boolean; run: (c: Parameters<MetadataRepairService['command']>[0]) => Promise<boolean>}) {
  const i18n = useI18n();
  const t = i18n.t;
  // WEB-MENU-03: the tabs are the entity kind's artwork roles, the same way the
  // Apple editor does; a role something already uses stays offered.
  const roles = useMemo(() => Array.from(new Set([...(data.artworkRoles ?? []), ...data.snapshot.artwork.map(a => a.role), ...data.artwork.candidates.map(c => c.role)])), [data]);
  const [role, setRole] = useState<string>(roles[0] ?? 'poster');
  useEffect(() => { if (!roles.includes(role) && roles[0]) setRole(roles[0]); }, [roles, role]);
  const chosen = data.snapshot.artwork.find(a => a.role === role);
  const inUse = (id: string) => chosen?.candidateId === id;
  const candidates = data.artwork.candidates.filter(c => c.role === role).sort((a, b) => Number(inUse(b.id)) - Number(inUse(a.id)) || b.rank - a.rank);
  const jobs = data.artwork.jobs.filter(j => j.role === role);
  return (
    <>
      <div className={s.artRoles}>{roles.map(r => <Button key={r} size="sm" variant={r === role ? 'secondary' : 'ghost'} selected={r === role} label={roleLabel(r, t)} onClick={() => setRole(r)} />)}</div>
      <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, flexWrap: 'wrap'}}>
        <Text variant="caption" tone="tertiary">{[t('web.metadata.candidateCount', {count: candidates.length}), chosen ? t(chosen.locked ? 'web.metadata.locked' : 'web.metadata.unlocked') : ''].filter(Boolean).join(' · ')}</Text>
        <div style={{display: 'flex', gap: 8}}>
          {chosen ? <Button size="sm" variant="ghost" icon={chosen.locked ? 'lock' : 'unlock'} label={chosen.locked ? t('web.metadata.unlock') : t('web.metadata.lock')} disabled={busy} onClick={() => void run({action: 'artwork_lock', role, subject: chosen.subject, locked: !chosen.locked, confirm: true})} /> : null}
          <Button size="sm" variant="ghost" icon="search" label={t('web.metadata.findMore')} disabled={busy} onClick={() => void run({action: 'discover_artwork'})} />
          <Button size="sm" variant="ghost" icon="refresh" label={t('web.metadata.refetchChosen')} disabled={busy} onClick={() => void run({action: 'repair_assets'})} />
        </div>
      </div>
      {jobs.filter(j => j.status !== 'complete' && j.status !== 'completed').length ? <Notice tone="info" compact>{jobs.filter(j => j.status !== 'complete' && j.status !== 'completed').map(j => t('web.metadata.artJob', {role: roleLabel(j.role, t), state: j.error || j.status === 'failed' ? 'failed' : 'working'})).join(' · ')}</Notice> : null}
      <div className={s.artGrid}>
        {candidates.map(c => (
          <div key={c.id} className={cx(s.artCard, inUse(c.id) && s.current)}>
            {/* WEB-MENU-03: every candidate shows its picture through Artwork, which fetches it lazily as the tile scrolls into view. */}
            <Artwork path={c.previewUrl ?? (inUse(c.id) && chosen?.url ? chosen.url : candidateArtworkPath(data.target, c))} shape={roleShape(c.role)} alt={t('web.metadata.artAlt', {provider: providerName(c.provider, t), role: roleLabel(c.role, t)})} fit={c.role === 'logo' ? 'contain' : undefined} />
            {/* Part 2.1: provider · language · votes; votes omitted when null. */}
            <div className={s.artMeta} title={c.attribution}>{[providerName(c.provider, t), c.locale ? i18n.languageName(c.locale) : '', c.votes != null ? t('web.metadata.votes', {count: c.votes}) : ''].filter(Boolean).join(' · ')}</div>
            <div className={s.artActions}>
              {inUse(c.id) ? <Text variant="caption" tone="accent">{t('web.metadata.inUse')}</Text> : <Button size="sm" variant="secondary" label={t('web.liveSource.useThis')} disabled={busy} onClick={() => void run({action: 'select_artwork', candidateId: c.id, confirm: true})} />}
            </div>
          </div>
        ))}
      </div>
      {!candidates.length ? <Text variant="body" tone="secondary">{t('web.metadata.noCandidates')}</Text> : null}
    </>
  );
}

/**
 * WEB-MENU-03: the artwork route previews an unapplied provider candidate for
 * owners (`?candidate=`); `size=thumbnail` asks for the small variant. Pending
 * artwork answers 404 with `Retry-After`, which the Artwork store retries.
 */
export function candidateArtworkPath(target: Pick<RepairTarget, 'kind' | 'id'>, candidate: {role: string; subject: string; id: string}): string {
  const q = new URLSearchParams({candidate: candidate.id, size: 'thumbnail'});
  if (candidate.subject) q.set('subject', candidate.subject);
  return `/v1/metadata/${encodeURIComponent(target.kind)}/${encodeURIComponent(target.id)}/art/${encodeURIComponent(candidate.role)}?${q.toString()}`;
}

/* ---------- Genres ---------- */

function GenresTab({data, busy, run}: {data: Data; busy: boolean; run: (c: Parameters<MetadataRepairService['command']>[0]) => Promise<boolean>}) {
  const i18n = useI18n();
  const t = i18n.t;
  const saved = useMemo(() => data.snapshot.relationships.filter(r => r.kind === 'genre').sort((a, b) => a.ordinal - b.ordinal), [data]);
  const [list, setList] = useState<RepairRelationship[]>(saved);
  const [draft, setDraft] = useState('');
  useEffect(() => setList(saved), [saved]);
  const locked = data.snapshot.relationshipLocks.genre ?? false;
  const dirty = JSON.stringify(list.map(g => g.label)) !== JSON.stringify(saved.map(g => g.label));
  const add = () => {
    const name = draft.trim();
    if (!name || list.some(g => g.label.toLowerCase() === name.toLowerCase())) { setDraft(''); return; }
    setList(prev => [...prev, {kind: 'genre', targetKind: 'genre', targetId: '', label: name, ordinal: prev.length, source: 'manual', locked: true}]);
    setDraft('');
  };
  if (data.target.kind !== 'item') return <Text variant="body" tone="secondary">{t('web.metadata.genresOnlyItems')}</Text>;
  return (
    <>
      <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8}}>
        <Text variant="heading">{t('web.metadata.genres')}</Text>
        <Button size="sm" variant="ghost" icon={locked ? 'lock' : 'unlock'} label={locked ? t('web.metadata.locked') : t('web.metadata.unlocked')} disabled={busy} onClick={() => void run({action: 'relationship_lock', role: 'genre', locked: !locked, confirm: true})} />
      </div>
      <div className={s.chips}>
        {list.map((g, i) => <span key={`${g.label}:${i}`} className={cx(s.chip, g.locked && s.locked)}>{g.label}<button type="button" aria-label={t('web.metadata.removeLabel', {label: g.label})} onClick={() => setList(prev => prev.filter((_, n) => n !== i).map((x, n) => ({...x, ordinal: n})))} disabled={busy}><Icon name="close" size={12} /></button></span>)}
        {!list.length ? <Text variant="caption" tone="tertiary">{t('web.metadata.noGenres')}</Text> : null}
      </div>
      <Input label={t('web.metadata.addGenre')} placeholder={t('web.metadata.addGenreHelp')} value={draft} onChange={e => setDraft(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); add(); } }} onBlur={add} disabled={busy} />
      <Footer copy={t(dirty ? 'web.metadata.genresChanged' : locked ? 'web.metadata.genresLocked' : 'web.metadata.genresUnlocked')} busy={busy} primary={dirty ? {label: t('web.metadata.saveGenres'), onClick: () => void run({action: 'edit_relationships', role: 'genre', relationships: list.map((g, n) => ({...g, ordinal: n})), confirm: true})} : undefined} onClose={() => setList(saved)} />
    </>
  );
}

/* ---------- Credits ---------- */

function CreditsTab({data, busy, run}: {data: Data; busy: boolean; run: (c: Parameters<MetadataRepairService['command']>[0]) => Promise<boolean>}) {
  const i18n = useI18n();
  const t = i18n.t;
  const saved = useMemo(() => data.snapshot.relationships.filter(r => r.kind === 'credit').sort((a, b) => a.ordinal - b.ordinal), [data]);
  const [rows, setRows] = useState<RepairRelationship[]>(saved);
  useEffect(() => setRows(saved), [saved]);
  const locked = data.snapshot.relationshipLocks.credit ?? false;
  const dirty = JSON.stringify(rows.map(r => [r.label, r.role, r.department])) !== JSON.stringify(saved.map(r => [r.label, r.role, r.department]));
  const update = (i: number, patch: Partial<RepairRelationship>) => setRows(prev => prev.map((r, n) => (n === i ? {...r, ...patch} : r)));
  if (data.target.kind !== 'item') return <Text variant="body" tone="secondary">{t('web.metadata.creditsOnlyItems')}</Text>;
  return (
    <>
      <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8}}>
        <Text variant="heading">{t('web.metadata.tabPeople')}</Text>
        <div style={{display: 'flex', gap: 8}}>
          <Button size="sm" variant="ghost" icon={locked ? 'lock' : 'unlock'} label={locked ? t('web.metadata.locked') : t('web.metadata.unlocked')} disabled={busy} onClick={() => void run({action: 'relationship_lock', role: 'credit', locked: !locked, confirm: true})} />
          <Button size="sm" variant="secondary" icon="plus" label={t('web.metadata.addPerson')} disabled={busy} onClick={() => setRows(prev => [...prev, {kind: 'credit', targetKind: 'scoped_credit', targetId: '', label: '', role: '', department: 'Acting', ordinal: prev.length, source: 'manual', locked: true}])} />
        </div>
      </div>
      <div className={s.credits}>
        {rows.map((r, i) => (
          <div key={`${r.recordId ?? 'new'}:${i}`} className={s.creditRow}>
            <Input label={t('profile.name')} value={r.label} onChange={e => update(i, {label: e.target.value})} disabled={busy} />
            <Input label={t('web.metadata.department')} value={r.department ?? ''} onChange={e => update(i, {department: e.target.value})} disabled={busy} placeholder={t('web.metadata.departmentExample')} />
            <Input label={r.department?.toLowerCase() === 'acting' ? t('web.metadata.characterLabel') : t('web.metadata.roleLabel')} value={r.role ?? ''} onChange={e => update(i, {role: e.target.value})} disabled={busy} />
            <Button variant="ghost" icon="trash" aria-label={t('web.metadata.removeLabel', {label: r.label || 'row'})} disabled={busy} onClick={() => setRows(prev => prev.filter((_, n) => n !== i))} />
          </div>
        ))}
        {!rows.length ? <Text variant="caption" tone="tertiary">{t('web.metadata.noCredits')}</Text> : null}
      </div>
      <Footer copy={dirty ? t('web.metadata.creditsChanged') : t('web.metadata.creditCount', {count: rows.length})} busy={busy} primary={dirty ? {label: t('web.metadata.saveCredits'), onClick: () => void run({action: 'edit_relationships', role: 'credit', relationships: rows.filter(r => r.label.trim()).map((r, n) => ({...r, label: r.label.trim(), ordinal: n})), confirm: true})} : undefined} onClose={() => setRows(saved)} />
    </>
  );
}

/* ---------- Matching ---------- */

/** CD-48: only the screen engine takes an owner-supplied query/year. MusicBrainz
 * search/retry omits both and uses scanned metadata; artists/books without a
 * remote matcher offer no working search. A typed or stale query is omitted
 * after switching away from screen. */
export function matchingSearchCommand(identity: Data['snapshot']['identity'], query: string): {action: 'search'; query?: string} {
  const trimmed = query.trim() || undefined;
  if (identity?.engine === 'screen') return trimmed ? {action: 'search', query: trimmed} : {action: 'search'};
  return {action: 'search'};
}

/** M25-1b: every identity status maps to catalogue copy, never a raw internal
 * value (e.g. needs_selection). Unknown future values fall back to a generic
 * catalogue string. */
export function matchingStatusKey(status: string | undefined): MessageId {
  switch (status) {
    case 'searching': return 'web.metadata.matchStatus.searching';
    case 'pending': return 'web.metadata.matchStatus.pending';
    case 'pending_search': return 'web.metadata.matchStatus.pendingSearch';
    case 'pending_episodes': return 'web.metadata.matchStatus.pendingEpisodes';
    case 'pending_children': return 'web.metadata.matchStatus.pendingChildren';
    case 'pending_apply': return 'web.metadata.matchStatus.pendingApply';
    case 'matched': return 'web.metadata.matchStatus.matched';
    case 'matched_work': return 'web.metadata.matchStatus.matchedWork';
    case 'accepted': return 'web.metadata.matchStatus.accepted';
    case 'published': return 'web.metadata.matchStatus.published';
    case 'complete': return 'web.metadata.matchStatus.complete';
    case 'needs_selection': return 'web.metadata.matchStatus.needsSelection';
    case 'needs_order': return 'web.metadata.matchStatus.needsOrder';
    case 'needs_parent_match': return 'web.metadata.matchStatus.needsParentMatch';
    case 'needs_consent': return 'web.metadata.matchStatus.needsConsent';
    case 'needs_season_mapping': return 'web.metadata.matchStatus.needsSeasonMapping';
    case 'unmatched': return 'web.metadata.matchStatus.unmatched';
    case 'unresolved': return 'web.metadata.matchStatus.unresolved';
    case 'unavailable': return 'web.metadata.matchStatus.unavailable';
    case 'source_unavailable': return 'web.metadata.matchStatus.sourceUnavailable';
    case 'provider_disabled': return 'web.metadata.matchStatus.providerDisabled';
    case 'provider_unavailable': return 'web.metadata.matchStatus.providerUnavailable';
    case 'manual_preserved': return 'web.metadata.matchStatus.manualPreserved';
    case 'delegated_tvdb': return 'web.metadata.matchStatus.delegatedTvdb';
    case 'identity_conflict': return 'web.metadata.matchStatus.identityConflict';
    default: return 'web.metadata.matchStatus.unknown';
  }
}

/** M25-1a: transient provider-work states that settle without another owner
 * action. A search/retry that returns one of these is reloaded until it
 * leaves the set (bounded in the caller). */
export function isTransientMatchingStatus(status: string | undefined): boolean {
  return status === 'searching' || status === 'pending' || status === 'pending_search' || status === 'pending_episodes' || status === 'pending_children' || status === 'pending_apply';
}

function MatchingTab({data, busy, run}: {data: Data; busy: boolean; run: (c: Parameters<MetadataRepairService['command']>[0]) => Promise<boolean>}) {
  const i18n = useI18n();
  const t = i18n.t;
  const identity = data.snapshot.identity;
  const [query, setQuery] = useState('');
  const identityLocked = data.snapshot.relationshipLocks.identity ?? identity?.locked ?? false;
  const isScreen = identity?.engine === 'screen';
  const providerName = (p: string) => ({tmdb: 'TMDB', tvdb: 'TheTVDB', musicbrainz: 'MusicBrainz', anilist: 'AniList'} as Record<string, string>)[p] ?? p;
  // Which episode order a match uses, and what is added to each file's own number when the files are numbered straight through.
  const [orders, setOrders] = useState<Record<string, {order: string; offset: number}>>({});
  const stored = readTmdbOrder(identity?.order ?? '');
  const orderName = (id: string) => data.candidates.flatMap(c => c.orders ?? []).find(o => o.id === id)?.name ?? '';
  const storedName = orderName(stored.base);
  const orderLine = storedName ? (stored.offset ? t('web.metadata.orderWithOffset', {order: storedName, offset: stored.offset}) : storedName) : '';
  return (
    <>
      <div style={{display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8, flexWrap: 'wrap'}}>
        <div style={{display: 'flex', flexDirection: 'column', gap: 4}}>
          <Text variant="heading">{t('web.metadata.currentMatch')}</Text>
          {identity ? <Text variant="body" tone="secondary">{providerName(identity.provider)} {identity.type ? `${identity.type} ` : ''}{identity.id || t('web.metadata.notMatched')}{orderLine ? ` · ${orderLine}` : ''} · {t(matchingStatusKey(identity.status))}</Text> : <Text variant="body" tone="secondary">{t('web.metadata.noIdentityFor', {kind: data.target.kind})}</Text>}
        </div>
        {identity ? <Button size="sm" variant="ghost" icon={identityLocked ? 'lock' : 'unlock'} label={t(identityLocked ? 'web.metadata.identityLocked' : 'web.metadata.identityUnlocked')} disabled={busy} onClick={() => void run({action: 'relationship_lock', role: 'identity', locked: !identityLocked, confirm: true})} /> : null}
      </div>
      <div style={{display: 'flex', gap: 8, alignItems: 'flex-end'}}>
        <div style={{flex: 1}}><Input label={t('web.metadata.searchProvider')} placeholder={t(isScreen ? 'web.metadata.searchProviderHelp' : 'web.metadata.searchProviderHelpAuto')} value={query} onChange={e => setQuery(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); void run(matchingSearchCommand(identity, query)); } }} disabled={busy || !isScreen} /></div>
        <Button variant="secondary" icon="search" label={t('web.search.title')} disabled={busy || !identity} onClick={() => void run(matchingSearchCommand(identity, query))} />
        <Button variant="ghost" icon="refresh" label={t('action.tryAgain')} disabled={busy || !identity} onClick={() => void run({action: 'retry'})} />
      </div>
      <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
        {data.candidates.map(c => {
          const current = identity?.id === c.id && identity.provider === c.provider;
          const key = `${c.provider}:${c.id}`;
          const offered = c.orders ?? [];
          // The order a match starts with: the one it already has when it is the current match, otherwise the provider's first.
          const chosen = orders[key] ?? (current && offered.some(o => o.id === stored.base) ? {order: stored.base, offset: stored.offset} : {order: offered[0]?.id ?? '', offset: 0});
          const takesOffset = c.provider === 'tmdb' && orderTakesOffset(chosen.order);
          const order = chosen.order ? (takesOffset ? tmdbAbsoluteOrder(chosen.order, chosen.offset) : chosen.order) : '';
          const set = (next: Partial<{order: string; offset: number}>) => setOrders(prev => ({...prev, [key]: {...chosen, ...next}}));
          const identify = () => void run({action: 'identify', provider: c.provider, candidateId: c.id, ...(order ? {order} : {}), confirm: true});
          return (
            <div key={key} className={cx(s.candidate, current && s.current)}>
              <div className={s.candidateCopy}>
                <Text variant="bodyStrong">{c.title || c.id}</Text>
                <Text variant="caption" tone="tertiary">{[providerName(c.provider), c.subtitle].filter(Boolean).join(' · ')}</Text>
                {offered.length ? (
                  <div style={{display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'flex-start', marginTop: 8}}>
                    {offered.length > 1 ? <Select label={t('web.metadata.episodeOrder')} value={chosen.order} disabled={busy} onChange={e => set({order: e.target.value})} options={offered.map(o => ({value: o.id, label: o.name}))} /> : <Text variant="caption" tone="tertiary">{t('web.metadata.orders', {names: offered[0]!.name})}</Text>}
                    {takesOffset ? <Input label={t('web.metadata.numberOffset')} help={t('web.metadata.numberOffsetHelp')} type="number" inputMode="numeric" min={-1000000} max={1000000} step={1} value={String(chosen.offset)} disabled={busy} onChange={e => { const n = Math.trunc(Number(e.target.value)); if (Number.isSafeInteger(n) && Math.abs(n) <= 1000000) set({offset: n}); }} style={{width: 140}} /> : null}
                  </div>
                ) : null}
              </div>
              {current
                ? (order && order !== identity?.order && offered.length ? <Button size="sm" variant="secondary" label={t('web.metadata.useOrder')} disabled={busy} onClick={identify} /> : <Text variant="caption" tone="accent">{t('profile.current')}</Text>)
                : <Button size="sm" variant="secondary" label={t('web.metadata.useMatch')} disabled={busy} onClick={identify} />}
            </div>
          );
        })}
        {!data.candidates.length ? <Text variant="body" tone="secondary">{t(isScreen ? 'web.metadata.noCandidatesMatch' : 'web.metadata.noCandidatesMatchAuto')}</Text> : null}
      </div>
      {data.cascades.length ? <Notice tone="info" compact>{data.cascades.map(c => t('web.metadata.cascadeProgress', {processed: c.processed, failed: c.failed})).join(' · ')}</Notice> : null}
    </>
  );
}

/* ---------- History ---------- */

const HISTORY_TRIGGERS: ReadonlySet<string> = new Set(['edit', 'lock_all', 'unlock_all', 'relationship_lock', 'edit_relationships', 'identify', 'undo', 'select_artwork', 'artwork_lock', 'repair_assets', 'cascade', 'search', 'retry']);

function HistoryTab({data, busy, run}: {data: Data; busy: boolean; run: (c: Parameters<MetadataRepairService['command']>[0]) => Promise<boolean>}) {
  const i18n = useI18n();
  const t = i18n.t;
  const label = (trigger: string) => t('web.metadata.history', {trigger: HISTORY_TRIGGERS.has(trigger) ? trigger : 'other'});
  return (
    <>
      <Text variant="heading">{t('web.metadata.recentChanges')}</Text>
      <div>
        {data.history.map(h => (
          <div key={h.id} className={s.historyRow}>
            <div style={{display: 'flex', flexDirection: 'column', gap: 2}}><Text variant="body">{label(h.trigger)}</Text><Text variant="caption" tone="tertiary">{i18n.dateTime(h.observedAt)}</Text></div>
            <Button size="sm" variant="ghost" icon="back" label={t('web.metadata.undoToHere')} disabled={busy} onClick={() => void run({action: 'undo', historyId: h.id, confirm: true})} />
          </div>
        ))}
        {!data.history.length ? <Text variant="body" tone="secondary">{t('web.metadata.noHistory')}</Text> : null}
      </div>
      <div style={{display: 'flex', gap: 8}}>
        <Button size="sm" variant="ghost" icon="lock" label={t('web.metadata.lockAll')} disabled={busy} onClick={() => void run({action: 'lock_all', confirm: true})} />
        <Button size="sm" variant="ghost" icon="unlock" label={t('web.metadata.unlockAll')} disabled={busy} onClick={() => void run({action: 'unlock_all', confirm: true})} />
      </div>
    </>
  );
}

/* ---------- Bulk ---------- */

type Outcome = {id: string; ok: boolean; message?: string};

// X-04: bulk batch failures are catalogue copy by code, never `error.message`.
const batchFailureMessage = (e: unknown): string => errorText(e, 'generic', 'save');

/** CD-44: chunk explicit metadata targets at the shared bulk limit. */
export function chunkBulkTargets<T>(targets: readonly T[]): readonly (readonly T[])[] {
  const out: (readonly T[])[] = [];
  for (let i = 0; i < targets.length; i += bulkTargetLimit) out.push(targets.slice(i, i + bulkTargetLimit));
  return out;
}

/** Group metadata targets into job selectors (M25-4: shared helper in
 * client-core `bulk-jobs.ts`, used from web and Apple). */
function BulkEditor({targets, onSaved, onClose}: {targets: readonly RepairTarget[]; onSaved?: () => void; onClose: () => void}) {
  const {api} = useSession();
  const i18n = useI18n();
  const t = i18n.t;
  const scope = useViewerScope();
  const kinds = new Set(targets.map(t => t.kind));
  const [values, setValues] = useState<Record<string, string>>({});
  const [lockEdited, setLockEdited] = useState(true);
  const [genresAdd, setGenresAdd] = useState('');
  const [genresRemove, setGenresRemove] = useState('');
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<Outcome[]>();
  const [error, setError] = useState<string>();
  const [schema, setSchema] = useState<FieldSpec[]>();
  // The first target's schema decides which fields bulk mode offers; all targets share a kind.
  useEffect(() => {
    let cancelled = false;
    const probe = new MetadataRepairService({api, scope});
    void probe.load(targets[0]).then(() => { const d = probe.getSnapshot().data as Data | null; if (!cancelled && d) setSchema((d.schema ?? fallbackSchema(d, t)).filter(f => f.bulk)); probe.dispose(); });
    return () => { cancelled = true; probe.dispose(); };
  }, [api, scope, targets, t]);
  if (kinds.size > 1) return <><Notice tone="warning">{t('web.metadata.bulkOneKind')}</Notice><Footer copy="" busy={false} onClose={onClose} /></>;
  const fieldChanges = Object.fromEntries(Object.entries(values).filter(([, v]) => v !== ''));
  const adds = genresAdd.split(',').map(v => v.trim()).filter(Boolean);
  const removes = genresRemove.split(',').map(v => v.trim()).filter(Boolean);
  const pending = Object.keys(fieldChanges).length + (adds.length || removes.length ? 1 : 0);
  const apply = async () => {
    // CD-46: bulk edits meet the same published bounds as single edits, before anything is sent.
    for (const [f, v] of Object.entries(fieldChanges)) {
      const spec = schema?.find(x => x.field === f);
      if (!spec) continue;
      const problem = spec.type === 'list' ? validateRepairList(spec, splitList(v)) : validateRepairValue(spec, v, targets[0].kind);
      if (problem) { setError(t(problem.key, problem.params)); return; }
    }
    const genreSpec: FieldSpec = {field: 'genres', label: t('web.metadata.genres'), type: 'list', bulk: true, maxLength: 128, max: 64};
    for (const g of [adds, removes]) {
      const problem = validateRepairList(genreSpec, g);
      if (problem) { setError(t(problem.key, problem.params)); return; }
    }
    setError(undefined);
    setBusy(true);
    const outcomes: Outcome[] = [];
    try {
      // One metadata-edit job per selector group. Item targets go out
      // as an items selector (200 per job); each container target (show,
      // season, album, artist, book) is its own container selector. No client
      // enumeration or per-item revisions: the server captures metadata
      // revision fences with membership, and a later edit surfaces as a
      // metadata_conflict per-item failure.
      const fields: Record<string, BulkFieldEdit> = {};
      for (const [f, v] of Object.entries(fieldChanges)) fields[f] = schema?.find(x => x.field === f)?.type === 'list' ? {values: splitList(v)} : {value: v};
      const args = {...(Object.keys(fields).length ? {fields} : {}), ...(adds.length || removes.length ? {genres: {...(adds.length ? {add: adds} : {}), ...(removes.length ? {remove: removes} : {})}} : {}), lockEdited};
      const groups = groupMetadataTargets(targets, () => crypto.randomUUID());
      for (const group of groups) {
        const ids = [...group.ids];
        try {
          const outcome = await runBulkRequests(api, 'metadata-edit', args, [group], (p) => {
            void p;
          });
          const failedById = new Map(outcome.failed.map(f => [f.itemId, f.code]));
          for (const id of ids) {
            const code = failedById.get(id);
            outcomes.push(code ? {id, ok: false, message: errorText({code}, 'generic', 'save')} : {id, ok: true});
          }
        } catch (e) {
          const message = batchFailureMessage(e);
          for (const id of ids) if (!outcomes.some(o => o.id === id)) outcomes.push({id, ok: false, message});
        }
      }
    } catch (e) {
      const message = batchFailureMessage(e);
      for (const t of targets) if (!outcomes.some(o => o.id === t.id)) outcomes.push({id: t.id, ok: false, message});
    }
    setDone(outcomes);
    setBusy(false);
    onSaved?.();
  };
  if (done) {
    const failed = done.filter(o => !o.ok);
    return (
      <>
        <Notice tone={failed.length ? 'warning' : 'success'} title={t('web.metadata.bulkTitle', {ok: done.length - failed.length, failed: failed.length})}>{failed.length ? failed.map(f => f.message).filter((m, i, a) => a.indexOf(m) === i).join(' ') : t('web.metadata.bulkAllUpdated')}</Notice>
        <Footer copy="" busy={false} onClose={onClose} />
      </>
    );
  }
  return (
    <>
      {!schema ? <Loading label={t('web.metadata.loadingFields')} /> : null}
      {schema ? (
        <div className={s.fields}>
          {schema.map(spec => (
            <div key={spec.field} className={cx(s.field, spec.type === 'multiline' && s.wide)}>
              <div className={s.fieldHead}><span className={s.fieldLabel}>{spec.label}</span></div>
              {spec.type === 'multiline' ? <TextArea label={spec.label} hideLabel rows={4} value={values[spec.field] ?? ''} onChange={e => setValues(p => ({...p, [spec.field]: e.target.value}))} placeholder={t('web.metadata.mixedLeave')} disabled={busy} />
                : <Input label={spec.label} hideLabel value={values[spec.field] ?? ''} onChange={e => setValues(p => ({...p, [spec.field]: e.target.value}))} placeholder={t('web.metadata.mixedLeave')} inputMode={spec.type === 'integer' ? 'numeric' : undefined} disabled={busy} />}
            </div>
          ))}
          {targets[0].kind === 'item' ? (
            <>
              <div className={s.field}><Input label={t('web.metadata.addGenres')} placeholder={t('web.metadata.commaSeparated')} value={genresAdd} onChange={e => setGenresAdd(e.target.value)} disabled={busy} /></div>
              <div className={s.field}><Input label={t('web.metadata.removeGenres')} placeholder={t('web.metadata.commaSeparated')} value={genresRemove} onChange={e => setGenresRemove(e.target.value)} disabled={busy} /></div>
            </>
          ) : null}
        </div>
      ) : null}
      <label style={{display: 'flex', alignItems: 'center', gap: 8, fontSize: 13}}><input type="checkbox" checked={lockEdited} onChange={e => setLockEdited(e.target.checked)} disabled={busy} /> {t('web.metadata.lockEdited')}</label>
      {error ? <Notice tone="error">{error}</Notice> : null}
      <Footer copy={pending ? t('web.metadata.pendingChanges', {pending, count: targets.length}) : t('web.metadata.fillHint')} busy={busy} primary={pending ? {label: t('web.metadata.applyTo', {count: targets.length}), onClick: () => void apply()} : undefined} onClose={onClose} />
      {busy ? <div style={{display: 'flex', alignItems: 'center', gap: 8}}><Spinner /><Text variant="caption" tone="tertiary">{t('web.metadata.applying')}</Text></div> : null}
    </>
  );
}
