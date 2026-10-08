/** Private DOM resource identity, not a public playback request. */
type ResourceSnapshot = { audioEffects?:{rendering:boolean};intentId:number; session?: {id:string;generation:number}|null; phase?:string };
export function webPlaybackResourceKey(snapshot:ResourceSnapshot):string {
  if(snapshot.audioEffects?.rendering&&snapshot.session)return 'audio-render';
  return JSON.stringify([snapshot.intentId,snapshot.session?.id??null,snapshot.session?.generation??null]);
}
/** The originating node AND captured resource identity must still own the current surface. */
export function acceptsWebPlaybackEvent(key:string,origin:object|null,currentNode:object|null,snapshot:ResourceSnapshot):boolean {
  return !snapshot.audioEffects?.rendering&&origin!==null&&origin===currentNode&&!!snapshot.session&&snapshot.phase!=='error'&&webPlaybackResourceKey(snapshot)===key;
}

/** Browsers can publish their final pause before the ended event. Reapplying a
 * still-playing intent at that boundary would call play() and restart the file. */
export function webMediaCompleted(media:{ended:boolean;paused:boolean;currentTime:number;duration:number}):boolean {
  return media.ended || media.paused && Number.isFinite(media.duration) && media.duration>0 && media.currentTime>=media.duration;
}
