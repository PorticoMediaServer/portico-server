import {useEffect, useMemo, useState} from 'react';
import {parseEnvelope, parseFilesystemPage, parseLibraryDocument, providerChoices, type FilesystemPage, type LibrarySettings} from '@core/administration.ts';
import {setLibraryAnalysis} from '../../admin/library-policy';
import {filesystemPath, mergeFilesystemPages} from '@core/server-admin/panel-logic.ts';
export {filesystemPath, mergeFilesystemPages} from '@core/server-admin/panel-logic.ts';
import {problem, useRead} from '../../admin/console';
import {useInlineForm} from '../settings/ServerForms';
import {createOperationIds} from './operation-ids';
import {useSession} from '../../app/session';
import {currentI18n, type MessageId} from '../../app/i18n';
import {useScreenLookup} from './MetadataLookup';
import {MetadataSourceGroup, useMetadataAgent} from './MetadataSource';
import {Button, Dialog, Input, ListRow, Loading, Notice, Select, SettingsGroup, SettingsRow, Surface, Switch, Text} from '../../ui';

function useServerId() { const {system, session} = useSession(); return system?.id ?? session?.viewer.serverId ?? ''; }

/** CD-43: the server caps roots at 64 and marks the page truncated. The roots are a window, never
 * the exhaustive set — an explicit typed path can still reach any other allowed media root. */
export function areFilesystemRootsPartial(page: FilesystemPage | undefined): boolean {
  return !!page?.truncated;
}

/** Browse folders on the server's own disks, so an owner picks a folder instead of typing a
 * path the server may not be able to read. */
export function FolderPicker({open, onOpenChange, onPick, start = ''}: {open: boolean; onOpenChange: (open: boolean) => void; onPick: (path: string) => void; start?: string}) {
  const {api} = useSession();
  const t = currentI18n().t;
  const serverId = useServerId();
  const [path, setPath] = useState(start);
  const [page, setPage] = useState<FilesystemPage>();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [typed, setTyped] = useState(start);
  const [more, setMore] = useState(false);
  useEffect(() => { if (page?.path) setTyped(page.path); }, [page?.path]);
  // CD-05: the server pages a folder (at most 200 entries a page, `nextCursor` for the rest).
  const read = (cursor = '') => api.request<unknown>(filesystemPath(path, cursor)).then(raw => parseEnvelope(raw, serverId, parseFilesystemPage).result);
  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true); setError('');
    read()
      .then(next => { if (active) setPage(next); }, e => { if (active) setError(problem(e) || 'That folder could not be opened.'); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, path, api, serverId]);
  const loadMore = () => {
    if (!page?.nextCursor || more) return;
    const current = page;
    setMore(true);
    read(current.nextCursor)
      .then(next => setPage(p => (p === current && next.path === current.path ? mergeFilesystemPages(current, next) : p)), e => setError(problem(e) || 'That folder could not be opened.'))
      .finally(() => setMore(false));
  };
  const folders = page?.entries.filter(e => e.kind === 'directory') ?? [];
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={t('web.folderPicker.title')} description={page?.path || t('web.folderPicker.onServer')} width={560}
      actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => onOpenChange(false)} /><Button variant="primary" label={t('web.folderPicker.useFolder')} disabled={!page?.path} onClick={() => { if (page?.path) { onPick(page.path); onOpenChange(false); } }} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        <form onSubmit={e => { e.preventDefault(); const next = typed.trim(); if (next) setPath(next); }}>
          <Input label={t('web.folderPicker.path')} hideLabel mono value={typed} onChange={e => setTyped(e.target.value)} placeholder="/media/movies" />
        </form>
        {error ? <Notice tone="error" compact action={{label: t('action.back'), onClick: () => setPath(page?.parent ?? '')}}>{error}</Notice> : null}
        <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
          {page?.parent && page.parent !== page.path ? <Button size="sm" variant="secondary" icon="back" label={t('web.folderPicker.up')} onClick={() => setPath(page.parent)} /> : null}
          {page?.roots.map(r => <Button key={r.path} size="sm" variant="ghost" label={r.name || r.path} onClick={() => setPath(r.path)} />)}
        </div>
        {areFilesystemRootsPartial(page) ? <Text variant="caption" tone="tertiary">{currentI18n().t('web.folderPicker.rootsTruncated', {count: page?.roots.length ?? 0})}</Text> : null}
        <Surface padless style={{maxHeight: 340, overflow: 'auto'}}>
          {loading && !page ? <div style={{padding: 16}}><Loading label={t('web.folderPicker.reading')} /></div> : null}
          {folders.map(f => <ListRow key={f.path} icon="folder" title={f.name} meta={!f.readable ? t('web.folderPicker.noAccess') : f.symlink ? t('web.folderPicker.symlink') : undefined} onClick={f.readable ? () => setPath(f.path) : undefined} />)}
          {page && !folders.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.folderPicker.noFolders')}</Text></div> : null}
        </Surface>
        {page?.nextCursor ? <div><Button size="sm" variant="ghost" label={currentI18n().t('web.folderPicker.more')} loading={more} onClick={loadMore} /></div> : null}
        {page?.truncated && !page.nextCursor ? <Text variant="caption" tone="tertiary">{t('web.folderPicker.moreEntries')}</Text> : null}
      </div>
    </Dialog>
  );
}

const kindLabel: Record<string, string> = {movie: 'Movies', show: 'Shows', episode: 'Episodes', music: 'Music', book: 'Audiobooks', audiobook: 'Audiobooks', anime: 'Anime', photo: 'Photos'};

export type LibraryTab = 'general' | 'metadata' | 'scanning' | 'advanced';

/**
 * How one library is matched, analysed and cleaned up, as the sections of its page's tabs
 * (Metadata · Scanning · Advanced). One settings document behind all three: it stays mounted
 * while the tabs change, and its draft is saved by the page's one Save.
 */
export function LibrarySettingsSections({library, tab}: {library: {id: string; name: string}; tab: LibraryTab}) {
  const {api} = useSession();
  const t = currentI18n().t;
  const serverId = useServerId();
  const read = useRead(async () => parseEnvelope(await api.request<unknown>(`/v1/admin/libraries/${encodeURIComponent(library.id)}/settings`), serverId, parseLibraryDocument).result, [api, library.id, serverId]);
  const [draft, setDraft] = useState<LibrarySettings>();
  const [keys, setKeys] = useState<Record<string, string>>({});
  const doc = read.data;
  useEffect(() => { if (doc) { setDraft(doc.settings); setKeys({}); } }, [doc]);
  const byCost = useMemo(() => (doc ? [...doc.analysisMatrix.costClasses].sort((a, b) => a.order - b.order).map(c => ({cost: c, operations: doc.analysisMatrix.operations.filter(o => o.costClass === c.id)})).filter(g => g.operations.length) : []), [doc]);
  // The metadata source (online or local only) is its own resource with its own revision, saved
  // when the owner confirms the change; the film/TV lookup policy says whether lookups wait for consent.
  const agent = useMetadataAgent(library.id);
  const screen = useScreenLookup(library.id);
  const local = agent.data?.agent === 'local';
  // The agent lists the metadata languages its source can fetch (MU12). A stored tag from
  // before the list (for example `en-US`) stays selectable so saving unchanged keeps it.
  const agentDoc = agent.data;
  const agentLanguages = (agentDoc ? agentDoc.agents.find(a => a.id === agentDoc.agent)?.languages : undefined) ?? [];
  const languageName = (code: string): string => {
    const id = `language.${code}`;
    try {
      if (currentI18n().has(id)) {
        const v = currentI18n().t(id as MessageId);
        if (v) return v;
      }
    } catch { /* fall back to the code, never Intl.DisplayNames */ }
    return code;
  };
  const languageOptions = (stored: string): readonly {value: string; label: string}[] => {
    const listed = agentLanguages.map(code => ({value: code, label: languageName(code)}));
    if (stored && !agentLanguages.includes(stored)) {
      const base = stored.split('-')[0]?.toLowerCase() ?? '';
      const baseId = `language.${base}`;
      let baseName = '';
      try { if (base && currentI18n().has(baseId)) baseName = currentI18n().t(baseId as MessageId); } catch { baseName = ''; }
      return [{value: stored, label: baseName ? `${baseName} (${stored})` : stored}, ...listed];
    }
    return listed;
  };
  const settingsDirty = !!draft && !!doc && (JSON.stringify(draft) !== JSON.stringify(doc.settings) || Object.values(keys).some(Boolean));
  const dirty = settingsDirty;
  const toggle = (id: string, on: boolean) => {
    if (!draft || !doc) return;
    setDraft(setLibraryAnalysis(draft, doc.analysisMatrix, id, on));
  };
  // BE-API-10: one operation ID per logical save; a retry of the same draft
  // reuses it, an edited draft mints a new one.
  const [opIds] = useState(createOperationIds);
  const inline = useInlineForm('library-settings:' + library.id, {
    dirty,
    discard: () => { if (doc) { setDraft(doc.settings); setKeys({}); } },
    save: async () => {
      if (!draft || !doc) return;
      const settings = {...draft, providers: draft.providers.map(p => ({...p, apiKey: keys[p.mediaKind] ?? ''}))};
      const key = JSON.stringify({revision: doc.revision, settings});
      await api.request(`/v1/admin/libraries/${encodeURIComponent(library.id)}/settings`, 'PUT', {expectedRevision: doc.revision, operationId: opIds.forPayload(key), settings});
      opIds.release();
      read.reload();
    },
  });
  if (tab === 'general') return null;
  return (
    <>
      {read.error && !doc ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice> : null}
      {!doc && !read.error ? <Loading label={t('web.librarySettings.loading')} /> : null}
      {doc && draft ? (
        <div style={{display: 'flex', flexDirection: 'column', gap: 20}}>
          {inline.error ? <Notice tone="error" compact>{problem(inline.error, 'action')}</Notice> : null}
          {tab === 'metadata' ? <MetadataSourceGroup libraryId={library.id} agent={agent} screen={screen.data} onSaved={() => { screen.reload(); read.reload(); }} /> : null}
          {/* The online providers only matter while the library looks titles up online. */}
          {tab === 'metadata' && !local && draft.providers.length ? <SettingsGroup title={t('web.librarySettings.matching')} description={t('web.librarySettings.matchingLede')}>
            {draft.providers.map((p, i) => {
              const choices = providerChoices(doc.availableProviders, p);
              const chosen = choices.find(c => c.id === p.provider);
              const change = (next: Partial<typeof p>) => setDraft({...draft, providers: draft.providers.map((x, n) => (n === i ? {...x, ...next} : x))});
              return (
                <SettingsRow key={`${i}:${p.mediaKind}`} label={kindLabel[p.mediaKind] ?? p.mediaKind} help={chosen?.attributionNote} stack control={
                  <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: 8, width: '100%'}}>
                    <Select label={t('web.librarySettings.source')} value={p.provider} onChange={e => change({provider: e.target.value as typeof p.provider})} options={choices.map(c => ({value: c.id, label: c.name}))} />
                    {chosen?.supportsLocale ? (agentLanguages.length ? <Select label={t('web.librarySettings.language')} value={p.language || agentLanguages[0]} options={languageOptions(p.language)} onChange={e => change({language: e.target.value})} /> : <Input label={t('web.librarySettings.language')} placeholder={t('web.librarySettings.languageExample')} value={p.language} onChange={e => change({language: e.target.value})} />) : null}
                    {chosen?.supportsLocale ? <Input label={t('web.librarySettings.country')} placeholder={t('web.librarySettings.countryExample')} value={p.region} onChange={e => change({region: e.target.value.toUpperCase()})} /> : null}
                    {chosen?.supportsApiKey ? <Input label={t('web.librarySettings.apiKey')} type="password" autoComplete="off" placeholder={p.apiKeySet ? t('web.librarySettings.apiKeySet') : t('web.librarySettings.apiKeyOptional')} value={keys[p.mediaKind] ?? ''} onChange={e => setKeys({...keys, [p.mediaKind]: e.target.value})} /> : null}
                  </div>} />
              );
            })}
          </SettingsGroup> : null}
          {tab === 'scanning' ? byCost.map(g => (
            <SettingsGroup key={g.cost.id} title={g.cost.name} description={g.cost.description}>
              {g.operations.map(o => <SettingsRow key={o.id} label={o.name} help={`${o.description}${o.requires.length ? ` Needs: ${o.requires.map(r => doc.analysisMatrix.operations.find(x => x.id === r)?.name ?? r).join(', ')}.` : ''}`} control={<Switch checked={draft.analysis.includes(o.id)} onCheckedChange={v => toggle(o.id, v)} label={o.name} />} />)}
            </SettingsGroup>
          )) : null}
          {tab === 'scanning' ? <SettingsGroup title={t('web.librarySettings.trickplay')} description={t('web.librarySettings.trickplayLede')}>
            <SettingsRow label={t('web.librarySettings.pictureEvery')} help={t('web.librarySettings.pictureEveryHelp')} control={<Input hideLabel label={t('web.librarySettings.pictureEveryLabel')} type="number" min={1} max={60} value={draft.navigation.trickplayIntervalSeconds} onChange={e => setDraft({...draft, navigation: {...draft.navigation, trickplayIntervalSeconds: Math.max(1, Number(e.target.value) || 1)}})} style={{width: 90}} />} />
            <SettingsRow label={t('web.librarySettings.pictureWidth')} help={t('web.librarySettings.pictureWidthHelp')} control={<Input hideLabel label={t('web.librarySettings.pictureWidth')} type="number" min={80} max={640} step={20} value={draft.navigation.trickplayTileWidth} onChange={e => setDraft({...draft, navigation: {...draft.navigation, trickplayTileWidth: Math.max(80, Number(e.target.value) || 80)}})} style={{width: 90}} />} />
            <SettingsRow label={t('web.librarySettings.chapters')} control={<Select hideLabel label={t('web.librarySettings.chapters')} value={draft.navigation.chapterThumbnailMode} onChange={e => toggle('chapter_images', e.target.value === 'generated')} options={(doc.enumerations.chapterThumbnailMode ?? ['embedded', 'generated']).map(m => ({value: m, label: m === 'none' ? 'Off' : m === 'embedded' ? 'From the file' : 'Made by the server'}))} />} />
          </SettingsGroup> : null}
          {tab === 'advanced' ? <SettingsGroup title={t('web.librarySettings.deleting')} description={t('web.librarySettings.deletingLede')}>
            <SettingsRow label={t('web.librarySettings.allowDelete')} control={<Switch checked={draft.allowMediaDeletion} onCheckedChange={v => setDraft({...draft, allowMediaDeletion: v})} label={t('web.librarySettings.allowDelete')} />} />
            <SettingsRow label={t('web.librarySettings.keepDeleted')} help={t('web.librarySettings.keepDeletedHelp')} control={<Input hideLabel label={t('web.librarySettings.keepDeletedLabel')} type="number" min={0} max={3650} value={draft.trashRetentionDays} disabled={!draft.allowMediaDeletion} onChange={e => setDraft({...draft, trashRetentionDays: Math.max(0, Number(e.target.value) || 0)})} style={{width: 90}} />} />
          </SettingsGroup> : null}
        </div>
      ) : null}
    </>
  );
}
