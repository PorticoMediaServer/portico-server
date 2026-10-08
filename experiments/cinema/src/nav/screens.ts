import type React from 'react';
import type {NativeStackNavigationOptions} from '@react-navigation/native-stack';
import type {RouteName} from './routes';
import {Placeholder} from '../screens/shared/Placeholder';
import {HomeScreen, HomeEmptyScreen} from '../screens/home/Home';
import {LandingScreen, ProfilesScreen} from '../screens/auth/Welcome';
import {SignInScreen, ChooserScreen, DeviceWaitingScreen, JoinScreen, LinkTVScreen, DeviceApproveScreen, ClaimScreen, SetupScreen, RestoreScreen, VerifyEmailScreen, ResetPasswordScreen, AccountReturnScreen, ServerUnavailableScreen, OfflineScreen, SignInEndedScreen, NotFoundScreen} from '../screens/auth/Auth';
import {LibrariesScreen, LibraryScreen, EntityScreen, NoLibrariesScreen} from '../screens/library/Library';
import {MovieScreen, ShowScreen, EpisodeScreen, PersonScreen} from '../screens/title/Titles';
import {PlayerScreen} from '../screens/player/Player';
import {NowPlayingScreen} from '../screens/player/NowPlaying';
import {HomeRowScreen, HomeCustomiseScreen, NotificationsScreen, StatesScreen, ScreenIndexScreen} from '../screens/home/HomeExtras';
import {ArtistScreen, AlbumScreen, AudiobookScreen, PhotoAlbumScreen, PhotoViewerScreen} from '../screens/media/Media';
import {SearchScreen} from '../screens/browse/Search';
import {SavedScreen, ResourceScreen} from '../screens/browse/Saved';
import {LiveScreen} from '../screens/live/Live';
import {LivePlayerScreen} from '../screens/live/LivePlayer';
import {TogetherScreen, TogetherGroupScreen, TogetherJoinScreen} from '../screens/together/Together';
import {DownloadsScreen} from '../screens/downloads/Downloads';
import {SettingsScreen, PreferencesScreen, ProfilesSettingsScreen, ProfileLimitsScreen, ServersSettingsScreen, AboutScreen} from '../screens/settings/Settings';
import {AccountScreen} from '../screens/settings/Account';
import {AdminScreen, AdminDeniedScreen} from '../screens/admin/Admin';
import {MetadataEditorScreen, FeedbackScreen} from '../screens/title/MetadataEditor';

type Entry = {name: RouteName; component: React.ComponentType<any>; options?: NativeStackNavigationOptions};

const P = Placeholder;
const modal: NativeStackNavigationOptions = {animation: 'fade', gestureEnabled: false};

/** Every route and the component that draws it. */
export const screens: Entry[] = [
  {name: 'Landing', component: LandingScreen, options: {animation: 'fade'}},
  {name: 'SignIn', component: SignInScreen},
  {name: 'Chooser', component: ChooserScreen},
  {name: 'Profiles', component: ProfilesScreen, options: {animation: 'fade'}},
  {name: 'DeviceWaiting', component: DeviceWaitingScreen},
  {name: 'Join', component: JoinScreen},
  {name: 'LinkTV', component: LinkTVScreen},
  {name: 'DeviceApprove', component: DeviceApproveScreen},
  {name: 'Claim', component: ClaimScreen},
  {name: 'Setup', component: SetupScreen},
  {name: 'Restore', component: RestoreScreen},
  {name: 'VerifyEmail', component: VerifyEmailScreen},
  {name: 'ResetPassword', component: ResetPasswordScreen},
  {name: 'AccountReturn', component: AccountReturnScreen},
  {name: 'ServerUnavailable', component: ServerUnavailableScreen},
  {name: 'Offline', component: OfflineScreen},
  {name: 'SignInEnded', component: SignInEndedScreen},
  {name: 'Home', component: HomeScreen, options: {animation: 'fade'}},
  {name: 'HomeRow', component: HomeRowScreen},
  {name: 'HomeCustomise', component: HomeCustomiseScreen},
  {name: 'HomeEmpty', component: HomeEmptyScreen},
  {name: 'Libraries', component: LibrariesScreen, options: {animation: 'fade'}},
  {name: 'Library', component: LibraryScreen, options: {animation: 'fade'}},
  {name: 'Entity', component: EntityScreen},
  {name: 'NoLibraries', component: NoLibrariesScreen},
  {name: 'Movie', component: MovieScreen},
  {name: 'Show', component: ShowScreen},
  {name: 'Episode', component: EpisodeScreen},
  {name: 'Person', component: PersonScreen},
  {name: 'Artist', component: ArtistScreen},
  {name: 'Album', component: AlbumScreen},
  {name: 'Audiobook', component: AudiobookScreen},
  {name: 'PhotoAlbum', component: PhotoAlbumScreen},
  {name: 'PhotoViewer', component: PhotoViewerScreen, options: modal},
  {name: 'Search', component: SearchScreen, options: {animation: 'fade'}},
  {name: 'Saved', component: SavedScreen, options: {animation: 'fade'}},
  {name: 'Resource', component: ResourceScreen},
  {name: 'Live', component: LiveScreen, options: {animation: 'fade'}},
  {name: 'Together', component: TogetherScreen, options: {animation: 'fade'}},
  {name: 'TogetherGroup', component: TogetherGroupScreen},
  {name: 'TogetherJoin', component: TogetherJoinScreen},
  {name: 'Downloads', component: DownloadsScreen, options: {animation: 'fade'}},
  {name: 'Notifications', component: NotificationsScreen, options: {animation: 'fade'}},
  {name: 'Settings', component: SettingsScreen, options: {animation: 'fade'}},
  {name: 'SettingsPreferences', component: PreferencesScreen},
  {name: 'SettingsProfiles', component: ProfilesSettingsScreen},
  {name: 'ProfileLimits', component: ProfileLimitsScreen},
  {name: 'SettingsServers', component: ServersSettingsScreen},
  {name: 'SettingsAbout', component: AboutScreen},
  {name: 'ScreenIndex', component: ScreenIndexScreen},
  {name: 'Account', component: AccountScreen},
  {name: 'Admin', component: AdminScreen, options: {animation: 'fade'}},
  {name: 'AdminDenied', component: AdminDeniedScreen},
  {name: 'MetadataEditor', component: MetadataEditorScreen},
  {name: 'Feedback', component: FeedbackScreen},
  {name: 'States', component: StatesScreen},
  {name: 'NotFound', component: NotFoundScreen},
  {name: 'Player', component: PlayerScreen, options: modal},
  {name: 'LivePlayer', component: LivePlayerScreen, options: modal},
  {name: 'NowPlaying', component: NowPlayingScreen, options: {animation: 'slide_from_bottom'}},
];
