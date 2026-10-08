/**
 * The notification inbox and viewer feedback wire shapes.
 *
 * Three rules a client has to hold on to, and which these parsers enforce:
 *
 * - One monotonic `revision` per inbox fences a batch write, resumes an event
 *   stream (`Last-Event-ID`) and parks a long poll. Keep exactly one.
 * - An action is either a navigation to an allowlisted view or one of six
 *   allowlisted commands. An action naming something this client does not know
 *   is kept, not dropped, and reported by `notificationActionSupported` so the
 *   UI can render it disabled rather than guessing at its behaviour.
 * - A notification's `dedupeKey` identifies the *condition*, not the message. A
 *   producer restating a condition edits the record in place, so a client must
 *   key its list on `id` and re-render on `revision`, never append blindly.
 */

/** The complete command allowlist. A server may publish more; see capabilities. */
export const notificationCommands=['retry-job','open-download','dismiss-conflict','review-device','open-feedback','run-scan'] as const;
export type NotificationCommand=(typeof notificationCommands)[number];

export type NotificationAudience='profile'|'account-admin';
export type NotificationSeverity='info'|'warning'|'critical';
export type NotificationState='unread'|'read'|'archived'|'all';
export type NotificationOutcome='applied'|'unchanged'|'not-found';

export type NavigateTarget={view:string;entityId?:string;libraryId?:string};
export type NotificationAction={
 kind:'navigate'|'command';
 label:string;
 target?:NavigateTarget;
 command?:string;
 arguments?:Record<string,string>;
};
export type Notification={
 id:string;
 audience:NotificationAudience;
 severity:NotificationSeverity;
 source:string;
 category:string;
 title:string;
 body:string;
 arguments:Record<string,string>;
 actions:NotificationAction[];
 dedupeKey:string;
 revision:number;
 createdAt:number;
 updatedAt:number;
 expiresAt:number;
 readAt:number|null;
 archivedAt:number|null;
 read:boolean;
 archived:boolean;
 cursor:string;
};
export type NotificationCounts={unread:number;read:number;archived:number;total:number};
export type NotificationInbox={
 revision:number;
 audience:string;
 audiences:string[];
 state:string;
 counts:NotificationCounts;
 items:Notification[];
 nextCursor:string;
 observedAt:number;
 retentionDays:number;
};
export type NotificationSummary={revision:number;audience:string;counts:NotificationCounts;observedAt:number};
export type NotificationDelta={revision:number;since:number;resync:boolean;items:Notification[];counts:NotificationCounts};
export type NotificationReceipt={action:string;id?:string;outcome:NotificationOutcome};
export type NotificationBatchResult={revision:number;audience:string;applied:number;receipts:NotificationReceipt[];counts:NotificationCounts};
export type NotificationBroadcastResult={audience:string;delivered:number;created:number;updated:number;revision:number;dedupeKey:string;ids:string[]};

const obj=(v:unknown):v is Record<string,any>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max:number):v is string=>typeof v==='string'&&v.length<=max;
const whole=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const nullableWhole=(v:unknown):v is number|null=>v===null||whole(v);
function invalid():never{throw new Error('Invalid notification response.');}

function parseArguments(value:unknown,max:number):Record<string,string>{
 if(value===undefined)return {};
 if(!obj(value))invalid();
 const keys=Object.keys(value);
 if(keys.length>max)invalid();
 for(const key of keys)if(!text(key,48)||!text(value[key],400))invalid();
 return value as Record<string,string>;
}

export function parseNotificationAction(value:unknown):NotificationAction{
 if(!obj(value)||!text(value.label,120)||!value.label)invalid();
 const args=parseArguments(value.arguments,8);
 if(value.kind==='navigate'){
  if(!obj(value.target)||!text(value.target.view,80)||!value.target.view)invalid();
  if(value.target.entityId!==undefined&&!text(value.target.entityId,200))invalid();
  if(value.target.libraryId!==undefined&&!text(value.target.libraryId,200))invalid();
  if(value.command!==undefined)invalid();
  return {kind:'navigate',label:value.label,target:value.target as NavigateTarget,arguments:args};
 }
 if(value.kind==='command'){
  if(!text(value.command,80)||!value.command||value.target!==undefined)invalid();
  return {kind:'command',label:value.label,command:value.command,arguments:args};
 }
 invalid();
}

/**
 * Whether this build knows how to carry out an action. An unknown command or
 * view means the server is newer than the client: show the action disabled,
 * never invent behaviour from the string.
 */
export function notificationActionSupported(action:NotificationAction,views:readonly string[]):boolean{
 if(action.kind==='command')return (notificationCommands as readonly string[]).includes(action.command??'');
 return views.includes(action.target?.view??'');
}

export function parseNotification(value:unknown):Notification{
 if(!obj(value))invalid();
 if(!text(value.id,200)||!value.id)invalid();
 if(value.audience!=='profile'&&value.audience!=='account-admin')invalid();
 if(!['info','warning','critical'].includes(value.severity))invalid();
 if(!text(value.source,60)||!value.source||!text(value.category,80)||!value.category)invalid();
 if(!text(value.title,300)||!value.title||!text(value.body,2000))invalid();
 if(!text(value.dedupeKey,240))invalid();
 if(!whole(value.revision)||!whole(value.createdAt)||!whole(value.updatedAt)||!whole(value.expiresAt))invalid();
 if(!nullableWhole(value.readAt)||!nullableWhole(value.archivedAt))invalid();
 if(typeof value.read!=='boolean'||typeof value.archived!=='boolean')invalid();
 if(!text(value.cursor,64))invalid();
 if(!Array.isArray(value.actions)||value.actions.length>3)invalid();
 return {
  ...value,
  arguments:parseArguments(value.arguments,12),
  actions:value.actions.map(parseNotificationAction),
 } as Notification;
}

function parseCounts(value:unknown):NotificationCounts{
 if(!obj(value)||!whole(value.unread)||!whole(value.read)||!whole(value.archived)||!whole(value.total))invalid();
 return value as NotificationCounts;
}

/** The server caps a page at 100; a longer one is a response we do not trust. */
export function parseNotificationInbox(value:unknown):NotificationInbox{
 if(!obj(value)||!whole(value.revision)||!whole(value.observedAt)||!whole(value.retentionDays))invalid();
 if(!text(value.audience,40)||!text(value.state,20)||!text(value.nextCursor,64))invalid();
 if(!Array.isArray(value.audiences)||value.audiences.length>4)invalid();
 if(!Array.isArray(value.items)||value.items.length>100)invalid();
 return {...value,counts:parseCounts(value.counts),items:value.items.map(parseNotification)} as NotificationInbox;
}

export function parseNotificationSummary(value:unknown):NotificationSummary{
 if(!obj(value)||!whole(value.revision)||!whole(value.observedAt)||!text(value.audience,40))invalid();
 return {...value,counts:parseCounts(value.counts)} as NotificationSummary;
}

/**
 * A stream frame or a long-poll answer. `resync` means more moved than one
 * frame carries: re-read the inbox instead of merging a partial list.
 */
export function parseNotificationDelta(value:unknown):NotificationDelta{
 if(!obj(value)||!whole(value.revision)||!whole(value.since)||typeof value.resync!=='boolean')invalid();
 if(!Array.isArray(value.items)||value.items.length>100)invalid();
 return {...value,counts:parseCounts(value.counts),items:value.items.map(parseNotification)} as NotificationDelta;
}

export function parseNotificationBatchResult(value:unknown):NotificationBatchResult{
 if(!obj(value)||!whole(value.revision)||!whole(value.applied)||!text(value.audience,40))invalid();
 if(!Array.isArray(value.receipts)||value.receipts.length>256)invalid();
 for(const receipt of value.receipts){
  if(!obj(receipt)||!text(receipt.action,40)||!['applied','unchanged','not-found'].includes(receipt.outcome))invalid();
  if(receipt.id!==undefined&&!text(receipt.id,200))invalid();
 }
 return {...value,counts:parseCounts(value.counts)} as NotificationBatchResult;
}

export function parseNotificationBroadcastResult(value:unknown):NotificationBroadcastResult{
 if(!obj(value)||!whole(value.delivered)||!whole(value.created)||!whole(value.updated)||!whole(value.revision))invalid();
 if(!text(value.audience,40)||!text(value.dedupeKey,240))invalid();
 if(!Array.isArray(value.ids)||value.ids.length>50)invalid();
 return value as NotificationBroadcastResult;
}

/**
 * Merge a delta into a held list. Deduping is by `id` because a producer
 * restating a condition edits the record it already wrote; appending would show
 * the same condition twice.
 */
export function mergeNotifications(held:readonly Notification[],delta:NotificationDelta):Notification[]{
 if(delta.resync)return [...held];
 const byID=new Map(held.map(n=>[n.id,n]));
 for(const item of delta.items)byID.set(item.id,item);
 return [...byID.values()].sort((a,b)=>b.createdAt-a.createdAt||(a.id<b.id?1:-1));
}

/* Feedback. */

export type FeedbackItemStatus='open'|'in-progress'|'resolved'|'closed';
export type FeedbackDecision='attached'|'unavailable'|'declined'|'not-requested';
export type FeedbackCategoryDefinition={id:string;label:string;description:string;wantsPlaybackSession:boolean;wantsItem:boolean};
export type FeedbackKindDefinition={id:string;label:string;categories:FeedbackCategoryDefinition[]};
export type FeedbackCapabilities={
 revision:string;
 canSubmit:boolean;
 submitBlockedReason?:string;
 kinds:FeedbackKindDefinition[];
 maxMessageLength:number;
 minMessageLength:number;
 diagnosticsSupported:boolean;
 diagnosticsOptional:boolean;
 duplicateWindowHours:number;
 retentionDays:number;
 statuses:string[];
 diagnosticsDecisions:string[];
 perProfileHourlyLimit:number;
 reporterName:string;
 reporterAuthority:string;
};
export type FeedbackReporter={name:string;authority:string;role:string;self:boolean};
export type FeedbackDiagnostics={decision:FeedbackDecision;reference?:string;detail?:string};
export type FeedbackThreadEvent={sequence:number;at:number;revision:number;status:string;reply:string;actorClass:string};
export type FeedbackItem={
 id:string;
 cursor:string;
 revision:number;
 status:FeedbackItemStatus;
 kind:string;
 category:string;
 message:string;
 itemId?:string;
 createdAt:number;
 updatedAt:number;
 expiresAt:number;
 reporter:FeedbackReporter;
 diagnostics:FeedbackDiagnostics;
 duplicateOf?:string;
 duplicates:number;
 thread:FeedbackThreadEvent[];
};
export type FeedbackSubmissionResult={report:FeedbackItem;duplicate:boolean;created:boolean;duplicateOf?:string};
export type FeedbackList={
 items:FeedbackItem[];
 nextCursor:string;
 statusCounts:Record<string,number>;
 total:number;
 filter:Record<string,string>;
 observedAt:number;
};

export function parseFeedbackCapabilities(value:unknown):FeedbackCapabilities{
 if(!obj(value)||!text(value.revision,40)||typeof value.canSubmit!=='boolean')invalid();
 if(!whole(value.maxMessageLength)||!whole(value.minMessageLength)||!whole(value.duplicateWindowHours))invalid();
 if(!whole(value.retentionDays)||!whole(value.perProfileHourlyLimit))invalid();
 if(typeof value.diagnosticsSupported!=='boolean'||typeof value.diagnosticsOptional!=='boolean')invalid();
 if(value.submitBlockedReason!==undefined&&!text(value.submitBlockedReason,400))invalid();
 if(!text(value.reporterName,200)||!text(value.reporterAuthority,40))invalid();
 for(const list of [value.statuses,value.diagnosticsDecisions])if(!Array.isArray(list)||list.length>12)invalid();
 if(!Array.isArray(value.kinds)||value.kinds.length<1||value.kinds.length>20)invalid();
 for(const kind of value.kinds){
  if(!obj(kind)||!text(kind.id,60)||!kind.id||!text(kind.label,120))invalid();
  if(!Array.isArray(kind.categories)||kind.categories.length<1||kind.categories.length>30)invalid();
  for(const category of kind.categories){
   if(!obj(category)||!text(category.id,60)||!category.id||!text(category.label,120)||!text(category.description,400))invalid();
   if(typeof category.wantsPlaybackSession!=='boolean'||typeof category.wantsItem!=='boolean')invalid();
  }
 }
 return value as FeedbackCapabilities;
}

export function parseFeedbackReport(value:unknown):FeedbackItem{
 if(!obj(value)||!text(value.id,200)||!value.id||!text(value.cursor,64))invalid();
 if(!whole(value.revision)||!whole(value.createdAt)||!whole(value.updatedAt)||!whole(value.expiresAt))invalid();
 if(!['open','in-progress','resolved','closed'].includes(value.status))invalid();
 if(!text(value.kind,60)||!text(value.category,60)||!text(value.message,4000))invalid();
 if(value.itemId!==undefined&&!text(value.itemId,200))invalid();
 if(value.duplicateOf!==undefined&&!text(value.duplicateOf,200))invalid();
 if(!whole(value.duplicates))invalid();
 const reporter=value.reporter;
 if(!obj(reporter)||!text(reporter.name,200)||!text(reporter.authority,40)||!text(reporter.role,40)||typeof reporter.self!=='boolean')invalid();
 const diagnostics=value.diagnostics;
 if(!obj(diagnostics)||!['attached','unavailable','declined','not-requested'].includes(diagnostics.decision))invalid();
 if(diagnostics.reference!==undefined&&!text(diagnostics.reference,240))invalid();
 if(diagnostics.detail!==undefined&&!text(diagnostics.detail,400))invalid();
 if(!Array.isArray(value.thread)||value.thread.length>200)invalid();
 for(const event of value.thread){
  if(!obj(event)||!whole(event.sequence)||!whole(event.at)||!whole(event.revision))invalid();
  if(!text(event.status,40)||!text(event.reply,4000)||!text(event.actorClass,40))invalid();
 }
 return value as FeedbackItem;
}

export function parseFeedbackSubmission(value:unknown):FeedbackSubmissionResult{
 if(!obj(value)||typeof value.duplicate!=='boolean'||typeof value.created!=='boolean')invalid();
 if(value.duplicateOf!==undefined&&!text(value.duplicateOf,200))invalid();
 return {...value,report:parseFeedbackReport(value.report)} as FeedbackSubmissionResult;
}

export function parseFeedbackList(value:unknown):FeedbackList{
 if(!obj(value)||!Array.isArray(value.items)||value.items.length>100)invalid();
 if(!text(value.nextCursor,64)||!whole(value.total)||!whole(value.observedAt))invalid();
 if(!obj(value.statusCounts)||!obj(value.filter))invalid();
 for(const key of Object.keys(value.statusCounts))if(!whole(value.statusCounts[key]))invalid();
 return {...value,items:value.items.map(parseFeedbackReport)} as FeedbackList;
}
