import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {MetadataRepairService, type RepairCommand, type RepairData, type RepairFieldSpec, type RepairTarget} from '@core/metadata-repair.ts';
import {Button, TextField} from '../../kit/controls';
import {providerName} from '@core/server-admin/attention-words.ts';
import {HostedError} from '../../kit/accounts/ServicesAuth';
import styles from './metadata.module.css';

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
  if (data && data.snapshot.relationships.length) tabs.push('people');
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
      return value.locked !== (original?.locked ?? false) || value.value !== (original?.value ?? '');
    }).map(([field, value]) => ({field, ...value}));
  }, [data, draft]);
  const edit = async (fields: Record<string, {value?: string; locked?: boolean}>) => {
    if (!data) return;
    setError(undefined);
    await service.command({action: 'edit', fields});
  };
  const run = async (command: RepairCommand) => {setError(undefined); if (!(await service.command(command))) setError(state.error ?? new Error(t('web.metadata.saveChanges')));};
  const set = (field: string, patch: Partial<{value: string; locked: boolean}>) => {
    const original = data?.snapshot.fields[field];
    setDraft(current => ({...current, [field]: {value: patch.value ?? current[field]?.value ?? original?.value ?? '', locked: patch.locked ?? current[field]?.locked ?? original?.locked ?? false}}));
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
  return <aside className={styles.editor} aria-label={t('web.metadata.sections')}>
    <header className={styles.editorHead}><h2>{heading}</h2><Button size="small" onClick={onClose}>{t('action.close')}</Button></header>
    <nav className={styles.editorTabs} aria-label={t('web.metadata.sections')}>
      {tabsFor(data).map(id => <button key={id} type="button" aria-current={shown === id ? 'page' : undefined} onClick={() => setTab(id)}>
        {t(id === 'general' ? 'web.metadata.tabGeneral' : id === 'artwork' ? 'web.metadata.tabArtwork' : id === 'tags' ? 'web.metadata.tabTags' : 'web.metadata.tabHistory')}</button>)}
    </nav>
    {state.phase === 'loading' ? <p className={styles.working}>{t('web.metadata.loading')}</p> : null}
    {state.error ? <HostedError error={state.error} /> : null}
    {error ? <HostedError error={error} /> : null}
    {state.phase === 'saving' ? <p className={styles.working}>{t('web.metadata.applying')}</p> : null}
    {data && state.phase !== 'loading' ? <>
      {shown === 'general' ? <General data={data} draft={draft} set={set} /> : null}
      {shown === 'tags' ? <Tags data={data} draft={draft} set={set} /> : null}
      {shown === 'people' ? <People data={data} busy={state.phase === 'saving'} onRemove={index => void run({action: 'edit_relationships', relationships: data.snapshot.relationships.filter((_, at) => at !== index)})} /> : null}
      {shown === 'matching' ? <Matching data={data} busy={state.phase === 'saving'} onSearch={query => void run({action: 'search', query})}
        onUse={(provider, candidateId, order) => void run({action: 'identify', provider, candidateId, ...(order ? {order} : {})})} /> : null}
      {shown === 'artwork' ? <Artwork data={data} busy={state.phase === 'saving'} onUse={(role, candidateId) => void run({action: 'select_artwork', role, candidateId})}
        onLock={(role, subject, locked) => void run({action: 'artwork_lock', role, subject, locked})}
        onFindMore={role => void run({action: 'discover_artwork', role})} onRefetch={role => void run({action: 'repair_assets', role})} /> : null}
      {shown === 'history' ? <History data={data} busy={state.phase === 'saving'} onUndo={historyId => void run({action: 'undo', historyId})}
        onAll={locked => void run({action: locked ? 'lock_all' : 'unlock_all'})} /> : null}
      {shown !== 'artwork' && shown !== 'history' ? <footer className={styles.editorFooter}>
        <Button size="small" disabled={!changed.length || state.phase === 'saving'} onClick={() => setDraft({})}>{t('action.discard')}</Button>
        <Button size="small" variant="primary" loading={state.phase === 'saving'} disabled={!changed.length || invalid} onClick={save}>
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
  if (spec.maxLength !== undefined && [...value].length > spec.maxLength) return t('web.metadata.textTooLong', {label, max: spec.maxLength});
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
  return <label className={styles.field} data-changed={changed || undefined}>
    <span className={styles.fieldLabel}>{t(spec.label as Parameters<typeof t>[0])}
      <button type="button" className={styles.lock} aria-pressed={locked} title={t(locked ? 'web.metadata.lockedTip' : 'web.metadata.unlockedTip')} onClick={() => set({locked: !locked})}>{locked ? t('web.metadata.locked') : t('web.metadata.unlocked')}</button>
    </span>
    {input(spec, value, next => set({value: next}))}
    {error ? <small className={styles.fieldError} role="alert">{error}</small> : null}
    <small>{field?.source}{field && field.automaticValue !== field.value && !locked ? <button type="button" onClick={() => set({value: field.automaticValue})}>{t('web.metadata.useIt')}</button> : null}</small>
  </label>;
}

function General({data, draft, set}: {data: RepairData; draft: Draft; set: (field: string, patch: Partial<{value: string; locked: boolean}>) => void}) {
  return <div className={styles.fields}>{data.schema.filter(spec => spec.type !== 'list' && spec.group !== 'identity').map(spec =>
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


/** Frame 10: the current match, a search, and the server's own candidates. The identity
 * lock has no supported repair command in this service, so it is not drawn as a dead control. */
function Matching({data, busy, onSearch, onUse}: {
  data: RepairData; busy: boolean; onSearch: (query: string) => void;
  onUse: (provider: string, candidateId: string, order?: string) => void;
}) {
  const [query, setQuery] = useState(''), [order, setOrder] = useState<string>();
  const identity = data.snapshot.identity;
  return <div className={styles.fields}>
    <section className={styles.candidate}>
      <div><b>{t('web.metadata.currentMatch')}</b>
        <small>{identity ? `${providerName(identity.provider)} · ${identity.type} · ${identity.id} · ${identity.status}` : t('web.metadata.matchStatus.unmatched')}</small></div>
    </section>
    <form className={styles.matchSearch} onSubmit={event => {event.preventDefault(); if (query.trim() && !busy) onSearch(query.trim());}}>
      <TextField label={t('web.metadata.searchProvider')} description={t('web.metadata.searchProviderHelp')} value={query} onChange={event => setQuery(event.currentTarget.value)} />
      <Button size="small" type="submit" disabled={busy || !query.trim()}>{t('web.search.title')}</Button>
    </form>
    {data.candidates.map(candidate => {
      const chosen = order ?? candidate.orders?.[0]?.id;
      return <div key={candidate.id} className={styles.candidate}>
        <div><b>{candidate.title}{candidate.year ? ` (${candidate.year})` : ''}</b><small>{providerName(candidate.provider)}{candidate.subtitle ? ` · ${candidate.subtitle}` : ''}</small>
          {candidate.orders && candidate.orders.length > 1 ? <label className={styles.order}>{t('metadata.attention.episodeOrder')}
            <select value={chosen} onChange={event => setOrder(event.currentTarget.value)}>{candidate.orders.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label> : null}</div>
        <Button size="small" disabled={busy} onClick={() => onUse(candidate.provider, candidate.id, candidate.orders?.length ? chosen : undefined)}>{t('web.liveSource.useThis')}</Button>
      </div>;
    })}
  </div>;
}


/** Frame 9: one row per credit from the server's own relationship list. Removal sends the
 * remaining list back unchanged; adding a person needs the producer's new-relationship shape,
 * so it is not drawn as a dead button here. */
function People({data, busy, onRemove}: {data: RepairData; busy: boolean; onRemove: (index: number) => void}) {
  return <div className={styles.fields}>{data.snapshot.relationships.map((row, index) =>
    <div key={`${row.kind}:${row.targetId}:${row.ordinal}:${index}`} className={styles.candidate}>
      <div><b>{row.label}</b><small>{[row.department, row.role].filter(Boolean).join(' · ')}{row.source ? ` · ${row.source}` : ''}{row.locked ? ` · ${t('web.metadata.locked')}` : ''}</small></div>
      <Button size="small" disabled={busy || row.locked} onClick={() => onRemove(index)}>{t('web.metadata.removeLabel', {label: row.label})}</Button>
    </div>)}</div>;
}
