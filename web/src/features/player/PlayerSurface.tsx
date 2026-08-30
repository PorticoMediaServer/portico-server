import { activeTrickplaySet, ApiError, burnInSubtitleIDFor, defaultPlaybackQuality, effectivePlaybackVolume, playbackDecisionLabel, playbackSegmentAutomationDecision, playbackSelectionRequiresHLS, playbackResourceUrl, playbackSourceFor, playerContentMode, productMessage, createPlaybackAutomationState, reducePlaybackAutomation, reduceUpNextCountdown, segmentLabel, supportsTrickplayPreview, watchWithFriendsTargetPosition, type MediaItem, type MediaTrickplaySet, type PlaybackCommand, type PlaybackHandoffRequest, type PlaybackPreparedResponse, type PlaybackPrepareNextRequest, type PlaybackProgressInput, type PlaybackRenegotiationRequest, type PlaybackRepeatMode, type PlaybackResponse, type PlaybackSessionQueueRequest, type PlaybackSessionQueueResponse, type UpNextCountdownState, } from '@porticomediaserver/client-core';
import type HlsInstance from 'hls.js';
import { ActionConfirmIcon, NavigationExpandIcon, ActionCustomizeIcon } from '#portico-icons';
import {
  createContext,
  type CSSProperties,
  type DragEvent as ReactDragEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type RefObject,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { SecondaryButton } from '../../components/controls/Buttons';
import { AnchoredOverlay, ModalOverlay } from '../../components/overlay/OverlayPortal';
import { ProductLanguageIcon } from '../../components/product/ProductLanguageIcon';
import { useAuthSession, usePorticoDataSource } from '../../data/DataProvider';
import type { PlaybackStartOptions } from '../../data/models';
import { secureRandomUUID } from '../../runtime/secureRandomUUID';
import './player.css';
import { selectLyricDocument } from './lyrics';
import { LyricsPanel } from './LyricsPanel';
import { musicTransitionRequest, MusicTransitionBridge, nextCandidate } from './MusicTransitionBridge';
import { playbackPreparationOwner } from './PlaybackPreparationOwner';
import { accountRepeatMode, normalizeMusicPlaybackPreferences } from './musicPlayback';
import { playbackOptionsFromNavigationState } from './watchNavigation';
import { playbackCommandClientFrom, subscribeToPlaybackCommands } from './playbackCommandRuntime';
import { useMediaSession } from './useMediaSession';
import { type SleepTimerMode, useSleepTimer } from './useSleepTimer';
import { createSeekTransaction, playbackStallReason } from './playerLifecycle';
import { useOptionalWebDisplayPreferences } from '../../preferences/WebDisplayPreferencesProvider';
import { defaultWebDisplayPreferences, webPlaybackIntent } from '../../preferences/webDisplayPreferences';
import { FeedbackDialog } from '../feedback/FeedbackDialog';
import { useOptionalRuntime } from '../../runtime/RuntimeContext';
import { mediaPresentation } from '../catalog/mediaPresentation';

type PlaybackStatus = 'idle' | 'preparing' | 'ready' | 'buffering' | 'recovering' | 'completed' | 'failed';
type PlaybackFailureKind = 'route' | 'source' | 'transcode' | 'decode' | 'unknown';
type PlaybackSessionOrigin = 'start' | 'restore' | 'handoff';

type PlaybackFailure = {
  kind: PlaybackFailureKind;
  title: string;
  message: string;
};

type PlaybackInterruption = {
  title: string;
  message: string;
};

const PLAYER_VOLUME_STORAGE_KEY = 'portico.player.volume.v1';
const PLAYBACK_SPEEDS = [0.75, 1, 1.25, 1.5, 2] as const;

type StoredPlayerVolume = { volume: number; muted: boolean };

export function stablePreparedHandoffRequestID(
  cache: Map<string, string>,
  sourceSessionID: string,
  preparedSessionID: string,
  suppliedRequestID?: string,
): string {
  const key = `${sourceSessionID}:${preparedSessionID}`;
  const requestID = suppliedRequestID?.trim() || cache.get(key) || `web-${secureRandomUUID()}`;
  cache.set(key, requestID);
  return requestID;
}

export function normalizedPlaybackHandoffProgress(value: number | undefined): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, Math.floor(value)) : undefined;
}

export function loadStoredPlayerVolume(storage?: Pick<Storage, 'getItem'>): StoredPlayerVolume {
  try {
    const parsed = JSON.parse((storage ?? globalThis.localStorage)?.getItem(PLAYER_VOLUME_STORAGE_KEY) ?? '') as Partial<StoredPlayerVolume>;
    const volume = typeof parsed.volume === 'number' && Number.isFinite(parsed.volume) ? Math.min(1, Math.max(0, parsed.volume)) : 1;
    return { volume, muted: parsed.muted === true };
  } catch {
    return { volume: 1, muted: false };
  }
}

function storePlayerVolume(volume: number, muted: boolean) {
  try {
    globalThis.localStorage?.setItem(PLAYER_VOLUME_STORAGE_KEY, JSON.stringify({ volume: Math.min(1, Math.max(0, volume)), muted }));
  } catch {
    // Storage can be unavailable in hardened browser contexts; playback still works for this session.
  }
}

type PostPlayState =
  | { phase: 'inactive' }
  | { phase: 'preparing'; next: MediaItem }
  | { phase: 'countdown'; next: MediaItem; preparedSessionId: string; expiresAt: string }
  | { phase: 'passout'; next: MediaItem; preparedSessionId?: string; expiresAt?: string }
  | { phase: 'cancelled'; next: MediaItem; preparedSessionId?: string; expiresAt?: string }
  | { phase: 'failed'; next: MediaItem; message: string; preparationRequest: PlaybackPrepareNextRequest }
  | { phase: 'exhausted' };

type PlaybackContextValue = {
  status: PlaybackStatus;
  playback?: PlaybackResponse;
  queue?: PlaybackSessionQueueResponse;
  queueError?: string;
  queueBusy: boolean;
  queueNeedsRefresh: boolean;
  repeatMode: PlaybackRepeatMode;
  sessionOrigin: PlaybackSessionOrigin;
  error?: string;
  failure?: PlaybackFailure;
  interruption?: PlaybackInterruption;
  postPlay: PostPlayState;
  mediaRef: RefObject<HTMLVideoElement | null>;
  start: (mediaId: string, options?: PlaybackStartOptions) => Promise<void>;
  startLive: (channelId: string) => Promise<PlaybackResponse | undefined>;
  startLibraryChannel: (channelId: string) => Promise<PlaybackResponse | undefined>;
  startDVR: (recordingId: string) => Promise<PlaybackResponse | undefined>;
  retry: () => Promise<void>;
  close: () => Promise<void>;
  complete: (durationSeconds: number) => Promise<void>;
  touch: (event: PlaybackProgressInput, keepalive?: boolean) => Promise<void>;
  renewGrant: () => Promise<void>;
  renegotiate: (request: Omit<PlaybackRenegotiationRequest, 'requestId' | 'expectedRevision' | 'clientProfile'>) => Promise<PlaybackResponse | undefined>;
  recoverRoute: () => Promise<void>;
  adapterRecoveryGeneration: number;
  completedSessionId?: string;
  next: (automatic?: boolean, preparationRequest?: PlaybackPrepareNextRequest) => Promise<boolean>;
  prepareNext: (candidate: MediaItem, request?: PlaybackPrepareNextRequest, force?: boolean) => Promise<PlaybackPreparedResponse>;
  handoff: (request: PlaybackHandoffRequest) => Promise<PlaybackResponse | undefined>;
  previous: () => Promise<void>;
  appendQueue: (mediaIds: string[]) => Promise<void>;
  playNext: (mediaIds: string[]) => Promise<void>;
  removeQueueItem: (index: number) => Promise<void>;
  moveQueueItem: (fromIndex: number, toIndex: number) => Promise<void>;
  shuffleQueue: () => Promise<void>;
  setRepeatMode: (mode: PlaybackRepeatMode) => Promise<void>;
  reloadQueue: () => Promise<void>;
  beginPostPlay: (candidate?: MediaItem, force?: boolean, preparationRequest?: PlaybackPrepareNextRequest) => Promise<void>;
  cancelPostPlay: () => void;
  replay: () => Promise<void>;
  markReady: () => void;
  markBuffering: () => void;
  markRecovering: () => void;
  fail: (kind?: PlaybackFailureKind) => void;
  interrupt: (interruption: PlaybackInterruption) => Promise<void>;
  dismissInterruption: () => void;
  applyExternalCommand: (command: PlaybackCommand) => Promise<void>;
  markMeaningfulInteraction: () => void;
};

const PlaybackContext = createContext<PlaybackContextValue | null>(null);

function isQueueRevisionConflict(reason: unknown) {
  return reason instanceof ApiError && reason.status === 409 && reason.code === 'queue_revision_conflict';
}

type QueueMutationInput = Omit<PlaybackSessionQueueRequest, 'expectedRevision'>;

function playbackFailure(reason: unknown, fallback: PlaybackFailureKind = 'unknown'): PlaybackFailure | undefined {
  if (reason instanceof DOMException && reason.name === 'AbortError') return undefined;
  const error = reason as { code?: string; status?: number } | undefined;
  const code = error?.code?.toLocaleLowerCase() ?? '';
  const networkFailure = reason instanceof TypeError || ['network', 'connection', 'route', 'timeout', 'server_unavailable', 'gateway'].some((token) => code.includes(token));
  const sourceFailure = ['source', 'media_missing', 'media_not_found', 'media_not_playable', 'file', 'offline'].some((token) => code.includes(token));
  const transcodeFailure = ['transcode', 'conversion', 'prepare_failed'].some((token) => code.includes(token));
  const kind: PlaybackFailureKind = networkFailure ? 'route' : sourceFailure ? 'source' : transcodeFailure ? 'transcode' : fallback;
  const copy = productMessage(kind === 'route' ? 'playback.route-failed' : kind === 'source' ? 'playback.source-failed' : kind === 'transcode' ? 'playback.transcode-failed' : kind === 'decode' ? 'playback.decode-failed' : 'playback.start-failed');
  return { kind, title: copy.title ?? '', message: copy.body ?? '' };
}

function shuffled<T>(items: T[]) {
  const output = [...items];
  for (let index = output.length - 1; index > 0; index -= 1) {
    const random = globalThis.crypto.getRandomValues(new Uint32Array(1))[0] / 2 ** 32;
    const target = Math.floor(random * (index + 1));
    [output[index], output[target]] = [output[target], output[index]];
  }
  return output;
}

export function PlaybackSessionProvider({ children }: { children: ReactNode }) {
  const source = usePorticoDataSource();
  const runtime = useOptionalRuntime();
  const auth = useAuthSession();
  const location = useLocation();
  const preferences = useOptionalWebDisplayPreferences()?.preferences ?? defaultWebDisplayPreferences;
  const mediaRef = useRef<HTMLVideoElement>(null);
  const [status, setStatus] = useState<PlaybackStatus>('idle');
  const [playback, setPlaybackState] = useState<PlaybackResponse>();
  const [queue, setQueue] = useState<PlaybackSessionQueueResponse>();
  const [queueError, setQueueError] = useState<string>();
  const [queueBusy, setQueueBusy] = useState(false);
  const [queueNeedsRefresh, setQueueNeedsRefresh] = useState(false);
  const [error, setError] = useState<string>();
  const [failure, setFailure] = useState<PlaybackFailure>();
  const [interruption, setInterruption] = useState<PlaybackInterruption>();
  const [postPlay, setPostPlay] = useState<PostPlayState>({ phase: 'inactive' });
  const [sessionOrigin, setSessionOrigin] = useState<PlaybackSessionOrigin>('restore');
  const [adapterRecoveryGeneration, setAdapterRecoveryGeneration] = useState(0);
  const [completedSessionId, setCompletedSessionId] = useState<string>();
  const playbackRef = useRef<PlaybackResponse | undefined>(undefined);
  const queueRef = useRef<PlaybackSessionQueueResponse | undefined>(undefined);
  const queueMutationRef = useRef(false);
  const queueNeedsRefreshRef = useRef(false);
  const queueControllerRef = useRef<AbortController | undefined>(undefined);
  const queueRetryTimerRef = useRef<number | undefined>(undefined);
  const preparedNextRef = useRef<PlaybackPreparedResponse | undefined>(undefined);
  const operationRef = useRef(0);
  const controllerRef = useRef<AbortController | undefined>(undefined);
  const renegotiationControllerRef = useRef<AbortController | undefined>(undefined);
  const grantRenewalRef = useRef<{ sessionId: string; promise: Promise<void> } | undefined>(undefined);
  const completedSessionRef = useRef('');
  const terminalOwnerRef = useRef<{ sessionId: string; promise: Promise<void> } | undefined>(undefined);
  const preparedHandoffRequestIDsRef = useRef(new Map<string, string>());
  const startingMediaRef = useRef('');
  const retryRef = useRef<({ kind: 'media'; mediaId: string; options: PlaybackStartOptions } | { kind: 'live'; channelId: string } | { kind: 'library-channel'; channelId: string } | { kind: 'dvr'; recordingId: string }) | undefined>(undefined);
  const restoreAttemptedRef = useRef(false);
  const pendingExternalCommandRef = useRef<PlaybackCommand | undefined>(undefined);
  const automationStateRef = useRef(createPlaybackAutomationState());
  const deliveryIntent = useCallback(() => webPlaybackIntent(preferences), [preferences]);
  const markMeaningfulInteraction = useCallback(() => {
    automationStateRef.current = reducePlaybackAutomation(
      automationStateRef.current,
      { type: 'meaningful-interaction', now: Date.now() },
      { passoutProtection: preferences.passoutProtection, passoutAfterEpisodes: preferences.passoutAfterEpisodes },
    ).state;
  }, [preferences.passoutAfterEpisodes, preferences.passoutProtection]);

  const setPlayback = useCallback((value: PlaybackResponse | undefined) => {
    playbackRef.current = value;
    setPlaybackState(value);
  }, []);

  useEffect(() => {
    const viewerRuntime = runtime?.viewerRuntime;
    viewerRuntime?.setPlaybackContinuityActive(Boolean(playback?.sessionId));
    return () => viewerRuntime?.setPlaybackContinuityActive(false);
  }, [playback?.sessionId, runtime?.viewerRuntime]);

  const replaceQueue = useCallback((value: PlaybackSessionQueueResponse | undefined) => {
    queueRef.current = value;
    setQueue(value);
    const current = playbackRef.current;
    if (!value || !current || current.sessionId !== value.sessionId) return;
    const retryTarget = retryRef.current;
    if (retryTarget?.kind === 'media' && retryTarget.mediaId === current.media.id) {
      retryRef.current = {
        ...retryTarget,
        options: {
          ...retryTarget.options,
          queueMediaIds: value.items.map((item) => item.id),
          repeatMode: value.repeatMode,
          sourceContext: value.sourceContext ?? current.sourceContext,
        },
      };
    }
  }, []);

  const markQueueNeedsRefresh = useCallback((value: boolean) => {
    queueNeedsRefreshRef.current = value;
    setQueueNeedsRefresh(value);
  }, []);

  const refreshQueue = useCallback(async (value: PlaybackResponse) => {
    queueControllerRef.current?.abort();
    const controller = new AbortController();
    queueControllerRef.current = controller;
    const read = async (attempt: number) => {
      try {
        const nextQueue = await source.playbackSessionQueue(value.sessionId, controller.signal);
        if (playbackRef.current?.sessionId === value.sessionId) {
          replaceQueue(nextQueue);
          markQueueNeedsRefresh(false);
          setQueueError(undefined);
        }
      } catch {
        if (controller.signal.aborted || playbackRef.current?.sessionId !== value.sessionId) return;
        markQueueNeedsRefresh(true);
        setQueueError(productMessage('playback.queue-refresh-failed').body);
        if (attempt >= 2) return;
        queueRetryTimerRef.current = window.setTimeout(() => void read(attempt + 1), attempt === 0 ? 1_000 : 3_000);
      }
    };
    await read(0);
  }, [markQueueNeedsRefresh, replaceQueue, source]);

  const acceptPlayback = useCallback((value: PlaybackResponse, origin: PlaybackSessionOrigin = 'handoff') => {
    renegotiationControllerRef.current?.abort();
    queueControllerRef.current?.abort();
    if (queueRetryTimerRef.current !== undefined) window.clearTimeout(queueRetryTimerRef.current);
    playbackPreparationOwner.preserveSession(value.sessionId);
    const retryTarget = retryRef.current;
    const retryTargetId = retryTarget?.kind === 'media' ? retryTarget.mediaId : retryTarget?.kind === 'live' || retryTarget?.kind === 'library-channel' ? retryTarget.channelId : retryTarget?.recordingId;
    if (retryTarget?.kind === 'media' && retryTarget.mediaId === value.media.id) {
      retryRef.current = {
        ...retryTarget,
        options: {
          ...retryTarget.options,
          startSeconds: value.resumePositionSeconds,
          queueMediaIds: value.queue.map((item) => item.id),
          repeatMode: value.repeatMode,
          sourceContext: value.sourceContext,
        },
      };
    } else if (retryTargetId !== value.media.id) {
      retryRef.current = value.isLive
        ? value.sourceContext?.type === 'library-channel'
          ? { kind: 'library-channel', channelId: value.sourceContext.id || value.media.id }
          : { kind: 'live', channelId: value.media.id }
        : {
          kind: 'media',
          mediaId: value.media.id,
          options: { startSeconds: value.resumePositionSeconds, queueMediaIds: value.queue.map((item) => item.id), repeatMode: value.repeatMode, sourceContext: value.sourceContext },
        };
    }
    setPlayback(value);
    completedSessionRef.current = '';
    terminalOwnerRef.current = undefined;
    setCompletedSessionId(undefined);
    setSessionOrigin(origin);
    setStatus('ready');
    setError(undefined);
    setFailure(undefined);
    replaceQueue(undefined);
    setQueueError(undefined);
    queueMutationRef.current = false;
    setQueueBusy(false);
    markQueueNeedsRefresh(false);
    setPostPlay({ phase: 'inactive' });
    preparedNextRef.current = undefined;
    automationStateRef.current = reducePlaybackAutomation(
      automationStateRef.current,
      { type: 'session-changed', sessionId: value.sessionId, now: Date.now() },
      { passoutProtection: preferences.passoutProtection, passoutAfterEpisodes: preferences.passoutAfterEpisodes },
    ).state;
    startingMediaRef.current = '';
    void refreshQueue(value);
  }, [markQueueNeedsRefresh, preferences.passoutAfterEpisodes, preferences.passoutProtection, refreshQueue, replaceQueue, setPlayback]);

  const start = useCallback(async (mediaId: string, options: PlaybackStartOptions = {}) => {
    if (!mediaId || startingMediaRef.current === mediaId) return;
    if (playbackRef.current?.media.id === mediaId) {
      if (options.startSeconds !== undefined) {
        const media = mediaRef.current;
        if (media) {
          media.currentTime = Math.max(0, options.startSeconds);
          markMeaningfulInteraction();
          await media.play().catch(() => undefined);
        }
      }
      return;
    }
    const operation = operationRef.current + 1;
    operationRef.current = operation;
    controllerRef.current?.abort();
    renegotiationControllerRef.current?.abort();
    playbackPreparationOwner.cancel();
    const controller = new AbortController();
    controllerRef.current = controller;
    startingMediaRef.current = mediaId;
    retryRef.current = { kind: 'media', mediaId, options };
    setStatus('preparing');
    setInterruption(undefined);
    setError(undefined);
    setFailure(undefined);
    setPostPlay({ phase: 'inactive' });
    preparedNextRef.current = undefined;
    try {
      const current = playbackRef.current;
      if (current) {
        const media = mediaRef.current;
        try {
          await source.touchPlayback(current.sessionId, {
            state: media?.paused ? 'paused' : 'playing',
            progressSeconds: media?.currentTime ?? 0,
            durationSeconds: Number.isFinite(media?.duration) ? media?.duration : undefined,
          }, controller.signal);
        } catch { /* the replacement session remains actionable if the final progress event misses */ }
      }
      markMeaningfulInteraction();
      // The server commits replacement only after the candidate has a sealed
      // plan and grant. Keep the current player/session intact until that
      // succeeds so a failed prepare never destroys usable playback.
      const value = await source.startPlayback(mediaId, { ...options, intent: options.intent ?? deliveryIntent() }, controller.signal);
      if (operationRef.current === operation && !controller.signal.aborted) acceptPlayback(value, 'start');
    } catch (reason) {
      if (operationRef.current !== operation || controller.signal.aborted) return;
      startingMediaRef.current = '';
      const nextFailure = playbackFailure(reason);
      if (nextFailure) {
        setFailure(nextFailure);
        setError(nextFailure.message);
        setStatus('failed');
      }
    }
  }, [acceptPlayback, deliveryIntent, markMeaningfulInteraction, replaceQueue, setPlayback, source]);

  const startLive = useCallback(async (channelId: string) => {
    if (!channelId || (playbackRef.current?.isLive && playbackRef.current.media.id === channelId) || startingMediaRef.current === channelId) return playbackRef.current;
    const operation = operationRef.current + 1;
    operationRef.current = operation;
    controllerRef.current?.abort();
    renegotiationControllerRef.current?.abort();
    playbackPreparationOwner.cancel();
    const controller = new AbortController();
    controllerRef.current = controller;
    startingMediaRef.current = channelId;
    retryRef.current = { kind: 'live', channelId };
    setStatus('preparing');
    setInterruption(undefined);
    setError(undefined);
    setFailure(undefined);
    setPostPlay({ phase: 'inactive' });
    preparedNextRef.current = undefined;
    try {
      const value = await source.startLiveTVPlayback(channelId, controller.signal);
      if (operationRef.current === operation && !controller.signal.aborted) {
        acceptPlayback(value, 'start');
        return value;
      }
    } catch (reason) {
      if (operationRef.current !== operation || controller.signal.aborted) return;
      startingMediaRef.current = '';
      const nextFailure = playbackFailure(reason);
      if (nextFailure) {
        setFailure(nextFailure);
        setError(nextFailure.message);
        setStatus('failed');
      }
    }
  }, [acceptPlayback, replaceQueue, setPlayback, source]);

  const startDVR = useCallback(async (recordingId: string) => {
    if (!recordingId || startingMediaRef.current === recordingId) return playbackRef.current;
    const operation = operationRef.current + 1;
    operationRef.current = operation;
    controllerRef.current?.abort();
    renegotiationControllerRef.current?.abort();
    playbackPreparationOwner.cancel();
    const controller = new AbortController();
    controllerRef.current = controller;
    startingMediaRef.current = recordingId;
    retryRef.current = { kind: 'dvr', recordingId };
    setStatus('preparing');
    setInterruption(undefined);
    setError(undefined);
    setFailure(undefined);
    setPostPlay({ phase: 'inactive' });
    preparedNextRef.current = undefined;
    try {
      const value = await source.startDVRPlayback(recordingId, controller.signal);
      if (operationRef.current === operation && !controller.signal.aborted) {
        acceptPlayback(value, 'start');
        return value;
      }
    } catch (reason) {
      if (operationRef.current !== operation || controller.signal.aborted) return;
      startingMediaRef.current = '';
      const nextFailure = playbackFailure(reason);
      if (nextFailure) {
        setFailure(nextFailure);
        setError(nextFailure.message);
        setStatus('failed');
      }
    }
  }, [acceptPlayback, replaceQueue, setPlayback, source]);

  const startLibraryChannel = useCallback(async (channelId: string) => {
    if (!channelId || (playbackRef.current?.isLive && playbackRef.current.sourceContext?.type === 'library-channel' && playbackRef.current.sourceContext.id === channelId) || startingMediaRef.current === channelId) return playbackRef.current;
    const operation = operationRef.current + 1;
    operationRef.current = operation;
    controllerRef.current?.abort();
    renegotiationControllerRef.current?.abort();
    playbackPreparationOwner.cancel();
    const controller = new AbortController();
    controllerRef.current = controller;
    startingMediaRef.current = channelId;
    retryRef.current = { kind: 'library-channel', channelId };
    setStatus('preparing');
    setInterruption(undefined);
    setError(undefined);
    setFailure(undefined);
    setPostPlay({ phase: 'inactive' });
    preparedNextRef.current = undefined;
    try {
      const value = await source.startLibraryChannelPlayback(channelId, controller.signal);
      if (operationRef.current === operation && !controller.signal.aborted) {
        acceptPlayback(value, 'start');
        return value;
      }
    } catch (reason) {
      if (operationRef.current !== operation || controller.signal.aborted) return;
      startingMediaRef.current = '';
      const nextFailure = playbackFailure(reason);
      if (nextFailure) {
        setFailure(nextFailure);
        setError(nextFailure.message);
        setStatus('failed');
      }
    }
  }, [acceptPlayback, replaceQueue, setPlayback, source]);

  const renewGrant = useCallback(async () => {
    const current = playbackRef.current;
    if (!current) return;
    if (grantRenewalRef.current?.sessionId === current.sessionId) return grantRenewalRef.current.promise;
    const revision = current.playbackRevision;
    const generation = current.generation;
    const promise = (async () => {
      let lastFailure: unknown;
      for (let attempt = 0; attempt < 2; attempt += 1) {
        const active = playbackRef.current;
        if (active?.sessionId !== current.sessionId || active.playbackRevision !== revision || active.generation !== generation) return;
        try {
          const controller = new AbortController();
          const mediaGrant = await source.renewPlaybackMediaGrant(current.sessionId, controller.signal);
          const latest = playbackRef.current;
          if (latest?.sessionId === current.sessionId && latest.playbackRevision === revision && latest.generation === generation) {
            setPlayback({ ...latest, mediaGrant });
            setAdapterRecoveryGeneration((value) => value + 1);
            setFailure(undefined);
            setError(undefined);
            setStatus('ready');
          }
          return;
        } catch (reason) {
          lastFailure = reason;
          if (attempt === 0) {
            setStatus('recovering');
            await new Promise((resolve) => window.setTimeout(resolve, 1_500));
          }
        }
      }
      const latest = playbackRef.current;
      if (latest?.sessionId !== current.sessionId || latest.playbackRevision !== revision || latest.generation !== generation) return;
      const nextFailure = playbackFailure(lastFailure, 'route');
      if (nextFailure) {
        setFailure(nextFailure);
        setError(nextFailure.message);
        setStatus('failed');
      }
    })().finally(() => {
      if (grantRenewalRef.current?.promise === promise) grantRenewalRef.current = undefined;
    });
    grantRenewalRef.current = { sessionId: current.sessionId, promise };
    return promise;
  }, [setPlayback, source]);

  const retry = useCallback(async () => {
    const current = retryRef.current;
    if (!current) return;
    const targetId = current.kind === 'media' ? current.mediaId : current.kind === 'live' || current.kind === 'library-channel' ? current.channelId : current.recordingId;
    if (playbackRef.current?.media.id === targetId) {
      setStatus('recovering');
      setFailure(undefined);
      setError(undefined);
      try {
        await renewGrant();
      } catch (reason) {
        const nextFailure = playbackFailure(reason, 'route');
        if (nextFailure) {
          setFailure(nextFailure);
          setError(nextFailure.message);
          setStatus('failed');
        }
      }
      return;
    }
    if (current.kind === 'live') await startLive(current.channelId);
    else if (current.kind === 'library-channel') await startLibraryChannel(current.channelId);
    else if (current.kind === 'dvr') await startDVR(current.recordingId);
    else await start(current.mediaId, current.options);
  }, [renewGrant, start, startDVR, startLibraryChannel, startLive]);

  const touch = useCallback(async (event: PlaybackProgressInput, keepalive = false) => {
    const current = playbackRef.current;
    if (!current || completedSessionRef.current === current.sessionId) return;
    try {
      await source.touchPlayback(current.sessionId, event, undefined, keepalive);
    } catch {
      // Playback progress is retried by the next ordered event; the player remains usable.
    }
  }, [source]);

  const prepareNextOnce = useCallback((current: PlaybackResponse, candidateId = '', request: PlaybackPrepareNextRequest = {}, force = false) => {
    const canonicalRequest = { ...request, intent: request.intent ?? deliveryIntent() };
    // The Server grants exactly one prepared continuation for a current
    // session/candidate pair.  UI projections can rebuild an equivalent
    // request while progress and runtime state are being rendered, so request
    // object identity (or serialization) must not create another owner.  A
    // reviewed Retry is the only path allowed to replace this result.
    const key = JSON.stringify({ sessionId: current.sessionId, candidateId });
    return playbackPreparationOwner.prepare(
      key,
      (signal) => source.prepareNextPlayback(current.sessionId, signal, canonicalRequest),
      force,
    );
  }, [deliveryIntent, source]);

  const prepareNext = useCallback((candidate: MediaItem, request: PlaybackPrepareNextRequest = {}, force = false) => {
    const current = playbackRef.current;
    if (!current || current.isLive) return Promise.reject(new Error('No finite playback session is available to prepare.'));
    return prepareNextOnce(current, candidate.id, request, force);
  }, [prepareNextOnce]);

  const terminalize = useCallback((disposition: 'stopped' | 'completed', positionSeconds: number, durationSeconds: number) => {
    const current = playbackRef.current;
    if (!current) return Promise.resolve();
    if (terminalOwnerRef.current?.sessionId === current.sessionId) return terminalOwnerRef.current.promise;
    completedSessionRef.current = current.sessionId;
    if (disposition === 'completed') {
      setCompletedSessionId(current.sessionId);
      setStatus('completed');
    }
    const controller = new AbortController();
    const promise = source.stopPlayback(current.sessionId, {
      disposition,
      positionSeconds,
      durationSeconds,
    }, controller.signal, true).catch(() => {
      // The terminal owner remains fenced after an uncertain result so UI
      // cleanup cannot re-enter with stale progress or a second mutation.
    });
    terminalOwnerRef.current = { sessionId: current.sessionId, promise };
    return promise;
  }, [source]);

  const close = useCallback(async () => {
    operationRef.current += 1;
    controllerRef.current?.abort();
    renegotiationControllerRef.current?.abort();
    playbackPreparationOwner.cancel();
    queueControllerRef.current?.abort();
    if (queueRetryTimerRef.current !== undefined) window.clearTimeout(queueRetryTimerRef.current);
    const current = playbackRef.current;
    const media = mediaRef.current;
    const positionSeconds = Math.max(0, Number.isFinite(media?.currentTime) ? media!.currentTime : 0);
    const durationSeconds = current?.timeline.type === 'live'
      ? 0
      : Math.max(
          1,
          Number.isFinite(media?.duration) && media!.duration > 0
            ? media!.duration
            : current?.timeline.durationSeconds ?? positionSeconds,
        );
    const stop = current
      ? terminalize('stopped', positionSeconds, durationSeconds)
      : Promise.resolve();
    media?.pause();
    if (media) {
      media.removeAttribute('src');
      media.load();
    }
    setPlayback(undefined);
    replaceQueue(undefined);
    setQueueError(undefined);
    queueMutationRef.current = false;
    setQueueBusy(false);
    markQueueNeedsRefresh(false);
    setError(undefined);
    setFailure(undefined);
    setInterruption(undefined);
    setPostPlay({ phase: 'inactive' });
    preparedNextRef.current = undefined;
    setStatus('idle');
    startingMediaRef.current = '';
    void stop.catch(() => {
      // The keepalive stop continues after the player has been dismissed.
    });
  }, [markQueueNeedsRefresh, replaceQueue, setPlayback, terminalize]);

  const complete = useCallback(async (durationSeconds: number) => {
    const current = playbackRef.current;
    if (!current) return;
    await terminalize('completed', durationSeconds, durationSeconds);
  }, [terminalize]);

  useEffect(() => () => {
    queueControllerRef.current?.abort();
    renegotiationControllerRef.current?.abort();
    if (queueRetryTimerRef.current !== undefined) window.clearTimeout(queueRetryTimerRef.current);
  }, []);

  const interrupt = useCallback(async (nextInterruption: PlaybackInterruption) => {
    await close();
    setInterruption(nextInterruption);
  }, [close]);

  const dismissInterruption = useCallback(() => setInterruption(undefined), []);

  const renegotiate = useCallback(async (request: Omit<PlaybackRenegotiationRequest, 'requestId' | 'expectedRevision' | 'clientProfile'>): Promise<PlaybackResponse | undefined> => {
    const current = playbackRef.current;
    if (!current) return;
    renegotiationControllerRef.current?.abort();
    const controller = new AbortController();
    renegotiationControllerRef.current = controller;
    setStatus('recovering');
    setFailure(undefined);
    setError(undefined);
    try {
      if (grantRenewalRef.current?.sessionId === current.sessionId) await grantRenewalRef.current.promise;
      if (controller.signal.aborted || playbackRef.current?.sessionId !== current.sessionId) return;
      const value = await source.renegotiatePlayback(current.sessionId, {
        ...request,
        requestId: `web-${secureRandomUUID()}`,
        expectedRevision: current.playbackRevision,
      }, controller.signal);
      if (controller.signal.aborted || playbackRef.current?.sessionId !== current.sessionId) return;
      setPlayback(value);
      setStatus('ready');
      return value;
    } catch (reason) {
      if (controller.signal.aborted || playbackRef.current?.sessionId !== current.sessionId) return;
      const nextFailure = playbackFailure(reason, 'source');
      if (nextFailure) {
        setFailure(nextFailure);
        setError(nextFailure.message);
        setStatus('failed');
      }
    }
  }, [setPlayback, source]);

  const recoverRoute = useCallback(async () => {
    const current = playbackRef.current;
    if (!current || !runtime) throw new Error('No active playback route is available to recover.');
    await runtime.recoverActiveRoute();
    if (playbackRef.current?.sessionId !== current.sessionId) return;
    await renewGrant();
  }, [renewGrant, runtime]);

  const handoff = useCallback(async (request: PlaybackHandoffRequest): Promise<PlaybackResponse | undefined> => {
    const current = playbackRef.current;
    if (!current) return;
    const operation = operationRef.current + 1;
    operationRef.current = operation;
    const controller = new AbortController();
    controllerRef.current?.abort();
    renegotiationControllerRef.current?.abort();
    controllerRef.current = controller;
    setStatus('preparing');
    setFailure(undefined);
    setError(undefined);
    try {
      const preparedRequestID = request.preparedSessionId
        ? stablePreparedHandoffRequestID(preparedHandoffRequestIDsRef.current, current.sessionId, request.preparedSessionId, request.requestId)
        : request.requestId;
      console.info('[portico-playback-handoff]', JSON.stringify({ phase: 'request', sourceSessionId: current.sessionId, preparedSessionId: request.preparedSessionId ?? '', requestId: preparedRequestID ?? '' }));
      const value = await source.handoffPlayback(current.sessionId, {
        ...request,
        ...(normalizedPlaybackHandoffProgress(request.progressSeconds) !== undefined
          ? { progressSeconds: normalizedPlaybackHandoffProgress(request.progressSeconds) }
          : { progressSeconds: undefined }),
        ...(preparedRequestID ? { requestId: preparedRequestID } : {}),
        intent: request.intent ?? deliveryIntent(),
      }, controller.signal);
      if (operationRef.current === operation && !controller.signal.aborted) {
        console.info('[portico-playback-handoff]', JSON.stringify({ phase: 'accepted', sourceSessionId: current.sessionId, preparedSessionId: request.preparedSessionId ?? '', requestId: preparedRequestID ?? '', nextSessionId: value.sessionId }));
        acceptPlayback(value, 'handoff');
        return value;
      }
    } catch (reason) {
      if (controller.signal.aborted) return;
      console.warn('[portico-playback-handoff]', JSON.stringify({ phase: 'rejected', sourceSessionId: current.sessionId, preparedSessionId: request.preparedSessionId ?? '', failure: reason instanceof Error ? reason.name : 'unknown', status: reason instanceof ApiError ? reason.status : 0, code: reason instanceof ApiError ? reason.code : '' }));
      const nextFailure = playbackFailure(reason);
      if (nextFailure) {
        setFailure(nextFailure);
        setError(nextFailure.message);
        setStatus('failed');
      }
    }
  }, [acceptPlayback, deliveryIntent, source]);

  const next = useCallback(async (automatic = false, preparationRequest: PlaybackPrepareNextRequest = {}) => {
    const current = playbackRef.current;
    if (!current || current.isLive) return false;
    const candidate = nextCandidate(current, queue);
    const cached = preparedNextRef.current;
    const cachedExpiry = cached ? Date.parse(cached.expiresAt) : 0;
    const cachedIsUsable = Boolean(cached && cachedExpiry > Date.now() + 2_000 && (!candidate || cached.playback.media.id === candidate.id));
    const automation = automatic ? reducePlaybackAutomation(
      automationStateRef.current,
      { type: 'automatic-advance-requested', now: Date.now() },
      { passoutProtection: preferences.passoutProtection, passoutAfterEpisodes: preferences.passoutAfterEpisodes },
    ) : undefined;
    if (automation) automationStateRef.current = automation.state;
    if (automation?.effect === 'confirm-still-watching') {
      if (candidate) setPostPlay({ phase: 'passout', next: candidate, preparedSessionId: cached?.preparedSessionId, expiresAt: cached?.expiresAt });
      return false;
    }
    if (!automatic) markMeaningfulInteraction();
    setPostPlay({ phase: 'inactive' });
    if (cachedIsUsable && cached) {
      return Boolean(await handoff({ preparedSessionId: cached.preparedSessionId }));
    }
    setStatus('preparing');
    setFailure(undefined);
    setError(undefined);
    try {
      const prepared = await prepareNextOnce(current, candidate?.id, preparationRequest);
      if (playbackRef.current?.sessionId !== current.sessionId) return false;
      return Boolean(await handoff({ preparedSessionId: prepared.preparedSessionId }));
    } catch (reason) {
      const nextFailure = playbackFailure(reason);
      if (!nextFailure) return false;
      if (candidate) setPostPlay({ phase: 'failed', next: candidate, message: nextFailure.message, preparationRequest });
      else setPostPlay({ phase: 'exhausted' });
      setStatus('ready');
      return false;
    }
  }, [handoff, markMeaningfulInteraction, preferences.passoutAfterEpisodes, preferences.passoutProtection, prepareNextOnce, queue?.items]);

  const beginPostPlay = useCallback(async (candidate?: MediaItem, force = false, preparationRequest: PlaybackPrepareNextRequest = {}) => {
    const current = playbackRef.current;
    if (!current || current.isLive) return;
    if (!candidate) {
      preparedNextRef.current = undefined;
      setPostPlay({ phase: 'exhausted' });
      return;
    }
    setPostPlay({ phase: 'preparing', next: candidate });
    const preparation = prepareNextOnce(current, candidate.id, preparationRequest, force);
    try {
      const prepared = await preparation;
      if (playbackRef.current?.sessionId !== current.sessionId) return;
      preparedNextRef.current = prepared;
      setPostPlay({
        phase: 'countdown',
        next: prepared.playback.media,
        preparedSessionId: prepared.preparedSessionId,
        expiresAt: prepared.expiresAt,
      });
    } catch (reason) {
      if (!playbackPreparationOwner.owns(preparation) || playbackRef.current?.sessionId !== current.sessionId) return;
      const nextFailure = playbackFailure(reason);
      setPostPlay({
        phase: 'failed',
        next: candidate,
        message: nextFailure?.message ?? productMessage('playback.up-next-failed').body ?? '',
        preparationRequest,
      });
    }
  }, [prepareNextOnce]);

  const cancelPostPlay = useCallback(() => {
    playbackPreparationOwner.cancel();
    setPostPlay((current) => {
      if (current.phase === 'countdown' || current.phase === 'passout') return {
        phase: 'cancelled',
        next: current.next,
        preparedSessionId: current.preparedSessionId,
        expiresAt: current.expiresAt,
      };
      if (current.phase === 'preparing' || current.phase === 'failed') return { phase: 'cancelled', next: current.next };
      return current;
    });
  }, []);

  const replay = useCallback(async () => {
    const current = playbackRef.current;
    if (!current || current.isLive) return;
    const operation = operationRef.current + 1;
    operationRef.current = operation;
    controllerRef.current?.abort();
    renegotiationControllerRef.current?.abort();
    playbackPreparationOwner.cancel();
    const controller = new AbortController();
    controllerRef.current = controller;
    preparedNextRef.current = undefined;
    setPostPlay({ phase: 'inactive' });
    setStatus('preparing');
    setFailure(undefined);
    setError(undefined);
    const queueMediaIds = (queue?.items ?? current.queue).map((item) => item.id);
    const request: PlaybackHandoffRequest = {
      mediaId: current.media.id,
      progressSeconds: 0,
      queueMediaIds,
      sourceContext: queue?.sourceContext ?? current.sourceContext,
    };
    try {
      markMeaningfulInteraction();
      const value = await source.handoffPlayback(current.sessionId, { ...request, intent: deliveryIntent() }, controller.signal);
      if (operationRef.current === operation && !controller.signal.aborted) acceptPlayback(value, 'handoff');
      return;
    } catch (handoffReason) {
      if (controller.signal.aborted || operationRef.current !== operation) return;
      try {
        const positionSeconds = 0;
        await source.stopPlayback(current.sessionId, {
          disposition: 'stopped',
          positionSeconds,
          durationSeconds: Math.max(1, current.timeline.durationSeconds ?? positionSeconds),
        }, controller.signal);
      } catch { /* an ended session may already be closed */ }
      setPlayback(undefined);
      replaceQueue(undefined);
      try {
        const value = await source.startPlayback(current.media.id, { startSeconds: 0, queueMediaIds, repeatMode: queue?.repeatMode ?? current.repeatMode, sourceContext: queue?.sourceContext ?? current.sourceContext, intent: deliveryIntent() }, controller.signal);
        if (operationRef.current === operation && !controller.signal.aborted) acceptPlayback(value, 'handoff');
      } catch (reason) {
        const nextFailure = playbackFailure(reason ?? handoffReason);
        if (nextFailure) {
          setFailure(nextFailure);
          setError(nextFailure.message);
          setStatus('failed');
        }
      }
    }
  }, [acceptPlayback, deliveryIntent, markMeaningfulInteraction, queue, replaceQueue, setPlayback, source]);

  const previous = useCallback(async () => {
    const previousItem = queue?.history.at(-1);
    if (previousItem) await handoff({ mediaId: previousItem.id, progressSeconds: 0 });
  }, [handoff, queue]);

  const applyExternalCommand = useCallback(async (command: PlaybackCommand) => {
    if (!command.action) return;
    if (command.action === 'stop') {
      pendingExternalCommandRef.current = undefined;
      const copy = productMessage('playback.remote-stopped');
      await interrupt({
        title: copy.title ?? '',
        message: command.message?.trim() || copy.body || '',
      });
      return;
    }
    if (command.action === 'next') {
      await next();
      return;
    }
    if (command.action === 'previous') {
      await previous();
      return;
    }

    const mediaId = command.mediaId?.trim();
    const current = playbackRef.current;
    if (mediaId && current?.media.id !== mediaId) {
      pendingExternalCommandRef.current = command;
      await start(mediaId, { startSeconds: command.positionSeconds });
      return;
    }

    const media = mediaRef.current;
    if (!current || !media) {
      if (mediaId) pendingExternalCommandRef.current = command;
      return;
    }

    if (typeof command.positionSeconds === 'number') {
      const moving = command.action === 'play' || command.action === 'load';
      const target = watchWithFriendsTargetPosition(command.positionSeconds, command.issuedAt, moving);
      const duration = Number.isFinite(media.duration) ? media.duration : undefined;
      media.currentTime = Math.max(0, duration === undefined ? target : Math.min(target, duration));
    }
    if (command.action === 'pause') media.pause();
    if (command.action === 'play' || command.action === 'load') {
      await media.play().catch(() => undefined);
    }
  }, [interrupt, next, previous, start]);

  useEffect(() => {
    const command = pendingExternalCommandRef.current;
    if (!command || !playback || !mediaRef.current) return;
    if (command.mediaId && command.mediaId !== playback.media.id) return;
    pendingExternalCommandRef.current = undefined;
    void applyExternalCommand(command);
  }, [applyExternalCommand, playback, status]);

  useEffect(() => {
    if (!playback) return;
    const client = playbackCommandClientFrom(source);
    if (!client) return;
    const close = subscribeToPlaybackCommands(client, playback.sessionId, (command) => {
      void applyExternalCommand(command);
    });
    const unregister = auth.registerRuntimeTeardown('realtime', close);
    return () => {
      unregister();
      close();
    };
  }, [applyExternalCommand, auth.registerRuntimeTeardown, playback?.sessionId, source]);

  const runQueueMutation = useCallback(async (
    mutation: (state: PlaybackSessionQueueResponse, signal: AbortSignal) => Promise<PlaybackSessionQueueResponse>,
  ) => {
    const current = playbackRef.current;
    const currentQueue = queueRef.current;
    if (!current || !currentQueue || currentQueue.canMutate === false || queueMutationRef.current || queueNeedsRefreshRef.current) {
      const message = productMessage('playback.queue-update-failed').body;
      setQueueError(message);
      throw new Error(message);
    }
    queueControllerRef.current?.abort();
    const controller = new AbortController();
    queueControllerRef.current = controller;
    queueMutationRef.current = true;
    setQueueBusy(true);
    setQueueError(undefined);
    try {
      const value = await mutation(currentQueue, controller.signal);
      if (playbackRef.current?.sessionId === current.sessionId) {
        replaceQueue(value);
        markQueueNeedsRefresh(false);
      }
    } catch (reason) {
      if (playbackRef.current?.sessionId !== current.sessionId) return;
      if (!isQueueRevisionConflict(reason)) {
        setQueueError(productMessage('playback.queue-update-failed').body);
        throw reason;
      }
      try {
        const latest = await source.playbackSessionQueue(current.sessionId, controller.signal);
        if (playbackRef.current?.sessionId !== current.sessionId) return;
        replaceQueue(latest);
        markQueueNeedsRefresh(false);
        setQueueError(productMessage('playback.queue-changed').body);
      } catch {
        if (playbackRef.current?.sessionId !== current.sessionId) return;
        markQueueNeedsRefresh(true);
        setQueueError(productMessage('playback.queue-changed-reload').body);
      }
      throw reason;
    } finally {
      if (queueControllerRef.current === controller) queueControllerRef.current = undefined;
      queueMutationRef.current = false;
      setQueueBusy(false);
    }
  }, [markQueueNeedsRefresh, replaceQueue, source]);

  const mutateQueue = useCallback(async (request: QueueMutationInput) => {
    await runQueueMutation((state, signal) => source.mutatePlaybackSessionQueue(
      state.sessionId,
      { ...request, expectedRevision: state.revision },
      signal,
    ));
  }, [runQueueMutation, source]);

  const appendQueue = useCallback(async (mediaIds: string[]) => {
    const normalized = [...new Set(mediaIds.map((id) => id.trim()).filter(Boolean))];
    if (!normalized.length) return;
    await mutateQueue({ action: 'append', mediaIds: normalized });
  }, [mutateQueue]);

  const playNext = useCallback(async (mediaIds: string[]) => {
    const normalized = [...new Set(mediaIds.map((id) => id.trim()).filter(Boolean))];
    if (!normalized.length) return;
    await mutateQueue({ action: 'play_next', mediaIds: normalized });
  }, [mutateQueue]);

  const removeQueueItem = useCallback(async (index: number) => {
    await mutateQueue({ action: 'remove', index });
  }, [mutateQueue]);

  const moveQueueItem = useCallback(async (fromIndex: number, toIndex: number) => {
    if (fromIndex === toIndex) return;
    await mutateQueue({ action: 'reorder', fromIndex, toIndex });
  }, [mutateQueue]);

  const shuffleQueue = useCallback(async () => {
    if ((queueRef.current?.items.length ?? 0) < 2) return;
    await runQueueMutation((state, signal) => source.updatePlaybackSessionQueue(
      state.sessionId,
      {
        expectedRevision: state.revision,
        mediaIds: shuffled(state.items).map((item) => item.id),
        repeatMode: state.repeatMode,
      },
      signal,
    ));
  }, [runQueueMutation, source]);

  const setRepeatMode = useCallback(async (mode: PlaybackRepeatMode) => {
    if (queueRef.current?.repeatMode === mode) return;
    await mutateQueue({ action: 'set_repeat', repeatMode: mode });
  }, [mutateQueue]);

  const reloadQueue = useCallback(async () => {
    const current = playbackRef.current;
    if (!current || queueMutationRef.current) return;
    queueControllerRef.current?.abort();
    const controller = new AbortController();
    queueControllerRef.current = controller;
    queueMutationRef.current = true;
    setQueueBusy(true);
    setQueueError(undefined);
    try {
      const latest = await source.playbackSessionQueue(current.sessionId, controller.signal);
      if (playbackRef.current?.sessionId !== current.sessionId) return;
      replaceQueue(latest);
      markQueueNeedsRefresh(false);
    } catch {
      if (playbackRef.current?.sessionId !== current.sessionId) return;
      markQueueNeedsRefresh(true);
      setQueueError(productMessage('playback.queue-refresh-failed').body);
    } finally {
      if (queueControllerRef.current === controller) queueControllerRef.current = undefined;
      queueMutationRef.current = false;
      setQueueBusy(false);
    }
  }, [markQueueNeedsRefresh, replaceQueue, source]);

  useEffect(() => {
    // The datasource is deliberately mounted while the fresh /api/auth/me
    // request is still establishing the canonical viewer scope. Never start a
    // restore through the scoped proxy before that security boundary is ready.
    if (auth.status !== 'ready' || !auth.viewer?.authenticated || !auth.viewerScopeKey
      || restoreAttemptedRef.current || playbackRef.current || location.pathname.startsWith('/watch/')) return;
    restoreAttemptedRef.current = true;
    const operation = operationRef.current + 1;
    operationRef.current = operation;
    const controller = new AbortController();
    source.restorePlayback(controller.signal, deliveryIntent()).then((response) => {
      if (controller.signal.aborted || operationRef.current !== operation) return;
      if (response.active && response.playback) acceptPlayback(response.playback, 'restore');
    }).catch(() => undefined);
    return () => controller.abort();
  }, [acceptPlayback, auth.status, auth.viewer?.authenticated, auth.viewerScopeKey, deliveryIntent, location.pathname, source]);

  useEffect(() => {
    if (!playback) return;
    const expiresAt = Date.parse(playback.mediaGrant.expiresAt);
    if (!Number.isFinite(expiresAt)) return;
    const delay = Math.min(2_147_000_000, Math.max(5_000, expiresAt - Date.now() - 60_000));
    const timer = window.setTimeout(() => void renewGrant(), delay);
    return () => window.clearTimeout(timer);
  }, [playback, renewGrant]);

  const markReady = useCallback(() => {
    setFailure(undefined);
    setError(undefined);
    setStatus('ready');
  }, []);
  const markBuffering = useCallback(() => setStatus('buffering'), []);
  const markRecovering = useCallback(() => setStatus('recovering'), []);
  const fail = useCallback((kind: PlaybackFailureKind = 'unknown') => {
    const nextFailure = playbackFailure(undefined, kind);
    if (nextFailure) {
      setFailure(nextFailure);
      setError(nextFailure.message);
    }
    setStatus('failed');
  }, []);

  const repeatMode = queue?.repeatMode ?? playback?.repeatMode ?? 'off';

  const value = useMemo<PlaybackContextValue>(() => ({
    status, playback, queue, queueError, queueBusy, queueNeedsRefresh, repeatMode, sessionOrigin, error, failure, interruption, postPlay, mediaRef, start, startLive, startLibraryChannel, startDVR, retry, close, complete, touch, renewGrant, renegotiate, recoverRoute, adapterRecoveryGeneration, completedSessionId, next, handoff, previous,
    prepareNext, appendQueue, playNext, removeQueueItem, moveQueueItem, shuffleQueue, setRepeatMode, reloadQueue, beginPostPlay, cancelPostPlay, replay, markReady, markBuffering, markRecovering, fail, interrupt, dismissInterruption, applyExternalCommand, markMeaningfulInteraction,
  }), [adapterRecoveryGeneration, appendQueue, applyExternalCommand, beginPostPlay, cancelPostPlay, close, complete, completedSessionId, dismissInterruption, error, fail, failure, handoff, interruption, interrupt, markBuffering, markMeaningfulInteraction, markReady, markRecovering, moveQueueItem, next, playback, playNext, postPlay, prepareNext, previous, queue, queueBusy, queueError, queueNeedsRefresh, recoverRoute, renegotiate, reloadQueue, removeQueueItem, renewGrant, repeatMode, replay, retry, sessionOrigin, setRepeatMode, shuffleQueue, start, startDVR, startLibraryChannel, startLive, status, touch]);

  return <PlaybackContext.Provider value={value}>{children}</PlaybackContext.Provider>;
}

function usePlayback() {
  const value = useContext(PlaybackContext);
  if (!value) throw new Error('Playback controls must be used inside PlaybackSessionProvider.');
  return value;
}

export function usePlaybackSession() {
  return usePlayback();
}

export function useOptionalPlaybackSession() {
  return useContext(PlaybackContext);
}

function timeLabel(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
  const rounded = Math.floor(seconds);
  const hours = Math.floor(rounded / 3600);
  const minutes = Math.floor((rounded % 3600) / 60);
  const remainder = rounded % 60;
  return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}` : `${minutes}:${String(remainder).padStart(2, '0')}`;
}

function sleepTimerMode(value: string): SleepTimerMode {
  if (value === 'end' || value === 'off') return value;
  const minutes = Number(value);
  return minutes === 15 || minutes === 30 || minutes === 45 || minutes === 60 ? minutes : 'off';
}

export function isExplicitMissingHLSManifest(responseStatus: number, details: unknown, playbackStarted: boolean) {
  if (playbackStarted || (responseStatus !== 404 && responseStatus !== 410)) return false;
  return String(details).toLocaleLowerCase().includes('manifest');
}

export function isTransientHLSManifestWait(responseStatus: number, details: unknown, playbackStarted: boolean) {
  if (playbackStarted || ![409, 425, 503].includes(responseStatus)) return false;
  return String(details).toLocaleLowerCase().includes('manifest');
}

export function shouldUseNativeHLS(nativeSupport: string, userAgent: string) {
  if (!nativeSupport) return false;
  return /safari/i.test(userAgent) && !/(chrome|chromium|crios|fxios|edg)/i.test(userAgent);
}

const NO_SUBTITLE_ID = 'sub_none';
const NATIVE_SUBTITLE_PREFIX = 'native_subtitle_';

type PlayerSubtitleOption = {
  id: string;
  label: string;
  language?: string;
  origin: 'contract' | 'native';
};

type PlayerSubtitleContextValue = {
  selectedId: string;
  setSelectedId: (id: string) => void;
  options: PlayerSubtitleOption[];
  activeCues: string[];
};

const PlayerSubtitleContext = createContext<PlayerSubtitleContextValue | null>(null);

function usePlayerSubtitles() {
  const value = useContext(PlayerSubtitleContext);
  if (!value) throw new Error('Player subtitles must be used inside PlayerSubtitleProvider.');
  return value;
}

export function playableSubtitleStreams(streams: PlaybackResponse['subtitleStreams']) {
  return streams.filter((stream) => {
    const title = (stream.displayTitle || '').trim().toLocaleLowerCase();
    return Boolean(stream.id) && stream.id !== NO_SUBTITLE_ID && title !== 'none' && title !== 'off';
  });
}

function subtitleLabel(stream: PlaybackResponse['subtitleStreams'][number], index: number) {
  return stream.displayTitle?.trim() || stream.language?.trim() || stream.codec?.trim() || `Subtitle ${index + 1}`;
}

function textTracksFrom(list: TextTrackList) {
  const tracks: TextTrack[] = [];
  for (let index = 0; index < list.length; index += 1) {
    const track = list[index];
    if (track && (track.kind === 'subtitles' || track.kind === 'captions')) tracks.push(track);
  }
  return tracks;
}

function cueText(cue: TextTrackCue) {
  const fragment = (cue as TextTrackCue & { getCueAsHTML?: () => DocumentFragment }).getCueAsHTML?.();
  if (fragment?.textContent?.trim()) return fragment.textContent.trim();
  const value = (cue as TextTrackCue & { text?: string }).text;
  return typeof value === 'string' ? value.trim() : '';
}

function sameSubtitle(left: PlayerSubtitleOption, right: PlayerSubtitleOption) {
  const leftLanguage = left.language?.trim().toLocaleLowerCase();
  const rightLanguage = right.language?.trim().toLocaleLowerCase();
  if (leftLanguage && rightLanguage && leftLanguage === rightLanguage) return true;
  return left.label.trim().toLocaleLowerCase() === right.label.trim().toLocaleLowerCase();
}

function PlayerSubtitleProvider({ playback, mediaRef, children }: { playback: PlaybackResponse; mediaRef: RefObject<HTMLVideoElement | null>; children: ReactNode }) {
  const contractOptions = useMemo<PlayerSubtitleOption[]>(() => playableSubtitleStreams(playback.subtitleStreams).map((stream, index) => ({
    id: stream.id,
    label: subtitleLabel(stream, index),
    language: stream.language,
    origin: 'contract',
  })), [playback.subtitleStreams]);
  const [selectedId, setSelectedId] = useState(playback.selectedSubtitleStreamId || NO_SUBTITLE_ID);
  const [nativeOptions, setNativeOptions] = useState<PlayerSubtitleOption[]>([]);
  const [activeCues, setActiveCues] = useState<string[]>([]);

  useEffect(() => {
    setSelectedId(playback.selectedSubtitleStreamId || NO_SUBTITLE_ID);
    setNativeOptions([]);
    setActiveCues([]);
  }, [playback.selectedSubtitleStreamId, playback.sessionId]);

  useEffect(() => {
    const media = mediaRef.current;
    if (!media) return;
    let selectedTrack: TextTrack | undefined;

    const updateCues = () => {
      if (!selectedTrack?.activeCues) {
        setActiveCues([]);
        return;
      }
      const values: string[] = [];
      for (let index = 0; index < selectedTrack.activeCues.length; index += 1) {
        const cue = selectedTrack.activeCues[index];
        if (!cue) continue;
        const text = cueText(cue);
        if (text) values.push(text);
      }
      setActiveCues(values);
    };

    const sync = () => {
      selectedTrack?.removeEventListener('cuechange', updateCues);
      const tracks = textTracksFrom(media.textTracks);
      const discovered = tracks.map<PlayerSubtitleOption>((track, index) => ({
        id: `${NATIVE_SUBTITLE_PREFIX}${index}`,
        label: track.label?.trim() || track.language?.trim() || (track.kind === 'captions'
          ? productMessage('playback.closed-captions').text ?? ''
          : productMessage('playback.subtitle-number', { number: index + 1 }).text ?? ''),
        language: track.language || undefined,
        origin: 'native',
      }));
      setNativeOptions(discovered);

      const nativeIndex = selectedId.startsWith(NATIVE_SUBTITLE_PREFIX) ? Number(selectedId.slice(NATIVE_SUBTITLE_PREFIX.length)) : -1;
      const selectedOption = contractOptions.find((option) => option.id === selectedId);
      const contractIndex = selectedOption ? discovered.findIndex((option) => sameSubtitle(selectedOption, option)) : -1;
      const targetIndex = Number.isInteger(nativeIndex) && nativeIndex >= 0 ? nativeIndex : contractIndex;
      selectedTrack = targetIndex >= 0 ? tracks[targetIndex] : undefined;
      for (const track of tracks) track.mode = track === selectedTrack ? 'hidden' : 'disabled';
      if (selectedTrack) selectedTrack.addEventListener('cuechange', updateCues);
      updateCues();
    };

    sync();
    const list = media.textTracks;
    list.addEventListener?.('addtrack', sync);
    list.addEventListener?.('removetrack', sync);
    return () => {
      selectedTrack?.removeEventListener('cuechange', updateCues);
      list.removeEventListener?.('addtrack', sync);
      list.removeEventListener?.('removetrack', sync);
      for (const track of textTracksFrom(list)) track.mode = 'disabled';
    };
  }, [contractOptions, mediaRef, playback.sessionId, selectedId]);

  const options = useMemo(() => [
    ...contractOptions,
    ...nativeOptions.filter((candidate) => !contractOptions.some((option) => sameSubtitle(candidate, option))),
  ], [contractOptions, nativeOptions]);

  useEffect(() => {
    if (selectedId !== NO_SUBTITLE_ID && !options.some((option) => option.id === selectedId)) setSelectedId(NO_SUBTITLE_ID);
  }, [options, selectedId]);

  const value = useMemo<PlayerSubtitleContextValue>(() => ({ selectedId, setSelectedId, options, activeCues }), [activeCues, options, selectedId]);
  return <PlayerSubtitleContext.Provider value={value}>{children}</PlayerSubtitleContext.Provider>;
}

function PlayerSubtitleLayer() {
  const { activeCues } = usePlayerSubtitles();
  if (!activeCues.length) return null;
  return <div className="player-subtitle-layer" aria-hidden="true">{activeCues.map((cue, index) => <span key={`${cue}-${index}`}>{cue}</span>)}</div>;
}

function PendingPlayerControls({ full, onClose }: { full: boolean; onClose: () => void }) {
  const disabled = true;
  return <div className={`player-controls player-pending-controls ${full ? 'full' : 'mini'}`}>
    <div className="player-timeline" aria-hidden="true"><span>0:00</span><span className="player-timeline-track"><input type="range" min={0} max={1} value={0} disabled readOnly tabIndex={-1} /></span><span>0:00</span></div>
    <div className="player-command-row">
      <div className="player-transport" aria-label={productMessage('playback.transport-controls').text}>
        <button type="button" disabled={disabled} aria-label={productMessage('action.previous-item').text}><ProductLanguageIcon id="action.previous" /></button>
        <button type="button" className="player-skip-button" disabled={disabled} aria-label={productMessage('action.rewind-seconds', { seconds: 10 }).text}><ProductLanguageIcon id="action.replay" /><span>10</span></button>
        <button type="button" className="play-toggle" disabled={disabled} aria-label={productMessage('action.play').text}><ProductLanguageIcon id="action.play" filled /></button>
        <button type="button" className="player-skip-button" disabled={disabled} aria-label={productMessage('action.forward-seconds', { seconds: 30 }).text}><ProductLanguageIcon id="action.refresh" /><span>30</span></button>
        <button type="button" disabled={disabled} aria-label={productMessage('action.next-item').text}><ProductLanguageIcon id="action.next" /></button>
      </div>
      <div className="player-utilities" aria-label={productMessage('playback.utilities').text}>
        <button type="button" disabled={disabled} aria-label={productMessage('playback.menu-volume').text}><ProductLanguageIcon id="action.volume" /></button>
        <button type="button" disabled={disabled} aria-label={productMessage('playback.menu-subtitles').text}><ProductLanguageIcon id="action.subtitles" /></button>
        <button type="button" disabled={disabled} aria-label={productMessage('playback.menu-settings').text}><ProductLanguageIcon id="action.settings" /></button>
        <button type="button" disabled={disabled} aria-label={productMessage('playback.menu-queue').text}><ProductLanguageIcon id="action.player-queue" /></button>
        <button type="button" disabled={disabled} aria-label={productMessage('action.fullscreen').text}><ProductLanguageIcon id="action.fullscreen" /></button>
        <button type="button" onClick={onClose} aria-label={productMessage('action.close-player').text}><ProductLanguageIcon id="action.close" /></button>
      </div>
    </div>
  </div>;
}

function PendingPlayerShell({ full, onClose }: { full: boolean; onClose: () => void }) {
  const { status, error, failure, retry } = usePlayback();
  const failed = status === 'failed';
  const copy = productMessage(failed ? 'playback.failed' : 'playback.preparing');
  const title = failed ? failure?.title ?? copy.title : copy.title;
  const message = failed ? failure?.message ?? error ?? copy.body : copy.body;
  return <section className={`${full ? 'player-full' : 'player-mini'} player-pending-shell`} aria-label={title}>
    <div className="player-media-stage player-pending-stage" aria-hidden="true"><ProductLanguageIcon id={copy.icon ?? (failed ? 'status.warning' : 'status.loading')} className={!failed ? 'state-spinner' : undefined} /></div>
    <div className={`player-pending-copy ${failed ? 'failed' : ''}`} role={failed ? 'alert' : 'status'} aria-live={failed ? 'assertive' : 'polite'} aria-busy={!failed}>
      <span><strong>{title}</strong>{message && <small>{message}</small>}</span>
      <div>{failed && <SecondaryButton onClick={() => void retry()}><ProductLanguageIcon id="action.retry" /> {productMessage('action.retry').text}</SecondaryButton>}<SecondaryButton onClick={onClose}><ProductLanguageIcon id="action.close" /> {productMessage('action.close-player').text}</SecondaryButton></div>
    </div>
    <PendingPlayerControls full={full} onClose={onClose} />
  </section>;
}

export function WatchPage() {
  const { id } = useParams();
  const { playback, start } = usePlayback();
  const auth = useAuthSession();
  const location = useLocation();
  const navigate = useNavigate();
  const routeMediaRef = useRef('');
  useEffect(() => {
    // initialViewer is only a rendering hint. Wait for the final fresh
    // /api/auth/me scope before opening a playback session.
    if (!id || auth.status !== 'ready' || !auth.viewer?.authenticated || !auth.viewerScopeKey) return;
    if (routeMediaRef.current === id && playback && playback.media.id !== id) {
      routeMediaRef.current = playback.media.id;
      navigate(`/watch/${playback.media.id}`, { replace: true });
      return;
    }
    if (routeMediaRef.current !== id) {
      routeMediaRef.current = id;
      if (playback?.media.id !== id) void start(id, playbackOptionsFromNavigationState(location.state));
    }
  }, [auth.status, auth.viewer?.authenticated, auth.viewerScopeKey, id, location.state, navigate, playback, start]);
  return null;
}

function PlayerMenu({
  label,
  icon,
  children,
  disabled = false,
  panelClassName = '',
  wide = false,
}: {
  label: string;
  icon: ReactNode;
  children: ReactNode | ((dismiss: () => void) => ReactNode);
  disabled?: boolean;
  panelClassName?: string;
  wide?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelId = useId();
  const dismiss = useCallback(() => setOpen(false), [setOpen]);
  const dismissAndReturnFocus = useCallback(() => {
    setOpen(false);
    window.requestAnimationFrame(() => triggerRef.current?.focus());
  }, [setOpen]);

  useEffect(() => {
    if (disabled) setOpen(false);
  }, [disabled]);

  return <div className="player-menu">
    <button
      ref={triggerRef}
      type="button"
      className={`player-menu-trigger ${open ? 'selected' : ''}`}
      aria-label={label}
      aria-expanded={open}
      aria-controls={open ? panelId : undefined}
      aria-haspopup="dialog"
      title={label}
      disabled={disabled}
      onClick={() => setOpen(!open)}
    >{icon}</button>
    {open && <AnchoredOverlay anchorRef={triggerRef} placement="top-end" className={`player-menu-panel ${wide ? 'queue-menu-panel' : ''} ${panelClassName}`.trim()} onDismiss={dismiss} role="dialog" ariaLabel={label}>
      <section id={panelId} className="player-menu-content">{typeof children === 'function' ? children(dismissAndReturnFocus) : children}</section>
    </AnchoredOverlay>}
  </div>;
}

type PlayerMenuCoordinatorValue = {
  activeId?: string;
  setActiveId: (id?: string) => void;
};

function PlayerMenuGroup({ children }: { children: ReactNode }) {
  return <>{children}</>;
}

function QueueArtwork({ item }: { item: MediaItem }) {
  const source = usePorticoDataSource();
  const artwork = item.displayImages?.poster || item.displayImages?.thumb || item.displayImages?.backdrop || item.images.poster || item.images.thumb || item.images.backdrop;
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [artwork]);
  return artwork && !failed
    ? <img className="queue-artwork" src={source.playbackResourceUrl(artwork)} alt="" onError={() => setFailed(true)} />
    : <span className="queue-artwork queue-artwork-fallback"><ProductLanguageIcon id="status.music" /></span>;
}

function ChapterThumbnail({ src }: { src?: string }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [src]);
  if (!src || failed) return null;
  return <img src={src} alt="" onError={() => setFailed(true)} />;
}

function PlayerArtworkImage({ src, className, fallback }: { src?: string; className?: string; fallback?: ReactNode }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [src]);
  if (!src || failed) return fallback ?? null;
  return <img className={className} src={src} alt="" onError={() => setFailed(true)} />;
}

function QueuePanel({ onDismiss }: { onDismiss: () => void }) {
  const {
    playback,
    queue,
    queueError,
    queueBusy,
    queueNeedsRefresh,
    repeatMode,
    removeQueueItem,
    moveQueueItem,
    shuffleQueue,
    setRepeatMode,
    reloadQueue,
  } = usePlayback();
  const [dragSourceIndex, setDragSourceIndex] = useState<number>();
  const [dragTargetIndex, setDragTargetIndex] = useState<number>();
  const pointerDragRef = useRef<{ sourceIndex: number; targetIndex: number } | undefined>(undefined);
  const current = queue?.current ?? playback?.media;
  const entries = (queue?.items ?? playback?.queue ?? [])
    .map((item, sourceIndex) => ({ item, sourceIndex }))
    .filter(({ item }) => item.id !== current?.id);
  const canMutate = queue?.canMutate === true && !queueBusy && !queueNeedsRefresh;
  const nextRepeat: PlaybackRepeatMode = repeatMode === 'off' ? 'all' : repeatMode === 'all' ? 'one' : 'off';
  const repeatLabel = productMessage(repeatMode === 'all' ? 'action.repeat-all' : repeatMode === 'one' ? 'action.repeat-one' : 'action.repeat-off').text;
  const queueCount = entries.length + (current ? 1 : 0);
  const beginDrag = (event: ReactDragEvent<HTMLLIElement>, sourceIndex: number) => {
    if (!canMutate) return;
    setDragSourceIndex(sourceIndex);
    setDragTargetIndex(sourceIndex);
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', String(sourceIndex));
  };
  const acceptDrag = (event: ReactDragEvent<HTMLLIElement>, sourceIndex: number) => {
    if (!canMutate || dragSourceIndex === undefined) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
    setDragTargetIndex(sourceIndex);
  };
  const finishDrag = () => {
    setDragSourceIndex(undefined);
    setDragTargetIndex(undefined);
  };
	const beginPointerDrag = (event: ReactPointerEvent<HTMLSpanElement>, sourceIndex: number) => {
		if (!canMutate || event.pointerType === 'mouse') return;
		event.preventDefault();
		event.currentTarget.setPointerCapture(event.pointerId);
		pointerDragRef.current = { sourceIndex, targetIndex: sourceIndex };
		setDragSourceIndex(sourceIndex);
		setDragTargetIndex(sourceIndex);
	};
	const movePointerDrag = (event: ReactPointerEvent<HTMLSpanElement>) => {
		const drag = pointerDragRef.current;
		if (!drag) return;
		event.preventDefault();
		const row = document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>('[data-queue-source-index]');
		const targetIndex = Number(row?.dataset.queueSourceIndex);
		if (!Number.isInteger(targetIndex) || targetIndex === drag.targetIndex) return;
		drag.targetIndex = targetIndex;
		setDragTargetIndex(targetIndex);
	};
	const endPointerDrag = (event: ReactPointerEvent<HTMLSpanElement>, commit: boolean) => {
		const drag = pointerDragRef.current;
		if (!drag) return;
		if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
		pointerDragRef.current = undefined;
		finishDrag();
		if (commit && canMutate && drag.sourceIndex !== drag.targetIndex) {
			void moveQueueItem(drag.sourceIndex, drag.targetIndex).catch(() => undefined);
		}
	};
  const drop = (event: ReactDragEvent<HTMLLIElement>, targetIndex: number) => {
    event.preventDefault();
    const sourceIndex = dragSourceIndex ?? Number(event.dataTransfer.getData('text/plain'));
    finishDrag();
    if (!canMutate || !Number.isInteger(sourceIndex) || sourceIndex === targetIndex) return;
    void moveQueueItem(sourceIndex, targetIndex).catch(() => undefined);
  };
  return <div className="player-queue-panel" aria-busy={queueBusy}>
    <div className="player-menu-heading"><span><strong>{productMessage('playback.queue-title').text}</strong><small>{productMessage(queueCount === 1 ? 'playback.queue-count-one' : 'playback.queue-count-many', { count: queueCount }).text}</small></span><button type="button" className="player-panel-close" aria-label={productMessage('action.close-queue').text} onClick={onDismiss}><ProductLanguageIcon id="action.close" /></button></div>
    <div className="queue-actions"><button type="button" className={repeatMode !== 'off' ? 'selected' : ''} aria-pressed={repeatMode !== 'off'} onClick={() => void setRepeatMode(nextRepeat).catch(() => undefined)} disabled={!canMutate}><ProductLanguageIcon id={repeatMode === 'one' ? 'action.repeat-one' : 'action.repeat'} /> {repeatLabel}</button><button type="button" onClick={() => void shuffleQueue().catch(() => undefined)} disabled={!canMutate || entries.length < 2}><ProductLanguageIcon id="action.shuffle" /> {productMessage('action.shuffle').text}</button></div>
    <ol className="queue-list">
      {current && <li className="queue-current" aria-current="true"><QueueArtwork item={current} /><span><strong>{current.title}</strong><small>{[productMessage('playback.queue-now-playing').text, current.parentTitle || current.tagline].filter(Boolean).join(' · ')}</small></span></li>}
      {entries.map(({ item, sourceIndex }, index) => <li key={`${item.id}-${sourceIndex}`} data-queue-source-index={sourceIndex} draggable={canMutate} className={`${dragSourceIndex === sourceIndex ? 'dragging' : ''} ${dragTargetIndex === sourceIndex && dragSourceIndex !== sourceIndex ? 'drag-target' : ''}`.trim()} onDragStart={(event) => beginDrag(event, sourceIndex)} onDragOver={(event) => acceptDrag(event, sourceIndex)} onDrop={(event) => drop(event, sourceIndex)} onDragEnd={finishDrag}>
      <span className="queue-drag-handle" aria-hidden="true" title="Drag to reorder" onPointerDown={(event) => beginPointerDrag(event, sourceIndex)} onPointerMove={movePointerDrag} onPointerUp={(event) => endPointerDrag(event, true)} onPointerCancel={(event) => endPointerDrag(event, false)}><ActionCustomizeIcon /></span><QueueArtwork item={item} /><span><strong>{item.title}</strong><small>{item.parentTitle || item.tagline || mediaPresentation({ entityKind: item.entityKind }).label}</small></span>
      <div><button type="button" aria-label={productMessage('action.move-queue-earlier', { title: item.title }).text} title={productMessage('action.move-queue-earlier', { title: item.title }).text} disabled={!canMutate || index === 0} onClick={() => void moveQueueItem(sourceIndex, entries[index - 1]?.sourceIndex ?? sourceIndex).catch(() => undefined)}><ProductLanguageIcon id="action.move-up" /></button><button type="button" aria-label={productMessage('action.move-queue-later', { title: item.title }).text} title={productMessage('action.move-queue-later', { title: item.title }).text} disabled={!canMutate || index === entries.length - 1} onClick={() => void moveQueueItem(sourceIndex, entries[index + 1]?.sourceIndex ?? sourceIndex).catch(() => undefined)}><ProductLanguageIcon id="action.move-down" /></button><button type="button" aria-label={productMessage('action.remove-from-queue', { title: item.title }).text} title={productMessage('action.remove-from-queue', { title: item.title }).text} disabled={!canMutate} onClick={() => void removeQueueItem(sourceIndex).catch(() => undefined)}><ProductLanguageIcon id="action.remove-queue" /></button></div>
    </li>)}</ol>
    {!entries.length && <p className="player-empty-copy">{productMessage('playback.queue-empty').body}</p>}
    {queueError && <div className="player-menu-error" role="alert"><span>{queueError}</span>{queueNeedsRefresh && <button type="button" onClick={() => void reloadQueue()} disabled={queueBusy}><ProductLanguageIcon id="action.retry" /> {productMessage('action.reload-queue').text}</button>}</div>}
  </div>;
}

function PlayerStatusLayer() {
  const { status, failure, playback, postPlay, retry } = usePlayback();
  const [reporting, setReporting] = useState(false);
  const [failureDismissed, setFailureDismissed] = useState(false);
  useEffect(() => {
    if (status !== 'failed') {
      setFailureDismissed(false);
      return;
    }
    setFailureDismissed(false);
    const timer = window.setTimeout(() => setFailureDismissed(true), 8_000);
    return () => window.clearTimeout(timer);
  }, [failure?.kind, failure?.message, status]);
  if (postPlay.phase !== 'inactive' || !['preparing', 'buffering', 'recovering', 'failed'].includes(status)) return null;
  if (status === 'failed') {
    const copy = productMessage('playback.failed');
    return <>
      {!failureDismissed && <div className="player-status-layer player-status-failed" role="alert">
        {copy.icon && <ProductLanguageIcon id={copy.icon} />}
        <span><strong>{failure?.title ?? copy.title}</strong><small>{failure?.message ?? copy.body}</small></span>
        <button type="button" onClick={() => void retry()}><ProductLanguageIcon id="action.retry" /> {productMessage('action.retry').text}</button>
        {(playback?.media.actions as readonly string[] | undefined)?.includes('feedback.report-problem') && <button type="button" onClick={() => setReporting(true)}><ProductLanguageIcon id="action.report" /> {productMessage('action.report-problem').text}</button>}
        <button type="button" className="player-status-dismiss" aria-label={productMessage('action.dismiss-playback-message').text} onClick={() => setFailureDismissed(true)}><ProductLanguageIcon id="action.close" /></button>
      </div>}
      {reporting && playback && <FeedbackDialog kind="playback" mediaId={playback.media.id} playbackSessionId={playback.sessionId} title={productMessage('feedback.kind.playback').text ?? 'Playback problem'} onDismiss={() => setReporting(false)} />}
    </>;
  }
  const copy = status === 'buffering' ? productMessage('playback.buffering') : status === 'recovering' ? productMessage('playback.reconnecting') : productMessage('playback.preparing');
  return <div className="player-status-layer" role="status" aria-live="polite">{copy.icon && <ProductLanguageIcon id={copy.icon} className="state-spinner" />} <strong>{copy.title}</strong></div>;
}

function PostPlaySurface({ onClose, autoplay }: { onClose: () => void; autoplay: boolean }) {
  const { postPlay, next, beginPostPlay, cancelPostPlay, replay } = usePlayback();
  const preferences = useOptionalWebDisplayPreferences()?.preferences ?? defaultWebDisplayPreferences;
  const countdownSeconds = autoplay ? preferences.upNextCountdownSeconds : 0;
  const [remaining, setRemaining] = useState<number>(countdownSeconds);
  const countdownStateRef = useRef<UpNextCountdownState>({ phase: 'inactive' });

  useEffect(() => {
    if (postPlay.phase !== 'countdown' || countdownSeconds === 0) return;
    const initial = reduceUpNextCountdown(countdownStateRef.current, {
      type: 'prepared', now: Date.now(), countdownSeconds, preparationExpiresAt: postPlay.expiresAt,
    });
    countdownStateRef.current = initial.state;
    if (initial.effect === 'handoff') {
      void next(true);
      return;
    }
    const update = () => {
      const now = Date.now();
      setRemaining(Math.max(0, Math.ceil(((countdownStateRef.current.deadlineAt ?? now) - now) / 1_000)));
      const transition = reduceUpNextCountdown(countdownStateRef.current, { type: 'tick', now });
      countdownStateRef.current = transition.state;
      if (transition.effect === 'handoff') void next(true);
    };
    update();
    const interval = window.setInterval(update, 250);
    return () => {
      window.clearInterval(interval);
    };
  }, [countdownSeconds, next, postPlay]);

  useEffect(() => {
    if (postPlay.phase === 'cancelled') countdownStateRef.current = reduceUpNextCountdown(countdownStateRef.current, { type: 'cancel' }).state;
    else if (postPlay.phase !== 'countdown') countdownStateRef.current = reduceUpNextCountdown(countdownStateRef.current, { type: 'reset' }).state;
  }, [postPlay.phase]);

  if (postPlay.phase === 'inactive') return null;
  if (postPlay.phase === 'exhausted') {
    const copy = productMessage('playback.complete');
    return <div className="post-play-surface post-play-exhausted" role="region" aria-label={copy.body}>
    <div className="post-play-copy"><span>{copy.body}</span><strong>{copy.title}</strong></div>
    <div className="post-play-actions"><button type="button" className="primary" onClick={() => void replay()}><ProductLanguageIcon id="action.replay" /> {productMessage('action.replay').text}</button><button type="button" onClick={onClose}><ProductLanguageIcon id="action.close" /> {productMessage('action.close-player').text}</button></div>
  </div>;
  }

  const nextItem = postPlay.next;
  const artwork = nextItem.images.poster || nextItem.images.thumb || nextItem.images.backdrop;
  const presentation = postPlay.phase === 'preparing'
    ? productMessage('playback.preparing')
    : postPlay.phase === 'passout'
      ? productMessage('playback.still-watching')
    : postPlay.phase === 'failed'
      ? productMessage('playback.up-next-failed')
    : postPlay.phase === 'cancelled'
      ? productMessage('playback.autoplay-cancelled')
      : productMessage('playback.up-next');
  return <div className="post-play-surface" role="region" aria-label={productMessage('playback.up-next').title}>
    {artwork && <img src={artwork} alt="" />}
    <div className="post-play-copy">
      <span>{presentation.title}</span>
      <strong>{nextItem.title}</strong>
      {postPlay.phase === 'countdown' && countdownSeconds > 0 && <small>{productMessage(remaining === 1 ? 'playback.up-next-countdown-one' : 'playback.up-next-countdown-many', { seconds: remaining }).body}</small>}
      {postPlay.phase === 'countdown' && countdownSeconds === 0 && <small>{productMessage('playback.up-next-ready').body}</small>}
      {postPlay.phase === 'failed' && <small>{presentation.body}</small>}
      {postPlay.phase === 'cancelled' && <small>{presentation.body}</small>}
      {postPlay.phase === 'passout' && <small>{presentation.body}</small>}
    </div>
    <div className="post-play-actions">
      {postPlay.phase === 'preparing' && <button type="button" onClick={cancelPostPlay}><ProductLanguageIcon id="action.cancel" /> {productMessage('action.cancel').text}</button>}
      {postPlay.phase === 'countdown' && <><button type="button" className="primary" onClick={() => void next()}><ProductLanguageIcon id="action.play" /> {productMessage('action.play-now').text}</button><button type="button" onClick={cancelPostPlay}><ProductLanguageIcon id="action.cancel" /> {productMessage('action.cancel').text}</button><button type="button" onClick={() => void replay()}><ProductLanguageIcon id="action.replay" /> {productMessage('action.replay').text}</button></>}
      {postPlay.phase === 'cancelled' && <><button type="button" className="primary" onClick={() => void next()}><ProductLanguageIcon id="action.next" /> {productMessage('action.play-next').text}</button><button type="button" onClick={() => void replay()}><ProductLanguageIcon id="action.replay" /> {productMessage('action.replay').text}</button></>}
      {postPlay.phase === 'passout' && <><button type="button" className="primary" onClick={() => void next()}><ProductLanguageIcon id="action.play" /> {productMessage('action.still-watching').text}</button><button type="button" onClick={cancelPostPlay}><ProductLanguageIcon id="action.cancel" /> {productMessage('action.stop-autoplay').text}</button></>}
      {postPlay.phase === 'failed' && <><button type="button" className="primary" onClick={() => void beginPostPlay(nextItem, true, postPlay.preparationRequest)}><ProductLanguageIcon id="action.retry" /> {productMessage('action.retry').text}</button><button type="button" onClick={() => void replay()}><ProductLanguageIcon id="action.replay" /> {productMessage('action.replay').text}</button></>}
    </div>
  </div>;
}

type PlayerSettingChoice = { id: string; label: string; detail?: string };

const PlayerSettingCoordinatorContext = createContext<PlayerMenuCoordinatorValue | undefined>(undefined);

function PlayerSettingGroup({ children }: { children: ReactNode }) {
  const [activeId, setActiveId] = useState<string>();
  const value = useMemo(() => ({ activeId, setActiveId }), [activeId]);
  return <PlayerSettingCoordinatorContext.Provider value={value}>{children}</PlayerSettingCoordinatorContext.Provider>;
}

function PlayerSettingDropdown({ label, value, options, onChange }: {
  label: string;
  value: string;
  options: PlayerSettingChoice[];
  onChange: (value: string) => void;
}) {
  const dropdownId = useId();
  const coordinator = useContext(PlayerSettingCoordinatorContext);
  const [localOpen, setLocalOpen] = useState(false);
  const open = coordinator ? coordinator.activeId === dropdownId : localOpen;
  const setOpen = (next: boolean) => {
    if (coordinator) coordinator.setActiveId(next ? dropdownId : undefined);
    else setLocalOpen(next);
  };
  const triggerRef = useRef<HTMLButtonElement>(null);
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const selected = options.find((option) => option.id === value) ?? options[0];
  const selectedIndex = Math.max(0, options.findIndex((option) => option.id === selected?.id));
  const listId = useId();
  const focusOption = (index: number) => {
    if (!options.length) return;
    const normalized = (index + options.length) % options.length;
    optionRefs.current[normalized]?.focus();
  };
  const openAndFocus = (index: number) => {
    setOpen(true);
    window.requestAnimationFrame(() => focusOption(index));
  };
  const choose = (option: PlayerSettingChoice) => {
    onChange(option.id);
    setOpen(false);
    window.requestAnimationFrame(() => triggerRef.current?.focus());
  };
  const onTriggerKeyDown = (event: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      openAndFocus(event.key === 'ArrowDown' ? selectedIndex : selectedIndex - 1);
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault();
      openAndFocus(event.key === 'Home' ? 0 : options.length - 1);
    }
  };
  const onOptionKeyDown = (event: ReactKeyboardEvent<HTMLButtonElement>, index: number) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      focusOption(index + (event.key === 'ArrowDown' ? 1 : -1));
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault();
      focusOption(event.key === 'Home' ? 0 : options.length - 1);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
      triggerRef.current?.focus();
    } else if (event.key === 'Tab') {
      setOpen(false);
    }
  };
  if (!selected) return null;
  return <section className={`player-setting-dropdown ${open ? 'open' : ''}`}>
    <span className="player-setting-label">{label}</span>
    <button ref={triggerRef} type="button" role="combobox" className="player-setting-dropdown-trigger" aria-label={label} aria-haspopup="listbox" aria-expanded={open} aria-controls={listId} onClick={() => setOpen(!open)} onKeyDown={onTriggerKeyDown}>
      <span><strong>{selected.label}</strong>{selected.detail && <small>{selected.detail}</small>}</span><NavigationExpandIcon aria-hidden="true" />
    </button>
    <div id={listId} className="player-setting-options" role="listbox" aria-label={label} aria-hidden={!open}>
      {options.map((option, index) => <button ref={(node) => { optionRefs.current[index] = node; }} key={option.id} type="button" role="option" aria-selected={option.id === value} className={option.id === value ? 'selected' : ''} tabIndex={open && index === selectedIndex ? 0 : -1} onClick={() => choose(option)} onKeyDown={(event) => onOptionKeyDown(event, index)}>
        <span><strong>{option.label}</strong>{option.detail && <small>{option.detail}</small>}</span>
        {option.id === value && <ActionConfirmIcon className="player-choice-check" aria-hidden="true" />}
      </button>)}
    </div>
  </section>;
}

function PlayerControls({
  full,
  browserFullscreen,
  sessionAutoplay,
  onSessionAutoplayChange,
  onToggleBrowserFullscreen,
  onClose,
}: {
  full: boolean;
  browserFullscreen: boolean;
  sessionAutoplay: boolean;
  onSessionAutoplayChange: (value: boolean) => void;
  onToggleBrowserFullscreen: () => void;
  onClose: () => void;
}) {
  const player = usePlayback();
  const source = usePorticoDataSource();
  const auth = useAuthSession();
  const displayPreferences = useOptionalWebDisplayPreferences();
  const preferences = displayPreferences?.preferences ?? defaultWebDisplayPreferences;
  const musicPreferences = useMemo(() => normalizeMusicPlaybackPreferences(auth.viewer?.user?.preferences?.musicPlayback), [auth.viewer?.user?.preferences?.musicPlayback]);
  const { selectedId: subtitleId, setSelectedId: setSubtitleId, options: subtitleOptions } = usePlayerSubtitles();
  const { status, playback, queue, repeatMode, sessionOrigin, mediaRef, touch, complete, next, prepareNext, handoff, previous, start, renegotiate, beginPostPlay, fail, shuffleQueue, setRepeatMode, markMeaningfulInteraction } = player;
  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const storedVolume = useRef(loadStoredPlayerVolume());
  const [volume, setVolume] = useState(storedVolume.current.volume);
  const [muted, setMuted] = useState(storedVolume.current.muted);
  const volumeRef = useRef(volume);
  volumeRef.current = volume;
  const [playbackRate, setPlaybackRate] = useState(() => Number(preferences.defaultPlaybackSpeed));
  const [quality, setQuality] = useState(playback?.selectedQualityId || (playback ? defaultPlaybackQuality(playback) : 'original'));
  const [audioStreamId, setAudioStreamId] = useState(playback?.selectedAudioStreamId ?? '');
  const [trickplaySets, setTrickplaySets] = useState<MediaTrickplaySet[]>([]);
  const [trickplayPreview, setTrickplayPreview] = useState<{ seconds: number; url?: string; leftPercent: number }>();
  const [trickplayImagesAvailable, setTrickplayImagesAvailable] = useState(true);
  const [dismissedSegments, setDismissedSegments] = useState<{ sessionId: string; ids: string[] }>({ sessionId: '', ids: [] });
  const dismissedSegmentIds = dismissedSegments.sessionId === (playback?.sessionId ?? '') ? dismissedSegments.ids : [];
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false);
  const [musicTransitioning, setMusicTransitioning] = useState(false);
  const defaultsAppliedSessionRef = useRef('');
  const terminalSessionRef = useRef('');
  const seekTransactionRef = useRef<ReturnType<typeof createSeekTransaction> | undefined>(undefined);
  const commandRowRef = useRef<HTMLDivElement>(null);
  const mode = playback ? playerContentMode(playback.media, playback.isLive) : 'video';
  const isLive = mode === 'live';
  const isMusic = mode === 'music';
  const segmentDecision = useMemo(() => playbackSegmentAutomationDecision(
    playback?.media.segments, currentTime, dismissedSegmentIds,
    { intro: preferences.introSkip, credits: preferences.creditsSkip }, isLive,
  ), [currentTime, dismissedSegmentIds, isLive, playback?.media.segments, preferences.creditsSkip, preferences.introSkip]);
  const activeSegment = segmentDecision.type === 'prompt' ? segmentDecision.segment : undefined;
  const upcomingItems = useMemo(
    () => (queue?.items ?? playback?.queue ?? []).filter((item) => item.id !== playback?.media.id),
    [playback?.media.id, playback?.queue, queue?.items],
  );
  const lyricDocument = useMemo(() => selectLyricDocument(playback?.media.lyrics ?? []), [playback?.media.lyrics]);
  const sleepTimer = useSleepTimer(onClose);
  const resolveMediaSessionResource = useCallback((path: string) => source.playbackResourceUrl(path), [source]);

  useEffect(() => {
    terminalSessionRef.current = '';
    setQuality(playback?.selectedQualityId || (playback ? defaultPlaybackQuality(playback) : 'original'));
    setAudioStreamId(playback?.selectedAudioStreamId ?? '');
    setPlaybackRate(Number(preferences.defaultPlaybackSpeed));
    setTrickplayPreview(undefined);
    setTrickplayImagesAvailable(true);
    setDiagnosticsOpen(false);
  }, [playback?.sessionId]);

  useEffect(() => {
    if (!playback || !queue || !isMusic || sessionOrigin !== 'start' || defaultsAppliedSessionRef.current === playback.sessionId) return;
    defaultsAppliedSessionRef.current = playback.sessionId;
    const applyDefaults = async () => {
      const desiredRepeatMode = accountRepeatMode(musicPreferences.repeatDefault);
      if (queue.repeatMode !== desiredRepeatMode) await setRepeatMode(desiredRepeatMode);
      if (musicPreferences.shuffleDefault && queue.items.length > 1 && queue.canMutate !== false) await shuffleQueue();
    };
    void applyDefaults().catch(() => undefined);
  }, [isMusic, musicPreferences.repeatDefault, musicPreferences.shuffleDefault, playback, queue, sessionOrigin, setRepeatMode, shuffleQueue]);

  useEffect(() => {
    const controller = new AbortController();
    setTrickplaySets([]);
    if (!playback || isLive || !supportsTrickplayPreview(playback.media)) return () => controller.abort();
    source.mediaTrickplay(playback.media.id, controller.signal).then((sets) => {
      if (!controller.signal.aborted) setTrickplaySets(sets);
    }).catch(() => undefined);
    return () => controller.abort();
  }, [isLive, playback?.media.id, playback?.sessionId, source]);

  useEffect(() => {
    if (commandRowRef.current) commandRowRef.current.scrollLeft = 0;
  }, [full, playback?.sessionId]);

  useEffect(() => {
    const element = mediaRef.current;
    if (!element) return;
    const persistedVolume = loadStoredPlayerVolume();
    element.volume = effectivePlaybackVolume(
      persistedVolume.volume,
      playback?.media.audioNormalization,
      isMusic ? musicPreferences.normalizationMode : preferences.audioNormalizationMode,
    );
    element.muted = persistedVolume.muted;
    element.playbackRate = playbackRate;
    const sync = () => {
      setPlaying(!element.paused);
      setCurrentTime(element.currentTime || 0);
      setDuration(Number.isFinite(element.duration) ? element.duration : playback?.timeline.durationSeconds ?? 0);
      setMuted(element.muted);
    };
    const persistVolume = () => storePlayerVolume(volumeRef.current, element.muted);
    const onEnded = () => {
      if (isLive) {
        fail('source');
        return;
      }
      if (sleepTimer.expireAtTrackEnd()) return;
      if (isMusic && musicTransitioning) return;
      if (repeatMode === 'one') {
        element.currentTime = 0;
        void element.play();
        return;
      }
      if (!playback?.sessionId || terminalSessionRef.current === playback.sessionId) return;
      terminalSessionRef.current = playback.sessionId;
      const terminalDuration = Number.isFinite(element.duration) && element.duration > 0
        ? element.duration
        : playback?.timeline.durationSeconds ?? 0;
      if (upcomingItems.length > 0) {
        const candidate = upcomingItems[0];
        if (isMusic) {
          if (sessionAutoplay) {
            void next(false, musicTransitionRequest(playback, queue, musicPreferences, preferences)).then((committed) => {
              if (!committed && terminalDuration > 0) void complete(terminalDuration);
            });
            return;
          }
        } else {
          void beginPostPlay(candidate);
        }
      } else if (repeatMode === 'all' && (queue?.history.length ?? 0) > 0) {
        const first = queue?.history[0];
        if (first) void start(first.id, { queueMediaIds: [...(queue?.history.slice(1) ?? []), queue?.current].filter(Boolean).map((item) => item.id), repeatMode: 'all', sourceContext: queue?.sourceContext });
      } else {
        void beginPostPlay();
      }
      if (terminalDuration > 0) void complete(terminalDuration);
    };
    sync();
    for (const event of ['play', 'pause', 'timeupdate', 'durationchange', 'volumechange', 'loadedmetadata']) element.addEventListener(event, sync);
    element.addEventListener('volumechange', persistVolume);
    element.addEventListener('ended', onEnded);
    return () => {
      for (const event of ['play', 'pause', 'timeupdate', 'durationchange', 'volumechange', 'loadedmetadata']) element.removeEventListener(event, sync);
      element.removeEventListener('volumechange', persistVolume);
      element.removeEventListener('ended', onEnded);
    };
  }, [beginPostPlay, complete, fail, isLive, isMusic, mediaRef, musicPreferences, musicTransitioning, next, playback, preferences, queue, repeatMode, sessionAutoplay, sleepTimer.expireAtTrackEnd, start, touch, upcomingItems]);

  useEffect(() => {
    const element = mediaRef.current;
    if (!element || !playback) return;
    element.volume = effectivePlaybackVolume(
      volume,
      playback.media.audioNormalization,
      isMusic ? musicPreferences.normalizationMode : preferences.audioNormalizationMode,
    );
    element.muted = muted;
    storePlayerVolume(volume, muted);
  }, [isMusic, mediaRef, musicPreferences.normalizationMode, muted, playback, preferences.audioNormalizationMode, volume]);

  useEffect(() => {
    const element = mediaRef.current;
    if (element) element.playbackRate = playbackRate;
  }, [mediaRef, playback?.sessionId, playbackRate]);

  useEffect(() => {
    const element = mediaRef.current;
    seekTransactionRef.current?.cancel();
    seekTransactionRef.current = element ? createSeekTransaction(element) : undefined;
    return () => {
      seekTransactionRef.current?.cancel();
      seekTransactionRef.current = undefined;
    };
  }, [mediaRef, playback?.sessionId]);

  useEffect(() => {
    if (segmentDecision.type !== 'seek') return;
    const element = mediaRef.current;
    if (!element || element.currentTime >= segmentDecision.positionSeconds) return;
    element.currentTime = segmentDecision.positionSeconds;
    setCurrentTime(segmentDecision.positionSeconds);
    setDismissedSegments((current) => {
      const sessionId = playback?.sessionId ?? '';
      const ids = current.sessionId === sessionId ? current.ids : [];
      return { sessionId, ids: ids.includes(segmentDecision.segment.id) ? ids : [...ids, segmentDecision.segment.id] };
    });
    void touch({ positionSeconds: segmentDecision.positionSeconds, durationSeconds: duration || undefined, state: element.paused ? 'paused' : 'playing' });
  }, [duration, mediaRef, playback?.sessionId, segmentDecision, touch]);

  const togglePlayback = () => {
    const element = mediaRef.current;
    if (!element) return;
    markMeaningfulInteraction();
    if (element.paused) void element.play(); else element.pause();
  };
  const seekBy = (seconds: number) => {
    const element = mediaRef.current;
    if (!element || isLive) return;
    markMeaningfulInteraction();
    const target = element.currentTime + seconds;
    const sourceDuration = Number.isFinite(element.duration) ? element.duration : duration;
    if (Number.isFinite(sourceDuration) && sourceDuration > 0 && target >= sourceDuration) {
      element.pause();
      if (repeatMode === 'one') {
        element.currentTime = 0;
        setCurrentTime(0);
        void element.play().catch(() => undefined);
        return;
      }
      setCurrentTime(sourceDuration);
      void complete(sourceDuration);
      void beginPostPlay();
      return;
    }
    const state = element.paused ? 'paused' : 'playing';
    void seekTransactionRef.current?.seek(target).then((result) => {
      if (result !== 'completed') return;
      setCurrentTime(element.currentTime);
      void touch({ positionSeconds: element.currentTime, durationSeconds: Number.isFinite(element.duration) ? element.duration : undefined, state });
    });
  };
  const seekTo = (seconds: number) => {
    const element = mediaRef.current;
    if (!element || isLive) return;
    markMeaningfulInteraction();
    const state = element.paused ? 'paused' : 'playing';
    setCurrentTime(seconds);
    void seekTransactionRef.current?.seek(seconds).then((result) => {
      if (result !== 'completed') return;
      setCurrentTime(element.currentTime);
      void touch({ positionSeconds: element.currentTime, durationSeconds: duration || undefined, state });
    });
  };
  const changeVolume = (value: number) => {
    setVolume(value);
    setMuted(value === 0);
  };
  const toggleMuted = () => {
    setMuted((value) => !value);
  };
  const closeWithSleepTimer = () => {
    sleepTimer.clear();
    onClose();
  };
  const selectQuality = async (value: string) => {
    const updated = await renegotiate({ qualityId: value });
    if (updated) setQuality(updated.selectedQualityId || value);
  };
  const selectAudio = async (value: string) => {
    const updated = await renegotiate({ audioStreamId: value });
    if (updated) setAudioStreamId(updated.selectedAudioStreamId || value);
  };
  const selectSubtitle = async (value: string) => {
    if (!playback) return;
    const stream = playback.subtitleStreams.find((candidate) => candidate.id === value);
    const browserOwned = value.startsWith(NATIVE_SUBTITLE_PREFIX) || Boolean(stream?.sourceUrl);
    const currentlyServerSelected = playback.selectedSubtitleMode && playback.selectedSubtitleMode !== 'off';
    if (value === NO_SUBTITLE_ID || browserOwned) {
      if (currentlyServerSelected) {
        const updated = await renegotiate({ subtitleMode: 'off', subtitleStreamId: '' });
        if (!updated) return;
      }
      setSubtitleId(value);
      return;
    }
    const subtitleMode = burnInSubtitleIDFor(playback.subtitleStreams, value) ? 'burn_in' : 'text';
    const updated = await renegotiate({ subtitleMode, subtitleStreamId: value });
    if (updated) setSubtitleId(updated.selectedSubtitleStreamId || value);
  };

  useMediaSession(playback?.media, mediaRef, {
    play: () => { if (mediaRef.current) void mediaRef.current.play().catch(() => undefined); },
    pause: () => mediaRef.current?.pause(),
    stop: closeWithSleepTimer,
    seekTo,
    seekBy,
    previous: () => { void previous(); },
    next: () => { void next(); },
    skipBackSeconds: preferences.skipBackSeconds,
    skipForwardSeconds: preferences.skipForwardSeconds,
  }, resolveMediaSessionResource);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target;
      if (target instanceof Element && target.closest('input, select, textarea, button, summary')) return;
      if (event.code === 'Space' || event.key.toLowerCase() === 'k') { event.preventDefault(); togglePlayback(); }
      else if (event.key === 'ArrowLeft') seekBy(-preferences.skipBackSeconds);
      else if (event.key === 'ArrowRight') seekBy(preferences.skipForwardSeconds);
      else if (event.key.toLowerCase() === 'm') toggleMuted();
      else if (event.key.toLowerCase() === 'f') onToggleBrowserFullscreen();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  });

  if (!playback) return null;
  const completed = status === 'completed';
  const completedDuration = playback.timeline.durationSeconds ?? duration;
  const displayedDuration = completed && completedDuration > 0 ? completedDuration : duration;
  const displayedCurrentTime = completed ? displayedDuration : currentTime;
  const hasLyrics = Boolean(lyricDocument);
  const audioMode = mode === 'music' || mode === 'audiobook';
  const trackMenuLabel = productMessage(audioMode ? 'playback.menu-lyrics' : 'playback.menu-subtitles').text ?? '';
  const hasTrackMenu = audioMode ? hasLyrics : subtitleOptions.length > 0;
  const progress = displayedDuration > 0 ? Math.min(100, Math.max(0, (displayedCurrentTime / displayedDuration) * 100)) : 0;
  const progressStyle = { '--player-progress': `${progress}%` } as CSSProperties;
  const trickplaySet = activeTrickplaySet(trickplaySets);
  const updateTrickplayPreview = (event: ReactMouseEvent<HTMLInputElement>) => {
    if (duration <= 0) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    const ratio = Math.min(1, Math.max(0, (event.clientX - bounds.left) / Math.max(1, bounds.width)));
    const seconds = ratio * duration;
    let url: string | undefined;
    if (trickplaySet && trickplayImagesAvailable) {
      const tileIndex = Math.min(trickplaySet.tileCount - 1, Math.max(0, Math.floor(seconds / trickplaySet.intervalSeconds)));
      const path = `/api/media/${encodeURIComponent(playback.media.id)}/trickplay/${encodeURIComponent(trickplaySet.id)}/tiles/${tileIndex}.jpg`;
      url = playbackResourceUrl(playback, path, (value) => source.playbackResourceUrl(value), window.location.href);
    }
    setTrickplayPreview({ seconds, url, leftPercent: ratio * 100 });
  };
  return <div className={`player-controls ${full ? 'full' : 'mini'}`}>
    {isLive
      ? <div className="player-timeline player-live-timeline"><span className="live-indicator">{productMessage('playback.live').text}</span><span>{productMessage('playback.watching-now').text}</span></div>
      : <div className="player-timeline">
        <span>{timeLabel(displayedCurrentTime)}</span>
        <span className="player-timeline-track">
          {trickplayPreview && <span className={`trickplay-preview ${trickplayPreview.url ? '' : 'time-only'}`} style={{ left: `${trickplayPreview.leftPercent}%` }}>{trickplayPreview.url && <img src={trickplayPreview.url} alt="" onError={() => { setTrickplayImagesAvailable(false); setTrickplayPreview((current) => current ? { ...current, url: undefined } : current); }} />}<small>{timeLabel(trickplayPreview.seconds)}</small></span>}
          <input type="range" min={0} max={Math.max(1, displayedDuration)} step={0.1} value={Math.min(displayedCurrentTime, Math.max(1, displayedDuration))} style={progressStyle} onChange={(event) => seekTo(Number(event.target.value))} onMouseMove={updateTrickplayPreview} onMouseLeave={() => setTrickplayPreview(undefined)} aria-label={productMessage('playback.position').text} />
        </span>
        <span>{timeLabel(displayedDuration)}</span>
      </div>}
    {activeSegment && <div className="player-skip-prompt"><button type="button" onClick={() => seekTo(activeSegment.endSeconds)}>{productMessage('action.skip-segment', { segment: segmentLabel(activeSegment.type) }).text}</button><button type="button" aria-label={productMessage('action.dismiss-skip-prompt', { segment: segmentLabel(activeSegment.type).toLocaleLowerCase() }).text} onClick={() => setDismissedSegments((current) => ({ sessionId: playback?.sessionId ?? '', ids: current.sessionId === (playback?.sessionId ?? '') ? [...current.ids, activeSegment.id] : [activeSegment.id] }))}><ProductLanguageIcon id="action.close" /></button></div>}
    {diagnosticsOpen && <ModalOverlay labelledBy="player-diagnostics-title" className="player-diagnostics-overlay" onDismiss={() => setDiagnosticsOpen(false)}><header><span><ProductLanguageIcon id="action.technical-stats" /><strong id="player-diagnostics-title">{productMessage('playback.diagnostics-title').text}</strong></span><button type="button" aria-label={productMessage('action.close-playback-diagnostics').text} onClick={() => setDiagnosticsOpen(false)}><ProductLanguageIcon id="action.close" /></button></header><dl><div><dt>{productMessage('playback.diagnostics-playback').text}</dt><dd>{playbackDecisionLabel(playback.decision.mode)}</dd></div><div><dt>{productMessage('playback.diagnostics-format').text}</dt><dd>{playbackSelectionRequiresHLS(playback, quality, audioStreamId) ? 'HLS' : playback.streamFormat || productMessage('playback.diagnostics-direct').text}</dd></div><div><dt>{productMessage('playback.diagnostics-quality').text}</dt><dd>{playback.qualities.find((item) => item.id === quality)?.label ?? ''}</dd></div><div><dt>{productMessage('playback.diagnostics-audio').text}</dt><dd>{playback.audioStreams.find((item) => item.id === audioStreamId)?.displayTitle ?? (audioStreamId || productMessage('playback.diagnostics-default').text)}</dd></div><div><dt>{productMessage('playback.diagnostics-rate').text}</dt><dd>{playbackRate}×</dd></div><div><dt>{productMessage('playback.diagnostics-position').text}</dt><dd>{timeLabel(currentTime)} / {timeLabel(duration)}</dd></div>{playback.decision.reason && <div><dt>{productMessage('playback.diagnostics-decision').text}</dt><dd>{playback.decision.reason}</dd></div>}</dl></ModalOverlay>}
    <PlayerMenuGroup><div ref={commandRowRef} className="player-command-row">
      <div className="player-transport" aria-label={productMessage('playback.transport-controls').text}>
        <button type="button" onClick={() => void previous()} disabled={isLive || !queue?.history.length} aria-label={productMessage('action.previous-item').text}><ProductLanguageIcon id="action.previous" /></button>
        <button type="button" className="player-skip-button" onClick={() => seekBy(-preferences.skipBackSeconds)} disabled={isLive} aria-label={productMessage('action.rewind-seconds', { seconds: preferences.skipBackSeconds }).text}><ProductLanguageIcon id="action.replay" /><span>{preferences.skipBackSeconds}</span></button>
        <button type="button" className="play-toggle" onClick={togglePlayback} aria-label={productMessage(playing ? 'action.pause' : 'action.play').text}><ProductLanguageIcon id={playing ? 'action.pause' : 'action.play'} filled /></button>
        <button type="button" className="player-skip-button" onClick={() => seekBy(preferences.skipForwardSeconds)} disabled={isLive} aria-label={productMessage('action.forward-seconds', { seconds: preferences.skipForwardSeconds }).text}><ProductLanguageIcon id="action.refresh" /><span>{preferences.skipForwardSeconds}</span></button>
        <button type="button" onClick={() => void next()} disabled={isLive || !upcomingItems.length} aria-label={productMessage('action.next-item').text}><ProductLanguageIcon id="action.next" /></button>
      </div>
      <div className="player-utilities" aria-label={productMessage('playback.utilities').text}>
        <PlayerMenu label={productMessage('playback.menu-volume').text ?? ''} icon={<ProductLanguageIcon id={muted || volume === 0 ? 'action.mute' : 'action.volume'} />} panelClassName="volume-menu-panel"><div className="volume-panel"><input className="player-volume-slider" type="range" min={0} max={1} step={0.05} value={muted ? 0 : volume} onChange={(event) => changeVolume(Number(event.target.value))} aria-label={productMessage('playback.menu-volume').text} aria-orientation="vertical" aria-valuetext={`${Math.round((muted ? 0 : volume) * 100)}%`} /><button type="button" onClick={toggleMuted}><ProductLanguageIcon id={muted ? 'action.unmute' : 'action.mute'} /> {productMessage(muted ? 'action.unmute' : 'action.mute').text}</button></div></PlayerMenu>
        <PlayerMenu label={trackMenuLabel} icon={<ProductLanguageIcon id={audioMode ? 'action.lyrics' : 'action.subtitles'} />} disabled={!hasTrackMenu} panelClassName={audioMode ? 'lyrics-menu-panel' : ''}>{(dismiss) =>
          <div className="track-panel">
          {!audioMode && <>
            <div className="player-menu-heading"><span><strong>{productMessage('playback.menu-subtitles').text}</strong><small>{subtitleId === NO_SUBTITLE_ID ? productMessage('playback.subtitles-off').text : subtitleOptions.find((option) => option.id === subtitleId)?.label}</small></span></div>
            <div className="subtitle-choice-list" role="radiogroup" aria-label={productMessage('playback.subtitle-track').text}>
              <button type="button" role="radio" aria-checked={subtitleId === NO_SUBTITLE_ID} className={subtitleId === NO_SUBTITLE_ID ? 'selected' : ''} onClick={() => { dismiss(); void selectSubtitle(NO_SUBTITLE_ID); }}>{productMessage('playback.subtitles-off').text}</button>
              {subtitleOptions.map((option) => <button key={option.id} type="button" role="radio" aria-checked={subtitleId === option.id} className={subtitleId === option.id ? 'selected' : ''} onClick={() => { dismiss(); void selectSubtitle(option.id); }}><span>{option.label}</span>{option.language && option.label.toLocaleLowerCase() !== option.language.toLocaleLowerCase() && <small>{option.language.toLocaleUpperCase()}</small>}</button>)}
            </div>
            {displayPreferences && <div className="subtitle-appearance">
              <span>{productMessage('playback.subtitle-text-size').text}</span><div>{(['small', 'medium', 'large'] as const).map((size) => <button key={size} type="button" className={preferences.subtitleSize === size ? 'selected' : ''} aria-pressed={preferences.subtitleSize === size} onClick={() => void displayPreferences.patch({ subtitleSize: size }).catch(() => undefined)}>{productMessage(size === 'small' ? 'playback.option-small' : size === 'medium' ? 'playback.option-medium' : 'playback.option-large').text}</button>)}</div>
              <span>{productMessage('playback.subtitle-background').text}</span><div>{(['none', 'subtle', 'solid'] as const).map((background) => <button key={background} type="button" className={preferences.subtitleBackground === background ? 'selected' : ''} aria-pressed={preferences.subtitleBackground === background} onClick={() => void displayPreferences.patch({ subtitleBackground: background }).catch(() => undefined)}>{productMessage(background === 'none' ? 'playback.option-none' : background === 'subtle' ? 'playback.option-subtle' : 'playback.option-solid').text}</button>)}</div>
            </div>}
          </>}
          {lyricDocument && <LyricsPanel document={lyricDocument} currentTime={currentTime} />}</div>}
        </PlayerMenu>
        {playback.chapters.some((chapter) => typeof chapter.startSeconds === 'number') && <PlayerMenu label={productMessage('playback.menu-chapters').text ?? ''} icon={<ProductLanguageIcon id="action.chapters" />}><div className="chapter-panel"><div className="player-menu-heading"><span><strong>{productMessage('playback.menu-chapters').text}</strong><small>{productMessage(playback.chapters.length === 1 ? 'playback.chapter-count-one' : 'playback.chapter-count-many', { count: playback.chapters.length }).text}</small></span></div><ol>{playback.chapters.flatMap((chapter, index) => typeof chapter.startSeconds === 'number' ? [<li key={chapter.id ?? `${chapter.startSeconds}-${index}`}><button type="button" className={currentTime >= chapter.startSeconds && currentTime < (playback.chapters[index + 1]?.startSeconds ?? Number.POSITIVE_INFINITY) ? 'selected' : ''} onClick={() => seekTo(chapter.startSeconds!)}><ChapterThumbnail src={chapter.thumbUrl ? playbackResourceUrl(playback, chapter.thumbUrl, (value) => source.playbackResourceUrl(value), window.location.href) : undefined} /><span>{chapter.title || productMessage('playback.chapter-number', { number: index + 1 }).text}</span><small>{timeLabel(chapter.startSeconds)}</small></button></li>] : [])}</ol></div></PlayerMenu>}
        <PlayerMenu label={productMessage('playback.menu-settings').text ?? ''} icon={<ProductLanguageIcon id="action.settings" />} panelClassName="settings-menu-panel">{(dismiss) => <PlayerSettingGroup><div className="settings-panel">
          {playback.qualities.some((item) => item.available !== false && Boolean(item.id && item.label)) && <PlayerSettingDropdown label={productMessage('playback.setting-quality').text ?? 'Quality'} value={quality} onChange={(value) => void selectQuality(value)} options={playback.qualities.filter((item) => item.available !== false && Boolean(item.id && item.label)).map((item) => ({ id: item.id!, label: item.label!, detail: item.description }))} />}
          {playback.audioStreams.length > 0 && <PlayerSettingDropdown label={productMessage('playback.setting-audio').text ?? 'Audio'} value={audioStreamId} onChange={(value) => void selectAudio(value)} options={playback.audioStreams.filter((stream) => Boolean(stream.id)).map((stream) => ({ id: stream.id ?? '', label: stream.displayTitle || stream.language || stream.codec || 'Audio', detail: [stream.language?.toLocaleUpperCase(), stream.codec?.toLocaleUpperCase(), stream.channels ? `${stream.channels} channels` : ''].filter(Boolean).join(' · ') }))} />}
          {!isLive && <PlayerSettingDropdown label={productMessage('playback.setting-speed').text ?? 'Playback speed'} value={String(playbackRate)} onChange={(value) => setPlaybackRate(Number(value))} options={PLAYBACK_SPEEDS.map((rate) => ({ id: String(rate), label: rate === 1 ? productMessage('playback.speed-normal').text ?? 'Normal' : `${rate}×` }))} />}
          {!isLive && <button type="button" role="switch" aria-checked={sessionAutoplay} className={`player-toggle-setting ${sessionAutoplay ? 'selected' : ''}`} onClick={() => onSessionAutoplayChange(!sessionAutoplay)}><span><strong>{productMessage('playback.setting-autoplay').text}</strong><small>Continue with the next item automatically.</small></span><i aria-hidden="true" /></button>}
          {!isLive && <PlayerSettingDropdown label={productMessage('playback.setting-sleep-timer').text ?? 'Sleep timer'} value={String(sleepTimer.mode)} onChange={(value) => sleepTimer.setMode(sleepTimerMode(value))} options={[{ id: 'off', label: productMessage('playback.sleep-off').text ?? 'Off' }, { id: 'end', label: productMessage('playback.sleep-end').text ?? 'End of item' }, { id: '15', label: '15 min' }, { id: '30', label: '30 min' }, { id: '45', label: '45 min' }, { id: '60', label: '60 min' }]} />}
          {preferences.playbackDiagnostics && <button type="button" className="player-diagnostics-action" onClick={() => { dismiss(); window.requestAnimationFrame(() => setDiagnosticsOpen(true)); }}><ProductLanguageIcon id="action.technical-stats" /> {productMessage('action.show-technical-stats').text}</button>}
        </div></PlayerSettingGroup>}</PlayerMenu>
        <PlayerMenu label={productMessage('playback.menu-queue').text ?? ''} icon={<ProductLanguageIcon id="action.player-queue" />} wide>{(dismiss) => <QueuePanel onDismiss={dismiss} />}</PlayerMenu>
        <button type="button" onClick={onToggleBrowserFullscreen} aria-label={productMessage(browserFullscreen ? 'action.exit-fullscreen' : 'action.fullscreen').text}><ProductLanguageIcon id={browserFullscreen ? 'action.exit-fullscreen' : 'action.fullscreen'} /></button>
        <button type="button" onClick={closeWithSleepTimer} aria-label={productMessage('action.close-player').text}><ProductLanguageIcon id="action.close" /></button>
      </div>
    </div></PlayerMenuGroup>
    <SourceSelectionBridge quality={quality} audioStreamId={audioStreamId} subtitleId={subtitleId} />
    {isMusic && <MusicTransitionBridge playback={playback} queue={queue} mediaRef={mediaRef} preferences={musicPreferences} volume={volume} muted={muted} enabled={sessionAutoplay} onTransitioning={setMusicTransitioning} handoff={handoff} prepareNext={prepareNext} />}
  </div>;
}

function bufferedSecondsAhead(media: HTMLMediaElement): number {
  const current = media.currentTime;
  if (!Number.isFinite(current)) return 0;
  for (let index = 0; index < media.buffered.length; index += 1) {
    if (media.buffered.start(index) <= current && media.buffered.end(index) >= current) {
      return Math.max(0, media.buffered.end(index) - current);
    }
  }
  return 0;
}

function mediaBufferCanCarryPlayback(media: HTMLMediaElement, minimumSeconds = 1.5): boolean {
  return media.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA && bufferedSecondsAhead(media) >= minimumSeconds;
}

function SourceSelectionBridge({ quality, audioStreamId, subtitleId }: { quality: string; audioStreamId: string; subtitleId: string }) {
  const { playback, mediaRef, status, touch, renewGrant, recoverRoute, adapterRecoveryGeneration, completedSessionId, markReady, markBuffering, markRecovering, fail, interrupt } = usePlayback();
  const source = usePorticoDataSource();
  const loadedSessionRef = useRef('');
  const pendingResumeRef = useRef<{ sessionId: string; positionSeconds: number } | undefined>(undefined);
  const playedSessionRef = useRef('');
  const sourceGenerationRef = useRef(0);
  const lastSourcePositionRef = useRef(0);
  const lastSourceShouldPlayRef = useRef(true);
  const selectedSubtitleStream = playback?.subtitleStreams.find((stream) => stream.id === subtitleId);
  // Browser-playable text tracks are independent overlays. Selecting one must
  // never rebuild the video pipeline; only image/bitmap formats that require
  // server-side burn-in participate in source selection.
  const serverSubtitleId = subtitleId.startsWith(NATIVE_SUBTITLE_PREFIX) || selectedSubtitleStream?.sourceUrl
    ? NO_SUBTITLE_ID
    : subtitleId;
  const selectedQualityId = playback?.selectedQualityId || (playback ? defaultPlaybackQuality(playback) : 'original');
  const selectedAudioStreamId = playback?.selectedAudioStreamId ?? '';
  const selectedServerSubtitleId = playback?.selectedSubtitleMode && playback.selectedSubtitleMode !== 'off'
    ? playback.selectedSubtitleStreamId || NO_SUBTITLE_ID
    : NO_SUBTITLE_ID;
  const selectionAuthorized = Boolean(playback)
    && quality === selectedQualityId
    && audioStreamId === selectedAudioStreamId
    && serverSubtitleId === selectedServerSubtitleId;
  useEffect(() => {
    const media = mediaRef.current;
    if (!media || !playback || !selectionAuthorized || completedSessionId === playback.sessionId) return;
    const sourceGeneration = sourceGenerationRef.current + 1;
    sourceGenerationRef.current = sourceGeneration;
    const sameSession = loadedSessionRef.current === playback.sessionId;
    if (!sameSession) {
      pendingResumeRef.current = {
        sessionId: playback.sessionId,
        positionSeconds: Math.max(0, playback.resumePositionSeconds ?? 0),
      };
    }
    const pendingResume = pendingResumeRef.current?.sessionId === playback.sessionId
      ? pendingResumeRef.current.positionSeconds
      : undefined;
    // Queue refreshes, grant renewals, and route recovery can replace the
    // PlaybackResponse before the media element has metadata. Preserve the
    // server-owned resume marker until it has actually been applied instead
    // of treating the element's temporary zero as authoritative.
    const mediaPosition = Number.isFinite(media.currentTime) ? media.currentTime : 0;
    const resumeAt = pendingResume ?? (sameSession ? Math.max(mediaPosition, lastSourcePositionRef.current) : playback.resumePositionSeconds ?? 0);
    const shouldPlay = pendingResume !== undefined ? true : sameSession ? lastSourceShouldPlayRef.current : true;
    if (!sameSession) {
      lastSourcePositionRef.current = 0;
      lastSourceShouldPlayRef.current = true;
    }
    const resolve = (path: string) => playbackResourceUrl(playback, path, (value) => source.playbackResourceUrl(value), window.location.href);
    let sourceUrl = '';
    try {
      const burnInSubtitleId = burnInSubtitleIDFor(playback.subtitleStreams, serverSubtitleId);
      const forceHLS = burnInSubtitleId !== '' || playbackSelectionRequiresHLS(playback, quality, audioStreamId);
      sourceUrl = playbackSourceFor(playback, resolve, {
        streamFormat: forceHLS ? 'hls' : undefined,
        quality,
        audioStreamId,
        burnInSubtitleId,
        textSubtitleId: serverSubtitleId === NO_SUBTITLE_ID ? '' : serverSubtitleId,
        baseHref: window.location.href,
      });
    } catch {
      fail('source');
      return;
    }
    loadedSessionRef.current = playback.sessionId;
    let hls: HlsInstance | undefined;
    let disposed = false;
    let manifestReadinessRecoveries = 0;
    let networkRecoveries = 0;
    let mediaRecoveries = 0;
    let nativeRecoveries = 0;
    let routeRecoveryAttempted = false;
    let pendingTerminalRecovery: 'network' | 'decode' | undefined;
    let nativeRecoveryReady: (() => void) | undefined;
    // Every source reload (including an in-session quality or route change)
    // must restore its captured position after the replacement source has
    // metadata. A brand-new session uses the pending server resume marker;
    // established sessions use the media element's last known position.
    let initialPositionApplied = resumeAt <= 0;
    const recoveryTimers = new Set<number>();
    const playbackStarted = () => playedSessionRef.current === playback.sessionId || media.currentTime > 0;
    const requestPlayback = () => {
      // The autoplay attribute, HLS manifest callbacks, and source setup can
      // all request playback before loadedmetadata. Never allow the first
      // frame to start at zero while a canonical resume seek is pending.
      if (!shouldPlay || disposed || sourceGenerationRef.current !== sourceGeneration || !initialPositionApplied) return;
      void media.play().catch(() => undefined);
    };
    const stopAfterRecovery = (kind: 'network' | 'decode') => {
      if (disposed) return;
      if (!media.paused && mediaBufferCanCarryPlayback(media)) {
        pendingTerminalRecovery = kind;
        return;
      }
      if (playbackStarted()) {
        const copy = productMessage(kind === 'network' ? 'playback.recovery-network-stopped' : 'playback.recovery-decode-stopped');
        void interrupt({
          title: copy.title ?? '',
          message: copy.body ?? '',
        });
        return;
      }
      fail(kind === 'network' ? 'route' : 'decode');
    };
    const scheduleRecovery = (delay: number, recover: () => void) => {
      // Route/source retries stay invisible while already buffered media can
      // continue. Recovery UI appears only once playback is actually at risk.
      if (!mediaBufferCanCarryPlayback(media)) markRecovering();
      const timer = window.setTimeout(() => {
        recoveryTimers.delete(timer);
        if (!disposed) recover();
      }, delay);
      recoveryTimers.add(timer);
    };
    const ready = () => {
      if (!initialPositionApplied && resumeAt > 0 && Number.isFinite(resumeAt)) {
        media.currentTime = resumeAt;
        initialPositionApplied = true;
      }
      if (pendingResumeRef.current?.sessionId === playback.sessionId) pendingResumeRef.current = undefined;
      markReady();
      requestPlayback();
    };
    const onCanPlay = () => requestPlayback();
    const onError = () => {
      const code = media.error?.code ?? 0;
      if (code === 2 && !routeRecoveryAttempted) {
        routeRecoveryAttempted = true;
        scheduleRecovery(0, () => {
          void recoverRoute().catch(() => {
            if (disposed) return;
            media.load();
            requestPlayback();
          });
        });
        return;
      }
      if (playbackStarted() && nativeRecoveries < 3) {
        const delays = [0, 400, 1_200];
        const delay = delays[nativeRecoveries];
        nativeRecoveries += 1;
        const position = media.currentTime;
        scheduleRecovery(delay, () => {
          nativeRecoveryReady = () => {
            if (disposed || sourceGenerationRef.current !== sourceGeneration) return;
            if (position > 0 && Number.isFinite(position)) media.currentTime = position;
            requestPlayback();
          };
          media.addEventListener('loadedmetadata', nativeRecoveryReady, { once: true });
          media.load();
          requestPlayback();
        });
      } else if (playbackStarted()) stopAfterRecovery(code === 3 ? 'decode' : 'network');
      else if (code === 2) fail('route');
      else if (media.error?.code === 3) fail('decode');
      else if (media.error?.code === 4) fail('source');
      else fail('unknown');
    };
    const resetRecoveries = () => {
      networkRecoveries = 0;
      mediaRecoveries = 0;
      nativeRecoveries = 0;
      pendingTerminalRecovery = undefined;
    };
    const stopPendingRecoveryWhenStarved = () => {
      const pending = pendingTerminalRecovery;
      pendingTerminalRecovery = undefined;
      if (pending) stopAfterRecovery(pending);
    };
    media.addEventListener('loadedmetadata', ready, { once: true });
    media.addEventListener('canplay', onCanPlay);
    media.addEventListener('playing', resetRecoveries);
    media.addEventListener('waiting', stopPendingRecoveryWhenStarved);
    const nativeHlsSupport = media.canPlayType('application/vnd.apple.mpegurl') || media.canPlayType('application/x-mpegURL');
    const nativeHls = shouldUseNativeHLS(nativeHlsSupport, globalThis.navigator?.userAgent ?? '');
    const isHls = playback.streamFormat === 'hls' || sourceUrl.includes('.m3u8');
    if (isHls && !nativeHls) {
      void import('hls.js').then(({ default: Hls }) => {
        if (disposed) return;
        if (!Hls.isSupported()) {
          fail('decode');
          return;
        }
        hls = new Hls({
          enableWorker: true,
          xhrSetup: (request) => {
            // HLS media grants are HttpOnly cookies. Route changes may move the
            // active server between approved origins, so every manifest,
            // subtitle, initialization, and media-segment request must carry
            // the sealed viewer credential rather than relying on Fetch's
            // same-origin default.
            request.withCredentials = true;
          },
          backBufferLength: 90,
          fragLoadingMaxRetry: 6,
          fragLoadingRetryDelay: 250,
          fragLoadingMaxRetryTimeout: 2_000,
          manifestLoadingMaxRetry: 4,
          manifestLoadingRetryDelay: 250,
          manifestLoadingMaxRetryTimeout: 2_000,
        });
        hls.subtitleDisplay = false;
        hls.subtitleTrack = -1;
        hls.on(Hls.Events.SUBTITLE_TRACKS_UPDATED, () => {
          if (!hls || serverSubtitleId === NO_SUBTITLE_ID) return;
          const selected = playableSubtitleStreams(playback.subtitleStreams).find((stream) => stream.id === serverSubtitleId);
          const selectedLanguage = selected?.language?.trim().toLocaleLowerCase();
          const selectedTitle = selected?.displayTitle?.trim().toLocaleLowerCase();
          const index = hls.subtitleTracks.findIndex((track) => {
            const language = track.lang?.trim().toLocaleLowerCase();
            const name = track.name?.trim().toLocaleLowerCase();
            return Boolean((selectedLanguage && language === selectedLanguage) || (selectedTitle && name === selectedTitle));
          });
          hls.subtitleTrack = index >= 0 ? index : hls.subtitleTracks.length === 1 ? 0 : -1;
        });
        hls.on(Hls.Events.MANIFEST_PARSED, requestPlayback);
        hls.on(Hls.Events.FRAG_LOADED, () => {
          networkRecoveries = 0;
          mediaRecoveries = 0;
          pendingTerminalRecovery = undefined;
        });
        hls.on(Hls.Events.ERROR, (_event, data) => {
          if (!data.fatal) return;
          if (recoveryTimers.size > 0) return;
          if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
            const responseStatus = data.response?.code ?? 0;
            const manifestMissing = isExplicitMissingHLSManifest(responseStatus, data.details, playbackStarted());
            if (manifestMissing) {
              fail('source');
              return;
            }
            const manifestPending = isTransientHLSManifestWait(responseStatus, data.details, playbackStarted());
            if (manifestPending) {
              const delays = [250, 500, 1_000, 2_000, 3_000];
              if (manifestReadinessRecoveries < delays.length) {
                const delay = delays[manifestReadinessRecoveries];
                manifestReadinessRecoveries += 1;
                scheduleRecovery(delay, () => hls?.loadSource(sourceUrl));
                return;
              }
              fail('source');
              return;
            }
            if ((responseStatus === 401 || responseStatus === 403) && networkRecoveries === 0) void renewGrant();
            if (responseStatus !== 401 && responseStatus !== 403 && !routeRecoveryAttempted) {
              routeRecoveryAttempted = true;
              scheduleRecovery(0, () => {
                void recoverRoute().catch(() => {
                  if (disposed) return;
                  hls?.startLoad(Number.isFinite(media.currentTime) ? media.currentTime : -1);
                  requestPlayback();
                });
              });
              return;
            }
            const delays = [0, 400, 1_000, 2_000, 3_500];
            if (networkRecoveries < delays.length) {
              const delay = delays[networkRecoveries];
              networkRecoveries += 1;
              scheduleRecovery(delay, () => {
                hls?.startLoad(Number.isFinite(media.currentTime) ? media.currentTime : -1);
                requestPlayback();
              });
              return;
            }
            stopAfterRecovery('network');
            return;
          }
          if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
            const delays = [0, 500, 1_500];
            if (mediaRecoveries < delays.length) {
              const delay = delays[mediaRecoveries];
              mediaRecoveries += 1;
              scheduleRecovery(delay, () => {
                hls?.recoverMediaError();
                requestPlayback();
              });
              return;
            }
            stopAfterRecovery('decode');
            return;
          }
          stopAfterRecovery('decode');
        });
        hls.loadSource(sourceUrl);
        hls.attachMedia(media);
        requestPlayback();
      }).catch(() => fail('decode'));
    } else {
      media.addEventListener('error', onError);
      media.src = sourceUrl;
      media.load();
      requestPlayback();
    }
    return () => {
      // React cleans the old source effect before installing a replacement.
      // Capture state here so a quality/audio/burn-in source change cannot
      // reset an established session to zero or resume something the viewer
      // intentionally paused.
      if (loadedSessionRef.current === playback.sessionId) {
        if (Number.isFinite(media.currentTime) && media.currentTime > 0) lastSourcePositionRef.current = media.currentTime;
        lastSourceShouldPlayRef.current = !media.paused;
      }
      disposed = true;
      for (const timer of recoveryTimers) window.clearTimeout(timer);
      recoveryTimers.clear();
      media.removeEventListener('loadedmetadata', ready);
      media.removeEventListener('canplay', onCanPlay);
      media.removeEventListener('playing', resetRecoveries);
      media.removeEventListener('waiting', stopPendingRecoveryWhenStarved);
      media.removeEventListener('error', onError);
      if (nativeRecoveryReady) media.removeEventListener('loadedmetadata', nativeRecoveryReady);
      hls?.destroy();
    };
  }, [adapterRecoveryGeneration, audioStreamId, completedSessionId, fail, interrupt, markReady, markRecovering, mediaRef, playback?.media.id, playback?.playbackRevision, playback?.sessionId, playback?.streamFormat, quality, recoverRoute, renewGrant, selectionAuthorized, serverSubtitleId, source]);

  useEffect(() => {
    const media = mediaRef.current;
    if (!media || !playback) return;
    const send = () => void touch({ state: media.paused ? 'paused' : 'playing', positionSeconds: media.currentTime, durationSeconds: Number.isFinite(media.duration) ? media.duration : undefined });
    const onPlay = () => void touch({ state: 'playing', positionSeconds: media.currentTime, durationSeconds: Number.isFinite(media.duration) ? media.duration : undefined });
    const onPause = () => void touch({ state: 'paused', positionSeconds: media.currentTime, durationSeconds: Number.isFinite(media.duration) ? media.duration : undefined });
    const onWaiting = () => { markBuffering(); void touch({ state: 'buffering', positionSeconds: media.currentTime, durationSeconds: Number.isFinite(media.duration) ? media.duration : undefined }); };
    const onStalled = () => { if (!mediaBufferCanCarryPlayback(media)) markRecovering(); };
    const onPlaying = () => {
      playedSessionRef.current = playback.sessionId;
      markReady();
    };
    const onOffline = () => { if (!mediaBufferCanCarryPlayback(media)) markRecovering(); };
    const onPageHide = () => void touch({ state: media.paused ? 'paused' : 'playing', positionSeconds: media.currentTime, durationSeconds: Number.isFinite(media.duration) ? media.duration : undefined }, true);
    const onVisibilityChange = () => { if (document.visibilityState === 'hidden') onPageHide(); };
    const interval = window.setInterval(send, 15_000);
    media.addEventListener('play', onPlay);
    media.addEventListener('pause', onPause);
    media.addEventListener('waiting', onWaiting);
    media.addEventListener('stalled', onStalled);
    media.addEventListener('playing', onPlaying);
    media.addEventListener('canplay', onPlaying);
    window.addEventListener('offline', onOffline);
    window.addEventListener('pagehide', onPageHide);
    document.addEventListener('visibilitychange', onVisibilityChange);
    return () => {
      window.clearInterval(interval);
      media.removeEventListener('play', onPlay);
      media.removeEventListener('pause', onPause);
      media.removeEventListener('waiting', onWaiting);
      media.removeEventListener('stalled', onStalled);
      media.removeEventListener('playing', onPlaying);
      media.removeEventListener('canplay', onPlaying);
      window.removeEventListener('offline', onOffline);
      window.removeEventListener('pagehide', onPageHide);
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
  }, [markBuffering, markReady, markRecovering, mediaRef, playback, touch]);

  useEffect(() => {
    const media = mediaRef.current;
    if (!media || !playback) return;
    const sourceGeneration = sourceGenerationRef.current;
    let disposed = false;
    let lastPosition = media.currentTime;
    let stagnantSince = Date.now();
    let recoveryAttempted = false;
    let recoveryStartedAt: number | undefined;
    let terminalFailure = false;
    const failRoute = () => {
      if (terminalFailure || disposed || sourceGenerationRef.current !== sourceGeneration) return;
      terminalFailure = true;
      fail('route');
    };
    const inspectProgress = () => {
      if (disposed || terminalFailure) return;
      const position = media.currentTime;
      if (Math.abs(position - lastPosition) >= 0.1) {
        lastPosition = position;
        stagnantSince = Date.now();
        recoveryAttempted = false;
        recoveryStartedAt = undefined;
        return;
      }
      const reason = playbackStallReason(media, document.visibilityState === 'hidden', bufferedSecondsAhead(media));
      if (reason !== 'stalled') {
        stagnantSince = Date.now();
        return;
      }
      const stalledFor = Date.now() - stagnantSince;
      if (stalledFor >= 15_000) markRecovering();
      if (recoveryAttempted) {
        if (recoveryStartedAt !== undefined && Date.now() - recoveryStartedAt >= 20_000) failRoute();
        return;
      }
      if (stalledFor < 30_000) return;
      recoveryAttempted = true;
      recoveryStartedAt = Date.now();
      if (sourceGenerationRef.current !== sourceGeneration) return;
      const nativeHlsSupport = media.canPlayType('application/vnd.apple.mpegurl') || media.canPlayType('application/x-mpegURL');
      const usesHls = playback.streamFormat === 'hls'
        || playbackSelectionRequiresHLS(playback, quality, audioStreamId)
        || burnInSubtitleIDFor(playback.subtitleStreams, serverSubtitleId) !== '';
      const managedHls = usesHls && !shouldUseNativeHLS(nativeHlsSupport, globalThis.navigator?.userAgent ?? '');
      if (managedHls) {
        // hls.js owns its loader retries and adapter reset. Ask the server for
        // a fresh route decision without calling media.load(), which would
        // detach the MediaSource pipeline underneath the adapter.
        void recoverRoute().catch(failRoute);
        return;
      }
      // Grant renewal owns a source-adapter generation change. The source
      // effect captures position and play/pause state during cleanup, then
      // creates the appropriate native or managed-HLS adapter for the fresh
      // grant. Do not race that rebuild with a blind media.load().
      void renewGrant().catch(failRoute);
    };
    const timer = window.setInterval(inspectProgress, 5_000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [audioStreamId, fail, markRecovering, mediaRef, playback?.sessionId, playback?.streamFormat, quality, recoverRoute, renewGrant, serverSubtitleId]);

  useEffect(() => {
    if (status !== 'recovering' && status !== 'buffering') return;
    const media = mediaRef.current;
    if (media?.readyState && media.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA) markReady();
  }, [markReady, mediaRef, status]);
  return null;
}

export function PlayerDock() {
  const { status, playback, mediaRef, close, completedSessionId, interruption, dismissInterruption } = usePlayback();
  const source = usePorticoDataSource();
  const auth = useAuthSession();
  const preferences = useOptionalWebDisplayPreferences()?.preferences ?? defaultWebDisplayPreferences;
  const musicPreferences = useMemo(() => normalizeMusicPlaybackPreferences(auth.viewer?.user?.preferences?.musicPlayback), [auth.viewer?.user?.preferences?.musicPlayback]);
  const location = useLocation();
  const navigate = useNavigate();
  const full = location.pathname.startsWith('/watch/');
  const [lastNonWatchPath, setLastNonWatchPath] = useState('/');
  const [browserFullscreen, setBrowserFullscreen] = useState(Boolean(document.fullscreenElement));
  const [sessionAutoplay, setSessionAutoplay] = useState(preferences.autoplayNext);
  const surfaceRef = useRef<HTMLElement>(null);
  const mediaStageRef = useRef<HTMLDivElement>(null);
  const failureRouteRef = useRef<string | undefined>(undefined);

  useEffect(() => {
    const route = `${location.pathname}${location.search}`;
    if (!playback && status === 'failed') {
      if (failureRouteRef.current && failureRouteRef.current !== route) void close();
      else failureRouteRef.current = route;
      return;
    }
    failureRouteRef.current = undefined;
  }, [close, location.pathname, location.search, playback, status]);

  useEffect(() => {
    if (!location.pathname.startsWith('/watch/')) setLastNonWatchPath(`${location.pathname}${location.search}`);
  }, [location.pathname, location.search]);

  useEffect(() => {
    const update = () => setBrowserFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener('fullscreenchange', update);
    return () => document.removeEventListener('fullscreenchange', update);
  }, []);

  useEffect(() => {
    const stage = mediaStageRef.current;
    const media = mediaRef.current;
    if (!stage || !media || typeof ResizeObserver === 'undefined') return;
    const fit = () => {
      const { width, height } = stage.getBoundingClientRect();
      if (width > 0) media.style.width = `${Math.round(width)}px`;
      if (height > 0) media.style.height = `${Math.round(height)}px`;
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(stage);
    return () => {
      observer.disconnect();
      media.style.removeProperty('width');
      media.style.removeProperty('height');
    };
  }, [mediaRef, playback?.sessionId]);

  useEffect(() => {
    const mode = playback ? playerContentMode(playback.media, playback.isLive) : 'video';
    setSessionAutoplay(mode === 'music' ? musicPreferences.autoplayDefault : preferences.autoplayNext);
  }, [musicPreferences.autoplayDefault, playback?.sessionId, preferences.autoplayNext]);

  useEffect(() => {
    if (interruption && location.pathname.startsWith('/watch/')) navigate(lastNonWatchPath, { replace: true });
  }, [interruption, lastNonWatchPath, location.pathname, navigate]);

  if (interruption && !playback) return <div className="playback-interruption-notice" role="alert" aria-live="assertive">
    <ProductLanguageIcon id="status.warning" />
    <span><strong>{interruption.title}</strong><small>{interruption.message}</small></span>
    <button type="button" aria-label={productMessage('action.dismiss-playback-message').text} onClick={dismissInterruption}><ProductLanguageIcon id="action.close" /></button>
  </div>;
  const closePendingPlayer = () => {
    if (full) navigate(lastNonWatchPath, { replace: true });
    void close();
  };
  if (full && !playback) return <PendingPlayerShell full onClose={closePendingPlayer} />;
  if (!playback && status !== 'idle') return <PendingPlayerShell full={false} onClose={closePendingPlayer} />;
  if (!playback) return null;
  const effectiveFull = full || browserFullscreen;
  const completed = completedSessionId === playback.sessionId;
  const mode = playerContentMode(playback.media, playback.isLive);
  const resolvePlaybackAsset = (path?: string) => path
    ? playbackResourceUrl(playback, path, (value) => source.playbackResourceUrl(value), window.location.href)
    : undefined;
  const artwork = resolvePlaybackAsset(playback.media.displayImages?.backdrop || playback.media.images.backdrop || playback.media.images.poster || playback.media.images.thumb);
  const identityArtwork = resolvePlaybackAsset(mode === 'live'
    ? playback.media.images.thumb || playback.media.images.poster
    : playback.media.images.poster || playback.media.images.thumb) || artwork;
  const subtitle = [playback.media.grandparentTitle || playback.media.parentTitle, playback.media.seasonNumber != null ? `S${playback.media.seasonNumber}` : '', playback.media.episodeNumber != null ? `E${playback.media.episodeNumber}` : ''].filter(Boolean).join(' · ');
  const expandPlayer = () => navigate(`/watch/${playback.media.id}`);
  const collapsePlayer = () => {
    if (document.fullscreenElement) void document.exitFullscreen?.().catch(() => undefined);
    navigate(lastNonWatchPath);
  };
  const toggleBrowserFullscreen = () => {
    if (document.fullscreenElement) {
      void document.exitFullscreen?.().catch(() => undefined);
      return;
    }
    void surfaceRef.current?.requestFullscreen?.().catch(() => undefined);
  };
  const closePlayer = () => {
    if (document.fullscreenElement) void document.exitFullscreen?.().catch(() => undefined);
    if (full) navigate(lastNonWatchPath, { replace: true });
    void close();
  };

  const audioArtwork = resolvePlaybackAsset(playback.media.displayImages?.poster || playback.media.displayImages?.thumb || playback.media.displayImages?.backdrop || playback.media.images.poster || playback.media.images.thumb || playback.media.images.backdrop);
  const seriesTitle = playback.media.grandparentTitle;
  const identityTitle = seriesTitle || playback.media.title;
  const identitySubtitle = seriesTitle
    ? [playback.media.title, playback.media.seasonNumber != null && playback.media.episodeNumber != null ? `S${playback.media.seasonNumber} · E${playback.media.episodeNumber}` : ''].filter(Boolean).join(' · ')
    : subtitle || playback.media.tagline || mediaPresentation({ entityKind: playback.media.entityKind }).label;
  return <PlayerSubtitleProvider playback={playback} mediaRef={mediaRef}>
    <section ref={surfaceRef} className={`${effectiveFull ? 'player-full' : 'player-mini'} mode-${mode} subtitle-size-${preferences.subtitleSize} subtitle-background-${preferences.subtitleBackground}`} aria-label={productMessage('playback.now-playing', { title: playback.media.title }).text}>
      {full && <button type="button" className="player-collapse" aria-label={productMessage('action.collapse-player').text} onClick={collapsePlayer}><ProductLanguageIcon id="action.collapse" /></button>}
      <div ref={mediaStageRef} className="player-media-stage" style={{ backgroundImage: artwork ? `url(${artwork})` : undefined }} role={!effectiveFull ? 'button' : undefined} tabIndex={!effectiveFull ? 0 : undefined} aria-label={!effectiveFull ? productMessage('action.expand-player').text : undefined} onClick={!effectiveFull ? (event) => { if (event.target instanceof Element && event.target.closest('button')) return; expandPlayer(); } : undefined} onKeyDown={!effectiveFull ? (event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); expandPlayer(); } } : undefined}>
        {!completed && <video ref={mediaRef} autoPlay playsInline preload="auto" poster={mode === 'video' || mode === 'live' ? artwork : undefined} aria-label={playback.media.title}>
          {playback.subtitleStreams.filter((stream) => stream.sourceUrl).map((stream) => <track key={stream.id} kind="subtitles" src={resolvePlaybackAsset(stream.sourceUrl)} srcLang={stream.language} label={stream.displayTitle || stream.language || stream.codec} />)}
        </video>}
        {(mode === 'music' || mode === 'audiobook') && <div className="audio-artwork">
          <PlayerArtworkImage src={audioArtwork} fallback={<span className="audio-artwork-fallback"><ProductLanguageIcon id={mode === 'audiobook' ? 'status.audiobook' : 'status.music'} /></span>} />
        </div>}
        <PlayerSubtitleLayer />
        <PlayerStatusLayer />
        <PostPlaySurface onClose={closePlayer} autoplay={sessionAutoplay} />
      </div>
      <div className="player-copy">
        <PlayerIdentityArtwork src={identityArtwork} live={mode === 'live'} />
        <span className="player-copy-text"><strong>{identityTitle}</strong><span>{identitySubtitle}</span></span>
      </div>
      <PlayerControls full={effectiveFull} browserFullscreen={browserFullscreen} sessionAutoplay={sessionAutoplay} onSessionAutoplayChange={setSessionAutoplay} onToggleBrowserFullscreen={toggleBrowserFullscreen} onClose={closePlayer} />
    </section>
  </PlayerSubtitleProvider>;
}

function PlayerIdentityArtwork({ src, live }: { src?: string; live: boolean }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [src]);
  if (src && !failed) return <img className={`player-copy-art ${live ? 'player-copy-logo' : ''}`} src={src} alt="" onError={() => setFailed(true)} />;
  if (!live) return null;
  return <span className="player-copy-art player-copy-art-fallback" aria-hidden="true"><ProductLanguageIcon id="status.live-tv" /></span>;
}
