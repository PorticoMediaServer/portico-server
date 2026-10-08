/**
 * The server's own "needs attention" list. Ordering and severity are server
 * policy: a client renders the list as given and never re-ranks it, so every
 * platform shows the owner the same most-urgent item first.
 */
export type AttentionSeverity='critical'|'warning'|'info';
export type AttentionAction={kind:string;target:string};
export type AttentionItem={id:string;severity:AttentionSeverity;title:string;detail:string;action:AttentionAction};

const obj=(v:unknown):v is Record<string,any>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max=512):v is string=>typeof v==='string'&&v.length<=max;
function invalid():never{throw new Error('Invalid selected-server attention response.');}

export function parseAttentionItem(value:unknown):AttentionItem{
 if(!obj(value))invalid();
 if(!text(value.id,200)||!value.id||!['critical','warning','info'].includes(value.severity)||
  !text(value.title,120)||!value.title||!text(value.detail,512)||!value.detail)invalid();
 if(!obj(value.action)||!text(value.action.kind,60)||!value.action.kind||!text(value.action.target,200))invalid();
 return value as AttentionItem;
}

/** The server caps this list; a longer one is a response we do not trust. */
export function parseAttention(value:unknown):AttentionItem[]{
 if(!obj(value)||!Array.isArray(value.items)||value.items.length>50)invalid();
 return value.items.map(parseAttentionItem);
}
