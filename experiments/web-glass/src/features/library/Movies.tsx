import { useSearchParams } from 'react-router';
import { Layers } from 'lucide-react';
import { useCollections, useLibraries, useLibraryHome } from '../../services/queries';
import { Hero, HeroSkeleton } from '../../components/Hero';
import { MediaCard, Shelf, ShelfSkeleton } from '../../components/media';
import { EmptyState, ErrorState, Segmented } from '../../components/ui';
import { PageHeader } from '../../components/PageHeader';
import { LibraryBrowser } from './LibraryBrowser';
import { fmtBytes } from '../../lib/hooks';

export function useViewParam(def = 'browse') {
  const [params, setParams] = useSearchParams();
  const view = params.get('view') ?? def;
  const setView = (v: string) => {
    const next = new URLSearchParams();
    if (v !== def) next.set('view', v);
    setParams(next, { replace: false });
  };
  return [view, setView] as const;
}

export function LibraryShelves({ libraryId }: { libraryId: string }) {
  const home = useLibraryHome(libraryId);
  if (home.isError) return <ErrorState error={home.error} retry={() => home.refetch()} />;
  if (home.isLoading || !home.data)
    return (
      <>
        <div className="billboard-wrap">
          <HeroSkeleton />
        </div>
        <ShelfSkeleton />
        <ShelfSkeleton style="landscape" />
      </>
    );
  return (
    <>
      {home.data.hero && home.data.hero.length > 0 && (
        <div className="billboard-wrap">
          <Hero items={home.data.hero} compact />
        </div>
      )}
      {home.data.shelves.map((s, i) => (
        <Shelf key={s.id} shelf={s} eager={i < 1} />
      ))}
    </>
  );
}

export function MoviesPage() {
  const [view, setView] = useViewParam();
  const libs = useLibraries();
  const lib = libs.data?.find((l) => l.id === 'lib-movies');
  const cols = useCollections('lib-movies');
  return (
    <div className="page">
      <PageHeader
        title="Movies"
        subtitle={lib ? `${lib.countLabel} · ${fmtBytes(lib.sizeBytes)}` : undefined}
        actions={
          <Segmented
            value={view}
            onChange={setView}
            label="View"
            options={[
              { value: 'browse', label: 'Browse' },
              { value: 'library', label: 'Library' },
              { value: 'collections', label: 'Collections' },
            ]}
          />
        }
      >
        <div className="m-seg">
          <Segmented
            block
            value={view}
            onChange={setView}
            label="View"
            options={[
              { value: 'browse', label: 'Browse' },
              { value: 'library', label: 'Library' },
              { value: 'collections', label: 'Collections' },
            ]}
          />
        </div>
      </PageHeader>
      <div className="page-body">
        {view === 'browse' && <LibraryShelves libraryId="lib-movies" />}
        {view === 'library' && (
          <div className="pad">
            <LibraryBrowser libraryId="lib-movies" shape="poster" noun={['movie', 'movies']} />
          </div>
        )}
        {view === 'collections' && (
          <div className="pad">
            {cols.isError ? (
              <ErrorState error={cols.error} retry={() => cols.refetch()} />
            ) : cols.data?.length === 0 ? (
              <EmptyState icon={<Layers />} title="No collections yet" body="Collections are created automatically from metadata, or you can make your own from any movie’s menu." />
            ) : (
              <div className="collection-grid">
                {(cols.data ?? Array.from({ length: 6 }, () => null)).map((c, i) =>
                  c ? (
                    <MediaCard key={c.id} shape="landscape" item={{ id: c.id, kind: 'collection', title: c.title, subtitle: c.count === 1 ? '1 film' : `${c.count} films`, image: c.art.backdrop!, href: `/collection/${c.id}`, palette: c.palette }} />
                  ) : (
                    <div key={i}>
                      <div className="skel" style={{ aspectRatio: '16 / 9', borderRadius: 12 }} />
                    </div>
                  ),
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
