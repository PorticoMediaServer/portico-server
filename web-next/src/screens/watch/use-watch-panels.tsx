import {useMemo,useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {shellLanguage} from '../../shell/settings';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import type {WebPlaybackRuntimeSnapshot} from '../../platform/playback';
import type {WatchBindings,WatchSurfaces,WatchPanel} from './types';
import {watchPanelModel} from './panel-model';
import {WatchOptions} from './WatchOptions';
import {WatchRoomPill} from './WatchRoomPill';
import {useTogetherBindings} from '../together/owner';
const noSubscribe=()=>()=>{},noPreferences=()=>null;
/** One adapter in the persistent parent; components borrow the existing runtime and stores. */
export function useWatchPanels(binding:WatchBindings|undefined,snapshot:WebPlaybackRuntimeSnapshot|undefined,context:ShellAssemblyContext|undefined):WatchSurfaces|undefined {
 const preferences=useSyncExternalStore(context?.preferences?.subscribe??noSubscribe,()=>context?.preferences?.getSnapshot('web')??context?.facts.preferences??null,noPreferences);
 const i18n=useMemo(()=>preferences?shellLanguage(preferences):defaultI18n,[preferences]);
 const together=useTogetherBindings(context),model=useMemo(()=>binding&&snapshot?watchPanelModel(binding.runtime,snapshot,i18n):null,[binding,snapshot,i18n]);
 if(!binding||!snapshot||!context?.isCurrent())return undefined;
 const available:WatchPanel[]=[];
 if(model?.audio.length||model?.subtitles.length)available.push('subtitles-audio');
 if(model?.audio.length)available.push('audio');if(model?.subtitles.length)available.push('subtitles');
 if(model?.quality.some(row=>row.enabled))available.push('quality');if(model?.speed.length)available.push('speed');
 if(available.length)available.push('options');
 return {available,render:panel=>model&&panel.panel&&available.includes(panel.panel)&&model.isCurrent()?<WatchOptions model={model} context={panel} i18n={i18n} reportError={binding.reportError}/>:null,
  quality:model?.qualityLabel&&model.qualityAccessibleLabel&&available.includes('quality')?{fence:model.fence,automatic:model.automatic,label:model.qualityLabel,accessibleLabel:model.qualityAccessibleLabel}:undefined,
  tracks:model?{fence:model.fence,audio:model.audioLabel,subtitles:model.subtitleLabel}:undefined,
  together:<WatchRoomPill binding={together} snapshot={snapshot} reportError={binding.reportError}/>,
 };
}
