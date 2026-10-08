import { useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { Clock, SearchX, Search as SearchIcon, X } from 'lucide-react';
import type { SearchHit } from '../../data/types';
import { useRecentSearches, useSearch } from '../../services/queries';
import { PageHeader } from '../../components/PageHeader';
import { Artwork } from '../../components/media';
import { EmptyState, ErrorState, Skel, cls } from '../../components/ui';

const GENRES: [string, string, string][] = [
  ['Science Fiction', '#2f86bb', '/movies?view=library&genre=Science%20Fiction'],
  ['Thriller', '#8a3a4c', '/movies?view=library&genre=Thriller'],
  ['Comedy', '#c9862a', '/movies?view=library&genre=Comedy'],
  ['Documentary', '#2e8a67', '/movies?view=library&genre=Documentary'],
  ['Anime', '#7b5ce0', '/anime'],
  ['Family', '#3aa39a', '/movies?view=library&genre=Family'],
  ['Romance', '#b84d7e', '/movies?view=library&genre=Romance'],
  ['Silent Era', '#8a7a5a', '/collection/col-silent'],
  ['Live Sports', '#c2463d', '/live'],
  ['Jazz', '#3b5bb5', '/album/blue-hour-sessions'],
  ['Audiobooks', '#b87333', '/audiobooks'],
  ['Home Videos', '#6b7f95', '/home-videos'],
];

const KINDS = [
  ['all', 'All'],
  ['movie', 'Movies'],
  ['show', 'Shows'],
  ['episode', 'Episodes'],
  ['person', 'People'],
  ['album', 'Music'],
  ['audiobook', 'Books'],
  ['program', 'Live TV'],
] as const;

function Hit({ h, big }: { h: SearchHit; big?: boolean }) {
  const ratio = h.shape === 'poster' ? '2 / 3' : h.shape === 'landscape' ? '16 / 9' : '1 / 1';
  const initials = h.title.split(' ').map((w) => w[0]).slice(0, 2).join('');
  return (
    <Link to={h.href} className={cls('hit', `hit-${h.shape}`, big && 'big')} viewTransition>
      {h.image ? (
        <Artwork src={h.image} ratio={ratio} className={cls('hit-art', h.shape === 'circle' && 'round')} />
      ) : (
        <span className={cls('hit-art hit-mono', h.shape === 'circle' && 'round')} style={{ aspectRatio: ratio }}>
          {initials}
        </span>
      )}
      <span className="hit-text">
        <span className="hit-title ellipsis">{h.title}</span>
        <span className="hit-sub ellipsis">{h.subtitle}</span>
      </span>
    </Link>
  );
}

export function SearchPage() {
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState(params.get('q') ?? '');
  const [debounced, setDebounced] = useState(q);
  const [kind, setKind] = useState<string>('all');
  const input = useRef<HTMLInputElement>(null);
  const res = useSearch(debounced);
  const recent = useRecentSearches();
  useEffect(() => {
    const t = setTimeout(() => {
      setDebounced(q);
      setParams(q ? { q } : {}, { replace: true });
    }, 160);
    return () => clearTimeout(t);
  }, [q, setParams]);
  useEffect(() => {
    if (!params.get('q')) input.current?.focus({ preventScroll: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  const topId = kind === 'all' ? res.data?.top?.id : undefined;
  const groups = (res.data?.groups ?? [])
    .map((g) => ({ ...g, hits: g.hits.filter((h) => h.id !== topId) }))
    .filter((g) => g.hits.length > 0)
    .filter((g) => kind === 'all' || g.kind === kind || (kind === 'album' && ['album', 'artist', 'track', 'playlist'].includes(g.kind)));
  const active = debounced.trim().length > 0;

  return (
    <div className="page search-page">
      <PageHeader title="Search" />
      <div className="pad">
        <div className="search-big glass">
          <SearchIcon />
          <input ref={input} value={q} onChange={(e) => setQ(e.target.value)} placeholder="Movies, shows, people, songs, channels…" aria-label="Search" enterKeyHint="search" autoComplete="off" spellCheck={false} />
          {q && (
            <button className="icon-btn sm plain" aria-label="Clear search" onClick={() => (setQ(''), input.current?.focus())}>
              <X />
            </button>
          )}
        </div>
        {active && (
          <div className="chip-scroll no-scrollbar search-kinds">
            {KINDS.map(([k, label]) => (
              <button key={k} className={cls('chip', kind === k && 'on')} onClick={() => setKind(k)}>
                {label}
              </button>
            ))}
            {res.data && <span className="t-caption search-took tnum">{res.data.total} results in {res.data.tookMs} ms</span>}
          </div>
        )}
      </div>
      <div className="page-body pad">
        {!active && (
          <>
            {(recent.data?.length ?? 0) > 0 && (
              <section className="search-recent">
                <h2 className="shelf-title">Recent</h2>
                <div className="recent-list">
                  {recent.data!.map((r) => (
                    <button key={r} className="recent" onClick={() => setQ(r)}>
                      <Clock /> {r}
                    </button>
                  ))}
                </div>
              </section>
            )}
            <section>
              <h2 className="shelf-title" style={{ marginBottom: 14 }}>
                Browse
              </h2>
              <div className="genre-grid">
                {GENRES.map(([g, c, href]) => (
                  <Link key={g} to={href} className="genre-tile" style={{ ['--c' as string]: c }} viewTransition>
                    <span>{g}</span>
                  </Link>
                ))}
              </div>
            </section>
          </>
        )}
        {active && res.isError && <ErrorState error={res.error} retry={() => res.refetch()} title="Search isn’t responding" />}
        {active && res.isLoading && (
          <div className="hit-grid">
            {Array.from({ length: 8 }, (_, i) => (
              <div key={i}>
                <Skel style={{ aspectRatio: '2 / 3' }} />
                <Skel h={12} w="70%" style={{ marginTop: 10 }} />
              </div>
            ))}
          </div>
        )}
        {active && res.data && groups.length === 0 && !topId && (
          <EmptyState icon={<SearchX />} title={`No results for “${debounced}”`} body="Check the spelling, or try a person, a song lyric or a channel name. Portico searches titles, cast, lyrics and the live guide." />
        )}
        {active && res.data && (groups.length > 0 || topId) && (
          <div className={cls('results', res.isFetching && 'fetching')}>
            {kind === 'all' && res.data.top && (
              <section className="top-hit-sec">
                <h2 className="shelf-title">Top Result</h2>
                <div className="top-hit">
                  <Hit h={res.data.top} big />
                </div>
              </section>
            )}
            {groups.map((g) => (
              <section key={g.kind} className="result-group">
                <h2 className="shelf-title">{g.label}</h2>
                <div className={cls('hit-grid', `g-${g.hits[0]?.shape}`)}>
                  {g.hits.map((h) => (
                    <Hit key={`${h.kind}-${h.id}`} h={h} />
                  ))}
                </div>
              </section>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
