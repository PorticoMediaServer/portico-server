import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {MetadataRepairService, type RepairCommand, type RepairData, type RepairFieldSpec, type RepairTarget} from '@core/metadata-repair.ts';
import {EditorHeader} from '../../kit/shell';
import {Button, TextField} from '../../kit/controls';
import {providerName} from '@core/server-admin/attention-words.ts';
import {HostedError} from '../../kit/accounts/ServicesAuth';
import styles from './metadata.module.css';
import {metadataDraftValue, patchMetadataDraft} from './draft-value';

const t = defaultI18n.t;
type Api = Readonly<{request: <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<T>}>;
type Draft = Readonly<Record<string, {value: string; locked: boolean}>>;
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
  const invalid = data ? Object.entries(draft).some(([name, value]) => {const spec = data.schema.find(row => row.field === name); return spec ? Boolean(fieldError(spec, value.value)) : false;}) : false;
  const changed = useMemo(() => {
    if (!data) return [];
    return Object.entries(draft).filter(([field, value]) => {
      const original = data.snapshot.fields[field];
      return value.locked !== (original?.locked ?? false) || value.value !== metadataDraftValue(data, field);
    }).map(([field, value]) => ({field, ...value}));
  }, [data, draft]);
  const edit = async (fields: Record<string, {value?: string; locked?: boolean}>) => {
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
  const set = (field: string, patch: Partial<{value: string; locked: boolean}>) => {
    setDraft(current => ({...current, [field]: patchMetadataDraft(data, field, current[field], patch)}));
  };
  const save = () => {
    if (!changed.length) return;
    const fields: Record<string, {value?: string; locked?: boolean}> = {};
    for (const item of changed) {
      const spec = data?.schema.find(row => row.field === item.field);
      fields[item.field] = {...(spec?.type === 'list' ? {values: item.value.split(',').map(value => value.trim()).filter(Boolean)} : {value: item.value}), locked: item.locked};
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
      {shown === 'tags' ? <Tags data={data} draft={draft} set={set} /> : null}
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
function fieldError(spec: RepairFieldSpec, value: string): string {
  const label = t(spec.label as Parameters<typeof t>[0]);
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

function Field({spec, field, draft, set}: {spec: RepairFieldSpec; field?: {value: string; automaticValue: string; locked: boolean; source: string; values?: readonly string[]}; draft?: {value: string; locked: boolean}; set: (patch: Partial<{value: string; locked: boolean}>) => void}) {
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

function General({data, draft, set}: {data: RepairData; draft: Draft; set: (field: string, patch: Partial<{value: string; locked: boolean}>) => void}) {
  return <div className={styles.generalFields}><p className={styles.lockHelp}>{t('web.metadata.editorLockHelp')}</p>{data.schema.filter(spec => spec.type !== 'list' && spec.group !== 'identity').map(spec =>
    <Field key={spec.field} spec={spec} field={data.snapshot.fields[spec.field]} draft={draft[spec.field]} set={patch => set(spec.field, patch)} />)}</div>;
}

function Tags({data, draft, set}: {data: RepairData; draft: Draft; set: (field: string, patch: Partial<{value: string; locked: boolean}>) => void}) {
  const list = data.schema.filter(spec => spec.type === 'list');
  return <div className={styles.fields}>
    {list.length ? list.map(spec => {
      const original = data.snapshot.fields[spec.field];
      const value = draft[spec.field]?.value ?? (original?.values ?? []).join(', ');
      return <Field key={spec.field} spec={{...spec, type: 'text'}}
        field={original ? {...original, value, automaticValue: (original.values ?? []).join(', ')} : undefined}
        draft={draft[spec.field]} set={patch => set(spec.field, patch)} />;
    }) : <p className={styles.working}>{t('web.metadata.genresOnlyItems')}</p>}
  </div>;
}

function Artwork({data, busy, onUse, onLock, onFindMore, onRefetch}: {
  data: RepairData; busy: boolean; onUse: (role: string, candidateId: string) => void;
  onLock: (role: string, subject: string, locked: boolean) => void; onFindMore: (role: string) => void; onRefetch: (role: string) => void;
}) {
  return <div className={styles.artwork}>{data.artworkRoles.map(role => {
    const candidates = data.artwork.candidates.filter(candidate => candidate.role === role);
    const chosen = data.snapshot.artwork.find(choice => choice.role === role);
    return <section key={role} className={styles.role}>
      <header><b>{role}</b>
        {chosen ? <Button size="small" disabled={busy} aria-pressed={chosen.locked} onClick={() => onLock(role, chosen.subject, !chosen.locked)}>{t(chosen.locked ? 'web.metadata.unlock' : 'web.metadata.lock')}</Button> : null}
        <Button size="small" onClick={() => onFindMore(role)} disabled={busy}>{t('web.metadata.findMore')}</Button>
        <Button size="small" onClick={() => onRefetch(role)} disabled={busy}>{t('web.metadata.refetchChosen')}</Button>
      </header>
      {candidates.length ? <div className={styles.choices}>{candidates.map(candidate => <figure key={candidate.id} data-current={candidate.current || undefined}>
        {candidate.previewUrl ? <img src={candidate.previewUrl} alt="" loading="lazy" /> : <div className={styles.placeholder} />}
        <figcaption><small>{candidate.provider}{candidate.locale ? ` · ${candidate.locale}` : ''}{candidate.votes ? ` · ${candidate.votes}` : ''}</small></figcaption>
        {candidate.current ? <span className={styles.badge}>{t('web.metadata.inUse')}</span>
          : <Button size="small" disabled={busy} onClick={() => onUse(role, candidate.id)}>{t('web.liveSource.useThis')}</Button>}
      </figure>)}</div> : <p className={styles.working}>{t('web.metadata.noCandidates')}</p>}
    </section>;
  })}</div>;
}

function History({data, busy, onUndo, onAll}: {data: RepairData; busy: boolean; onUndo: (historyId: number) => void; onAll: (locked: boolean) => void}) {
  return <div className={styles.fields}>
    <div className={styles.editorFooter}><Button size="small" disabled={busy} onClick={() => onAll(true)}>{t('web.metadata.lockEverything')}</Button>
      <Button size="small" disabled={busy} onClick={() => onAll(false)}>{t('web.metadata.unlockEverything')}</Button></div>
    {data.history.length ? data.history.map(row => <div key={row.id} className={styles.historyRow}>
      <span>{row.trigger}</span><small>{row.observedAt}</small>
      <Button size="small" disabled={busy} onClick={() => onUndo(row.id)}>{t('web.metadata.undo')}</Button>
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
  const scannedMatch = target.kind === 'album';
  return <div className={styles.fields}>
    <section className={styles.candidate}>
      <div><b>{t('web.metadata.currentMatch')}</b>
        <small>{identity ? `${providerName(identity.provider)} · ${identity.type} · ${identity.id} · ${identity.status}` : t('web.metadata.matchStatus.unmatched')}</small></div>
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


/** Frame 9. Credits are replaced as one list through the real producer: existing rows keep
 * their recordId/provider/target tuple, and a name-only row is created by omitting recordId.
 * Only `credit` rows are sent, never genre or hierarchy families, and only for an item. */
function People({data, target, busy, onCommit, onLock}: {
  data: RepairData; target: RepairTarget; busy: boolean;
  onCommit: (rows: RepairData['snapshot']['relationships']) => Promise<boolean>; onLock: (locked: boolean) => void;
}) {
  const credits = data.snapshot.relationships.filter(row => row.kind === 'credit');
  const [name, setName] = useState(''), [role, setRole] = useState(''), [department, setDepartment] = useState('');
  const clean = (value: string) => !/[\u0000-\u001f\u007f]/.test(value);
  const valid = target.kind === 'item' && credits.length < 100 && name.trim() !== '' && name.length <= 300 && role.length <= 500 && department.length <= 200 && clean(name) && clean(role) && clean(department);
  const remove = (index: number) => onCommit(credits.filter((_, at) => at !== index));
  const add = async () => {
    if (!valid) return;
    const saved = await onCommit([...credits, {
      kind: 'credit', targetKind: 'scoped_credit', targetId: '', label: name.trim(), role: role.trim(), department: department.trim(),
      ordinal: credits.reduce((top, row) => Math.max(top, row.ordinal), 0) + 1, source: 'manual', locked: true,
    } as RepairData['snapshot']['relationships'][number]]);
    if (saved) {setName(''); setRole(''); setDepartment('');}
  };
  return <div className={styles.fields}>
    <button type="button" className={styles.lock} disabled={busy} aria-pressed={data.snapshot.relationshipLocks.credit === true}
      onClick={() => onLock(data.snapshot.relationshipLocks.credit !== true)}>{t(data.snapshot.relationshipLocks.credit ? 'web.metadata.locked' : 'web.metadata.unlocked')}</button>
    {credits.map((row, index) => <div key={`${row.recordId ?? 'new'}:${row.targetId}:${row.ordinal}:${index}`} className={styles.candidate}>
      <div><b>{row.label}</b><small>{[row.department, row.role].filter(Boolean).join(' · ')}{row.locked ? ` · ${t('web.metadata.locked')}` : ''}</small></div>
      <Button size="small" disabled={busy || row.locked} onClick={() => void remove(index)}>{t('web.metadata.removeLabel', {label: row.label})}</Button>
    </div>)}
    {target.kind === 'item' && credits.length < 100 ? <div className={styles.candidate}>
      <div className={styles.creditNew}>
        <TextField label={t('web.metadata.field.name')} value={name} onChange={event => setName(event.currentTarget.value)} maxLength={300} />
        <TextField label={t('web.metadata.field.role')} value={role} onChange={event => setRole(event.currentTarget.value)} maxLength={500} />
        <TextField label={t('web.metadata.field.department')} value={department} onChange={event => setDepartment(event.currentTarget.value)} maxLength={200} />
      </div>
      <Button size="small" disabled={busy || !valid} onClick={() => void add()}>{t('web.metadata.addPerson')}</Button>
    </div> : null}
  </div>;
}
