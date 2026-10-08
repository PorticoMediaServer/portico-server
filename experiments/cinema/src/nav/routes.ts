import {createNavigationContainerRef, StackActions, CommonActions} from '@react-navigation/native';

export type Routes = {
  // Getting in
  Landing: undefined;
  SignIn: {mode?: 'account' | 'direct' | 'create' | 'forgot' | 'code' | 'newPassword' | 'recovery'} | undefined;
  Chooser: undefined;
  Profiles: {switching?: boolean} | undefined;
  DeviceWaiting: undefined;
  Join: {direct?: boolean} | undefined;
  LinkTV: undefined;
  DeviceApprove: undefined;
  Claim: undefined;
  Setup: undefined;
  Restore: undefined;
  VerifyEmail: {token?: string} | undefined;
  ResetPassword: undefined;
  AccountReturn: undefined;
  ServerUnavailable: undefined;
  Offline: undefined;
  SignInEnded: undefined;
  // Main
  Home: undefined;
  HomeRow: {rowId: string};
  HomeCustomise: undefined;
  HomeEmpty: undefined;
  Libraries: undefined;
  Library: {libraryId: string; view?: string};
  Entity: {libraryId: string; view: string; entityId: string};
  NoLibraries: {member?: boolean} | undefined;
  Movie: {id: string};
  Show: {id: string; season?: string};
  Episode: {id: string};
  Person: {id: string};
  Artist: {id: string};
  Album: {id: string};
  Audiobook: {id: string};
  PhotoAlbum: {id: string};
  PhotoViewer: {albumId: string; index: number; slideshow?: boolean};
  Search: {query?: string} | undefined;
  Saved: {tab?: string} | undefined;
  Resource: {kind: 'playlist' | 'collection' | 'view'; id: string};
  Live: {tab?: 'guide' | 'channels' | 'dvr'} | undefined;
  Together: undefined;
  TogetherGroup: {id: string};
  TogetherJoin: {code?: string} | undefined;
  Downloads: undefined;
  Notifications: undefined;
  Settings: undefined;
  SettingsPreferences: undefined;
  SettingsProfiles: undefined;
  ProfileLimits: {profileId: string};
  SettingsServers: undefined;
  SettingsAbout: undefined;
  ScreenIndex: undefined;
  Account: {tab?: 'profiles' | 'security' | 'sessions' | 'devices' | 'notifications' | 'servers' | 'invitations'} | undefined;
  Admin: {section?: string} | undefined;
  AdminDenied: undefined;
  MetadataEditor: {id: string; ids?: string[]};
  Feedback: {itemId?: string; playback?: boolean} | undefined;
  States: undefined;
  NotFound: undefined;
  // Playback
  Player: {id: string; from?: number; queue?: string[]; trailer?: boolean};
  LivePlayer: {channelId: string};
  NowPlaying: undefined;
};

export type RouteName = keyof Routes;

export const navigationRef = createNavigationContainerRef<Routes>();

/** Routes that draw without the rail / tab bar (full-screen experiences). */
export const chromeless = new Set<RouteName>([
  'Landing', 'SignIn', 'Chooser', 'Profiles', 'DeviceWaiting', 'Join', 'LinkTV', 'DeviceApprove', 'Claim', 'Setup', 'Restore', 'VerifyEmail', 'ResetPassword',
  'AccountReturn', 'ServerUnavailable', 'Offline', 'SignInEnded', 'Player', 'LivePlayer', 'NowPlaying', 'PhotoViewer', 'TogetherJoin',
]);

/** Top-level destinations reached from the rail or the tab bar: switching between them resets the stack. */
export const topLevel = new Set<RouteName>(['Home', 'Search', 'Saved', 'Live', 'Downloads', 'Together', 'Settings', 'Libraries', 'Library', 'Admin', 'Notifications']);

export function go<K extends RouteName>(name: K, ...params: undefined extends Routes[K] ? [] | [Routes[K]] : [Routes[K]]) {
  if (!navigationRef.isReady()) return;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (navigationRef.navigate as any)(name, params[0]);
}

export function push<K extends RouteName>(name: K, ...params: undefined extends Routes[K] ? [] | [Routes[K]] : [Routes[K]]) {
  if (!navigationRef.isReady()) return;
  navigationRef.dispatch(StackActions.push(name, params[0] as object | undefined));
}

/** Rail/tab destinations: the section becomes the only thing on the stack above Home. */
export function goSection<K extends RouteName>(name: K, ...params: undefined extends Routes[K] ? [] | [Routes[K]] : [Routes[K]]) {
  if (!navigationRef.isReady()) return;
  const routes = name === 'Home' ? [{name: 'Home'}] : [{name: 'Home'}, {name, params: params[0]}];
  navigationRef.dispatch(CommonActions.reset({index: routes.length - 1, routes: routes as never}));
}

export function replaceAll<K extends RouteName>(name: K, ...params: undefined extends Routes[K] ? [] | [Routes[K]] : [Routes[K]]) {
  if (!navigationRef.isReady()) return;
  navigationRef.dispatch(CommonActions.reset({index: 0, routes: [{name, params: params[0]} as never]}));
}

export function back() {
  if (navigationRef.isReady() && navigationRef.canGoBack()) navigationRef.goBack();
}
