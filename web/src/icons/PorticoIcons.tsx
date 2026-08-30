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

export type PorticoSemanticIconProps = Omit<PorticoIconProps, 'id' | 'state'>;
export type PorticoSemanticIconComponent = ForwardRefExoticComponent<PorticoSemanticIconProps & RefAttributes<SVGSVGElement>>;

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
      className={['portico-icon', `portico-icon-${master}`, className].filter(Boolean).join(' ')}
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

function createSemanticIconComponent(id: PorticoIconId): PorticoSemanticIconComponent {
  const Icon = forwardRef<SVGSVGElement, PorticoSemanticIconProps>(function SemanticPorticoIcon(props, ref) {
    return <PorticoIcon {...props} id={id} ref={ref} />;
  });
  Icon.displayName = `PorticoIcon(${id})`;
  return Icon;
}

export const PlaybackTechnicalStatsIcon = createSemanticIconComponent('playback.technical-stats');
export const StatusWarningIcon = createSemanticIconComponent('status.warning');
export const DeviceNetworkIcon = createSemanticIconComponent('device.network');
export const ActionArchiveIcon = createSemanticIconComponent('action.archive');
export const ActionUndoIcon = createSemanticIconComponent('action.undo');
export const NavigationMoveDownIcon = createSemanticIconComponent('navigation.move-down');
export const NavigationBackIcon = createSemanticIconComponent('navigation.back');
export const NavigationForwardIcon = createSemanticIconComponent('navigation.forward');
export const NavigationMoveUpIcon = createSemanticIconComponent('navigation.move-up');
export const AccountVerifiedIcon = createSemanticIconComponent('account.verified');
export const StatusErrorIcon = createSemanticIconComponent('status.error');
export const CommunicationNotificationsIcon = createSemanticIconComponent('communication.notifications');
export const ActionWatchlistIcon = createSemanticIconComponent('action.watchlist');
export const ActionAddToListIcon = createSemanticIconComponent('action.add-to-list');
export const NavigationLibraryIcon = createSemanticIconComponent('navigation.library');
export const MetadataTimeIcon = createSemanticIconComponent('metadata.time');
export const MediaCalendarIcon = createSemanticIconComponent('media.calendar');
export const MediaPhotoIcon = createSemanticIconComponent('media.photo');
export const PlaybackCaptionsIcon = createSemanticIconComponent('playback.captions');
export const ActionConfirmIcon = createSemanticIconComponent('action.confirm');
export const StatusSuccessIcon = createSemanticIconComponent('status.success');
export const NavigationExpandIcon = createSemanticIconComponent('navigation.expand');
export const PlaybackPreviousIcon = createSemanticIconComponent('playback.previous');
export const PlaybackNextIcon = createSemanticIconComponent('playback.next');
export const NavigationPreviousIcon = createSemanticIconComponent('navigation.previous');
export const NavigationDisclosureIcon = createSemanticIconComponent('navigation.disclosure');
export const NavigationCollapseIcon = createSemanticIconComponent('navigation.collapse');
export const StatusInfoIcon = createSemanticIconComponent('status.info');
export const PlaybackAutoplayIcon = createSemanticIconComponent('playback.autoplay');
export const AccountProfileIcon = createSemanticIconComponent('account.profile');
export const ViewListIcon = createSemanticIconComponent('view.list');
export const ActionPrepareDownloadIcon = createSemanticIconComponent('action.prepare-download');
export const DeviceStorageIcon = createSemanticIconComponent('device.storage');
export const MediaMusicIcon = createSemanticIconComponent('media.music');
export const ActionDownloadIcon = createSemanticIconComponent('action.download');
export const ActionMoreIcon = createSemanticIconComponent('action.more');
export const ActionOpenExternalIcon = createSemanticIconComponent('action.open-external');
export const AccountVisibilityShowIcon = createSemanticIconComponent('account.visibility.show');
export const AccountVisibilityHideIcon = createSemanticIconComponent('account.visibility.hide');
export const PlaybackSeekForwardIcon = createSemanticIconComponent('playback.seek-forward');
export const ViewDetailsIcon = createSemanticIconComponent('view.details');
export const MediaMovieIcon = createSemanticIconComponent('media.movie');
export const ActionReportIcon = createSemanticIconComponent('action.report');
export const LibraryCollectionIcon = createSemanticIconComponent('library.collection');
export const LibrarySavedIcon = createSemanticIconComponent('library.saved');
export const StatusLockedIcon = createSemanticIconComponent('status.locked');
export const ActionAddIcon = createSemanticIconComponent('action.add');
export const PlaybackQualityIcon = createSemanticIconComponent('playback.quality');
export const ViewGridIcon = createSemanticIconComponent('view.grid');
export const ActionCustomizeIcon = createSemanticIconComponent('action.customize');
export const MediaAudiobookIcon = createSemanticIconComponent('media.audiobook');
export const ActionFavoriteIcon = createSemanticIconComponent('action.favorite');
export const NavigationHomeIcon = createSemanticIconComponent('navigation.home');
export const StatusArtworkUnavailableIcon = createSemanticIconComponent('status.artwork-unavailable');
export const StatusEmptyIcon = createSemanticIconComponent('status.empty');
export const MetadataInfoIcon = createSemanticIconComponent('metadata.info');
export const AccountSecurityIcon = createSemanticIconComponent('account.security');
export const DeviceClientIcon = createSemanticIconComponent('device.client');
export const DeviceOfflineIcon = createSemanticIconComponent('device.offline');
export const MediaPlaylistIcon = createSemanticIconComponent('media.playlist');
export const PlaybackQueueIcon = createSemanticIconComponent('playback.queue');
export const StatusLoadingIcon = createSemanticIconComponent('status.loading');
export const AccountSignInIcon = createSemanticIconComponent('account.sign-in');
export const AccountSignOutIcon = createSemanticIconComponent('account.sign-out');
export const ActionMarkUnreadIcon = createSemanticIconComponent('action.mark-unread');
export const ActionFullscreenEnterIcon = createSemanticIconComponent('action.fullscreen-enter');
export const CommunicationMessageIcon = createSemanticIconComponent('communication.message');
export const CommunicationReportIcon = createSemanticIconComponent('communication.report');
export const CommunicationMicrophoneIcon = createSemanticIconComponent('communication.microphone');
export const ActionFullscreenExitIcon = createSemanticIconComponent('action.fullscreen-exit');
export const NavigationSettingsIcon = createSemanticIconComponent('navigation.settings');
export const PlaybackPauseIcon = createSemanticIconComponent('playback.pause');
export const ActionEditIcon = createSemanticIconComponent('action.edit');
export const ActionPinIcon = createSemanticIconComponent('action.pin');
export const ActionUnpinIcon = createSemanticIconComponent('action.unpin');
export const PlaybackPlayIcon = createSemanticIconComponent('playback.play');
export const NavigationChannelsIcon = createSemanticIconComponent('navigation.channels');
export const StatusLiveIcon = createSemanticIconComponent('status.live');
export const ActionRefreshIcon = createSemanticIconComponent('action.refresh');
export const PlaybackRepeatIcon = createSemanticIconComponent('playback.repeat');
export const PlaybackRepeatOneIcon = createSemanticIconComponent('playback.repeat-one');
export const PlaybackSeekBackIcon = createSemanticIconComponent('playback.seek-back');
export const ActionResetIcon = createSemanticIconComponent('action.reset');
export const NavigationSearchIcon = createSemanticIconComponent('navigation.search');
export const DeviceServerIcon = createSemanticIconComponent('device.server');
export const PreferencePlaybackIcon = createSemanticIconComponent('preference.playback');
export const MediaCollectionIcon = createSemanticIconComponent('media.collection');
export const StatusSecureIcon = createSemanticIconComponent('status.secure');
export const PlaybackShuffleIcon = createSemanticIconComponent('playback.shuffle');
export const StatusActiveIcon = createSemanticIconComponent('status.active');
export const ActionRateIcon = createSemanticIconComponent('action.rate');
export const ActionLikeIcon = createSemanticIconComponent('action.like');
export const ActionDeleteIcon = createSemanticIconComponent('action.delete');
export const DeviceTvIcon = createSemanticIconComponent('device.tv');
export const MediaLiveTvIcon = createSemanticIconComponent('media.live-tv');
export const ActionSendIcon = createSemanticIconComponent('action.send');
export const AccountProfilesIcon = createSemanticIconComponent('account.profiles');
export const AccountUserIcon = createSemanticIconComponent('account.user');
export const AccountWatchTogetherIcon = createSemanticIconComponent('account.watch-together');
export const MediaVideoIcon = createSemanticIconComponent('media.video');
export const PlaybackVolumeIcon = createSemanticIconComponent('playback.volume');
export const PlaybackMuteIcon = createSemanticIconComponent('playback.mute');
export const DeviceWifiIcon = createSemanticIconComponent('device.wifi');
export const ActionCloseIcon = createSemanticIconComponent('action.close');
