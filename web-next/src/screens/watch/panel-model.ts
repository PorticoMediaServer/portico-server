import type {WebPlaybackRuntimeSnapshot,WebShellPlaybackRuntime} from '../../platform/playback';
import type {QualityChoice} from '@core/playback-v1/choices.ts';
import {audioStreamIndex,fixedQuality,isQualityChoice,qualityChoiceOf} from '@core/playback-v1/choices.ts';
import {videoTrackName,videoTrackWords} from '@core/video-track-labels.ts';
import {sameFence,type QueuePlaybackFence} from '../../../../packages/player-core/src/index.ts';
import type {I18n} from '@i18n';
import type {TrackChoice} from '../../kit/media';

export type WatchRadioChoice=Readonly<{id:string;label:string;detail?:string;selected:boolean;enabled:boolean}>;
export type WatchPanelModel=Readonly<{
 fence:QueuePlaybackFence;audio:readonly TrackChoice[];subtitles:readonly TrackChoice[];
 quality:readonly WatchRadioChoice[];speed:readonly WatchRadioChoice[];
 qualityLabel?:string;qualityAccessibleLabel?:string;qualityNotes:readonly string[];audioNotes:readonly string[];automatic:boolean;audioLabel?:string;subtitleLabel?:string;
 isCurrent():boolean;selectAudio(id:string):Promise<void>;selectSubtitle(id:string):Promise<void>;
 selectQuality(id:string):Promise<void>;selectSpeed(id:string):void;
}>;

function heightLabel(height:number,t:I18n['t']):string {
 return height===2160?t('quality.4k'):t('quality.height',{height});
}
/** Inert projection of the actual selected grouped version and the current source's offers.
 * No labels are parsed, no default track is treated as selected, and no options are invented. */
export function watchPanelModel(runtime:WebShellPlaybackRuntime,snapshot:WebPlaybackRuntimeSnapshot,i18n:I18n):WatchPanelModel|null {
 const fence=snapshot.fence,session=snapshot.session,offers=snapshot.offers,options=snapshot.options,current=offers?.current;
 if(snapshot.disposed||!fence||fence.kind!=='queue'||!session||!offers||!current||!options
  ||session.kind!=='vod'||session.id!==fence.sessionId||session.itemId!==fence.itemId||session.queue?.entryId!==fence.entryId
  ||session.presentation.generation!==fence.generation||options.itemId!==fence.itemId||offers.scope.itemId!==fence.itemId
  ||offers.scope.serverId!==snapshot.authority.serverId
  ||current.sessionId!==fence.sessionId||current.generation!==fence.generation||current.sourceId!==fence.sourceId)return null;
 const version=options.versions.find(v=>v.id===session.versionId),source=offers.sources.find(s=>s.id===fence.sourceId);
 if(!version||!source||!source.available)return null;
 // OffersService owns its detail viewer fence. The Home authority fence intentionally
 // has a different scope and must never be compared with that opaque token.
 const words=videoTrackWords(i18n.t,i18n.locale),t=i18n.t;
 const isCurrent=()=>{const live=runtime.getSnapshot();return !live.disposed&&live.authority===snapshot.authority&&sameFence(live.fence,fence)&&live.session?.versionId===version.id&&live.session.revision===session.revision&&live.options===options&&live.offers===offers;};
 const check=()=>{if(!isCurrent())throw new DOMException('Playback source retired','AbortError');};
 const audio=Object.freeze(version.audio.map(track=>{
  const name=videoTrackName(track,words),enabled=source.streams.some(s=>s.type==='audio'&&s.enabled&&s.index===audioStreamIndex(track.id));
  return Object.freeze({id:track.id,primary:name.first,secondary:name.rest||undefined,fullName:name.label,selected:session.presentation.audioTrackId===track.id,unavailable:!enabled});
 }));
 const plan=offers.subtitlePlan,matched=!!plan&&plan.sessionId===fence.sessionId&&plan.generation===fence.generation&&plan.sourceId===fence.sourceId;
 const subtitleIds=new Map<string,string|null>();const subtitleRows:TrackChoice[]=[];
 if(matched){
  subtitleIds.set('off',null);subtitleRows.push({id:'off',primary:t('player.opt.off'),fullName:t('player.opt.off'),selected:plan.mode==='off',unavailable:!plan.offAvailable});
  for(const resource of plan.resources.filter(r=>r.sourceId===fence.sourceId&&!r.retired)){
   const facts={...resource,sdh:resource.hearingImpaired,source:resource.origin==='embedded'?'embedded' as const:resource.origin==='sidecar'?'sidecar' as const:'downloaded' as const};
   const name=videoTrackName(facts,words);
   // desktop-player f3: a track from outside the file names where it came from on its second line.
   const origin=resource.origin==='provider'||resource.origin==='upload'?t('player.opt.origin',{origin:resource.origin}):undefined;
   const secondary=[name.rest,origin].filter(Boolean).join(', ')||undefined;
   const id='resource:'+resource.id;subtitleIds.set(id,resource.id);
   subtitleRows.push({id,primary:name.first,secondary,fullName:name.label,selected:plan.mode==='track'&&plan.selected?.id===resource.id,
    unavailable:!resource.enabled||!Number.isSafeInteger(Number(resource.offsetUs)/1000)});
  }
 }
 const selected=qualityChoiceOf(session.presentation.quality),qualityRows:WatchRadioChoice[]=[],qualityIds=new Map<string,QualityChoice>();
 // Requested mode chooses the radio; only an actual bound rung names the chip.
 const actualId=session.presentation.quality.id??current.qualityId,actualStep=fixedQuality(actualId);
 const qualityLabel=actualStep?heightLabel(actualStep.height,t):actualId==='original'?t('next.kit.quality.original'):undefined;
 const action=session.presentation.decision?.video?.action;
 // desktop-player f3: each quality footer fact keeps its own line, the network limit then the route.
 const limit=offers.deliveryPolicy&&offers.deliveryPolicy.maxVideoHeight>0?heightLabel(offers.deliveryPolicy.maxVideoHeight,t):undefined;
 const networkNote=limit?(offers.deliveryPolicy!.networkClass==='local'?t('next.kit.quality.home',{quality:limit})
  :offers.deliveryPolicy!.networkClass==='remote'?t('next.kit.quality.away',{quality:limit}):undefined):undefined;
 const routeNote=action==='transcode'?(qualityLabel?t('player.quality.converting',{quality:qualityLabel}):undefined):action?t('player.quality.playingOriginal'):undefined;
 const qualityNotes=Object.freeze([networkNote,routeNote].filter((note):note is string=>!!note));
 // f3b: a version with one audio track says so in place of a choice.
 const audioNotes=Object.freeze(version.audio.length===1?[t('player.opt.oneAudioTrack')]:[]);
 for(const offer of source.qualities){
  const choice=offer.kind==='automatic'?'automatic':offer.kind==='original'?'original':isQualityChoice(offer.id)?offer.id:undefined;
  if(!choice)continue;qualityIds.set(offer.id,choice);
  const step=fixedQuality(choice),label=choice==='automatic'?t('next.kit.quality.automatic'):choice==='original'?t('next.kit.quality.original'):step?heightLabel(step.height,t):undefined;
  if(!label)continue;
  const detail=choice==='automatic'&&qualityLabel?t('next.kit.quality.adjusts',{quality:qualityLabel}):choice==='original'?t('next.kit.quality.originalDetail'):
   typeof offer.maxVideoBitrateBps==='number'?t('web.player.quality.rate',{mbps:offer.maxVideoBitrateBps/1e6}):undefined;
  const enabled=offer.enabled&&options.qualities.some(option=>option.applicable!==false&&qualityChoiceOf(option)===choice);
  qualityRows.push({id:offer.id,label,detail,selected:selected===choice,enabled});
 }
 let speeds:readonly number[]=[];try{speeds=runtime.speedChoices(fence);}catch{/* source changed */}
 const speed=Object.freeze(speeds.map(rate=>Object.freeze({id:String(rate),label:rate===1?t('player.opt.normalSpeed'):t('next.dock.rate',{rate}),selected:snapshot.presentation?.rate===rate,enabled:true})));
 return Object.freeze({fence,audio,subtitles:Object.freeze(subtitleRows),quality:Object.freeze(qualityRows),speed,automatic:selected==='automatic',qualityLabel,qualityNotes,audioNotes,
  qualityAccessibleLabel:qualityLabel?(selected==='automatic'?t('player.quality.automaticNow',{quality:qualityLabel}):qualityLabel):undefined,
  audioLabel:audio.find(r=>r.selected)?.primary,subtitleLabel:subtitleRows.find(r=>r.selected)?.primary,isCurrent,
  selectAudio:async id=>{check();if(!audio.some(r=>r.id===id&&!r.unavailable))throw new Error('playback_audio_unavailable');await runtime.selectAudio(fence,id);},
  selectSubtitle:async id=>{check();if(!subtitleRows.some(r=>r.id===id&&!r.unavailable)||!subtitleIds.has(id))throw new Error('playback_subtitle_unavailable');await runtime.selectSubtitle(fence,subtitleIds.get(id)!);},
  selectQuality:async id=>{check();const choice=qualityIds.get(id);if(!choice||!qualityRows.some(r=>r.id===id&&r.enabled))throw new Error('playback_quality_unavailable');await runtime.setQuality(fence,choice);},
  selectSpeed:id=>{check();const row=speed.find(r=>r.id===id);if(!row)throw new Error('playback_speed_unavailable');runtime.setSpeed(fence,Number(row.id));},
 });
}

export type PhoneOptionRow=Readonly<{id:'subtitles'|'audio'|'quality'|'speed';label:string;value?:string;detail?:string}>;
/** The drawn phone row list: one row per real control, carrying its current choice only. */
export function phoneOptionRows(model:WatchPanelModel,t:I18n['t']):readonly PhoneOptionRow[]{
 return [
  ...(model.subtitles.length?[{id:'subtitles' as const,label:t('player.opt.subtitles'),value:model.subtitleLabel??t('player.opt.off')}]:[]),
  ...(model.audio.length?[{id:'audio' as const,label:t('player.opt.audio'),...(model.audioLabel?{value:model.audioLabel}:{})}]:[]),
  ...(model.quality.length?[{id:'quality' as const,label:t('player.opt.quality'),value:model.automatic?t('next.kit.quality.automatic'):model.qualityLabel,...(model.automatic&&model.qualityLabel?{detail:model.qualityLabel}:{})}]:[]),
  ...(model.speed.length?[{id:'speed' as const,label:t('player.opt.speed'),value:model.speed.find(row=>row.selected)?.label}]:[]),
 ];
}
