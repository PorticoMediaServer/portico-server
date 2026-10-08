/**
 * Playback v1 resource shapes and tolerant readers (style guide amendment 3): unknown fields are
 * ignored, unknown open-enum values map to a fallback, absent arrays read as empty. Strict only
 * where the contract-drift policy requires it: ids, revisions, generations and URLs.
 * These are hand-written until lane C's generated `packages/contracts` lands (plan §2); the
 * readers then become thin adapters over the generated decoders.
 */
export class ContractError extends Error {
  constructor(what: string) { super(`Unreadable playback response: ${what}`); this.name = 'ContractError'; }
}

import {parseAudioRenderV2, playableAudio, type AudioRenderV2} from './audio-render.ts';

type Obj = Record<string, unknown>;
export const obj = (v: unknown): v is Obj => !!v && typeof v === 'object' && !Array.isArray(v);
const idRe = /^[A-Za-z0-9_.-]{1,128}$/;
export function id(v: unknown, what: string): string { if (typeof v !== 'string' || !idRe.test(v)) throw new ContractError(what); return v; }
const optId = (v: unknown, what: string) => (v === undefined ? undefined : id(v, what));
export function revision(v: unknown, what = 'revision'): string { if (typeof v !== 'string' || !v || v.length > 256) throw new ContractError(what); return v; }
const str = (v: unknown, fallback = '') => (typeof v === 'string' ? v : fallback);
const num = (v: unknown, fallback = 0) => (typeof v === 'number' && Number.isFinite(v) ? v : fallback);
const optNum = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : undefined);
const bool = (v: unknown) => v === true;
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
function oneOf<T extends string>(v: unknown, values: readonly T[], fallback: T): T { return (values as readonly unknown[]).includes(v) ? v as T : fallback; }
/** A relative `/v1/…` path or an absolute http(s) URL. */
export function url(v: unknown, what: string): string {
  if (typeof v !== 'string' || v.length > 4096 || !(/^\/v1\//.test(v) || /^https?:\/\//.test(v))) throw new ContractError(what);
  return v;
}

// ── Options (spec §4.1) ─────────────────────────────────────────────
export type VideoStream = Readonly<{id: string; codec: string; profile?: string; bitDepth?: number; width?: number; height?: number; fps?: number; hdr?: string; bitrateKbps?: number}>;
export type AudioStream = Readonly<{id: string; codec: string; channels?: number; layout?: string; atmos: boolean; language?: string; title?: string; default: boolean; commentary: boolean; visualImpaired: boolean}>;
export type SubtitleStream = Readonly<{id: string; format: string; source: 'embedded' | 'sidecar' | 'downloaded' | 'other'; language?: string; title?: string; forced: boolean; sdh: boolean; default: boolean}>;
export type VersionPart = Readonly<{id: string; index: number; durationMs: number; sizeBytes?: number}>;
export type MediaVersion = Readonly<{id: string; label: string; edition?: string; container?: string; sizeBytes?: number; durationMs: number; bitrateKbps?: number; parts: readonly VersionPart[]; video: readonly VideoStream[]; audio: readonly AudioStream[]; subtitles: readonly SubtitleStream[]}>;
/** A skippable segment (spec §4.2). `id` is what a skip report names; the viewer's `auto`
 * preference applies only when `automaticSafe`. */
export type Marker = Readonly<{id: string; type: 'intro' | 'recap' | 'credits' | 'preview' | 'commercial' | 'other'; startMs: number; endMs: number; confidence: 'detected' | 'manual' | 'embedded' | 'other'; automaticSafe: boolean}>;
export type Chapter = Readonly<{startMs: number; title: string; thumbnail?: string}>;
export type Trickplay = Readonly<{url: string; intervalMs: number; tileWidth: number; tileHeight: number; columns: number; rows: number}>;
export type StreamAction = 'direct' | 'copy' | 'transcode' | 'burn' | 'drop' | 'other';
export type PlanStream = Readonly<{id: string; action: StreamAction; reasons: readonly string[]}>;
export type PlaybackPlan = Readonly<{versionId: string; mode: 'direct' | 'stream'; streams: readonly PlanStream[]; estimatedBitrateKbps?: number}>;
export type PlaybackOptions = Readonly<{
  itemId: string; kind: string;
  resume?: Readonly<{positionMs: number; partIndex: number}>;
  versions: readonly MediaVersion[]; chapters: readonly Chapter[]; markers: readonly Marker[]; trickplay?: Trickplay;
  preferred?: Readonly<{versionId: string; audioId?: string; subtitleId?: string; reason: string}>;
  plan?: PlaybackPlan;
}>;

export function parseOptions(v: unknown): PlaybackOptions {
  if (!obj(v)) throw new ContractError('playback options');
  const versions = arr(v.versions).map((x, i): MediaVersion => {
    if (!obj(x)) throw new ContractError(`version ${i}`);
    return Object.freeze({
      id: id(x.id, 'version id'), label: str(x.label), edition: typeof x.edition === 'string' ? x.edition : undefined, container: typeof x.container === 'string' ? x.container : undefined,
      sizeBytes: optNum(x.sizeBytes), durationMs: num(x.durationMs), bitrateKbps: optNum(x.bitrateKbps),
      parts: Object.freeze(arr(x.parts).filter(obj).map(p => Object.freeze({id: id(p.id, 'part id'), index: num(p.index), durationMs: num(p.durationMs), sizeBytes: optNum(p.sizeBytes)}))),
      video: Object.freeze(arr(x.video).filter(obj).map(s => Object.freeze({id: id(s.id, 'video id'), codec: str(s.codec), profile: typeof s.profile === 'string' ? s.profile : undefined, bitDepth: optNum(s.bitDepth), width: optNum(s.width), height: optNum(s.height), fps: optNum(s.fps), hdr: typeof s.hdr === 'string' ? s.hdr : undefined, bitrateKbps: optNum(s.bitrateKbps)}))),
      audio: Object.freeze(arr(x.audio).filter(obj).map(s => Object.freeze({id: id(s.id, 'audio id'), codec: str(s.codec), channels: optNum(s.channels), layout: typeof s.layout === 'string' ? s.layout : undefined, atmos: bool(s.atmos), language: typeof s.language === 'string' ? s.language : undefined, title: typeof s.title === 'string' ? s.title : undefined, default: bool(s.default), commentary: bool(s.commentary), visualImpaired: bool(s.visualImpaired)}))),
      subtitles: Object.freeze(arr(x.subtitles).filter(obj).map(s => Object.freeze({id: id(s.id, 'subtitle id'), format: str(s.format), source: oneOf(s.source, ['embedded', 'sidecar', 'downloaded'] as const, 'other' as never), language: typeof s.language === 'string' ? s.language : undefined, title: typeof s.title === 'string' ? s.title : undefined, forced: bool(s.forced), sdh: bool(s.sdh), default: bool(s.default)}))),
    });
  });
  const plan = obj(v.plan) ? Object.freeze({
    versionId: id(v.plan.versionId, 'plan version'), mode: oneOf(v.plan.mode, ['direct', 'stream'] as const, 'stream'),
    streams: Object.freeze(arr(v.plan.streams).filter(obj).map(s => Object.freeze({id: str(s.id), action: oneOf(s.action, ['direct', 'copy', 'transcode', 'burn', 'drop'] as const, 'other' as never), reasons: Object.freeze(arr(s.reasons).filter((r): r is string => typeof r === 'string'))}))),
    estimatedBitrateKbps: optNum(v.plan.estimatedBitrateKbps),
  }) : undefined;
  const t = obj(v.trickplay) ? v.trickplay : undefined;
  return Object.freeze({
    itemId: id(v.itemId, 'item id'), kind: str(v.kind, 'other'),
    resume: obj(v.resume) ? Object.freeze({positionMs: num(v.resume.positionMs), partIndex: num(v.resume.partIndex)}) : undefined,
    versions: Object.freeze(versions),
    chapters: Object.freeze(arr(v.chapters).filter(obj).map(c => Object.freeze({startMs: num(c.startMs), title: str(c.title), thumbnail: typeof c.thumbnail === 'string' ? c.thumbnail : undefined}))),
    markers: Object.freeze(arr(v.markers).filter(obj).map(m => Object.freeze({id: str(m.id), type: oneOf(m.type, ['intro', 'recap', 'credits', 'preview', 'commercial'] as const, 'other' as never), startMs: num(m.startMs), endMs: num(m.endMs), confidence: oneOf(m.confidence, ['detected', 'manual', 'embedded'] as const, 'other' as never), automaticSafe: bool(m.automaticSafe)}))),
    trickplay: t ? Object.freeze({url: url(t.url, 'trickplay url'), intervalMs: num(t.intervalMs), tileWidth: num(t.tileWidth), tileHeight: num(t.tileHeight), columns: num(t.columns), rows: num(t.rows)}) : undefined,
    preferred: obj(v.preferred) ? Object.freeze({versionId: id(v.preferred.versionId, 'preferred version'), audioId: optId(v.preferred.audioId, 'preferred audio'), subtitleId: optId(v.preferred.subtitleId, 'preferred subtitle'), reason: str(v.preferred.reason)}) : undefined,
    plan,
  });
}

// ── Sessions (spec §5) ──────────────────────────────────────────────
export type Decision = Readonly<{action: StreamAction; reasons: readonly string[]; to?: string; channels?: number}>;
export type Presentation = Readonly<{
  generation: number; mode: 'direct' | 'stream'; url: string; startPositionMs: number;
  subtitles: readonly Readonly<{trackId: string; format: string; url: string}>[];
  decision: Readonly<{video?: Decision; audio?: Decision; subtitles?: Decision}>;
  bitrateKbps?: number;
  hdr?: Readonly<{source?: string; delivered?: string; toneMapped: boolean}>;
  /** The version 2 plan (spec §18.1): the client decodes the original file (or a FLAC/Opus
   * conversion) with the server's exact trim and gains. */
  audio?: AudioRenderV2;
}>;
export type SessionKind = 'vod' | 'audio' | 'live' | 'channel' | 'other';
export type Session = Readonly<{
  id: string; revision: string; kind: SessionKind; role: 'local' | 'receiver' | 'group_member' | 'other';
  state: 'playing' | 'paused' | 'ended' | 'other'; itemId?: string; versionId?: string;
  queue?: Readonly<{queueId: string; entryId: string}>;
  lease: Readonly<{expiresAt?: string; reportEveryMs: number}>;
  presentation: Presentation;
  /** Why an ended session ended (spec §14): "terminated" by an administrator with their message,
   * "stopped", "transferred", "lease_expired", "replaced"… Absent until it ends. */
  end?: SessionEnd;
}>;

/** Why the server ended a session, and what the viewer is told. */
export type SessionEnd = Readonly<{reason: string; message?: string}>;

/** A server end from a `session.updated` payload or a session's `end`; undefined if it has none. */
export function parseSessionEnd(v: unknown): SessionEnd | undefined {
  if (!obj(v) || typeof v.reason !== 'string' || v.reason === '') return undefined;
  return Object.freeze({reason: v.reason, message: typeof v.message === 'string' && v.message !== '' ? v.message : undefined});
}

function decision(v: unknown): Decision | undefined {
  if (!obj(v)) return undefined;
  return Object.freeze({action: oneOf(v.action, ['direct', 'copy', 'transcode', 'burn', 'drop'] as const, 'other' as never), reasons: Object.freeze(arr(v.reasons).filter((r): r is string => typeof r === 'string')), to: typeof v.to === 'string' ? v.to : undefined, channels: optNum(v.channels)});
}

export function parseSession(v: unknown): Session {
  if (!obj(v) || !obj(v.presentation)) throw new ContractError('session');
  return Object.freeze({
    id: id(v.id, 'session id'), revision: revision(v.revision), kind: oneOf(v.kind, ['vod', 'audio', 'live', 'channel'] as const, 'other' as never),
    role: oneOf(v.role, ['local', 'receiver', 'group_member'] as const, 'other' as never), state: oneOf(v.state, ['playing', 'paused', 'ended'] as const, 'other' as never),
    itemId: optId(v.itemId, 'item id'), versionId: optId(v.versionId, 'version id'),
    queue: obj(v.queue) ? Object.freeze({queueId: id(v.queue.queueId, 'queue id'), entryId: id(v.queue.entryId, 'entry id')}) : undefined,
    lease: Object.freeze({expiresAt: obj(v.lease) && typeof v.lease.expiresAt === 'string' ? v.lease.expiresAt : undefined, reportEveryMs: obj(v.lease) ? num(v.lease.reportEveryMs, 10_000) : 10_000}),
    presentation: parsePresentation(v.presentation),
    end: parseSessionEnd(v.end),
  });
}

/** A presentation (§5.1). A prepared next entry's (§18.2) has no media URL until committed. */
export function parsePresentation(p: unknown, prepared = false): Presentation {
  if (!obj(p)) throw new ContractError('presentation');
  const generation = p.generation;
  if (typeof generation !== 'number' || !Number.isSafeInteger(generation) || generation < 0) throw new ContractError('generation');
  const d = obj(p.decision) ? p.decision : {};
  return Object.freeze({
    generation, mode: oneOf(p.mode, ['direct', 'stream'] as const, 'stream'), url: prepared && (p.url === undefined || p.url === '') ? '' : url(p.url, 'presentation url'), startPositionMs: num(p.startPositionMs),
    subtitles: Object.freeze(arr(p.subtitles).filter(obj).map(s => Object.freeze({trackId: id(s.trackId, 'subtitle track'), format: str(s.format), url: url(s.url, 'subtitle url')}))),
    decision: Object.freeze({video: decision(d.video), audio: decision(d.audio), subtitles: decision(d.subtitles)}),
    bitrateKbps: optNum(p.bitrateKbps),
    hdr: obj(p.hdr) ? Object.freeze({source: typeof p.hdr.source === 'string' ? p.hdr.source : undefined, delivered: typeof p.hdr.delivered === 'string' ? p.hdr.delivered : undefined, toneMapped: bool(p.hdr.toneMapped)}) : undefined,
    audio: parseAudioRenderV2(p.audioRender),
  });
}

/** `:prepare-next`'s answer (spec §18.2). The plan is required: without one there is no edge. */
export function parsePreparedNext(v: unknown): PreparedNext {
  if (!obj(v) || typeof v.expiresAt !== 'string' || !Number.isFinite(Date.parse(v.expiresAt))) throw new ContractError('prepared next');
  const presentation = parsePresentation(v.presentation, true);
  if (!playableAudio(presentation.audio)) throw new ContractError('prepared next render plan');
  return Object.freeze({token: id(v.token, 'prepared token'), expiresAt: v.expiresAt, entryId: id(v.entryId, 'entry id'), itemId: id(v.itemId, 'item id'), presentation});
}

// ── Queues (spec §8, ARCH-MEDIA-03) ─────────────────────────────────
export type Selector =
  | Readonly<{container: Readonly<{kind: string; id: string}>}>
  | Readonly<{query: Readonly<Record<string, unknown>>}>
  | Readonly<{items: Readonly<{ids: readonly string[]}>}>;
export type QueueSegment = Readonly<{id: string; kind: 'items' | 'selector' | 'other'; label: string; count: number; state: 'building' | 'ready' | 'failed' | 'other'}>;
export type QueueHeader = Readonly<{
  id: string; revision: string; total: number; current?: Readonly<{entryId: string; position: number}>;
  repeat: 'off' | 'one' | 'all'; shuffle?: Readonly<{seed: string; lap: number}>; segments: readonly QueueSegment[];
  /** What plays after the current entry, through the viewer's fence (spec §18.5). */
  next?: Readonly<{entryId: string | null; available: boolean; reason: 'ready' | 'end' | 'unavailable'}>;
  /** The viewer's post-play policy for this queue (spec §18.5). */
  postPlay?: Readonly<{autoplay: boolean; countdownSeconds: number; passoutCheckDue: boolean; automaticAdvances: number}>;
}>;

/** A private preparation of the next entry's audio (spec §18.2): frame 0 only until committed. */
export type PreparedNext = Readonly<{token: string; expiresAt: string; entryId: string; itemId: string; presentation: Presentation}>;
export type QueueEntry = Readonly<{entryId: string; position: number; itemId: string; kind: string; title: string; durationMs?: number; available: boolean; artwork?: unknown;
  /** A song's album (a book file's book), with its disc and track (spec §18.5); absent when unknown. */
  albumId?: string; disc?: number; track?: number}>;

/**
 * Whether `next` follows `current` on the same album, so the engine joins them gaplessly rather
 * than crossfading (Plexamp's behavior, Plan §9.6): the next track on the disc, or the first track
 * of the next disc. With track numbers unknown, being on the same album is enough.
 */
export function consecutiveOnAlbum(current: QueueEntry | undefined, next: QueueEntry | undefined): boolean {
  if (!current?.albumId || current.albumId !== next?.albumId) return false;
  if (!current.track || !next.track) return true;
  const disc = current.disc ?? 1, nextDisc = next.disc ?? 1;
  return nextDisc === disc && next.track === current.track + 1 || nextDisc === disc + 1 && next.track === 1;
}

export function parseQueue(v: unknown): QueueHeader {
  if (!obj(v)) throw new ContractError('queue');
  const total = v.total;
  if (typeof total !== 'number' || !Number.isSafeInteger(total) || total < 0) throw new ContractError('queue total');
  return Object.freeze({
    id: id(v.id, 'queue id'), revision: revision(v.revision), total,
    current: obj(v.current) ? Object.freeze({entryId: id(v.current.entryId, 'current entry'), position: num(v.current.position)}) : undefined,
    repeat: oneOf(v.repeat, ['off', 'one', 'all'] as const, 'off'),
    shuffle: obj(v.shuffle) ? Object.freeze({seed: str(v.shuffle.seed), lap: num(v.shuffle.lap)}) : undefined,
    segments: Object.freeze(arr(v.segments).filter(obj).map(s => Object.freeze({id: id(s.id, 'segment id'), kind: oneOf(s.kind, ['items', 'selector'] as const, 'other' as never), label: str(s.label), count: num(s.count), state: oneOf(s.state, ['building', 'ready', 'failed'] as const, 'other' as never)}))),
    next: obj(v.next) ? Object.freeze({entryId: typeof v.next.entryId === 'string' ? id(v.next.entryId, 'next entry') : null, available: v.next.available === true, reason: oneOf(v.next.reason, ['ready', 'end', 'unavailable'] as const, 'end')}) : undefined,
    postPlay: obj(v.postPlay) ? Object.freeze({autoplay: v.postPlay.autoplay !== false, countdownSeconds: num(v.postPlay.countdownSeconds, 10), passoutCheckDue: v.postPlay.passoutCheckDue === true, automaticAdvances: num(v.postPlay.automaticAdvances)}) : undefined,
  });
}

export function parseEntry(v: unknown): QueueEntry {
  if (!obj(v)) throw new ContractError('queue entry');
  const available = v.available !== false;
  // A placeholder carries no item: one that became unavailable, or (kind "pending") one still
  // being snapshotted in a large queue that fills in shortly (P16).
  return Object.freeze({entryId: id(v.entryId, 'entry id'), position: num(v.position), itemId: available ? id(v.itemId, 'item id') : typeof v.itemId === 'string' ? v.itemId : '', kind: str(v.kind, 'other'), title: str(v.title), durationMs: optNum(v.durationMs), available, artwork: v.artwork,
    ...(typeof v.albumId === 'string' && v.albumId ? {albumId: v.albumId} : {}), ...(optNum(v.disc) ? {disc: v.disc as number} : {}), ...(optNum(v.track) ? {track: v.track as number} : {})});
}

/** The style guide's collection envelope (§4.5). */
export function parsePage<T>(v: unknown, item: (x: unknown) => T): Readonly<{items: readonly T[]; nextCursor?: string; total?: number; start?: number; revision?: string}> {
  if (!obj(v) || !Array.isArray(v.items)) throw new ContractError('collection');
  const page = obj(v.page) ? v.page : {};
  return Object.freeze({items: Object.freeze(v.items.map(item)), nextCursor: typeof page.nextCursor === 'string' ? page.nextCursor : undefined, total: optNum(page.total), start: optNum(page.start), revision: typeof page.revision === 'string' ? page.revision : undefined});
}

// ── Devices, commands, transfers (spec §9) ──────────────────────────
export type ControllableDevice = Readonly<{id: string; name: string; form: string; nowPlaying?: Readonly<{itemId?: string; title?: string; state?: string; positionMs?: number}>}>;
export function parseDevice(v: unknown): ControllableDevice {
  if (!obj(v)) throw new ContractError('device');
  const np = obj(v.nowPlaying) ? v.nowPlaying : undefined;
  return Object.freeze({id: id(v.id, 'device id'), name: str(v.name), form: str(v.form, 'other'), nowPlaying: np ? Object.freeze({itemId: typeof np.itemId === 'string' ? np.itemId : undefined, title: typeof np.title === 'string' ? np.title : undefined, state: typeof np.state === 'string' ? np.state : undefined, positionMs: optNum(np.positionMs)}) : undefined});
}

// ── Admin Now Playing (spec §14) ────────────────────────────────────
export type AdminSession = Readonly<{
  id: string; user: string; profile: string; device: string; itemId?: string; title: string; kind: SessionKind; state: string; positionMs: number;
  decision: Readonly<{video?: Decision; audio?: Decision; subtitles?: Decision}>; bitrateKbps?: number; bandwidthKbps?: number;
  transcode?: Readonly<{speed?: number; hardware: boolean; throttled: boolean}>; location: 'local' | 'remote' | 'other'; startedAt?: string;
}>;
export function parseAdminSession(v: unknown): AdminSession {
  if (!obj(v)) throw new ContractError('admin session');
  const d = obj(v.decision) ? v.decision : {};
  const named = (x: unknown) => (obj(x) ? str(x.name, str(x.id)) : str(x));
  return Object.freeze({
    id: id(v.id, 'session id'), user: named(v.user), profile: named(v.profile), device: named(v.device),
    itemId: obj(v.item) && typeof v.item.id === 'string' ? v.item.id : typeof v.itemId === 'string' ? v.itemId : undefined,
    title: obj(v.item) ? str(v.item.title) : str(v.title),
    kind: oneOf(v.kind, ['vod', 'audio', 'live', 'channel'] as const, 'other' as never), state: str(v.state), positionMs: num(v.positionMs),
    decision: Object.freeze({video: decision(d.video), audio: decision(d.audio), subtitles: decision(d.subtitles)}),
    bitrateKbps: optNum(v.bitrateKbps), bandwidthKbps: optNum(v.bandwidthKbps),
    transcode: obj(v.transcode) ? Object.freeze({speed: optNum(v.transcode.speed), hardware: bool(v.transcode.hardware), throttled: bool(v.transcode.throttled)}) : undefined,
    location: oneOf(v.location, ['local', 'remote'] as const, 'other' as never), startedAt: typeof v.startedAt === 'string' ? v.startedAt : undefined,
  });
}

// ── Events (style guide amendment 8; Public API ARCH-API-07) ────────
export type ServerEvent = Readonly<{id: string; type: string; at?: string; resource?: Readonly<{kind: string; id: string}>; revision?: string; data?: unknown}>;
export function parseEvent(v: unknown): ServerEvent | undefined {
  if (!obj(v) || typeof v.id !== 'string' || !v.id || v.id.length > 128 || typeof v.type !== 'string' || !v.type) return undefined;
  return Object.freeze({id: v.id, type: v.type, at: typeof v.at === 'string' ? v.at : undefined, resource: obj(v.resource) && typeof v.resource.kind === 'string' && typeof v.resource.id === 'string' ? Object.freeze({kind: v.resource.kind, id: v.resource.id}) : undefined, revision: typeof v.revision === 'string' ? v.revision : undefined, data: v.data});
}
