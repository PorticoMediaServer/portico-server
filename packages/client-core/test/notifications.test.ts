import {test} from 'node:test';
import assert from 'node:assert/strict';
import {
 parseNotification,parseNotificationInbox,parseNotificationDelta,parseNotificationBatchResult,
 parseNotificationSummary,parseNotificationBroadcastResult,notificationActionSupported,
 mergeNotifications,parseFeedbackCapabilities,parseFeedbackSubmission,parseFeedbackList,
 type Notification,
} from '../src/notifications.ts';

const notification=(over:Record<string,unknown>={})=>({
 id:'n-1',audience:'profile',severity:'info',source:'downloads',category:'download.finished',
 title:'Download ready',body:'A title finished downloading.',
 arguments:{title:'A title',downloadId:'dl-1'},
 actions:[{kind:'command',label:'Open',command:'open-download',arguments:{downloadId:'dl-1'}}],
 dedupeKey:'download:dl-1',revision:7,createdAt:1,updatedAt:1,expiresAt:99,
 readAt:null,archivedAt:null,read:false,archived:false,cursor:'3',...over});

const inbox=(items:unknown[])=>({
 revision:7,audience:'all',audiences:['profile','account-admin'],state:'all',
 counts:{unread:1,read:0,archived:0,total:1},items,nextCursor:'',observedAt:2,retentionDays:180});

test('a notification parses with its action and its dedupe identity',()=>{
 const parsed=parseNotification(notification());
 assert.equal(parsed.dedupeKey,'download:dl-1');
 assert.equal(parsed.actions[0].command,'open-download');
 assert.equal(parsed.readAt,null);
});

test('an action outside the shape is refused rather than half-read',()=>{
 // A navigate carrying a command, or a command carrying a target, is not a
 // shape the server produces; accepting it would let one be mistaken for the other.
 assert.throws(()=>parseNotification(notification({actions:[{kind:'navigate',label:'Go',command:'run-scan'}]})));
 assert.throws(()=>parseNotification(notification({actions:[{kind:'command',label:'Go',command:'run-scan',target:{view:'home'}}]})));
 assert.throws(()=>parseNotification(notification({actions:[{kind:'explode',label:'Go'}]})));
 assert.throws(()=>parseNotification(notification({severity:'catastrophic'})));
 assert.throws(()=>parseNotification(notification({audience:'everyone'})));
 assert.throws(()=>parseNotification(notification({revision:-1})));
});

test('an unknown command is kept but reported as unsupported',()=>{
 // The server may be newer than this client. The action survives parsing so the
 // UI can show it disabled; it must not be silently dropped or guessed at.
 const parsed=parseNotification(notification({actions:[{kind:'command',label:'Do',command:'teleport'}]}));
 assert.equal(parsed.actions.length,1);
 assert.equal(notificationActionSupported(parsed.actions[0],['home']),false);
 const known=parseNotification(notification()).actions[0];
 assert.equal(notificationActionSupported(known,['home']),true);
 const navigate=parseNotification(notification({actions:[{kind:'navigate',label:'Go',target:{view:'atlantis'}}]})).actions[0];
 assert.equal(notificationActionSupported(navigate,['home','library']),false);
 assert.equal(notificationActionSupported(navigate,['atlantis']),true);
});

test('the inbox document carries its counts, revision and retention together',()=>{
 const parsed=parseNotificationInbox(inbox([notification()]));
 assert.equal(parsed.revision,7);
 assert.equal(parsed.counts.unread,1);
 assert.equal(parsed.retentionDays,180);
 assert.equal(parsed.items.length,1);
 assert.throws(()=>parseNotificationInbox(inbox(Array.from({length:101},()=>notification()))));
 assert.throws(()=>parseNotificationInbox({...inbox([]),counts:{unread:1}}));
});

test('a delta merges by identity so a restated condition does not double up',()=>{
 const held=[parseNotification(notification())];
 const delta=parseNotificationDelta({revision:9,since:7,resync:false,
  counts:{unread:1,read:0,archived:0,total:1},
  items:[notification({revision:9,title:'Download failed',severity:'warning'})]});
 const merged=mergeNotifications(held,delta);
 assert.equal(merged.length,1);
 assert.equal(merged[0].title,'Download failed');
 assert.equal(merged[0].severity,'warning');
 const added=mergeNotifications(merged,parseNotificationDelta({revision:10,since:9,resync:false,
  counts:{unread:2,read:0,archived:0,total:2},items:[notification({id:'n-2',createdAt:5})]}));
 assert.deepEqual(added.map((n:Notification)=>n.id),['n-2','n-1']);
});

test('a resync delta leaves the held list alone so a partial list is never merged',()=>{
 const held=[parseNotification(notification())];
 const delta=parseNotificationDelta({revision:99,since:7,resync:true,
  counts:{unread:0,read:0,archived:0,total:0},items:[]});
 assert.equal(delta.resync,true);
 assert.deepEqual(mergeNotifications(held,delta).map((n:Notification)=>n.id),['n-1']);
});

test('a batch result reports an outcome for every identifier',()=>{
 const parsed=parseNotificationBatchResult({revision:8,audience:'all',applied:1,
  counts:{unread:0,read:1,archived:0,total:1},
  receipts:[{action:'read',id:'n-1',outcome:'applied'},{action:'read-all',outcome:'unchanged'}]});
 assert.equal(parsed.receipts[0].outcome,'applied');
 assert.equal(parsed.receipts[1].id,undefined);
 assert.throws(()=>parseNotificationBatchResult({revision:8,audience:'all',applied:0,
  counts:{unread:0,read:0,archived:0,total:0},receipts:[{action:'read',outcome:'maybe'}]}));
});

test('the summary and the broadcast report parse to their published shapes',()=>{
 assert.equal(parseNotificationSummary({revision:4,audience:'profile',observedAt:1,
  counts:{unread:2,read:1,archived:0,total:3}}).counts.unread,2);
 const broadcast=parseNotificationBroadcastResult({audience:'profile',delivered:3,created:2,updated:1,
  revision:11,dedupeKey:'maintenance-window',ids:['a','b']});
 assert.equal(broadcast.delivered,3);
 assert.throws(()=>parseNotificationBroadcastResult({audience:'profile',delivered:-1,created:0,updated:0,
  revision:1,dedupeKey:'k',ids:[]}));
});

const capabilities={
 revision:'e1.0',canSubmit:true,
 kinds:[{id:'playback',label:'Playback',categories:[
  {id:'buffering',label:'Buffering',description:'It keeps pausing.',wantsPlaybackSession:true,wantsItem:true}]}],
 maxMessageLength:2000,minMessageLength:8,diagnosticsSupported:true,diagnosticsOptional:true,
 duplicateWindowHours:24,retentionDays:180,statuses:['open','in-progress','resolved','closed'],
 diagnosticsDecisions:['attached','unavailable','declined','not-requested'],
 perProfileHourlyLimit:10,reporterName:'Sam',reporterAuthority:'local'};

test('feedback capabilities say what may be sent and why it may not',()=>{
 const parsed=parseFeedbackCapabilities(capabilities);
 assert.equal(parsed.kinds[0].categories[0].wantsPlaybackSession,true);
 const blocked=parseFeedbackCapabilities({...capabilities,canSubmit:false,
  submitBlockedReason:"This profile's access has been revoked on this server."});
 assert.equal(blocked.canSubmit,false);
 assert.match(blocked.submitBlockedReason??'',/revoked/);
 // A kind with no categories cannot be rendered as a two-level chooser.
 assert.throws(()=>parseFeedbackCapabilities({...capabilities,kinds:[{id:'x',label:'X',categories:[]}]}));
});

const report={
 id:'r-1',cursor:'4',revision:1,status:'open',kind:'playback',category:'buffering',
 message:'It pauses every minute.',itemId:'item-1',createdAt:1,updatedAt:1,expiresAt:9,
 reporter:{name:'Sam',authority:'local',role:'member',self:true},
 diagnostics:{decision:'attached',reference:'playback-session:s-1',detail:'Linked.'},
 duplicates:0,thread:[{sequence:1,at:1,revision:1,status:'open',reply:'',actorClass:'viewer'}]};

test('a duplicate submission returns the existing report rather than a new one',()=>{
 const created=parseFeedbackSubmission({report,duplicate:false,created:true});
 assert.equal(created.created,true);
 const duplicate=parseFeedbackSubmission({report:{...report,duplicates:1},duplicate:true,created:false,duplicateOf:report.id});
 assert.equal(duplicate.duplicate,true);
 assert.equal(duplicate.duplicateOf,report.id);
 assert.equal(duplicate.created,false);
 assert.equal(duplicate.report.id,created.report.id);
 assert.equal(duplicate.report.duplicates,1);
});

test('every diagnostics decision parses, and an invented one does not',()=>{
 for(const decision of ['attached','unavailable','declined','not-requested']){
  const parsed=parseFeedbackSubmission({report:{...report,diagnostics:{decision}},duplicate:false,created:true});
  assert.equal(parsed.report.diagnostics.decision,decision);
 }
 assert.throws(()=>parseFeedbackSubmission({report:{...report,diagnostics:{decision:'maybe'}},duplicate:false,created:true}));
});

test('the triage list carries its status counts with the page',()=>{
 const parsed=parseFeedbackList({items:[report],nextCursor:'',total:3,observedAt:2,
  statusCounts:{open:2,'in-progress':1,resolved:0,closed:0},filter:{kind:'playback'}});
 assert.equal(parsed.statusCounts.open,2);
 assert.equal(parsed.filter.kind,'playback');
 assert.throws(()=>parseFeedbackList({items:[report],nextCursor:'',total:1,observedAt:2,
  statusCounts:{open:'many'},filter:{}}));
});
