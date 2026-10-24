import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {MetadataRepairService, type RepairCommand, type RepairData, type RepairFieldSpec, type RepairTarget} from '@core/metadata-repair.ts';
import {Button} from '../../kit/controls';
import {HostedError} from '../../kit/accounts/ServicesAuth';
import styles from './metadata.module.css';

const t = defaultI18n.t;
type Api = Readonly<{request: <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<T>}>;
type Draft = Readonly<Record<string, {value: string; locked: boolean}>>;
type Tab = 'general' | 'artwork' | 'tags';

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
  useEffect(() => setDraft({}), [state.phase === 'ready' ? state.data?.revision : undefined]);
  const data = state.data;
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
      {(['general', 'artwork', 'tags'] as const).map(id => <button key={id} type="button" aria-current={tab === id ? 'page' : undefined} onClick={() => setTab(id)}>
        {t(id === 'general' ? 'web.metadata.tabGeneral' : id === 'artwork' ? 'web.metadata.tabArtwork' : 'web.metadata.tabTags')}</button>)}
    </nav>
    {state.phase === 'loading' ? <p className={styles.working}>{t('web.metadata.loading')}</p> : null}
    {state.error ? <HostedError error={state.error} /> : null}
    {error ? <HostedError error={error} /> : null}
    {state.phase === 'saving' ? <p className={styles.working}>{t('web.metadata.applying')}</p> : null}
    {data && state.phase !== 'loading' ? <>
      {tab === 'general' ? <General data={data} draft={draft} set={set} /> : null}
      {tab === 'tags' ? <Tags data={data} draft={draft} set={set} /> : null}
      {tab === 'artwork' ? <Artwork data={data} busy={state.phase === 'saving'} onUse={(role, candidateId) => void run({action: 'select_artwork', role, candidateId})}
        onFindMore={role => void run({action: 'discover_artwork', role})} onRefetch={role => void run({action: 'repair_assets', role})} /> : null}
      {tab !== 'artwork' ? <footer className={styles.editorFooter}>
        <Button size="small" disabled={!changed.length || state.phase === 'saving'} onClick={() => setDraft({})}>{t('action.discard')}</Button>
        <Button size="small" variant="primary" loading={state.phase === 'saving'} disabled={!changed.length} onClick={save}>
          {t('web.metadata.saveChanges')}{changed.length ? ` (${changed.length})` : ''}</Button>
      </footer> : null}
    </> : null}
  </aside>;
}

function input(spec: RepairFieldSpec, value: string, set: (value: string) => void) {
  if (spec.type === 'enum' && spec.allowed) return <select value={value} onChange={event => set(event.currentTarget.value)}>
    {spec.allowed.map(choice => <option key={choice} value={choice}>{choice}</option>)}</select>;
  if (spec.type === 'multiline') return <textarea rows={4} maxLength={spec.maxLength} value={value} onChange={event => set(event.currentTarget.value)} />;
  return <input type={spec.type === 'date' ? 'date' : 'text'} inputMode={spec.type === 'integer' || spec.type === 'number' ? 'numeric' : undefined} maxLength={spec.maxLength} value={value} onChange={event => set(event.currentTarget.value)} />;
}

function Field({spec, field, draft, set}: {spec: RepairFieldSpec; field?: {value: string; automaticValue: string; locked: boolean; source: string; values?: readonly string[]}; draft?: {value: string; locked: boolean}; set: (patch: Partial<{value: string; locked: boolean}>) => void}) {
  const value = draft?.value ?? field?.value ?? '', locked = draft?.locked ?? field?.locked ?? false;
  const changed = draft !== undefined && (draft.value !== (field?.value ?? '') || draft.locked !== (field?.locked ?? false));
  return <label className={styles.field} data-changed={changed || undefined}>
    <span className={styles.fieldLabel}>{t(spec.label as Parameters<typeof t>[0])}
      <button type="button" className={styles.lock} aria-pressed={locked} title={t(locked ? 'web.metadata.lockedTip' : 'web.metadata.unlockedTip')} onClick={() => set({locked: !locked})}>{locked ? t('web.metadata.locked') : t('web.metadata.unlocked')}</button>
    </span>
    {input(spec, value, next => set({value: next}))}
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

function Artwork({data, busy, onUse, onFindMore, onRefetch}: {
  data: RepairData; busy: boolean; onUse: (role: string, candidateId: string) => void;
  onFindMore: (role: string) => void; onRefetch: (role: string) => void;
}) {
  return <div className={styles.artwork}>{data.artworkRoles.map(role => {
    const candidates = data.artwork.candidates.filter(candidate => candidate.role === role);
    return <section key={role} className={styles.role}>
      <header><b>{role}</b>
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
