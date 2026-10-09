import {GUIDE_PROTOCOL, parseGuideChannels, parseGuideSources, type ChannelApi, type GuideChannel, type GuideSource} from '../channel-guide.ts';
import {unreadableServerResponse} from '../server-messages.ts';

export type GuideDirectory = Readonly<{viewerFence:string;revision:string;total:number;offset:number;limit:number;channels:readonly GuideChannel[];sources:readonly GuideSource[]}>;
export type GuideSourceSummary = GuideSource & Readonly<{position:number;recordAvailable:boolean;guideDays:number;channelCount:number;favoriteCount:number;groups:readonly string[];groupCounts:readonly Readonly<{name:string;count:number}>[];unavailableReasons:Readonly<Record<string,number>>}>;
export type GuideSourceSummaries = Readonly<{viewerFence:string;revision:string;sources:readonly GuideSourceSummary[]}>;
const object=(v:unknown):v is Record<string,unknown>=>v!==null&&typeof v==='object'&&!Array.isArray(v);
const text=(v:unknown,max=256):v is string=>typeof v==='string'&&v.length>0&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const whole=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
function invalid():never{throw Object.assign(new Error(unreadableServerResponse),{code:'invalid_guide_directory',retryable:false});}
export function parseGuideDirectory(raw:unknown,serverId:string,offset:number|undefined,limit:number):GuideDirectory{
 if(!object(raw)||raw.protocolVersion!==GUIDE_PROTOCOL||raw.serverId!==serverId||!object(raw.directory))invalid();
 const d=raw.directory;
 if(!text(d.viewerFence)||!text(d.revision)||!whole(d.total)||!whole(d.offset)||(offset!==undefined&&d.offset!==offset)||d.limit!==limit||!Array.isArray(d.channels)||d.channels.length>limit||d.channels.length>Math.max(0,d.total-d.offset))invalid();
 if(offset===undefined&&((d.total>0&&d.offset>=d.total)||(d.total===0&&d.offset!==0)))invalid();
 const sources=parseGuideSources(d.sources),channels=parseGuideChannels(d.channels,sources);
 return {viewerFence:d.viewerFence,revision:d.revision,total:d.total,offset:d.offset,limit,channels,sources};
}
export function parseGuideSourceSummaries(raw:unknown,serverId:string):GuideSourceSummaries{
 if(!object(raw)||raw.protocolVersion!==GUIDE_PROTOCOL||raw.serverId!==serverId||!text(raw.viewerFence)||!text(raw.revision)||!Array.isArray(raw.sources)||raw.sources.length>65)invalid();
 const sources=parseGuideSources(raw.sources).map((source,index)=>{
  const value=(raw.sources as Record<string,unknown>[])[index];
  if(!whole(value.position)||typeof value.recordAvailable!=='boolean'||!whole(value.guideDays)||value.guideDays>31||!whole(value.channelCount)||!whole(value.favoriteCount)||value.favoriteCount>value.channelCount||!Array.isArray(value.groups)||value.groups.length>2000||!value.groups.every(v=>text(v,120))||!Array.isArray(value.groupCounts)||value.groupCounts.length>2000||!object(value.unavailableReasons)||Object.keys(value.unavailableReasons).length>64)invalid();
  const channelCount=value.channelCount;
  const groupCounts=value.groupCounts.map(v=>{if(!object(v)||!text(v.name,120)||!whole(v.count)||v.count>channelCount)invalid();return{name:v.name,count:v.count};});
  const unavailableReasons:Record<string,number>={};let unavailable=0;
  for(const [reason,count] of Object.entries(value.unavailableReasons)){if(!text(reason,64)||!whole(count))invalid();unavailable+=count;unavailableReasons[reason]=count;}
  if(unavailable>value.channelCount)invalid();
  return {...source,position:value.position,recordAvailable:value.recordAvailable,guideDays:value.guideDays,channelCount:value.channelCount,favoriteCount:value.favoriteCount,groups:value.groups as string[],groupCounts,unavailableReasons};
 });
 return {viewerFence:raw.viewerFence,revision:raw.revision,sources};
}
export function guideEndpointUnsupported(error:unknown):boolean{return object(error)&&(error.status===404||error.status===405);}
export async function fetchGuideSourceSummaries(api:ChannelApi,serverId:string,timezone:string,signal?:AbortSignal):Promise<GuideSourceSummaries>{
 return parseGuideSourceSummaries(await api.request('/v1/guide/sources?'+new URLSearchParams({timezone}),'GET',undefined,signal),serverId);
}
