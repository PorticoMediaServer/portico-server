import type {LibraryView, SavedTab} from '../data/types';

/** Every destination in the signed-in app, with its parameters. */
export type AppRoutes = {
  Home: undefined;
  HomeRow: {rowId: string; title?: string};
  Library: {libraryId: string; view?: LibraryView};
  Entity: {libraryId: string; kind: string; id: string; title?: string};
  Movie: {id: string};
  HomeVideo: {id: string};
  Show: {id: string; season?: number};
  Episode: {id: string};
  Person: {id: string};
  Artist: {id: string};
  Album: {id: string};
  Audiobook: {id: string};
  PhotoAlbum: {id: string};
  Resource: {kind: 'playlist' | 'collection' | 'view'; id: string};
  Search: {query?: string} | undefined;
  Saved: {tab?: SavedTab} | undefined;
  Live: {view?: 'guide' | 'channels' | 'recordings'; source?: string} | undefined;
  Together: undefined;
  Downloads: undefined;
  Settings: undefined;
  SettingsSection: {section: 'preferences' | 'account' | 'servers' | 'notifications' | 'about' | 'profiles' | 'security' | 'devices' | 'linkTv' | 'screens'};
  Account: {tab?: 'profiles' | 'sessions' | 'security' | 'servers' | 'notifications' | 'invitations'} | undefined;
  Admin: {section?: string; param?: string} | undefined;
  NotFound: undefined;
};

export type AuthRoutes = {
  Landing: undefined;
  AccountSignIn: {mode?: 'signIn' | 'create' | 'forgot'} | undefined;
  TwoStep: {next: 'servers' | 'profiles'};
  DirectSignIn: {host?: string} | undefined;
  ServerChooser: undefined;
  Profiles: {serverId?: string} | undefined;
  Join: {code?: string} | undefined;
  Activate: undefined;
  QuickConnect: undefined;
  Claim: undefined;
  Setup: undefined;
  Restore: undefined;
  VerifyEmail: {token?: string} | undefined;
  ResetPassword: {token?: string} | undefined;
  ServerUnavailable: {serverId?: string} | undefined;
  Offline: undefined;
  DeviceApproval: undefined;
};

export type RootRoutes = AuthRoutes & {Main: undefined};
export type AppRouteName = keyof AppRoutes;
