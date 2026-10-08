import type {PlaybackSession} from './index.ts';
import type {AudioRenderV2} from './playback-v1/audio-render.ts';

/** Render-only settings. No controller, lease, item list or observation writer. */
export type AudioEffectsSettings=Readonly<{gapless:boolean;crossfadeSeconds:number;normalization:'off'|'track'|'album'}>;
/** Before the server's music preferences are known (and when a device has none cached): the
 * registry's own defaults, so gapless is on (X-02). */
export const defaultAudioEffects:AudioEffectsSettings=Object.freeze({gapless:true,crossfadeSeconds:0,normalization:'off'});
/** Every effect off: what a viewer who turned them all off gets. */
export const noAudioEffects:AudioEffectsSettings=Object.freeze({gapless:false,crossfadeSeconds:0,normalization:'off'});
/** A prepared next track (§18.2): the version 2 on-device-decode plan (`audio`, §18.1)
 * that v1 queues prepare. */
export type PreparedAudio=Readonly<{token:string;expiresAtMs:number;audio?:AudioRenderV2}>;
export type AudioEffectsSnapshot=Readonly<{
 settings:AudioEffectsSettings;rendering:boolean;rate:number;notice:string;effectiveRate?:number;waiting?:boolean;
 prepared?:PreparedAudio;committed?:Readonly<{token:string;itemId:string;session:PlaybackSession}>;
}>;
export function parseAudioEffects(v:any):AudioEffectsSettings {
 if(!v||typeof v.gapless!=='boolean'||!Number.isFinite(v.crossfadeSeconds)||v.crossfadeSeconds<0||v.crossfadeSeconds>12||!['off','track','album'].includes(v.normalization)||Object.keys(v).some(k=>!['gapless','crossfadeSeconds','normalization'].includes(k)))throw new Error('Invalid audio processing settings.');
 return Object.freeze({gapless:v.gapless,crossfadeSeconds:v.crossfadeSeconds,normalization:v.normalization});
}
export const audioEffectsEnabled=(s:AudioEffectsSettings)=>s.gapless||s.crossfadeSeconds>0||s.normalization!=='off';
export const audioTransitionsEnabled=(s:AudioEffectsSettings)=>s.gapless||s.crossfadeSeconds>0;
export function transitionTiming(now:number,end:number,seconds:number,rate:number){
 if(![now,end,seconds,rate].every(Number.isFinite)||seconds<0||seconds>12||rate<.25||rate>4)throw new Error('Invalid audio transition timing.');
 // Scheduling uses the engine clock. A late network result must not cut samples
 // from an already-playing incoming item or pretend a missed edge was gapless.
 const start=Math.max(now+.03,end-seconds),duration=Math.max(0,Math.min(seconds,end-start));
 return {start,duration,late:start>end};
}
