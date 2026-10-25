import {encodedRoutePath, routePath} from "../../../contracts/src/routes.ts";
/**
 * Playback options (spec §4.1) and device capabilities (§3).
 *
 * `options(itemId, preview?)` reads what an item offers and what the server would choose for this
 * device (`plan`); `preview` asks "what if I chose this quality/track/version", so the UI can label
 * choices ("Plays as original", "Converts audio") before starting. Capabilities are published with
 * `PUT /v1/me/devices/current/capabilities` only when they change.
 */
import type {CapabilityDocument, StoredCapabilities} from '../client-profile.ts';
import {capabilityDocumentProblem} from '../client-profile.ts';
import {PlaybackApiError, call, enc, type V1Http} from './http.ts';
import {ContractError, parseDiscTitleRef, parseOptions, type DiscTitleRef, type AudioStream, type MediaVersion, type PlaybackOptions, type PlaybackPlan, type SubtitleStream} from './types.ts';
import type {QualityRequest} from './quality.ts';

export type OptionsPreview = Readonly<{versionId?: string; discTitle?: DiscTitleRef; partId?: string; quality?: QualityRequest; audioId?: string; subtitleId?: string | null; throughputKbps?: number}>;

function query(p: OptionsPreview): string {
  const q = new URLSearchParams();
  if (p.throughputKbps && Number.isSafeInteger(p.throughputKbps) && p.throughputKbps>0) q.set('throughputKbps',String(p.throughputKbps));
  if (p.versionId) q.set('versionId', p.versionId);
  if (p.discTitle !== undefined) {
    const title = parseDiscTitleRef(p.discTitle);
    if (!title || !p.versionId || p.partId !== undefined) throw new ContractError('disc title preview');
    q.set('discTitleId', title.id); q.set('discTitleRevision', title.revision);
  }
  if (p.partId) q.set('partId', p.partId);
  if (p.audioId) q.set('audioId', p.audioId);
  if (p.subtitleId !== undefined) q.set('subtitleId', p.subtitleId ?? 'none');
  if (p.quality) {
    q.set('quality', p.quality.mode);
    if ((p.quality.mode === 'fixed' || p.quality.mode === 'automatic') && p.quality.id) q.set('qualityId', p.quality.id);
    if (p.quality.mode === 'limit') {
      if (p.quality.maxVideoBitrateKbps) q.set('maxVideoBitrateKbps', String(p.quality.maxVideoBitrateKbps));
      if (p.quality.maxHeight) q.set('maxHeight', String(p.quality.maxHeight));
      if (p.quality.maxAudioBitrateKbps) q.set('maxAudioBitrateKbps', String(p.quality.maxAudioBitrateKbps));
    }
  }
  const s = q.toString();
  return s ? '?' + s : '';
}

export class PlaybackOptionsClient {
  private http: V1Http;
  constructor(http: V1Http) { this.http = http; }

  async options(itemId: string, preview: OptionsPreview = {}, signal?: AbortSignal): Promise<PlaybackOptions> {
    const title = parseDiscTitleRef(preview.discTitle), versionId = preview.versionId;
    const r = await call(this.http, {method: 'GET', path: encodedRoutePath("get_playback_options", {"itemId": enc(itemId)}) + (query({...preview, discTitle: title})), signal});
    const options = parseOptions(r.body);
    if (title) {
      const versions = options.versions.filter(version => version.id === versionId);
      if (options.itemId !== itemId || versions.length !== 1 || versions[0].discTitle?.id !== title.id
        || versions[0].discTitle.revision !== title.revision) throw new ContractError('disc title preview response');
    }
    return options;
  }
}

/** What a plan does, for one-line labels and the technical details panel. */
export type PlanSummary = Readonly<{
  /** direct: the original file; remux: container changed, streams copied; convert: something is transcoded. */
  kind: 'direct' | 'remux' | 'convert';
  convertsVideo: boolean;
  convertsAudio: boolean;
  burnsSubtitles: boolean;
  /** Reason codes (e.g. `audio_codec_unsupported`, `admin_cap_remote_bitrate`), deduplicated. Words come from the catalogue. */
  reasons: readonly string[];
}>;

export function summarizePlan(plan: PlaybackPlan, version?: MediaVersion): PlanSummary {
  const kinds = (id: string) => (version?.video.some(v => v.id === id) ? 'video' : version?.audio.some(a => a.id === id) ? 'audio' : version?.subtitles.some(s => s.id === id) ? 'subtitles' : undefined);
  let convertsVideo = false, convertsAudio = false, burnsSubtitles = false;
  for (const s of plan.streams) {
    const k = kinds(s.id) ?? (s.id.startsWith('v') ? 'video' : s.id.startsWith('a') ? 'audio' : s.id.startsWith('s') ? 'subtitles' : undefined);
    if (s.action === 'burn') { burnsSubtitles = true; convertsVideo = true; }
    else if (s.action === 'transcode') { if (k === 'video') convertsVideo = true; else if (k === 'audio') convertsAudio = true; }
  }
  const kind = plan.mode === 'direct' ? 'direct' : convertsVideo || convertsAudio || burnsSubtitles ? 'convert' : 'remux';
  return Object.freeze({kind, convertsVideo, convertsAudio, burnsSubtitles, reasons: Object.freeze([...new Set(plan.streams.flatMap(s => s.reasons))])});
}

/** The starting choice: the server's `preferred`, else the first version with its default tracks. */
export function defaultSelection(o: PlaybackOptions): Readonly<{version?: MediaVersion; audio?: AudioStream; subtitle?: SubtitleStream}> {
  const version = o.versions.find(v => v.id === o.preferred?.versionId) ?? o.versions[0];
  if (!version) return Object.freeze({});
  const audio = version.audio.find(a => a.id === o.preferred?.audioId) ?? version.audio.find(a => a.default) ?? version.audio[0];
  // With the server's preference the subtitle is the server's to choose at the start (the viewer's Settings).
  const subtitle = o.preferred ? undefined : version.subtitles.find(s => s.forced && (!audio?.language || s.language === audio.language));
  return Object.freeze({version, audio, subtitle});
}

/** The marker (intro, credits…) under a position, for the Skip prompt. */
export function markerAt(o: PlaybackOptions, positionMs: number) {
  return o.markers.find(m => positionMs >= m.startMs && positionMs < m.endMs);
}

/**
 * Publishes this device's capability document (`client-profile.ts`) when it differs from the last
 * one the server accepted: `PUT /v1/me/devices/current/capabilities`, answered 204 with the new
 * revision in `ETag`. The document belongs to the device, not the sign-in, so one publish serves
 * every later sign-in of the device. A document the server would refuse is never sent. A failure
 * is swallowed by design: a device that could not publish is planned against the server's
 * baseline, which always plays, and publishing is tried again the next time anything calls this.
 */
export class CapabilityPublisher {
  private http: V1Http;
  private last?: string;
  private inflight?: Promise<boolean>;
  private readonly scope: () => string;
  /** `scope` names the server and device (a different scope publishes again). */
  constructor(http: V1Http, scope: () => string = () => '') { this.http = http; this.scope = scope; }
  publish(document: CapabilityDocument, signal?: AbortSignal): Promise<boolean> {
    if (capabilityDocumentProblem(document)) return Promise.resolve(false);
    const key = this.scope() + '\n' + JSON.stringify(document);
    if (key === this.last) return Promise.resolve(false);
    if (this.inflight) return this.inflight;
    const work = call(this.http, {method: 'PUT', path: routePath("put_device_capabilities"), body: document, signal})
      .then(() => { this.last = key; return true; })
      .catch(() => false)
      .finally(() => { if (this.inflight === work) this.inflight = undefined; });
    this.inflight = work;
    return work;
  }
  /** What the server holds for this device, or undefined when it has published nothing. */
  async read(signal?: AbortSignal): Promise<StoredCapabilities | undefined> {
    try {
      const r = await call(this.http, {method: 'GET', path: routePath("get_device_capabilities"), signal});
      const b = r.body as {revision?: unknown; capabilities?: unknown} | undefined;
      if (!b || typeof b.revision !== 'number' || !b.capabilities || typeof b.capabilities !== 'object') return undefined;
      return Object.freeze({revision: b.revision, capabilities: b.capabilities as CapabilityDocument});
    } catch (e) {
      if (e instanceof PlaybackApiError && e.status === 404) return undefined;
      throw e;
    }
  }
  /** Forget what was sent (sign-in, server switch). */
  reset() { this.last = undefined; }
}
