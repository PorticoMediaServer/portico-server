// PorticoApi over the Portico Server REST API (v3). Every method maps onto one
// endpoint; responses are the same JSON shapes the fixture returns, so screens
// don't change when a real server is connected.
import type { PorticoApi } from './api';
import { ApiError } from './latency';

export function createHttpApi(base: string): PorticoApi {
  const root = base.replace(/\/$/, '') + '/api/v3';
  let token: string | null = null;
  try {
    token = localStorage.getItem('portico.token');
  } catch {
    token = null;
  }
  const profile = () => {
    try {
      return JSON.parse(localStorage.getItem('portico.session.v1') ?? '{}').profileId ?? '';
    } catch {
      return '';
    }
  };

  async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await fetch(root + path, {
      method,
      headers: {
        Accept: 'application/json',
        ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        'X-Portico-Profile': profile(),
        'X-Portico-Client': 'portico-web/3.2',
      },
      body: body !== undefined ? JSON.stringify(body) : undefined,
      credentials: 'include',
    });
    if (!res.ok) {
      const msg = await res.json().then((j) => j.message as string).catch(() => res.statusText);
      throw new ApiError(msg || `Request failed (${res.status})`, res.status);
    }
    if (res.status === 204) return undefined as T;
    const json = await res.json();
    if (json && typeof json === 'object' && 'token' in json && typeof json.token === 'string') {
      token = json.token;
      try {
        localStorage.setItem('portico.token', json.token);
      } catch {
        /* ignore */
      }
    }
    return json as T;
  }
  const get = <T,>(p: string) => call<T>('GET', p);
  const post = <T,>(p: string, b: unknown = {}) => call<T>('POST', p, b);
  const patch = <T,>(p: string, b: unknown) => call<T>('PATCH', p, b);
  const del = <T,>(p: string) => call<T>('DELETE', p);
  const qs = (o: Record<string, unknown>) => {
    const s = new URLSearchParams();
    for (const [k, v] of Object.entries(o)) if (v !== undefined && v !== null && v !== '') s.set(k, Array.isArray(v) ? v.join(',') : String(v));
    const str = s.toString();
    return str ? `?${str}` : '';
  };
  const e = encodeURIComponent;

  return {
    listServers: () => get('/servers'),
    signIn: (email, password) => post('/auth/password', { email, password }),
    verifyMfa: (code) => post('/auth/mfa', { code }),
    signInWithPasskey: () => post('/auth/passkey', {}),
    getProfiles: () => get('/profiles'),
    unlockProfile: (id, pin) => post(`/profiles/${e(id)}/unlock`, { pin }),
    createProfile: (p) => post('/profiles', p),
    updateProfile: (id, p) => patch(`/profiles/${e(id)}`, p),
    deleteProfile: (id) => del(`/profiles/${e(id)}`),

    getLibraries: () => get('/libraries'),
    getHome: () => get('/home'),
    getLibraryHome: (id) => get(`/libraries/${e(id)}/home`),
    getLibraryItems: (id, q) => get(`/libraries/${e(id)}/items${qs({ ...q })}`),
    getMovie: (id) => get(`/movies/${e(id)}`),
    getShow: (id) => get(`/shows/${e(id)}`),
    getEpisode: (id) => get(`/episodes/${e(id)}`),
    getCollection: (id) => get(`/collections/${e(id)}`),
    getCollections: (lib) => get(`/libraries/${e(lib)}/collections`),
    getPerson: (id) => get(`/people/${e(id)}`),
    getMusicHome: () => get('/music/home'),
    getArtists: () => get('/music/artists'),
    getAlbums: (sort) => get(`/music/albums${qs({ sort })}`),
    getSongs: (offset, limit, sort) => get(`/music/tracks${qs({ offset, limit, sort })}`),
    getArtist: (id) => get(`/music/artists/${e(id)}`),
    getAlbum: (id) => get(`/music/albums/${e(id)}`),
    getPlaylists: () => get('/music/playlists'),
    getPlaylist: (id) => get(`/music/playlists/${e(id)}`),
    getTracks: (ids) => get(`/music/tracks${qs({ ids })}`),
    getAudiobooks: () => get('/audiobooks'),
    getAudiobook: (id) => get(`/audiobooks/${e(id)}`),
    getPhotos: (album) => get(`/photos${qs({ album })}`),
    getPhotoAlbums: () => get('/photos/albums'),
    getMemories: () => get('/photos/memories'),
    getHomeVideos: () => get('/home-videos'),
    getHomeVideo: (id) => get(`/home-videos/${e(id)}`),
    search: (q, kinds) => get(`/search${qs({ q, kinds })}`),
    getRecentSearches: () => get('/search/recent'),

    getWatchlist: () => get('/me/watchlist'),
    toggleWatchlist: (id) => post(`/me/watchlist/${e(id)}/toggle`),
    setWatched: (id, watched) => post(`/me/watched/${e(id)}`, { watched }),
    setRating: (id, rating) => post(`/me/ratings/${e(id)}`, { rating }),
    getHistory: () => get('/me/history'),
    clearHistory: () => del('/me/history'),
    saveProgress: (id, position, duration) => post(`/me/progress/${e(id)}`, { position, duration }),
    getContinue: () => get('/me/continue'),
    getDownloads: () => get('/me/downloads'),
    startDownload: (itemId, quality) => post('/me/downloads', { itemId, quality }),
    removeDownload: (id) => del(`/me/downloads/${e(id)}`),
    getNotifications: () => get('/me/notifications'),
    markNotificationsRead: (ids) => post('/me/notifications/read', { ids }),
    getCastTargets: () => get('/cast/targets'),
    getSecurity: () => get('/me/security'),
    revokeDevice: (id) => del(`/me/devices/${e(id)}`),

    getPlayable: (id) => get(`/playback/${e(id)}/source`),

    getGuide: () => get('/live/guide'),
    getDvr: () => get('/live/dvr'),
    scheduleRecording: (programId, series) => post('/live/dvr/schedule', { programId, series }),
    cancelRecording: (id) => del(`/live/dvr/schedule/${e(id)}`),
    deleteRecording: (id) => del(`/live/dvr/recordings/${e(id)}`),

    getAdminOverview: () => get('/admin/overview'),
    getMetrics: (since) => get(`/admin/metrics${qs({ since })}`),
    getSessions: () => get('/admin/sessions'),
    terminateSession: (id, message) => post(`/admin/sessions/${e(id)}/terminate`, { message }),
    getActivity: () => get('/admin/activity'),
    getLogs: (q) => get(`/admin/logs${qs(q)}`),
    getLibrariesAdmin: () => get('/admin/libraries'),
    scanLibrary: (id) => post(`/admin/libraries/${e(id)}/scan`),
    getUsers: () => get('/admin/users'),
    updateUser: (id, p) => patch(`/admin/users/${e(id)}`, p),
    inviteUser: (email, libraries) => post('/admin/users/invite', { email, libraries }),
    getTasks: () => get('/admin/tasks'),
    runTask: (id) => post(`/admin/tasks/${e(id)}/run`),
    getAdminSection: (section) => get(`/admin/${section}`),
    updateAdminSection: (section, p) => patch(`/admin/${section}`, p),
    runBackup: () => post('/admin/backups/run'),
    runDiagnostics: () => post('/admin/diagnostics/run'),
  };
}
