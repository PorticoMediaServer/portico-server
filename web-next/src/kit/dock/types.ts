import type {CSSProperties, ReactNode, RefObject} from 'react';
import type {FactPart, PlaybackIdentityModel} from '@core/presentation/kit-model/index.ts';
import type {DockPlayback, DockState, PlaybackFence, RemoteDock} from '../../../../packages/player-core/src/index.ts';
import type {AmbientArtSource, ArtworkPalette, ArtworkSource} from '../artwork';
import type {dockMessages} from '../../../../packages/i18n/src/catalog/en-US/next/dock.ts';

export type DockMessageId = keyof typeof dockMessages;
export type DockTranslate = (id: DockMessageId, values?: Record<string, string | number>) => string;
export type DockLayout = 'desktop' | 'phone' | 'tv';
export type DockMode = DockPlayback['kind'] | RemoteDock['kind'];
export type DockControl = 'play' | 'pause' | 'replay' | 'previous' | 'next' | 'seek' | 'subtitles-audio' | 'options' | 'play-on' | 'lyrics' | 'queue' | 'chapters' | 'speed' | 'sleep' | 'shuffle' | 'repeat' | 'volume';
export type DockActionKind = DockControl | 'expand' | 'collapse' | 'stop' | 'mute' | 'continue-here' | 'dismiss' | 'restore-audio' | 'restore-video';
export type DockIntent = Readonly<
  {kind: Exclude<DockActionKind, 'seek' | 'volume' | 'shuffle' | 'repeat'>} |
  {kind: 'seek'; seconds: number} | {kind: 'volume'; volume: number} |
  {kind: 'shuffle'; shuffle: boolean} | {kind: 'repeat'; repeat: 'off' | 'all' | 'one'}
>;
export type DockAction = DockIntent & Readonly<{fence: PlaybackFence; remote?: Readonly<{id: string; sequence: string}>}>;
/** Supplied by the current engine owner. Dispatch must never fabricate session effects. */
export type DockBinding = Readonly<{getSnapshot: () => DockState; onAction: (action: DockAction) => void}>;
export type DockPresentation = Readonly<{
  fence: PlaybackFence; identity: PlaybackIdentityModel;
  artwork?: ArtworkSource | null; ambient?: AmbientArtSource | null; palette?: ArtworkPalette;
  album?: readonly FactPart[]; durationSeconds?: number;
  seekWindow?: Readonly<{startSeconds: number; endSeconds: number}>;
  controls: readonly DockControl[]; finished?: boolean;
  previousLabel?: string; nextLabel?: string; skipBackSeconds: number; skipForwardSeconds: number;
  shuffle?: boolean; repeat?: 'off' | 'all' | 'one'; rate?: number; sleepLabel?: string;
  volume?: number; muted?: boolean; deviceName?: string; remoteError?: string;
}>;
export type DockView = Readonly<{mode: DockMode; playback: DockPlayback; remote?: RemoteDock; presentation: DockPresentation}>;
export type DockRendererProps = Readonly<{
  view: DockView; binding: DockBinding; t?: DockTranslate; className?: string; style?: CSSProperties;
  artSlotRef?: RefObject<HTMLSpanElement | null>; videoSurface?: boolean; artworkSurface?: boolean;
  identityRef?: RefObject<HTMLButtonElement | null>;
}>;
export type DockHostProps = Readonly<{
  state: DockState; presentations: readonly DockPresentation[]; binding: DockBinding; layout: DockLayout;
  t?: DockTranslate; className?: string; style?: CSSProperties;
  artSlotRef?: RefObject<HTMLSpanElement | null>; videoSurface?: boolean; artworkSurface?: boolean;
  /** Paint the SAME actual current music occupant while its full view is open; caller makes it inert. */
  retainExpandedMusic?: boolean;
  tvExpanded?: boolean; onTVExpandedChange?: (expanded: boolean) => void;
  tvReturnFocusRef?: RefObject<HTMLElement | null>;
}>;
export type MediaSurfaceProps = Readonly<{
  children: ReactNode; expanded: boolean; dockSlot: RefObject<HTMLElement | null>;
  expandedSlot: RefObject<HTMLElement | null>; hidden?: boolean; finished?: boolean; className?: string;
  /** Actual current native frame observation from the runtime; ordinary web playback omits it. */
  nativeRenderActive?: boolean;
}>;
