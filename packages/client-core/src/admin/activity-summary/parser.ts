import {currentUTCTimestamp, utcOrderKey} from '../../local-session.ts';
import type {OwnerActivitySummary} from './types.ts';

type Reader = (value: unknown) => unknown;
type ObjectValue = Record<string, unknown>;
export class ActivitySummaryContractError extends Error {
  readonly code = 'invalid_activity_summary';
  constructor() { super('invalid_activity_summary'); this.name = 'ActivitySummaryContractError'; }
}
function invalid(): never { throw new ActivitySummaryContractError(); }
const object = (v: unknown): v is ObjectValue => v !== null && typeof v === 'object' && !Array.isArray(v);
const text = (max: number, nonempty = false): Reader => v => typeof v === 'string' && v.length <= max && (!nonempty || v.length > 0) && !/[\x00-\x1f\x7f]/.test(v) ? v : invalid();
const id = text(512, true), label = text(256, true), words = text(4096);
const integer = (max = Number.MAX_SAFE_INTEGER, min = 0): Reader => v => typeof v === 'number' && Number.isSafeInteger(v) && v >= min && v <= max ? v : invalid();
const count = integer();
const bool: Reader = v => typeof v === 'boolean' ? v : invalid();
const timestamp: Reader = v => currentUTCTimestamp(v) ? v : invalid();
/** Reject accidental capability/private additions rather than carrying opaque server objects. */
function record(required: Record<string, Reader>, optional: Record<string, Reader> = {}): Reader {
  return value => {
    if (!object(value) || Object.keys(value).some(k => !Object.hasOwn(required, k) && !Object.hasOwn(optional, k))) invalid();
    const out: ObjectValue = {};
    for (const [key, read] of Object.entries(required)) {
      if (!Object.hasOwn(value, key)) invalid();
      out[key] = read(value[key]);
    }
    for (const [key, read] of Object.entries(optional)) if (Object.hasOwn(value, key)) out[key] = read(value[key]);
    return Object.freeze(out);
  };
}
function list(read: Reader, max = Number.MAX_SAFE_INTEGER, uniqueKey?: string): Reader {
  return value => {
    if (!Array.isArray(value) || value.length > max) invalid();
    const out = value.map(read);
    if (uniqueKey && new Set(out.map(v => (v as ObjectValue)[uniqueKey])).size !== out.length) invalid();
    return Object.freeze(out);
  };
}
const named = record({id, name: words});
const decision = record({action: label, reasons: list(text(512, true), 128)}, {to: label, channels: integer(256, 1)});
const decisions = record({}, {video: decision, audio: decision, subtitles: decision});
const art = record({id, kind: label, role: label, revision: id});
const airDate: Reader = v => typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v) && currentUTCTimestamp(v + 'T00:00:00Z') ? v : invalid();
const item = record({id, title: words, kind: label, art: list(art, 4)}, {
  showId: id, showTitle: words, seasonNumber: count, episodeNumber: count,
  episodeNumbering: label, airDate, year: integer(9999, 1), artist: words, album: words,
});
const qualityShape = record({mode: label}, {id: label, maxHeight: integer(8640, 1), maxVideoBitrateKbps: integer(2_000_000, 1), maxAudioBitrateKbps: integer(10_000, 1)});
const quality: Reader = v => {
  const out = qualityShape(v) as ObjectValue;
  const numbers = ['maxHeight', 'maxVideoBitrateKbps', 'maxAudioBitrateKbps'].some(k => Object.hasOwn(out, k));
  if (!['original', 'fixed', 'automatic', 'limit'].includes(out.mode as string) ||
      out.mode === 'original' && (numbers || out.id !== undefined) ||
      out.mode === 'fixed' && (numbers || out.id === undefined) ||
      out.mode === 'automatic' && numbers || out.mode === 'limit' && out.id !== undefined) invalid();
  return out;
};
const username: Reader = value => {
  const out = text(256, true)(value) as string;
  return out.trim() ? out : invalid();
};
const sessionShape = record({id, user: record({id}, {username}), profile: named, device: named, kind: label, state: label,
  startedAt: timestamp, problem: bool, converting: bool, positionMs: count, decision: decisions},
  {durationMs: integer(Number.MAX_SAFE_INTEGER, 1), item, quality, videoHeight: integer(Number.MAX_SAFE_INTEGER, 1)});
const session: Reader = value => {
  const out = sessionShape(value) as ObjectValue;
  const decision = out.decision as {video?: {action: string}; audio?: {action: string}; subtitles?: {action: string}};
  const converting = ['transcode', 'burn'].includes(decision.video?.action ?? '') || decision.audio?.action === 'transcode' || decision.subtitles?.action === 'burn';
  if (out.problem !== ['error', 'buffering'].includes(out.state as string) || out.converting !== converting) invalid();
  return out;
};
const streams: Reader = value => {
  const out = record({count, convertingCount: count, overflow: record({count, convertingCount: count}), items: list(session, 3, 'id')})(value) as ObjectValue;
  const items = out.items as ObjectValue[], total = out.count as number, converting = out.convertingCount as number;
  const overflow = out.overflow as {count: number; convertingCount: number};
  if (items.length !== Math.min(total, 3) || converting > total || overflow.count !== total - items.length ||
      overflow.convertingCount !== converting - items.filter(item => item.converting).length || overflow.convertingCount > overflow.count) invalid();
  const priority = (item: ObjectValue) => item.problem ? 0 : item.converting ? 1 : 2;
  for (let n = 1; n < items.length; n++) {
    const previous = items[n - 1], current = items[n], p = priority(previous), c = priority(current);
    const pt = utcOrderKey(previous.startedAt as string), ct = utcOrderKey(current.startedAt as string);
    if (p > c || p === c && (pt < ct || pt === ct && (previous.id as string) > (current.id as string))) invalid();
  }
  return out;
};
const relativeFile: Reader = v => {
  const path = text(4096, true)(v) as string;
  if (/^(?:[/\\]|[A-Za-z]:)/.test(path) || path.split(/[/\\]/).some(p => p === '..') || path === '.') invalid();
  return path;
};
const scanShape = record({libraryId: id, libraryName: words, kind: label, discovered: count, analyzed: count, phase: label}, {total: count, current: relativeFile});
const scan: Reader = v => {
  const out = scanShape(v) as ObjectValue;
  if ((out.analyzed as number) > (out.discovered as number) || out.total !== undefined && out.total !== out.discovered) invalid();
  return out;
};
const taskShape = record({id, kind: label, state: label, phase: words}, {libraryId: id, libraryName: words, processed: count, total: count, unit: label});
const task: Reader = v => {
  const out = taskShape(v) as ObjectValue;
  if (out.total !== undefined && (out.processed === undefined || (out.processed as number) > (out.total as number))) invalid();
  return out;
};
const capped = (read: Reader, cap: number): Reader => value => {
  const out = record({count, items: list(read, cap, 'id')})(value) as ObjectValue;
  if ((out.items as unknown[]).length !== Math.min(out.count as number, cap)) invalid();
  return out;
};
const updateShape = record({available: bool, state: label, current: text(256)}, {latest: text(256, true)});
const update: Reader = v => {
  const out = updateShape(v) as ObjectValue;
  if (!['unconfigured', 'unavailable', 'current', 'available'].includes(out.state as string) || out.available !== (out.state === 'available') || out.available && out.latest === undefined) invalid();
  return out;
};
const summary = record({
  scope: record({serverId: id, viewerFence: text(64, true)}), observedAt: timestamp, serverName: words,
  streams, scans: list(scan, Number.MAX_SAFE_INTEGER, 'libraryId'), tasks: capped(task, 3),
  update, health: record({degraded: bool}),
}, {lastScanFinishedAt: timestamp});

/** One current owner document; no tolerant defaults, joined captions or invented denominators. */
export function parseActivitySummary(value: unknown): OwnerActivitySummary {
  const out = summary(value) as OwnerActivitySummary;
  if (!/^[a-f0-9]{64}$/.test(out.scope.viewerFence)) invalid();
  return out;
}
