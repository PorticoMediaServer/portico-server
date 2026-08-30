import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import type { ServerViewerPreferenceBundle } from '@porticomediaserver/client-core';
import { useAuthSession, useLiveDataRevision, usePorticoDataSource } from '../data/DataProvider';
import {
  defaultWebDisplayPreferences,
  normalizeWebDisplayPreferences,
  type WebDisplayPreferences,
} from './webDisplayPreferences';
import { combineAbortSignals, timeoutSignal } from '../runtime/abortSignal';

type PreferenceScope = 'profile-server' | 'profile-device-class' | 'account-server-installation';

type WebDisplayPreferencesContextValue = {
  preferences: WebDisplayPreferences;
  bundle?: ServerViewerPreferenceBundle;
  status: 'loading' | 'ready' | 'error';
  busy: boolean;
  error?: Error;
  update: (next: WebDisplayPreferences) => Promise<void>;
  patch: (next: Partial<WebDisplayPreferences>) => Promise<void>;
  patchScope: (scope: PreferenceScope, changes: Record<string, unknown>) => Promise<void>;
  retry: () => void;
};

const WebDisplayPreferencesContext = createContext<WebDisplayPreferencesContextValue | null>(null);
const LOCAL_PREFERENCES_KEY = 'portico.web.installation-preferences.v1';
const PREFERENCE_REQUEST_DEADLINE_MS = 15_000;

function preferenceRequestSignal(signal: AbortSignal) {
  return combineAbortSignals([signal, timeoutSignal(PREFERENCE_REQUEST_DEADLINE_MS)]);
}

function installationPreferences() {
  try {
    const value = JSON.parse(localStorage.getItem(LOCAL_PREFERENCES_KEY) ?? '{}') as Record<string, unknown>;
    return {
      reduceMotion: typeof value.reduceMotion === 'boolean' ? value.reduceMotion : globalThis.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false,
      playbackDiagnostics: value.playbackDiagnostics === true,
    };
  } catch {
    return { reduceMotion: false, playbackDiagnostics: false };
  }
}

function viewPreferences(bundle: ServerViewerPreferenceBundle): WebDisplayPreferences {
  const server = bundle.profileServer.values;
  const device = bundle.effectiveProfileDeviceClass.values;
  const local = installationPreferences();
  return normalizeWebDisplayPreferences({
    showBackdrops: device.appearance.showBackdrops,
    cardSizePercent: device.appearance.cardSizePercent,
    sidebarCollapsed: device.navigation.sidebarCollapsed,
    pinnedLibraryIds: device.navigation.pinnedLibraryIds,
    homeRowOrder: server.home.rowOrder,
    hiddenHomeRows: server.home.hiddenRowIds,
    rememberSearchHistory: server.search.rememberHistory,
    recentSearches: server.search.recentQueries,
    skipBackSeconds: server.playback.skipBackSeconds,
    skipForwardSeconds: server.playback.skipForwardSeconds,
    autoplayNext: server.playback.autoplayNext,
    upNextCountdownSeconds: server.playback.upNextCountdownSeconds,
    subtitleSize: server.playback.subtitleSize,
    subtitleBackground: server.playback.subtitleBackground,
    showSyncedLyrics: server.playback.showSyncedLyrics,
		introSkip: server.playback.introSkip,
		creditsSkip: server.playback.creditsSkip,
    defaultPlaybackSpeed: String(server.playback.defaultSpeed),
    audioNormalizationMode: server.music.audioNormalization,
    ...local,
  });
}

function changedPreferences(current: WebDisplayPreferences, next: WebDisplayPreferences): Partial<WebDisplayPreferences> {
  return Object.fromEntries(Object.entries(next).filter(([key, value]) => JSON.stringify(current[key as keyof WebDisplayPreferences]) !== JSON.stringify(value))) as Partial<WebDisplayPreferences>;
}

function scopeChanges(next: Partial<WebDisplayPreferences>) {
  const server: Record<string, unknown> = {};
  const device: Record<string, unknown> = {};
  if ('showBackdrops' in next || 'cardSizePercent' in next) device.appearance = {
    ...('showBackdrops' in next ? { showBackdrops: next.showBackdrops } : {}),
    ...('cardSizePercent' in next ? { cardSizePercent: next.cardSizePercent } : {}),
  };
  if ('sidebarCollapsed' in next || 'pinnedLibraryIds' in next) device.navigation = {
    ...('sidebarCollapsed' in next ? { sidebarCollapsed: next.sidebarCollapsed } : {}),
    ...('pinnedLibraryIds' in next ? { pinnedLibraryIds: next.pinnedLibraryIds } : {}),
  };
  if ('homeRowOrder' in next || 'hiddenHomeRows' in next) server.home = {
    ...('homeRowOrder' in next ? { rowOrder: next.homeRowOrder } : {}),
    ...('hiddenHomeRows' in next ? { hiddenRowIds: next.hiddenHomeRows } : {}),
  };
  if ('rememberSearchHistory' in next || 'recentSearches' in next) server.search = {
    ...('rememberSearchHistory' in next ? { rememberHistory: next.rememberSearchHistory } : {}),
    ...(next.rememberSearchHistory === false
      ? { recentQueries: [] }
      : 'recentSearches' in next ? { recentQueries: next.recentSearches } : {}),
  };
  const playback: Record<string, unknown> = {};
  if ('skipBackSeconds' in next) playback.skipBackSeconds = next.skipBackSeconds;
  if ('skipForwardSeconds' in next) playback.skipForwardSeconds = next.skipForwardSeconds;
  if ('autoplayNext' in next) playback.autoplayNext = next.autoplayNext;
  if ('upNextCountdownSeconds' in next) playback.upNextCountdownSeconds = next.upNextCountdownSeconds;
  if ('subtitleSize' in next) playback.subtitleSize = next.subtitleSize;
  if ('subtitleBackground' in next) playback.subtitleBackground = next.subtitleBackground;
  if ('showSyncedLyrics' in next) playback.showSyncedLyrics = next.showSyncedLyrics;
	if ('introSkip' in next) playback.introSkip = next.introSkip;
	if ('creditsSkip' in next) playback.creditsSkip = next.creditsSkip;
  if ('defaultPlaybackSpeed' in next) playback.defaultSpeed = Number(next.defaultPlaybackSpeed);
  if (Object.keys(playback).length) server.playback = playback;
  if ('audioNormalizationMode' in next) server.music = { audioNormalization: next.audioNormalizationMode };
  return { server, device };
}

export function WebDisplayPreferencesProvider({ children }: { children: ReactNode }) {
  const source = usePorticoDataSource();
  const auth = useAuthSession();
  const [bundle, setBundle] = useState<ServerViewerPreferenceBundle>();
  const [preferences, setPreferences] = useState(defaultWebDisplayPreferences);
  const [status, setStatus] = useState<WebDisplayPreferencesContextValue['status']>('loading');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<Error>();
  const [revision, setRevision] = useState(0);
  const liveRevision = useLiveDataRevision(['display-preferences', 'settings']);
  const bundleRef = useRef<ServerViewerPreferenceBundle | undefined>(undefined);
  const preferencesRef = useRef(preferences);
  const writeChain = useRef<Promise<void>>(Promise.resolve());

  useEffect(() => { bundleRef.current = bundle; }, [bundle]);
  useEffect(() => { preferencesRef.current = preferences; }, [preferences]);

  const enqueueWrite = useCallback(<T,>(operation: () => Promise<T>): Promise<T> => {
    const scheduled = writeChain.current.catch(() => undefined).then(operation);
    writeChain.current = scheduled.then(() => undefined, () => undefined);
    return scheduled;
  }, []);

  const load = useCallback(async (signal: AbortSignal) => {
    const response = await source.viewerPreferences(preferenceRequestSignal(signal));
    bundleRef.current = response;
    setBundle(response);
    const nextPreferences = viewPreferences(response);
    preferencesRef.current = nextPreferences;
    setPreferences(nextPreferences);
    setError(undefined);
    setStatus('ready');
    return response;
  }, [source]);

  useEffect(() => {
    if (auth.status !== 'ready' || !auth.viewer?.authenticated || !auth.viewerScopeKey) {
      setStatus('loading');
      setBundle(undefined);
      return;
    }
    const controller = new AbortController();
    if (!bundle) setStatus('loading');
    load(controller.signal).catch((reason: unknown) => {
      if (controller.signal.aborted) return;
      if (!bundle) {
        setError(reason instanceof Error ? reason : new Error('Preferences could not be loaded.'));
        setStatus('error');
      }
    });
    return () => controller.abort();
  }, [auth.status, auth.viewer?.authenticated, auth.viewerScopeKey, revision, liveRevision, load]);

  const patchScope = useCallback(async (scope: PreferenceScope, changes: Record<string, unknown>) => {
    return enqueueWrite(async () => {
      const current = bundleRef.current;
      if (!current) throw new Error('Preferences are still loading.');
      const document = scope === 'profile-server' ? current.profileServer : scope === 'profile-device-class' ? current.profileDeviceClass : current.accountServerInstallation;
      const controller = new AbortController();
      setBusy(true);
      setError(undefined);
      try {
        await source.patchViewerPreference(scope, document.revision, changes, preferenceRequestSignal(controller.signal));
        await load(controller.signal);
      } catch (reason) {
        const nextError = reason instanceof Error ? reason : new Error('Preferences could not be saved.');
        setError(nextError);
        setStatus('error');
        if ((reason as { status?: number }).status === 409) await load(controller.signal).catch(() => undefined);
        throw nextError;
      } finally {
        setBusy(false);
      }
    });
  }, [enqueueWrite, load, source]);

  const patch = useCallback(async (next: Partial<WebDisplayPreferences>) => {
    return enqueueWrite(async () => {
      const current = bundleRef.current;
      if (!current) throw new Error('Preferences are still loading.');
      const previous = preferencesRef.current;
      const optimistic = normalizeWebDisplayPreferences({ ...previous, ...next });
      const { server, device } = scopeChanges(next);
      preferencesRef.current = optimistic;
      setPreferences(optimistic);
      setBusy(true);
      setError(undefined);
      const controller = new AbortController();
      try {
        let working = current;
        if (Object.keys(server).length) {
          const document = await source.patchViewerPreference('profile-server', working.profileServer.revision, server, preferenceRequestSignal(controller.signal));
          working = { ...working, profileServer: document as typeof working.profileServer };
        }
        if (Object.keys(device).length) {
          const document = await source.patchViewerPreference('profile-device-class', working.profileDeviceClass.revision, device, preferenceRequestSignal(controller.signal));
          working = { ...working, profileDeviceClass: document as typeof working.profileDeviceClass };
        }
        if ('reduceMotion' in next || 'playbackDiagnostics' in next) {
          try {
            localStorage.setItem(LOCAL_PREFERENCES_KEY, JSON.stringify({
              reduceMotion: optimistic.reduceMotion,
              playbackDiagnostics: optimistic.playbackDiagnostics,
            }));
          } catch {
            // Server preferences remain authoritative when browser storage is
            // blocked or over quota.
          }
        }
        await load(controller.signal);
      } catch (reason) {
        const nextError = reason instanceof Error ? reason : new Error('Preferences could not be saved.');
        let reconciled = false;
        try {
          await load(controller.signal);
          reconciled = true;
        } catch {
          preferencesRef.current = previous;
          setPreferences(previous);
        }
        setError(nextError);
        if (!reconciled) setStatus('error');
        throw nextError;
      } finally {
        setBusy(false);
      }
    });
  }, [enqueueWrite, load, source]);

  const value = useMemo<WebDisplayPreferencesContextValue>(() => ({
    preferences,
    bundle,
    status,
    busy,
    error,
    update: (next) => patch(changedPreferences(preferences, normalizeWebDisplayPreferences(next))),
    patch,
    patchScope,
    retry: () => setRevision((current) => current + 1),
  }), [bundle, busy, error, patch, patchScope, preferences, status]);

  return <WebDisplayPreferencesContext.Provider value={value}>{children}</WebDisplayPreferencesContext.Provider>;
}

export function useWebDisplayPreferences() {
  const value = useContext(WebDisplayPreferencesContext);
  if (!value) throw new Error('Web display preferences must be used inside WebDisplayPreferencesProvider.');
  return value;
}

export function useOptionalWebDisplayPreferences() {
  return useContext(WebDisplayPreferencesContext);
}
