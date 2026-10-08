import {trackName} from './presentation/language.ts';
import {randomId} from './random-id.ts';
import {routeOrigin} from './route-identity.ts';
import {sessionRoute,bindSessionRoute,type RouteConnection} from './route-connection.ts';
export * from './route-identity.ts';
export * from './identity-restrictions.ts';
export * from './identity-devices.ts';
export * from './account-notifications.ts';
export * from './route-connection.ts';
export * from './known-servers.ts';
export * from './portico-servers.ts';
import type {PreparedChoice} from './prepared-media.ts';
import {ListeningDeadlineClock,noListeningDeadlines,type NativeListeningStatus,type NativeListeningV1Control,type NativeListeningV1Event,type ListeningDeadlines,type ListeningPolicy} from './playback/native-listening.ts';
import {audioEffectsEnabled,defaultAudioEffects,parseAudioEffects,type AudioEffectsSnapshot,type AudioEffectsSettings,type PreparedAudio} from './audio-effects.ts';
import {playableAudio,type AudioRenderV2} from './playback-v1/audio-render.ts';
export * from './audio-effects.ts';
export * from './music-preferences.ts';
import {sessionEndMessage} from './session-end.ts';
import {readCertificateResponse, type CertificateAction} from './certificates.ts';
import {reportRouteFailure,type RouteFailureCode,type RouteFailureResult} from './client-profile.ts';
import {normalizeTransportClass,transportClassHeader,deviceClassHeader,deliveryContextHeaders,setDeliveryContext,type TransportClass} from './delivery.ts';
export * from './certificates.ts';
export * from './delivery.ts';
export * from './client-profile.ts';
import {readConsoleJson,consoleUTF8Size} from './console-transport.ts';
import {channelSeconds,channelMessage,type ChannelProjection,type ChannelReference} from './playback/linear-protocol.ts';
import type {ChannelPlayback,ChannelView} from './playback-v1/channel-control.ts';
export type {ChannelReference,ChannelProjection} from './playback/linear-protocol.ts';
import type {QueueController} from './queue-controller.ts';
import {readLyricsResponse} from "./lyrics-http.ts";
import {readBoundedJson} from './bounded-json.ts';
import {parseLocalSession} from './local-session.ts';
import {installationHeaders,issuesCredentials,provesIdentity} from './installation.ts';
import {AudioSelection,emptyAudio,type AudioPlan,type AudioBinding,type AudioSelectionSnapshot} from './audio-selection.ts';
export type {AudioPlan,AudioBinding,AudioSelectionSnapshot} from './audio-selection.ts';
/** Portable wire types and services. No renderer, native module or process globals. */
export type Viewer = {
  accountId: string;
  profileId: string;
  serverId: string;
  authority: string;
  role: string;
};
export type NativeServerIdentity = {publicKey:string;fingerprint:string};
export type LocalSession = {
  serverIdentity?: NativeServerIdentity;
  accessToken: string;
  expiresAt: string;
  viewer: Viewer;
  sessionFamilyId: string;
  tokenGeneration: string;
  authorizationHorizon: string;
  /** The device record this family is bound to (device-bound sessions). */
  deviceId?: string;
  /** The installation the family is bound to (C45: sent with every issued session; refresh names it). */
  installationId?: string;
  /** Rotating refresh credential (HTTPS only). Lives in protected storage; never in UI snapshots or the viewer record. */
  refreshToken?: string;
};
/** How a transport keeps its access token alive (`LocalSessionRefresher` implements it). */
export type AuthRecovery = Readonly<{
  /** A usable access token, refreshing first when `current` is about to expire. */
  token(current: string): Promise<string>;
  /** The server refused `rejected` (401): its replacement, or undefined when the sign-in has ended. */
  recover(rejected: string): Promise<string | undefined>;
}>;
export type Library = { id: string; name: string; kind: string; defaultView?: import('./library-content.ts').ContentView };
export type MediaItem = {
  id: string;
  libraryId: string;
  title: string;
  kind: string;
  year?: number;
  duration: number;
  overview?: string;
  posterUrl?: string;
  backdropUrl?: string;
  progressSeconds: number;
  available: boolean;
  sources?: {
    id: string;
    container: string;
    videoCodec: string;
    audioCodec: string;
    width: number;
    height: number;
  }[];
};
/** Whether a session's audio can go through the effects engine: a version 2 plan
 * the device decodes (`direct`/`converted`). */
export const renderable=(s?:{audio?:AudioRenderV2}):boolean=>playableAudio(s?.audio);
/** X-02: why effects are off for this session, said quietly (a version 2 `unavailable` plan, §18.1). */
export const renderNotice=(s:{audio?:AudioRenderV2}):string=>s.audio?.mode==='unavailable'?s.audio.reason??'':'';
export type PlaybackSession = {
  preparedVersionId?: string;
  /** §18.1 version 2: the client decodes the original file (Plexamp's model); `mode` says how. */
  audio?: AudioRenderV2;
  /** 'v1': a Playback Protocol v1 session, whose authority is the session resource itself (its
   * lease is renewed by the timeline, spec §5–§6). A local (downloaded) session has none. */
  protocol?: 'v1';
  id: string;
  generation: number;
  streamUrl: string;
  mode: string;
  duration: number;
  resumeSeconds: number;
};
export type SystemInfo = {
  id: string;
  name: string;
  setupRequired: boolean;
  /** This build can reach the Portico Account service (true for every release build). */
  hostedConfigured: boolean;
  /** This server signs people in with Portico Accounts. Absent from older servers. */
  hostedAttached?: boolean;
  version: string;
};
/** A server-named request field (`name`, `rules[0].query.libraryIds`), or undefined for anything else. */
/** CD-06: `error.confirmRoots`, kept only as a short list of plain http(s) URLs. */
export function errorConfirmRoots(value: unknown): readonly string[] | undefined {
  if (!Array.isArray(value) || value.length === 0 || value.length > 16) return undefined;
  const out: string[] = [];
  for (const v of value) {
    if (typeof v !== 'string' || v.length > 2048 || /[\x00-\x1f\x7f\s]/.test(v)) return undefined;
    try { const u = new URL(v); if ((u.protocol !== 'http:' && u.protocol !== 'https:') || u.username || u.password) return undefined; } catch { return undefined; }
    out.push(v);
  }
  return Object.freeze(out);
}
export function errorFieldPath(value: unknown): string | undefined {
  return typeof value === "string" && /^[A-Za-z][A-Za-z0-9_]*(?:\[\d{1,4}\]|\.[A-Za-z][A-Za-z0-9_]*){0,12}$/.test(value) && value.length <= 200 ? value : undefined;
}
/** RFC 9110 Retry-After: delay-seconds or an HTTP-date. Returns whole seconds from now, or undefined. */
export function retryAfterSecondsFrom(value: string | null, now = Date.now()): number | undefined {
  if (!value) return undefined;
  const trimmed = value.trim();
  if (/^\d+$/.test(trimmed)) { const n = Number(trimmed); return n > 0 ? n : undefined; }
  const at = Date.parse(trimmed);
  if (!Number.isFinite(at)) return undefined;
  const seconds = Math.ceil((at - now) / 1000);
  return seconds > 0 ? seconds : undefined;
}

/** The HTTP status an API failure carries, whichever API raised it (the v1 playback client has its own error class). */
function statusOf(error: unknown): number {
  const status = (error as { status?: unknown } | null)?.status;
  return typeof status === "number" ? status : 0;
}
export class ApiError extends Error {
  code: string;
  retryable: boolean;
  retryAfterSeconds?: number;
  /** Validated RFC3339 clock hint only; never retains the server error body. */
  serverTime?: string;
  status: number;
  /**
   * The request field the server rejected (`rules[0].query.kinds`), when it names one. A machine
   * field for highlighting, never shown as text; validated to a plain field path.
   */
  path?: string;
  /**
   * CD-06: the local-network roots a Live TV source preview needs the owner to confirm
   * (`source_lan_confirmation_required`), sent back in `confirmedLanRoots`. Validated plain URLs.
   */
  confirmRoots?: readonly string[];
  constructor(
    status: number,
    code: string,
    message: string,
    retryable = false,
    retryAfterSeconds?: number,
    serverTime?: string
  ) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.retryable = retryable;
    this.retryAfterSeconds = retryAfterSeconds;
    this.status = status;
    if (typeof serverTime === "string" && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(serverTime) && Number.isFinite(Date.parse(serverTime))) this.serverTime = serverTime;
  }
}
export interface PlaybackApi {
  /** Optional: tells the server the engine rejected the route of one session, so
   * the next plan for that file on this device takes the next route up. */
  reportRouteFailure?(sessionId: string, code: RouteFailureCode, detail: string): Promise<RouteFailureResult | null>;
  createPlayback(
    itemId: string,
    requestId: string,
    signal?: AbortSignal
  ): Promise<PlaybackSession>;
  stopPlayback(id: string, signal?: AbortSignal): Promise<void>;
  progressPlayback(
    id: string,
    payload: {
      generation: number;
      sequence: number;
      positionSeconds: number;
      state: "playing" | "paused" | "ended";
    }
  ): Promise<void>;
  /** Optional: tells the service when the server ended a session it is playing (an administrator
   * terminated it, it moved to another device, its lease lapsed), with the reason. */
  onServerEnded?(listener: (sessionId: string, end: SessionEndInfo) => void): () => void;
  /** Optional: a marker the viewer skipped (spec §4.2), reported as evidence with the timeline. */
  markerSkipped?(sessionId: string, skipped: Readonly<{markerId: string; mode: 'automatic' | 'manual'; positionMs: number}>): Promise<void>;
}
/** Why the server ended a playback, and the administrator's message when there is one. */
export type SessionEndInfo = Readonly<{ reason: string; message?: string }>;
/** Match Go's canonical JSON escaping for signed-claim wrapper endpoints. */
export function transportJSON(value:unknown):string {return JSON.stringify(value).replace(/[<>&\u2028\u2029]/g,c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'));}
export class HttpLocalApi implements PlaybackApi {
  readonly baseUrl: string;
  private token: string;
  private certificateSessionEpoch = 0;
  private certificateSessionListeners = new Set<() => void>();
  readonly getCertificateSessionEpoch = () => this.certificateSessionEpoch;
  readonly subscribeCertificateSession = (listener: () => void) => {
    this.certificateSessionListeners.add(listener);
    return () => { this.certificateSessionListeners.delete(listener); };
  };
  private fetcher: typeof fetch;
  private rawFetcher: typeof fetch;
  private routes?: RouteConnection;
  private routeListeners = new Set<() => void>();
  private routeUnsubscribe?: () => void;
  readonly subscribeRoute = (listener: () => void) => { this.routeListeners.add(listener); return () => { this.routeListeners.delete(listener); }; };
  readonly getRouteSnapshot = () => this.routes?.getSnapshot() ?? null;
  readonly routeFetch: typeof fetch = (input,init) => this.fetcher(input,init);
  getRouteConnection() { return this.routes; }
  useRoutes(routes: RouteConnection) {
    if (routes.logicalOrigin !== this.baseUrl) throw new Error('This route belongs to another logical server origin.');
    if (this.routes === routes) return;
    this.routeUnsubscribe?.(); this.routes = routes;
    this.fetcher = routes.fetch;
    this.routeUnsubscribe = routes.subscribe(() => { for (const listener of this.routeListeners) listener(); });
    if (this.token) bindSessionRoute(this.token,routes);
  }
  recoverRoute() { return this.routes?.recover() ?? Promise.reject(new Error('No remembered server routes are available.')); }

  constructor(
    baseUrl: string,
    token = "",
    fetcher: typeof fetch = (input, init) => globalThis.fetch(input, init)
  ) {
    this.baseUrl = routeOrigin(baseUrl);
    this.token = token;
    this.rawFetcher = fetcher;
    this.fetcher = (input,init) => this.rawFetcher(input,{...init,redirect:'error',credentials:'omit',referrerPolicy:'no-referrer'});
    const routes = sessionRoute(token); if (routes) this.useRoutes(routes);
  }
  /** What this device is connected over, for the server to resolve a network
   * class from. The client only declares a transport: the lane, its ceilings and
   * the quality rungs remain the server's decision, and an unset or unknown
   * value is a valid answer rather than a missing one. */
  private transportClass: TransportClass = 'unknown';
  private deviceClass = '';
  setTransportClass(value: string | null | undefined) {
    this.transportClass = normalizeTransportClass(value);
    // The playback control and queue transports read the shared copy.
    setDeliveryContext({ transportClass: this.transportClass });
  }
  getTransportClass(): TransportClass { return this.transportClass; }
  /** The surface this client renders on, which scopes its device-class
   * preferences. Unset means the server applies no device-class document. */
  setDeviceClass(value: 'web' | 'mobile' | 'television' | '') { this.deviceClass = value; setDeliveryContext({ deviceClass: value }); }
  getDeviceClass() { return this.deviceClass; }
  private deliveryHeaders(): Record<string, string> {
    return {
      ...deliveryContextHeaders(),
      ...(this.transportClass === 'unknown' ? {} : { [transportClassHeader]: this.transportClass }),
      ...(this.deviceClass ? { [deviceClassHeader]: this.deviceClass } : {}),
    };
  }
  setAccessToken(token: string) {
    if (this.token === token) return;
    this.token = token;
    if (this.routes && token) bindSessionRoute(token,this.routes);
    this.certificateSessionEpoch++;
    for (const listener of this.certificateSessionListeners) listener();
  }
  /** Fixed owner-certificate operations, selected-server credentials and bounded responses. */
  async requestCertificate<T>(action:CertificateAction,body:unknown,signal:AbortSignal):Promise<T>{
    const suffix={status:'',config:'/config',retry:'/retry'}[action];
    if(suffix===undefined)throw new Error('Invalid certificate operation.');
    const raw=action==='status'?undefined:JSON.stringify(body);
    if(raw!==undefined&&raw.length>2048)throw new Error('Certificate configuration is too large.');
    const token=this.token,epoch=this.certificateSessionEpoch;
    const bounded=AbortSignal.any([signal,AbortSignal.timeout(10000)]);
    const response=await this.fetcher(this.baseUrl+'/v1/networking/certificate'+suffix,{method:action==='status'?'GET':'POST',signal:bounded,redirect:'error',credentials:'omit',headers:{Accept:'application/json',...(raw===undefined?{}:{'Content-Type':'application/json'}),...(token?{Authorization:'Bearer '+token}:{})},...(raw===undefined?{}:{body:raw})});
    if(bounded.aborted||this.certificateSessionEpoch!==epoch){void response.body?.cancel().catch(()=>{});throw new Error('The server session changed.');}
    const value=await readCertificateResponse<T>(response,bounded);
    if(bounded.aborted||this.certificateSessionEpoch!==epoch)throw new Error('The server session changed.');
    return value;
  }
  /** GET-only bounded diagnostics transport; existing request behavior is unchanged. */
  async requestBounded<T>(path:string,maxBytes:number,signal:AbortSignal):Promise<T>{
    if(!Number.isSafeInteger(maxBytes)||maxBytes<1||maxBytes>1048576)throw new Error('Invalid response byte limit.');
    if(!path.startsWith('/v1/')||path.includes('?')||path.includes('#'))throw new Error('Use a fixed selected-server API path.');
    const token=this.token,fetcher=this.fetcher;
    const bounded=AbortSignal.any([signal,AbortSignal.timeout(10000)]);
    const response=await fetcher(this.baseUrl+path,{method:'GET',signal:bounded,headers:{Accept:'application/json',...(token?{Authorization:'Bearer '+token}:{})}});
    if(bounded.aborted||this.token!==token){void response.body?.cancel().catch(()=>{});throw new Error('Your server session changed. Reopen support.');}
    const value=await readBoundedJson<T>(response,maxBytes,bounded);
    if(bounded.aborted||this.token!==token)throw new Error('Your server session changed. Reopen support.');
    return value;
  }
  /** Bounded selected-server console transport, including paged reads and writes. */
  async requestConsole<T>(path:string, method:string, body:unknown, signal:AbortSignal):Promise<T>{
    if(!path.startsWith('/v1/')||path.includes('#')||path.includes('\\')||!['GET','POST','PATCH','PUT'].includes(method))throw new Error('Invalid console request.');
    const payload=body===undefined?undefined:JSON.stringify(body);
    if(payload!==undefined&&consoleUTF8Size(payload)>65536)throw new Error('Console request is too large.');
    const token=this.token,c=new AbortController();const abort=()=>c.abort();signal.addEventListener('abort',abort,{once:true});
    if(signal.aborted)c.abort();const timer=setTimeout(abort,10000);
    try{
      const response=await this.fetcher(this.baseUrl+path,{method,signal:c.signal,redirect:'error',credentials:'omit',headers:{Accept:'application/json',...(token?{Authorization:'Bearer '+token}:{}),...(payload===undefined?{}:{'Content-Type':'application/json'})},...(payload===undefined?{}:{body:payload})});
      if(c.signal.aborted||this.token!==token){void response.body?.cancel().catch(()=>{});throw new Error('The selected server session changed.');}
      const value=await readConsoleJson<T>(response,c.signal);
      if(c.signal.aborted||this.token!==token)throw new Error('The selected server session changed.');return value;
    }finally{clearTimeout(timer);signal.removeEventListener('abort',abort);}
  }
  /** Lyric resources never inherit authority from their digest or a cached URL. */
  async requestLyrics<T>(path:string, method='GET', body?:unknown, signal?:AbortSignal):Promise<T> {
    if(!/^\/v1\/(?:items\/[A-Za-z0-9_-]+\/lyrics(?:\/[A-Za-z0-9_/-]+)?|lyrics\/provider)(?:\?[^#]*)?$/.test(path)) throw new Error('Invalid lyric API path.');
    const token=this.token, controller=new AbortController();
    const abort=()=>controller.abort(); signal?.addEventListener('abort',abort,{once:true}); if(signal?.aborted)abort();
    const timer=setTimeout(abort,22000);
    try {
      const response=await this.fetcher(this.baseUrl+path,{method,signal:controller.signal,headers:{Accept:'application/json',...(body===undefined?{}:{'Content-Type':'application/json'}),...(token?{Authorization:'Bearer '+token}:{})},...(body===undefined?{}:{body:transportJSON(body)})});
      const value=await readLyricsResponse(response,controller.signal);
      if(this.token!==token||controller.signal.aborted)throw new Error('The lyric request belongs to an old session.');
      return value as T;
    } finally {clearTimeout(timer);signal?.removeEventListener('abort',abort);}
  }
  private auth?: AuthRecovery;
  /** Keep this transport's access token alive: refresh before expiry and once on a 401. */
  setAuthRecovery(auth: AuthRecovery | undefined) { this.auth = auth; }
  private send(path: string, method: string, body: unknown, signal: AbortSignal | undefined, token: string, extra?: Readonly<Record<string, string>>) {
    return this.fetcher(this.baseUrl + path, {
      method,
      signal,
      // Credential/profile proof exchanges must never follow an HTTP redirect.
      ...(/^\/v1\/(?:direct(?:\/|$)|auth\/|hosted\/(?:attach|profiles\/offline-select))/.test(path)?{redirect:'error' as const,credentials:'omit' as const}:{}),
      headers: {
        Accept: "application/json",
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...this.deliveryHeaders(),
        ...(issuesCredentials(path, method) ? installationHeaders() : {}),
        // Per-request protocol headers (If-Match, Idempotency-Key, merge-patch Content-Type); never Authorization.
        ...Object.fromEntries(Object.entries(extra ?? {}).filter(([k]) => k.toLowerCase() !== 'authorization')),
      },
      ...(body === undefined ? {} : { body: transportJSON(body) }),
    });
  }
  /** Send with a live token; on a 401 renew once and resend (the server did not act on it). */
  private async exchange(path: string, method: string, body: unknown, signal: AbortSignal | undefined, extra?: Readonly<Record<string, string>>): Promise<Response> {
    if (!path.startsWith("/v1/"))
      throw new Error("API path must remain on the selected server.");
    // Sign-in and refresh never refresh or retry themselves.
    const exchange = provesIdentity(path, method);
    let token = this.token;
    if (this.auth && token && !exchange) { try { token = await this.auth.token(token); } catch { /* send what we have; a 401 recovers below */ } }
    let response = await this.send(path, method, body, signal, token, extra);
    if (response.status === 401 && this.auth && token && !exchange && !signal?.aborted) {
      let next: string | undefined;
      try { next = await this.auth.recover(token); } catch { next = undefined; }
      if (next && next !== token) { void response.body?.cancel().catch(() => {}); response = await this.send(path, method, body, signal, next, extra); }
    }
    return response;
  }
  /**
   * Any status, with headers and the parsed body (playback v1's `V1Http` seam: 202, 412 `current`,
   * `ETag`, `Report-Every-Ms`). Rejects only when there is no response (offline, aborted).
   */
  async requestRaw(path: string, method: string, options: Readonly<{headers?: Readonly<Record<string, string>>; body?: unknown; signal?: AbortSignal}> = {}): Promise<{status: number; headers: Record<string, string>; body?: unknown}> {
    const response = await this.exchange(path, method, options.body, options.signal, options.headers);
    const headers: Record<string, string> = {};
    response.headers.forEach((value, key) => { headers[key] = value; });
    let body: unknown;
    if (response.status !== 204 && response.status !== 205 && response.status !== 304) {
      const text = await response.text();
      if (text) { try { body = JSON.parse(text); } catch { body = undefined; } }
    }
    return {status: response.status, headers, body};
  }
  async request<T>(
    path: string,
    method = "GET",
    body?: unknown,
    signal?: AbortSignal
  ): Promise<T> {
    // A 401 means the server did not act on the request, so one retry with a renewed token is safe for any method.
    const response = await this.exchange(path, method, body, signal);
    if (!response.ok) {
      let data: any;
      try {
        data = await response.json();
      } catch {}
      const error = new ApiError(
        response.status,
        data?.error?.code ?? data?.code ?? "request_failed",
        data?.error?.message ?? `Request failed (${response.status}).`,
        data?.error?.retryable ?? response.status >= 500,
        retryAfterSecondsFrom(response.headers.get("Retry-After")),
        data?.error?.serverTime
      );
      const path = errorFieldPath(data?.error?.path);
      if (path) error.path = path;
      const roots = errorConfirmRoots(data?.error?.confirmRoots);
      if (roots) error.confirmRoots = roots;
      throw error;
    }
    return response.status === 204 ? (undefined as T) : response.json();
  }
  system() {
    return this.request<SystemInfo>("/v1/system");
  }
  login(username: string, password: string) {
    return this.request<unknown>("/v1/sessions", "POST", {
      username,
      password,
    }).then(value=>parseLocalSession(value,{authority:'local'}));
  }
  setup(setupToken: string, username: string, password: string, name: string) {
    return this.request<unknown>("/v1/setup", "POST", {
      setupToken,
      username,
      password,
      name,
    }).then(value=>parseLocalSession(value,{authority:'local'}));
  }
  me() {
    return this.request<{ viewer: Viewer }>("/v1/me");
  }
  logout() {
    return this.request<void>("/v1/sessions/current", "DELETE");
  }
  libraries() {
    return this.request<{ items: Library[] }>("/v1/libraries");
  }
  items(libraryId = "", cursor = "", limit = 40) {
    const q = new URLSearchParams({ limit: String(limit) });
    if (libraryId) q.set("libraryId", libraryId);
    if (cursor) q.set("cursor", cursor);
    return this.request<{ items: MediaItem[]; nextCursor?: string }>(
      "/v1/items?" + q
    );
  }
  item(id: string) {
    return this.request<MediaItem>("/v1/items/" + encodeURIComponent(id));
  }
  reportRouteFailure(sessionId: string, code: RouteFailureCode, detail: string) {
    return reportRouteFailure((path, method, body) => this.request<unknown>(path, method, body), sessionId, code, detail);
  }
  createPlayback(itemId: string, requestId: string, signal?: AbortSignal) {
    return this.request<PlaybackSession>(
      "/v1/playback/sessions",
      "POST",
      { itemId, quality: "auto", requestId },
      signal
    );
  }
  stopPlayback(id: string, signal?: AbortSignal) {
    return this.request<void>(
      "/v1/playback/sessions/" + encodeURIComponent(id),
      "DELETE",
      undefined,
      signal
    );
  }
  progressPlayback(
    id: string,
    payload: {
      generation: number;
      sequence: number;
      positionSeconds: number;
      state: "playing" | "paused" | "ended";
    }
  ) {
    return this.request<void>(
      "/v1/playback/sessions/" + encodeURIComponent(id) + "/progress",
      "POST",
      payload
    );
  }
  mediaUrl(path: string) {
    if (this.routes) return this.routes.mediaURL(path);
    const u = new URL(path, this.baseUrl);
    if (u.origin !== this.baseUrl)
      throw new Error("Cross-origin media capability rejected.");
    return u.href;
  }
}
export type ViewerSnapshot = {
  generation: number;
  phase: "signedOut" | "ready";
  serverUrl?: string;
  session?: LocalSession;
  internetReachable: boolean | null;
  hostedReachable: boolean | null;
};
export class ViewerService {
  private snapshot: ViewerSnapshot = {
    generation: 0,
    phase: "signedOut",
    internetReachable: null,
    hostedReachable: null,
  };
  private listeners = new Set<() => void>();
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private publish(value: ViewerSnapshot) {
    this.snapshot = Object.freeze(value);
    for (const l of this.listeners) l();
  }
  /** `stored`: a sign-in this device saved earlier, shown before it is verified. Its access token
   * may have expired since, which the restore renews; it is never a reason to refuse it here. */
  select(serverUrl: string, session: LocalSession, options: {stored?: boolean} = {}) {
    const {refreshToken: _refresh, ...checked}=parseLocalSession(session,{},{stored:options.stored});
    this.publish({
      ...this.snapshot,
      generation: this.snapshot.generation + 1,
      phase: "ready",
      serverUrl,
      session:checked,
    });
  }
  /** The same viewer with a renewed access token: no new generation, so nothing viewer-scoped restarts. */
  rotate(session: LocalSession): boolean {
    const current = this.snapshot.session;
    if (!current || current.sessionFamilyId !== session.sessionFamilyId || (['accountId','profileId','serverId','authority','role'] as const).some(k => current.viewer[k] !== session.viewer[k])) return false;
    if (BigInt(session.tokenGeneration) < BigInt(current.tokenGeneration)) return false;
    const {refreshToken: _refresh, ...checked} = parseLocalSession(session);
    this.publish({ ...this.snapshot, session: checked });
    return true;
  }
  clear() {
    this.publish({
      generation: this.snapshot.generation + 1,
      phase: "signedOut",
      internetReachable: this.snapshot.internetReachable,
      hostedReachable: this.snapshot.hostedReachable,
    });
  }
  setConnectivity(
    internetReachable: boolean | null,
    hostedReachable: boolean | null
  ) {
    this.publish({ ...this.snapshot, internetReachable, hostedReachable });
  }
  isCurrent(generation: number) {
    return this.snapshot.generation === generation;
  }
}
export type ProgressCheckpoint = {
  generation: number;
  sequence: number;
  positionSeconds: number;
  state: "playing" | "paused" | "ended";
};
export type PlaybackJournalEntry = {
  id?: string;
  request?: { itemId: string; requestId: string };
  checkpoint?: ProgressCheckpoint;
};
/** Adapter must scope the protected journal to the selected server/account/profile. */
export interface PlaybackJournal {
  load(): Promise<PlaybackJournalEntry[]>;
  save(entries: PlaybackJournalEntry[]): Promise<void>;
}
export type PlaybackOptions = {
  /** Live TV and Library Channels on v1 sessions (spec §18.6): V1ChannelControl. */
  channels?: ChannelPlayback;
  journal?: PlaybackJournal;
  deferRecovery?: boolean;
  startupTimeoutMs?: number;
  operationTimeoutMs?: number;
  seekTimeoutMs?: number;
  audioTimeoutMs?: number;
  maxPendingCleanup?: number;
  queueRequired?: boolean;
  /** Development builds: log why a start timed out (see setStartupProbe). */
  diagnostics?: boolean;
  /**
   * Web: whether the page is hidden. Browsers defer loading media in a hidden page, so a start
   * there can't become ready; the startup window then waits for the page to be shown and starts
   * again, instead of failing the play (O4, 24 Sep).
   */
  pageVisibility?: PageVisibility;
};
/** A hidden page and a way to hear when it is shown again (web `document.visibilityState`). */
export type PageVisibility = {hidden(): boolean; onVisible(listener: () => void): () => void};
/** One selectable audio track of the playing source. */
export type AudioTrackOffer = Readonly<{streamIndex:number;label:string;language?:string;channels?:number;codec:string}>;
export type AudioTracksSnapshot = Readonly<{tracks:readonly AudioTrackOffer[];selected:number|null;pending?:number}>;
/** A readable name for a track the file did not name. */
export function audioTrackLabel(t:{title?:string;language?:string;channels?:number;codec:string},ordinal:number):string{
  const layout=t.channels===undefined||t.channels<1?'':t.channels===1?'Mono':t.channels===2?'Stereo':t.channels===6?'5.1':t.channels===8?'7.1':t.channels+' ch';
  const codec=({aac:'AAC',ac3:'Dolby Digital',eac3:'Dolby Digital Plus',truehd:'Dolby TrueHD',dts:'DTS',flac:'FLAC',opus:'Opus',vorbis:'Vorbis',mp3:'MP3',alac:'ALAC',pcm:'PCM'} as Record<string,string>)[t.codec]??t.codec.toUpperCase();
  // The language leads; the track's own title follows only when it says more than the language (presentation/language.ts).
  return [trackName(t)||('Track '+ordinal),[codec,layout].filter(Boolean).join(' ')].filter(Boolean).join(' · ');
}
export type PlaybackSnapshot = {
  audioTracks?: AudioTracksSnapshot;
  nativeListening?: NativeListeningStatus;
  listeningDeadlines?: ListeningDeadlines;
  audioEffects: AudioEffectsSnapshot;
  channel?: ChannelReference;
  linear?: ChannelProjection;
  /** The channel's start can't proceed (occurrence status "recoverable"); `linear.media.errorCode` says why. */
  linearStalled?: boolean;
  audio: AudioSelectionSnapshot;
  revision: number;
  intentId: number;
  seekRevision: number;
  pendingSeek?: Readonly<{revision: number; positionSeconds: number}>;
  seekError?: string;
  failedSeek?: Readonly<{revision: number; positionSeconds: number}>;
  /** The player ran out of buffered media and is trying to get more. A status, never an error:
   * playback is only ended by `fail`, on a definite refusal or after recovery is exhausted. */
  recovering?: boolean;
  phase: "idle" | "starting" | "ready" | "ended" | "error";
  intent: "playing" | "paused";
  itemId?: string;
  session?: PlaybackSession;
  positionSeconds: number;
  duration: number;
  error?: string;
  /** The server ended this playback, and why: "terminated" by an administrator (with their
   * message), "transferred", "lease_expired", "stopped"… `phase` is "error" and `error` is the
   * viewer's text for it (`sessionEndMessage`); nothing advances. Cleared by the next start. */
  ended?: SessionEndInfo;
  cleanupPending: number;
  cleanupError?: string;
};
export interface PlaybackAdapter {
  apply(snapshot: PlaybackSnapshot): void;
}
/** UI intent is the sole command authority. Native events report facts, never autoplay intent. */
export class PlaybackService {
  private channels?: ChannelPlayback;
  private channelWantedState:'playing'|'paused'='playing';
  private channelSeekId?: string;
  private channelObservedSeek?: string;
  private channelSubmittedSeek?: string;
  private channelReference?: ChannelReference;
  private audioSelection: AudioSelection;
  private queue?: QueueController;
  private queueConnection:{error?:string;retry?:()=>void}={};
  private api: PlaybackApi;
  /** A Playback v1 audio session the native listening owner plays (Apple, plan §9.3 C3): it
   * reports the timeline and moves the queue; this service only forwards intents to it. */
  private nativeV1?: NativeListeningV1Control;
  private listeningPolicy?:ListeningPolicy;
  private nativePolicyRevision=0;private nativePolicyKey="";
  private listeningClock=new ListeningDeadlineClock(()=>{this.pause();this.update({listeningDeadlines:this.listeningClock.getSnapshot(),error:'Listening timer paused playback.'});});
  private options: PlaybackOptions;
  private snapshot: PlaybackSnapshot = {
    audioEffects: {settings:defaultAudioEffects,rendering:false,rate:1,notice:""},
    audio: emptyAudio(),
    revision: 0,
    intentId: 0,
    seekRevision: 0,
    phase: "idle",
    intent: "paused",
    positionSeconds: 0,
    duration: 0,
    cleanupPending: 0,
  };
  private listeners = new Set<() => void>();
  private adapter?: PlaybackAdapter;
  private sequence = 0;
  private endedSeekPosition?: number;
  private failedRecovery?: { positionSeconds?: number; intent: "playing" | "paused" };
  /** The server's music preferences have been adopted: the device journal no longer overrides them. */
  private effectsFromServer = false;
  private lastPlay?: { itemId: string; prepared?: PreparedChoice; quality?: string; audioStream?: number; local?: () => PlaybackSession };
  private escalations = 0;
  private seekTimer?: ReturnType<typeof setTimeout>;
  private hasObserved = false;
  private bookmarkProtected = true;
  private lastProgress = 0;
  private creates = new Map<string, { itemId: string; running: boolean }>();
  private cleanup = new Map<
    string,
    { running: boolean; checkpoint?: ProgressCheckpoint; work?: Promise<void> }
  >();
  private progressPending = new Map<string, ProgressCheckpoint>();
  private progressRunning = new Set<string>();
  private requestId: () => string;
  private startupTimer?: ReturnType<typeof setTimeout>;
  private startupWait?: () => void;
  private journalTail = Promise.resolve();
  private restored?: Promise<void>;
  constructor(
    api: PlaybackApi,
    requestId: () => string = () => randomId(),
    options: PlaybackOptions = {}
  ) {
    this.api = api;
    this.requestId = requestId;
    this.options = options;
    this.channels=options.channels;
    this.audioSelection = new AudioSelection(audio => this.update({audio}), () => this.snapshot, options.audioTimeoutMs);
    if (options.seekTimeoutMs !== undefined && (!Number.isFinite(options.seekTimeoutMs) || options.seekTimeoutMs < 1 || options.seekTimeoutMs > 60000)) throw new Error("Invalid seek timeout");
    api.onServerEnded?.((id, end) => this.serverEnded(id, end));
    if (!options.deferRecovery) this.startRecovery();
  }
  /** The server ended a session: an old one is dropped from the progress backlog; the current one
   * stops, telling the viewer why (an administrator's message as written). A session the engine
   * already finished is completion, unless an administrator ended it. */
  private serverEnded(sessionId: string, end: SessionEndInfo) {
    this.progressPending.delete(sessionId);
    const s = this.snapshot;
    if (s.session?.id !== sessionId || s.phase === "idle" || (s.phase === "ended" && end.reason !== "terminated")) return;
    this.fail(s.intentId, sessionEndMessage(end), { ended: Object.freeze({ reason: end.reason, message: end.message }) });
  }
  /** Explicit committed startup for React owners; safe to call repeatedly. */
  startRecovery(): Promise<void> {
    if (this.restored) return this.restored;
    this.restored = this.options.journal
      ? this.options.journal
          .load()
          .then((entries) => {
            for (const entry of entries) {
              if (entry.id && !this.cleanup.has(entry.id))
                this.cleanup.set(entry.id, {
                  running: false,
                  checkpoint: entry.checkpoint,
                });
              if (entry.request)
                this.creates.set(entry.request.requestId, {
                  itemId: entry.request.itemId,
                  running: false,
                });
            }
            this.update({});
            this.retryCleanup();
          })
          .catch(() => {
            this.update({
              cleanupError:
                "Playback recovery could not be read. Retry before starting another session.",
            });
          })
      : Promise.resolve();
    return this.restored;
  }

  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  attachAdapter(adapter: PlaybackAdapter) {
    this.adapter = adapter;
    adapter.apply(this.snapshot);
    return () => {
      if (this.adapter === adapter) this.adapter = undefined;
    };
  }
  private update(patch: Partial<PlaybackSnapshot>) {
    // `ended` belongs to one session: a new start or another session clears it.
    if (!("ended" in patch) && this.snapshot.ended && (patch.phase === "starting" || ("session" in patch && patch.session?.id !== this.snapshot.session?.id)))
      patch = { ...patch, ended: undefined };
    this.snapshot = Object.freeze({
      ...this.snapshot,
      ...patch,
      revision: this.snapshot.revision + 1,
      cleanupPending: this.cleanup.size + this.creates.size,
    });
    this.adapter?.apply(this.snapshot);
    for (const listener of this.listeners) listener();
  }
  private presentationAudioPreference?:string;
  /** Same session and occurrence; only committed transport generation changes.
   * Current intent, speed (adapter-owned), pending seek and source position win
   * over the older position used to prepare the replacement. */
  installSubtitlePresentation(p:import('./subtitles.ts').SubtitlePresentation):boolean {
    const before=this.snapshot,session=before.session;
    if(!session||p.sessionId!==session.id||p.generation<session.generation||!['starting','ready'].includes(before.phase))return false;
    if(p.generation===session.generation)return p.streamUrl===session.streamUrl&&p.mode===session.mode;
    if(!Number.isSafeInteger(p.generation)||!/^\/v1\/media\/[A-Za-z0-9_-]+(?:\/master\.m3u8|\/subtitle-video\/[A-Za-z0-9_-]+\/master\.m3u8)?$/.test(p.streamUrl))return false;
    const position=before.pendingSeek?.positionSeconds??before.positionSeconds;
    const oldPlan=before.audio.plan;
    this.presentationAudioPreference=before.audio.pending?.renditionId??before.audio.observedRenditionId??undefined;
    this.clearStartup();this.bookmarkProtected=true;this.hasObserved=false;
    const audio=this.audioSelection.reset();
    this.requestSeek(position,{session:{...session,generation:p.generation,streamUrl:p.streamUrl,mode:p.mode,resumeSeconds:position},phase:'starting',audio,error:undefined});
    if(oldPlan&&p.mode==='hls')this.audioSelection.install(before.intentId,{...oldPlan,generation:p.generation});
    return true;
  }
  /** ListeningService is the sole policy producer; render preferences join the
   * same snapshot before crossing the native boundary. Identical observations
   * never reset native deadlines or native-only remote-control activity. */
  setListeningPolicy(value:ListeningPolicy){this.listeningPolicy=value;this.syncListeningPolicy();}
  private syncListeningPolicy(){
    if(!this.listeningPolicy)return;
    const value={...this.listeningPolicy,effects:this.snapshot.audioEffects.settings},key=JSON.stringify(value);
    if(key===this.nativePolicyKey)return;this.nativePolicyKey=key;
    const revision=++this.nativePolicyRevision,owner=this.snapshot.nativeListening?.owner;
    const configured=this.nativeV1?.configure?.({...value,revision});
    void configured?.catch(()=>{
      // A failed bridge policy must not leave audible playback behind a timer
      // the UI assumes is armed. A late error cannot pause a different owner.
      if(revision!==this.nativePolicyRevision||this.snapshot.nativeListening?.owner!==owner)return;
      this.pause();this.audioEffectsNotice('Listening policy could not be applied. Playback paused; reopen it before relying on background timers.');
    });
  }
  isNativeListening(){return !!this.snapshot.nativeListening||!!this.nativeV1;}
  async setListeningDeadline(kind:'sleep'|'passout',minutes:number|null){
    if(minutes!==null&&(!Number.isFinite(minutes)||minutes<=0||minutes>1440))throw new Error('Choose a listening timer between 1 minute and 24 hours.');
    const key=kind==='sleep'?'sleepAtMs':'passoutAtMs',value={...(this.snapshot.listeningDeadlines??noListeningDeadlines),[key]:minutes===null?null:Date.now()+Math.round(minutes*60000)};
    if(this.nativeV1){await this.nativeV1.deadlines(value);return;}
    this.listeningClock.set(value);
    this.update({listeningDeadlines:value});
  }
  checkListeningDeadlines(){if(!this.nativeV1)this.listeningClock.check();}
  systemListeningAction(action:'next'|'previous'){return this.nativeV1?.advance(action)??Promise.resolve();}
  setPlaybackRate(rate:number){
    if(!Number.isFinite(rate)||rate<.25||rate>4)return;
    // AVAudioUnitTimePitch is controlled by the native owner. A JS seek here
    // would duplicate its intent and restart a PCM window at the old epoch.
    if(this.nativeV1){void this.nativeV1.action('rate',rate).catch(()=>{});return;}
    this.queue?.seekRetiringAudio?.(this.snapshot.positionSeconds);this.queue?.invalidateAudioPreparation?.();
    this.update({audioEffects:{...this.snapshot.audioEffects,rate}});
    if(this.snapshot.audioEffects.rendering&&this.snapshot.session)this.requestSeek(this.snapshot.positionSeconds);
  }
  /** The native v1 owner re-reads its session (the app returned to the foreground). */
  refreshPlaybackAuthority(){return this.nativeV1?.refresh()??Promise.resolve();}
  installAudioPlan(intentId:number,plan:AudioPlan){return this.audioSelection.install(intentId,plan);}
  invalidateAudioPlan(intentId:number,reason:string){this.audioSelection.invalidate(intentId,reason);}
  audioUnavailable(intentId:number,reason:string){this.audioSelection.unavailable(intentId,reason);}
  attachAudioEngine(binding:AudioBinding){return this.audioSelection.attach(binding);}
  detachAudioEngine(binding:AudioBinding,epoch:number){this.audioSelection.detach(binding,epoch);}
  audioMapped(binding:AudioBinding,epoch:number,ids:readonly string[]){const mapped=this.audioSelection.mapped(binding,epoch,ids);const preferred=this.presentationAudioPreference;if(mapped&&preferred&&ids.includes(preferred)){this.presentationAudioPreference=undefined;this.audioSelection.select(preferred);}return mapped;}
  selectAudio(id:string){return this.audioSelection.select(id);}
  /** The session's audio tracks, for a stream that carries one track at a time (an original, a
   * repackaged or converted stream without alternate renditions). Choosing one asks for the title
   * again from the same position with that track named; the server picks the delivery that reaches
   * it, which may differ from the current one. */
  installAudioTracks(intentId:number,tracks:readonly AudioTrackOffer[],selected:number|null){
    if(intentId!==this.snapshot.intentId||!this.snapshot.session)return;
    const clean=tracks.filter(t=>Number.isInteger(t.streamIndex)&&t.streamIndex>=0&&t.streamIndex<=65535).slice(0,64);
    // The track this play asked for by name is the one in use; otherwise the caller's (the file's default).
    const asked=this.lastPlay?.itemId===this.snapshot.itemId?this.lastPlay?.audioStream:undefined;
    if(asked!==undefined&&clean.some(t=>t.streamIndex===asked))selected=asked;
    this.update({audioTracks:clean.length>1?Object.freeze({tracks:Object.freeze(clean.map(t=>Object.freeze({...t}))),selected}):undefined});
  }
  selectAudioTrack(streamIndex:number):Promise<void>{
    const tracks=this.snapshot.audioTracks,itemId=this.snapshot.itemId,last=this.lastPlay;
    if(!tracks||!itemId||last?.local||tracks.pending!==undefined||tracks.selected===streamIndex||!tracks.tracks.some(t=>t.streamIndex===streamIndex))return Promise.resolve();
    const position=this.snapshot.pendingSeek?.positionSeconds??this.snapshot.positionSeconds;
    this.update({audioTracks:Object.freeze({...tracks,pending:streamIndex})});
    return this.play(itemId,position,undefined,last?.itemId===itemId?last.quality:undefined,streamIndex);
  }
  audioObserved(binding:AudioBinding,epoch:number,sequence:number,id:string|null){return this.audioSelection.observed(binding,epoch,sequence,id);}
  audioApplied(binding:AudioBinding,epoch:number,revision:number,id:string){return this.audioSelection.applied(binding,epoch,revision,id);}
  audioRenditionUnavailable(binding:AudioBinding,epoch:number,id:string){return this.audioSelection.renditionUnavailable(binding,epoch,id);}
  audioFailed(binding:AudioBinding,epoch:number,revision:number,reason:string,unavailable=false){return this.audioSelection.failed(binding,epoch,revision,reason,unavailable);}
  /** The device's cached effects (the queue journal): used only until the server's music
   * preferences have been adopted, and offline (X-02). */
  restoreAudioEffects(settings:AudioEffectsSettings){if(this.effectsFromServer)return;this.update({audioEffects:{...this.snapshot.audioEffects,settings:parseAudioEffects(settings)}});this.syncListeningPolicy();}
  /** X-02: the viewer's music preferences from the server (`musicAudioEffects`), applied on
   * sign-in, profile selection and every preference change. They win over the device's cached
   * copy, which they also refresh. The same settings again change nothing. */
  adoptAudioEffectsPreference(settings:AudioEffectsSettings){
    settings=parseAudioEffects(settings);this.effectsFromServer=true;
    const current=this.snapshot.audioEffects.settings;
    if(current.gapless===settings.gapless&&current.crossfadeSeconds===settings.crossfadeSeconds&&current.normalization===settings.normalization)return;
    this.setAudioEffects(settings);
  }
  setAudioEffects(settings:AudioEffectsSettings){
    settings=parseAudioEffects(settings);if(!this.nativeV1){this.queue?.seekRetiringAudio?.(this.snapshot.positionSeconds);this.queue?.invalidateAudioPreparation?.();}
    const current=this.snapshot,effects=current.audioEffects;
    const enable=renderable(current.session)&&audioEffectsEnabled(settings);
    this.update({audioEffects:{...effects,settings,rendering:effects.rendering||enable,notice:''}});
    this.syncListeningPolicy();
    if(enable&&!effects.rendering&&current.session&&!this.nativeV1)this.requestSeek(current.positionSeconds);
    void this.queue?.saveAudioEffects(settings).catch(()=>this.audioEffectsNotice('These audio settings could not be saved on this device.'));
  }
  audioRenderStatus(effectiveRate:number,waiting:boolean){
    if(!Number.isFinite(effectiveRate)||effectiveRate<0||effectiveRate>4)return;
    const effects=this.snapshot.audioEffects;
    if(!effects.rendering||effects.effectiveRate===effectiveRate&&effects.waiting===waiting)return;
    this.update({audioEffects:{...effects,effectiveRate,waiting}});
  }
  audioEffectsNotice(notice:string){if(this.snapshot.audioEffects.notice!==notice)this.update({audioEffects:{...this.snapshot.audioEffects,notice}});}
  prepareAudio(prepared:PreparedAudio){this.update({audioEffects:{...this.snapshot.audioEffects,prepared,notice:''}});}
  clearPreparedAudio(){const e=this.snapshot.audioEffects;if(e.prepared||e.committed)this.update({audioEffects:{...e,prepared:undefined,committed:undefined}});}
  audioPrepared(token:string){this.queue?.audioPrepared(token);}
  audioCommit(token:string){void this.queue?.commitAudio(token);}
  audioBoundary(token:string,position:number){void this.queue?.audioBoundary(token,position);}
  audioPreparationFailed(message:string){this.queue?.discardAudio(message);}
  audioRenderFailed(message:string){this.queue?.audioRenderFailed(message);}
  async commitAudioAuthority(token:string,itemId:string,session:PlaybackSession,expectedIntent:number):Promise<boolean>{
    if(this.snapshot.intentId!==expectedIntent)return false;
    // The server has committed the next session (§18.3); the bridge reports its timeline from the
    // boundary on. Buffered outgoing samples may still drain, but they write no progress.
    this.bookmarkProtected=true;
    this.update({audioEffects:{...this.snapshot.audioEffects,committed:{token,itemId,session}}});return true;
  }
  adoptAudioBoundary(token:string,position:number):boolean{
    const next=this.snapshot.audioEffects.committed;if(!next||next.token!==token||!Number.isFinite(position)||position<0)return false;
    this.clearStartup();this.clearSeek();this.hasObserved=true;this.bookmarkProtected=false;this.sequence=0;this.lastProgress=position;
    this.update({intentId:this.snapshot.intentId+1,session:next.session,itemId:next.itemId,phase:'ready',positionSeconds:position,duration:next.session.duration,pendingSeek:undefined,failedSeek:undefined,seekError:undefined,audio:this.audioSelection.reset(),audioEffects:{...this.snapshot.audioEffects,prepared:undefined,committed:undefined,rendering:true}});
    this.checkpoint(this.snapshot.intent);void this.persist();return true;
  }
  private clearStartup() {
    if (this.startupTimer) clearTimeout(this.startupTimer);
    this.startupTimer = undefined;
    this.startupWait?.();
    this.startupWait = undefined;
  }
  /**
   * The startup window: `expire` runs when a start hasn't become ready in time. While the page
   * is hidden (web), the window doesn't expire; it waits for the page to be shown and starts a
   * full window again, since a browser loads no media in a hidden page.
   */
  private armStartup(expire: () => void) {
    const ms = this.options.startupTimeoutMs ?? 20000;
    const arm = () => {
      this.startupTimer = setTimeout(() => {
        this.startupTimer = undefined;
        const visibility = this.options.pageVisibility;
        if (visibility?.hidden()) {
          this.startupWait?.();
          this.startupWait = visibility.onVisible(() => { this.startupWait?.(); this.startupWait = undefined; arm(); });
          return;
        }
        expire();
      }, ms);
      (this.startupTimer as any)?.unref?.();
    };
    arm();
  }
  /** Channel selection uses this same service/adapter owner, but never a VOD
   * create, persisted catalog placeholder, resume checkpoint or chapter query. */
  async playChannel(reference:ChannelReference):Promise<void> {
    const oldVOD=this.snapshot.channel?undefined:this.snapshot.session?.id;
    this.leave();this.channelReference=reference;this.channelWantedState='playing';
    const intentId=this.snapshot.intentId;
    this.channelSeekId=undefined;this.channelObservedSeek=undefined;this.channelSubmittedSeek=undefined;
    this.update({channel:reference,phase:'starting',intent:'playing',itemId:reference.channelId});
    if(!this.channels){this.update({phase:'error',error:'Channel playback is not configured on this client.'});return;}
    // Release the exact prior VOD before live admission. This is resource
    // cleanup, not a predicted capacity check or a profile-wide singleton.
    if(oldVOD){try{await this.persist();await this.tryCleanup(oldVOD);}catch{
      if(this.snapshot.intentId===intentId)this.update({phase:'error',error:'The previous playback could not be stopped. Retry this channel when the server reconnects.'});return;
    }}
    if(this.snapshot.intentId!==intentId||!this.snapshot.channel)return;
    await this.channels.open(reference,{
      state:value=>{if(this.snapshot.intentId===intentId&&this.snapshot.channel)this.applyChannel(value);},
      error:(message,terminal)=>{if(this.snapshot.intentId!==intentId||!this.snapshot.channel)return;this.update({error:message,...(terminal?{phase:'error' as const,session:undefined,pendingSeek:undefined}:{} )});}
    });
  }
  /** Channels on v1 sessions (spec §18.6): the apps switch to this when the server offers them. */
  useChannelPlayback(control:ChannelPlayback){if(this.snapshot.channel)this.leave();this.channels=control;}
  private applyChannel(state:ChannelView){
    const linear=state.linear,m=linear.media,seek=linear.desired.seek;
    const changed=this.snapshot.session?.generation!==Number(m.bufferGeneration);
    let patch:Partial<PlaybackSnapshot>={linear,channel:linear.channel,linearStalled:state.status==='recoverable',intent:this.channelWantedState==='paused'?'paused':linear.desired.state,error:m.errorCode?channelMessage(m.errorCode):undefined};
    // No engine attachment until an ordered, server-resolved join target exists.
    if(m.streamUrl&&seek&&seek.bufferGeneration===m.bufferGeneration&&seek.positionUs!==null){
      const target=channelSeconds(seek.positionUs);
      if(!this.snapshot.session||changed){
        const generation=Number(m.bufferGeneration);if(!Number.isSafeInteger(generation)){this.fail(this.snapshot.intentId,'Channel generation is not supported.');return;}
        patch.session={id:state.playbackId,generation,streamUrl:m.streamUrl,mode:'hls',duration:0,resumeSeconds:target};
        patch.phase='starting';patch.duration=0;this.channelSeekId=undefined;this.channelObservedSeek=undefined;this.channelSubmittedSeek=undefined;
      }else if(this.snapshot.phase==='error')patch.phase='error';
      if(seek.seekCommandId!==this.channelSeekId){
        this.clearSeek();this.channelSeekId=seek.seekCommandId;this.channelSubmittedSeek=undefined;
        const revision=this.snapshot.seekRevision+1,intent=this.snapshot.intentId;
        patch.seekRevision=revision;patch.pendingSeek=Object.freeze({revision,positionSeconds:target});patch.failedSeek=undefined;patch.seekError=undefined;
        this.seekTimer=setTimeout(()=>this.seekFailed(intent,revision,'That position is no longer available. Choose Go Live or another position.'),this.options.seekTimeoutMs??15000);
        (this.seekTimer as any)?.unref?.();
      }
      if(linear.acknowledgedSeekCommandId===seek.seekCommandId&&this.channelSubmittedSeek===seek.seekCommandId&&this.channelObservedSeek!==seek.seekCommandId&&!linear.seekError){
        this.channelObservedSeek=seek.seekCommandId;this.clearSeek();patch.pendingSeek=undefined;patch.seekError=undefined;patch.failedSeek=undefined;
        if(linear.confirmedPositionUs!==null)patch.positionSeconds=channelSeconds(linear.confirmedPositionUs);
      }
    }else if(changed&&this.snapshot.session){
      this.clearSeek();patch.session=undefined;patch.pendingSeek=undefined;patch.phase='starting';patch.seekError='The retained window changed. Choose Go Live to rejoin.';
    }
    if(linear.seekError){this.clearSeek();patch.failedSeek=patch.pendingSeek??this.snapshot.pendingSeek??this.snapshot.failedSeek;patch.pendingSeek=undefined;patch.seekError=channelMessage(linear.seekError);}
    // Expiry is recovery, not a successful clamped seek. Freeze the engine until
    // an explicit new target is accepted; the rolling producer remains active.
    const observedPosition=patch.positionSeconds??this.snapshot.positionSeconds;
    if(m.streamUrl&&this.channelObservedSeek&&!patch.pendingSeek&&!this.snapshot.pendingSeek&&
       (observedPosition<channelSeconds(m.windowStartUs)||observedPosition>channelSeconds(m.windowEndUs)+1)){
      patch.seekError='That position has left the retained window. Choose Go Live.';
      patch.failedSeek={revision:this.snapshot.seekRevision,positionSeconds:observedPosition};
    }
    this.update(patch);
  }
  goLive(){if(this.snapshot.channel){this.channelWantedState='playing';this.channels?.goLive();}}
  getQueueConnection(){return this.queueConnection;}
  setQueueConnection(connection:{error?:string;retry?:()=>void}){this.queueConnection=connection;this.update({});}
  getQueueController(){return this.queue;}
  installQueue(queue:QueueController){this.queue=queue;this.update({});return()=>{if(this.queue===queue){this.queue=undefined;this.update({});}};}
  beginQueueIntent(itemId?:string):number{
    // Live uses another lane and never becomes a queue replacement predecessor.
    if(this.snapshot.channel)this.leave(false);
    if(!this.snapshot.session){this.clearStartup();this.update({phase:'starting',intent:'playing',itemId,error:undefined});}
    else this.update({intent:'playing',error:undefined});
    return this.snapshot.intentId;
  }
  async prepareQueueIntent(intentId:number):Promise<boolean>{
    if(this.snapshot.intentId!==intentId||this.snapshot.channel)return false;
    await this.channels?.recover();
    return this.snapshot.intentId===intentId&&!this.snapshot.channel;
  }
  queueEnded(expectedIntentId:number){if(this.snapshot.intentId===expectedIntentId&&this.snapshot.phase==='ended')this.update({intent:'paused'});}
  queueFailed(itemId:string,message:string){if(!this.snapshot.channel&&!this.snapshot.session)this.update({phase:'error',itemId,intent:'paused',error:message});}
  async adoptQueueSession(itemId:string,session:PlaybackSession,expectedIntentId:number):Promise<boolean>{
    if(this.snapshot.intentId!==expectedIntentId)return false;
    const intent=this.snapshot.intent;
    this.leave(false,true);this.sequence=0;this.lastProgress=0;
    const intentId=this.snapshot.intentId;
    this.update({phase:'starting',itemId,intent,error:undefined});
    this.requestSeek(session.resumeSeconds,{session,duration:session.duration,audioEffects:{...this.snapshot.audioEffects,rendering:renderable(session)&&audioEffectsEnabled(this.snapshot.audioEffects.settings),prepared:undefined,committed:undefined,notice:renderNotice(session)}});
    if(this.snapshot.intentId===intentId&&this.snapshot.phase!=='ready'){
      this.armStartup(()=>{this.reportStalledStart(intentId);this.fail(intentId,'The selected queue item did not become ready. Retry playback or choose another item.');});
    }
    await this.persist();return this.snapshot.intentId===intentId&&!this.snapshot.cleanupError;
  }
  /**
   * A v1 audio session the native owner plays (plan §9.3 C3). `start` hands the session over (the
   * owner's first snapshot arrives through `applyNativeQueueEvent`); from then on pause, resume,
   * seek, rate, retry, timers and queue moves go to `native`, and this service never reports the
   * session's timeline or stops it itself.
   */
  async adoptNativeQueueSession(itemId:string,expectedIntentId:number,native:NativeListeningV1Control,start:()=>Promise<void>):Promise<boolean>{
    if(this.snapshot.intentId!==expectedIntentId)return false;
    const intent=this.snapshot.intent;
    const deadlines=this.snapshot.listeningDeadlines??noListeningDeadlines;
    this.leave(false,true);this.sequence=0;this.lastProgress=0;
    this.update({phase:'starting',itemId,intent,error:undefined});
    this.nativeV1=native;
    try{await start();}catch(error){if(this.nativeV1===native){this.nativeV1=undefined;void native.detach(true).catch(()=>{});}throw error;}
    if(this.nativeV1!==native)return false;
    if(this.listeningPolicy){this.nativePolicyKey='';this.syncListeningPolicy();}
    else await native.deadlines(deadlines).catch(()=>{});
    if(this.snapshot.phase!=='ready'&&this.snapshot.phase!=='error'&&this.nativeV1===native){
      this.armStartup(()=>{if(this.nativeV1===native&&this.snapshot.phase==='starting')this.fail(this.snapshot.intentId,'The selected queue item did not become ready. Retry playback or choose another item.');});
    }
    return true;
  }
  /** The native v1 owner's snapshot, with its session in this service's shape. */
  applyNativeQueueEvent(event:NativeListeningV1Event,session:PlaybackSession){
    if(!this.nativeV1)return;
    const before=this.snapshot;
    if(before.nativeListening?.owner===event.owner&&event.sequence<=before.nativeListening.sequence)return;
    const changed=before.session?.id!==session.id||before.session?.generation!==session.generation;
    if(event.ready)this.clearStartup();
    if(event.observed&&!event.seeking){this.clearSeek();this.bookmarkProtected=false;this.hasObserved=true;this.lastProgress=event.positionSeconds;}
    this.update({nativeListening:event,session,itemId:event.session.itemId??before.itemId,
      intentId:event.binding?.intentId??before.intentId,intent:event.intent,
      duration:session.duration,positionSeconds:event.observed?event.positionSeconds:(changed?session.resumeSeconds:before.positionSeconds),
      phase:event.terminal?'error':event.ended?'ended':event.ready?'ready':'starting',
      ...(changed?{audio:this.audioSelection.reset()}:{}),
      ...(event.observed&&!event.seeking?{pendingSeek:undefined,failedSeek:undefined,seekError:undefined}:{}),
      audioEffects:{...this.snapshot.audioEffects,rendering:event.rendering??this.snapshot.audioEffects.rendering,rate:event.rate,effectiveRate:event.effectiveRate,waiting:event.rateWaiting,prepared:undefined,committed:undefined,notice:event.notice??this.snapshot.audioEffects.notice},
      error:event.error??undefined,listeningDeadlines:event.deadlines});
    // The server ended it (an administrator, a transfer, the lease): its reason, never an advance.
    if(event.terminal&&event.end)this.serverEnded(session.id,event.end);
    // A terminal owner acts on nothing more: let go of it, so Retry starts afresh (a new session,
    // handed to a new owner) instead of asking the finished one.
    if(event.terminal){const native=this.nativeV1;this.nativeV1=undefined;void native?.detach(true).catch(()=>{});}
  }
  /** Whether a native v1 owner holds the current session (queue moves then go through it). */
  hasNativeOwner(){return !!this.nativeV1;}
  async confirmQueueEnd(intentId:number):Promise<void>{
    const snapshot=this.snapshot,session=snapshot.session;
    if(snapshot.channel||intentId!==snapshot.intentId||snapshot.phase!=='ended'||!session||!this.hasObserved||this.bookmarkProtected)throw new Error('Playback completion is not confirmed.');
    const payload:ProgressCheckpoint={generation:session.generation,sequence:++this.sequence,positionSeconds:snapshot.positionSeconds,state:'ended'};
    this.progressPending.set(session.id,payload);await this.persist();
    await this.bounded(()=>this.api.progressPlayback(session.id,payload));
    if(this.progressPending.get(session.id)===payload)this.progressPending.delete(session.id);await this.persist();
  }
  /** `audioStream` is the absolute stream index of an audio track from the
   * offer. The server may change the delivery mode to reach it (an original whose
   * second track the engine cannot select is repackaged instead). */
  play(itemId: string, startPosition?: number, prepared?: PreparedChoice, quality?: string, audioStream?: number): Promise<void> {
    if (this.lastPlay?.itemId !== itemId) this.escalations = 0;
    this.lastPlay = { itemId, prepared, quality, audioStream };
    if(this.queue)return this.queue.playItem(itemId,startPosition,prepared,quality,audioStream);
    if(prepared){this.queueFailed(itemId,"Prepared playback requires the device queue. Reconnect it before choosing a version.");return Promise.resolve();}
    if(this.options.queueRequired){if(this.snapshot.channel)this.leave();this.queueFailed(itemId,this.queueConnection.error??'The device queue is still connecting. Retry once it is ready.');return Promise.resolve();}
    return this.startPlayback(itemId, startPosition, "playing");
  }
  /**
   * OS-07: a title stored on this device (a download). It never goes through the device queue and
   * never asks the server for a session: `session` makes the local one (a fresh id each time, also
   * for a retry), and its progress and stop go to this service's PlaybackApi like any other, which
   * keeps them on the device (see the Apple app's offline playback).
   */
  playLocal(itemId: string, session: () => PlaybackSession, startPosition?: number): Promise<void> {
    this.escalations = 0;
    this.lastPlay = { itemId, local: session };
    return this.startLocal(itemId, session, startPosition, "playing");
  }
  private async startLocal(itemId: string, make: () => PlaybackSession, startPosition: number | undefined, intent: "playing" | "paused"): Promise<void> {
    this.endedSeekPosition = undefined;
    this.leave();
    this.failedRecovery = { positionSeconds: startPosition, intent };
    const intentId = this.snapshot.intentId;
    this.update({ phase: "starting", intent, itemId, error: undefined });
    // The journal is read first so an earlier session's cleanup is not lost; nothing here waits on
    // the server.
    await this.startRecovery();
    if (this.snapshot.intentId !== intentId) return;
    this.sequence = 0;
    this.lastProgress = 0;
    this.armStartup(() => {
      if (this.snapshot.intentId !== intentId) return;
      const session = this.snapshot.session;
      this.clearSeek();
      this.update({ audio: this.audioSelection.reset(), pendingSeek: undefined, failedSeek: undefined, phase: "error", intent: "paused", session: undefined, error: "This download did not start. Try again, or remove it and download it again." });
      if (session) this.enqueueCleanup(session.id);
    });
    let base: PlaybackSession;
    try { base = make(); } catch (error) {
      this.clearStartup();
      this.update({ phase: "error", intent: "paused", error: error instanceof Error ? error.message : "This download could not be played." });
      return;
    }
    const session = { ...base, resumeSeconds: startPosition ?? base.resumeSeconds };
    this.requestSeek(session.resumeSeconds, { session, duration: session.duration });
    this.persist();
  }
  private startupProbe?: () => Readonly<Record<string, unknown>>;
  /** Development aid (demo, 23 Sep: a start after a Metro reload never produced a frame): the
   * engine adapter says what a start is waiting on (route ready, load command sent, native ready),
   * logged in development builds when the startup timer fails a start. */
  setStartupProbe(probe: (() => Readonly<Record<string, unknown>>) | undefined) { this.startupProbe = probe; }
  private reportStalledStart(intentId: number) {
    if (!this.options.diagnostics || intentId !== this.snapshot.intentId) return;
    const s = this.snapshot;
    let engine: unknown;
    try { engine = this.startupProbe?.(); } catch (e) { engine = String(e); }
    console.warn('[playback] start timed out', JSON.stringify({session: s.session?.id, protocol: s.session?.protocol, phase: s.phase, intent: s.intent, pendingSeek: s.pendingSeek?.positionSeconds, engine}));
  }
  /** Records a marker skip (spec §4.2) against the playing session; the caller seeks. Evidence
   * only: a failure is dropped. `automatic` is for the viewer's `auto` preference on a marker the
   * server marks safe. */
  markerSkipped(markerId: string, mode: 'automatic' | 'manual', positionSeconds: number = this.snapshot.positionSeconds): void {
    const session = this.snapshot.session;
    if (!session || this.snapshot.channel || !markerId) return;
    void this.api.markerSkipped?.(session.id, {markerId, mode, positionMs: Math.max(0, Math.round(positionSeconds * 1000))}).catch(() => {});
  }
  /** Where a retry of the current title starts: the target of a start or seek that failed or
   * never settled (a resume point included), else the last observed position. A title that never
   * played retries at its requested start, never at 0:00. */
  recoveryTarget(): number|undefined {
    const s=this.snapshot;
    return this.failedRecovery?.positionSeconds??s.pendingSeek?.positionSeconds??s.failedSeek?.positionSeconds??(this.hasObserved?s.positionSeconds:s.session?.resumeSeconds);
  }
  /** Error retry preserves the requested target and intent; ordinary Play starts playing. */
  retry(): Promise<void> {
    if(this.snapshot.channel){
      if(this.snapshot.phase==='error'&&this.channelReference)return this.playChannel(this.channelReference);
      this.channels?.retry();this.update({error:undefined});return Promise.resolve();
    }
    if (this.snapshot.phase !== "error" || !this.snapshot.itemId) return Promise.resolve();
    const recovery = this.failedRecovery ?? { intent: this.snapshot.intent };
    if(this.nativeV1)return this.nativeV1.action('retry',recovery.positionSeconds??this.snapshot.positionSeconds);
    // A local title retries locally; it never belonged to the device queue or the server.
    if(this.lastPlay?.local&&this.lastPlay.itemId===this.snapshot.itemId)return this.startLocal(this.snapshot.itemId, this.lastPlay.local, recovery.positionSeconds??this.snapshot.positionSeconds, recovery.intent);
    if(this.queue||this.options.queueRequired){return this.queue?.check(true)??Promise.resolve();}
    return this.startPlayback(this.snapshot.itemId, recovery.positionSeconds, recovery.intent);
  }
  private async startPlayback(itemId: string, startPosition: number | undefined, intent: "playing" | "paused"): Promise<void> {
    this.endedSeekPosition = undefined;
    this.leave();
    // Preserve the command before any journal, cleanup, create or readiness failure.
    this.failedRecovery = { positionSeconds: startPosition, intent };
    const intentId = this.snapshot.intentId;
    this.update({
      phase: "starting",
      intent,
      itemId,
      error: undefined,
    });
    await this.startRecovery();
    if (this.snapshot.intentId !== intentId) return;
    if (
      this.cleanup.size + this.creates.size >=
        (this.options.maxPendingCleanup ?? 32) ||
      this.snapshot.cleanupError
    ) {
      this.update({
        phase: "error",
        intent: "paused",
        error:
          "Playback cleanup needs to reconnect before another session can start.",
      });
      return;
    }
    this.sequence = 0;
    this.lastProgress = 0;
    let expired = false;
    this.armStartup(() => {
      expired = true;
      if (this.snapshot.intentId === intentId) {
        const session = this.snapshot.session;
        this.clearSeek();
        this.update({
          audio: this.audioSelection.reset(),
          pendingSeek: undefined,
          failedSeek: undefined,
          phase: "error",
          intent: "paused",
          session: undefined,
          error:
            "Playback did not become ready. Check the server and try again.",
        });
        if (session) this.enqueueCleanup(session.id);
      }
    });
    // Persist the request receipt before sending so ambiguous creates can be replayed.
    const requestId = this.requestId();
    this.creates.set(requestId, { itemId, running: true });
    await this.persist();
    if (this.snapshot.cleanupError) {
      this.clearStartup();
      this.update({
        phase: "error",
        intent: "paused",
        error: this.snapshot.cleanupError,
      });
      return;
    }
    if (this.snapshot.intentId !== intentId || expired) {
      this.creates.delete(requestId);
      void this.persist();
      return;
    }
    try {
      const receivedSession = await this.bounded((signal) =>
        this.api.createPlayback(itemId, requestId, signal).then((value) => {
          if (this.snapshot.intentId !== intentId || expired) {
            this.creates.delete(requestId);
            this.enqueueCleanup(value.id);
          }
          return value;
        })
      );
      const session = {
        ...receivedSession,
        // The server owns completion/resume policy, including near-end audiobook
        // bookmarks. Only an explicit caller position (including zero) overrides it.
        resumeSeconds: startPosition ?? receivedSession.resumeSeconds,
      };
      this.creates.delete(requestId);
      if (this.snapshot.intentId !== intentId || expired) {
        this.enqueueCleanup(session.id);
        return;
      }
      this.requestSeek(session.resumeSeconds, {session, duration: session.duration});
      this.persist();
    } catch (error) {
      const pending = this.creates.get(requestId);
      if (pending) pending.running = false;
      if (
        error instanceof ApiError &&
        [400, 401, 403, 404, 409, 422].includes(error.status)
      )
        this.creates.delete(requestId);
      this.persist();
      if (this.snapshot.intentId === intentId && !expired) {
        this.clearStartup();
        this.update({
          phase: "error",
          intent: "paused",
          error:
            error instanceof Error
              ? error.message
              : "Playback could not start.",
        });
      }
    }
  }
  pause() {
    this.queue?.invalidateAudioPreparation?.();
    if(this.nativeV1){void this.nativeV1.action('pause').catch(()=>{});if(this.snapshot.phase!=='idle')this.update({intent:'paused'});return;}
    if(this.snapshot.channel){this.channelWantedState="paused";this.update({intent:"paused"});this.channels?.pause();return;}
    if (this.snapshot.phase !== "idle") {
      if (this.failedRecovery) this.failedRecovery = { ...this.failedRecovery, intent: "paused" };
      if (this.snapshot.phase === "ready") this.clearStartup();
      this.update({ intent: "paused" });
      this.checkpoint("paused");
    }
  }
  resume() {
    this.checkListeningDeadlines();
    if(this.nativeV1){void this.nativeV1.action('resume').catch(()=>{});return;}
    if(this.snapshot.channel){
      if(this.snapshot.seekError||this.snapshot.failedSeek){this.update({error:'Choose Go Live or a position in the retained window before resuming.'});return;}
      this.channelWantedState="playing";this.channels?.resume();return;
    }

    if (this.snapshot.phase === "ended" && this.snapshot.itemId) {
      void this.play(this.snapshot.itemId, this.endedSeekPosition ?? 0);
      return;
    }
    if (this.snapshot.phase === "starting" || this.snapshot.phase === "ready") {
      if (this.failedRecovery) this.failedRecovery = { ...this.failedRecovery, intent: "playing" };
      if ((this.bookmarkProtected || this.snapshot.failedSeek) && this.snapshot.session && !this.snapshot.pendingSeek) {
        this.requestSeek(this.snapshot.failedSeek?.positionSeconds ?? this.snapshot.session.resumeSeconds, {intent: "playing"});
        return;
      }
      this.update({ intent: "playing" });
    }
  }
  private clearSeek() {
    if (this.seekTimer) clearTimeout(this.seekTimer);
    this.seekTimer = undefined;
  }
  private requestSeek(positionSeconds: number, patch: Partial<PlaybackSnapshot> = {}) {
    this.clearSeek();
    const duration = patch.duration ?? this.snapshot.duration;
    const target = Math.max(0, Math.min(positionSeconds, duration || positionSeconds));
    this.failedRecovery = { positionSeconds: target, intent: patch.intent ?? this.snapshot.intent };
    const revision = this.snapshot.seekRevision + 1, intentId = this.snapshot.intentId;
    this.seekTimer = setTimeout(() => this.seekFailed(intentId, revision, "That position is not available to seek yet. Try again."), this.options.seekTimeoutMs ?? 15000);
    (this.seekTimer as any)?.unref?.();
    this.update({...patch, seekRevision: revision, pendingSeek: Object.freeze({revision, positionSeconds: target}), seekError: undefined, failedSeek: undefined});
  }
  seek(positionSeconds: number) {
    if(this.queue?.seekRetiringAudio?.(positionSeconds))return;
    this.queue?.invalidateAudioPreparation?.();
    if(this.snapshot.channel){this.channels?.seek(positionSeconds);return;}
    if (!this.snapshot.session || !Number.isFinite(positionSeconds) || !["starting", "ready", "ended"].includes(this.snapshot.phase)) return;
    if (this.snapshot.phase === "ended") {
      this.endedSeekPosition = Math.max(0, Math.min(positionSeconds, this.snapshot.duration || positionSeconds));
      return;
    }
    if(this.nativeV1){void this.nativeV1.action('seek',positionSeconds).catch(()=>{});return;}
    this.requestSeek(positionSeconds);
  }
  /** Only a correlated adapter completion acknowledges a seek; ordinary time facts do not. */
  seekApplied(intentId: number, revision: number, observedSeconds: number, state: "playing" | "paused" | "ended"): boolean {
    const pending = this.snapshot.pendingSeek;
    if(this.snapshot.channel){
      if(intentId!==this.snapshot.intentId||!pending||pending.revision!==revision||!Number.isFinite(observedSeconds)||Math.abs(observedSeconds-pending.positionSeconds)>.5||!this.channelSeekId)return false;
      if(this.channelSubmittedSeek!==this.channelSeekId){const seekId=this.channelSeekId;this.channelSubmittedSeek=seekId;void this.channels?.observe(observedSeconds,seekId).then(ok=>{if(!ok&&this.snapshot.intentId===intentId&&this.channelSubmittedSeek===seekId)this.channelSubmittedSeek=undefined;});}
      return true;
    }
    if (intentId !== this.snapshot.intentId || !this.snapshot.session || !pending || pending.revision !== revision || !["starting", "ready"].includes(this.snapshot.phase) || !Number.isFinite(observedSeconds) || observedSeconds < 0 || this.snapshot.duration > 0 && observedSeconds > this.snapshot.duration || Math.abs(observedSeconds-pending.positionSeconds) > 0.5 || !["playing", "paused", "ended"].includes(state)) return false;
    this.clearSeek();
    this.hasObserved = true;
    this.bookmarkProtected = false;
    this.update({positionSeconds: observedSeconds, pendingSeek: undefined, seekError: undefined, failedSeek: undefined});
    this.lastProgress = observedSeconds;
    this.checkpoint(state);
    return true;
  }
  /** Reports that media has stopped arriving and the buffer is empty (or that it is flowing
   * again). Anything short of that is the adapter's business and is never shown. */
  recovering(intentId: number, active: boolean) {
    if (intentId !== this.snapshot.intentId || this.snapshot.phase === "error" || this.snapshot.phase === "idle") return;
    if (!!this.snapshot.recovering !== active) this.update({ recovering: active || undefined });
  }
  seekFailed(intentId: number, revision: number, message: string) {
    if (intentId !== this.snapshot.intentId || !this.snapshot.session || this.snapshot.pendingSeek?.revision !== revision) return;
    this.clearSeek();
    if(this.snapshot.channel)void this.channels?.observe(this.snapshot.positionSeconds,null,'engine_seek_failed');
    const safe = typeof message === "string" && message.trim() && message.length <= 512 && !/[\x00-\x1f\x7f]/.test(message) ? message : "That position could not be reached. Try again.";
    this.update({failedSeek: this.snapshot.pendingSeek, pendingSeek: undefined, seekError: safe, intent: "paused" as const});
  }
  /** A lost engine must restore the last settled observation before checkpoints resume. */
  engineLost(intentId: number) {
    if(this.snapshot.channel){
      if(intentId===this.snapshot.intentId&&this.snapshot.session&&this.channelObservedSeek)this.channels?.seek(this.snapshot.positionSeconds);
      return;
    }
    if (intentId !== this.snapshot.intentId || !this.snapshot.session || !["starting", "ready"].includes(this.snapshot.phase)) return;
    const target = this.hasObserved ? this.snapshot.positionSeconds : this.snapshot.session.resumeSeconds;
    this.bookmarkProtected = true;
    this.requestSeek(target);
  }
  ready(intentId: number) {
    if (
      intentId === this.snapshot.intentId &&
      this.snapshot.session &&
      this.snapshot.phase === "starting"
    ) {
      this.clearStartup();
      this.update({ phase: "ready" });
    }
  }
  fact(
    intentId: number,
    positionSeconds: number,
    state: "playing" | "paused" | "ended"
  ) {
    if(this.snapshot.channel){
      if(intentId!==this.snapshot.intentId||!this.snapshot.session||this.snapshot.pendingSeek||this.snapshot.failedSeek||!Number.isFinite(positionSeconds))return;
      if(state==='ended'){this.update({error:'The channel stream ended. Retry or choose Go Live.'});return;}
      this.update({positionSeconds});void this.channels?.observe(positionSeconds);return;
    }
    if (
      intentId !== this.snapshot.intentId ||
      !this.snapshot.session ||
      !!this.snapshot.pendingSeek ||
      !!this.snapshot.failedSeek ||
      !["starting", "ready"].includes(this.snapshot.phase) ||
      !Number.isFinite(positionSeconds) ||
      positionSeconds < 0
    )
      return;
    if (
      state === "playing" &&
      positionSeconds > this.snapshot.session.resumeSeconds + 0.01
    )
      this.clearStartup();
    this.hasObserved = true;
    // An element that has already reported its end keeps reporting it on every
    // later event. Only the first report is a transition; later ones must not
    // pause an intent the queue has since set to playing for the next item.
    if (state === "ended" && this.snapshot.phase === "ended") return;
    if (state === "ended") this.clearSeek();
    this.update({
      positionSeconds,
      ...(state === "ended"
        ? { phase: "ended" as const, intent: "paused" as const, pendingSeek: undefined, audio: this.audioSelection.reset() }
        : {}),
    });
    if (
      state !== "playing" ||
      Math.abs(positionSeconds - this.lastProgress) >= 3
    ) {
      this.lastProgress = positionSeconds;
      this.checkpoint(state);
    }
  }
  /** The engine rejected the stream it was given: a decode error, an unsupported
   * source, a manifest it cannot read. Not the network, and not the server saying
   * no. A capability document can be wrong, so this is not yet a failure the
   * viewer should see: the server is told which route failed, and when there is a
   * route above it the title is asked for again from the same position and plays
   * through that one instead. The viewer sees the recovery state, never an error,
   * unless the last resort has failed too. */
  engineFailed(intentId: number, code: RouteFailureCode, detail: string, message: string) {
    const session = this.snapshot.session, itemId = this.snapshot.itemId, last = this.lastPlay;
    if (intentId !== this.snapshot.intentId || this.snapshot.phase === "error" || this.snapshot.phase === "idle") return;
    if (!session || !itemId || this.snapshot.channel || !this.api.reportRouteFailure || !last || last.itemId !== itemId || last.prepared || last.local || this.escalations >= 3) {
      this.fail(intentId, message);
      return;
    }
    const position = this.snapshot.pendingSeek?.positionSeconds ?? (this.hasObserved ? this.snapshot.positionSeconds : session.resumeSeconds ?? 0);
    this.escalations++;
    this.recovering(intentId, true);
    void this.api.reportRouteFailure(session.id, code, detail).then(result => {
      if (intentId !== this.snapshot.intentId) return;
      if (!result?.escalates) { this.fail(intentId, message); return; }
      return this.play(itemId, position, undefined, last.quality, last.audioStream);
    }).catch(() => { if (intentId === this.snapshot.intentId) this.fail(intentId, message); });
  }
  fail(intentId: number, message: string, extra: Partial<PlaybackSnapshot> = {}) {
    if (intentId === this.snapshot.intentId && this.snapshot.phase !== "idle") {
      if (this.snapshot.phase !== "error") {
        this.failedRecovery = {
          positionSeconds: this.snapshot.pendingSeek?.positionSeconds ?? this.snapshot.failedSeek?.positionSeconds ?? (this.hasObserved ? this.snapshot.positionSeconds : this.snapshot.session?.resumeSeconds ?? this.failedRecovery?.positionSeconds),
          intent: this.snapshot.intent,
        };
      }
      this.clearStartup();
      this.clearSeek();
      this.update({ audio: this.audioSelection.reset(), phase: "error", intent: "paused", error: message, pendingSeek: undefined, seekError: undefined, failedSeek: undefined, recovering: undefined, ...extra });
    }
  }
  leave(notifyQueue=true,preserveListeningDeadlines=false) {
    if(!preserveListeningDeadlines)this.listeningClock.clear();
    if(notifyQueue)this.queue?.leave();
    const wasChannel=!!this.snapshot.channel;
    // A native v1 owner reports and stops its own session; nothing here writes for it.
    const native=this.nativeV1;this.nativeV1=undefined;
    if(native)void native.detach(true).catch(()=>{});
    this.channels?.close();this.channelReference=undefined;this.channelSeekId=undefined;this.channelObservedSeek=undefined;this.channelSubmittedSeek=undefined;
    this.failedRecovery = undefined;
    this.clearStartup();
    this.clearSeek();
    if(!native)this.checkpoint("paused");
    this.hasObserved = false;
    this.bookmarkProtected = true;
    const old = this.snapshot.session;
    this.update({
      audioEffects:{...this.snapshot.audioEffects,rendering:false,effectiveRate:0,waiting:false,prepared:undefined,committed:undefined,notice:""},
      audio: this.audioSelection.reset(),
      audioTracks: undefined,
      intentId: this.snapshot.intentId + 1,
      phase: "idle",
      intent: "paused",
      itemId: preserveListeningDeadlines?this.snapshot.itemId:undefined,
      nativeListening:undefined,
      listeningDeadlines:preserveListeningDeadlines?this.snapshot.listeningDeadlines:noListeningDeadlines,
      channel: undefined,
      linear: undefined,
      session: undefined,
      pendingSeek: undefined,
      seekError: undefined,
      failedSeek: undefined,
      positionSeconds: 0,
      duration: 0,
      error: undefined,
    });
    if (old&&!wasChannel&&!native) this.enqueueCleanup(old.id);
  }
  private checkpoint(state: "playing" | "paused" | "ended") {
    if(this.snapshot.channel||this.nativeV1)return;
    const session = this.snapshot.session;
    if (!session || !this.hasObserved || this.bookmarkProtected) return;
    this.progressPending.set(session.id, {
      generation: session.generation,
      sequence: ++this.sequence,
      positionSeconds: this.snapshot.positionSeconds,
      state,
    });
    this.persist();
    this.flushProgress();
  }
  private async bounded<T>(
    work: (signal: AbortSignal) => Promise<T>
  ): Promise<T> {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      return await Promise.race([
        work(controller.signal),
        new Promise<T>((_, reject) => {
          timer = setTimeout(() => {
            controller.abort();
            reject(new Error("Operation timed out"));
          }, this.options.operationTimeoutMs ?? 5000);
        }),
      ]);
    } finally {
      if (timer) clearTimeout(timer);
    }
  }
  private flushProgress() {
    for (const [id, payload] of this.progressPending) {
      if (this.progressRunning.has(id) || this.cleanup.has(id)) continue;
      this.progressRunning.add(id);
      void this.bounded(() => this.api.progressPlayback(id, payload)).then(
        () => {
          if (this.progressPending.get(id) === payload)
            this.progressPending.delete(id);
          this.progressRunning.delete(id);
          this.persist();
          if (
            this.progressPending.get(id) &&
            this.progressPending.get(id) !== payload
          )
            this.flushProgress();
        },
        () => {
          this.progressRunning.delete(id);
          this.persist();
        }
      );
    }
  }
  private enqueueCleanup(id: string) {
    if (!this.cleanup.has(id))
      this.cleanup.set(id, {
        running: false,
        checkpoint: this.progressPending.get(id),
      });
    this.update({});
    void this.persist().then(() => this.tryCleanup(id)).catch(() => {});
  }
  private tryCleanup(id: string): Promise<void> {
    const entry = this.cleanup.get(id);
    if (!entry) return Promise.resolve();
    if (entry.work) return entry.work;
    entry.running = true;
    entry.work = (async () => {
      try {
        const checkpoint = this.progressPending.get(id) ?? entry.checkpoint;
        if (checkpoint) {
          try {
            await this.bounded(() => this.api.progressPlayback(id, checkpoint));
          } catch (error) {
            if (![404, 409, 410].includes(statusOf(error))) throw error;
          }
        }
        try {
          await this.bounded((signal) => this.api.stopPlayback(id, signal));
        } catch (error) {
          // Already ended (it played to its end) or already gone: there is nothing left to stop.
          if (![404, 410].includes(statusOf(error))) throw error;
        }
        this.cleanup.delete(id);
        this.progressPending.delete(id);
        this.update({});
        await this.persist();
      } catch (error) {
        entry.running = false;
        entry.work = undefined;
        this.update({});
        throw error;
      }
    })();
    // Background leave/retry callers do not await, but an explicit cross-kind
    // handoff awaits the same work and surfaces failure without a second player.
    void entry.work.catch(() => {});
    return entry.work;
  }
  retryCleanup() {
    for (const id of this.cleanup.keys()) this.tryCleanup(id);
    for (const [requestId, entry] of this.creates) {
      if (entry.running) continue;
      entry.running = true;
      void this.bounded((signal) =>
        this.api.createPlayback(entry.itemId, requestId, signal)
      ).then(
        (session) => {
          this.creates.delete(requestId);
          this.enqueueCleanup(session.id);
        },
        () => {
          entry.running = false;
        }
      );
    }
    this.flushProgress();
  }
  private persist(): Promise<void> {
    if (!this.options.journal) return Promise.resolve();
    const records: PlaybackJournalEntry[] = [...this.cleanup].map(
      ([id, value]) => ({
        id,
        checkpoint: this.progressPending.get(id) ?? value.checkpoint,
      })
    );
    for (const [requestId, entry] of this.creates)
      records.push({ request: { requestId, itemId: entry.itemId } });
    // A native v1 owner's session is its own to stop (it holds the timeline and the lease).
    const active = this.nativeV1 ? undefined : this.snapshot.session;
    if (active && !records.some((v) => v.id === active.id))
      records.push({
        id: active.id,
        checkpoint: this.progressPending.get(active.id),
      });
    this.journalTail = this.journalTail
      .then(() => this.options.journal!.save(records))
      .catch(() => {
        this.update({
          cleanupError:
            "Playback recovery could not be saved. Check secure storage.",
        });
      });
    return this.journalTail;
  }
}
export type HostedAccount = {
  id: string;
  username: string;
  displayName: string;
};
export type HostedProfile = { id: string; name: string };
export type HostedSession = {
  refreshToken?: string;
  familyId: string;
  refreshExpiresAt: string;
  accessToken: string;
  expiresAt: string;
  account: HostedAccount;
  profiles: HostedProfile[];
};
/**
 * Whether Hosted has heard from a server recently (its presence lease). Absent
 * means unknown (an older Hosted); `lastSeenAt` is null when it never checked in.
 */
export type ServerPresence = Readonly<{ online: boolean; lastSeenAt: string | null }>;
export type HostedServer = {
  routes?: import('./route-identity.ts').SignedRoutes;
  id: string;
  name: string;
  baseUrl: string;
  publicKey: string;
  policyRevision: number;
  presence?: ServerPresence;
};
/** Reads a server's presence defensively: anything malformed is "unknown" (undefined). */
export function parseServerPresence(value: unknown): ServerPresence | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const v = value as Record<string, unknown>;
  if (typeof v.online !== "boolean") return undefined;
  const last = typeof v.lastSeenAt === "string" && v.lastSeenAt.length <= 64 && Number.isFinite(Date.parse(v.lastSeenAt)) ? v.lastSeenAt : null;
  return Object.freeze({ online: v.online, lastSeenAt: last });
}
/**
 * X-13 (client): chooser ordering and the offline-dialog rule. Pure, no
 * formatting: apps render dates through the i18n catalogue (`i18n.date`).
 * Unknown presence (absent or malformed) sorts last and renders normally.
 */
/** Within the last hour a server still gets the "keep trying" copy; older than that it is stale. */
export const SERVER_RECENTLY_SEEN_MS = 3_600_000;
/** Chooser order: online first, then offline, then unknown. */
export function presenceRank(presence: ServerPresence | undefined): 0 | 1 | 2 {
  if (!presence) return 2;
  return presence.online ? 0 : 1;
}
function lastSeenMs(presence: ServerPresence | undefined): number | undefined {
  if (!presence || !presence.lastSeenAt) return undefined;
  const ms = Date.parse(presence.lastSeenAt);
  return Number.isFinite(ms) ? ms : undefined;
}
/** Online servers first, then the rest by `lastSeenAt` descending (unknown last). Stable. */
export function sortServersByPresence<T extends { presence?: ServerPresence }>(servers: readonly T[]): T[] {
  return [...servers].sort((a, b) => {
    const rank = presenceRank(a.presence) - presenceRank(b.presence);
    if (rank !== 0) return rank;
    const aSeen = lastSeenMs(a.presence), bSeen = lastSeenMs(b.presence);
    if (aSeen === undefined && bSeen === undefined) return 0;
    if (aSeen === undefined) return 1;
    if (bSeen === undefined) return -1;
    return bSeen - aSeen;
  });
}
/** Whether Hosted saw the server within `windowMs` (default one hour). Never-seen is never recent. */
export function wasRecentlySeen(presence: ServerPresence | undefined, nowMs: number = Date.now(), windowMs: number = SERVER_RECENTLY_SEEN_MS): boolean {
  const seen = lastSeenMs(presence);
  return seen !== undefined && seen <= nowMs && nowMs - seen < windowMs;
}
export type PresenceLabel = Readonly<
  | { kind: "unknown" }
  | { kind: "online" }
  | { kind: "lastSeen"; lastSeenAt: string; recent: boolean }
  | { kind: "neverSeen" }
>;
/** What a chooser row says about presence: "Last seen {date}" (`lastSeen`), "Never seen" (`neverSeen`), or nothing. */
export function presenceLabel(presence: ServerPresence | undefined, nowMs: number = Date.now()): PresenceLabel {
  if (!presence) return Object.freeze({ kind: "unknown" }) as PresenceLabel;
  if (presence.online) return Object.freeze({ kind: "online" }) as PresenceLabel;
  if (!presence.lastSeenAt || !Number.isFinite(Date.parse(presence.lastSeenAt))) return Object.freeze({ kind: "neverSeen" }) as PresenceLabel;
  return Object.freeze({ kind: "lastSeen", lastSeenAt: presence.lastSeenAt, recent: wasRecentlySeen(presence, nowMs) });
}
/**
 * A failed connect gets the specific "can't reach" dialog (not the generic
 * waiting notice) only for an offline, stale server: unknown presence renders
 * normally, and a server seen within the hour keeps the keep-trying copy.
 */
export function needsOfflineServerDialog(presence: ServerPresence | undefined, unreachable: boolean, nowMs: number = Date.now()): boolean {
  return unreachable && !!presence && !presence.online && !wasRecentlySeen(presence, nowMs);
}
export class HttpHostedApi {
  private http: HttpLocalApi;
  constructor(
    baseUrl: string,
    token = "",
    fetcher: typeof fetch = (input, init) => globalThis.fetch(input, init)
  ) {
    this.http = new HttpLocalApi(baseUrl, token, fetcher);
  }
  setAccessToken(token: string) {
    this.http.setAccessToken(token);
  }
  /** `GET /v1/servers`. `version` and `watch` feed `ServerListWatcher.adopt`. */
  async servers(signal?: AbortSignal): Promise<{ items: HostedServer[]; version?: string; watch?: string }> {
    const out = await this.http.request<{ items: HostedServer[]; version?: string; watch?: string }>("/v1/servers", "GET", undefined, signal);
    if (!out || !Array.isArray(out.items)) return out;
    return { ...out, items: out.items.map(server => {
      if (!server || typeof server !== "object") return server;
      const { presence: raw, ...rest } = server as HostedServer & { presence?: unknown };
      const presence = parseServerPresence(raw);
      return presence ? { ...rest, presence } : rest;
    }) };
  }
  /** The account's transport, for `porticoSignIn` and `acceptPorticoInvitation`. */
  request<T>(path: string, method = "GET", body?: unknown, signal?: AbortSignal): Promise<T> {
    return this.http.request<T>(path, method, body, signal);
  }
  logout() {
    return this.http.request<void>("/v1/sessions/current", "DELETE");
  }
}
export {
  CatalogService,
  type CatalogSnapshot,
  type CatalogApi,
} from "./catalog.ts";

export { DeviceAuthorizationService, HttpDeviceAuthorizationApi, type DeviceAuthorization, type DeviceAuthorizationApi, type DeviceAuthorizationSnapshot } from "./device-authorization.ts";

export { HostedGate, hostedGate, type HostedGateSnapshot } from "./hosted-gate.ts";
export { CredentialSessionService,HttpCredentialApi,type CredentialContext,type CredentialRecord,type CredentialStorage,type CredentialApi,type CredentialSnapshot,type CredentialCoordinator } from "./credentials.ts";
export {LibraryNavigationService,navigationStorageKey,type NavigationScope,type LibraryRoute,type LibraryEntityView,type EntityViewState,type NavigationFocus,type ScrollAnchor,type LibraryNavigationMetadata,type LibraryViewState,type LibraryViewUpdate,type NavigationStorage,type LibraryNavigationSnapshot} from './library-navigation.ts';
export {LibraryContentService,type ContentView,type ContentSurface,type ContentRoute,type ContentScope,type LibraryContentApi,type ContentEntryKind,type ContentHeading,type ContentEntry,type ContentSection,type ContentProjection,type LibraryContentSnapshot} from './library-content.ts';

export * from './hosted-security.ts';
export {DetailService,type DetailScope,type DetailTarget,type DetailApi,type PersonalState,type PersonalOfflineMutation,type PersonalConflict,type DetailAction,type DetailMetadata,type DetailEpisodeInfo,type DetailSongInfo,type DetailBookFileInfo,type DetailSource,type DetailItem,type DetailProjection,type PersonalIntent,type DetailError,type DetailSnapshot} from './detail.ts';
export * from "./personal-saved.ts";
export {SearchService,searchPath,parseSearchHistory,type SearchGroup,type SearchSort,type SearchDirection,type SearchGroupStatus,type SearchGroupErrorCode,type SearchQuery,type SearchEntry,type SearchGroupResult,type SearchGroupCapability,type SearchCapabilities,type SearchData,type SearchSnapshot,type SearchHistoryEntry,type SearchHistoryPage} from './search.ts';
export * from './people.ts';
export {savedListPath,personalHistoryPath,savedListFilters,savedListSorts,historyPeriods,type SavedListFilter,type SavedListSort,type HistoryPeriod,type SavedApi,type SavedScope,type SavedRoute,type SavedActor} from './saved.ts';
export * from './browse.ts';
export * from './social-playback.ts';
export * from './group-session.ts';
export * from './remote-playback.ts';
export * from './library-pins.ts';
export * from './subtitles.ts';
export * from "./lyrics.ts";

export {type MusicPolicy,type MusicPolicyInput,type MusicMatchObservation,type MusicEvidenceDetails,type LocalBookMetadata,musicEvidenceLabel,fingerprintStatusLabel} from "./music-metadata.ts";

export {saveLocalBookPolicy,parseLocalAudioPolicy,type LocalAudioPolicy} from './music-metadata.ts';
export {MetadataRepairService,validateRepair} from "./metadata-repair.ts";
export type {RepairTarget,RepairScope,RepairData,RepairCommand,RepairView,RepairPreview,RepairField,RepairFieldSpec} from "./metadata-repair.ts";
export {validBulkEdit,validBulkListEntry,validateBulkReceipt,bulkConflicts,bulkTargetLimit} from './metadata-bulk.ts';
export type {BulkEdit,BulkTarget,BulkListEdit,BulkFieldEdit,BulkResult,BulkReceipt} from './metadata-bulk.ts';
export * from './prepared-media.ts';

export * from './trickplay.ts';
export {PlayerChaptersService} from './player-chapters.ts';
export type {PlayerChapter,PlayerChaptersData,PlayerChaptersTarget,PlayerChaptersSnapshot} from './player-chapters.ts';
export * from './listening.ts';
export * from './preferences.ts';
export {parseSegmentMarkers,parseSegmentMarkerSet,canSkipAutomatically,type SegmentMarker,type SegmentMarkerKind,type SegmentMarkerSet} from './segment-markers.ts';
export {parseQueuePostPlay,passoutCheckRequired,type QueuePostPlay,type QueueAdvanceMode} from './post-play.ts';
export * from './home.ts';
export * from './play-history.ts';
export * from './recommendations.ts';
export * from './notifications.ts';
export {detailExtraTypes,type DetailExtra,type DetailExtraType,parseCreditPage,type CreditPage,type DetailCredit} from './detail.ts';

export * from './downloads.ts';
export * from './bulk-jobs.ts';
export * from './container-state.ts';
export * from './download-requests.ts';
export * from './inbox.ts';
export * from './identity-client.ts';
export * from './sse.ts';
export * from './downloads-service.ts';
