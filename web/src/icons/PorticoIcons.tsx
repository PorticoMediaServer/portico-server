import {createElement, forwardRef, type ForwardRefExoticComponent, type RefAttributes, type SVGProps} from 'react';

import {iconNodes, type PorticoIconId, semanticToMaster} from './generated';

export type {PorticoIconId} from './generated';

export type PorticoIconState = 'default' | 'focused' | 'selected' | 'disabled' | 'destructive';

export interface PorticoIconProps extends Omit<SVGProps<SVGSVGElement>, 'color' | 'height' | 'width'> {
  id: PorticoIconId;
  size?: number | string;
  color?: string;
  state?: PorticoIconState;
  strokeWidth?: number;
  absoluteStrokeWidth?: boolean;
}

export type LucideProps = Omit<PorticoIconProps, 'id' | 'state'>;
export type LucideIcon = ForwardRefExoticComponent<LucideProps & RefAttributes<SVGSVGElement>>;

const filledSelectedIds: ReadonlySet<PorticoIconId> = new Set(['action.favorite', 'action.watchlist']);

export const PorticoIcon = forwardRef<SVGSVGElement, PorticoIconProps>(function PorticoIcon(
  {
    id,
    size = 24,
    color = 'currentColor',
    state = 'default',
    strokeWidth = 2,
    absoluteStrokeWidth = false,
    className,
    ...svgProps
  },
  ref,
) {
  const master = semanticToMaster[id];
  if (!master) throw new Error(`Unknown Portico semantic icon ID: ${String(id)}`);
  const nodes = iconNodes[master];
  if (!nodes) throw new Error(`Missing generated Portico icon master: ${master}`);
  const numericSize = typeof size === 'number' ? size : Number.parseFloat(size);
  const resolvedStrokeWidth = absoluteStrokeWidth && Number.isFinite(numericSize) && numericSize > 0
    ? (strokeWidth * 24) / numericSize
    : strokeWidth;
  const fill = state === 'selected' && filledSelectedIds.has(id) ? color : 'none';
  return (
    <svg
      aria-hidden="true"
      className={['lucide', `lucide-${master}`, 'portico-icon', className].filter(Boolean).join(' ')}
      color={color}
      fill={fill}
      height={size}
      ref={ref}
      stroke={color}
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={resolvedStrokeWidth}
      viewBox="0 0 24 24"
      width={size}
      xmlns="http://www.w3.org/2000/svg"
      {...svgProps}
    >
      {nodes.map(([tag, attributes], index) => createElement(tag, {key: `${master}-${index}`, ...attributes}))}
    </svg>
  );
});

export function semanticIcon(id: PorticoIconId): LucideIcon {
  const Icon = forwardRef<SVGSVGElement, LucideProps>(function SemanticPorticoIcon(props, ref) {
    return <PorticoIcon {...props} id={id} ref={ref} />;
  });
  Icon.displayName = `PorticoIcon(${id})`;
  return Icon;
}

// Compatibility-shaped names keep the Web migration mechanical while every
// exported component is governed by a semantic ID and the shared pinned
// geometry. New code should prefer <PorticoIcon id="…" /> directly.
export const Activity = semanticIcon('playback.technical-stats');
export const AlertTriangle = semanticIcon('status.warning');
export const Antenna = semanticIcon('device.network');
export const Archive = semanticIcon('action.archive');
export const ArchiveRestore = semanticIcon('action.undo');
export const ArrowDown = semanticIcon('navigation.move-down');
export const ArrowLeft = semanticIcon('navigation.back');
export const ArrowRight = semanticIcon('navigation.forward');
export const ArrowUp = semanticIcon('navigation.move-up');
export const BadgeCheck = semanticIcon('account.verified');
export const Ban = semanticIcon('status.error');
export const Bell = semanticIcon('communication.notifications');
export const Bookmark = semanticIcon('action.watchlist');
export const BookmarkPlus = semanticIcon('action.add-to-list');
export const BookOpen = semanticIcon('navigation.library');
export const CalendarClock = semanticIcon('metadata.time');
export const CalendarDays = semanticIcon('media.calendar');
export const Camera = semanticIcon('media.photo');
export const Captions = semanticIcon('playback.captions');
export const Check = semanticIcon('action.confirm');
export const CheckCheck = semanticIcon('status.success');
export const CheckCircle2 = semanticIcon('status.success');
export const ChevronDown = semanticIcon('navigation.expand');
export const ChevronFirst = semanticIcon('playback.previous');
export const ChevronLast = semanticIcon('playback.next');
export const ChevronLeft = semanticIcon('navigation.previous');
export const ChevronRight = semanticIcon('navigation.disclosure');
export const ChevronUp = semanticIcon('navigation.collapse');
export const CircleAlert = semanticIcon('status.warning');
export const CircleCheck = semanticIcon('status.success');
export const CircleCheckBig = semanticIcon('status.success');
export const CircleHelp = semanticIcon('status.info');
export const CircleOff = semanticIcon('status.error');
export const CirclePlay = semanticIcon('playback.autoplay');
export const CircleUserRound = semanticIcon('account.profile');
export const CircleX = semanticIcon('status.error');
export const Clipboard = semanticIcon('view.list');
export const ClipboardCopy = semanticIcon('view.list');
export const Clock3 = semanticIcon('metadata.time');
export const CloudDownload = semanticIcon('action.prepare-download');
export const Cpu = semanticIcon('playback.technical-stats');
export const Database = semanticIcon('device.storage');
export const DatabaseBackup = semanticIcon('action.archive');
export const Disc3 = semanticIcon('media.music');
export const Download = semanticIcon('action.download');
export const DownloadCloud = semanticIcon('action.prepare-download');
export const Ellipsis = semanticIcon('action.more');
export const ExternalLink = semanticIcon('action.open-external');
export const Eye = semanticIcon('account.visibility.show');
export const EyeOff = semanticIcon('account.visibility.hide');
export const FastForward = semanticIcon('playback.seek-forward');
export const FileMusic = semanticIcon('media.music');
export const FileText = semanticIcon('view.details');
export const FileWarning = semanticIcon('status.warning');
export const Film = semanticIcon('media.movie');
export const Flag = semanticIcon('action.report');
export const Folder = semanticIcon('library.collection');
export const FolderHeart = semanticIcon('library.saved');
export const FolderLock = semanticIcon('status.locked');
export const FolderOpen = semanticIcon('library.collection');
export const FolderPlus = semanticIcon('action.add');
export const Gauge = semanticIcon('playback.quality');
export const Globe2 = semanticIcon('device.network');
export const Grid3X3 = semanticIcon('view.grid');
export const GripVertical = semanticIcon('action.customize');
export const HardDrive = semanticIcon('device.storage');
export const Headphones = semanticIcon('media.audiobook');
export const Heart = semanticIcon('action.favorite');
export const History = semanticIcon('metadata.time');
export const Home = semanticIcon('navigation.home');
export const Image = semanticIcon('media.photo');
export const ImageIcon = semanticIcon('status.artwork-unavailable');
export const ImagePlus = semanticIcon('action.add');
export const Inbox = semanticIcon('status.empty');
export const Info = semanticIcon('metadata.info');
export const KeyRound = semanticIcon('account.security');
export const Laptop = semanticIcon('device.client');
export const LayoutList = semanticIcon('view.list');
export const Library = semanticIcon('navigation.library');
export const LibraryBig = semanticIcon('navigation.library');
export const Link2 = semanticIcon('device.network');
export const Link2Off = semanticIcon('device.offline');
export const List = semanticIcon('view.list');
export const ListMusic = semanticIcon('media.playlist');
export const ListPlus = semanticIcon('action.add-to-list');
export const ListVideo = semanticIcon('playback.queue');
export const LoaderCircle = semanticIcon('status.loading');
export const Lock = semanticIcon('status.locked');
export const LockKeyhole = semanticIcon('status.locked');
export const LockOpen = semanticIcon('account.security');
export const LogIn = semanticIcon('account.sign-in');
export const LogOut = semanticIcon('account.sign-out');
export const Mail = semanticIcon('action.mark-unread');
export const Maximize2 = semanticIcon('action.fullscreen-enter');
export const MemoryStick = semanticIcon('device.storage');
export const Menu = semanticIcon('view.list');
export const MessageSquare = semanticIcon('communication.message');
export const MessageSquareWarning = semanticIcon('communication.report');
export const Mic2 = semanticIcon('communication.microphone');
export const Minimize2 = semanticIcon('action.fullscreen-exit');
export const MonitorCog = semanticIcon('navigation.settings');
export const MonitorSmartphone = semanticIcon('device.client');
export const MoreHorizontal = semanticIcon('action.more');
export const Music = semanticIcon('media.music');
export const Music2 = semanticIcon('media.music');
export const Network = semanticIcon('device.network');
export const PanelLeftClose = semanticIcon('navigation.collapse');
export const PanelLeftOpen = semanticIcon('navigation.expand');
export const Pause = semanticIcon('playback.pause');
export const Pencil = semanticIcon('action.edit');
export const Pin = semanticIcon('action.pin');
export const PinOff = semanticIcon('action.unpin');
export const Play = semanticIcon('playback.play');
export const Plus = semanticIcon('action.add');
export const Radio = semanticIcon('navigation.channels');
export const RadioTower = semanticIcon('status.live');
export const RefreshCw = semanticIcon('action.refresh');
export const Repeat = semanticIcon('playback.repeat');
export const Repeat1 = semanticIcon('playback.repeat-one');
export const Rewind = semanticIcon('playback.seek-back');
export const RotateCcw = semanticIcon('action.reset');
export const RotateCw = semanticIcon('playback.seek-forward');
export const Rows3 = semanticIcon('view.list');
export const Save = semanticIcon('action.confirm');
export const ScanSearch = semanticIcon('navigation.search');
export const Search = semanticIcon('navigation.search');
export const Server = semanticIcon('device.server');
export const ServerCog = semanticIcon('navigation.settings');
export const ServerOff = semanticIcon('device.offline');
export const Settings = semanticIcon('navigation.settings');
export const Settings2 = semanticIcon('preference.playback');
export const Shapes = semanticIcon('media.collection');
export const ShieldAlert = semanticIcon('status.warning');
export const ShieldCheck = semanticIcon('status.secure');
export const ShieldOff = semanticIcon('status.error');
export const Shuffle = semanticIcon('playback.shuffle');
export const SkipBack = semanticIcon('playback.previous');
export const SkipForward = semanticIcon('playback.next');
export const SlidersHorizontal = semanticIcon('action.customize');
export const Sparkles = semanticIcon('action.customize');
export const Square = semanticIcon('status.active');
export const Star = semanticIcon('action.rate');
export const Table2 = semanticIcon('view.grid');
export const TerminalSquare = semanticIcon('device.server');
export const ThumbsUp = semanticIcon('action.like');
export const Timer = semanticIcon('metadata.time');
export const Trash2 = semanticIcon('action.delete');
export const TriangleAlert = semanticIcon('status.warning');
export const Tv = semanticIcon('device.tv');
export const TvMinimalPlay = semanticIcon('media.live-tv');
export const Upload = semanticIcon('action.send');
export const UserPlus = semanticIcon('account.profiles');
export const UserRound = semanticIcon('account.user');
export const UserRoundCheck = semanticIcon('account.verified');
export const Users = semanticIcon('account.profiles');
export const UsersRound = semanticIcon('account.watch-together');
export const Video = semanticIcon('media.video');
export const Volume2 = semanticIcon('playback.volume');
export const VolumeX = semanticIcon('playback.mute');
export const WandSparkles = semanticIcon('action.customize');
export const Wifi = semanticIcon('device.wifi');
export const WifiOff = semanticIcon('device.offline');
export const X = semanticIcon('action.close');
