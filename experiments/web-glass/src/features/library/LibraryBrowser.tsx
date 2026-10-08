import { useMemo } from 'react';
import { Link, useSearchParams } from 'react-router';
import { ArrowDownWideNarrow, ArrowUpNarrowWide, Check, ChevronDown, Film, LayoutGrid, List, SlidersHorizontal } from 'lucide-react';
import type { LibraryCard, LibraryQuery } from '../../data/types';
import { useLibraryItems } from '../../services/queries';
import { VirtualGrid } from '../../components/VirtualGrid';
import { MediaCard } from '../../components/media';
import { Button, EmptyState, ErrorState, Progress, cls } from '../../components/ui';
import { Menu, useMenu } from '../../components/overlay';
import { useIsMobile } from '../../lib/hooks';

export type Shape = 'poster' | 'landscape' | 'square';

const SORTS: { value: NonNullable<LibraryQuery['sort']>; label: string }[] = [
  { value: 'title', label: 'Title' },
  { value: 'added', label: 'Date Added' },
  { value: 'year', label: 'Release Year' },
  { value: 'rating', label: 'Critic Rating' },
  { value: 'runtime', label: 'Runtime' },
];

const FILTERS: { value: NonNullable<LibraryQuery['filter']>; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'unwatched', label: 'Unwatched' },
  { value: 'in-progress', label: 'In Progress' },
  { value: 'watched', label: 'Watched' },
  { value: '4k', label: '4K' },
  { value: 'hdr', label: 'HDR' },
  { value: 'downloaded', label: 'Downloaded' },
];

export function useLibraryParams(defaults: Partial<LibraryQuery> = {}) {
  const [params, setParams] = useSearchParams();
  const q: LibraryQuery = {
    sort: (params.get('sort') as LibraryQuery['sort']) ?? defaults.sort ?? 'title',
    dir: (params.get('dir') as LibraryQuery['dir']) ?? defaults.dir ?? 'asc',
    genre: params.get('genre') ?? undefined,
    filter: (params.get('filter') as LibraryQuery['filter']) ?? 'all',
  };
  const set = (patch: Record<string, string | undefined>) => {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries(patch)) {
      if (v === undefined || v === '' || (k === 'filter' && v === 'all')) next.delete(k);
      else next.set(k, v);
    }
    setParams(next, { replace: true });
  };
  return { q, set, layout: params.get('layout') === 'list' ? 'list' : 'grid' };
}

export function LibraryBrowser({ libraryId, shape, noun, filters = ['all', 'unwatched', 'in-progress', 'watched', '4k', 'hdr'], sorts = ['title', 'added', 'year', 'rating', 'runtime'] }: { libraryId: string; shape: Shape; noun: [string, string]; filters?: string[]; sorts?: string[] }) {
  const { q, set, layout } = useLibraryParams();
  const res = useLibraryItems(libraryId, q);
  const mobile = useIsMobile();
  const sortMenu = useMenu();
  const genreMenu = useMenu();
  const filterMenu = useMenu();
  const total = res.data?.total ?? 0;
  const aspect = shape === 'poster' ? 2 / 3 : shape === 'landscape' ? 16 / 9 : 1;
  const minW = shape === 'poster' ? (mobile ? 104 : 168) : shape === 'landscape' ? (mobile ? 160 : 280) : mobile ? 150 : 180;
  const sortLabel = SORTS.find((s) => s.value === q.sort)?.label ?? 'Title';
  const fOpts = FILTERS.filter((f) => filters.includes(f.value));
  const activeFilter = fOpts.find((f) => f.value === q.filter) ?? fOpts[0];

  const grid = useMemo(() => res.data?.items ?? [], [res.data]);

  return (
    <div className="lib">
      <div className="lib-toolbar glass-bar">
        <div className="lib-count tnum">
          {res.isLoading ? <span className="skel" style={{ display: 'inline-block', width: 80, height: 14 }} /> : `${total.toLocaleString('en-US')} ${total === 1 ? noun[0] : noun[1]}`}
          {res.isFetching && !res.isLoading && <span className="spinner" style={{ width: 14, height: 14 }} />}
        </div>
        <div className="lib-controls">
          {!mobile ? (
            <div className="chip-row">
              {fOpts.map((f) => (
                <button key={f.value} className={cls('chip', q.filter === f.value && 'on')} onClick={() => set({ filter: f.value })}>
                  {f.label}
                </button>
              ))}
            </div>
          ) : (
            <button className="chip" onClick={filterMenu.open}>
              <SlidersHorizontal />
              {activeFilter.label}
            </button>
          )}
          {(res.data?.genres.length ?? 0) > 0 && (
            <button className={cls('chip', q.genre && 'on')} onClick={genreMenu.open}>
              {q.genre ?? 'All Genres'}
              <ChevronDown />
            </button>
          )}
          <button className="chip" onClick={sortMenu.open} aria-label={`Sort by ${sortLabel}`}>
            {q.dir === 'desc' ? <ArrowDownWideNarrow /> : <ArrowUpNarrowWide />}
            {sortLabel}
          </button>
          {!mobile && shape === 'poster' && (
            <div className="segmented sm lib-layout" role="group" aria-label="Layout">
              <button aria-pressed={layout === 'grid'} onClick={() => set({ layout: undefined })} aria-label="Grid">
                <LayoutGrid />
              </button>
              <button aria-pressed={layout === 'list'} onClick={() => set({ layout: 'list' })} aria-label="List">
                <List />
              </button>
            </div>
          )}
        </div>
      </div>
      <Menu anchor={filterMenu.anchor} open={filterMenu.isOpen} onClose={filterMenu.close} title="Show" items={fOpts.map((f) => ({ label: f.label, checked: q.filter === f.value, onSelect: () => set({ filter: f.value }) }))} />
      <Menu
        anchor={genreMenu.anchor}
        open={genreMenu.isOpen}
        onClose={genreMenu.close}
        title="Genre"
        width={240}
        items={[{ label: 'All Genres', checked: !q.genre, onSelect: () => set({ genre: undefined }) }, { separator: true }, ...(res.data?.genres ?? []).map((g) => ({ label: g, checked: q.genre === g, onSelect: () => set({ genre: g }) }))]}
      />
      <Menu
        anchor={sortMenu.anchor}
        open={sortMenu.isOpen}
        onClose={sortMenu.close}
        title="Sort By"
        items={[
          ...SORTS.filter((s) => sorts.includes(s.value)).map((s) => ({ label: s.label, checked: q.sort === s.value, onSelect: () => set({ sort: s.value }) })),
          { separator: true },
          { label: 'Ascending', checked: q.dir !== 'desc', onSelect: () => set({ dir: 'asc' }) },
          { label: 'Descending', checked: q.dir === 'desc', onSelect: () => set({ dir: 'desc' }) },
        ]}
      />

      {res.isError ? (
        <ErrorState error={res.error} retry={() => res.refetch()} />
      ) : res.isLoading ? (
        <div className="lib-skeleton" style={{ gridTemplateColumns: `repeat(auto-fill, minmax(${minW}px, 1fr))` }}>
          {Array.from({ length: 18 }, (_, i) => (
            <div key={i}>
              <div className="skel" style={{ aspectRatio: String(aspect), borderRadius: 12 }} />
              <div className="skel" style={{ height: 12, width: '70%', marginTop: 10 }} />
            </div>
          ))}
        </div>
      ) : grid.length === 0 ? (
        <EmptyState
          icon={<Film />}
          title={`No ${noun[1].toLowerCase()} match`}
          body="Try a different filter or genre."
          actions={
            <Button variant="plain" onClick={() => set({ filter: 'all', genre: undefined })}>
              Clear Filters
            </Button>
          }
        />
      ) : layout === 'list' && !mobile ? (
        <LibraryTable items={grid} />
      ) : (
        <VirtualGrid items={grid} minWidth={minW} gap={mobile ? 12 : 22} aspect={aspect} textHeight={shape === 'square' ? 50 : 48} getKey={(c) => c.id} render={(c) => <MediaCard item={c} shape={shape} />} />
      )}
    </div>
  );
}

function LibraryTable({ items }: { items: LibraryCard[] }) {
  return (
    <div className="lib-table" role="table" aria-label="Library">
      <div className="lib-tr head" role="row">
        <span role="columnheader">Title</span>
        <span role="columnheader">Year</span>
        <span role="columnheader">Status</span>
        <span role="columnheader" className="num">
          Quality
        </span>
      </div>
      {items.map((c) => (
        <Link key={c.id} to={c.href} className="lib-tr" role="row" viewTransition>
          <span className="lib-td-title" role="cell">
            <img src={c.image} alt="" loading="lazy" />
            <span className="ellipsis">{c.title}</span>
          </span>
          <span role="cell" className="tnum">
            {c.year}
          </span>
          <span role="cell">
            {c.progress ? (
              <span className="lib-prog">
                <Progress value={c.progress} accent />
                {Math.round(c.progress * 100)}%
              </span>
            ) : c.watched ? (
              <span className="pill-status ok">
                <Check className="inline-ic" />
                Watched
              </span>
            ) : c.unwatchedCount ? (
              `${c.unwatchedCount} unwatched`
            ) : (
              <span className="t-sub">Unwatched</span>
            )}
          </span>
          <span role="cell" className="num">
            {c.badge ? <span className="badge">{c.badge}</span> : <span className="badge">HD</span>}
          </span>
        </Link>
      ))}
    </div>
  );
}
