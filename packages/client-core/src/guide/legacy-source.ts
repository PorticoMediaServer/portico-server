/**
 * Shared guide adapter: current servers page globally ordered channel rows and
 * summarize sources without loading programmes; visible programme blocks name
 * their channels explicitly. Older servers retain signed cursor pagination with
 * shared, cancellable whole-directory reads for compatibility.
 *
 * Rows and programmes keep validated native handles for tune/record/preferences.
 */
import {parseChannelGuide, type ChannelApi, type ChannelGuide, type GuideChannel, type GuideProgramme, type GuideRoute} from '../channel-guide.ts';
import {fetchGuideSourceSummaries, guideEndpointUnsupported, parseGuideDirectory} from './directory.ts';
import {HOUR_MS, floorTo} from './time.ts';
import type {ChannelSource, GuideChannelRow, GuideDataSource, GuideProgram} from './types.ts';

export type LegacyGuideOptions = Readonly<{
  /** `all`: live sources, then Library Channels (All channels, §2.0). */
  kind: 'live-source' | 'library-channel' | 'all';
  /** '' for every source of the kind. */
  sourceId: string;
  timezone: string;
  group?: string;
  favorites?: boolean;
  /** Owners: include hidden channels (to unhide them). */
  includeHidden?: boolean;
  /**
   * `server` keeps the server's order. `number`: sources in the owner's order, each by channel
   * number; `name` interleaves sources by name (§2.2).
   */
  sort?: 'server' | 'number' | 'name';
  /** Above this, the adapter still works but warns: it walks every page per block. */
  maxChannels?: number;
  now?: () => number;
  warn?: (message: string) => void;
}>;

const iso = (ms: number) => new Date(ms).toISOString().replace('.000Z', 'Z');

type GuideSelection = Readonly<{channels?: readonly string[]; rowsOnly?: boolean}>;

function query(route: GuideRoute, cursor: string, select: GuideSelection = {}): string {
  const q = new URLSearchParams({kind: route.kind, start: route.start, end: route.end, timezone: route.timezone, search: route.search, sourceId: route.sourceId, limit: select.channels ? '50' : '30', cursor});
  if (select.channels) q.set('channels', select.channels.join(','));
  if (select.rowsOnly) q.set('programmes', 'none');
  if (route.favorites !== undefined) q.set('favorites', String(route.favorites));
  if (route.includeHidden) q.set('includeHidden', 'true');
  if (route.group) q.set('group', route.group);
  return '/v1/guide?' + q.toString();
}

/** Every page of one window, in order. */
async function walk(api: ChannelApi, serverId: string, route: GuideRoute, signal?: AbortSignal, select: GuideSelection = {}): Promise<ChannelGuide[]> {
  const pages: ChannelGuide[] = [];
  let cursor = '';
  for (let i = 0; i < 200; i++) {
    const page = parseChannelGuide(await api.request<unknown>(query(route, cursor, select), 'GET', undefined, signal), serverId, route);
    pages.push(page);
    if (!page.nextCursor) return pages;
    if (page.nextCursor===cursor) throw new Error('The guide repeated a continuation.');
    cursor = page.nextCursor;
  }
  throw Object.assign(new Error('The guide listing requires server windowing.'),{code:'guide_windowing_required',retryable:false});
}

/** A legacy whole-directory walk may serve several visible tiles. Keep its
 * transport alive only while subscribed; a passed viewport abort cancels the walk. */
function sharedWalk<T>(load:(signal:AbortSignal)=>Promise<T>){
 let cached:{value:T}|undefined;
 let flight:{controller:AbortController;promise:Promise<T>;users:number}|undefined;
 return (signal:AbortSignal):Promise<T>=>{
  if(signal.aborted)return Promise.reject(signal.reason??new Error('aborted'));
  if(cached)return Promise.resolve(cached.value);
  if(!flight){
   const controller=new AbortController();
   const current={controller,promise:undefined as unknown as Promise<T>,users:0};flight=current;
   current.promise=Promise.resolve().then(()=>load(controller.signal)).then(value=>{if(flight===current&&!controller.signal.aborted)cached={value};return value;}).finally(()=>{if(flight===current)flight=undefined;});
  }
  const current=flight;current.users++;
  return new Promise<T>((resolve,reject)=>{
   let done=false;
   const finish=(error:unknown,value?:T)=>{if(done)return;done=true;signal.removeEventListener('abort',abort);current.users--;if(!current.users&&!cached&&flight===current){flight=undefined;current.controller.abort();}if(error!==undefined)reject(error);else resolve(value as T);};
   const abort=()=>finish(signal.reason??new Error('aborted'));signal.addEventListener('abort',abort,{once:true});
   current.promise.then(value=>finish(undefined,value),error=>finish(error));
  });
 };
}

// The handles this adapter attached, so the accessors never trust an object they didn't make.
const nativeChannels = new WeakSet<object>();
const nativeProgrammes = new WeakSet<object>();

/** The `/v1/guide` channel behind a row this adapter made (for tuning, recording and favorites). */
export function legacyChannel(row: GuideChannelRow): GuideChannel | undefined {
  const n = row.native;
  return n !== null && typeof n === 'object' && nativeChannels.has(n) ? n as GuideChannel : undefined;
}

/** The `/v1/guide` programme behind a program this adapter made (for recording). */
export function legacyProgramme(p: GuideProgram): GuideProgramme | undefined {
  const n = p.native;
  return n !== null && typeof n === 'object' && nativeProgrammes.has(n) ? n as GuideProgramme : undefined;
}

export function legacyChannelRow(c: GuideChannel): GuideChannelRow {
  nativeChannels.add(c);
  return {
    id: c.id, sourceId: c.provenance === 'library-channel' ? 'library' : c.sourceId, kind: c.provenance === 'library-channel' ? 'library' : 'live',
    number: c.number, name: c.name, group: c.group, ...(c.logoPath ? {logoUrl: c.logoPath} : {}), favorite: c.favorite,
    tuneAvailable: c.tuneAvailable, ...(c.tuneUnavailableReason ? {tuneUnavailableReason: c.tuneUnavailableReason} : {}),
    recordAvailable: c.recordAvailable, guide: 'full', native: c,
  };
}

export function legacyProgram(p: GuideProgramme): GuideProgram {
  nativeProgrammes.add(p);
  return {
    id: p.id, channelId: p.channelId, title: p.title, start: Date.parse(p.start), end: Date.parse(p.end),
    ...(p.seriesId ? {seriesId: p.seriesId} : {}),
    ...(p.subtitle ? {subtitle: p.subtitle} : {}),
    ...(p.episode ? {episode: p.episode} : {}),
    ...(p.categories !== undefined ? {categories: p.categories} : {}),
    ...(p.rating ? {rating: p.rating.value} : {}),
    ...(p.year !== undefined ? {year: p.year} : {}),
    ...(p.starRating ? {starRating: p.starRating} : {}),
    ...(p.image ? {image: p.image} : {}),
    flags: {live: p.flags?.live === true, new: p.flags?.new === true || p.newEvidence === 'new', premiere: p.flags?.premiere === true, repeat: p.flags?.repeat === true || p.newEvidence === 'repeat'},
    ...(p.recordingId ? {recording: {id: p.recordingId, state: p.recordingState ?? ''}} : {}),
    ...(p.description ? {description: p.description} : {}),
    native: p,
  };
}

const byNumber = (a: GuideChannelRow, b: GuideChannelRow) => {
  const na = parseFloat(a.number), nb = parseFloat(b.number);
  if (Number.isFinite(na) && Number.isFinite(nb) && na !== nb) return na - nb;
  if (Number.isFinite(na) !== Number.isFinite(nb)) return Number.isFinite(na) ? -1 : 1;
  return a.name.localeCompare(b.name);
};

/** Rows in the requested order, each channel once. */
export function orderLegacyRows(rows: readonly GuideChannelRow[], sort: 'server' | 'number' | 'name'): GuideChannelRow[] {
  const seen = new Set<string>();
  const unique = rows.filter(r => !seen.has(r.id) && !!seen.add(r.id));
  if (sort === 'server') return unique;
  if (sort === 'name') return unique.sort((a, b) => a.name.localeCompare(b.name) || byNumber(a, b));
  const order = new Map<string, number>();
  unique.forEach(r => { if (!order.has(r.sourceId ?? '')) order.set(r.sourceId ?? '', order.size); });
  return unique.sort((a, b) => (order.get(a.sourceId ?? '')! - order.get(b.sourceId ?? '')!) || byNumber(a, b));
}

function createLegacyGuideSource(api: ChannelApi, serverId: string, o: LegacyGuideOptions): GuideDataSource {
  const now = o.now ?? Date.now;
  const kinds = o.kind === 'all' ? (['live-source', 'library-channel'] as const) : [o.kind];
  const route = (kind: 'live-source' | 'library-channel', start: number, end: number): GuideRoute => ({
    kind, start: iso(start), end: iso(end), timezone: o.timezone, search: '', sourceId: o.sourceId,
    ...(o.favorites !== undefined ? {favorites: o.favorites} : {}), ...(o.includeHidden ? {includeHidden: true} : {}), ...(o.group ? {group: o.group} : {}),
  });
  const windows = new Map<string, ReturnType<typeof sharedWalk<ChannelGuide[]>>>();
  // Library Channels are few and owner-made: their programmes come from one walk per window.
  const libraryKinds = kinds.filter(k => k === 'library-channel');
  const window = (start: number, end: number, signal:AbortSignal) => {
    const key = `${start}:${end}`;
    let pages = windows.get(key);
    if (!pages) {
      // Subscribers share the walk; their last abort stops its transport.
      pages = sharedWalk<ChannelGuide[]>(signal=>Promise.allSettled(libraryKinds.map(k => walk(api, serverId, route(k, start, end),signal))).then(results => {
        if (results.length && results.every(r => r.status === 'rejected')) throw (results[0] as PromiseRejectedResult).reason;
        return results.flatMap(r => (r.status === 'fulfilled' ? r.value : []));
      }));
      windows.set(key, pages);
      while (windows.size > 8) windows.delete(windows.keys().next().value!);
    }
    return pages(signal);
  };
  const list=sharedWalk<GuideChannelRow[]>(signal=>{
    const start=floorTo(now(),HOUR_MS);
    return Promise.allSettled(kinds.map(k=>walk(api,serverId,route(k,start,start+3*HOUR_MS),signal,{rowsOnly:true}))).then(results=>{
      if(results.every(r=>r.status==='rejected'))throw (results[0] as PromiseRejectedResult).reason;
      return orderLegacyRows(results.flatMap(r=>r.status==='fulfilled'?r.value:[]).flatMap(p=>p.channels.map(legacyChannelRow)),o.sort??'server');
    });
  });
  return {
    async channels(from, limit, signal) {
      const all=await list(signal);
      if (signal.aborted) throw new Error('aborted');
      return {items: all.slice(from, from + limit), total: all.length};
    },
    async programs(channelIds, start, end, signal) {
      const out: Record<string, GuideProgram[]> = {};
      for (const id of channelIds) out[id] = [];
      const rows = new Map((await list(signal)).map(r => [r.id,r]));
      // Live channels on screen are asked for by id, 50 at a time, for this window only.
      const live = channelIds.filter(id => rows.get(id)?.kind !== 'library');
      const library = channelIds.filter(id => rows.get(id)?.kind === 'library');
      const reads: Promise<ChannelGuide[]>[] = [];
      if (kinds.includes('live-source')) for (let i = 0; i < live.length; i += 50) reads.push(walk(api, serverId, route('live-source', start, end), signal, {channels: live.slice(i, i + 50)}));
      if (library.length) reads.push(window(start, end, signal));
      const wanted = new Set(channelIds);
      for (const pages of await Promise.all(reads)) for (const page of pages) for (const c of page.channels) if (wanted.has(c.id)) out[c.id] = c.programmes.map(legacyProgram);
      if (signal.aborted) throw new Error('aborted');
      return out;
    },
  };
}

/** The directory sends only the channel rows on screen; programmes use named
 * channels and time windows. Older servers retain their existing guide contract. */
export function legacyGuideSource(api:ChannelApi,serverId:string,o:LegacyGuideOptions):GuideDataSource {
 let mode:'unknown'|'windowed'|'legacy'='unknown';
 let fallback:GuideDataSource|undefined;
 let revision:string|undefined,viewerFence:string|undefined;
 const rows=new Map<string,GuideChannel>();
 const old=()=>fallback??=createLegacyGuideSource(api,serverId,o);
 const route=(kind:'live-source'|'library-channel',start:number,end:number):GuideRoute=>({kind,start:iso(start),end:iso(end),timezone:o.timezone,search:'',sourceId:o.kind==='all'?'':o.sourceId,...(o.group?{group:o.group}:{}),...(o.favorites!==undefined?{favorites:o.favorites}:{}),...(o.includeHidden?{includeHidden:true}:{})});
 return {
  reset(){revision=undefined;viewerFence=undefined;rows.clear();fallback=undefined;},
 async neighbor(channel,delta,signal){
  if(mode==='unknown')await this.channels(0,1,signal);
  if(mode==='legacy'){
   const page=await old().channels(0,Number.MAX_SAFE_INTEGER,signal);
   const available=page.items.filter(c=>c.tuneAvailable);
   if(available.length<2)return undefined;
   const index=available.findIndex(c=>c.id===channel.id&&c.kind===channel.kind&&c.sourceId===channel.sourceId);
   const next=available[((index+(index<0&&delta<0?1:0)+delta)%available.length+available.length)%available.length];
   return next?.id===channel.id&&next.kind===channel.kind?undefined:next;
  }
  const native=legacyChannel(channel);if(!native)return undefined;
  for(let attempt=0;attempt<2;attempt++){
   const q=new URLSearchParams({kind:o.kind,sourceId:o.sourceId,timezone:o.timezone,sort:o.sort??'server',limit:'1',anchorChannelId:native.id,anchorSourceId:native.sourceId,anchorProvenance:native.provenance,direction:delta>0?'next':'previous'});
   if(o.group)q.set('group',o.group);if(o.favorites!==undefined)q.set('favorites',String(o.favorites));if(o.includeHidden)q.set('includeHidden','true');if(revision)q.set('revision',revision);
   try{
    const page=parseGuideDirectory(await api.request('/v1/guide/channels?'+q,'GET',undefined,signal),serverId,undefined,1);
    if(signal.aborted)throw signal.reason??new Error('aborted');
    if(revision&&revision!==page.revision||viewerFence&&viewerFence!==page.viewerFence)throw Object.assign(new Error('The guide changed. Refresh this view.'),{code:'guide_refresh_required',retryable:true});
    revision=page.revision;viewerFence=page.viewerFence;
    const next=page.channels[0];
    if(!next||!next.tuneAvailable||next.id===native.id&&next.provenance===native.provenance&&next.sourceId===native.sourceId)return undefined;
    return legacyChannelRow(next);
   }catch(error){
    if(attempt===0&&(error as {code?:string})?.code==='guide_refresh_required'){revision=undefined;viewerFence=undefined;rows.clear();continue;}
    throw error;
   }
  }
  return undefined;
 },
  async channels(from,limit,signal){
   if(mode==='legacy')return old().channels(from,limit,signal);
   if(!Number.isSafeInteger(from)||from<0||!Number.isInteger(limit)||limit<1||limit>50)throw new Error('Use a bounded channel directory page.');
   const q=new URLSearchParams({kind:o.kind,sourceId:o.sourceId,timezone:o.timezone,sort:o.sort??'server',offset:String(from),limit:String(limit)});
   if(o.group)q.set('group',o.group);if(o.favorites!==undefined)q.set('favorites',String(o.favorites));if(o.includeHidden)q.set('includeHidden','true');if(revision)q.set('revision',revision);
   try{
    const page=parseGuideDirectory(await api.request('/v1/guide/channels?'+q,'GET',undefined,signal),serverId,from,limit);
    if(signal.aborted)throw signal.reason??new Error('aborted');
    if(revision&&revision!==page.revision||viewerFence&&viewerFence!==page.viewerFence)throw Object.assign(new Error('The guide changed. Refresh this view.'),{code:'guide_refresh_required',retryable:true});
    mode='windowed';revision=page.revision;viewerFence=page.viewerFence;
    for(const channel of page.channels){rows.delete(channel.id);rows.set(channel.id,channel);}
    while(rows.size>300)rows.delete(rows.keys().next().value!);
    return {items:page.channels.map(legacyChannelRow),total:page.total,revision:page.revision};
   }catch(error){if(mode!=='windowed'&&guideEndpointUnsupported(error)){mode='legacy';return old().channels(from,limit,signal);}throw error;}
  },
  async programs(channelIds,start,end,signal){
   if(mode==='legacy')return old().programs(channelIds,start,end,signal);
   const out:Record<string,GuideProgram[]>=Object.fromEntries(channelIds.map(id=>[id,[]]));
   const kinds=o.kind==='all'?['live-source','library-channel'] as const:[o.kind];
   const reads:Promise<ChannelGuide[]>[]=[];
   for(const kind of kinds){const ids=channelIds.filter(id=>!rows.has(id)||rows.get(id)!.provenance===kind);for(let i=0;i<ids.length;i+=50)reads.push(walk(api,serverId,route(kind,start,end),signal,{channels:ids.slice(i,i+50)}));}
   for(const pages of await Promise.all(reads))for(const page of pages)for(const channel of page.channels)if(channel.id in out)out[channel.id]=channel.programmes.map(legacyProgram);
   if(signal.aborted)throw signal.reason??new Error('aborted');
   return out;
  },
 };
}

export type GuideCatalog=Readonly<{groups:readonly string[];favorites:boolean;unavailableReasons:readonly string[]}>;
/** Filter chips and the server-wide availability explanation use summaries,
 * never a thousand-row fetch. Group counts are combined across selected sources. */
export async function legacyGuideCatalog(api:ChannelApi,serverId:string,o:LegacyGuideOptions,signal?:AbortSignal):Promise<GuideCatalog>{
 try{
  const summary=await fetchGuideSourceSummaries(api,serverId,o.timezone,signal);
  const sources=summary.sources.filter(s=>o.kind==='all'||(o.kind==='library-channel')===(s.provenance==='library-channel')).filter(s=>!o.sourceId||o.sourceId==='library'&&s.provenance==='library-channel'||s.id===o.sourceId);
  const groups=new Map<string,number>();let total=0,unavailable=0;
  const reasons=new Set<string>();
  for(const source of sources){total+=source.channelCount;for(const group of source.groupCounts)groups.set(group.name,(groups.get(group.name)??0)+group.count);for(const [reason,count] of Object.entries(source.unavailableReasons)){unavailable+=count;if(count)reasons.add(reason);}}
  return {groups:[...groups].sort((a,b)=>b[1]-a[1]).slice(0,6).map(([name])=>name),favorites:sources.some(s=>s.favoriteCount>0),unavailableReasons:total>0&&unavailable===total?[...reasons]:[]};
 }catch(error){
  if(!guideEndpointUnsupported(error))throw error;
  const c=signal??new AbortController().signal;
  const page=await createLegacyGuideSource(api,serverId,o).channels(0,1000,c);
  const groups=new Map<string,number>();for(const row of page.items)if(row.group)groups.set(row.group,(groups.get(row.group)??0)+1);
  return {groups:[...groups].sort((a,b)=>b[1]-a[1]).slice(0,6).map(([name])=>name),favorites:page.items.some(c=>c.favorite),unavailableReasons:page.items.length&&!page.items.some(c=>c.tuneAvailable)?[...new Set(page.items.map(c=>c.tuneUnavailableReason??''))]:[]};
 }
}

/** The refresh states in which a source's guide is out of date; any other state is fine. */
const staleRefreshStates: ReadonlySet<string> = new Set(['degraded', 'credentials-required']);

/** A live source's guide state from its `/v1/guide` refresh state. */
export function legacyGuideState(refreshState: string): 'ready' | 'refreshing' | 'stale' {
  return refreshState === 'refreshing' ? 'refreshing' : staleRefreshStates.has(refreshState) ? 'stale' : 'ready';
}

/**
 * The Channels entries from today's API: each live source by name, plus "Library Channels" when
 * the library-channel guide has any channel. `libraryName` is the catalogue's name for it.
 */
export async function legacyChannelSources(api: ChannelApi, serverId: string, timezone: string, libraryName: string, now = Date.now(), signal?: AbortSignal): Promise<ChannelSource[]> {
  try{
    const summaries=await fetchGuideSourceSummaries(api,serverId,timezone,signal);
    return summaries.sources.filter(s=>s.channelCount>0).map(s=>({id:s.provenance==='library-channel'?'library':s.id,name:s.provenance==='library-channel'?libraryName:s.name,type:s.provenance==='library-channel'?'library':'live',position:s.position,recordAvailable:s.recordAvailable,guideState:legacyGuideState(s.refreshState),...(s.availableStart?{availableStart:Date.parse(s.availableStart)}:{}),...(s.availableEnd?{availableEnd:Date.parse(s.availableEnd)}:{}),guideDays:s.guideDays}));
  }catch(error){if(!guideEndpointUnsupported(error))throw error;}
  const start = floorTo(now, HOUR_MS), end = start + 3 * HOUR_MS;
  const base = {start: iso(start), end: iso(end), timezone, search: '', sourceId: ''};
  const [livePages, library] = await Promise.all([
    walk(api, serverId, {...base, kind: 'live-source'}, signal,{rowsOnly:true}),
    api.request<unknown>(query({...base, kind: 'library-channel'}, '',{rowsOnly:true}), 'GET', undefined, signal).then(raw => parseChannelGuide(raw, serverId, {...base, kind: 'library-channel'}), () => undefined),
  ]);
  const recordable = new Set(livePages.flatMap(p => p.channels).filter(c => c.recordAvailable).map(c => c.sourceId));
  const liveDays = livePages.map(p => p.days).filter((d): d is number => d !== undefined);
  const liveGuideDays = liveDays.length ? Math.max(...liveDays) : undefined;
  const allSources=[...new Map(livePages.flatMap(p=>p.sources).map(s=>[s.id,s])).values()];
  const out: ChannelSource[] = allSources.map((s, i) => ({
    id: s.id, name: s.name, type: 'live', position: i, recordAvailable: recordable.has(s.id),
    guideState: legacyGuideState(s.refreshState),
    ...(s.availableStart ? {availableStart: Date.parse(s.availableStart)} : {}), ...(s.availableEnd ? {availableEnd: Date.parse(s.availableEnd)} : {}),
    ...(liveGuideDays !== undefined ? {guideDays: liveGuideDays} : {}),
  }));
  if (library && library.channels.length) out.push({id: 'library', name: libraryName, type: 'library', position: out.length, recordAvailable: false, guideState: 'ready', ...(library.days !== undefined ? {guideDays: library.days} : {})});
  return out;
}

