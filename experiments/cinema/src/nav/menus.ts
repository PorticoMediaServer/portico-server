import {Platform} from 'react-native';
import {ui} from '../design/overlays';
import type {Session} from '../state/session';
import {go, goSection, replaceAll} from './routes';
import type {IconName} from '../design/Icon';
import type {LibraryKind} from '../data/types';

export const libraryIcon: Record<LibraryKind, IconName> = {movies: 'film', shows: 'shows', anime: 'sparkles', music: 'music', audiobooks: 'headphones', photos: 'images', homevideos: 'video'};

/** The profile menu: the same actions from the TV rail, the iPad sidebar and the phone header. */
export function openProfileMenu(session: Session & {signOut: () => void}, unread: number) {
  const manager = session.role === 'owner' || session.role === 'admin';
  ui.menu({
    title: session.profile?.name ?? 'Profile',
    subtitle: `${session.server?.name ?? 'Portico'} · ${session.via === 'account' ? 'Portico Account' : 'Server account'}`,
    actions: [
      {label: 'Switch profile', icon: 'users', onPress: () => go('Profiles', {switching: true})},
      {label: 'Switch server', icon: 'server', onPress: () => go('Chooser')},
      {label: unread ? `Notifications · ${unread}` : 'Notifications', icon: 'bell', onPress: () => goSection('Notifications')},
      {label: 'Account', icon: 'user', detail: 'Profiles, security and devices', onPress: () => go('Account')},
      {label: 'Settings', icon: 'settings', onPress: () => goSection('Settings')},
      ...(Platform.isTV ? [] : [{label: 'Downloads', icon: 'download' as IconName, onPress: () => goSection('Downloads')}]),
      {label: 'Watch Together', icon: 'together', onPress: () => goSection('Together')},
      ...(manager ? [{label: 'Manage server', icon: 'server' as IconName, detail: 'Server administration', onPress: () => goSection('Admin', {section: 'overview'})}] : []),
      {label: 'Send feedback…', icon: 'message', onPress: () => go('Feedback')},
      {label: 'Help and support', icon: 'help', onPress: () => ui.toast('Help opens getportico.tv/help in a browser.')},
      {
        label: 'Sign out', icon: 'logout', destructive: true,
        onPress: () => ui.confirm({title: `Sign out of ${session.server?.name ?? 'this server'} on this ${Platform.isTV ? 'TV' : 'device'}?`, body: Platform.isTV ? 'You’ll need to sign this TV in again.' : 'Downloads stay on this device.', confirmLabel: 'Sign out', destructive: true, onConfirm: () => { session.signOut(); replaceAll('SignIn'); }}),
      },
    ],
  });
}
