import {unreadableServerResponse} from './server-messages.ts';
/** Owner repairs are current-view CAS commands. A lost response is reconciled, never replayed. */
export type RepairTarget = Readonly<{kind:'item'|'show'|'season'|'album'|'artist'|'book';id:string;libraryId:string}>;
export type RepairScope = Readonly<{serverId:string;viewerId:string}>;
export type RepairField = Readonly<{value:string;automaticValue:string;locked:boolean;source:string;observedAt?:string;values?:readonly string[]}>;
/** The server publishes the editable field registry; a client never hard-codes one. */
export type RepairFieldSpec = Readonly<{field:string;label:string;group:'general'|'numbers'|'text'|'identity';type:'text'|'multiline'|'integer'|'number'|'date'|'enum'|'list';maxLength?:number;min?:number;max?:number;allowed?:readonly string[];bulk:boolean}>;
export type RepairRelationship = Readonly<{recordId?:string;provider?:string;department?:string;kind:string;targetKind:string;targetId:string;label:string;role?:string;ordinal:number;source:string;locked:boolean;observedAt?:string}>;
export type RepairChoice = Readonly<{role:string;subject:string;candidateId:string;digest:string;thumbnailDigest:string;locked:boolean;revision:number;attribution:string;url:string}>;
export type RepairData = Readonly<{
 target:{kind:RepairTarget['kind'];id:string};libraryId:string;serverId:string;viewerFence:string;revision:string;
 snapshot:{relationshipLocks:Readonly<Record<string,boolean>>;fields:Readonly<Record<string,RepairField>>;identity?:{engine?:'screen';type?:string;provider:string;id:string;order?:string;revision:number;status:string;locked:boolean};relationships:readonly RepairRelationship[];artwork:readonly RepairChoice[]};
 schema:readonly RepairFieldSpec[];artworkRoles:readonly string[];
 candidates:readonly {id:string;provider:string;title:string;subtitle:string;observedAt:string;year?:number;previewUrl?:string;orders?:readonly {id:string;name:string}[]}[];
 history:readonly {id:number;trigger:string;observedAt:string}[];
 artwork:{candidates:readonly {id:string;role:string;subject:string;provider:string;imageId:string;locale:string;rank:number;votes?:number|null;attribution:string;observedAt:string;current:boolean;previewUrl?:string}[];jobs:readonly {id:string;role:string;subject:string;status:string;error:string;attempts:number;nextAttempt:string}[]};
 cascades:readonly {id:string;intent:string;status:string;processed:number;failed:number;completed?:number;skipped?:number;pending?:number}[];
}>;
export type RepairCommand = Readonly<{
 action:'retry'|'preview_artwork'|'edit'|'identify'|'search'|'undo'|'lock_all'|'unlock_all'|'select_artwork'|'artwork_lock'|'repair_assets'|'discover_artwork'|'cascade'|'cancel_cascade'|'relationship_lock'|'edit_relationships';
 fields?:Record<string,{value?:string;values?:readonly string[];locked?:boolean;useAutomatic?:boolean}>;
 provider?:string;candidateId?:string;order?:string;historyId?:number;confirm?:boolean;
 role?:string;subject?:string;locked?:boolean;intent?:string;operationId?:string;query?:string;year?:number;
 relationships?:readonly RepairRelationship[];
}>;
export type RepairPreview = Readonly<{revision:string;descendants:number;lockedFields:number;selectedImages:number;message:string;merge?:{target:{kind:string;id:string};allowed:boolean;blockers:readonly string[];references:Readonly<Record<string,number>>}}>; 
export type RepairView = Readonly<{phase:'idle'|'loading'|'ready'|'saving'|'conflict'|'uncertain'|'denied'|'error';data:RepairData|null;error:string|null;preview:RepairPreview|null}>;
type Api = {request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>};
const obj=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const str=(v:unknown,max=2048):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const id=(v:unknown):v is string=>str(v,256)&&v.length>0&&!/[\r\n\t]/.test(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const num=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const arr=(v:unknown,max:number,check:(v:Record<string,unknown>)=>boolean):boolean=>Array.isArray(v)&&v.length<=max&&v.every(x=>obj(x)&&check(x));
const when=(v:unknown)=>str(v,64)&&(v===''||Number.isFinite(Date.parse(v)));
const roles=['poster','backdrop','cover','square','thumbnail','banner','logo','still','portrait'];
function bad():never{throw Object.assign(new Error(unreadableServerResponse),{code:'invalid_metadata_repair'});}
export function validateRepair(raw:unknown,scope:RepairScope,target:RepairTarget):RepairData {
 if(!obj(raw)||!obj(raw.target)||raw.target.kind!==target.kind||raw.target.id!==target.id||raw.libraryId!==target.libraryId||raw.serverId!==scope.serverId||!hash(raw.viewerFence)||!hash(raw.revision)||!obj(raw.snapshot)||!obj(raw.snapshot.fields)||Object.keys(raw.snapshot.fields).length>32)bad();
 // The registry decides which fields exist. A field the registry does not name
 // is not rendered, and a registry entry with no value is a broken projection.
 const groups=['general','numbers','text','identity'],types=['text','multiline','integer','number','date','enum','list'];
 if(!arr(raw.schema,32,f=>id(f.field)&&str(f.field,64)&&str(f.label,200)&&groups.includes(String(f.group))&&types.includes(String(f.type))&&typeof f.bulk==='boolean'&&(f.maxLength===undefined||num(f.maxLength))&&(f.min===undefined||num(f.min))&&(f.max===undefined||num(f.max))&&(f.allowed===undefined||Array.isArray(f.allowed)&&f.allowed.length<=200&&f.allowed.every(v=>str(v,200)))))bad();
 const specs=new Map((raw.schema as Record<string,unknown>[]).map(f=>[String(f.field),f]));
 if(specs.size!==(raw.schema as unknown[]).length)bad();
 if(!Array.isArray(raw.artworkRoles)||raw.artworkRoles.length>16||!raw.artworkRoles.every(r=>roles.includes(String(r))))bad();
 for(const [key,value] of Object.entries(raw.snapshot.fields)){
  const spec=specs.get(key);
  if(!spec||!obj(value)||!str(value.value,65536)||!str(value.automaticValue,65536)||typeof value.locked!=='boolean'||!str(value.source,100)||value.observedAt!==undefined&&!when(value.observedAt))bad();
  if(spec.type==='list'&&value.values!==undefined&&(!Array.isArray(value.values)||value.values.length>64||!value.values.every(v=>typeof v==='string'&&[...v].length<=128&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v))))bad();
  if(spec.type!=='list'&&value.values!==undefined)bad();
 }
 for(const field of specs.keys())if(!(field in (raw.snapshot.fields as Record<string,unknown>)))bad();
 if(!obj(raw.snapshot.relationshipLocks)||Object.keys(raw.snapshot.relationshipLocks).length>3||Object.entries(raw.snapshot.relationshipLocks).some(([k,v])=>!['credit','genre','identity'].includes(k)||typeof v!=='boolean'))bad();
 const ident=raw.snapshot.identity;
 if(ident!==undefined&&(!obj(ident)||!str(ident.provider,40)||!str(ident.id,256)||!num(ident.revision)||!str(ident.status,64)||typeof ident.locked!=='boolean'||ident.order!==undefined&&!str(ident.order,160)||ident.engine!==undefined&&ident.engine!=='screen'||ident.type!==undefined&&!str(ident.type,40)))bad();
 if(!arr(raw.snapshot.relationships,200,r=>(r.recordId===undefined||str(r.recordId,256))&&(r.provider===undefined||str(r.provider,40))&&(r.department===undefined||str(r.department,200))&&id(r.kind)&&id(r.targetKind)&&str(r.targetId,256)&&str(r.label,2048)&&str(r.source,100)&&typeof r.locked==='boolean'&&num(r.ordinal)&&(r.role===undefined||str(r.role,500))&&(r.observedAt===undefined||when(r.observedAt))))bad();
 const prefix='/v1/metadata/'+encodeURIComponent(target.kind)+'/'+encodeURIComponent(target.id)+'/art/';
 if(!arr(raw.snapshot.artwork,200,a=>roles.includes(String(a.role))&&str(a.subject,256)&&hash(a.candidateId)&&hash(a.digest)&&hash(a.thumbnailDigest)&&typeof a.locked==='boolean'&&num(a.revision)&&str(a.attribution,2048)&&str(a.url,2048)&&a.url.startsWith(prefix)&&!/[\r\n\\]/.test(a.url)))bad();
 if(!arr(raw.candidates,50,c=>id(c.id)&&str(c.provider,40)&&str(c.title,2048)&&str(c.subtitle,2048)&&when(c.observedAt)&&(c.year===undefined||num(c.year)&&c.year<=9999)&&(c.previewUrl===undefined||str(c.previewUrl,2048)&&c.previewUrl.startsWith(prefix)&&!/[\r\n\\]/.test(c.previewUrl))&&(c.orders===undefined||arr(c.orders,64,o=>id(o.id)&&str(o.id,160)&&str(o.name,2048))))||!arr(raw.history,30,h=>num(h.id)&&h.id>0&&str(h.trigger,100)&&when(h.observedAt)))bad();
 if(!obj(raw.artwork)||!arr(raw.artwork.candidates,200,a=>hash(a.id)&&roles.includes(String(a.role))&&str(a.subject,256)&&str(a.provider,40)&&str(a.imageId,256)&&str(a.locale,30)&&typeof a.rank==='number'&&Number.isFinite(a.rank)&&(a.votes===undefined||a.votes===null||num(a.votes))&&str(a.attribution,2048)&&when(a.observedAt)&&typeof a.current==='boolean'&&(a.previewUrl===undefined||str(a.previewUrl,2048)&&a.previewUrl.startsWith(prefix)&&!/[\r\n\\]/.test(a.previewUrl)))||!arr(raw.artwork.jobs,201,j=>id(j.id)&&str(j.role,40)&&str(j.subject,256)&&str(j.status,64)&&str(j.error,256)&&num(j.attempts)&&when(j.nextAttempt))||!arr(raw.cascades,10,c=>id(c.id)&&str(c.intent,40)&&str(c.status,40)&&num(c.processed)&&num(c.failed)&&['completed','skipped','pending'].every(k=>c[k]===undefined||num(c[k]))))bad();
 // Do not retain extra transport/audit fields in a viewer projection.
 const pick=(value:unknown,keys:readonly string[])=>Object.freeze(Object.fromEntries(keys.filter(k=>(value as Record<string,unknown>)[k]!==undefined).map(k=>[k,(value as Record<string,unknown>)[k]])));
 const rows=(value:unknown,keys:readonly string[])=>Object.freeze((value as unknown[]).map(v=>pick(v,keys)));
 return Object.freeze({target:Object.freeze({kind:target.kind,id:target.id}),libraryId:target.libraryId,serverId:scope.serverId,viewerFence:raw.viewerFence,revision:raw.revision,
 schema:rows(raw.schema,['field','label','group','type','maxLength','min','max','allowed','bulk']),artworkRoles:Object.freeze([...(raw.artworkRoles as string[])]),
 snapshot:Object.freeze({fields:Object.freeze(Object.fromEntries(Object.entries(raw.snapshot.fields).map(([k,v])=>[k,pick(v,['value','automaticValue','locked','source','observedAt','values'])]))),relationshipLocks:Object.freeze({...raw.snapshot.relationshipLocks}),...(ident===undefined?{}:{identity:pick(ident,['engine','type','provider','id','order','revision','status','locked'])}),relationships:rows(raw.snapshot.relationships,['recordId','provider','department','kind','targetKind','targetId','label','role','ordinal','source','locked','observedAt']),artwork:rows(raw.snapshot.artwork,['role','subject','candidateId','digest','thumbnailDigest','locked','revision','attribution','url'])}),
 candidates:Object.freeze((raw.candidates as Record<string,unknown>[]).map(c=>Object.freeze({...pick(c,['id','provider','title','subtitle','observedAt','year','previewUrl']),...(c.orders===undefined?{}:{orders:rows(c.orders,['id','name'])})}))),history:rows(raw.history,['id','trigger','observedAt']),
 artwork:Object.freeze({candidates:rows(raw.artwork.candidates,['id','role','subject','provider','imageId','locale','rank','votes','attribution','observedAt','current','previewUrl']),jobs:rows(raw.artwork.jobs,['id','role','subject','status','error','attempts','nextAttempt'])}),cascades:rows(raw.cascades,['id','intent','status','processed','failed','completed','skipped','pending'])}) as unknown as RepairData;
}
export class MetadataRepairService {
 private state:RepairView=Object.freeze({phase:'idle',data:null,error:null,preview:null});
 private listeners=new Set<()=>void>();private epoch=0;private controller?:AbortController;private fence?:string;private target?:RepairTarget;private disposed=false;
 private options:{api:Api;scope:RepairScope;timeoutMs?:number};
 constructor(options:{api:Api;scope:RepairScope;timeoutMs?:number}){if(!id(options.scope.serverId)||!options.scope.viewerId)throw Error('A concrete viewer scope is required.');if(options.timeoutMs!==undefined&&(!num(options.timeoutMs)||options.timeoutMs<1||options.timeoutMs>30000))throw Error('Invalid deadline.');this.options={...options,scope:Object.freeze({...options.scope})};}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(state:RepairView){if(this.disposed)return;this.state=Object.freeze(state);for(const fn of this.listeners)fn();}
 cancel(){this.epoch++;this.controller?.abort();this.target=undefined;this.fence=undefined;this.publish({phase:'idle',data:null,error:null,preview:null});}
 dispose(){this.cancel();this.disposed=true;this.listeners.clear();}
 private async request(target:RepairTarget,method:string,body?:unknown,suffix=''){
  const c=new AbortController();this.controller?.abort();this.controller=c;
  let rejectAbort!:(reason:Error)=>void;const cancelled=new Promise<never>((_,reject)=>{rejectAbort=reject;});
  const abort=()=>rejectAbort(new Error('Repair request interrupted.'));c.signal.addEventListener('abort',abort,{once:true});
  const timer=setTimeout(()=>c.abort(),this.options.timeoutMs??15000);
  try{return await Promise.race([this.options.api.request<unknown>('/v1/metadata/'+encodeURIComponent(target.kind)+'/'+encodeURIComponent(target.id)+suffix,method,body,c.signal),cancelled]);}
  finally{clearTimeout(timer);c.signal.removeEventListener('abort',abort);}
 }
 async load(target:RepairTarget){
  if(this.disposed)throw Error('Repair view is closed.');if(!id(target.id)||!id(target.libraryId)||!['item','show','season','album','artist','book'].includes(target.kind))throw Error('Select a concrete entity.');
  if(JSON.stringify(target)!==JSON.stringify(this.target))this.fence=undefined;
  this.target=Object.freeze({...target});const epoch=++this.epoch;this.publish({phase:'loading',data:null,error:null,preview:null});
  try{const raw=await this.request(target,'GET');if(epoch!==this.epoch||this.disposed)return;this.accept(raw,target);}
  catch(error){if(epoch===this.epoch&&!this.disposed)this.failure(error,false);}
 }
 private accept(raw:unknown,target:RepairTarget){const data=validateRepair(raw,this.options.scope,target);if(this.fence&&this.fence!==data.viewerFence)throw Object.assign(new Error('Owner access changed.'),{status:403});this.fence=data.viewerFence;this.publish({phase:'ready',data,error:null,preview:null});}
 async command(command:RepairCommand):Promise<boolean>{
  const data=this.state.data,target=this.target;if(this.state.phase!=='ready'||!data||!target)throw Error('Load current metadata before making another change.');
  const epoch=++this.epoch;this.publish({...this.state,phase:'saving',error:null});
  try{const raw=await this.request(target,'POST',{...command,expectedRevision:data.revision});if(epoch!==this.epoch||this.disposed)return false;this.accept(raw,target);return true;}
  catch(error){if(epoch===this.epoch&&!this.disposed)this.failure(error,true);return false;}
 }
 async preview(mergeId?:string):Promise<void>{
  const target=this.target,data=this.state.data;if(!target||!data||this.state.phase!=='ready')return;
  const epoch=++this.epoch;this.publish({...this.state,phase:'loading',error:null});
  try{const raw=await this.request(target,'GET',undefined,'/preview'+(mergeId?'?mergeId='+encodeURIComponent(mergeId):''));if(epoch!==this.epoch||this.disposed)return;
   if(!obj(raw)||raw.revision!==data.revision)throw Object.assign(new Error('Metadata changed.'),{status:409});
   if(!num(raw.descendants)||!num(raw.lockedFields)||!num(raw.selectedImages)||!str(raw.message,4096))bad();
   if(raw.merge!==undefined){const m=raw.merge;if(!obj(m)||!obj(m.target)||m.target.id!==mergeId||m.target.kind!==target.kind||typeof m.allowed!=='boolean'||!Array.isArray(m.blockers)||m.blockers.length>100||!m.blockers.every(s=>str(s,4096))||!obj(m.references)||Object.keys(m.references).length>1000||!Object.values(m.references).every(num))bad();}
   this.publish({phase:'ready',data,error:null,preview:raw as unknown as RepairPreview});
  }catch(error){if(epoch===this.epoch&&!this.disposed)this.failure(error,false);}
 }
 private failure(error:unknown,write:boolean){const e=obj(error)?error:{};const denied=e.status===401||e.status===403;const conflict=e.status===409||e.code==='metadata_conflict';
  this.publish({phase:denied?'denied':conflict?'conflict':write?'uncertain':'error',data:denied?null:this.state.data,preview:null,error:denied?'Owner access is no longer available. Close this view and sign in again.':conflict?'Metadata changed. Nothing was replayed. Reload the current values and review your decision.':write?'The result is uncertain. Reload saved values before making another change.':'Could not read metadata repair state. Reload to try again.'});
 }
}
