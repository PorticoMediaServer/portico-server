import {unreadableServerResponse} from './server-messages.ts';
import {parseLocalBook,parseLocalAudioPolicy,type LocalAudioPolicy,type LocalBookMetadata} from "./music-metadata.ts";
/** Server-authored detail and explicit personal-state commands; no playback or ranking owner. */
import type { MediaItem } from './index.ts';
import {validateRelatedMovies,type RelatedMovieProjection} from './related-movies.ts';
import {parseSegmentMarkers,type SegmentMarker} from './segment-markers.ts';
import {validateContentEntry,type ContentEntry} from './library-content.ts';
export type DetailExtraType = 'trailer' | 'featurette' | 'deleted_scene' | 'behind_the_scenes' | 'interview' | 'scene' | 'short' | 'other';
export type DetailExtra = Readonly<{ type: DetailExtraType; label: string; items: readonly ContentEntry[] }>;
export const detailExtraTypes: readonly DetailExtraType[] = ['trailer','featurette','deleted_scene','behind_the_scenes','interview','scene','short','other'];
export type DetailScope = Readonly<{ serverId: string; viewerId: string }>;
export type DetailTarget = Readonly<{ libraryId: string; itemId: string }>;
export interface DetailApi { request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> }
export type PersonalConflict = Readonly<{id:string;field:string;baseRevision:number;baseValue:unknown;choices:readonly Readonly<{operationId:string;deviceId:string;authoredAt:string;value:unknown}>[]}>;
export type PersonalOfflineMutation = Readonly<{deviceId:string;deviceMutationId:string;sequence:number;baseRevision:number;authoredAt:string}>;
export type PersonalState = Readonly<{ watchlisted:boolean;favorite:boolean;rating:number|null;revision:number;watched:boolean;progressSeconds:number;lastPlayedAt:string;status:'current'|'needs-resolution';conflicts:readonly PersonalConflict[] }>;
export type DetailAction = Readonly<{ id: string; labelKey: string; enabled: boolean; min?: number; max?: number; step?: number; playback?: Readonly<{ itemId: string; startSeconds: number }> }>;
export type DetailMetadata = Readonly<{
  sources?: readonly Readonly<{provider:string;sourceUrl:string;observedAt:string}>[];
  attributions?: readonly string[];
  status: 'available' | 'unavailable';
  ratings: readonly Readonly<{ provider: string; value: number; max: number; votes?: number; sourceUrl: string; observedAt: string }>[];
  genres: readonly Readonly<{ id: string; name: string; provider: string }>[];
  /** The cast's first page and the key crew; `creditTotals` counts both groups, and parseCreditPage reads the rest. */
  credits: readonly DetailCredit[];
  creditTotals?: Readonly<{ cast: number; crew: number }>;
}>;
export type DetailCredit = Readonly<{ id: string; name: string; role: string; department: string; provider: string; portraitUrl?:string; personId?:string}>;
/** One page of a title's cast or crew (GET /v1/items/{id}/credits). */
export type CreditPage = Readonly<{ group: 'cast' | 'crew'; credits: readonly DetailCredit[]; total: number; nextCursor?: string }>;
export type DetailEpisodeInfo = Readonly<{ showId: string; showTitle: string; seasonId?: string; seasonNumber?: number | null; numbering: string; number: number; localIdentityStatus: string; providerMatchStatus: string; orderingBasis: string; sourceBoundary: string }>;
export type DetailSongInfo = Readonly<{ albumId: string; albumTitle: string; albumArtist: string; artist: string; discNumber?: number | null; trackNumber?: number | null; providerMatchStatus: string; localMetadataIssue?: string; providerTitle?:string;providerArtist?:string;providerRecordingId?:string;providerReleaseId?:string;providerReleaseGroupId?:string;providerTrackId?:string;providerReleaseStatus?:string;providerObservedAt?:string }>;
export type DetailBookFileInfo = Readonly<{ bookId: string; bookTitle: string; discNumber?: number | null; partNumber?: number | null; sourceBoundary: string; localMetadataIssue?: string;localMetadata?:LocalBookMetadata;localPolicy?:LocalAudioPolicy }>;
export type DetailSource = NonNullable<MediaItem['sources']>[number] & { duration?: number };
export type DetailItem = Omit<MediaItem, 'sources'> & { /** An episode's own still, never inherited. */ stillUrl?: string; addedAt?: string | null; sources?: DetailSource[]; episode?: DetailEpisodeInfo; song?: DetailSongInfo; bookFile?: DetailBookFileInfo };
/** The title's own facts (the editor's General fields): the facts line and the Details section. */
export type TitleFacts = Readonly<{ contentRating?: string; studio?: string; network?: string; tagline?: string; releaseDate?: string; originalTitle?: string; country?: string; edition?: string }>;
/** One audio or subtitle track of a file. `external` is a subtitle file beside the media. */
export type TitleFileTrack = Readonly<{ language?: string; codec: string; title?: string; channels?: number; channelLayout?: string; objectAudio?: string; bitRate?: number; default?: boolean; forced?: boolean; external?: boolean }>;
/** One file of a title with every track in it (the page's Files list). `path` is sent to owners only. */
export type TitleFile = Readonly<{
  id: string; path?: string; size: number; container: string; duration: number; bitRate?: number; available: boolean;
  video?: Readonly<{ codec: string; width: number; height: number; dynamicRange?: string; hdr10Plus?: boolean; frameRate?: number; bitDepth?: number; bitRate?: number }>;
  audio: readonly TitleFileTrack[]; subtitles: readonly TitleFileTrack[];
}>;
export type DetailProjection = Readonly<{
  facts?: TitleFacts;
  files?: readonly TitleFile[];
  scope: Readonly<{ serverId: string; libraryId: string; itemId: string; viewerFence: string }>;
  revision: Readonly<{ catalog: number; viewer: number }>;
  item: Readonly<DetailItem>;
  personal: PersonalState;
  actions: readonly DetailAction[];
  metadata: DetailMetadata;
  markers: readonly SegmentMarker[];
  related?:RelatedMovieProjection;
  extras?:readonly DetailExtra[];
}>;
export type PersonalIntent = Readonly<{ action: 'watchlist' | 'favorite' | 'watched'; value: boolean } | { action: 'rating'; value: number | null } | { action:'resolution';value:Readonly<{conflictId:string;choiceOperationId:string}> }>;
export type DetailError = Readonly<{ code: string; message: string; retryable: boolean }>;
export type DetailSnapshot = Readonly<{
  generation: number; scope: DetailScope; target: DetailTarget | null;
  phase: 'idle' | 'loading' | 'ready' | 'error'; data: DetailProjection | null; error: DetailError | null;
  pending: readonly Readonly<{ intent: PersonalIntent; phase: 'queued' | 'sending' | 'retry-required' | 'conflict' }>[];
  mutationError: DetailError | null;
}>;
type Receipt = { operationId: string; serverId: string; itemId: string; libraryId: string; viewerFence: string; personal: PersonalState };
type Command = { offline?:PersonalOfflineMutation; intent: PersonalIntent; operationId?: string; expectedRevision?: number; phase: 'queued' | 'sending' | 'retry-required' | 'conflict'; promise: Promise<void>; resolve: () => void };
const obj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const str = (v: unknown, max = 512): v is string => typeof v === 'string' && v.length <= max && !/[\x00-\x1f\x7f]/.test(v);
const id = (v: unknown): v is string => str(v, 256) && v.length > 0;
const integer = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0;
const finite = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v) && v >= 0;
const rating = (v: unknown): v is number | null => v === null || typeof v === 'number' && Number.isFinite(v) && v >= 0.5 && v <= 5 && Number.isInteger(v * 2);
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
class Failure extends Error { code: string; retryable: boolean; constructor(code: string, message: string, retryable = false) { super(message); this.code = code; this.retryable = retryable; } }
function invalid(): never { throw new Failure('invalid_detail', unreadableServerResponse); }
/** One credit row as the detail and the credits pages carry it. */
function creditRow(row: unknown, itemId: string): DetailCredit { if (!obj(row) || !id(row.id) || !str(row.name) || !str(row.role) || !str(row.department) || !id(row.provider)||row.personId!==undefined&&!id(row.personId)||row.portraitUrl!==undefined&&(!str(row.portraitUrl)||!row.portraitUrl.startsWith('/v1/metadata/item/'+encodeURIComponent(itemId)+'/art/portrait?')||/[\\\r\n]/.test(row.portraitUrl))) invalid(); return Object.freeze({ id: row.id, name: row.name, role: row.role, department: row.department, provider: row.provider,...(row.portraitUrl?{portraitUrl:row.portraitUrl as string}:{}),...(row.personId?{personId:row.personId as string}:{}) }); }
function array(value: unknown, max: number): unknown[] { if (!Array.isArray(value) || value.length > max) invalid(); return value; }
function personal(value: unknown): PersonalState {
  if (!obj(value) || typeof value.watchlisted !== 'boolean' || typeof value.favorite !== 'boolean' || !rating(value.rating) || !integer(value.revision)) invalid();
  const watched=value.watched??false,progressSeconds=value.progressSeconds??0,lastPlayedAt=value.lastPlayedAt??'',status=value.status??'current';
  if(typeof watched!=='boolean'||!finite(progressSeconds)||!str(lastPlayedAt,128)||!['current','needs-resolution'].includes(status as string))invalid();
  const conflicts=array(value.conflicts??[],8).map(c=>{if(!obj(c)||!id(c.id)||!id(c.field)||!integer(c.baseRevision))invalid();const choices=array(c.choices,100).map(v=>{if(!obj(v)||!id(v.operationId)||!id(v.deviceId)||!str(v.authoredAt,128))invalid();return Object.freeze({operationId:v.operationId,deviceId:v.deviceId,authoredAt:v.authoredAt,value:v.value});});return Object.freeze({id:c.id,field:c.field,baseRevision:c.baseRevision,baseValue:c.baseValue,choices:Object.freeze(choices)});});
  return Object.freeze({watchlisted:value.watchlisted,favorite:value.favorite,rating:value.rating,revision:value.revision,watched,progressSeconds,lastPlayedAt,status:status as PersonalState['status'],conflicts:Object.freeze(conflicts)});
}
function fileTrack(value: unknown): TitleFileTrack {
  if (!obj(value) || !str(value.codec, 64)) invalid();
  const text = (v: unknown, max = 256) => (str(v, max) && v ? v : undefined);
  const count = (v: unknown) => (integer(v) && v > 0 ? v : undefined);
  const language = text(value.language, 64), title = text(value.title), channelLayout = text(value.channelLayout, 64), objectAudio = text(value.objectAudio, 32), channels = count(value.channels), bitRate = count(value.bitRate);
  return Object.freeze({ codec: value.codec, ...(language ? { language } : {}), ...(title ? { title } : {}), ...(channels ? { channels } : {}), ...(channelLayout ? { channelLayout } : {}), ...(objectAudio ? { objectAudio } : {}), ...(bitRate ? { bitRate } : {}), ...(value.default === true ? { default: true } : {}), ...(value.forced === true ? { forced: true } : {}), ...(value.external === true ? { external: true } : {}) });
}
function titleFile(value: unknown): TitleFile {
  if (!obj(value) || !id(value.id) || !integer(value.size) || !str(value.container, 64) || !finite(value.duration) || typeof value.available !== 'boolean') invalid();
  if (value.path !== undefined && !str(value.path, 4096)) invalid();
  let video: TitleFile['video'];
  if (value.video !== undefined) {
    const v = value.video;
    if (!obj(v) || !str(v.codec, 64) || !integer(v.width) || !integer(v.height)) invalid();
    video = Object.freeze({ codec: v.codec, width: v.width, height: v.height, ...(str(v.dynamicRange, 32) && v.dynamicRange ? { dynamicRange: v.dynamicRange } : {}), ...(v.hdr10Plus === true ? { hdr10Plus: true } : {}), ...(finite(v.frameRate) && v.frameRate > 0 ? { frameRate: v.frameRate } : {}), ...(integer(v.bitDepth) && v.bitDepth > 0 ? { bitDepth: v.bitDepth } : {}), ...(integer(v.bitRate) && v.bitRate > 0 ? { bitRate: v.bitRate } : {}) });
  }
  return Object.freeze({ id: value.id, ...(value.path ? { path: value.path as string } : {}), size: value.size, container: value.container, duration: value.duration, ...(integer(value.bitRate) && value.bitRate > 0 ? { bitRate: value.bitRate } : {}), available: value.available, ...(video ? { video } : {}), audio: Object.freeze(array(value.audio ?? [], 128).map(fileTrack)), subtitles: Object.freeze(array(value.subtitles ?? [], 256).map(fileTrack)) });
}
function checkScope(value: DetailScope): DetailScope {
  if (!obj(value) || !id(value.serverId) || !str(value.viewerId, 1024) || !value.viewerId) throw new Error('A bound detail viewer/server scope is required.');
  return Object.freeze({ serverId: value.serverId, viewerId: value.viewerId });
}
function checkTarget(value: DetailTarget): DetailTarget {
  if (!obj(value) || !id(value.libraryId) || !id(value.itemId)) throw new Error('A library and item are required.');
  return Object.freeze({ libraryId: value.libraryId, itemId: value.itemId });
}
function checkIntent(value:PersonalIntent):PersonalIntent {
 if(!obj(value))throw new Error('Invalid personal-state intent.');
 if(value.action==='resolution'){if(!obj(value.value)||!id(value.value.conflictId)||!id(value.value.choiceOperationId))throw new Error('Choose a current conflict option.');return Object.freeze({action:'resolution',value:Object.freeze({...value.value})});}
 if(!((['watchlist','favorite','watched'].includes(value.action)&&typeof value.value==='boolean')||(value.action==='rating'&&rating(value.value))))throw new Error('Invalid personal-state intent.');
 return Object.freeze({...value});
}
function hierarchy(value: unknown, required: string[], optional: string[], numbers: string[]): Readonly<Record<string, string | number | null>> {
  if (!obj(value)) invalid();
  const result: Record<string, string | number | null> = {};
  for (const key of required) { if (!str(value[key], 2048)) invalid(); result[key] = value[key] as string; }
  for (const key of optional) if (value[key] !== undefined) { if (!str(value[key], 2048)) invalid(); result[key] = value[key] as string; }
  for (const key of numbers) if (value[key] !== undefined) { if (value[key] !== null && !integer(value[key])) invalid(); result[key] = value[key] as number | null; }
  return Object.freeze(result);
}
// Extras are ordinary playable items grouped by a server-published type.
function extras(value: unknown, target: DetailTarget): readonly DetailExtra[] {
  const groups = array(value, detailExtraTypes.length).map(group => {
    if (!obj(group) || !detailExtraTypes.includes(group.type as DetailExtraType) || !str(group.label, 256) || !group.label) invalid();
    const items = array(group.items, 100).map(entry => {
      const parsed = validateContentEntry(entry);
      if (parsed.kind !== 'extra' || parsed.id === target.itemId || parsed.playback?.itemId !== parsed.id) invalid();
      return parsed;
    });
    if (items.length === 0 || new Set(items.map(item => item.id)).size !== items.length) invalid();
    return Object.freeze({ type: group.type as DetailExtraType, label: group.label, items: Object.freeze(items) });
  });
  if (new Set(groups.map(group => group.type)).size !== groups.length) invalid();
  return Object.freeze(groups);
}
function projection(raw: unknown, scope: DetailScope, target: DetailTarget): DetailProjection {
  if (!obj(raw) || !obj(raw.scope) || !obj(raw.revision) || !obj(raw.item) || !obj(raw.metadata)) invalid();
  const s = raw.scope, r = raw.revision, item = raw.item, m = raw.metadata;
  if (s.serverId !== scope.serverId || s.libraryId !== target.libraryId || s.itemId !== target.itemId || !id(s.viewerFence) || !integer(r.catalog) || !integer(r.viewer)) invalid();
  if (item.id !== target.itemId || item.libraryId !== target.libraryId || !str(item.title, 2048) || !id(item.kind) || !finite(item.duration) || !finite(item.progressSeconds) || typeof item.available !== 'boolean') invalid();
  const media: DetailItem = { id: target.itemId, libraryId: target.libraryId, title: item.title, kind: item.kind, duration: item.duration, progressSeconds: item.progressSeconds, available: item.available };
  for (const key of ['overview','posterUrl','backdropUrl','stillUrl'] as const) if (item[key] !== undefined) { if (key === 'overview' ? typeof item[key] !== 'string' || item[key].length > 65536 || /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(item[key]) : !str(item[key], 4096)) invalid(); media[key] = item[key] as string; }
  if (item.year !== undefined) { if (!integer(item.year)) invalid(); media.year = item.year; }
  if (item.addedAt !== undefined) { if (item.addedAt !== null && !str(item.addedAt, 128)) invalid(); media.addedAt = item.addedAt as string | null; }
  if (item.sources !== undefined) media.sources = array(item.sources, 64).map(source => {
    if (!obj(source) || !id(source.id) || !str(source.container) || !str(source.videoCodec) || !str(source.audioCodec) || !integer(source.width) || !integer(source.height)) invalid();
    if (source.duration !== undefined && !finite(source.duration)) invalid();
    return Object.freeze({ ...(source.duration === undefined ? {} : { duration: source.duration as number }), id: source.id, container: source.container, videoCodec: source.videoCodec, audioCodec: source.audioCodec, width: source.width, height: source.height });
  });
  if (media.sources) Object.freeze(media.sources);
  if (item.episode !== undefined) {
    const fields = hierarchy(item.episode, ['showId','showTitle','numbering','localIdentityStatus','providerMatchStatus','orderingBasis','sourceBoundary'], ['seasonId'], ['seasonNumber','number']);
    if (!integer(fields.number) || !id(fields.showId) || fields.seasonId !== undefined && !id(fields.seasonId)) invalid();
    media.episode = fields as DetailEpisodeInfo;
  }
  if (item.song !== undefined) {
    const fields = hierarchy(item.song, ['albumId','albumTitle','albumArtist','artist','providerMatchStatus'], ['localMetadataIssue','providerTitle','providerArtist','providerRecordingId','providerReleaseId','providerReleaseGroupId','providerTrackId','providerReleaseStatus','providerObservedAt'], ['discNumber','trackNumber']);
    if (!id(fields.albumId)) invalid(); media.song = fields as DetailSongInfo;
  }
  if (item.bookFile !== undefined) {
    const fields = hierarchy(item.bookFile, ['bookId','bookTitle','sourceBoundary'], ['localMetadataIssue'], ['discNumber','partNumber']);
    if (!id(fields.bookId)) invalid(); media.bookFile = Object.freeze({...fields,...(obj(item.bookFile)&&item.bookFile.localPolicy!==undefined?{localPolicy:parseLocalAudioPolicy(item.bookFile.localPolicy)}:{}),...(obj(item.bookFile)&&item.bookFile.localMetadata!==undefined?{localMetadata:parseLocalBook(item.bookFile.localMetadata)}:{})}) as DetailBookFileInfo;
  }
  const actions = array(raw.actions, 32).map(action => {
    if (!obj(action) || !id(action.id) || !id(action.labelKey) || typeof action.enabled !== 'boolean') invalid();
    const result: { -readonly [K in keyof DetailAction]: DetailAction[K] } = { id: action.id, labelKey: action.labelKey, enabled: action.enabled };
    for (const field of ['min','max','step'] as const) if (action[field] !== undefined) { if (!finite(action[field])) invalid(); result[field] = action[field] as number; }
    if (action.playback !== undefined) {
      const p = action.playback;
      if (!obj(p) || p.itemId !== target.itemId || !finite(p.startSeconds) || p.startSeconds > media.duration) invalid();
      result.playback = Object.freeze({ itemId: target.itemId, startSeconds: p.startSeconds });
    }
    return Object.freeze(result);
  });
  if (new Set(actions.map(a => a.id)).size !== actions.length || !['available','unavailable'].includes(m.status as string)) invalid();
  const sources = array(m.sources??[],16).map(row=>{if(!obj(row)||!id(row.provider)||!str(row.sourceUrl,4096)||!str(row.observedAt,128))invalid();if(row.sourceUrl){try{const url=new URL(row.sourceUrl);if(url.protocol!=='https:'||url.username||url.password)invalid();}catch{invalid();}}return Object.freeze({provider:row.provider,sourceUrl:row.sourceUrl,observedAt:row.observedAt});});
  const ratings = array(m.ratings, 16).map(row => {
    if (!obj(row) || !id(row.provider) || !finite(row.value) || !finite(row.max) || row.max <= 0 || row.value > row.max || !str(row.sourceUrl, 4096) || !str(row.observedAt, 128) || row.votes !== undefined && !integer(row.votes)) invalid();
    return Object.freeze({ provider: row.provider, value: row.value, max: row.max, sourceUrl: row.sourceUrl, observedAt: row.observedAt, ...(row.votes === undefined ? {} : { votes: row.votes as number }) });
  });
  const genres = array(m.genres, 10000).map(row => { if (!obj(row) || !id(row.id) || !str(row.name) || !id(row.provider)) invalid(); return Object.freeze({ id: row.id, name: row.name, provider: row.provider }); });
  const attributions=m.attributions===undefined?[]:array(m.attributions,12).map(v=>{if(!str(v)||v.length>2048)invalid();return v;});
  const credits = array(m.credits, 10000).map(row => creditRow(row, target.itemId));
  const creditTotals=m.creditTotals===undefined?undefined:obj(m.creditTotals)&&integer(m.creditTotals.cast)&&integer(m.creditTotals.crew)?Object.freeze({cast:m.creditTotals.cast,crew:m.creditTotals.crew}):invalid();
  let markers: readonly SegmentMarker[] = [];
  try { markers = parseSegmentMarkers(raw.markers ?? [], media.duration); } catch { invalid(); }
  let facts: TitleFacts | undefined;
  if (obj(raw.facts)) { const f: {-readonly [K in keyof TitleFacts]: TitleFacts[K]} = {}; for (const key of ['contentRating','studio','network','tagline','releaseDate','originalTitle','country','edition'] as const) { const v = raw.facts[key]; if (str(v, 2048) && v) f[key] = v; } if (Object.keys(f).length) facts = Object.freeze(f); }
  const files = raw.files === undefined ? undefined : Object.freeze(array(raw.files, 64).map(titleFile));
  return Object.freeze({ ...(facts ? { facts } : {}), ...(files?.length ? { files } : {}), markers, scope: Object.freeze({ serverId: scope.serverId, libraryId: target.libraryId, itemId: target.itemId, viewerFence: s.viewerFence }), revision: Object.freeze({ catalog: r.catalog, viewer: r.viewer }), item: Object.freeze(media), personal: personal(raw.personal), actions: Object.freeze(actions), ...(raw.related===undefined?{}:{related:validateRelatedMovies(raw.related,{...target,kind:media.kind})}), ...(raw.extras===undefined?{}:{extras:extras(raw.extras,target)}), metadata: Object.freeze({ ...(m.sources===undefined?{}:{sources:Object.freeze(sources)}), ...(m.attributions===undefined?{}:{attributions:Object.freeze(attributions)}),status: m.status as DetailMetadata['status'], ratings: Object.freeze(ratings), genres: Object.freeze(genres), credits: Object.freeze(credits), ...(creditTotals?{creditTotals}:{}) }) });
}
function receipt(raw: unknown, operationId: string, data: DetailProjection): Receipt {
  if (!obj(raw) || raw.operationId !== operationId || raw.serverId !== data.scope.serverId || raw.libraryId !== data.scope.libraryId || raw.itemId !== data.scope.itemId || raw.viewerFence !== data.scope.viewerFence) invalid();
  return { operationId, serverId: data.scope.serverId, libraryId: data.scope.libraryId, itemId: data.scope.itemId, viewerFence: data.scope.viewerFence, personal: personal(raw.current ?? raw.personal) };
}
function errorInfo(error: unknown): DetailError {
  const e = error as { code?: unknown; retryable?: unknown; status?:number } | null;
  return Object.freeze({ code: id(e?.code) ? e.code : 'request_failed', message: error instanceof Error ? error.message : 'The detail request failed.', retryable: e?.status!==401&&e?.status!==403&&(e?.retryable===true||error instanceof TypeError||(e?.status??0)>=500||e?.status===408||e?.status===429) });
}

export class DetailService {
  private api: DetailApi; private bound: DetailScope; private requestId: () => Promise<string>; private timeoutMs: number;
  private generation = 0; private readGeneration = 0; private readController?: AbortController; private mutationController?: AbortController;
  private listeners = new Set<() => void>(); private state: DetailSnapshot; private disposed = false;
  private commands: Command[] = []; private running = false; private readPromise?: Promise<void>;
  constructor(options: { api: DetailApi; scope: DetailScope; requestId: () => Promise<string>; timeoutMs?: number }) {
    this.api = options.api; this.bound = checkScope(options.scope); this.requestId = options.requestId; this.timeoutMs = options.timeoutMs ?? 15000;
    if (!Number.isFinite(this.timeoutMs) || this.timeoutMs <= 0 || this.timeoutMs > 120000) throw new Error('Invalid detail deadline.');
    this.state = this.empty();
  }
  private empty(): DetailSnapshot { return Object.freeze({ generation: this.generation, scope: this.bound, target: null, phase: 'idle', data: null, error: null, pending: Object.freeze([]), mutationError: null }); }
  getSnapshot = (): DetailSnapshot => this.state;
  subscribe = (listener: () => void): (() => void) => { if (this.disposed) return () => {}; this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(patch: Partial<DetailSnapshot> = {}): void { if (this.disposed) return; this.state = Object.freeze({ ...this.state, ...patch, pending: Object.freeze(this.commands.map(c => Object.freeze({ intent: c.intent, phase: c.phase }))) }); for (const listener of this.listeners) listener(); }
  private active(): void { if (this.disposed) throw new Error('Detail service is disposed.'); }
  private fence(): void { this.generation++; this.readGeneration++; this.readController?.abort(); this.mutationController?.abort(); for (const c of this.commands) c.resolve(); this.commands = []; this.running = false; this.readPromise = undefined; }
  select(input: DetailTarget): Promise<void> {
    this.active(); const target = checkTarget(input);
    if (this.state.phase === 'loading' && this.state.target?.itemId === target.itemId && this.state.target.libraryId === target.libraryId && this.readPromise) return this.readPromise;
    this.fence(); this.publish({ ...this.empty(), target }); return this.refresh();
  }
  refresh(): Promise<void> {
    this.active(); if (!this.state.target) return Promise.resolve();
    this.readController?.abort(); const controller = new AbortController(); this.readController = controller;
    const generation = this.generation, read = ++this.readGeneration, target = this.state.target, bound = this.bound, api = this.api;
    this.publish({ phase: 'loading', error: null });
    const task = (async () => {
      try {
        const raw = await this.deadline(api.request<unknown>('/v1/items/' + encodeURIComponent(target.itemId) + '/detail?related=all', 'GET', undefined, controller.signal), controller);
        if (this.disposed || generation !== this.generation || read !== this.readGeneration) return;
        const data = projection(raw, bound, target);
        if (this.state.data && (data.scope.viewerFence!==this.state.data.scope.viewerFence || data.revision.catalog<this.state.data.revision.catalog)) throw new Failure('detail_scope_changed','The catalog or your access changed. Reload these details before continuing.',true);
        if (this.state.data && data.personal.revision < this.state.data.personal.revision) { this.publish({ phase: 'ready' }); return; }
        this.publish({ phase: 'ready', data });
      } catch (error) {
        if (this.disposed || generation !== this.generation || read !== this.readGeneration) return;
        const info=errorInfo(error); this.publish({ phase: 'error', data: ['unauthorized','forbidden','not_found','detail_scope_changed','invalid_detail'].includes(info.code)?null:this.state.data, error: info });
      }
    })();
    this.readPromise = task; return task;
  }
  retryRead(): Promise<void> { return this.refresh(); }
  mutate(input: PersonalIntent): Promise<void> {
    this.active(); const intent = checkIntent(input), data = this.state.data;
    if (!data || (intent.action==='resolution' ? !data.personal.conflicts.some(c=>c.id===intent.value.conflictId) : !data.actions.some(a => a.id === intent.action && a.enabled))) throw new Error('This personal action is not currently available.');
    const last = this.commands.at(-1);
    if (last && JSON.stringify(last.intent) === JSON.stringify(intent)) return last.promise;
    if (this.commands.length >= 8) throw new Error('Wait for pending personal actions to finish.');
    let resolve!: () => void; const promise = new Promise<void>(r => { resolve = r; });
    this.commands.push({ intent, phase: 'queued', promise, resolve }); this.publish(); void this.pump(); return promise;
  }
  retryMutation(): Promise<void> {
    this.active(); const command = this.commands[0];
    if (!command || this.running || command.phase === 'queued' || command.phase === 'sending') return command?.promise ?? Promise.resolve();
    if(command.offline && command.phase==='conflict')throw new Error('This offline intent needs review; do not silently rebase it.');
    if (command.phase === 'conflict') { command.operationId = undefined; command.expectedRevision = undefined; }
    let resolve!: () => void; command.promise = new Promise<void>(r => { resolve = r; }); command.resolve = resolve; command.phase = 'queued';
    this.publish({ mutationError: null }); void this.pump(); return command.promise;
  }
  /** Abandon retry only after an explicit user decision. Never manufacture a replacement receipt. */
  dismissMutation():void {this.active();if(this.running)return;for(const c of this.commands)c.resolve();this.commands=[];this.publish({mutationError:null});}
  /** Online reconciliation boundary for an existing device-authored operation. No device storage owner. */
  reconcile(input:Readonly<{operationId:string;offline:PersonalOfflineMutation;intent:PersonalIntent}>):Promise<void>{
    this.active();const intent=checkIntent(input.intent),o=input.offline;
    if(!uuid.test(input.operationId)||!id(o.deviceId)||!id(o.deviceMutationId)||!integer(o.sequence)||o.sequence===0||!integer(o.baseRevision)||!Number.isFinite(Date.parse(o.authoredAt))||intent.action==='resolution')throw new Error('Invalid device-authored operation.');
    if(!this.state.data||this.commands.length)throw new Error('Load the exact item and reconcile outstanding operations serially.');
    let resolve!:()=>void;const promise=new Promise<void>(r=>resolve=r);
    this.commands.push({intent,operationId:input.operationId,expectedRevision:o.baseRevision,offline:Object.freeze({...o}),phase:'queued',promise,resolve});this.publish();void this.pump();return promise;
  }
  setScope(next: DetailScope, api: DetailApi): void { this.active(); const checked = checkScope(next); this.fence(); this.bound = checked; this.api = api; this.publish(this.empty()); }
  cancel(): void { this.active(); this.fence(); this.publish(this.empty()); }
  dispose(): void { this.fence(); this.disposed = true; this.listeners.clear(); }
  private async pump(): Promise<void> {
    if (this.running || this.disposed || !this.commands.length || this.commands[0].phase !== 'queued') return;
    const command = this.commands[0], data = this.state.data;
    if (!data) { command.phase = 'retry-required'; command.resolve(); this.publish({ mutationError: Object.freeze({ code: 'detail_required', message: 'Reload details before retrying this action.', retryable: true }) }); return; }
    this.running = true; command.phase = 'sending'; this.publish({ mutationError: null });
    const generation = this.generation, api = this.api, controller = new AbortController(); this.mutationController = controller;
    try {
      if (!command.operationId) {
        const operationId = await this.deadline(this.requestId(), controller);
        if (!uuid.test(operationId)) throw new Failure('invalid_request_id', 'A UUIDv4 operation ID is required.');
        if (this.disposed || generation !== this.generation) return;
        command.operationId = operationId; command.expectedRevision = data.personal.revision;
      }
      const field = command.intent.action === 'watchlist' ? 'watchlisted' : command.intent.action;
      const body = { operationId: command.operationId, expectedRevision: command.expectedRevision, [field]: command.intent.value, ...(command.offline?{offline:command.offline}:{}) };
      const raw = await this.deadline(api.request<unknown>('/v1/items/' + encodeURIComponent(data.scope.itemId) + '/personal-state', 'PUT', body, controller.signal), controller);
      if (this.disposed || generation !== this.generation) return;
      const applied = receipt(raw, command.operationId, data);
      if (this.state.data && applied.personal.revision >= this.state.data.personal.revision) this.publish({ data: Object.freeze({ ...this.state.data, item:Object.freeze({...this.state.data.item,progressSeconds:applied.personal.progressSeconds}), personal: applied.personal }) });
      this.commands.shift(); command.resolve(); this.publish({ mutationError: null });
      await this.refresh();
    } catch (error) {
      if (this.disposed || generation !== this.generation) return;
      const info = errorInfo(error);
      command.phase = ['personal_state_conflict','personal_needs_resolution','personal_needs_review','operation_expired'].includes(info.code) ? 'conflict' : 'retry-required'; command.resolve();
      this.publish({ mutationError: info });
      if (command.phase === 'conflict') await this.refresh();
    } finally {
      if (generation === this.generation) { this.running = false; if (this.commands[0]?.phase === 'queued') void this.pump(); }
    }
  }
  private async deadline<T>(operation: Promise<T>, controller: AbortController): Promise<T> {
    let timer: ReturnType<typeof setTimeout> | undefined, onAbort: (() => void) | undefined;
    try {
      const abort = new Promise<never>((_, reject) => { onAbort = () => reject(new Failure('cancelled', 'Detail request cancelled.')); if (controller.signal.aborted) onAbort(); else controller.signal.addEventListener('abort', onAbort, { once: true }); });
      const timeout = new Promise<never>((_, reject) => { timer = setTimeout(() => { reject(new Failure('timeout', 'The server took too long to respond. Retry to reconcile the same operation.', true)); controller.abort(); }, this.timeoutMs); });
      return await Promise.race([operation, abort, timeout]);
    } finally { if (timer) clearTimeout(timer); if (onAbort) controller.signal.removeEventListener('abort', onAbort); }
  }
}

/** Validates one page of GET /v1/items/{itemId}/credits. */
export function parseCreditPage(raw: unknown, itemId: string, group: 'cast' | 'crew'): CreditPage {
  if (!obj(raw) || raw.group !== group || !integer(raw.total) || raw.nextCursor !== undefined && (!str(raw.nextCursor, 4096) || !raw.nextCursor)) invalid();
  const credits = array(raw.credits, 10000).map(row => creditRow(row, itemId));
  return Object.freeze({ group, credits: Object.freeze(credits), total: raw.total as number, ...(raw.nextCursor ? { nextCursor: raw.nextCursor as string } : {}) });
}
