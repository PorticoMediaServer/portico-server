/**
 * Playback Protocol v1 client core (Plan — Client Playback Migration, phases 0–1). Not wired to
 * any app yet: the `/v2` path stays in use until lane C lands the server side.
 */
export type {MediaPlayer, PlayerEvent, PlayerObservation, PlayerSource, PlayerState, SidecarSubtitle} from './port.ts';
export {ServerClock, driftCorrection, timelinePosition, type ClockEstimate, type ClockSample, type DriftCorrection} from './clock.ts';
export {GenerationGate} from './generation.ts';
export {TimelineReporter, type FailureKind, type MarkerType, type TimelineReport, type TimelineReporterOptions, type TimelineSend, type TimelineSnapshot, type TimelineState} from './timeline.ts';
export {automaticRung, qualityDecision, qualityLadder, qualityLane, qualityPreferences, sameQuality, type NetworkClass, type QualityDecision, type QualityLane, type QualityMode, type QualityPreferences, type QualityRequest} from './quality.ts';
export {PlaybackApiError, call, header, idempotencyKey, type RetryClass, type V1Http, type V1Request, type V1Response} from './http.ts';
export {ShufflePermutation} from './shuffle.ts';
export {ContractError, consecutiveOnAlbum, parseAdminSession, parseDevice, parseEntry, parseEvent, parseOptions, parsePage, parseQueue, parseSession, parseSessionEnd, type AdminSession, type AudioStream, type Chapter, type ControllableDevice, type Decision, type Marker, type MediaVersion, type PlanStream, type PlaybackOptions, type PlaybackPlan, type Presentation, type QueueEntry, type QueueHeader, type QueueSegment, type Selector, type ServerEvent, type Session, type SessionEnd, type SessionKind, type StreamAction, type SubtitleStream, type Trickplay, type VersionPart, type VideoStream} from './types.ts';
export {CapabilityPublisher, PlaybackOptionsClient, defaultSelection, markerAt, summarizePlan, type CapabilityProfile, type OptionsPreview, type PlanSummary} from './options.ts';
export {PlaybackSessionController, SessionsClient, seekNeedsServer, type ControllerOptions, type ControllerPhase, type ControllerSnapshot, type SessionChange, type StartOptions, type StartTarget, type Timers} from './sessions.ts';
export {QUEUE_WINDOW_MAX, QueueClient, QueueView, parseQueuePlaylistSave, queueSaveMessage, saveQueueAsPlaylist, type QueuePlaylistSave, type CreateResult, type Placement, type QueueAnchor, type QueueOrder, type SegmentInput} from './queue.ts';
export {CommandRouter, DeviceCommandsClient, commandHandler, parseCommand, type CommandHandler, type CommandTarget, type DeviceCommand, type PlayTarget, type ReceivedCommand, type TransferOffer} from './commands.ts';
export {EventsClient, type EventsClientOptions, type EventsStatus, type RawStreamEvent, type StreamOpener} from './events.ts';
export {AdminSessionsClient, NowPlayingStore, type NowPlayingSnapshot} from './admin-sessions.ts';
export {localV1Http} from './local-http.ts';
export {AppleMediaPlayer, type AppleFact, type AppleLease, type ApplePlayerOptions, type AppleVideoEngine} from './apple-media-player.ts';
export {V1PlaybackApi, toPlaybackSession} from './legacy-bridge.ts';
export {V1QueuePlayer, movedEntry, playbackV1ForKind, serverSupportsPlaybackV1, type V1QueuePlayerOptions, type V1Viewer} from './v1-queue-player.ts';
export {sessionEndMessage} from '../session-end.ts';
export {audioGainLinear, audioSeconds, audioTrimWindow, parseAudioRenderV2, playableAudio, type AudioDecodeCapability, type AudioGain, type AudioRenderMode, type AudioRenderV2, type AudioTrim, type Normalization, type PlayableAudioRender} from './audio-render.ts';
export {V1ChannelControl, serverSupportsV1Channels, channelView, parseChannelSession, v1ChannelId, type ChannelPlayback, type ChannelView, type ChannelViewSink, type V1ChannelOptions} from './channel-control.ts';
export {capabilityProfileFromClient, networkClassOf, type CapabilityOptions} from './capability-profile.ts';
