import { Link } from 'react-router';
import { ChevronRight } from 'lucide-react';
import { useDownloads, useLibraries, useWatchlist } from '../../services/queries';
import { PageHeader } from '../../components/PageHeader';
import { MORE, PERSONAL, PRIMARY } from '../../app/nav';
import { Skel } from '../../components/ui';
import { fmtBytes } from '../../lib/hooks';

const COLORS: Record<string, string> = { movies: '#409fd4', tv: '#2f86bb', anime: '#8a7cf0', music: '#35c28a', audiobooks: '#f0a02a', photos: '#e25aa8', homevideos: '#c9a86a', live: '#ff4d58', watchlist: '#5cb3e2', history: '#7a8ea3', downloads: '#3fae6b' };

export function LibraryHub() {
  const libs = useLibraries();
  const wl = useWatchlist();
  const dl = useDownloads();
  const items = [...PRIMARY, ...MORE].filter((n) => n.id !== 'home' && n.id !== 'live' && (!n.library || libs.data?.some((l) => l.id === n.library)));
  return (
    <div className="page hub">
      <PageHeader title="Library" subtitle="Everything on Harbor" />
      <div className="page-body pad hub-body">
        <div className="hub-tiles">
          {!libs.data &&
            Array.from({ length: 7 }, (_, i) => (
              <Skel key={i} h={110} r={20} />
            ))}
          {libs.data &&
            items.map((n) => {
              const lib = libs.data.find((l) => l.id === n.library);
              return (
                <Link key={n.id} to={n.to} className="hub-tile" style={{ ['--c' as string]: COLORS[n.id] }} viewTransition>
                  <span className="hub-icon">
                    <n.icon />
                  </span>
                  <span className="hub-name">{n.label}</span>
                  <span className="hub-count">{lib ? `${lib.countLabel.split(' · ')[0]} · ${fmtBytes(lib.sizeBytes)}` : ''}</span>
                  <ChevronRight className="hub-chev" />
                </Link>
              );
            })}
        </div>
        <h2 className="group-title hub-yours">Yours</h2>
        <div className="group">
          {PERSONAL.map((n) => (
            <Link key={n.id} to={n.to} className="row has-icon" viewTransition>
              <span className="row-icon" style={{ background: COLORS[n.id] }}>
                <n.icon />
              </span>
              <span className="row-main">
                <span className="row-title">{n.label}</span>
              </span>
              <span className="row-value">{n.id === 'watchlist' ? wl.data?.length ?? '' : n.id === 'downloads' ? dl.data?.filter((d) => d.status === 'complete').length ?? '' : ''}</span>
              <ChevronRight className="row-chev" />
            </Link>
          ))}
        </div>
      </div>
    </div>
  );
}
