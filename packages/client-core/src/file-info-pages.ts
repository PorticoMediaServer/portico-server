import {parseDiscMediaFacts, type DiscMediaFacts} from './disc-media-facts.ts';
import type {DetailScope, DetailTarget, FileInfoDiscTitle, TitleFile, TitleFileTrack} from './detail.ts';
import {FileInfoWindowError, type FileInfoWindowFacts} from './file-info-windows.ts';
import type {PreparationResponse, PreparingRead} from './read-preparation.ts';
import {wireDisplayDurations} from './wire-units.ts';

/** The accepted Detail a FileInfo opening was made from. Every lane echoes it unchanged; a
 * window carrying another revision belongs to another opening and is refused. */
export type FileInfoOpening = Readonly<{scope: DetailScope; target: DetailTarget; revision: Readonly<{catalog: number; viewer: number}>}>;
export type FileInfoManage = Readonly<{delete: boolean; split: boolean; merge: boolean}>;
/** Facts of the complete accepted surface, never of the resident window. */
export type FileInfoSurface = Readonly<{
  fileCount: number; eligibleFileCount: number; groupCount: number;
  canSplit: boolean; hasDiscColumns: boolean; ownerDiscOccurrence?: string; manage?: FileInfoManage;
}>;
type Video = NonNullable<TitleFile['video']>;
/** One exact file occurrence. It grants nothing: every command revalidates current rights. */
export type FileInfoFileRow = Readonly<{
  occurrence: string; itemId: string; id: string; path?: string; size: number; container: string;
  duration?: number; bitRate?: number; available: boolean; partIndex: number; edition?: string;
  video?: Video; disc?: DiscMediaFacts; audioCount: number; subtitleCount: number;
}>;
/** One version: the exact Options grouping, informational scalars and lane counts only. */
export type FileInfoGroupRow = Readonly<{
  id: string; headOccurrence: string; ordinal: number; memberCount: number; edition?: string; container: string;
  sizeBytes: number; durationMs: number; video?: Video; disc?: DiscMediaFacts; audioCount: number; subtitleCount: number;
}>;
export type FileInfoTrackRow = TitleFileTrack & Readonly<{ordinal: number}>;
export type FileInfoSourceRow = Readonly<{occurrence: string; itemId: string; id: string; container: string; videoCodec: string; audioCodec: string; width: number; height: number; duration: number}>;
type Basis = Readonly<{basis: string; window: FileInfoWindowFacts}>;
export type FileInfoFilesPage = Basis & Readonly<{kind: string; itemKind: string; surface: FileInfoSurface; files: readonly FileInfoFileRow[]}>;
export type FileInfoGroupsPage = Basis & Readonly<{kind: string; surface: FileInfoSurface; groups: readonly FileInfoGroupRow[]}>;
export type FileInfoMembersPage = Basis & Readonly<{kind: string; groupId: string; total: number; members: readonly FileInfoFileRow[]}>;
export type FileInfoTracksPage = Basis & Readonly<{kind: string; lane: string; occurrence: string; total: number; tracks: readonly FileInfoTrackRow[]}>;
export type FileInfoSourcesPage = Basis & Readonly<{kind: string; total: number; sources: readonly FileInfoSourceRow[]}>;
export type FileInfoDiscPage = Basis & Readonly<{kind: string; occurrence: string; discKind: 'bluray' | 'dvd'; total: number; titles: readonly FileInfoDiscTitle[]}>;
export type FileInfoPreparing = Readonly<{kind: string; state: 'preparing'; queryId: string; retryAfterMs: number}>;

export const FILE_INFO_DEFAULT_LIMIT = 40, FILE_INFO_MAXIMUM_LIMIT = 100;
const obj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const opaque = (v: unknown): v is string => typeof v === 'string' && /^[A-Za-z0-9._~-]{1,1024}$/.test(v);
const count = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0;
const finite = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v) && v >= 0;
/** Short identifiers and codes keep their bound and control-character rule. */
const short = (v: unknown, max = 256): v is string => typeof v === 'string' && v.length <= max && !/[\x00-\x1f\x7f]/.test(v);
/** Legal stored text (paths, editions, track titles) arrives whole at any length; only NUL is refused. */
const text = (v: unknown): v is string => typeof v === 'string' && !v.includes('\x00');
function invalid(response?: PreparationResponse): never { throw new FileInfoWindowError('invalid_file_info_window', response); }

function rows(value: unknown, window: FileInfoWindowFacts, response?: PreparationResponse): unknown[] {
  if (!Array.isArray(value) || value.length !== window.returned) invalid(response);
  return value;
}
function windowFacts(value: unknown, basis: string, response?: PreparationResponse): FileInfoWindowFacts {
  if (!obj(value) || !opaque(value.queryId) || !count(value.limit) || value.limit < 1 || value.limit > FILE_INFO_MAXIMUM_LIMIT
    || !count(value.returned) || value.returned > value.limit) invalid(response);
  const cursor = (key: 'nextCursor' | 'previousCursor' | 'reloadCursor') => {
    const token = value[key];
    if (token === undefined || token === '') return undefined;
    if (!opaque(token)) invalid(response);
    return token;
  };
  const next = cursor('nextCursor'), previous = cursor('previousCursor'), reload = cursor('reloadCursor');
  return Object.freeze({queryId: value.queryId, basis, limit: value.limit, returned: value.returned, ...(next ? {next} : {}), ...(previous ? {previous} : {}), ...(reload ? {reload} : {})});
}
/** The common envelope: this server, item and viewer fence, the opening's own Detail revision. */
function envelope(raw: unknown, opening: FileInfoOpening, response?: PreparationResponse): Record<string, unknown> & Basis {
  if (!obj(raw) || !obj(raw.scope) || raw.scope.serverId !== opening.scope.serverId || raw.scope.libraryId !== opening.target.libraryId
    || raw.scope.itemId !== opening.target.itemId || !short(raw.scope.viewerFence) || !raw.scope.viewerFence
    || !obj(raw.detail) || !obj(raw.detail.revision) || raw.detail.revision.catalog !== opening.revision.catalog
    || raw.detail.revision.viewer !== opening.revision.viewer || !opaque(raw.basis) || !short(raw.kind, 64)) invalid(response);
  return Object.assign({}, raw, {basis: raw.basis, window: windowFacts(raw.window, raw.basis, response)});
}
function manage(value: unknown, response?: PreparationResponse): FileInfoManage | undefined {
  if (value === undefined) return undefined;
  if (!obj(value) || typeof value.delete !== 'boolean' || typeof value.split !== 'boolean' || typeof value.merge !== 'boolean') invalid(response);
  return Object.freeze({delete: value.delete, split: value.split, merge: value.merge});
}
function surface(value: unknown, response?: PreparationResponse): FileInfoSurface {
  if (!obj(value) || !count(value.fileCount) || !count(value.eligibleFileCount) || !count(value.groupCount)
    || typeof value.canSplit !== 'boolean' || typeof value.hasDiscColumns !== 'boolean'
    || value.ownerDiscOccurrence !== undefined && !occurrence(value.ownerDiscOccurrence)) invalid(response);
  const owner = manage(value.manage, response);
  // Owner-only facts without the owner's manage block mean the audience fence was not applied.
  if (!owner && (value.canSplit || value.ownerDiscOccurrence !== undefined)) invalid(response);
  return Object.freeze({fileCount: value.fileCount, eligibleFileCount: value.eligibleFileCount, groupCount: value.groupCount, canSplit: value.canSplit, hasDiscColumns: value.hasDiscColumns,
    ...(value.ownerDiscOccurrence !== undefined ? {ownerDiscOccurrence: value.ownerDiscOccurrence as string} : {}), ...(owner ? {manage: owner} : {})});
}
/** `<parent public id>.<asset token>`: exactly one separator, neither side empty. */
export function occurrence(value: unknown): value is string {
  if (typeof value !== 'string' || value.length > 1024) return false;
  const at = value.indexOf('.');
  return at > 0 && at < value.length - 1 && value.indexOf('.', at + 1) === -1 && short(value) && !/\s/.test(value);
}
function video(value: unknown, response?: PreparationResponse): Video | undefined {
  if (value === undefined) return undefined;
  if (!obj(value) || !short(value.codec, 64) || !count(value.width) || !count(value.height)) invalid(response);
  return Object.freeze({codec: value.codec, width: value.width, height: value.height,
    ...(short(value.dynamicRange, 32) && value.dynamicRange ? {dynamicRange: value.dynamicRange} : {}), ...(value.hdr10Plus === true ? {hdr10Plus: true} : {}),
    ...(finite(value.frameRate) && value.frameRate > 0 ? {frameRate: value.frameRate} : {}), ...(count(value.bitDepth) && value.bitDepth > 0 ? {bitDepth: value.bitDepth} : {}),
    ...(count(value.bitRate) && value.bitRate > 0 ? {bitRate: value.bitRate} : {})}) as Video;
}
function file(raw: unknown, owner: boolean, response?: PreparationResponse): FileInfoFileRow {
  const value = wireDisplayDurations(raw, {duration: 'durationMs'}, () => invalid(response));
  if (!obj(value) || !occurrence(value.occurrence) || !short(value.itemId) || !value.itemId || !short(value.id) || !value.id
    || value.occurrence !== `${value.itemId}.${value.id}` || !count(value.size) || !short(value.container ?? '', 64) || typeof value.available !== 'boolean'
    || !count(value.partIndex) || !count(value.audioCount) || !count(value.subtitleCount)
    || value.path !== undefined && (!owner || !text(value.path) || !value.path) || value.edition !== undefined && !text(value.edition)) invalid(response);
  const facts = video(value.video, response), disc = parseDiscMediaFacts(value.disc);
  if (value.disc !== undefined && (!owner || !disc)) invalid(response);
  return Object.freeze({occurrence: value.occurrence, itemId: value.itemId, id: value.id, ...(value.path ? {path: value.path as string} : {}), size: value.size,
    container: (value.container ?? '') as string, ...(finite(value.duration) ? {duration: value.duration} : {}), ...(count(value.bitRate) && value.bitRate > 0 ? {bitRate: value.bitRate} : {}),
    available: value.available, partIndex: value.partIndex, ...(value.edition ? {edition: value.edition as string} : {}), ...(facts ? {video: facts} : {}), ...(disc ? {disc} : {}),
    audioCount: value.audioCount, subtitleCount: value.subtitleCount});
}
function group(value: unknown, owner: boolean, response?: PreparationResponse): FileInfoGroupRow {
  if (!obj(value) || !opaque(value.id) || !occurrence(value.headOccurrence) || !count(value.ordinal) || value.ordinal < 1 || !count(value.memberCount) || value.memberCount < 1
    || !short(value.container ?? '', 64) || !count(value.sizeBytes) || !count(value.durationMs) || !count(value.audioCount) || !count(value.subtitleCount)
    || value.edition !== undefined && !text(value.edition)) invalid(response);
  const facts = video(value.video, response), disc = parseDiscMediaFacts(value.disc);
  if (value.disc !== undefined && (!owner || !disc)) invalid(response);
  return Object.freeze({id: value.id, headOccurrence: value.headOccurrence, ordinal: value.ordinal, memberCount: value.memberCount, ...(value.edition ? {edition: value.edition as string} : {}),
    container: (value.container ?? '') as string, sizeBytes: value.sizeBytes, durationMs: value.durationMs, ...(facts ? {video: facts} : {}), ...(disc ? {disc} : {}),
    audioCount: value.audioCount, subtitleCount: value.subtitleCount});
}
function track(value: unknown, response?: PreparationResponse): FileInfoTrackRow {
  if (!obj(value) || !count(value.ordinal) || value.ordinal < 1 || !short(value.codec ?? '', 64) || value.title !== undefined && !text(value.title)) invalid(response);
  const label = (v: unknown, max: number) => short(v, max) && v ? v : undefined, positive = (v: unknown) => count(v) && v > 0 ? v : undefined;
  const language = label(value.language, 64), channelLayout = label(value.channelLayout, 64), objectAudio = label(value.objectAudio, 32), channels = positive(value.channels), bitRate = positive(value.bitRate);
  return Object.freeze({ordinal: value.ordinal, codec: (value.codec ?? '') as string, ...(language ? {language} : {}), ...(value.title ? {title: value.title as string} : {}),
    ...(channels ? {channels} : {}), ...(channelLayout ? {channelLayout} : {}), ...(objectAudio ? {objectAudio} : {}), ...(bitRate ? {bitRate} : {}),
    ...(value.default === true ? {default: true} : {}), ...(value.forced === true ? {forced: true} : {}), ...(value.external === true ? {external: true} : {})});
}
/** Ordinals must climb strictly through a window: a gap is fine (another window), a repeat is not. */
function ordered<T extends {ordinal: number}>(list: readonly T[], response?: PreparationResponse): readonly T[] {
  for (let i = 1; i < list.length; i++) if (list[i]!.ordinal <= list[i - 1]!.ordinal) invalid(response);
  return Object.freeze(list);
}
function unique<T>(list: readonly T[], key: (value: T) => string, response?: PreparationResponse): readonly T[] {
  if (new Set(list.map(key)).size !== list.length) invalid(response);
  return Object.freeze(list);
}

export function parseFileInfoFilesPage(raw: unknown, opening: FileInfoOpening, response?: PreparationResponse): FileInfoFilesPage {
  const page = envelope(raw, opening, response), facts = surface(page.surface, response);
  if (!short(page.itemKind, 64) || !page.itemKind) invalid(response);
  const files = unique(rows(page.files, page.window, response).map(value => file(value, !!facts.manage, response)), value => value.occurrence, response);
  return Object.freeze({kind: page.kind as string, basis: page.basis, window: page.window, itemKind: page.itemKind as string, surface: facts, files});
}
export function parseFileInfoGroupsPage(raw: unknown, opening: FileInfoOpening, response?: PreparationResponse): FileInfoGroupsPage {
  const page = envelope(raw, opening, response), facts = surface(page.surface, response);
  const groups = ordered(rows(page.groups, page.window, response).map(value => group(value, !!facts.manage, response)), response);
  if (groups.some(value => value.ordinal > facts.groupCount)) invalid(response);
  return Object.freeze({kind: page.kind as string, basis: page.basis, window: page.window, surface: facts, groups: unique(groups, value => value.id, response)});
}
export function parseFileInfoMembersPage(raw: unknown, opening: FileInfoOpening, groupId: string, owner: boolean, response?: PreparationResponse): FileInfoMembersPage {
  const page = envelope(raw, opening, response);
  if (page.groupId !== groupId || !count(page.total) || page.window.returned > page.total) invalid(response);
  const members = unique(rows(page.members, page.window, response).map(value => file(value, owner, response)), value => value.occurrence, response);
  return Object.freeze({kind: page.kind as string, basis: page.basis, window: page.window, groupId, total: page.total, members});
}
export function parseFileInfoTracksPage(raw: unknown, opening: FileInfoOpening, lane: string, at: string, response?: PreparationResponse): FileInfoTracksPage {
  const page = envelope(raw, opening, response);
  if (page.lane !== lane || page.occurrence !== at || !count(page.total) || page.window.returned > page.total) invalid(response);
  const tracks = ordered(rows(page.tracks, page.window, response).map(value => track(value, response)), response);
  if (tracks.some(value => value.ordinal > (page.total as number))) invalid(response);
  return Object.freeze({kind: page.kind as string, basis: page.basis, window: page.window, lane, occurrence: at, total: page.total, tracks});
}
export function parseFileInfoSourcesPage(raw: unknown, opening: FileInfoOpening, response?: PreparationResponse): FileInfoSourcesPage {
  const page = envelope(raw, opening, response);
  if (!count(page.total) || page.window.returned > page.total) invalid(response);
  const sources = rows(page.sources, page.window, response).map(raw => {
    const value = wireDisplayDurations(raw, {duration: 'durationMs'}, () => invalid(response));
    if (!obj(value) || !occurrence(value.occurrence) || !short(value.itemId) || !short(value.id) || value.occurrence !== `${value.itemId}.${value.id}`
      || !short(value.container ?? '', 64) || !short(value.videoCodec ?? '', 64) || !short(value.audioCodec ?? '', 64) || !count(value.width) || !count(value.height)) invalid(response);
    return Object.freeze({occurrence: value.occurrence, itemId: value.itemId, id: value.id, container: (value.container ?? '') as string, videoCodec: (value.videoCodec ?? '') as string,
      audioCodec: (value.audioCodec ?? '') as string, width: value.width, height: value.height, duration: finite(value.duration) ? value.duration : 0});
  });
  return Object.freeze({kind: page.kind as string, basis: page.basis, window: page.window, total: page.total, sources: unique(sources, value => value.occurrence, response)});
}
/** Owner only: the whole surface's first successful owner-disc file and its titles. */
export function parseFileInfoDiscPage(raw: unknown, opening: FileInfoOpening, response?: PreparationResponse): FileInfoDiscPage {
  const page = envelope(raw, opening, response);
  if (!occurrence(page.occurrence) || (page.discKind !== 'bluray' && page.discKind !== 'dvd') || !count(page.total) || page.window.returned > page.total) invalid(response);
  const titles = rows(page.titles, page.window, response).map(value => {
    if (!obj(value) || !count(value.number) || !count(value.durationMs) || !count(value.chapters) || !count(value.angles)) invalid(response);
    return Object.freeze({number: value.number, durationMs: value.durationMs, chapters: value.chapters, angles: value.angles});
  });
  return Object.freeze({kind: page.kind as string, basis: page.basis, window: page.window, occurrence: page.occurrence, discKind: page.discKind, total: page.total,
    titles: unique(titles, value => String(value.number), response)});
}
export function parseFileInfoPreparing(raw: unknown, opening: FileInfoOpening, response?: PreparationResponse): PreparingRead {
  if (!obj(raw) || raw.state !== 'preparing' || !short(raw.kind, 64) || !opaque(raw.queryId) || !count(raw.retryAfterMs)
    || !obj(raw.detail) || !obj(raw.detail.revision) || raw.detail.revision.catalog !== opening.revision.catalog || raw.detail.revision.viewer !== opening.revision.viewer) invalid(response);
  return Object.freeze({queryId: raw.queryId, retryAfterMs: raw.retryAfterMs});
}
/** The handle of any ready window, read before its rows so a malformed page is still released. */
export function fileInfoReadyQueryId(raw: unknown, response?: PreparationResponse): string | undefined {
  if (!obj(raw) || !obj(raw.window)) invalid(response);
  if (!opaque(raw.window.queryId)) invalid(response);
  return raw.window.queryId;
}
