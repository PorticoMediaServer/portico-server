import {unreadableServerResponse} from './server-messages.ts';
/** Video post-play: the server's finish policy for a queue playback. Countdown, autoplay and
 * passout protection are server preferences; the client renders them and never substitutes its
 * own. */
export type QueuePostPlay=Readonly<{nextEntryId:string|null;available:boolean;reason:'ready'|'end'|'unavailable';countdownSeconds:0|5|10|15;autoplay:boolean;passoutCheckDue:boolean;automaticAdvances:number}>;
export type QueueAdvanceMode='automatic'|'manual'|'still-watching';
const obj=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const wireID=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9_-]{1,128}$/.test(v);
const only=(v:Record<string,unknown>,keys:readonly string[]):void=>{if(Object.keys(v).some(k=>!keys.includes(k)))fail();};
function fail():never{throw new Error(unreadableServerResponse);}
/** The next entry in postPlay is the queue's own next entry; a projection that
 * disagrees with itself is refused rather than reconciled here. */
export function parseQueuePostPlay(raw:unknown,next?:Readonly<{entryId:string|null;available:boolean;reason:string}>):QueuePostPlay{
 if(!obj(raw))fail();only(raw,['nextEntryId','available','reason','countdownSeconds','autoplay','passoutCheckDue','automaticAdvances']);
 if(raw.nextEntryId!==null&&!wireID(raw.nextEntryId))fail();
 if(typeof raw.available!=='boolean'||!['ready','end','unavailable'].includes(String(raw.reason)))fail();
 if(![0,5,10,15].includes(Number(raw.countdownSeconds))||typeof raw.autoplay!=='boolean'||typeof raw.passoutCheckDue!=='boolean')fail();
 if(!Number.isSafeInteger(raw.automaticAdvances)||Number(raw.automaticAdvances)<0||Number(raw.automaticAdvances)>10000)fail();
 if(raw.available&&(raw.nextEntryId===null||raw.reason!=='ready'))fail();
 if(next&&(next.entryId!==raw.nextEntryId||next.available!==raw.available||next.reason!==raw.reason))fail();
 return Object.freeze({nextEntryId:raw.nextEntryId as string|null,available:raw.available,reason:raw.reason as QueuePostPlay['reason'],countdownSeconds:raw.countdownSeconds as QueuePostPlay['countdownSeconds'],autoplay:raw.autoplay,passoutCheckDue:raw.passoutCheckDue,automaticAdvances:raw.automaticAdvances as number});
}
/** An automatic advance is refused with this code while the server is waiting for
 * the viewer to confirm they are still watching; the client answers by advancing
 * with mode "still-watching", never by retrying "automatic". */
export const passoutCheckRequired='passout_check_required';
