import type {IconName} from '@/ui/Icon';
import type {Channel, Entry, Program} from '@/data/types';

/** Live TV entries in navigation: every channel, each source by name, and Library Channels. */
export const SOURCES: {id: string; name: string; icon: IconName}[] = [
  {id: 'all', name: 'All channels', icon: 'guide'},
  {id: 'src-hdhr', name: 'HDHomeRun Flex 4K', icon: 'tuner'},
  {id: 'src-m3u', name: 'Harbor IPTV', icon: 'live'},
  {id: 'library', name: 'Library Channels', icon: 'channels'},
];

/** A channel as a playable entry, titled with what's on now. */
export const channelEntry = (ch: Channel, now?: Program): Entry => ({id: ch.id, kind: 'channel', title: now?.title ?? ch.name, subtitle: `${ch.number} · ${ch.name}`, image: now?.image, shape: 'wide'});
