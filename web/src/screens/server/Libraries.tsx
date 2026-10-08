import {FolderPicker, LibrarySettingsSections, type LibraryTab} from './LibrarySettings';
import {LookupStatusRow, useScreenLookup} from './MetadataLookup';
import {MetadataSourceChoice} from './MetadataSource';
import {kindMetadataAgentsPath, parseKindMetadataAgents, type KindMetadataAgents, type MetadataAgentId, type MetadataAgentOption} from '../../admin/metadata-agent';
import {useI18n, type MessageId} from '../../app/i18n';
import {useEffect, useMemo, useState} from 'react';
import {useNavigate, useSearch} from '@tanstack/react-router';
import {LibraryManagementService, fetchEpisodeIssues, type EpisodeIssue, type LibraryAdminJob, type LibraryManagementSnapshot, type ManagedLibrary} from '@core/library-management.ts';
import {LibraryInventoryService, scanOperations, type LibraryInventorySnapshot, type ScanTier, type SourceSettings} from '@core/library-inventory.ts';
import {addSourceSettings, canAddSource, createLibraryAndScan, scanJobLabels as jobLabel, scanTierHelp as tierHelp, scanTierLabels as tierLabel} from '@core/server-admin/panel-logic.ts';
export {addSourceSettings, canAddSource, libraryMutationError} from '@core/server-admin/panel-logic.ts';
import {problem, sentence, useAction} from '../../admin/console';
import {useService} from '../../app/content';
import {iconFor} from '@core/presentation/index.ts';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {libraryKindLabel, useLibrariesContext} from '../../app/libraries';
import {useInlineForm} from '../settings/ServerForms';
import page from '../settings/Settings.module.css';
import {Badge, Button, Checkbox, ConfirmDialog, Dialog, Freshness, Icon, Input, ListRow, Loading, Menu, Notice, Select, SettingsGroup, SettingsRow, StateView, Tabs, Text, type MenuItem} from '../../ui';
import {SectionHeader} from './Server';

const kinds: readonly {value: ManagedLibrary['kind']; label: MessageId; description: MessageId}[] = [{value: 'movie', label: 'web.search.movies', description: 'web.libraries.typeMovies'}, {value: 'tv', label: 'builder.shows', description: 'web.libraries.typeTv'}, {value: 'anime', label: 'web.libraries.kindAnime', description: 'web.libraries.typeAnime'}, {value: 'music', label: 'pref.group.music.title', description: 'web.libraries.typeMusic'}, {value: 'audiobook', label: 'web.libraries.kindAudiobooks', description: 'web.libraries.typeAudiobooks'}];
/** The Add dialog names a new library after its type ("Movies"), editable. */
export function defaultLibraryName(kind: ManagedLibrary['kind']): string {
  return libraryKindLabel(kind);
}
const jobTone = (s: LibraryAdminJob['status']): 'accent' | 'healthy' | 'warning' | 'danger' | 'neutral' => (s === 'running' ? 'accent' : s === 'complete' ? 'healthy' : s === 'complete_with_warnings' || s === 'paused' ? 'warning' : s === 'failed' ? 'danger' : 'neutral');

const opLabel: Record<string, string> = {probe: 'Inspect media files', local_metadata: 'Local metadata and artwork', subtitles: 'Subtitle discovery', checksum: 'Checksums', chapter_images: 'Chapter images', trickplay: 'Preview thumbnails', waveform: 'Audio waveforms', loudness: 'Loudness analysis', fingerprint: 'Audio fingerprints', segment_detection: 'Intro and credit detection'};

/**
 * Libraries: the directory, and one library's sources, scan policy and scan
 * state. Library-level settings and operations only; the file browser is a
 * power tool behind a disclosure.
 */
export function LibrariesPage() {
  const {api} = useSession();
  const {t} = useI18n();
  const scope = useViewerScope();
  const {service, snapshot} = useService<LibraryManagementService, LibraryManagementSnapshot>(() => new LibraryManagementService({api, scope}), [api, scope]);
  const search = useSearch({from: '/app/settings/$section'});
  const navigate = useNavigate();
  const selectedId = search.id;
  useEffect(() => {
    void service.loadLibraries().catch(() => {});
  }, [service]);
  useEffect(() => {
    if (selectedId) void service.selectLibrary(selectedId).catch(() => {});
  }, [service, selectedId]);
  const [adding, setAdding] = useState(false);
  // The list is in the owner's order, the one every client shows (GET /v1/libraries).
  const order = useLibrariesContext();
  const rank = new Map(order.items.map((l, i) => [l.id, i]));
  const libraries = [...(snapshot.directory.data?.items ?? [])].sort((a, b) => (rank.get(a.id) ?? 1e9) - (rank.get(b.id) ?? 1e9));
  const action = useAction();
  const [issuesFor, setIssuesFor] = useState<string>();
  const select = (id?: string) => void navigate({to: '/settings/$section', params: {section: 'server-libraries'}, search: id ? {id} : {}});
  const move = (index: number, by: -1 | 1) => {
    const ids = libraries.map(l => l.id);
    const [id] = ids.splice(index, 1);
    ids.splice(index + by, 0, id!);
    void action.run(() => api.request('/v1/admin/libraries/order', 'PUT', {libraryIds: ids})).then(ok => { if (ok) { order.reload(); void service.loadLibraries(); } });
  };
  if (selectedId) return <LibraryDetail service={service} snapshot={snapshot} libraryId={selectedId} onBack={() => select()} />;
  return (
    <>
      <SectionHeader lede={t('web.console.nav.librariesLede')} actions={<Button variant="primary" icon="plus" label={t('web.empty.addLibrary')} onClick={() => setAdding(true)} />} />
      {snapshot.directory.phase === 'error' ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: () => void service.loadLibraries()}}>{snapshot.directory.error ? problem(snapshot.directory.error) : null}</Notice> : null}
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
      {snapshot.directory.phase === 'loading' && !libraries.length ? <Loading label={t('web.libraries.loading')} /> : null}
      {snapshot.directory.phase === 'ready' && !libraries.length ? <StateView icon="library" title={t('web.shell.noLibraries')} body={t('web.libraries.emptyLede')} action={{label: t('web.empty.addLibrary'), onClick: () => setAdding(true)}} /> : null}
      {libraries.length ? (
        <SettingsGroup>
          {libraries.map((l, i) => {
            const scan = l.lastScan;
            const facts = [libraryKindLabel(l.kind), l.source.path, scan?.finishedAt ? t('server.libraries.scanned', {when: new Date(scan.finishedAt).toLocaleDateString(undefined, {month: 'short', day: 'numeric'})}) : ''].filter(Boolean).join(' · ');
            const warned = !!l.attention || !!scan?.warnings;
            const items: MenuItem[] = [{id: 'open', label: t('web.selection.edit'), icon: 'settings'}, {id: 'up', label: t('server.libraries.moveUp'), icon: 'chevronUp', disabled: i === 0, separatorBefore: true}, {id: 'down', label: t('server.libraries.moveDown'), icon: 'chevronDown', disabled: i === libraries.length - 1}];
            return (
              <SettingsRow key={l.id} icon={iconFor(l.kind)} label={<button type="button" className={page.rowLink} onClick={() => select(l.id)}>{l.name}</button>} help={facts}
                control={<div style={{display: 'flex', gap: 8, alignItems: 'center'}}>
                  {l.source.availability === 'unavailable' ? <Badge tone="danger">{t('web.libraries.sourceUnavailable')}</Badge> : null}
                  {/* A status that needs reading opens what it is about. */}
                  {warned ? <Button size="sm" variant="outline" icon="warning" label={l.attention ? t('server.libraries.needAttention', {count: l.attention.files}) : t('server.libraries.warnings', {count: scan?.warnings ?? 0})} onClick={() => (l.attention ? setIssuesFor(l.id) : select(l.id))} /> : scan ? <Badge tone={jobTone(scan.status)} dot={scan.status === 'running'} live={scan.status === 'running'}>{jobLabel[scan.status]}</Badge> : null}
                  <Menu label={l.name} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('player.more')} />} items={items} onSelect={id => { if (id === 'open') select(l.id); else if (id === 'up') move(i, -1); else if (id === 'down') move(i, 1); }} />
                </div>} />
            );
          })}
        </SettingsGroup>
      ) : null}
      {issuesFor ? <EpisodeIssuesDialog libraryId={issuesFor} onClose={() => setIssuesFor(undefined)} /> : null}
      <AddLibraryDialog open={adding} onOpenChange={setAdding} service={service} onCreated={id => { setAdding(false); void service.loadLibraries(); if (id) select(id); }} />
    </>
  );
}

/** The server only ever offers the two known sources here; anything else is ignored. */
function onlineOrLocal(id: MetadataAgentId): 'online' | 'local' | undefined {
  if (id === 'online') return 'online';
  if (id === 'local') return 'local';
  return undefined;
}

/** The initial metadata language for an agent: the viewer's UI language base when listed, else the default, else the first code. */
export function initialAgentLanguage(option: MetadataAgentOption | undefined, uiBase: string): string {
  const langs = option?.languages ?? [];
  if (!langs.length) return '';
  if (langs.includes(uiBase)) return uiBase;
  if (option?.defaultLanguage && langs.includes(option.defaultLanguage)) return option.defaultLanguage;
  return langs[0]!;
}

export function AddLibraryDialog({open, onOpenChange, service, onCreated}: {open: boolean; onOpenChange: (o: boolean) => void; service: LibraryManagementService; onCreated: (libraryId?: string) => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const action = useAction();
  const [name, setName] = useState(defaultLibraryName('movie'));
  const [nameTouched, setNameTouched] = useState(false);
  const [kind, setKind] = useState<ManagedLibrary['kind']>('movie');
  const [path, setPath] = useState('');
  const [picking, setPicking] = useState(false);
  const [agent, setAgent] = useState<'online' | 'local'>('online');
  const [choices, setChoices] = useState<Partial<Record<ManagedLibrary['kind'], KindMetadataAgents>>>({});
  const [failedKinds, setFailedKinds] = useState<readonly ManagedLibrary['kind'][]>([]);
  const [language, setLanguage] = useState('en');
  const uiBase = (i18n.locale.split('-')[0] ?? 'en').toLowerCase();
  useEffect(() => {
    if (open) {
      setName(defaultLibraryName('movie'));
      setNameTouched(false);
      setKind('movie');
      setPath('');
      setAgent('online');
      setChoices({});
      setFailedKinds([]);
      setLanguage('en');
      action.clear();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  // The server's source choices for the selected kind: re-read on kind change, abort the
  // previous read, cache per kind while the dialog is open. A failed or slow read never blocks
  // adding a library: the form keeps today's fixed choices with no language control.
  useEffect(() => {
    if (!open) return;
    if (choices[kind] || failedKinds.includes(kind)) return;
    const ctl = new AbortController();
    let live = true;
    api.request<unknown>(kindMetadataAgentsPath(kind), 'GET', undefined, ctl.signal).then(
      raw => {
        if (!live) return;
        try {
          const doc = parseKindMetadataAgents(raw);
          setChoices(prev => ({...prev, [kind]: doc}));
        } catch {
          if (live) setFailedKinds(prev => (prev.includes(kind) ? prev : [...prev, kind]));
        }
      },
      e => {
        if (!live) return;
        if ((e as {name?: unknown} | null)?.name === 'AbortError') return;
        setFailedKinds(prev => (prev.includes(kind) ? prev : [...prev, kind]));
      },
    );
    return () => { live = false; ctl.abort(); };
  }, [open, kind, api, choices, failedKinds]);
  const doc = open ? choices[kind] : undefined;
  const chosenOption = doc?.agents.find(a => a.id === agent);
  const showLanguage = !!doc && (chosenOption?.languages.length ?? 0) > 0;
  const languageCodes = showLanguage && chosenOption ? [...chosenOption.languages] : [];
  // Changing the kind or the source resets the language the same way (base, default, first).
  useEffect(() => {
    if (!open || !doc) return;
    const option = doc.agents.find(a => a.id === agent);
    if (!option || !option.languages.length) return;
    setLanguage(initialAgentLanguage(option, uiBase));
  }, [open, doc, agent, uiBase]);
  const languageName = (code: string): string => {
    const id = `language.${code}`;
    try {
      if (i18n.has(id)) {
        const v = t(id as MessageId);
        if (v) return v;
      }
    } catch { /* fall back to the code, never Intl.DisplayNames */ }
    return code;
  };
  const effectiveLanguage = showLanguage && chosenOption ? (languageCodes.includes(language) ? language : initialAgentLanguage(chosenOption, uiBase)) : '';
  const pickKind = (next: ManagedLibrary['kind']) => {
    setKind(next);
    if (!nameTouched) setName(defaultLibraryName(next));
    const nextDoc = choices[next];
    const nextOption = nextDoc?.agents.find(a => a.id === agent) ?? nextDoc?.agents[0];
    const nextId = nextOption ? onlineOrLocal(nextOption.id) : undefined;
    if (nextDoc && nextOption && nextId) {
      if (nextId !== agent) setAgent(nextId);
      if (nextOption.languages.length) setLanguage(initialAgentLanguage(nextOption, uiBase));
    }
  };
  const pickAgent = (next: MetadataAgentId) => {
    const id = onlineOrLocal(next);
    if (!id) return;
    setAgent(id);
    if (doc) {
      const option = doc.agents.find(a => a.id === id);
      if (option && option.languages.length) setLanguage(initialAgentLanguage(option, uiBase));
    }
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={t('web.libraries.addTitle')} description={t('web.libraries.addLede')} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => onOpenChange(false)} /><Button variant="primary" label={t('web.empty.addLibrary')} loading={action.busy} disabled={!name.trim() || !path.trim()} onClick={() => { let created: string | undefined; void action.run(async () => { created = await createLibraryAndScan(service, {name: name.trim(), kind, path: path.trim(), metadataAgent: agent, ...(showLanguage && effectiveLanguage ? {metadataLanguage: effectiveLanguage} : {})}); }).then(ok => ok && onCreated(created)); }} /></>}>
      <Input label={t('profile.name')} value={name} onChange={e => { setName(e.target.value); setNameTouched(true); }} maxLength={100} autoFocus />
      <div role="radiogroup" aria-label={t('web.libraries.type')} style={{display: 'flex', flexDirection: 'column', gap: 8}}>
        <Text as="span" variant="label" tone="secondary">{t('web.libraries.type')}</Text>
        {kinds.map(k => <Checkbox key={k.value} radio checked={kind === k.value} onCheckedChange={v => { if (v) pickKind(k.value); }} label={<span style={{display: 'inline-flex', alignItems: 'center', gap: 8}}><Icon name={iconFor(k.value)} size={18} />{t(k.label)}</span>} help={t(k.description)} />)}
        <Text as="span" variant="caption" tone="tertiary">{t('web.libraries.kindHelp')}</Text>
      </div>
      <Input label={t('web.libraries.serverFolder')} value={path} onChange={e => setPath(e.target.value)} placeholder="/Volumes/Media/Movies" mono help={t('web.libraries.serverFolderHelp')} />
      <div><Button size="sm" variant="secondary" icon="folder" label={t('server.chooseFolder')} onClick={() => setPicking(true)} /></div>
      <FolderPicker open={picking} onOpenChange={setPicking} start={path} onPick={setPath} />
      {doc ? (
        <div role="radiogroup" aria-label={t('web.metadataSource.addLabel')} style={{display: 'flex', flexDirection: 'column', gap: 8}}>
          <Text as="span" variant="label" tone="secondary">{t('web.metadataSource.addLabel')}</Text>
          {doc.agents.map(a => <Checkbox key={a.id} radio checked={agent === a.id} onCheckedChange={v => { if (v) pickAgent(a.id); }} label={a.name} help={a.description} />)}
          <Text as="span" variant="caption" tone="tertiary">{t('web.metadataSource.addHelper')}</Text>
        </div>
      ) : (
        <MetadataSourceChoice value={agent} onChange={pickAgent} />
      )}
      {showLanguage && chosenOption ? <Select label={t('web.libraries.metadataLanguage')} help={t('web.libraries.metadataLanguageHelp')} value={effectiveLanguage} options={languageCodes.map(code => ({value: code, label: languageName(code)}))} onChange={e => setLanguage(e.target.value)} /> : null}
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
    </Dialog>
  );
}

function LibraryDetail({service, snapshot, libraryId, onBack}: {service: LibraryManagementService; snapshot: LibraryManagementSnapshot; libraryId: string; onBack: () => void}) {
  const {api} = useSession();
  const scope = useViewerScope();
  const inventory = useService<LibraryInventoryService, LibraryInventorySnapshot>(() => new LibraryInventoryService({api, scope, libraryId}), [api, scope, libraryId]);
  useEffect(() => {
    void inventory.service.refresh();
  }, [inventory.service]);
  const action = useAction();
  const library = snapshot.selected.data?.library;
  const search = useSearch({from: '/app/settings/$section'});
  const navigate = useNavigate();
  const tab: LibraryTab = search.tab === 'metadata' || search.tab === 'scanning' || search.tab === 'advanced' ? search.tab : 'general';
  const setTab = (id: string) => void navigate({to: '.', search: {id: libraryId, ...(id === 'general' ? {} : {tab: id})}});
  const lookup = useScreenLookup(libraryId, [tab]);
  const [rename, setRename] = useState<string | null>(null);
  const [editSource, setEditSource] = useState<{id?: string; settings: SourceSettings} | null>(null);
  const [removeSource, setRemoveSource] = useState<{id: string; name: string; revision: number} | null>(null);
  const config = inventory.snapshot.config;
  const scan = library?.lastScan;
  const {t} = useI18n();
  const [issuesOpen, setIssuesOpen] = useState(false);
  const menu: MenuItem[] = [{id: 'rename', label: t('web.libraries.rename'), icon: 'edit'}, ...(library?.actions.includes('scan') ? [{id: 'scan-all', label: t('web.libraries.scanAllFiles'), icon: 'refresh' as const}] : []), ...(library?.kind === 'music' ? [{id: 'lyrics', label: t('web.libraries.fetchLyrics'), icon: 'lyrics' as const}] : [])];
  return (
    <>
      <div>
        <Button variant="link" size="sm" icon="back" label={t('web.console.nav.libraries')} onClick={onBack} />
      </div>
      {!library ? (snapshot.selected.phase === 'loading' ? <Loading label={t('web.libraries.loadingOne')} /> : snapshot.selected.phase === 'idle' && !snapshot.selected.error ? <Loading label={t('web.libraries.loadingOne')} /> : <Notice tone="error" action={{label: t('action.tryAgain'), onClick: () => void service.selectLibrary(libraryId).catch(() => {})}}>{snapshot.selected.error ? problem(snapshot.selected.error) : t('web.libraries.loadFailed')}</Notice>) : (
        <div style={{display: 'flex', flexDirection: 'column', gap: 28}}>
          <div style={{display: 'flex', alignItems: 'flex-end', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap'}}>
            <div>
              <Text as="h1" variant="title">{library.name}</Text>
              <Text as="p" variant="body" tone="secondary">{libraryKindLabel(library.kind)}</Text>
            </div>
            <div style={{display: 'flex', gap: 8}}>
              <Button variant="primary" icon="refresh" label={scan?.status === 'running' ? t('web.libraries.scanningNow') : t('web.libraries.scanNow')} disabled={!library.actions.includes('scan') || scan?.status === 'running'} loading={action.busy && snapshot.mutation.kind === 'scan'} onClick={() => void action.run(() => service.scan(), t('web.library.scanStarted'))} />
              <Menu label={t('player.more')} trigger={<Button variant="secondary" icon="more" aria-label={t('player.more')} />} items={menu} onSelect={id => { if (id === 'rename') setRename(library.name); if (id === 'scan-all') void action.run(() => service.scan('full'), t('web.libraries.scanAllStarted')); if (id === 'lyrics') void action.run(() => api.request('/v1/libraries/' + encodeURIComponent(library.id) + '/lyrics/fetch', 'POST', {idempotencyKey: crypto.randomUUID()}), 'Lyric fetch queued. Progress shows under Scheduled tasks.'); }} />
            </div>
          </div>
          {/* A library is a page with tabs, not a dialog (Justin, 2 Oct 2026). */}
          <Tabs label={library.name} items={[{id: 'general', label: t('server.library.tab.general')}, {id: 'metadata', label: t('server.library.tab.metadata')}, {id: 'scanning', label: t('server.library.tab.scanning')}, {id: 'advanced', label: t('settings.section.advanced')}]} value={tab} onChange={setTab} />
          {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
          {tab === 'general' ? <SettingsGroup>
            <SettingsRow label={t('profile.name')} meta={library.name} control={<Button size="sm" variant="ghost" label={t('web.libraries.rename')} onClick={() => setRename(library.name)} />} />
            <SettingsRow label={t('server.library.type')} meta={libraryKindLabel(library.kind)} />
          </SettingsGroup> : null}
          {tab === 'scanning' ? <SettingsGroup title={t('web.libraries.scan')} description={t('web.libraries.scanLede')}>
            <SettingsRow label={t('web.libraries.lastScan')} state={scan ? `${jobLabel[scan.status]} · ${scan.processed.toLocaleString()} files${scan.warnings ? ` · ${scan.warnings} warnings` : ''}${scan.finishedAt ? ` · finished ${new Date(scan.finishedAt).toLocaleString()}` : scan.updatedAt ? ` · updated ${new Date(scan.updatedAt).toLocaleString()}` : ''}${scan.error ? ` · ${scan.error}` : ''}` : t('web.libraries.notScannedYet')} control={scan && scan.status === 'running' && scan.actions.includes('cancel') ? <Button size="sm" variant="outline" label={t('web.libraries.cancelScan')} onClick={() => void action.run(() => service.cancelJob(scan.id))} /> : scan?.status === 'paused' && scan.actions.includes('resume') ? <Button size="sm" variant="outline" label={t('entry.resume')} onClick={() => void action.run(() => service.controlJob(scan.id, 'resume'))} /> : scan?.status === 'failed' && scan.actions.includes('retry') ? <Button size="sm" variant="outline" label={t('action.tryAgain')} onClick={() => void action.run(() => service.controlJob(scan.id, 'retry'))} /> : undefined} meta={scan ? <Badge tone={jobTone(scan.status)} dot={scan.status === 'running'} live={scan.status === 'running'}>{jobLabel[scan.status]}</Badge> : undefined} />
            {library.attention ? <SettingsRow icon="warning" label={library.attention.atLeast ? t('web.libraries.attentionAtLeast', {count: library.attention.files.toLocaleString()}) : t('web.libraries.attention', {count: library.attention.files})} help={t('web.libraries.attentionHelp')} control={<Button size="sm" variant="outline" label={t('web.libraries.attentionReview')} onClick={() => setIssuesOpen(true)} />} /> : null}
            {config ? <ScanPolicyRow inventory={inventory.service} snapshot={inventory.snapshot} /> : null}
            {lookup.data ? <LookupStatusRow policy={lookup.data} onChange={() => setTab('metadata')} /> : null}
            {/* When each folder is looked at again by itself: its schedule. */}
            {config?.sources.map(src => <SettingsRow key={src.id} icon="folder" label={t('server.library.rescan', {folder: src.name || src.path})} meta={rescanLabel(t, src.intervalSeconds)} control={<Button size="sm" variant="ghost" label={t('web.selection.edit')} onClick={() => setEditSource({id: src.id, settings: {name: src.name, path: src.path, classification: src.classification, followSymlinks: src.followSymlinks, missingGraceSeconds: src.missingGraceSeconds, intervalSeconds: src.intervalSeconds, expectedRevision: config.revision, acceptReplacement: false}})} />} />)}
          </SettingsGroup> : null}
          <LibrarySettingsSections library={{id: library.id, name: library.name}} tab={tab} />
          {tab === 'general' ? <SettingsGroup title={t('web.libraries.folders')} description={t('web.libraries.foldersLede')} action={<Button size="sm" variant="secondary" icon="plus" label={t('web.libraries.addFolder')} disabled={!canAddSource(config?.revision)} onClick={() => config && setEditSource({settings: addSourceSettings(config.revision)})} />}>
            {inventory.snapshot.error ? <div style={{padding: 12}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: () => void inventory.service.refresh()}}>{problem(inventory.snapshot.error)}</Notice></div> : null}
            {!config && !inventory.snapshot.error ? <div style={{padding: 12}}><Loading label={t('web.libraries.loadingFolders')} /></div> : null}
            {config && !config.sources.length ? <SettingsRow label={t('web.libraries.noFolders')} help={t('web.libraries.noFoldersHelp')} /> : null}
            {config?.sources.map(src => (
              <SettingsRow key={src.id} icon="folder" label={src.name || src.path} help={<><span style={{fontFamily: 'var(--font-mono)', fontSize: 12}}>{src.path}</span>{src.classification === 'network' ? ' · Network' : ''}{src.lastCompleteAt ? ` · Scanned ${new Date(src.lastCompleteAt).toLocaleString()}` : ''}</>} meta={<Badge tone={src.health === 'healthy' || src.health === 'available' ? 'healthy' : src.health === 'unknown' ? 'neutral' : 'warning'} dot>{sentence(src.health || 'unknown')}{!src.enabled ? ' · Paused' : ''}</Badge>} control={<Menu label={t('web.libraries.folderMenu', {name: src.name})} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('web.libraries.folderActions')} />} items={[{id: 'scan', label: t('web.libraries.scanFolder'), icon: 'refresh'}, {id: 'check', label: t('web.libraries.checkAvailability'), icon: 'pulse'}, {id: 'edit', label: t('web.selection.edit'), icon: 'edit'}, {id: 'remove', label: t('web.libraries.removeFolder'), icon: 'trash', destructive: true, separatorBefore: true}]} onSelect={id => {
                if (id === 'scan') void action.run(() => inventory.service.scanSource(src.id), 'Scan started.');
                else if (id === 'check') void action.run(() => inventory.service.checkSource(src.id), 'Checked.');
                else if (id === 'edit') setEditSource({id: src.id, settings: {name: src.name, path: src.path, classification: src.classification, followSymlinks: src.followSymlinks, missingGraceSeconds: src.missingGraceSeconds, intervalSeconds: src.intervalSeconds, expectedRevision: config.revision, acceptReplacement: false}});
                else if (id === 'remove') setRemoveSource({id: src.id, name: src.name || src.path, revision: config.revision});
              }} />} />
            ))}
          </SettingsGroup> : null}
          {/* Remove library returns with DELETE /v1/admin/libraries/{id} (lane C, BE-SRV-20); no dead control until then (WEB-SRV-04). */}
        </div>
      )}
      <Dialog open={rename !== null} onOpenChange={o => !o && setRename(null)} title={t('web.libraries.renameTitle')} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => setRename(null)} /><Button variant="primary" label={t('web.libraries.rename')} loading={action.busy} disabled={!rename?.trim()} onClick={() => library && void action.run(() => service.rename(rename!.trim(), library.revision)).then(ok => ok && setRename(null))} /></>}>
        <Input label={t('profile.name')} value={rename ?? ''} onChange={e => setRename(e.target.value)} maxLength={100} autoFocus />
      </Dialog>
      {issuesOpen && library ? <EpisodeIssuesDialog libraryId={library.id} onClose={() => setIssuesOpen(false)} /> : null}
      <SourceDialog value={editSource} onClose={() => setEditSource(null)} onSave={(id, settings) => void action.run(() => inventory.service.saveSource(id, settings), 'Saved.').then(ok => { if (ok) setEditSource(null); void inventory.service.refresh(); })} busy={action.busy} error={action.error} />
      <ConfirmDialog open={!!removeSource} onOpenChange={o => !o && setRemoveSource(null)} title={t('web.libraries.removeFolderTitle', {name: removeSource?.name ?? ''})} body={t('web.libraries.removeFolderBody')} confirmLabel={t('web.libraries.removeFolder')} busy={action.busy} onConfirm={() => { if (removeSource) void action.run(() => inventory.service.removeSource(removeSource.id, removeSource.revision)).then(ok => { if (ok) { setRemoveSource(null); void inventory.service.refresh(); } }); }} />
    </>
  );
}

/** A folder's own rescan interval in words. */
function rescanLabel(t: ReturnType<typeof useI18n>['t'], seconds: number): string {
  return seconds === 900 ? t('web.libraries.every15m') : seconds === 3600 ? t('web.libraries.everyHour') : seconds === 21600 ? t('web.libraries.every6h') : seconds === 86400 ? t('web.libraries.daily') : seconds === 0 ? t('web.libraries.onlyWhenAsked') : t('server.library.everyMinutes', {count: Math.round(seconds / 60)});
}

/** The files a scan couldn't place (GET /v1/libraries/{id}/episode-issues), a page at a time. */
function EpisodeIssuesDialog({libraryId, onClose}: {libraryId: string; onClose: () => void}) {
  const {api} = useSession();
  const {t} = useI18n();
  const [issues, setIssues] = useState<readonly EpisodeIssue[]>([]);
  const [cursor, setCursor] = useState<string | null>('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const load = (from: string) => {
    setLoading(true); setError(null);
    fetchEpisodeIssues(api, libraryId, from).then(page => { setIssues(prev => (from ? [...prev, ...page.issues] : page.issues)); setCursor(page.nextCursor || null); }, setError).finally(() => setLoading(false));
  };
  useEffect(() => { load(''); }, [api, libraryId]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('web.libraries.attentionTitle')} description={t('web.libraries.attentionHelp')} width={560} actions={<Button variant="ghost" label={t('action.close')} onClick={onClose} />}>
      {error ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: () => load(issues.length && cursor ? cursor : '')}}>{problem(error)}</Notice> : null}
      {!issues.length && loading ? <Loading /> : null}
      {!issues.length && !loading && !error ? <Text variant="body" tone="secondary">{t('web.libraries.attentionEmpty')}</Text> : null}
      {issues.length ? <div style={{display: 'grid', gap: 8, maxHeight: 360, overflowY: 'auto'}}>{issues.map(x => <Text key={x.assetId} variant="caption" style={{fontFamily: 'var(--font-mono)', wordBreak: 'break-all'}}>{x.sourceName}</Text>)}</div> : null}
      {cursor && issues.length ? <div><Button size="sm" variant="ghost" label={t('action.showMore')} loading={loading} onClick={() => load(cursor)} /></div> : null}
    </Dialog>
  );
}

function ScanPolicyRow({inventory, snapshot}: {inventory: LibraryInventoryService; snapshot: LibraryInventorySnapshot}) {
  const {t} = useI18n();
  const policy = snapshot.config!.policy;
  const [tier, setTier] = useState<ScanTier>(policy.tier);
  const [ops, setOps] = useState<string[]>([...policy.operations]);
  useEffect(() => {
    setTier(policy.tier);
    setOps([...policy.operations]);
  }, [policy]);
  const dirty = tier !== policy.tier || (tier === 'custom' && JSON.stringify([...ops].sort()) !== JSON.stringify([...policy.operations].sort()));
  const inline = useInlineForm('scan-policy', {
    dirty,
    discard: () => { setTier(policy.tier); setOps([...policy.operations]); },
    save: async () => { await inventory.setPolicy({tier, operations: tier === 'custom' ? (ops as typeof policy.operations) : policy.operations, expectedRevision: snapshot.config!.revision}); void inventory.refresh(); },
  });
  return (
    <>
      <SettingsRow label={t('web.libraries.scanDepth')} help={tierHelp[tier]} control={<Select hideLabel label={t('web.libraries.scanDepth')} value={tier} options={(['file_list_only', 'basic', 'complete', 'custom'] as ScanTier[]).map(t => ({value: t, label: tierLabel[t]}))} onChange={e => setTier(e.target.value as ScanTier)} />} />
      {tier === 'custom' ? <SettingsRow label={t('web.libraries.steps')} stack control={<div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: 8}}>{scanOperations.map(op => <Checkbox key={op} checked={ops.includes(op)} onCheckedChange={v => setOps(prev => (v ? [...prev, op] : prev.filter(x => x !== op)))} label={opLabel[op] ?? sentence(op)} />)}</div>} start /> : null}
      {inline.error ? <div style={{padding: '0 16px 12px'}}><Notice tone="error" compact>{problem(inline.error, 'action')}</Notice></div> : null}
    </>
  );
}

function SourceDialog({value, onClose, onSave, busy, error}: {value: {id?: string; settings: SourceSettings} | null; onClose: () => void; onSave: (id: string | undefined, settings: SourceSettings) => void; busy: boolean; error: string}) {
  const {t} = useI18n();
  const [draft, setDraft] = useState<SourceSettings | null>(null);
  useEffect(() => setDraft(value?.settings ?? null), [value]);
  const [picking, setPicking] = useState(false);
  if (!value || !draft) return null;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={value.id ? t('web.libraries.editFolder') : t('web.libraries.addFolder')} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={value.id ? t('action.save') : t('web.libraries.addFolder')} loading={busy} disabled={!draft.path.trim()} onClick={() => onSave(value.id, {...draft, name: draft.name.trim() || draft.path.trim(), path: draft.path.trim()})} /></>}>
      <Input label={t('web.libraries.serverFolder')} value={draft.path} onChange={e => setDraft({...draft, path: e.target.value})} mono placeholder="/Volumes/Media/Movies" autoFocus />
      <div><Button size="sm" variant="secondary" icon="folder" label={t('server.chooseFolder')} onClick={() => setPicking(true)} /></div>
      <FolderPicker open={picking} onOpenChange={setPicking} start={draft.path} onPick={path => setDraft({...draft, path})} />
      <Input label={t('profile.name')} optional value={draft.name} onChange={e => setDraft({...draft, name: e.target.value})} maxLength={100} help={t('web.libraries.folderNameHelp')} />
      <Select label={t('web.libraries.location')} value={draft.classification} options={[{value: 'local', label: t('web.libraries.localDisk')}, {value: 'network', label: t('web.libraries.networkShare')}]} onChange={e => setDraft({...draft, classification: e.target.value as 'local' | 'network'})} help={t('web.libraries.locationHelp')} />
      <Checkbox checked={draft.followSymlinks} onCheckedChange={v => setDraft({...draft, followSymlinks: v})} label={t('web.libraries.followSymlinks')} />
      <Select label={t('web.libraries.rescanInterval')} value={String(draft.intervalSeconds)} options={[{value: '900', label: t('web.libraries.every15m')}, {value: '3600', label: t('web.libraries.everyHour')}, {value: '21600', label: t('web.libraries.every6h')}, {value: '86400', label: t('web.libraries.daily')}, {value: '0', label: t('web.libraries.onlyWhenAsked')}]} onChange={e => setDraft({...draft, intervalSeconds: Number(e.target.value)})} />
      <Select label={t('web.libraries.missingAfter')} value={String(draft.missingGraceSeconds)} options={[{value: '3600', label: t('web.telemetry.window1h')}, {value: '86400', label: t('web.libraries.graceDay')}, {value: '604800', label: t('web.libraries.graceWeek')}]} onChange={e => setDraft({...draft, missingGraceSeconds: Number(e.target.value)})} help={t('web.libraries.missingAfterHelp')} />
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
    </Dialog>
  );
}
