import {RouteError,probeRoute,routeOrigin,privateRouteHost,sameServerPin,webRouteCrypto,type RouteCrypto,type RouteCandidate,type ServerPin,type VerifiedRoutes} from './route-identity.ts';
export type DiscoveryRoute = Readonly<{baseUrl:string;serverId:string;fingerprint:string;port:number;path:string;expiresAt:number}>;
export type RouteSnapshot = Readonly<{phase:'idle'|'probing'|'ready'|'offline'|'identity_mismatch';origin:string|null;revision:number;network:string;message:string;discovery:'unknown'|'available'|'unsupported'|'denied'|'empty'}>;
export type RouteMemory = Readonly<{current?:VerifiedRoutes;previous?:VerifiedRoutes;paired:readonly string[];lastGood?:string}>;
export interface RouteConnectionOptions {
 pin:ServerPin;
 logicalOrigin:string;
 memory?:RouteMemory;
 crypto?:RouteCrypto;
 fetcher?:typeof fetch;
 discover?:(signal:AbortSignal)=>Promise<readonly DiscoveryRoute[]>;
 fresh?:(signal:AbortSignal)=>Promise<VerifiedRoutes>;
 /** How long remembered routes may stay silent before `fresh` is asked. */
 freshAfterMs?:number;
 persist?:(memory:RouteMemory)=>Promise<void>;
 probe?:typeof probeRoute;
}
/** Normalize a Bonjour result without making it trusted. A conflicting group
 * contributes no route, including otherwise plausible records in that group. */
export function discoveredRoutes(records:readonly DiscoveryRoute[],pin:ServerPin,now=Date.now()):RouteCandidate[] {
 if(records.length>64)return [];
 const groups=new Map<string,DiscoveryRoute[]>();for(const record of records){const group=groups.get(record.fingerprint)??[];group.push(record);groups.set(record.fingerprint,group);}
 const group=groups.get(pin.fingerprint)??[];if(group.some(r=>r.serverId!==pin.serverId||r.path!==''&&r.path!=='/'||!Number.isInteger(r.port)||r.port<1||r.port>65535)||new Set(group.map(r=>r.port+'|'+r.path)).size>1)return [];
 const out:RouteCandidate[]=[];for(const r of group){if(!Number.isFinite(r.expiresAt)||r.expiresAt<=now||r.expiresAt>now+120000)continue;try{const origin=routeOrigin(r.baseUrl),u=new URL(origin);if(!privateRouteHost(u.hostname)||Number(u.port||(u.protocol==='https:'?443:80))!==r.port)continue;if(!out.some(c=>c.baseUrl===origin))out.push({baseUrl:origin,class:'lan',quality:'probe_required',generation:'1'});}catch{}}
 return out.slice(0,8);
}
function newer(a:VerifiedRoutes,b:VerifiedRoutes):boolean {
 if(a.document.policyRevision<b.document.policyRevision||Date.parse(a.document.issuedAt)<Date.parse(b.document.issuedAt))return false;
 for(const key of ['claimGeneration','credentialGeneration','generation'] as const){const x=BigInt(a.document[key]),y=BigInt(b.document[key]);if(x!==y)return x>y;}
 return Date.parse(a.document.issuedAt)>=Date.parse(b.document.issuedAt)&&a.document.policyRevision>=b.document.policyRevision;
}
/** Remembered routes are asked first, and the Portico Account service only if they fail
 * (at once) or are still silent after this long. A server that is merely slow to answer,
 * on a mobile link or waking from sleep, must not cost the account service a request. */
const FRESH_AFTER_MS=3000;
const failureMessage='Your server is unavailable. Your sign-in is saved, and Portico will keep trying to connect.';
export class RouteConnection {
 readonly pin:ServerPin;
 readonly logicalOrigin:string;
 private options:RouteConnectionOptions;
 private memory:RouteMemory;
 private state:RouteSnapshot;
 private listeners=new Set<()=>void>();
 private serial=0;
 private run?:{controller:AbortController;promise:Promise<string>};
 private disposed=false;
 private retry?:ReturnType<typeof setTimeout>;
 private attempts=0;
 private foreground=true;
 private lastForegroundRetry=0;
 private freshNotBefore=0;
 private recent=new Set<string>();
 private persistTail:Promise<void>=Promise.resolve();
 private rejected=new Map<string,{retryAt:number;attempts:number}>();
 constructor(options:RouteConnectionOptions){
  this.options=options;this.pin=options.pin;this.logicalOrigin=routeOrigin(options.logicalOrigin);
  this.memory=options.memory??{paired:[]};for(const doc of [this.memory.current,this.memory.previous])if(doc&&!sameServerPin(doc.document,this.pin))throw new RouteError('identity_mismatch','Remembered routes belong to a different server.');
  if(this.memory.paired.length>8)throw new RouteError('invalid_routes','Too many remembered addresses.');this.memory={...this.memory,paired:this.memory.paired.map(routeOrigin)};
  this.state=Object.freeze({phase:'idle',origin:null,revision:0,network:'launch',message:'Verifying the server connection.',discovery:options.discover?'unknown':'unsupported'});
 }
 getSnapshot=():RouteSnapshot=>this.state;
 getMemory=():RouteMemory=>this.memory;
 subscribe=(listener:()=>void):(()=>void)=>{this.listeners.add(listener);return()=>{this.listeners.delete(listener);};};
 private publish(value:Partial<RouteSnapshot>){if(this.disposed)return;this.state=Object.freeze({...this.state,...value,revision:this.state.revision+1});for(const listener of this.listeners)listener();}
 private save(){const memory=this.memory,persist=this.options.persist;this.persistTail=this.persistTail.catch(()=>{}).then(()=>persist?.(memory));void this.persistTail.catch(()=>{});}
 /** Persistence failure must prevent the initial credential publication, but
  * cannot revoke an already selected viewer during later location changes. */
 flush():Promise<void>{return this.persistTail;}
 setPersistence(persist:(memory:RouteMemory)=>Promise<void>){this.options.persist=persist;}
 setFreshSource(fresh:(signal:AbortSignal)=>Promise<VerifiedRoutes>){this.options.fresh=fresh;}
 rememberPaired(origin:string){const value=routeOrigin(origin);this.memory={...this.memory,paired:[value,...this.memory.paired.filter(v=>v!==value)].slice(0,8)};this.save();}
 install(document:VerifiedRoutes){
  if(!sameServerPin(document.document,this.pin)||this.memory.current&&!newer(document,this.memory.current))throw new RouteError('identity_mismatch','Connection evidence moved backwards or changed identity.');
  const old=this.memory.current,allowed=new Set(document.document.candidates.map(c=>c.baseUrl));
  // Hosted includes its one eligible previous-verified set in the new signed
  // response. An omitted old candidate must not be resurrected by local cache.
  const previous=old&&old.document.keyId===document.document.keyId&&old.document.candidates.some(c=>allowed.has(c.baseUrl))?old:undefined;
  this.memory={...this.memory,current:document,previous,lastGood:this.memory.lastGood&&allowed.has(this.memory.lastGood)?this.memory.lastGood:undefined};
  if(!old||old.document.generation!==document.document.generation||old.document.credentialGeneration!==document.document.credentialGeneration||old.document.claimGeneration!==document.document.claimGeneration)this.rejected.clear();this.save();
 }
 setForeground(value:boolean){
  if(this.disposed)return;this.foreground=value;
  if(value)this.foregroundActivity();
  else if(this.retry){clearTimeout(this.retry);this.scheduleRecovery();}
 }
 foregroundActivity(){
  if(this.disposed||!this.foreground||this.state.phase==='ready'||this.run||Date.now()-this.lastForegroundRetry<5000)return;
  this.lastForegroundRetry=Date.now();this.attempts=0;if(this.retry)clearTimeout(this.retry);this.retry=undefined;void this.resolve().catch(()=>{});
 }
 private scheduleRecovery(){
  if(this.retry)clearTimeout(this.retry);
  const delay=Math.min(this.foreground?300000:900000,5000*2**Math.min(8,this.attempts++))*(.9+Math.random()*.2);
  this.retry=setTimeout(()=>{this.retry=undefined;void this.resolve().catch(()=>{});},delay);
 }
 networkChanged(network:string){if(this.disposed||network===this.state.network)return;this.rejected.clear();this.invalidate(network);void this.resolve().catch(()=>{});}
 private invalidate(network=this.state.network){this.serial++;this.run?.controller.abort();this.run=undefined;if(this.retry)clearTimeout(this.retry);this.retry=undefined;this.publish({phase:'probing',origin:null,network,message:'The network changed. Verifying the same server before reconnecting.'});}
 recover():Promise<string>{if(this.run)return this.run.promise;this.invalidate();return this.resolve();}
 async resolve(signal?:AbortSignal):Promise<string>{
  if(this.disposed)throw new RouteError('cancelled','This server connection is closed.');if(signal?.aborted)throw new RouteError('cancelled','Connection request cancelled.');
  if(this.state.phase==='ready'&&this.state.origin)return this.state.origin;
  if(!this.run){const generation=++this.serial,controller=new AbortController();this.publish({phase:'probing',origin:null,message:'Finding a verified connection to your server.'});
   const promise=this.race(controller.signal).then(async origin=>{if(this.disposed||generation!==this.serial)throw new RouteError('cancelled','A newer connection check replaced this one.');this.recent.add(origin);while(this.recent.size>8)this.recent.delete(this.recent.values().next().value!);this.memory={...this.memory,lastGood:origin};this.save();this.attempts=0;this.publish({phase:'ready',origin,message:''});return origin;}).catch(error=>{if(!this.disposed&&generation===this.serial){const mismatch=error instanceof RouteError&&error.code==='identity_mismatch';this.publish({phase:mismatch?'identity_mismatch':'offline',origin:null,message:mismatch?'An address answered with a different server identity. No credentials were sent.':failureMessage});this.scheduleRecovery();}throw error;}).finally(()=>{controller.abort();if(this.run?.controller===controller)this.run=undefined;});this.run={controller,promise};
  }
  if(!signal)return this.run.promise;
  return new Promise<string>((resolve,reject)=>{const abort=()=>reject(new RouteError('cancelled','Connection request cancelled.'));signal.addEventListener('abort',abort,{once:true});void this.run!.promise.then(resolve,reject).finally(()=>signal.removeEventListener('abort',abort));if(signal.aborted)abort();});
 }
 private race(signal:AbortSignal):Promise<string>{
  const now=Date.now(),queue:RouteCandidate[]=[],seen=new Set<string>(),controllers=new Set<AbortController>();let active=0,activeLAN=0,finished=false,freshStarted=false,freshDone=!this.options.fresh||Date.now()<this.freshNotBefore,discoveryDone=!this.options.discover,mismatch=false;
  let freshAllowed:Set<string>|undefined;const independent=new Set(this.memory.paired),inflight=new Map<AbortController,string>();
  const eligible=(baseUrl:string)=>!freshAllowed||freshAllowed.has(baseUrl)||independent.has(baseUrl);
  const current=this.memory.current,eligiblePrevious=this.memory.previous?.document.candidates.filter(c=>!current||current.document.candidates.some(n=>n.baseUrl===c.baseUrl))??[];
  const initial=[...(current?.document.candidates??[]),...eligiblePrevious,...this.memory.paired.map(baseUrl=>({baseUrl,class:'lan' as const,quality:'probe_required' as const,generation:'1'}))];
  initial.sort((a,b)=>Number(b.baseUrl===this.memory.lastGood)-Number(a.baseUrl===this.memory.lastGood)||Number(a.class!=='lan')-Number(b.class!=='lan')||Number(a.quality!=='reachable')-Number(b.quality!=='reachable'));
  return new Promise<string>((resolve,reject)=>{
   let boundary:ReturnType<typeof setTimeout>|undefined,publicTimer:ReturnType<typeof setTimeout>|undefined;const deadline=setTimeout(()=>finish(undefined,new RouteError(mismatch?'identity_mismatch':'route_unavailable',failureMessage)),8500);
   const abort=()=>finish(undefined,new RouteError('cancelled','Connection check cancelled.'));
   const finish=(origin?:string,error?:Error)=>{if(finished)return;finished=true;clearTimeout(deadline);if(boundary)clearTimeout(boundary);if(publicTimer)clearTimeout(publicTimer);signal.removeEventListener('abort',abort);for(const c of controllers)c.abort();origin?resolve(origin):reject(error??new RouteError('route_unavailable',failureMessage));};
   const add=(values:readonly RouteCandidate[])=>{for(const c of values){if(seen.size>=32)break;const key=c.baseUrl+'|'+c.generation;if(seen.has(c.baseUrl))continue;const rejected=this.rejected.get(key);if(rejected&&rejected.retryAt>Date.now()){mismatch=true;continue;}try{routeOrigin(c.baseUrl);}catch{continue;}seen.add(c.baseUrl);queue.push(c);}pump();};
   const fresh=()=>{if(finished||freshStarted||!this.options.fresh||Date.now()<this.freshNotBefore)return;freshStarted=true;this.freshNotBefore=Date.now()+300000;const c=new AbortController();controllers.add(c);void this.options.fresh(c.signal).then(doc=>{if(finished||signal.aborted)return;this.install(doc);freshAllowed=new Set(doc.document.candidates.map(v=>v.baseUrl));for(let i=queue.length-1;i>=0;i--)if(!eligible(queue[i].baseUrl))queue.splice(i,1);for(const [probe,url] of inflight)if(!eligible(url))probe.abort();add(doc.document.candidates);}).catch(error=>{if(finished||signal.aborted)return;const retry=(error as {retryAfterSeconds?:unknown})?.retryAfterSeconds;if(typeof retry==='number'&&Number.isFinite(retry)&&retry>0)this.freshNotBefore=Math.max(this.freshNotBefore,Date.now()+Math.min(86400,retry)*1000);}).finally(()=>{controllers.delete(c);freshDone=true;pump();});};
   const pump=()=>{
    if(finished||signal.aborted)return;
    while(active<3&&queue.length){
     const index=queue.findIndex(c=>c.class==='lan'?activeLAN<2:Date.now()-now>=120||!initial.some(v=>v.class==='lan'));
     if(index<0){if(!publicTimer&&queue.some(c=>c.class!=='lan')&&Date.now()-now<120)publicTimer=setTimeout(()=>{publicTimer=undefined;pump();},Math.max(1,120-(Date.now()-now)));break;}
     const candidate=queue.splice(index,1)[0],controller=new AbortController();controllers.add(controller);inflight.set(controller,candidate.baseUrl);active++;if(candidate.class==='lan')activeLAN++;
     void (this.options.probe??probeRoute)(candidate.baseUrl,this.pin,{fetcher:this.options.fetcher,crypto:this.options.crypto??webRouteCrypto,signal:controller.signal}).then(()=>{if(!signal.aborted&&!controller.signal.aborted&&eligible(candidate.baseUrl)){this.rejected.delete(candidate.baseUrl+'|'+candidate.generation);finish(candidate.baseUrl);}}).catch(error=>{if(!finished&&!signal.aborted&&!controller.signal.aborted&&error instanceof RouteError&&error.code==='identity_mismatch'){mismatch=true;const key=candidate.baseUrl+'|'+candidate.generation,attempts=Math.min(7,(this.rejected.get(key)?.attempts??0)+1);this.rejected.set(key,{attempts,retryAt:Date.now()+Math.min(300000,5000*2**(attempts-1))});while(this.rejected.size>64)this.rejected.delete(this.rejected.keys().next().value!);}}).finally(()=>{active--;if(candidate.class==='lan')activeLAN--;controllers.delete(controller);inflight.delete(controller);pump();});
    }
    if(!queue.length&&!active){if(!freshStarted)fresh();if(freshDone&&discoveryDone)finish(undefined,new RouteError(mismatch?'identity_mismatch':'route_unavailable',failureMessage));}
   };
   signal.addEventListener('abort',abort,{once:true});if(signal.aborted){abort();return;}
   if(this.options.discover){const c=new AbortController();controllers.add(c);void this.options.discover(c.signal).then(records=>{if(finished)return;const candidates=discoveredRoutes(records,this.pin);for(const candidate of candidates)independent.add(candidate.baseUrl);this.publish({discovery:candidates.length?'available':'empty'});add(candidates);}).catch(e=>{if(!finished)this.publish({discovery:(e as {code?:string})?.code==='permission_denied'?'denied':'empty'});}).finally(()=>{controllers.delete(c);discoveryDone=true;pump();});}
   boundary=setTimeout(fresh,this.options.freshAfterMs??FRESH_AFTER_MS);add(initial);
  });
 }
 /** Logical origin is stable for all existing command/journal ownership. Only
  * transport changes. Mutations are NEVER automatically replayed after loss. */
 fetch:typeof fetch=async(input,init={})=>{
  if(typeof input!=='string')throw new RouteError('invalid_address','Only selected-server URLs may be routed.');
  const requested=new URL(input);if(requested.username||requested.password||requested.hash||!this.acceptsOrigin(requested.origin))throw new RouteError('invalid_address','Cross-server request rejected.');
  const method=(init.method??'GET').toUpperCase(),read=method==='GET'||method==='HEAD';
  let sentGeneration:number|undefined;
  const perform=async()=>{
   let origin=await this.resolve(init.signal??undefined);
   // A network event can run while resolve's fulfilled promise is queued.
   // Never dispatch credentials using an origin that is no longer verified.
   if(this.state.phase!=='ready'||this.state.origin!==origin)origin=await this.resolve(init.signal??undefined);
   if(init.signal?.aborted||this.disposed||this.state.phase!=='ready'||this.state.origin!==origin)throw new RouteError('cancelled','Request cancelled or superseded.');
   sentGeneration=this.serial;
   return (this.options.fetcher??globalThis.fetch)(origin+requested.pathname+requested.search,{...init,redirect:'error',credentials:'omit',referrerPolicy:'no-referrer'});
  };
  // A gateway answering for the server (a 502/503/504 that isn't Portico's own JSON: the
  // server is down or restarting behind its proxy) is a lost route, not an answer. Probe
  // again so the shell can show one reconnecting state; reads retry once after recovery.
  try{const response=await perform();if(!gatewayFailure(response)||init.signal?.aborted||this.disposed||sentGeneration===undefined)return response;const recovery=sentGeneration===this.serial?this.recover():this.resolve(init.signal??undefined);if(read){await recovery;return perform();}void recovery.catch(()=>{});return response;}catch(error){
   if(init.signal?.aborted||this.disposed||sentGeneration===undefined)throw error;
   // An old socket's delayed failure must not cancel the replacement race or
   // invalidate a newer proven transport. Read retry only; no mutation replay.
   const recovery=sentGeneration===this.serial?this.recover():this.resolve(init.signal??undefined);
   if(read){await recovery;return perform();}void recovery.catch(()=>{});throw error;
  }
 };
 acceptsOrigin(origin:string):boolean{return origin===this.logicalOrigin||this.recent.has(origin)||origin===this.state.origin;}
 mediaURL(path:string):string{const url=new URL(path,this.logicalOrigin);if(!this.acceptsOrigin(url.origin)||url.username||url.password)throw new RouteError('invalid_address','Cross-server media capability rejected.');if(this.state.phase!=='ready'||!this.state.origin)return '';return this.state.origin+url.pathname+url.search+url.hash;}
 dispose(){this.disposed=true;this.serial++;this.run?.controller.abort();if(this.retry)clearTimeout(this.retry);this.listeners.clear();}
}
/** A proxy's error page for an unavailable upstream; Portico's own 503s are JSON. */
export function gatewayFailure(response:Response):boolean{return (response.status===502||response.status===503||response.status===504)&&!(response.headers.get('content-type')??'').toLowerCase().includes('json');}
// A bearer is bound in memory only after identity proof + durable credential
// storage. New APIs for the same selected family inherit that exact connection.
const bindings=new Map<string,RouteConnection>();
export function bindSessionRoute(token:string,connection:RouteConnection){if(!token)throw new Error('A native credential is required.');bindings.delete(token);bindings.set(token,connection);while(bindings.size>16)bindings.delete(bindings.keys().next().value!);}
export function sessionRoute(token:string):RouteConnection|undefined{return token?bindings.get(token):undefined;}
export function forgetSessionRoute(token:string){bindings.delete(token);}
