import type {HttpLocalApi, PlaybackOptions as ServiceOptions, PlaybackSnapshot} from '@core/index.ts';
import type {ContentEntry, ContentScope} from '@core/library-content.ts';
import type {PlaybackSubject} from '@core/presentation/kit-model/index.ts';
import type {PlayerOffersData} from '@core/player-offers.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import type {V1Viewer} from '@core/playback-v1/v1-queue-player.ts';
import type {PlaybackOptions, Session} from '@core/playback-v1/types.ts';
import type {Timers} from '@core/playback-v1/sessions.ts';
import type {SubtitlePreferences} from '@core/preference-effects.ts';
import type {CapabilityDocument} from '@core/client-profile.ts';
import type {PlaybackMediaPort, RenderEngine} from './web-playback.ts';
import type {PlaybackService} from '@core/index.ts';
import type {WebMediaPlayerOptions} from './web-media-player.ts';
import type {AudioEffectsSettings} from '@core/audio-effects.ts';
import type {QualityRequest} from '@core/playback-v1/quality.ts';
import type {QueuePlaybackView} from '@core/queue-controller.ts';
import type {SelectionContext} from '@core/selection-model/types.ts';
import type {DockState, DockTransition, PlaybackFence, PolicyView, SkipMode} from '../../../../packages/player-core/src/index.ts';
import type {HlsModule, VideoLike, WebPlayerTimers} from './web-media-player.ts';

export type WebPlaybackAuthority = V1Viewer & Readonly<{epoch: string | number; viewerFence: string}>;
export type WebPlaybackStart = Readonly<{entry: ContentEntry; target: NonNullable<ContentEntry['playback']>; origin?: Readonly<{kind: 'home-row'; rowId: string}>}>;
export type WebPlaybackVersionStart = Readonly<{itemId: string; versionId: string; context: SelectionContext; signal: AbortSignal}>;
export type WebPlaybackScreenTarget = Readonly<{libraryId: string; itemId: string; startSeconds: number; versionId?: string}>;
export type WebPlaybackScreenLifetime = Readonly<{signal: AbortSignal; isCurrent(): boolean}>;
export type WebPlaybackSettings = Readonly<{
  subtitlePreferences?: () => SubtitlePreferences;
  videoSpeed: () => number;
  audiobookSpeed: () => number;
  /** Undefined leaves the saved delivery default to the server. Original is an explicit choice. */
  defaultQuality?: () => QualityRequest | undefined;
  skipModes: () => Readonly<Record<string, SkipMode>>;
  audioEffects: () => AudioEffectsSettings;
  musicDefaults?: () => Readonly<{shuffle: boolean; repeat: 'off' | 'one' | 'all'}> | undefined;
  webVolume: () => boolean;
}>;
/** The shell supplies an actual viewer-bound transport/feed, and destroys this runtime before
 * replacing that authority. isCurrent also fences an async callback during shell teardown. */
export type WebPlaybackSource = Readonly<{
  api: HttpLocalApi; authority: WebPlaybackAuthority; contentScope: ContentScope;
  events: Pick<EventsClient, 'on'>; isCurrent: () => boolean;
  settings: WebPlaybackSettings;
}>;
export type WebPlaybackEnvironment = Readonly<{
  /** Build-time platform injection. null disables the browser music engine. */
  createPlayer?: (video: VideoLike, options: WebMediaPlayerOptions) => PlaybackMediaPort;
  createRender?: ((service: PlaybackService, video: VideoLike) => RenderEngine) | null;
  capabilities?: () => Promise<CapabilityDocument>;
  subtitleSurface?: boolean;
  page?: Document; browser?: Window; fetch?: typeof fetch;
  loadHls?: () => Promise<HlsModule>; nativeHls?: 'first' | 'fallback' | 'never';
  timers?: Timers; playerTimers?: WebPlayerTimers; key?: () => string;
  service?: Pick<ServiceOptions, 'startupTimeoutMs' | 'seekTimeoutMs' | 'operationTimeoutMs'>;
}>;
export type WebPlaybackControl = 'play' | 'pause' | 'replay' | 'seek' | 'previous' | 'next' | 'volume';
export type WebPlaybackPresentation = Readonly<{
  fence: PlaybackFence; subject: PlaybackSubject; kind: 'video' | 'music' | 'audiobook';
  posterUrl?: string; backdropUrl?: string; durationSeconds: number; positionSeconds: number;
  playing: boolean; expanded: boolean; finished: boolean; controls: readonly WebPlaybackControl[];
  seekWindow?: Readonly<{startSeconds: number; endSeconds: number}>;
  volume: number; muted: boolean; rate: number; policy: PolicyView | null;
}>;
export type WebPlaybackRuntimeSnapshot = Readonly<{
  nativeVideoSurfaceActive: boolean;
  authority: WebPlaybackAuthority; sequence: number; playback: PlaybackSnapshot; dock: DockState;
  fence?: PlaybackFence; subject?: PlaybackSubject; presentation?: WebPlaybackPresentation;
  presentations: readonly WebPlaybackPresentation[];
  queueView: QueuePlaybackView | null;
  session?: Session; offers: PlayerOffersData | null; options: PlaybackOptions | null;
  problem?: string; disposed: boolean;
}>;
/** Structural input accepted directly from DockBinding. Unsupported presentation controls are
 * rejected, never listed as a capability. Remote ids are rejected without a real remote binding. */
export type WebPlaybackAction = Readonly<{
  kind: string; fence: PlaybackFence; remote?: Readonly<{id: string; sequence: string}>;
  seconds?: number; volume?: number; shuffle?: boolean; repeat?: 'off' | 'one' | 'all';
  direction?: -1 | 1;
}>;
export type WebPlaybackTransition = DockTransition;
export type WebPlaybackElement = VideoLike;
