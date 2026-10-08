import { lazy } from 'react';
import { createBrowserRouter, Navigate } from 'react-router';
import { Bare, RequireSession, Shell } from './Shell';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const L = <T extends Record<string, any>>(f: () => Promise<T>, name: keyof T) =>
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  lazy(() => f().then((m) => ({ default: m[name] as React.ComponentType<any> })));

const Welcome = L(() => import('../features/auth/Auth'), 'Welcome');
const SignIn = L(() => import('../features/auth/Auth'), 'SignIn');
const Servers = L(() => import('../features/auth/Auth'), 'Servers');
const Profiles = L(() => import('../features/auth/Profiles'), 'Profiles');
const Setup = L(() => import('../features/auth/Setup'), 'Setup');
const Home = L(() => import('../features/home/Home'), 'Home');
const MoviesPage = L(() => import('../features/library/Movies'), 'MoviesPage');
const ShowsPage = L(() => import('../features/library/Shows'), 'ShowsPage');
const LibraryHub = L(() => import('../features/library/Hub'), 'LibraryHub');
const CollectionPage = L(() => import('../features/library/Collection'), 'CollectionPage');
const MoviePage = L(() => import('../features/detail/Movie'), 'MoviePage');
const ShowPage = L(() => import('../features/detail/Show'), 'ShowPage');
const EpisodePage = L(() => import('../features/detail/Episode'), 'EpisodePage');
const PersonPage = L(() => import('../features/detail/Person'), 'PersonPage');
const HomeVideoPage = L(() => import('../features/detail/HomeVideo'), 'HomeVideoPage');
const MusicPage = L(() => import('../features/music/Music'), 'MusicPage');
const AlbumPage = L(() => import('../features/music/Album'), 'AlbumPage');
const ArtistPage = L(() => import('../features/music/Artist'), 'ArtistPage');
const PlaylistPage = L(() => import('../features/music/Playlist'), 'PlaylistPage');
const AudiobooksPage = L(() => import('../features/books/Books'), 'AudiobooksPage');
const AudiobookPage = L(() => import('../features/books/Books'), 'AudiobookPage');
const PhotosPage = L(() => import('../features/photos/Photos'), 'PhotosPage');
const HomeVideosPage = L(() => import('../features/library/HomeVideos'), 'HomeVideosPage');
const LivePage = L(() => import('../features/live/Live'), 'LivePage');
const DvrPage = L(() => import('../features/live/Dvr'), 'DvrPage');
const SearchPage = L(() => import('../features/search/Search'), 'SearchPage');
const WatchlistPage = L(() => import('../features/personal/Personal'), 'WatchlistPage');
const HistoryPage = L(() => import('../features/personal/Personal'), 'HistoryPage');
const DownloadsPage = L(() => import('../features/personal/Downloads'), 'DownloadsPage');
const SettingsPage = L(() => import('../features/settings/Settings'), 'SettingsPage');
const VideoPlayer = L(() => import('../features/player/VideoPlayer'), 'VideoPlayer');
const ConsoleShell = L(() => import('../features/console/ConsoleShell'), 'ConsoleShell');
const ScreenIndex = L(() => import('../features/screens/ScreenIndex'), 'ScreenIndex');
const NotFound = L(() => import('../features/misc/NotFound'), 'NotFound');

export const router = createBrowserRouter([
  {
    element: <Bare />,
    children: [
      { path: '/welcome', element: <Welcome /> },
      { path: '/signin', element: <SignIn /> },
      { path: '/servers', element: <Servers /> },
      { path: '/profiles', element: <Profiles /> },
      { path: '/setup/:step?', element: <Setup /> },
      { path: '/screens', element: <ScreenIndex /> },
      {
        path: '/watch/:id',
        element: (
          <RequireSession>
            <VideoPlayer />
          </RequireSession>
        ),
      },
      {
        path: '/console/*',
        element: (
          <RequireSession>
            <ConsoleShell />
          </RequireSession>
        ),
      },
    ],
  },
  {
    element: <Shell />,
    children: [
      { path: '/', element: <Home /> },
      { path: '/movies', element: <MoviesPage /> },
      { path: '/tv', element: <ShowsPage anime={false} /> },
      { path: '/anime', element: <ShowsPage anime /> },
      { path: '/library', element: <LibraryHub /> },
      { path: '/collection/:id', element: <CollectionPage /> },
      { path: '/movie/:id', element: <MoviePage /> },
      { path: '/show/:id', element: <ShowPage /> },
      { path: '/episode/:id', element: <EpisodePage /> },
      { path: '/person/:id', element: <PersonPage /> },
      { path: '/home-video/:id', element: <HomeVideoPage /> },
      { path: '/music', element: <MusicPage /> },
      { path: '/album/:id', element: <AlbumPage /> },
      { path: '/artist/:id', element: <ArtistPage /> },
      { path: '/playlist/:id', element: <PlaylistPage /> },
      { path: '/audiobooks', element: <AudiobooksPage /> },
      { path: '/audiobook/:id', element: <AudiobookPage /> },
      { path: '/photos', element: <PhotosPage /> },
      { path: '/home-videos', element: <HomeVideosPage /> },
      { path: '/live', element: <LivePage /> },
      { path: '/live/dvr', element: <DvrPage /> },
      { path: '/search', element: <SearchPage /> },
      { path: '/watchlist', element: <WatchlistPage /> },
      { path: '/history', element: <HistoryPage /> },
      { path: '/downloads', element: <DownloadsPage /> },
      { path: '/settings/:section?', element: <SettingsPage /> },
      { path: '/account', element: <Navigate to="/settings" replace /> },
      { path: '*', element: <NotFound /> },
    ],
  },
]);
