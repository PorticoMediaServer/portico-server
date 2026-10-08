/**
 * The Portico Cast channel (`urn:x-cast:tv.getportico.cast`), shared by every sender. The receiver
 * is `server/receiver/cast/receiver.js`.
 *
 * The television never receives the sender's sign-in or a media address: it gets a short code the
 * server issued (`POST /v1/cast/bootstrap`), redeems it for its own credential against `origin`,
 * and is then told only which item to play.
 */
export const CAST_NAMESPACE = 'urn:x-cast:tv.getportico.cast';
export const CAST_PROTOCOL_VERSION = '1.0';
/** The TV asks the person watching before a second sender takes over; allow for their answer. */
export const CAST_PAIR_TIMEOUT_MS = 35_000;
/** The TV treats 30 s without an answer to a takeover as no. */
export const CAST_TAKEOVER_WINDOW_MS = 30_000;

/** What the TV shows while it prepares, and what Cast's own player labels the title with. */
export type CastMetadata = Readonly<{title: string; subtitle?: string; seriesTitle?: string; season?: number; episode?: number; kind?: string; artworkPath?: string}>;

export type CastTrack = Readonly<{id: number; language: string; name: string}>;

/** Sender → receiver. */
export type CastCommand =
  | {type: 'pair'; code: string; origin: string; displayName: string}
  | ({type: 'load'; itemId: string; startSeconds: number} & Record<string, string | number>)
  | {type: 'play'} | {type: 'pause'} | {type: 'stop'} | {type: 'status'}
  | {type: 'seek'; positionSeconds: number}
  | {type: 'audio'; trackId: number}
  | {type: 'subtitles'; trackIds: number[]}
  | {type: 'takeover'; allow: boolean};

/** Receiver → sender, validated. Unknown or malformed messages are dropped. */
export type CastEvent =
  | {type: 'paired'}
  | {type: 'pair-pending'}
  | {type: 'pair-failed'; code: string}
  | {type: 'busy'}
  | {type: 'pair-required'}
  | {type: 'load-failed'}
  | {type: 'takeover-requested'; requester: string}
  | {type: 'replaced'}
  | {type: 'status'; itemId: string; positionSeconds: number; durationSeconds: number; paused: boolean; audioTracks: readonly CastTrack[]; textTracks: readonly CastTrack[]};

const finite = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v >= 0 ? v : 0);
const short = (v: unknown, max: number) => (typeof v === 'string' ? v.trim().slice(0, max) : '');
function tracks(v: unknown): readonly CastTrack[] {
  if (!Array.isArray(v)) return [];
  return Object.freeze(v.slice(0, 32).flatMap(t => (t && typeof t === 'object' && Number.isSafeInteger((t as {id?: unknown}).id) ? [Object.freeze({id: (t as {id: number}).id, language: short((t as {language?: unknown}).language, 35), name: short((t as {name?: unknown}).name, 100)})] : [])));
}

/** Parse one receiver message (a JSON string, or an object on SDKs that decode it). */
export function parseCastEvent(raw: unknown): CastEvent | undefined {
  let m: unknown = raw;
  if (typeof raw === 'string') { try { m = JSON.parse(raw); } catch { return undefined; } }
  if (!m || typeof m !== 'object') return undefined;
  const o = m as Record<string, unknown>;
  switch (o.type) {
    case 'paired': case 'pair-pending': case 'busy': case 'pair-required': case 'load-failed': case 'replaced':
      return {type: o.type};
    case 'pair-failed': return {type: 'pair-failed', code: short(o.code, 64) || 'pair_failed'};
    case 'takeover-requested': return {type: 'takeover-requested', requester: short(o.requester, 64)};
    case 'status': return {type: 'status', itemId: short(o.itemId, 128), positionSeconds: finite(o.positionSeconds), durationSeconds: finite(o.durationSeconds), paused: o.paused === true, audioTracks: tracks(o.audioTracks), textTracks: tracks(o.textTracks)};
    default: return undefined;
  }
}

/** Display fields for `load`: only what's present, bounded, integers for season and episode. */
export function castMetadataFields(meta: CastMetadata): Record<string, string | number> {
  const out: Record<string, string | number> = {title: meta.title.slice(0, 200)};
  if (meta.subtitle) out.subtitle = meta.subtitle.slice(0, 200);
  if (meta.seriesTitle) out.seriesTitle = meta.seriesTitle.slice(0, 200);
  if (Number.isInteger(meta.season)) out.season = meta.season!;
  if (Number.isInteger(meta.episode)) out.episode = meta.episode!;
  if (meta.kind) out.kind = meta.kind.slice(0, 32);
  if (meta.artworkPath && meta.artworkPath.startsWith('/v1/')) out.artworkPath = meta.artworkPath.slice(0, 512);
  return out;
}
