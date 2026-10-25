import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {MetadataRepairService, type RepairCommand, type RepairData, type RepairFieldSpec, type RepairTarget} from '@core/metadata-repair.ts';
import {EditorHeader} from '../../kit/shell';
import {Button, TextField} from '../../kit/controls';
import {providerName} from '@core/server-admin/attention-words.ts';
import {HostedError} from '../../kit/accounts/ServicesAuth';
import styles from './metadata.module.css';
import {metadataDraftValue, patchMetadataDraft, type MetadataFieldDraft} from './draft-value';

const t = defaultI18n.t;
type Api = Readonly<{request: <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<T>}>;
type Draft = Readonly<Record<string, MetadataFieldDraft>>;
type Tab = 'general' | 'artwork' | 'tags' | 'people' | 'matching' | 'history';

/** Only the tabs this type really has: no list fields means no Tags tab, no roles means no Artwork. */
function tabsFor(data: RepairData | null | undefined): readonly Tab[] {
  const tabs: Tab[] = ['general'];
  if (!data || data.artworkRoles.length) tabs.push('artwork');
  if (!data || data.schema.some(spec => spec.type === 'list')) tabs.push('tags');
  // Only a type the server actually matches online has a Matching tab (frame 10); a local-only
  // book has none because there is nothing to match against (frame 13).
  // Frame 9: credits exist only where the type carries them; an empty list is not a tab.
  if (data && (data.snapshot.relationships.some(row => row.kind === 'credit') || Object.hasOwn(data.snapshot.relationshipLocks, 'credit'))) tabs.push('people');
  if (data && (data.snapshot.identity || data.candidates.length)) tabs.push('matching');
  tabs.push('history');
  return tabs;
}

/** owner-metadata frames 6-8: one editor for a title, an episode, a show, an album or a book.
 * Every field and every choice comes from the server's schema and current snapshot; a change is
 * exactly one edit command carrying only the fields the owner touched. */
export function MetadataEditor({api, scope, target, onClose}: {
  api: Api; scope: Readonly<{serverId: string; viewerId: string}>; target: RepairTarget; onClose: () => void;
}) {
  const service = useMemo(() => new MetadataRepairService({api, scope}), [api, scope]);
  const state = useSyncExternalStore(service.subscribe, service.getSnapshot);
  const [tab, setTab] = useState<Tab>('general'), [draft, setDraft] = useState<Draft>({}), [error, setError] = useState<unknown>();
  useEffect(() => {void service.load(target); return () => service.dispose();}, [service, target]);
  const data = state.data;
  const offered = tabsFor(data);
  const shown: Tab = offered.includes(tab) ? tab : 'general';
  const invalid = data ? Object.entries(draft).some(([name, value]) => {const spec = data.schema.find(row => row.field === name); return spec ? Boolean(fieldError(spec, value.value, value.values)) : false;}) : false;
  const changed = useMemo(() => {
    if (!data) return [];
    return Object.entries(draft).filter(([field, value]) => {
      const original = data.snapshot.fields[field];
      const list = data.schema.find(spec => spec.field === field)?.type === 'list';
      return value.locked !== (original?.locked ?? false) || (list ? JSON.stringify(value.values ?? original?.values ?? []) !== JSON.stringify(original?.values ?? []) : value.value !== metadataDraftValue(data, field));
    }).map(([field, value]) => ({field, ...value}));
  }, [data, draft]);
  const edit = async (fields: NonNullable<RepairCommand['fields']>) => {
    if (!data) return;
    setError(undefined);
    await service.command({action: 'edit', fields});
  };
  const run = async (command: RepairCommand) => {
    setError(undefined);
    const saved = await service.command(command);
    if (!saved) setError(service.getSnapshot().error ?? new Error(t('web.metadata.saveChanges')));
    return saved;
  };
  const set = (field: string, patch: Partial<MetadataFieldDraft>) => {
    setDraft(current => ({...current, [field]: patchMetadataDraft(data, field, current[field], patch)}));
  };
  const save = () => {
    if (!changed.length) return;
    const fields: NonNullable<RepairCommand['fields']> = {};
    for (const item of changed) {
      const spec = data?.schema.find(row => row.field === item.field);
      fields[item.field] = {...(spec?.type === 'list' ? {values: item.values ?? data?.snapshot.fields[item.field]?.values ?? []} : {value: item.value}), locked: item.locked};
    }
    void edit(fields);
  };
  const heading = data ? (data.snapshot.fields.title?.value || target.id) : '';
  const kind = target.kind === 'item' ? data?.schema.some(field => field.field === 'episodeNumber') ? 'episode' : 'film' : target.kind;
  return <aside className={styles.editor} aria-label={t('web.metadata.sections')}>
    <EditorHeader audience={{authority: 'owner'}} title={heading ? t('web.metadata.editTitle', {title: heading}) : t('web.metadata.editFallbackTitle')} description={t('web.metadata.editorLede', {kind})} onClose={onClose} className={styles.editorHead}/>
    <nav className={styles.editorTabs} aria-label={t('web.metadata.sections')}>
      {tabsFor(data).map(id => <button key={id} type="button" aria-current={shown === id ? 'page' : undefined} onClick={() => setTab(id)}>
        {t(id === 'general' ? 'web.metadata.tabGeneral' : id === 'artwork' ? 'web.metadata.tabArtwork' : id === 'tags' ? 'web.metadata.tabTags' : id === 'people' ? 'web.metadata.tabPeople' : id === 'matching' ? 'web.metadata.tabMatching' : 'web.metadata.tabHistory')}</button>)}
    </nav>
    {state.phase === 'loading' ? <p className={styles.working}>{t('web.metadata.loading')}</p> : null}
    {state.error ? <HostedError error={state.error} /> : null}
    {error ? <HostedError error={error} /> : null}
    {state.phase === 'saving' ? <p className={styles.working}>{t('web.metadata.applying')}</p> : null}
    {data && state.phase !== 'loading' ? <>
      {shown === 'general' ? <General data={data} draft={draft} set={set} /> : null}
      {shown === 'tags' ? <Tags data={data} draft={draft} set={set} busy={state.phase !== 'ready'} /> : null}
      {shown === 'people' ? <People data={data} target={target} busy={state.phase !== 'ready'}
        onCommit={rows => run({action: 'edit_relationships', role: 'credit', confirm: true, relationships: rows})}
        onLock={locked => void run({action: 'relationship_lock', role: 'credit', locked})} /> : null}
      {shown === 'matching' ? <Matching data={data} target={target} busy={state.phase !== 'ready'} onSearch={query => void run({action: 'search', query})}
        onRetry={() => void run({action: 'retry'})} onLock={locked => void run({action: 'relationship_lock', role: 'identity', locked})}
        onUse={(provider, candidateId, order) => void run({action: 'identify', provider, candidateId, ...(order ? {order} : {})})} /> : null}
      {shown === 'artwork' ? <Artwork data={data} busy={state.phase !== 'ready'} onUse={(role, candidateId) => void run({action: 'select_artwork', role, candidateId})}
        onLock={(role, subject, locked) => void run({action: 'artwork_lock', role, subject, locked})}
        onFindMore={role => void run({action: 'discover_artwork', role})} onRefetch={role => void run({action: 'repair_assets', role})} /> : null}
      {shown === 'history' ? <History data={data} busy={state.phase !== 'ready'} onUndo={historyId => void run({action: 'undo', historyId})}
        onAll={locked => void run({action: locked ? 'lock_all' : 'unlock_all'})} /> : null}
      {shown === 'general' || shown === 'tags' ? <footer className={styles.editorFooter}>
        <span className={styles.pending}>{t(changed.length ? 'web.metadata.pendingCount' : 'web.metadata.noChanges', {count: changed.length})}</span><Button size="small" onClick={onClose}>{t('action.cancel')}</Button>
        <Button size="small" variant="primary" loading={state.phase === 'saving'} disabled={!changed.length || invalid || state.phase !== 'ready'} onClick={save}>
          {t('web.metadata.saveChanges')}{changed.length ? ` (${changed.length})` : ''}</Button>
      </footer> : null}
    </> : null}
  </aside>;
}


/** The server's own bounds, checked beside the field as the frame draws them. */
function fieldError(spec: RepairFieldSpec, value: string, values: readonly string[] = []): string {
  const label = t(spec.label as Parameters<typeof t>[0]);
  if (spec.type === 'list') {
    if (spec.max !== undefined && values.length > spec.max) return t('web.metadata.tooManyEntries', {label, max: spec.max});
    if (spec.maxLength !== undefined && values.some(entry => Array.from(entry).length > spec.maxLength!)) return t('web.metadata.listEntryTooLong', {label, max: spec.maxLength});
    return '';
  }
  if (spec.type === 'integer' || spec.type === 'number') {
    if (value.trim() === '') return '';
    const number = Number(value);
    if (!Number.isFinite(number)) return t('web.metadata.wholeNumber', {label});
    if (spec.type === 'integer' && !Number.isInteger(number)) return t('web.metadata.wholeNumber', {label});
    if (spec.min !== undefined || spec.max !== undefined) {
      if ((spec.min !== undefined && number < spec.min) || (spec.max !== undefined && number > spec.max)) return t('web.metadata.numberRange', {label, min: spec.min ?? '', max: spec.max ?? ''});
    }
    return '';
  }
  if (spec.type === 'date' && value && Number.isNaN(Date.parse(value))) return t('web.metadata.dateError', {label});
  if (spec.type === 'enum' && value && spec.allowed && !spec.allowed.includes(value)) return t('web.metadata.enumError', {label});
  if (spec.maxLength !== undefined && value.length > spec.maxLength) return t('web.metadata.textTooLong', {label, max: spec.maxLength});
  return '';
}

function input(spec: RepairFieldSpec, value: string, set: (value: string) => void) {
  if (spec.type === 'enum' && spec.allowed) return <select value={value} onChange={event => set(event.currentTarget.value)}>
    {spec.allowed.map(choice => <option key={choice} value={choice}>{choice}</option>)}</select>;
  if (spec.type === 'multiline') return <textarea rows={4} maxLength={spec.maxLength} value={value} onChange={event => set(event.currentTarget.value)} />;
  return <input type={spec.type === 'date' ? 'date' : 'text'} inputMode={spec.type === 'integer' || spec.type === 'number' ? 'numeric' : undefined} maxLength={spec.maxLength} value={value} onChange={event => set(event.currentTarget.value)} />;
}

function Field({spec, field, draft, set}: {spec: RepairFieldSpec; field?: {value: string; automaticValue: string; locked: boolean; source: string; values?: readonly string[]}; draft?: {value: string; locked: boolean}; set: (patch: Partial<MetadataFieldDraft>) => void}) {
  const value = draft?.value ?? field?.value ?? '', locked = draft?.locked ?? field?.locked ?? false, error = fieldError(spec, value);
  const changed = draft !== undefined && (draft.value !== (field?.value ?? '') || draft.locked !== (field?.locked ?? false));
  return <label className={styles.field} data-wide={spec.type === 'multiline' || spec.field === 'tagline' || undefined} data-changed={changed || undefined}>
    <span className={styles.fieldLabel}>{t(spec.label as Parameters<typeof t>[0])}
      {changed ? <em>{t('web.metadata.editorChanged')}</em> : null}
    </span>
    <span className={styles.fieldControl}>{input(spec, value, next => set({value: next}))}
      <button type="button" className={styles.lock} aria-pressed={locked} title={t(locked ? 'web.metadata.lockedTip' : 'web.metadata.unlockedTip')} aria-label={t(locked ? 'web.metadata.unlockField' : 'web.metadata.lockField', {label: t(spec.label as Parameters<typeof t>[0])})} onClick={() => set({locked: !locked})}>{locked ? t('web.metadata.locked') : t('web.metadata.unlocked')}</button>
    </span>
    {error ? <small className={styles.fieldError} role="alert">{error}</small> : null}
    <small>{field?.source === 'manual' ? t('web.metadata.editedHere') : field?.source ? t('web.metadata.fromSource', {source: ['tmdb', 'tvdb', 'anilist', 'musicbrainz'].includes(field.source) ? providerName(field.source) : field.source}) : ''}{field && field.automaticValue !== value ? <button type="button" onClick={() => set({value: field.automaticValue})}>{t('web.metadata.useIt')}</button> : null}</small>
  </label>;
}

function General({data, draft, set}: {data: RepairData; draft: Draft; set: (field: string, patch: Partial<MetadataFieldDraft>) => void}) {
  return <div className={styles.generalFields}><p className={styles.lockHelp}>{t('web.metadata.editorLockHelp')}</p>{data.schema.filter(spec => spec.type !== 'list' && spec.group !== 'identity').map(spec =>
    <Field key={spec.field} spec={spec} field={data.snapshot.fields[spec.field]} draft={draft[spec.field]} set={patch => set(spec.field, patch)} />)}</div>;
}

function Tags({data, draft, set, busy}: {data: RepairData; draft: Draft; busy: boolean; set: (field: string, patch: Partial<MetadataFieldDraft>) => void}) {
  return <div className={styles.fields}>{data.schema.filter(spec => spec.type === 'list').map(spec => {
    const original = data.snapshot.fields[spec.field], current = draft[spec.field];
    return <TagList key={spec.field} spec={spec} busy={busy} values={current?.values ?? original?.values ?? []} locked={current?.locked ?? original?.locked ?? false}
      onValues={values => set(spec.field, {values})} onLock={locked => set(spec.field, {locked})}/>;
  })}</div>;
}

function TagList({spec, values, locked, onValues, onLock, busy}: {spec: RepairFieldSpec; values: readonly string[]; locked: boolean; busy: boolean;
  onValues: (values: readonly string[]) => void; onLock: (locked: boolean) => void;
}) {
  const [text, setText] = useState(''), [error, setError] = useState('');
  const label = t(spec.label as Parameters<typeof t>[0]), genre = spec.field === 'genres' || spec.field === 'genre';
  const placeholder = genre ? t('web.metadata.addGenreHelp') : spec.field === 'tags' ? t('web.metadata.addTagChip') : spec.field === 'labels' ? t(values.length ? 'web.metadata.addLabelChip' : 'web.metadata.emptyLabelsChip') : t('web.metadata.addListChip', {label});
  const add = () => {
    if (busy) return;
    const value = text.trim();
    if (!value || values.includes(value)) {setText(''); return;}
    const next = [...values, value], problem = fieldError(spec, '', next);
    if (problem) {setError(problem); return;}
    onValues(next); setText(''); setError('');
  };
  return <section className={styles.tagGroup} aria-label={label}>
    <header><h3>{label}</h3>{genre ? <small>{t(locked ? 'web.metadata.genresLocked' : 'web.metadata.genresUnlocked')}</small> : spec.field === 'tags' ? <small>{t('web.metadata.tagsFiling')}</small> : null}
      <button type="button" className={styles.lock} disabled={busy} aria-pressed={locked} onClick={() => onLock(!locked)}>{t(locked ? 'web.metadata.locked' : 'web.metadata.unlocked')}</button></header>
    <div className={styles.tagChips}>{values.map((value, index) => <span className={styles.tagChip} key={index}>
      {value}<button type="button" disabled={busy} aria-label={t('web.metadata.removeLabel', {label: value})} onClick={() => onValues(values.filter((_, at) => at !== index))}>×</button>
    </span>)}<input disabled={busy} value={text} aria-label={placeholder} placeholder={placeholder} onChange={event => {setText(event.currentTarget.value); setError('');}} onKeyDown={event => {if (event.key === 'Enter' && !event.nativeEvent.isComposing) {event.preventDefault(); add();}}} onBlur={add}/></div>
    {error ? <small className={styles.fieldError} role="alert">{error}</small> : null}
  </section>;
}

function Artwork({data, busy, onUse, onLock, onFindMore, onRefetch}: {
  data: RepairData; busy: boolean; onUse: (role: string, candidateId: string) => void;
  onLock: (role: string, subject: string, locked: boolean) => void; onFindMore: (role: string) => void; onRefetch: (role: string) => void;
}) {
  const [selected, setSelected] = useState('');
  const role = data.artworkRoles.includes(selected) ? selected : data.artworkRoles[0];
  const candidates = data.artwork.candidates.filter(candidate => candidate.role === role);
  const chosen = data.snapshot.artwork.find(choice => choice.role === role);
  return <div className={styles.artwork}>
    <div className={styles.artRoles} role="tablist" aria-label={t('web.metadata.tabArtwork')}>{data.artworkRoles.map(value =>
      <button key={value} type="button" role="tab" aria-selected={role === value} onClick={() => setSelected(value)}>{t('web.metadata.artRole', {role: value})}</button>)}</div>
    {role ? <section className={styles.role}>
      <header><b>{t('web.metadata.candidateCount', {count: candidates.length})}</b>
        {chosen ? <Button size="small" disabled={busy} aria-pressed={chosen.locked} onClick={() => onLock(role, chosen.subject, !chosen.locked)}>{t(chosen.locked ? 'web.metadata.unlock' : 'web.metadata.lock')}</Button> : null}
        <Button size="small" onClick={() => onFindMore(role)} disabled={busy}>{t('web.metadata.findMore')}</Button>
        <Button size="small" onClick={() => onRefetch(role)} disabled={busy || !chosen}>{t('web.metadata.refetchChosen')}</Button>
      </header>
      {candidates.length ? <div className={styles.choices}>{candidates.map(candidate => <figure key={candidate.id} data-current={candidate.current || undefined}>
        {candidate.previewUrl ? <img src={candidate.previewUrl} alt={t('web.metadata.artAlt', {role: t('web.metadata.artRole', {role}), provider: providerName(candidate.provider)})} loading="lazy" /> : <div className={styles.placeholder} />}
        <figcaption><small>{providerName(candidate.provider)}{candidate.locale ? ` · ${candidate.locale}` : ''}{candidate.votes != null ? ` · ${t('web.metadata.votes', {count: candidate.votes})}` : ''}</small></figcaption>
        {candidate.current ? <span className={styles.badge}>{t('web.metadata.inUse')}</span>
          : <Button size="small" disabled={busy} onClick={() => onUse(role, candidate.id)}>{t('web.liveSource.useThis')}</Button>}
      </figure>)}</div> : <p className={styles.working}>{t('web.metadata.noCandidates')}</p>}
      {data.artwork.jobs.filter(job => job.role === role).map(job => <p key={job.id} role="status">{t('web.metadata.artJob', {role: t('web.metadata.artRole', {role}), state: job.status})}</p>)}
    </section> : null}
  </div>;
}


function History({data, busy, onUndo, onAll}: {data: RepairData; busy: boolean; onUndo: (historyId: number) => void; onAll: (locked: boolean) => void}) {
  return <div className={styles.fields}>
    <div className={styles.editorFooter}><Button size="small" disabled={busy} onClick={() => onAll(true)}>{t('web.metadata.lockEverything')}</Button>
      <Button size="small" disabled={busy} onClick={() => onAll(false)}>{t('web.metadata.unlockEverything')}</Button></div>
    <h3>{t('web.metadata.recentChanges')}</h3>
    {data.history.length ? data.history.map(row => <div key={row.id} className={styles.historyRow}>
      <div><b>{t('web.metadata.history', {trigger: row.trigger})}</b>{row.observedAt ? <small>{defaultI18n.dateTime(row.observedAt)}</small> : null}</div>
      <Button size="small" disabled={busy} onClick={() => onUndo(row.id)}>{t('web.metadata.undoToHere')}</Button>
    </div>) : <p className={styles.working}>{t('web.metadata.noHistory')}</p>}
  </div>;
}


/** Frame 10: current identity, its lock, and the server's own matching commands. */
function Matching({data, target, busy, onSearch, onUse, onRetry, onLock}: {
  data: RepairData; target: RepairTarget; busy: boolean; onSearch: (query: string) => void;
  onUse: (provider: string, candidateId: string, order?: string) => void; onRetry: () => void; onLock: (locked: boolean) => void;
}) {
  const [query, setQuery] = useState(''), [orders, setOrders] = useState<Readonly<Record<string, string>>>({});
  const identity = data.snapshot.identity;
  const statusKey = `web.metadata.matchStatus.${identity?.status.replace(/_([a-z])/g, (_, char: string) => char.toUpperCase()) ?? 'unmatched'}`;
  const status = identity?.status === 'needs_selection' ? t('web.metadata.moreThanOneMatch') : defaultI18n.has(statusKey) ? t(statusKey) : t('web.metadata.matchStatus.unknown');
  const typeKey = `metadata.attention.kind.${identity?.type === 'movie' ? 'film' : identity?.type ?? ''}`;
  const type = defaultI18n.has(typeKey) ? t(typeKey) : '';
  const scannedMatch = target.kind === 'album';
  return <div className={styles.fields}>
    <section className={styles.candidate}>
      <div><b>{t('web.metadata.currentMatch')}</b>
        <small>{identity ? [providerName(identity.provider), type, identity.id, status].filter(Boolean).join(' · ') : t('web.metadata.matchStatus.unmatched')}</small></div>
      <button type="button" className={styles.lock} disabled={busy} aria-pressed={identity?.locked === true}
        onClick={() => onLock(identity?.locked !== true)}>{t(identity?.locked ? 'web.metadata.identityLocked' : 'web.metadata.identityUnlocked')}</button>
    </section>
    <form className={styles.matchSearch} onSubmit={event => {event.preventDefault(); if (!scannedMatch && query.trim() && !busy) onSearch(query.trim());}}>
      {scannedMatch ? <p>{t('web.metadata.searchProviderHelpAuto')}</p> : <>
        <TextField label={t('web.metadata.searchProvider')} description={t('web.metadata.searchProviderHelp')} value={query} onChange={event => setQuery(event.currentTarget.value)} />
        <Button size="small" type="submit" disabled={busy || !query.trim()}>{t('web.search.title')}</Button>
      </>}
      <Button size="small" type="button" disabled={busy} onClick={onRetry}>{t('web.chooser.tryAgain')}</Button>
    </form>
    {data.candidates.map(candidate => {
      const key = `${candidate.provider}:${candidate.id}`;
      const chosen = candidate.orders?.find(item => item.id === orders[key])?.id ?? candidate.orders?.[0]?.id;
      return <div key={key} className={styles.candidate}>
        <div><b>{candidate.title}{candidate.year ? ` (${candidate.year})` : ''}</b><small>{providerName(candidate.provider)}{candidate.subtitle ? ` · ${candidate.subtitle}` : ''}</small>
          {candidate.orders && candidate.orders.length > 1 ? <label className={styles.order}>{t('metadata.attention.episodeOrder')}
            <select value={chosen} onChange={event => {const value = event.currentTarget.value; setOrders(current => ({...current, [key]: value}));}}>{candidate.orders.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label> : null}</div>
        <Button size="small" disabled={busy} onClick={() => onUse(candidate.provider, candidate.id, candidate.orders?.length ? chosen : undefined)}>{t('web.liveSource.useThis')}</Button>
      </div>;
    })}
  </div>;
}


/** Frame 9: draft edits preserve every existing identity tuple. The current producer only
 * writes manual credits; provider rows stay read-only until its owner override is available. */
function People({data, target, busy, onCommit, onLock}: {
  data: RepairData; target: RepairTarget; busy: boolean;
  onCommit: (rows: RepairData['snapshot']['relationships']) => Promise<boolean>; onLock: (locked: boolean) => void;
}) {
  const credits = data.snapshot.relationships.filter(row => row.kind === 'credit');
  const [draft, setDraft] = useState<RepairData['snapshot']['relationships'] | null>(null);
  const rows = draft ?? credits;
  const editable = (row: RepairData['snapshot']['relationships'][number]) => !row.recordId || row.provider === 'manual' && !row.locked;
  const clean = (value: string) => !/[\u0000-\u001f\u007f]/.test(value);
  const valid = rows.length <= 100 && rows.every(row => row.label.trim() !== '' && row.label.length <= 300 &&
    (row.role?.length ?? 0) <= 500 && (row.department?.length ?? 0) <= 200 && clean(row.label) && clean(row.role ?? '') && clean(row.department ?? ''));
  const change = (index: number, patch: Partial<Pick<RepairData['snapshot']['relationships'][number], 'label' | 'role' | 'department'>>) => {
    if (busy || !editable(rows[index])) return;
    setDraft(rows.map((row, at) => at === index ? {...row, ...patch} : row));
  };
  const remove = (index: number) => {
    if (busy || !editable(rows[index])) return;
    setDraft(rows.filter((_, at) => at !== index));
  };
  const add = () => {
    if (busy || target.kind !== 'item' || rows.length >= 100) return;
    setDraft([...rows, {kind: 'credit', targetKind: 'scoped_credit', targetId: '', label: '', role: '', department: '',
      ordinal: rows.reduce((top, row) => Math.max(top, row.ordinal), 0) + 1, source: 'manual', locked: true}]);
  };
  const save = async () => {
    if (busy || !draft || !valid) return;
    if (await onCommit(rows.map(row => editable(row) ? {...row, label: row.label.trim(), role: row.role?.trim(), department: row.department?.trim()} : row))) setDraft(null);
  };
  return <div className={styles.fields}>
    <div className={styles.creditHeader}><h3>{t('web.metadata.tabPeople')}</h3><small>{t('web.metadata.creditCount', {count: rows.length})}</small>
      <button type="button" className={styles.lock} disabled={busy || draft !== null} aria-pressed={data.snapshot.relationshipLocks.credit === true}
        onClick={() => onLock(data.snapshot.relationshipLocks.credit !== true)}>{t(data.snapshot.relationshipLocks.credit ? 'web.metadata.locked' : 'web.metadata.unlocked')}</button>
      {target.kind === 'item' ? <Button size="small" disabled={busy || rows.length >= 100} onClick={add}>{t('web.metadata.addPerson')}</Button> : null}
    </div>
    {rows.map((row, index) => <div key={`${row.recordId ?? 'new'}:${row.targetId}:${index}`} className={styles.creditRow} data-new={!row.recordId || undefined}>
      <TextField label={t('web.metadata.field.name')} value={row.label} disabled={busy || !editable(row)} maxLength={300} onChange={event => change(index, {label: event.currentTarget.value})}/>
      <TextField label={t('web.metadata.field.department')} value={row.department ?? ''} disabled={busy || !editable(row)} maxLength={200} onChange={event => change(index, {department: event.currentTarget.value})}/>
      <TextField label={t(row.department?.toLowerCase() === 'acting' ? 'web.metadata.creditCharacter' : 'web.metadata.creditRole')} value={row.role ?? ''} disabled={busy || !editable(row)} maxLength={500} onChange={event => change(index, {role: event.currentTarget.value})}/>
      <Button size="small" disabled={busy || !editable(row)} onClick={() => remove(index)}>{t('web.metadata.removeLabel', {label: row.label})}</Button>
    </div>)}
    <footer className={styles.editorFooter}>
      <Button size="small" disabled={busy || draft === null} onClick={() => setDraft(null)}>{t('action.discard')}</Button>
      <Button size="small" variant="primary" disabled={busy || draft === null || !valid} onClick={() => void save()}>{t('web.metadata.saveCredits')}</Button>
    </footer>
  </div>;
}
