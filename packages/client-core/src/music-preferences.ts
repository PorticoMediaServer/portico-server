/**
 * X-02: the viewer's music preferences are what the audio engine renders with. The server
 * publishes them per profile (`music.gapless`, `music.crossfadeSeconds`, `music.audioNormalization`,
 * scope profile-server); `PlaybackService.adoptAudioEffectsPreference` applies them, and the device's
 * cached copy (the queue journal) is only used until the server has answered, or offline.
 */
import {defaultAudioEffects,type AudioEffectsSettings} from './audio-effects.ts';
import {preferenceValue,type PreferenceSnapshot} from './preferences.ts';

/** The effects a preference snapshot asks for. A key the server doesn't publish (an older server)
 * keeps the default; a value outside the engine's domain is brought inside it. */
export function musicAudioEffects(snapshot:PreferenceSnapshot):AudioEffectsSettings {
 const read=<T,>(key:string,fallback:T,valid:(v:unknown)=>boolean):T=>{
  try{const v=preferenceValue(snapshot,key);return valid(v)?v as T:fallback;}catch{return fallback;}
 };
 const gapless=read('music.gapless',defaultAudioEffects.gapless,v=>typeof v==='boolean');
 const crossfade=read('music.crossfadeSeconds',defaultAudioEffects.crossfadeSeconds,v=>typeof v==='number'&&Number.isFinite(v));
 const normalization=read<AudioEffectsSettings['normalization']>('music.audioNormalization',defaultAudioEffects.normalization,v=>v==='off'||v==='track'||v==='album');
 return Object.freeze({gapless,crossfadeSeconds:Math.min(12,Math.max(0,Math.round(crossfade))),normalization});
}

/** Whether this device can render effects for what's playing now: undefined when nothing tells
 * (no music session yet); otherwise the server's version 2 plan, with its reason when it can't
 * (for example when the owner doesn't allow converting audio). A quiet notice, not an error. */
export function audioEffectsAvailability(session:{audio?:{mode:string;reason?:string}}|undefined):Readonly<{available:boolean;reason?:string}>|undefined {
 const plan=session?.audio;
 if(!plan)return undefined;
 return plan.mode==='unavailable'?{available:false,reason:plan.reason||undefined}:{available:true};
}

/** The viewer's defaults for a music queue that starts with Play: shuffle and repeat. */
export function musicQueueDefaults(snapshot:PreferenceSnapshot):Readonly<{shuffle:boolean;repeat:'off'|'one'|'all'}> {
 const read=<T,>(key:string,fallback:T,valid:(v:unknown)=>boolean):T=>{
  try{const v=preferenceValue(snapshot,key);return valid(v)?v as T:fallback;}catch{return fallback;}
 };
 return Object.freeze({shuffle:read('music.shuffleDefault',false,v=>typeof v==='boolean'),repeat:read<'off'|'one'|'all'>('music.repeatDefault','off',v=>v==='off'||v==='one'||v==='all')});
}
