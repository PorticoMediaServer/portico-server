import {useEffect,useRef,type RefObject} from 'react';
import type {PlaybackService,PlaybackSnapshot,MediaItem,HttpLocalApi} from '@core/index.ts';
import type {QueueController as QueuePlayer} from '@core/queue-controller.ts';
import {protectedArtwork} from './protectedArtwork';
import {systemListeningState} from './system-listening-state';
let systemOwner:symbol|undefined;
/** One MediaSession belongs to the persistent player, not expanded/collapsed UI.
 * Browsers without this API retain the existing visible transport controls. */
export function useSystemListening(service:PlaybackService,state:PlaybackSnapshot,element:RefObject<HTMLVideoElement|null>,item:MediaItem,queue:QueuePlayer|undefined,api:HttpLocalApi,token:()=>string){
 const owned=useRef<symbol|undefined>(undefined);
 useEffect(()=>{
  const session=globalThis.navigator?.mediaSession;if(!session||!state.session||state.itemId!==item.id||!['song','audiobook_file'].includes(item.kind))return;
  const owner=Symbol('listening'),playbackId=state.session.id,generation=state.session.generation;systemOwner=owner;owned.current=owner;
  const current=()=>{const now=service.getSnapshot();return systemOwner===owner&&now.session?.id===playbackId&&now.session.generation===generation;};
  const actions:MediaSessionAction[]=[];
  const bind=(action:MediaSessionAction,handler:(event:MediaSessionActionDetails)=>void)=>{try{session.setActionHandler(action,event=>{if(current()){service.checkListeningDeadlines();queue?.listening.interact();handler(event);}});actions.push(action);}catch{/* A missing individual OS action is not a missing player feature. */}};
  bind('play',()=>service.resume());bind('pause',()=>service.pause());bind('stop',()=>service.leave());
  bind('seekto',event=>{if(Number.isFinite(event.seekTime))service.seek(event.seekTime!);});
  bind('seekbackward',event=>service.seek(service.getSnapshot().positionSeconds-(event.seekOffset??30)));
  bind('seekforward',event=>service.seek(service.getSnapshot().positionSeconds+(event.seekOffset??30)));
  if(queue){
   bind('nexttrack',()=>{const view=queue.getSnapshot().view;if(view?.next.available)void queue.next(view.queue.id,view.queue.revision).catch(()=>{});});
   bind('previoustrack',()=>{void queue.previous().catch(()=>{});});
  }
  const song=(item as MediaItem&{song?:{artist:string;albumTitle:string}}).song;
  const metadata:MediaMetadataInit={title:item.title,artist:song?.artist??'',album:song?.albumTitle??''};
  if(typeof MediaMetadata!=='undefined')session.metadata=new MediaMetadata(metadata);
  let objectURL='';const controller=new AbortController();
  if(item.posterUrl)void protectedArtwork(api,item.posterUrl,token(),controller.signal).then(blob=>{if(!current())return;objectURL=URL.createObjectURL(blob);if(typeof MediaMetadata!=='undefined')session.metadata=new MediaMetadata({...metadata,artwork:[{src:objectURL,type:blob.type}]});}).catch(()=>{});
  return()=>{controller.abort();if(objectURL)URL.revokeObjectURL(objectURL);if(owned.current===owner)owned.current=undefined;if(systemOwner!==owner)return;for(const action of actions){try{session.setActionHandler(action,null);}catch{}}session.metadata=null;session.playbackState='none';try{session.setPositionState?.();}catch{}systemOwner=undefined;};
 },[service,state.session?.id,state.session?.generation,item.id,item.title,item.posterUrl,item.kind,queue,api]); // eslint-disable-line react-hooks/exhaustive-deps -- token is read at call time; it rotates every ~13 minutes
 useEffect(()=>{const media=navigator.mediaSession;if(!media||!systemOwner||systemOwner!==owned.current||!state.session)return;const projection=systemListeningState(state,element.current);media.playbackState=projection.playbackState;if(state.duration>0){try{media.setPositionState?.(projection.position);}catch{}}},[service,state.intent,state.phase,state.positionSeconds,state.duration,state.session,state.audioEffects,state.pendingSeek,state.failedSeek,element]);
 useEffect(()=>{const reconcile=()=>{service.checkListeningDeadlines();if(document.visibilityState==='visible')void service.refreshPlaybackAuthority().catch(()=>{});};document.addEventListener('visibilitychange',reconcile);window.addEventListener('pageshow',reconcile);return()=>{document.removeEventListener('visibilitychange',reconcile);window.removeEventListener('pageshow',reconcile);};},[service]);
}
