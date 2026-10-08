/**
 * Playback options (spec §4.1) and device capabilities (§3).
 *
 * `options(itemId, preview?)` reads what an item offers and what the server would choose for this
 * device (`plan`); `preview` asks "what if I chose this quality/track/version", so the UI can label
 * choices ("Plays as original", "Converts audio") before starting. Capabilities are published with
 * `PUT /v1/me/devices/current/capabilities` only when they change.
 */
import type {AudioDecodeCapability} from './audio-render.ts';
import {call, enc, type V1Http} from './http.ts';
import {parseOptions, type AudioStream, type MediaVersion, type PlaybackOptions, type PlaybackPlan, type SubtitleStream} from './types.ts';
import type {QualityRequest} from './quality.ts';

export type OptionsPreview = Readonly<{versionId?: string; partId?: string; quality?: QualityRequest; audioId?: string; subtitleId?: string | null}>;

function query(p: OptionsPreview): string {
  const q = new URLSearchParams();
  if (p.versionId) q.set('versionId', p.versionId);
  if (p.partId) q.set('partId', p.partId);
  if (p.audioId) q.set('audioId', p.audioId);
  if (p.subtitleId !== undefined) q.set('subtitleId', p.subtitleId ?? 'none');
  if (p.quality) {
    q.set('quality', p.quality.mode);
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
    const r = await call(this.http, {method: 'GET', path: `/v1/items/${enc(itemId)}/playback-options${query(preview)}`, signal});
    return parseOptions(r.body);
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
  const subtitle = o.preferred ? version.subtitles.find(s => s.id === o.preferred!.subtitleId) : version.subtitles.find(s => s.forced && (!audio?.language || s.language === audio.language));
  return Object.freeze({version, audio, subtitle});
}

/** The marker (intro, credits…) under a position, for the Skip prompt. */
export function markerAt(o: PlaybackOptions, positionMs: number) {
  return o.markers.find(m => positionMs >= m.startMs && positionMs < m.endMs);
}

/** The device capability profile (spec §3). Platforms fill it from their probes. */
export type CapabilityProfile = Readonly<{
  form: 'tv' | 'phone' | 'tablet' | 'desktop' | 'browser' | 'cast' | 'speaker';
  network: Readonly<{class: 'local' | 'remote' | 'cellular'; maxBitrateKbps?: number}>;
  containers: readonly Readonly<{container: string; direct: boolean}>[];
  streaming: Readonly<{hls: Readonly<{fmp4: boolean; ts: boolean; maxSegmentMs?: number}>; progressive: boolean}>;
  video: readonly Readonly<Record<string, unknown>>[];
  display?: Readonly<{hdr: readonly string[]; maxWidth?: number; maxHeight?: number}>;
  audio: readonly Readonly<Record<string, unknown>>[];
  /** What this client's own audio engine decodes (spec §3, §18.1): only pairs that pass the §18.8
   * fixtures here. Absent or empty: every music presentation is converted (or unavailable). */
  audioDecode?: readonly AudioDecodeCapability[];
  subtitles: readonly Readonly<{format: string; render: 'native' | 'client' | 'none'}>[];
  features: Readonly<Record<string, boolean>>;
}>;

/** Publishes the profile when it differs from the last one the server accepted. */
export class CapabilityPublisher {
  private http: V1Http;
  private last?: string;
  constructor(http: V1Http) { this.http = http; }
  async publish(profile: CapabilityProfile, signal?: AbortSignal): Promise<boolean> {
    const body = JSON.stringify(profile);
    if (body === this.last) return false;
    await call(this.http, {method: 'PUT', path: '/v1/me/devices/current/capabilities', body: profile, signal});
    this.last = body;
    return true;
  }
  /** Forget what was sent (sign-in, server switch). */
  reset() { this.last = undefined; }
}
