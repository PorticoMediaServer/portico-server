import {
  BookHeadphones,
  Clapperboard,
  Clock,
  Download,
  Film,
  Home,
  Images,
  Library,
  ListVideo,
  Music2,
  Radio,
  Search,
  Sparkles,
  Tv,
  Video,
  type LucideIcon,
} from 'lucide-react';

export interface NavItem {
  id: string;
  label: string;
  to: string;
  icon: LucideIcon;
  library?: string; // required library access
  match?: string[]; // path prefixes that count as "inside" this destination
}

export const PRIMARY: NavItem[] = [
  { id: 'home', label: 'Home', to: '/', icon: Home, match: ['/'] },
  { id: 'movies', label: 'Movies', to: '/movies', icon: Film, library: 'lib-movies', match: ['/movies', '/movie/', '/collection/'] },
  { id: 'tv', label: 'TV Shows', to: '/tv', icon: Tv, library: 'lib-tv', match: ['/tv'] },
  { id: 'anime', label: 'Anime', to: '/anime', icon: Sparkles, library: 'lib-anime', match: ['/anime'] },
  { id: 'music', label: 'Music', to: '/music', icon: Music2, library: 'lib-music', match: ['/music', '/album/', '/artist/', '/playlist/'] },
  { id: 'live', label: 'Live TV', to: '/live', icon: Radio, match: ['/live'] },
];

export const MORE: NavItem[] = [
  { id: 'audiobooks', label: 'Audiobooks', to: '/audiobooks', icon: BookHeadphones, library: 'lib-audiobooks', match: ['/audiobooks', '/audiobook/'] },
  { id: 'photos', label: 'Photos', to: '/photos', icon: Images, library: 'lib-photos', match: ['/photos'] },
  { id: 'homevideos', label: 'Home Videos', to: '/home-videos', icon: Video, library: 'lib-homevideos', match: ['/home-videos', '/home-video/'] },
];

export const PERSONAL: NavItem[] = [
  { id: 'watchlist', label: 'Watchlist', to: '/watchlist', icon: ListVideo, match: ['/watchlist'] },
  { id: 'history', label: 'History', to: '/history', icon: Clock, match: ['/history'] },
  { id: 'downloads', label: 'Downloads', to: '/downloads', icon: Download, match: ['/downloads'] },
];

export const MOBILE_TABS: NavItem[] = [
  { id: 'home', label: 'Home', to: '/', icon: Home, match: ['/'] },
  { id: 'library', label: 'Library', to: '/library', icon: Library, match: ['/library', '/movies', '/movie/', '/tv', '/anime', '/show/', '/episode/', '/collection/', '/audiobooks', '/audiobook/', '/photos', '/home-videos', '/home-video/', '/watchlist', '/history', '/downloads', '/person/'] },
  { id: 'music', label: 'Music', to: '/music', icon: Music2, library: 'lib-music', match: ['/music', '/album/', '/artist/', '/playlist/'] },
  { id: 'live', label: 'Live', to: '/live', icon: Radio, match: ['/live'] },
];

export const SEARCH_ITEM: NavItem = { id: 'search', label: 'Search', to: '/search', icon: Search, match: ['/search'] };
export const CONSOLE_ITEM: NavItem = { id: 'console', label: 'Server Console', to: '/console', icon: Clapperboard, match: ['/console'] };

export function isActive(item: NavItem, path: string): boolean {
  if (item.id === 'home' || item.id === 'home-tab') return path === '/';
  return (item.match ?? [item.to]).some((m) => (m === '/' ? path === '/' : path.startsWith(m)));
}

/** TV Shows owns /show/ and /episode/ unless the show is anime; the page tells us via data-section. */
export function sectionFor(path: string, animeHint: boolean): string | undefined {
  if (path.startsWith('/show/') || path.startsWith('/episode/')) return animeHint ? 'anime' : 'tv';
  return [...PRIMARY, ...MORE, ...PERSONAL].find((i) => i.id !== 'home' && isActive(i, path))?.id ?? (path === '/' ? 'home' : undefined);
}
