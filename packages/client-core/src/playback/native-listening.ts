import type {AudioEffectsSettings} from '../audio-effects.ts';
import type {QueueHeader,Session,SessionEnd} from '../playback-v1/types.ts';
/** Absolute user deadlines; neither a render nor a lease renewal restarts them. */
export type ListeningDeadlines = Readonly<{sleepAtMs:number|null;passoutAtMs:number|null}>;
export const noListeningDeadlines:ListeningDeadlines=Object.freeze({sleepAtMs:null,passoutAtMs:null});
/** Non-authorizing policy. The native owner applies it to the session it plays; neither policy
 * revisions nor activity send playback commands. */
export type ListeningPolicy=Readonly<{
 preferences:Readonly<{musicRate:number;bookRate:number;autoplayNext:boolean;passoutMinutes:number}>|null;
 activityRevision:number;activityAtMs:number;timerRevision:number;
 timer:Readonly<{choice:string|number;label:string;wallDeadline?:number;endSeconds?:number;itemId?:string;bookId?:string;sessionId?:string;generation?:number;libraryId?:string}>|null;
}>;
export type NativeListeningPolicy=ListeningPolicy&Readonly<{revision:number;effects:AudioEffectsSettings}>;
export type ListeningAction='pause'|'resume'|'seek'|'rate'|'retry'|'next'|'previous';
/** What every native listening snapshot says. The player UI, the engine coordinator and the
 * listening policy read only this. */
export type NativeListeningStatus=Readonly<{
 owner:string; sequence:number; intent:'playing'|'paused'; rate:number;
 positionSeconds:number; effectiveRate?:number;supportedRates?:readonly number[];rateWaiting?:boolean;rateError?:string; observed:boolean; ready:boolean; seeking:boolean; ended:boolean;
 error:string|null; terminal:boolean; route:string; deadlines:ListeningDeadlines;
 rendering?:boolean;timerStopped?:boolean;expiredTimerRevision?:number;policyRevision?:number;
 commandRevision:number; binding:{lease:string;intentId:number;sessionId:string;generation:number;itemId:string}|null;
}>;
export function listeningDeadline(value:unknown):value is number|null {
 return value===null||typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
}
/** Browser fallback. The monotonic check prevents a clock rollback extending a
 * timer; wall-clock checking on foreground catches suspension/forward changes. */
export class ListeningDeadlineClock {
 private value:ListeningDeadlines=noListeningDeadlines;
 private limits:Partial<Record<keyof ListeningDeadlines,number>>={};
 private timer?:ReturnType<typeof setTimeout>;
 private expire:(kind:'sleepAtMs'|'passoutAtMs')=>void;private wall:()=>number;private mono:()=>number;
 constructor(expire:(kind:'sleepAtMs'|'passoutAtMs')=>void,wall:()=>number=Date.now,mono:()=>number=()=>typeof performance==='undefined'?Date.now():performance.now()){this.expire=expire;this.wall=wall;this.mono=mono;}

 getSnapshot(){return this.value;}
 set(value:ListeningDeadlines){
  if(!listeningDeadline(value.sleepAtMs)||!listeningDeadline(value.passoutAtMs))throw new Error('Invalid listening deadline.');
  for(const key of ['sleepAtMs','passoutAtMs'] as const){if(value[key]!==this.value[key]){if(value[key]===null)delete this.limits[key];else this.limits[key]=this.mono()+Math.max(0,value[key]!-this.wall());}}
  this.value=Object.freeze({...value});this.schedule();
 }
 check(){for(const key of ['sleepAtMs','passoutAtMs'] as const){const at=this.value[key];if(at!==null&&(this.wall()>=at||this.mono()>=(this.limits[key]??Infinity))){this.value=Object.freeze({...this.value,[key]:null});delete this.limits[key];this.expire(key);}}this.schedule();}
 private schedule(){if(this.timer)clearTimeout(this.timer);this.timer=undefined;const pending=Object.values(this.limits);if(pending.length){this.timer=setTimeout(()=>this.check(),Math.max(1,Math.min(1000,...pending.map(at=>at-this.mono()))));(this.timer as any)?.unref?.();}}
 clear(){if(this.timer)clearTimeout(this.timer);this.timer=undefined;this.value=noListeningDeadlines;this.limits={};}
}

// ── Playback v1 (Plan — Client Playback Migration §9.3 C3) ──────────────────────────────
/**
 * The native owner of a v1 audio session (Apple): it reports the session's timeline (and so holds
 * its lease), pauses and resumes it, and moves the queue on (`:advance`, and §18's
 * `:prepare-next`/`:commit-next` for gapless and crossfade) while JS sleeps. JS hands a session
 * over once, with the `seq` it had reached, and from then on only sends intents; it never reports
 * the session's timeline itself. A queue move the owner made shows up in the next snapshot as a
 * `transition`: a new session (and the queue header it came with), which JS adopts as its own.
 */
export type NativeListeningV1Context=Readonly<{
 /** The v1 session (`parseSession`'s shape; it is also the wire shape). */
 session:Session;
 queueId:string|null; queueRevision:string|null;
 durationSeconds:number;
 /** The highest timeline `seq` already sent for this session (0 when none). */
 lastSeq:number;
 /** How much of the server's 120 s lease is left, as JS last knew it. */
 leaseMs:number;
 /** The server offers §18 queue transitions (`features.queueTransitions`). */
 transitions:boolean;
 intent:'playing'|'paused';
}>;
export type NativeListeningV1Event=NativeListeningStatus&Readonly<{
 /** The session the owner reports for now (after a transition, the new one). */
 session:Session; durationSeconds:number;
 queue:Readonly<{id:string;revision:string}>|null;
 /** The highest timeline `seq` the owner has sent for `session`. */
 lastSeq:number;
 /** Why the server ended the session (terminal events only). */
 end?:SessionEnd;
 /** A queue move the owner made while JS may have slept: the session it left, and the queue
  * header the move returned. Carried until JS acknowledges a later snapshot. */
 transition?:Readonly<{previousSessionId:string;queue:QueueHeader|null}>;
 /** A quiet notice for the viewer (X-02: this audio plays without gapless or normalization). */
 notice?:string;
}>;
export type NativeQueueAdvance='next'|'previous'|'entry';
export interface NativeListeningV1Control {
 adopt(context:NativeListeningV1Context,sink:(event:NativeListeningV1Event)=>void):Promise<void>;
 action(action:ListeningAction,value?:number):Promise<void>;
 /** A queue move the viewer asked for; the owner sends the `:advance` and plays its session. */
 advance(reason:NativeQueueAdvance,entryId?:string):Promise<void>;
 refresh():Promise<void>;
 deadlines(value:ListeningDeadlines):Promise<void>;
 configure?(policy:NativeListeningPolicy):Promise<void>;
 /** Ends native ownership; `stop` also stops the session on the server. */
 detach(stop:boolean):Promise<void>;
 /** The device's event stream says the session changed (an administrator ended it): the owner
  * checks at once rather than at its next timeline report. */
 sessionUpdated?(sessionId:string):Promise<void>;
}
