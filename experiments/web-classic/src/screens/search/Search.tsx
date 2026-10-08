import {useEffect, useRef, useState} from 'react';
import {Link, useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {SearchGroupId} from '@/data/types';
import {libraryIcon} from '@/app/libraryMeta';
import {IconButton, Button} from '@/ui/Button';
import {Chip, Chips, EmptyState, ErrorState} from '@/ui/Bits';
import {Icon} from '@/ui/Icon';
import {CardGrid, Shelf} from '@/ui/Shelf';
import {usePhone, useTitle} from '@/ui/hooks';
import {useRouteSearch} from '../common';
import s from './Search.module.css';

const KINDS: {id: SearchGroupId | 'all'; label: string}[] = [
  {id: 'all', label: 'All'}, {id: 'movies', label: 'Movies'}, {id: 'shows', label: 'Shows'}, {id: 'episodes', label: 'Episodes'}, {id: 'people', label: 'People'},
  {id: 'artists', label: 'Artists'}, {id: 'albums', label: 'Albums'}, {id: 'songs', label: 'Songs'}, {id: 'books', label: 'Audiobooks'}, {id: 'live', label: 'Live TV'},
];
const SHAPE: Record<SearchGroupId, 'poster' | 'wide' | 'square' | 'circle'> = {movies: 'poster', shows: 'poster', episodes: 'wide', people: 'circle', artists: 'circle', albums: 'square', songs: 'square', books: 'poster', live: 'wide'};

/**
 * Search everything on the server. The query and kind live in the URL, so
 * results survive reload and Back. Recent searches are kept per profile.
 */
export default function SearchScreen() {
  const search = useRouteSearch<{q?: string; type?: string}>();
  const navigate = useNavigate();
  const client = useClient();
  const phone = usePhone();
  const [text, setText] = useState(search.q ?? '');
  const input = useRef<HTMLInputElement>(null);
  const q = (search.q ?? '').trim();
  const type = (KINDS.find(k => k.id === search.type)?.id ?? 'all') as SearchGroupId | 'all';
  useTitle(q ? `“${q}”` : 'Search');
  useEffect(() => { setText(search.q ?? ''); }, [search.q]);
  useEffect(() => {
    const t = setTimeout(() => { if (text.trim() !== q) void navigate({to: '/search' as never, search: {q: text.trim() || undefined, type: type === 'all' ? undefined : type} as never, replace: true}); }, 250);
    return () => clearTimeout(t);
  }, [text]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (!phone) input.current?.focus(); }, [phone]);
  useEffect(() => { if (q.length > 1) { const t = setTimeout(() => client.search.remember(q), 1500); return () => clearTimeout(t); } }, [q, client]);
  const results = useQuery(c => (q ? c.search.query(q, type === 'all' ? undefined : type) : Promise.resolve(undefined)), [q, type]);
  const libs = useQuery(c => c.libraries.list(), []);
  const [recent, setRecent] = useState(() => client.search.recent());
  const setType = (t: string) => void navigate({to: '/search' as never, search: {q: q || undefined, type: t === 'all' ? undefined : t} as never, replace: true});
  return (
    <div style={{padding: '28px 0 72px'}}>
      <div className={s.box}>
        <form role="search" className={s.field} onSubmit={e => { e.preventDefault(); if (text.trim()) client.search.remember(text.trim()); }}>
          <Icon name="search" size={22} className={s.icon} />
          <input ref={input} type="search" className={s.input} value={text} onChange={e => setText(e.target.value)} placeholder="Movies, shows, people, music, channels" aria-label="Search Portico" autoComplete="off" spellCheck={false} onKeyDown={e => { if (e.key === 'Escape' && text) { e.preventDefault(); setText(''); } }} />
          {text ? <IconButton className={s.clear} variant="ghost" icon="close" label="Clear search" onClick={() => { setText(''); input.current?.focus(); }} /> : null}
        </form>
      </div>
      {q ? <div className={s.chips}><Chips scroll={phone} label="Show results for">{KINDS.map(k => <Chip key={k.id} pressed={type === k.id} onClick={() => setType(k.id)}>{k.label}</Chip>)}</Chips></div> : null}
      {!q ? (
        <>
          {recent.length ? (
            <div className={s.recent}>
              <div className={s.recentHead}><span className="t-label">Recent searches</span><Button size="sm" variant="ghost" label="Clear" onClick={() => { client.search.clearRecent(); setRecent([]); }} /></div>
              {recent.map(r => <button key={r} type="button" className={s.recentItem} onClick={() => setText(r)}><Icon name="history" size={18} />{r}</button>)}
            </div>
          ) : null}
          <div className={s.browse}>
            <span className="t-label">Browse</span>
            <div className={s.browseGrid}>
              {libs.data?.map(l => <Link key={l.id} to={`/library/${l.id}?view=browse` as never} className={s.browseTile}><Icon name={libraryIcon(l.kind)} size={22} />{l.name}</Link>)}
              <Link to="/channels" className={s.browseTile}><Icon name="guide" size={22} />Live TV guide</Link>
            </div>
          </div>
        </>
      ) : results.error ? <ErrorState onRetry={results.refresh} /> : !results.data ? (
        <div className={s.groups}><Shelf title="" /><Shelf title="" /></div>
      ) : results.data.groups.length === 0 ? (
        <EmptyState icon="search" title={`Nothing found for “${q}”`} body={type === 'all' ? 'Check the spelling, or try a shorter search. Search looks at titles, people, genres and channels.' : `No ${KINDS.find(k => k.id === type)!.label.toLowerCase()} match. Try All.`} actions={type !== 'all' ? [{label: 'Search everything', onClick: () => setType('all')}] : undefined} />
      ) : type === 'all' ? (
        <div className={s.groups}>
          {results.data.groups.map(g => <Shelf key={g.id} title={g.title} entries={g.entries} shape={SHAPE[g.id]} total={g.total} headExtra={g.total > g.entries.length ? <button type="button" className={s.seeAll} onClick={() => setType(g.id)}>See all {g.total}</button> : undefined} />)}
        </div>
      ) : (
        <>
          <p className="t-caption c-3" style={{padding: '0 var(--gutter)', marginBottom: 16}}>{results.data.groups[0]?.total ?? 0} results</p>
          <CardGrid entries={results.data.groups[0]?.entries ?? []} shape={SHAPE[type as SearchGroupId]} />
        </>
      )}
    </div>
  );
}
