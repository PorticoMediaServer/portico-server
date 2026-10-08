import type {PlaybackSnapshot} from '@core/index';
/** The dormant HTML element is not the PCM clock. Paused/waiting reports use a
 * nonzero declared rate because MediaSession rejects zero, with playbackState
 * paused so the OS does not extrapolate unrendered progress. */
export function systemListeningState(state:PlaybackSnapshot,element?:Pick<HTMLMediaElement,'paused'|'playbackRate'|'readyState'>|null){
 const effects=state.audioEffects;
 const effective=effects.rendering?(effects.effectiveRate??0):element&&!element.paused&&element.readyState>=3?element.playbackRate:0;
 const playing=state.intent==='playing'&&state.phase==='ready'&&!state.pendingSeek&&!state.failedSeek&&effective>0;
 const rate=playing?effective:effects.rendering?effects.rate:element?.playbackRate??1;
 return {playbackState:playing?'playing' as const:'paused' as const,
  position:{duration:state.duration,position:Math.max(0,Math.min(state.duration,state.positionSeconds)),playbackRate:Number.isFinite(rate)&&rate>0?rate:1}};
}
