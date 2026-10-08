import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { LibraryQuery } from '../data/types';
import { useSession } from '../state/session';
import { api } from './client';

/** Every personal query is keyed by profile so switching profiles never shows stale data. */
const useP = () => useSession((s) => s.profileId ?? 'none');

export const useServers = () => useQuery({ queryKey: ['servers'], queryFn: api.listServers });
export const useProfiles = () => useQuery({ queryKey: ['profiles'], queryFn: api.getProfiles });
export const useLibraries = () => {
  const p = useP();
  return useQuery({ queryKey: ['libraries', p], queryFn: api.getLibraries, staleTime: 60_000 });
};
export const useHome = () => {
  const p = useP();
  return useQuery({ queryKey: ['home', p], queryFn: api.getHome });
};
export const useLibraryHome = (id: string) => {
  const p = useP();
  return useQuery({ queryKey: ['libraryHome', p, id], queryFn: () => api.getLibraryHome(id) });
};
export const useLibraryItems = (id: string, q: LibraryQuery) => {
  const p = useP();
  return useQuery({ queryKey: ['libraryItems', p, id, q], queryFn: () => api.getLibraryItems(id, q), placeholderData: keepPreviousData });
};
export const useMovie = (id: string) => {
  const p = useP();
  return useQuery({ queryKey: ['movie', p, id], queryFn: () => api.getMovie(id) });
};
export const useShow = (id: string) => {
  const p = useP();
  return useQuery({ queryKey: ['show', p, id], queryFn: () => api.getShow(id) });
};
export const useEpisode = (id: string) => {
  const p = useP();
  return useQuery({ queryKey: ['episode', p, id], queryFn: () => api.getEpisode(id) });
};
export const useCollection = (id: string) => {
  const p = useP();
  return useQuery({ queryKey: ['collection', p, id], queryFn: () => api.getCollection(id) });
};
export const useCollections = (lib: string) => {
  const p = useP();
  return useQuery({ queryKey: ['collections', p, lib], queryFn: () => api.getCollections(lib) });
};
export const usePerson = (id: string) => useQuery({ queryKey: ['person', id], queryFn: () => api.getPerson(id) });
export const useMusicHome = () => useQuery({ queryKey: ['musicHome'], queryFn: api.getMusicHome });
export const useArtists = () => useQuery({ queryKey: ['artists'], queryFn: api.getArtists });
export const useAlbums = (sort: 'added' | 'title' | 'artist' | 'year' = 'added') => useQuery({ queryKey: ['albums', sort], queryFn: () => api.getAlbums(sort) });
export const useSongs = (offset: number, limit: number, sort: 'title' | 'artist' | 'album' | 'plays') =>
  useQuery({ queryKey: ['songs', offset, limit, sort], queryFn: () => api.getSongs(offset, limit, sort), placeholderData: keepPreviousData });
export const useArtist = (id: string) => useQuery({ queryKey: ['artist', id], queryFn: () => api.getArtist(id) });
export const useAlbum = (id: string) => useQuery({ queryKey: ['album', id], queryFn: () => api.getAlbum(id) });
export const usePlaylists = () => useQuery({ queryKey: ['playlists'], queryFn: api.getPlaylists });
export const usePlaylist = (id: string) => useQuery({ queryKey: ['playlist', id], queryFn: () => api.getPlaylist(id) });
export const useAudiobooks = () => {
  const p = useP();
  return useQuery({ queryKey: ['audiobooks', p], queryFn: api.getAudiobooks });
};
export const useAudiobook = (id: string) => {
  const p = useP();
  return useQuery({ queryKey: ['audiobook', p, id], queryFn: () => api.getAudiobook(id) });
};
export const usePhotos = (album?: string) => useQuery({ queryKey: ['photos', album ?? 'all'], queryFn: () => api.getPhotos(album) });
export const usePhotoAlbums = () => useQuery({ queryKey: ['photoAlbums'], queryFn: api.getPhotoAlbums });
export const useMemories = () => useQuery({ queryKey: ['memories'], queryFn: api.getMemories });
export const useHomeVideos = () => useQuery({ queryKey: ['homeVideos'], queryFn: api.getHomeVideos });
export const useHomeVideo = (id: string) => {
  const p = useP();
  return useQuery({ queryKey: ['homeVideo', p, id], queryFn: () => api.getHomeVideo(id) });
};
export const useSearch = (q: string) => {
  const p = useP();
  return useQuery({ queryKey: ['search', p, q], queryFn: () => api.search(q), enabled: q.trim().length > 0, placeholderData: keepPreviousData });
};
export const useRecentSearches = () => {
  const p = useP();
  return useQuery({ queryKey: ['recentSearches', p], queryFn: api.getRecentSearches });
};
export const useWatchlist = () => {
  const p = useP();
  return useQuery({ queryKey: ['watchlist', p], queryFn: api.getWatchlist });
};
export const useHistory = () => {
  const p = useP();
  return useQuery({ queryKey: ['history', p], queryFn: api.getHistory });
};
export const useDownloads = () => useQuery({ queryKey: ['downloads'], queryFn: api.getDownloads });
export const useNotifications = () => {
  const p = useP();
  return useQuery({ queryKey: ['notifications', p], queryFn: api.getNotifications, refetchInterval: 60_000 });
};
export const useCastTargets = (enabled: boolean) => useQuery({ queryKey: ['cast'], queryFn: api.getCastTargets, enabled });
export const useSecurity = () => useQuery({ queryKey: ['security'], queryFn: api.getSecurity });
export const useGuide = () => {
  const p = useP();
  return useQuery({ queryKey: ['guide', p], queryFn: api.getGuide, staleTime: 60_000 });
};
export const useDvr = () => useQuery({ queryKey: ['dvr'], queryFn: api.getDvr });

// ---------- admin ----------
export const useAdminOverview = () => useQuery({ queryKey: ['admin', 'overview'], queryFn: api.getAdminOverview });
export const useSessions = () => useQuery({ queryKey: ['admin', 'sessions'], queryFn: api.getSessions, refetchInterval: 5000 });
export const useActivity = () => useQuery({ queryKey: ['admin', 'activity'], queryFn: api.getActivity });
export const useLibrariesAdmin = () => useQuery({ queryKey: ['admin', 'libraries'], queryFn: api.getLibrariesAdmin });
export const useUsers = () => useQuery({ queryKey: ['admin', 'users'], queryFn: api.getUsers });
export const useTasks = () => useQuery({ queryKey: ['admin', 'tasks'], queryFn: api.getTasks });
export function useAdminSection<T>(section: 'transcoding' | 'network' | 'storage' | 'backups' | 'livetv' | 'metadata' | 'diagnostics' | 'settings') {
  return useQuery({ queryKey: ['admin', section], queryFn: () => api.getAdminSection<T>(section) });
}

// ---------- mutations ----------
export function useInvalidate() {
  const qc = useQueryClient();
  return (...keys: string[]) => Promise.all(keys.map((k) => qc.invalidateQueries({ queryKey: [k] })));
}

export function useToggleWatchlist() {
  const inv = useInvalidate();
  return useMutation({ mutationFn: api.toggleWatchlist, onSuccess: () => inv('watchlist', 'movie', 'show', 'episode', 'audiobook', 'homeVideo') });
}
export function useSetWatched() {
  const inv = useInvalidate();
  return useMutation({
    mutationFn: ({ id, watched }: { id: string; watched: boolean }) => api.setWatched(id, watched),
    onSuccess: () => inv('movie', 'show', 'episode', 'home', 'libraryItems', 'libraryHome', 'history', 'audiobook'),
  });
}
export function useStartDownload() {
  const inv = useInvalidate();
  return useMutation({ mutationFn: ({ id, quality }: { id: string; quality: Parameters<typeof api.startDownload>[1] }) => api.startDownload(id, quality), onSuccess: () => inv('downloads', 'movie', 'show', 'episode', 'audiobook', 'album') });
}
