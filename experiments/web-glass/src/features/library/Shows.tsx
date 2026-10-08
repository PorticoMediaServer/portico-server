import { useLibraries } from '../../services/queries';
import { Segmented } from '../../components/ui';
import { PageHeader } from '../../components/PageHeader';
import { LibraryBrowser } from './LibraryBrowser';
import { LibraryShelves, useViewParam } from './Movies';
import { fmtBytes } from '../../lib/hooks';

export function ShowsPage({ anime }: { anime: boolean }) {
  const [view, setView] = useViewParam();
  const libs = useLibraries();
  const id = anime ? 'lib-anime' : 'lib-tv';
  const lib = libs.data?.find((l) => l.id === id);
  const seg = (block?: boolean) => (
    <Segmented
      block={block}
      value={view}
      onChange={setView}
      label="View"
      options={[
        { value: 'browse', label: 'Browse' },
        { value: 'library', label: anime ? 'All Series' : 'All Shows' },
      ]}
    />
  );
  return (
    <div className="page" key={id}>
      <PageHeader title={anime ? 'Anime' : 'TV Shows'} subtitle={lib ? `${lib.countLabel} · ${fmtBytes(lib.sizeBytes)}` : undefined} actions={seg()}>
        <div className="m-seg">{seg(true)}</div>
      </PageHeader>
      <div className="page-body">
        {view === 'browse' ? (
          <LibraryShelves libraryId={id} />
        ) : (
          <div className="pad">
            <LibraryBrowser libraryId={id} shape="poster" noun={anime ? ['series', 'series'] : ['show', 'shows']} filters={['all', 'unwatched', 'in-progress', 'watched']} sorts={['title', 'added', 'year', 'rating']} />
          </div>
        )}
      </div>
    </div>
  );
}
