import {useEffect,useMemo,useState,useSyncExternalStore} from 'react';
import {router} from '../../router';
import {defaultI18n} from '@i18n';
import {shellLanguage} from '../../shell/settings';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import type {WebPlaybackRuntimeSnapshot} from '../../platform/playback';
import type {WatchBindings,WatchSurfaces,WatchPanel} from './types';
import {watchPanelModel} from './panel-model';
import {WatchOptions} from './WatchOptions';
import {WatchRoomPill} from './WatchRoomPill';
import {WatchStartRoom} from './StartRoom';
import {useTogetherBindings} from '../together/owner';
import {playbackIdentity} from '@core/presentation/kit-model/index.ts';
import {castDisplayName} from '../../platform/cast';
import {WatchInfoPanel} from './WatchInfoPanel';
import {watchLossNotice} from './loss-copy';
import {settingsPlatform} from '../../platform/settings-platform';
const noSubscribe=()=>()=>{},noPreferences=()=>null;
/** One adapter in the persistent parent; components borrow the existing runtime and stores. */
export function useWatchPanels(binding:WatchBindings|undefined,snapshot:WebPlaybackRuntimeSnapshot|undefined,context:ShellAssemblyContext|undefined):WatchSurfaces|undefined {
 const preferences=useSyncExternalStore(context?.preferences?.subscribe??noSubscribe,()=>context?.preferences?.getSnapshot('web')??context?.facts.preferences??null,noPreferences);
 const i18n=useMemo(()=>preferences?shellLanguage(preferences):defaultI18n,[preferences]);
 // This device's own setting, read from the app's device document in the same render.
 const deviceDocument=settingsPlatform()?.device?.getSnapshot().document;
 const together=useTogetherBindings(context),model=useMemo(()=>binding&&snapshot?watchPanelModel(binding.runtime,snapshot,i18n):null,[binding,snapshot,i18n]);
 const [startRoom,setStartRoom]=useState(false),room=together?.getSnapshot().room;
 // desktop-player f1: Watch Together is listed only when this playback has no room yet.
 const canStartRoom=!!together&&!!snapshot&&!snapshot.disposed&&snapshot.fence?.kind==='queue'&&snapshot.presentation?.kind==='video'
  &&!(room?.group&&(room.phase==='live'||room.phase==='reconnecting'));
 useEffect(()=>{if(!canStartRoom)setStartRoom(false);},[canStartRoom]);
 if(!binding||!snapshot||!context?.isCurrent())return undefined;
 const available:WatchPanel[]=[];
 if(model?.audio.length||model?.subtitles.length||model?.subtitleAdditions.length)available.push('subtitles-audio');
 if(model?.audio.length)available.push('audio');if(model?.subtitles.length||model?.subtitleAdditions.length)available.push('subtitles');
 if(model?.quality.some(row=>row.enabled))available.push('quality');if(model?.speed.length)available.push('speed');
 if(available.length)available.push('options');
 // Playback info f1: offered only when the server committed its own comparison for
 // this presentation; the panel draws those facts and this browser's observations.
 const infoReady=!!snapshot&&!snapshot.disposed&&snapshot.fence?.kind==='queue'&&!!snapshot.session?.playbackInfo&&!!snapshot.subject;
 if(infoReady)available.push('info');
 // Frame 6: one note per playback for Dolby Vision, HDR or Atmos lost here, unless
 // this device's own "Picture and sound notices" setting is off.
 const loss=snapshot?.session?.playbackInfo?watchLossNotice(i18n,snapshot.session.playbackInfo,snapshot.session.observedPlayback,String(snapshot.fence?.playback??snapshot.session.id)):undefined;
 return {available,render:panel=>{
   const live=panel.snapshot;
   if(panel.panel==='info'&&live.fence?.kind==='queue'&&live.session?.playbackInfo&&live.subject)
    return <WatchInfoPanel runtime={binding.runtime} snapshot={live} fence={live.fence} title={playbackIdentity(live.subject,'full',i18n).title}
      deviceName={castDisplayName()} i18n={i18n} layout={panel.layout} phoneTop={panel.phoneTop} returnFocusRef={panel.returnFocusRef}
      close={panel.close} reportError={binding.reportError}/>;
   return model&&panel.panel&&available.includes(panel.panel)&&model.isCurrent()?<WatchOptions model={model} context={panel} i18n={i18n} reportError={binding.reportError} info={infoReady} openSettings={()=>{if(!context.isCurrent()||!model.isCurrent())throw new DOMException('Playback source retired','AbortError');return router.navigate({to:'/settings'});}}/>:null;
  },
  quality:model?.qualityLabel&&model.qualityAccessibleLabel&&available.includes('quality')?{fence:model.fence,automatic:model.automatic,label:model.qualityLabel,accessibleLabel:model.qualityAccessibleLabel}:undefined,
  tracks:model?{fence:model.fence,audio:model.audioLabel,subtitles:model.subtitleLabel}:undefined,
  ...(loss?{loss:{notice:loss,enabled:deviceDocument?.values['lossNotices']!==false}}:{}),
  together:<WatchRoomPill binding={together} runtime={binding.runtime} snapshot={snapshot} reportError={binding.reportError}/>,
  more:canStartRoom&&together?[{id:'watch-together',label:i18n.t('web.settings.together'),action:()=>setStartRoom(true)}]:[],
  prompts:canStartRoom&&together?<WatchStartRoom binding={together} open={startRoom} onOpenChange={setStartRoom}/>:undefined,
 };
}
