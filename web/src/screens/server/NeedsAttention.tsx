import {useEffect, useMemo, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {metadataAttentionService, type AttentionCandidate, type AttentionCommand, type AttentionEntry} from '@core/metadata-attention.ts';
import {attentionCount, attentionInProgress, attentionOffers, attentionReason, candidateFacts, candidateLine, hasAttentionList, providerName, typedProviderId, type AttentionCount} from '@core/server-admin/attention-words.ts';
export {hasAttentionList} from '@core/server-admin/attention-words.ts';
import {useAction} from '../../admin/console';
import {useCursorRead} from '../../admin/cursor-read';
import {usePollWhile} from '../../admin/poll-while';
import {ListPager} from './ListPager';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Dialog, Input, Menu, Notice, Select, SettingsGroup, SettingsRow, Text, type MenuItem} from '../../ui';
import page from '../settings/Settings.module.css';

type Api = ReturnType<typeof useSession>['api'];
const serviceFor = (api: Api, libraryId: string) => metadataAttentionService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body), libraryId);

/**
 * How many titles wait for the owner in each library, for the badge beside it. The server has no
 * count of its own yet, so this reads each list's first page, one library after another, and
 * says "25+" when there are more pages. A library whose list cannot be read simply has no badge.
 */
export function useAttentionCounts(libraries: readonly {id: string; kind: string}[]): Readonly<Record<string, AttentionCount>> {
  const {api} = useSession();
  const [counts, setCounts] = useState<Readonly<Record<string, AttentionCount>>>({});
  const ids = libraries.filter(l => hasAttentionList(l.kind)).map(l => l.id).join(',');
  useEffect(() => {
    let live = true;
    void (async () => {
      for (const id of ids ? ids.split(',') : []) {
        try {
          const first = await serviceFor(api, id).list('', 25);
          if (!live) return;
          setCounts(prev => ({...prev, [id]: attentionCount(first)}));
        } catch { /* no badge for this library */ }
      }
    })();
    return () => { live = false; };
  }, [api, ids]);
  return counts;
}

/**
 * A library › Needs attention: the titles Portico could not match with confidence, each with
 * why, the closest matches, and what the owner can do: use a match, search by hand, give the
 * title's ID at a provider, or dismiss it. The server says which of those a title allows.
 */
export function NeedsAttentionPanel({libraryId, onCount}: {libraryId: string; onCount?: (count: AttentionCount) => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const navigate = useNavigate();
  const service = useMemo(() => serviceFor(api, libraryId), [api, libraryId]);
  const read = useCursorRead(cursor => service.list(cursor, 25), [service]);
  const action = useAction();
  const [searching, setSearching] = useState<AttentionEntry>();
  const [identifying, setIdentifying] = useState<AttentionEntry>();
  const entries = read.data?.entries;
  useEffect(() => { if (read.data && read.page === 1) onCount?.(attentionCount(read.data)); }, [read.data, read.page]); // eslint-disable-line react-hooks/exhaustive-deps
  // A search or a match the owner just asked for settles on the server; the list follows it.
  usePollWhile(!!entries?.some(e => attentionInProgress(e.reason)), read.reload);
  const has = (id: string) => i18n.has(id);
  const send = (entry: AttentionEntry, command: Omit<AttentionCommand, 'kind' | 'entityId' | 'expectedRevision'>, done: string) =>
    action.run(async () => { await service.act({kind: entry.kind, entityId: entry.entityId, expectedRevision: entry.revision, ...command}); }, done).then(ok => { read.reload(); return ok; });
  const open = (entry: AttentionEntry) => (entry.kind === 'show'
    ? void navigate({to: '/show/$showId', params: {showId: entry.entityId}, search: {library: libraryId}})
    : void navigate({to: '/media/$itemId', params: {itemId: entry.entityId}, search: {library: libraryId}}));
  return (
    <SettingsGroup title={t('metadata.attention.title')} description={t('server.attention.lede')}>
      {read.error ? <div style={{padding: 12}}><Notice tone={entries ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {action.error && !searching && !identifying ? <div style={{padding: 12}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
      {action.notice ? <div style={{padding: 12}}><Notice tone="success" compact>{action.notice}</Notice></div> : null}
      {entries?.map(entry => {
        const items: MenuItem[] = [
          ...(attentionOffers(entry, 'search') ? [{id: 'search', label: t('server.attention.search'), icon: 'search' as const}] : []),
          ...(attentionOffers(entry, 'id') ? [{id: 'id', label: t('server.attention.enterId'), icon: 'link' as const}] : []),
          ...(attentionOffers(entry, 'parent') && entry.parentId ? [{id: 'parent', label: t('server.attention.openShow'), icon: 'tv' as const}] : []),
          ...(attentionOffers(entry, 'dismiss') ? [{id: 'dismiss', label: t('metadata.attention.dismiss'), icon: 'close' as const, separatorBefore: true}] : []),
        ];
        const offered = attentionOffers(entry, 'accept') ? entry.candidates : [];
        return (
          <SettingsRow key={entry.entityId} stack icon={entry.kind === 'show' ? 'tv' : 'film'}
            label={<button type="button" className={page.rowLink} onClick={() => open(entry)}>{entry.title || t('server.attention.untitled')}</button>}
            help={attentionReason(t, has, entry.reason)}
            control={<div style={{display: 'flex', flexDirection: 'column', gap: 8, width: '100%'}}>
              {offered.map(candidate => <Candidate key={candidate.key} candidate={candidate} busy={action.busy} onUse={() => void send(entry, {action: 'accept', candidateKey: candidate.key, ...(candidate.orders[0] ? {order: candidate.orders[0].id} : {})}, t('server.attention.matched', {title: candidate.title}))} />)}
              {items.length ? <div><Menu label={t('server.attention.actions', {title: entry.title})} trigger={<Button size="sm" variant="ghost" icon="more" label={offered.length ? t('server.attention.somethingElse') : t('server.attention.choose')} />} items={items} onSelect={id => {
                action.clear();
                if (id === 'search') setSearching(entry);
                else if (id === 'id') setIdentifying(entry);
                else if (id === 'parent') void navigate({to: '/show/$showId', params: {showId: entry.parentId}, search: {library: libraryId}});
                else if (id === 'dismiss') void send(entry, {action: 'dismiss'}, t('server.attention.dismissed'));
              }} /></div> : null}
            </div>} />
        );
      })}
      {entries && !entries.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('server.attention.none')}</Text></div> : null}
      <ListPager pages={read} />
      <SearchDialog entry={searching} busy={action.busy} error={action.error} onClose={() => setSearching(undefined)}
        onSearch={query => searching && void send(searching, {action: 'search', query}, t('server.attention.searchStarted')).then(ok => { if (ok) setSearching(undefined); })} />
      <IdentifyDialog entry={identifying} busy={action.busy} error={action.error} onClose={() => setIdentifying(undefined)}
        onIdentify={(provider, providerId) => identifying && void send(identifying, {action: 'id', providerId, ...(provider ? {provider} : {})}, t('server.attention.idSaved')).then(ok => { if (ok) setIdentifying(undefined); })} />
    </SettingsGroup>
  );
}

/** One suggested match: what it is, why it was suggested, and Use this. */
function Candidate({candidate, busy, onUse}: {candidate: AttentionCandidate; busy: boolean; onUse: () => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  const facts = candidateFacts(t, id => i18n.has(id), candidate);
  return (
    <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', padding: '8px 12px', border: '1px solid var(--line-soft)', borderRadius: 'var(--radius-control)', background: 'var(--surface-raised)'}}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0}}>
        <Text variant="bodyStrong">{candidateLine(candidate)}</Text>
        {facts.length ? <Text variant="caption" tone="tertiary">{facts.join(' · ')}</Text> : null}
      </div>
      <Button size="sm" variant="secondary" label={t('web.liveSource.useThis')} aria-label={t('server.attention.useNamed', {title: candidate.title})} disabled={busy} onClick={onUse} />
    </div>
  );
}

/** Search by hand: the words to look for; the matches arrive in the list a moment later. */
function SearchDialog({entry, onClose, onSearch, busy, error}: {entry?: AttentionEntry; onClose: () => void; onSearch: (query: string) => void; busy: boolean; error: string}) {
  const {t} = useI18n();
  const [query, setQuery] = useState('');
  useEffect(() => { if (entry) setQuery(entry.title); }, [entry]);
  return (
    <Dialog open={!!entry} onOpenChange={o => !o && onClose()} title={t('server.attention.searchTitle')} description={t('server.attention.searchBody')} width={480}
      actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" icon="search" label={t('web.search.title')} loading={busy} disabled={query.trim().length < 2} onClick={() => onSearch(query.trim())} /></>}>
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      <form onSubmit={e => { e.preventDefault(); if (query.trim().length >= 2 && !busy) onSearch(query.trim()); }}>
        <Input label={t('server.attention.searchFor')} value={query} onChange={e => setQuery(e.target.value)} maxLength={200} autoFocus />
      </form>
    </Dialog>
  );
}

const ID_PROVIDERS = ['tmdb', 'tvdb', 'anilist'] as const;

/** Enter a provider ID: an address or an IMDb ID says whose it is; a bare number needs to be told. */
function IdentifyDialog({entry, onClose, onIdentify, busy, error}: {entry?: AttentionEntry; onClose: () => void; onIdentify: (provider: (typeof ID_PROVIDERS)[number] | undefined, providerId: string) => void; busy: boolean; error: string}) {
  const {t} = useI18n();
  const [typed, setTyped] = useState('');
  const [provider, setProvider] = useState<(typeof ID_PROVIDERS)[number]>('tmdb');
  useEffect(() => { if (entry) { setTyped(''); setProvider('tmdb'); } }, [entry]);
  const id = typedProviderId(typed);
  const bare = !!id && /^\d+$/.test(id);
  return (
    <Dialog open={!!entry} onOpenChange={o => !o && onClose()} title={t('metadata.attention.enterId')} description={entry?.title} width={480}
      actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={t('server.attention.useId')} loading={busy} disabled={!id} onClick={() => id && onIdentify(bare ? provider : undefined, id)} /></>}>
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      <Input label={t('server.attention.idLabel')} help={t('server.attention.idHelp')} value={typed} onChange={e => setTyped(e.target.value)} mono maxLength={2048} autoCapitalize="off" spellCheck={false} autoFocus />
      {bare ? <Select label={t('server.attention.idProvider')} value={provider} options={ID_PROVIDERS.map(p => ({value: p, label: providerName(p)}))} onChange={e => setProvider(e.target.value as (typeof ID_PROVIDERS)[number])} /> : null}
    </Dialog>
  );
}
