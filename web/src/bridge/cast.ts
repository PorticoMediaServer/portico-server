/** Sending playback to a Chromecast from Chrome or Edge: a thin web adapter over the shared
 * `CastController` (`@core/cast`), which owns the Portico receiver protocol (pairing by a
 * short-lived code, pair-pending/tv_busy, load with display metadata, takeover, replaced,
 * status for the controller only). This file only loads Google's sender SDK, decides whether
 * Cast can be offered here (an application id and an HTTPS server), and bridges a Cast
 * session to the controller's transport. */
import {CAST_NAMESPACE, CastController, type CastMetadata, type CastSnapshot, type CastTransport} from '@core/cast/index.ts';
import {noteCastMeta} from '@core/presentation/index.ts';

export type {CastMetadata, CastSnapshot};

/** How this browser introduces itself on the TV ("Chrome on Mac wants to play…"). No account details. */
export function castDisplayName(ua = typeof navigator === 'undefined' ? '' : navigator.userAgent): string {
  const browser = /Edg\//.test(ua) ? 'Edge' : /OPR\//.test(ua) ? 'Opera' : /Chrome\//.test(ua) ? 'Chrome' : /Firefox\//.test(ua) ? 'Firefox' : /Safari\//.test(ua) ? 'Safari' : 'A browser';
  const os = /CrOS/.test(ua) ? 'Chromebook' : /Mac OS X|Macintosh/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : /Android/.test(ua) ? 'Android' : /Linux/.test(ua) ? 'Linux' : '';
  return os ? `${browser} on ${os}` : browser;
}
const SDK = 'https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1';
type Api = {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>; mediaUrl(path: string): string};
/** Portico's own Cast application, whose receiver is the shared page at cast.getportico.tv. A
 * server's owner may set a different one (a self-hosted receiver); otherwise this is used. */
const PORTICO_CAST_APPLICATION = (import.meta.env?.VITE_CAST_APPLICATION_ID as string | undefined) ?? '';
type Listener = (ns: string, message: string) => void;
type CastSession = {getCastDevice(): {friendlyName: string}; sendMessage(ns: string, body: unknown): Promise<unknown>; addMessageListener(ns: string, fn: Listener): void; removeMessageListener(ns: string, fn: Listener): void; endSession(stop: boolean): void};
type Framework = {CastContext: {getInstance(): {setOptions(o: unknown): void; requestSession(): Promise<unknown>; getCurrentSession(): CastSession | null; addEventListener(type: string, fn: (e: {sessionState: string}) => void): void; removeEventListener(type: string, fn: (e: {sessionState: string}) => void): void}}; CastContextEventType: {SESSION_STATE_CHANGED: string}; SessionState: Record<string, string>};
declare global { interface Window { __onGCastApiAvailable?: (available: boolean) => void; cast?: {framework: Framework}; chrome?: {cast?: {AutoJoinPolicy: {ORIGIN_SCOPED: string}}} } }

const idle: CastSnapshot = Object.freeze({available: false, phase: 'off', positionSeconds: 0, durationSeconds: 0, paused: false, audioTracks: Object.freeze([]), textTracks: Object.freeze([])});

let sdk: Promise<boolean> | undefined;
function loadSdk(): Promise<boolean> {
  return sdk ??= new Promise(resolve => {
    if (typeof window === 'undefined' || !('chrome' in window)) { resolve(false); return; }
    if (window.cast?.framework) { resolve(true); return; }
    const timer = setTimeout(() => resolve(false), 8000);
    window.__onGCastApiAvailable = available => { clearTimeout(timer); resolve(available && !!window.cast?.framework); };
    const script = document.createElement('script');
    script.src = SDK; script.async = true; script.onerror = () => { clearTimeout(timer); resolve(false); };
    document.head.appendChild(script);
  });
}

export class CastSender {
  private api: Api;
  private controller?: CastController;
  private displayName: string;
  private prepared = false;
  constructor(api: Api, displayName = castDisplayName()) {
    this.api = api;
    this.displayName = displayName;
    // A Cast device loads nothing but HTTPS, so a server reached over plain HTTP on the home
    // network can't be cast from; that needs remote access (and its certificate) turned on.
    const origin = new URL(api.mediaUrl('/v1/system')).origin;
    if (origin.startsWith('https://')) this.controller = new CastController({api, origin, displayName: this.displayName});
  }
  getSnapshot = (): CastSnapshot => this.controller?.getSnapshot() ?? idle;
  subscribe = (fn: () => void) => this.controller?.subscribe(fn) ?? (() => {});

  /** Asks the server whether Cast is set up; only then is Google's sender script loaded. Runs once per sender (one per session). */
  async prepare(): Promise<void> {
    if (this.prepared) return;
    this.prepared = true;
    const controller = this.controller;
    if (!controller) return;
    try {
      const configuration = await this.api.request<{applicationId?: string}>('/v1/cast/configuration');
      const applicationId = (typeof configuration.applicationId === 'string' && configuration.applicationId) || PORTICO_CAST_APPLICATION;
      if (!applicationId || !(await loadSdk())) return;
      window.cast!.framework.CastContext.getInstance().setOptions({receiverApplicationId: applicationId, autoJoinPolicy: window.chrome?.cast?.AutoJoinPolicy.ORIGIN_SCOPED ?? 'origin_scoped'});
      controller.setAvailable(true);
      // CAST-05: after a reload Chrome rejoins a session that is still playing (origin-scoped
      // auto-join); take it back so the Cast bar returns and can control and stop the TV.
      const context = window.cast!.framework.CastContext.getInstance();
      const adopt = () => { const session = context.getCurrentSession(); if (session && controller.getSnapshot().phase === 'off') void controller.adopt(transportFor(session), id => this.api.request<{title?: unknown; posterUrl?: unknown}>(`/v1/items/${encodeURIComponent(id)}`).then(item => {
        // MU4 CAST-03: remember the adopted title's display facts for the controller sheet.
        if (typeof item?.title === 'string') noteCastMeta(id, {title: item.title, ...(typeof item.posterUrl === 'string' ? {artworkPath: item.posterUrl} : {})});
        return typeof item?.title === 'string' ? item.title : undefined;
      })); };
      adopt();
      const states = window.cast!.framework.SessionState;
      context.addEventListener(window.cast!.framework.CastContextEventType.SESSION_STATE_CHANGED, e => { if (e.sessionState === states.SESSION_RESUMED) adopt(); });
    } catch { /* Cast stays unoffered */ }
  }

  /** Resolves true once the television has started the title, so the local player can stop. */
  async start(itemId: string, meta: CastMetadata, startSeconds: number): Promise<boolean> {
    const controller = this.controller;
    if (!controller || !controller.getSnapshot().available) return false;
    controller.connecting(meta.title);
    const framework = window.cast!.framework;
    const context = framework.CastContext.getInstance();
    // A rejection here is the viewer closing Chrome's device picker: not an error.
    try { await context.requestSession(); } catch { controller.cancelConnecting(); return false; }
    const session = context.getCurrentSession();
    if (!session) { controller.cancelConnecting(); return false; }
    return controller.start(transportFor(session), itemId, meta, startSeconds);
  }
  toggle() { this.controller?.toggle(); }
  setAudioTrack(trackId: number) { this.controller?.setAudioTrack(trackId); }
  setSubtitleTrack(trackId: number | null) { this.controller?.setSubtitleTrack(trackId); }
  seek(positionSeconds: number) { this.controller?.seek(positionSeconds); }
  answerTakeover(allow: boolean) { this.controller?.answerTakeover(allow); }
  stop() { this.controller?.stop(); }
  dismiss() { this.controller?.dismiss(); }
  dispose() { this.controller?.dispose(); }
}

/** The Cast SDK session as the shared controller's transport. */
function transportFor(session: CastSession): CastTransport {
  const framework = window.cast!.framework;
  const context = framework.CastContext.getInstance();
  return {
    deviceName: session.getCastDevice().friendlyName,
    send: message => session.sendMessage(CAST_NAMESPACE, message).then(() => undefined),
    onMessage: fn => { const l: Listener = (_ns, raw) => fn(raw); session.addMessageListener(CAST_NAMESPACE, l); return () => session.removeMessageListener(CAST_NAMESPACE, l); },
    onSessionEnd: fn => {
      const l = (e: {sessionState: string}) => { if (e.sessionState === framework.SessionState.SESSION_ENDED || e.sessionState === framework.SessionState.SESSION_START_FAILED) fn(); };
      context.addEventListener(framework.CastContextEventType.SESSION_STATE_CHANGED, l);
      return () => context.removeEventListener(framework.CastContextEventType.SESSION_STATE_CHANGED, l);
    },
    end: stop => session.endSession(stop),
  };
}
