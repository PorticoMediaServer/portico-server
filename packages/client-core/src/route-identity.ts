/** P09 connection evidence. None of these values grants a viewer or media lease. */
export type ServerPin = Readonly<{serverId:string;publicKey:string;fingerprint:string}>;
export type SignedRoutes = Readonly<{payload:string;signature:string}>;
export type RouteKey = Readonly<{keyId:string;publicKey:string}>;
export type RouteCandidate = Readonly<{baseUrl:string;class:'lan'|'public'|'manual';quality:'reachable'|'probe_required';generation:string}>;
export type RouteDocument = Readonly<ServerPin & {kind:'portico.routes';version:'1';audience:string;name:string;policyRevision:number;claimGeneration:string;credentialGeneration:string;generation:string;candidates:readonly RouteCandidate[];issuedAt:string;expiresAt:string;algorithm:'Ed25519';keyId:string}>;
export type VerifiedRoutes = Readonly<{envelope:SignedRoutes;document:RouteDocument}>;
export interface RouteCrypto {
 random(length:number):Uint8Array;
 sha256(value:Uint8Array):Promise<Uint8Array>;
 verify(publicKey:Uint8Array,signature:Uint8Array,payload:Uint8Array):Promise<boolean>;
}
export class RouteError extends Error {
 readonly code:string;
 constructor(code:string,message:string){super(message);this.name='RouteError';this.code=code;}
}
const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';
export function routeBase64(bytes:Uint8Array):string {
 let out='',buffer=0,bits=0;for(const byte of bytes){buffer=(buffer<<8)|byte;bits+=8;while(bits>=6){bits-=6;out+=alphabet[(buffer>>>bits)&63];}}if(bits)out+=alphabet[(buffer<<(6-bits))&63];return out;
}
export function routeBytes(value:unknown,length?:number,max=65536):Uint8Array {
 if(typeof value!=='string'||!value.length||value.length>max*4/3+4||!/^[A-Za-z0-9_-]+$/.test(value)||value.length%4===1)throw new RouteError('invalid_identity','Invalid identity encoding.');
 const out=new Uint8Array(Math.floor(value.length*6/8));let bits=0,buffer=0,index=0;
 for(const c of value){buffer=(buffer<<6)|alphabet.indexOf(c);bits+=6;if(bits>=8){bits-=8;out[index++]=(buffer>>>bits)&255;}}
 if((length!==undefined&&out.length!==length)||out.length>max||routeBase64(out)!==value)throw new RouteError('invalid_identity','Invalid identity encoding.');return out;
}
/** Route crypto for a page without `crypto.subtle` (plain HTTP on the LAN is not a secure
 * context). The web app registers an audited JavaScript SHA-256 and Ed25519 here at startup;
 * the checks themselves are the same, only the implementation differs. */
let routeCryptoFallback:Pick<RouteCrypto,'sha256'|'verify'>|undefined;
export function setRouteCryptoFallback(fallback:Pick<RouteCrypto,'sha256'|'verify'>):void{routeCryptoFallback=fallback;}
const subtle=()=>globalThis.crypto?.subtle;
const noRouteCrypto=()=>new RouteError('insecure_context','This browser can\'t verify the server here. Open Portico over HTTPS or update the browser.');
export const webRouteCrypto:RouteCrypto={
 random:n=>globalThis.crypto.getRandomValues(new Uint8Array(n)),
 sha256:async b=>{if(subtle())return new Uint8Array(await subtle()!.digest('SHA-256',new Uint8Array(b)));if(routeCryptoFallback)return routeCryptoFallback.sha256(b);throw noRouteCrypto();},
 verify:async(key,sig,raw)=>{if(!subtle()){if(routeCryptoFallback)return routeCryptoFallback.verify(key,sig,raw);throw noRouteCrypto();}try{const k=await globalThis.crypto.subtle.importKey('raw',new Uint8Array(key),'Ed25519',false,['verify']);return await globalThis.crypto.subtle.verify('Ed25519',k,new Uint8Array(sig),new Uint8Array(raw));}catch{return false;}},
};
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const text=(v:unknown,max:number):v is string=>typeof v==='string'&&v.length>0&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const generation=(v:unknown):v is string=>typeof v==='string'&&/^[1-9][0-9]{0,18}$/.test(v)&&BigInt(v)<=9223372036854775807n;
export function privateRouteHost(host:string):boolean {
 const h=host.replace(/^\[|\]$/g,'').toLowerCase();if(h==='localhost'||h==='::1'||h.endsWith('.local'))return true;
 if(h.includes(':'))return !h.includes('.')&&!h.includes('%')&&(/^(fc|fd)[0-9a-f]{2}:/.test(h)||/^fe[89ab][0-9a-f]:/.test(h));
 if(!/^(?:0|[1-9]\d{0,2})(?:\.(?:0|[1-9]\d{0,2})){3}$/.test(h))return false;
 const p=h.split('.').map(Number);return p.every(n=>n<=255)&&(p[0]===10||p[0]===127||p[0]===192&&p[1]===168||p[0]===172&&p[1]>=16&&p[1]<=31||p[0]===169&&p[1]===254);
}
/** Origin only. No userinfo, path trick, query, IDNA ambiguity, or public HTTP. */
export function routeOrigin(raw:string):string {
 if(typeof raw!=='string'||raw.length>300||/[\\\s\x00-\x1f\x7f]/.test(raw)||!/^https?:\/\//i.test(raw))throw new RouteError('invalid_address','Enter an HTTPS server address or a private LAN address.');
 const u=new URL(raw);if(u.username||u.password||u.search||u.hash||u.pathname!=='/'||!u.hostname||u.hostname.endsWith('.')||u.hostname.includes('%')||!/^https?:$/.test(u.protocol)||(u.protocol==='http:'&&!privateRouteHost(u.hostname)))throw new RouteError('invalid_address','Public server addresses require HTTPS.');
 // URL accepts legacy decimal/octal/hex IPv4. Do not silently reinterpret it.
 const authority=raw.slice(raw.indexOf('://')+3).replace(/\/$/,'');const host=authority.startsWith('[')?authority.slice(0,authority.indexOf(']')+1):authority.split(':')[0];
 if(host.toLowerCase()!==u.hostname)throw new RouteError('invalid_address','Use a canonical server address.');return u.origin;
}
export async function serverPin(serverId:string,publicKey:string,fingerprint:string|undefined,crypto:RouteCrypto=webRouteCrypto):Promise<ServerPin> {
 const raw=routeBytes(publicKey,32);const digest=routeBase64(await crypto.sha256(raw));
 const context=new TextEncoder().encode('portico.server.identity.v1\0');const bytes=new Uint8Array(context.length+raw.length);bytes.set(context);bytes.set(raw,context.length);
 if(serverId!=='srv_'+routeBase64(await crypto.sha256(bytes))||(fingerprint!==undefined&&fingerprint!==digest))throw new RouteError('identity_mismatch','This address does not match the selected server identity.');
 return Object.freeze({serverId,publicKey,fingerprint:digest});
}
export function sameServerPin(a:ServerPin,b:ServerPin):boolean{return a.serverId===b.serverId&&a.publicKey===b.publicKey&&a.fingerprint===b.fingerprint;}
function envelope(value:unknown,keyId?:string):SignedRoutes {
 // Hosted may attach the signing key's certificate from its offline root (784d3de5); the key itself
 // is checked against the account service's current key, so the certificate is accepted and not kept.
 if(!object(value)||Object.keys(value).some(k=>!['payload','signature',...(keyId===undefined?[]:['keyId','certificate'])].includes(k))||(value.certificate!==undefined&&!object(value.certificate))||(value.keyId!==undefined&&value.keyId!==keyId)||typeof value.payload!=='string'||typeof value.signature!=='string')throw new RouteError('invalid_identity','Invalid signed connection evidence.');
 routeBytes(value.payload,undefined,32768);routeBytes(value.signature,64);return Object.freeze({payload:value.payload,signature:value.signature});
}
function parseSignedJSON(value:SignedRoutes):Record<string,unknown>{try{const v:unknown=JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(routeBytes(value.payload,undefined,32768)));if(object(v))return v;}catch{}throw new RouteError('invalid_identity','Invalid signed connection evidence.');}
function times(v:Record<string,unknown>,maxAge:number,now:number,allowExpired:boolean):void {
 const issued=typeof v.issuedAt==='string'?Date.parse(v.issuedAt):NaN,expires=typeof v.expiresAt==='string'?Date.parse(v.expiresAt):NaN;
 if(!Number.isFinite(issued)||!Number.isFinite(expires)||issued>now+120000||expires<=issued||expires-issued>maxAge||(!allowExpired&&expires<=now))throw new RouteError('route_stale','The connection evidence is expired or has an invalid clock.');
}
export async function verifyRoutes(value:unknown,key:RouteKey,expected:{accountId:string;serverId:string;pin?:ServerPin;allowExpiredHint?:boolean;now?:number},crypto:RouteCrypto=webRouteCrypto):Promise<VerifiedRoutes> {
 // A rotated transport key ID must take the same bounded fresh-key path as a payload/signature mismatch.
 if(object(value)&&typeof value.keyId==='string'&&value.keyId!==key.keyId)throw new RouteError('invalid_signature','The account service signing key changed.');
 const signed=envelope(value,key.keyId),v=parseSignedJSON(signed);
 if(!text(key.keyId,256)||v.keyId!==key.keyId||!await crypto.verify(routeBytes(key.publicKey,32),routeBytes(signed.signature,64),routeBytes(signed.payload)))throw new RouteError('invalid_signature','The account service did not sign these server routes.');
 if(v.kind!=='portico.routes'||v.version!=='1'||v.algorithm!=='Ed25519'||v.audience!==expected.accountId||v.serverId!==expected.serverId||!text(v.name,2048)||typeof v.publicKey!=='string'||typeof v.fingerprint!=='string'||!generation(v.generation)||!generation(v.claimGeneration)||!generation(v.credentialGeneration)||!Number.isSafeInteger(v.policyRevision)||Number(v.policyRevision)<0||!Array.isArray(v.candidates)||v.candidates.length>24)throw new RouteError('invalid_routes','The account service returned incompatible connection information.');
 const pin=await serverPin(v.serverId as string,v.publicKey,v.fingerprint,crypto);if(expected.pin&&!sameServerPin(pin,expected.pin))throw new RouteError('identity_mismatch','The selected server identity changed.');
 times(v,45*60000,expected.now??Date.now(),expected.allowExpiredHint===true);
 const candidates=v.candidates.map(c=>{if(!object(c)||!['lan','public','manual'].includes(String(c.class))||!['reachable','probe_required'].includes(String(c.quality))||!generation(c.generation)||BigInt(c.generation)>BigInt(v.generation as string)||typeof c.baseUrl!=='string'||routeOrigin(c.baseUrl)!==c.baseUrl||(c.class!=='lan'&&!c.baseUrl.startsWith('https://'))||(c.class==='lan'&&!privateRouteHost(new URL(c.baseUrl).hostname)))throw new RouteError('invalid_routes','Invalid signed route candidate.');return Object.freeze({baseUrl:c.baseUrl,class:c.class as RouteCandidate['class'],quality:c.quality as RouteCandidate['quality'],generation:c.generation});});
 if(new Set(candidates.map(c=>c.baseUrl)).size!==candidates.length)throw new RouteError('invalid_routes','Duplicate signed route candidates.');
 return Object.freeze({envelope:signed,document:Object.freeze({...v,...pin,candidates:Object.freeze(candidates)}) as RouteDocument});
}
/** Bounded JSON reader with neutral, non-sensitive diagnostics (also works with RN Response). */
export async function routeJSON(response:Response,signal:AbortSignal,max=32768):Promise<unknown> {
 if(!response.ok){void response.body?.cancel().catch(()=>{});const e=new RouteError(response.status===401?'authentication_required':response.status===403?'access_denied':response.status===429?'rate_limited':'route_unavailable','The connection could not be verified.');throw e;}
 if(response.headers.get('Content-Type')?.split(';')[0].toLowerCase().trim()!=='application/json'||Number(response.headers.get('Content-Length')??0)>max)throw new RouteError('invalid_response','Invalid connection response.');
 // Native's bounded NSURLSession transport has already limited bytes. Fetch
 // streams are consumed incrementally so a hostile endpoint cannot allocate arbitrarily.
 if(!response.body?.getReader){const s=await response.text();if(signal.aborted||new TextEncoder().encode(s).length>max)throw new RouteError('invalid_response','Invalid connection response.');return JSON.parse(s);}
 const reader=response.body.getReader(),chunks:Uint8Array[]=[];let length=0;const abort=()=>{void reader.cancel().catch(()=>{});};signal.addEventListener('abort',abort,{once:true});
 try{for(;;){if(signal.aborted)throw new RouteError('cancelled','Connection check cancelled.');const next=await reader.read();if(next.done)break;length+=next.value.byteLength;if(length>max)throw new RouteError('invalid_response','Connection response exceeded its limit.');chunks.push(next.value);}if(signal.aborted)throw new RouteError('cancelled','Connection check cancelled.');const bytes=new Uint8Array(length);let n=0;for(const b of chunks){bytes.set(b,n);n+=b.length;}return JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(bytes));}finally{signal.removeEventListener('abort',abort);void reader.cancel().catch(()=>{});try{reader.releaseLock();}catch{}}
}
export async function probeRoute(origin:string,pin:ServerPin|undefined,options:{fetcher?:typeof fetch;crypto?:RouteCrypto;signal?:AbortSignal;now?:()=>number}={}):Promise<ServerPin> {
 const baseUrl=routeOrigin(origin),crypto=options.crypto??webRouteCrypto,nonce=routeBase64(crypto.random(32));
 const controller=new AbortController(),cancel=()=>controller.abort();options.signal?.addEventListener('abort',cancel,{once:true});const timer=setTimeout(cancel,3000);
 try{
  if(options.signal?.aborted)cancel();if(controller.signal.aborted)throw new RouteError('cancelled','Connection check cancelled.');
  const response=await (options.fetcher??globalThis.fetch)(baseUrl+'/v1/networking/identity-proof',{method:'POST',redirect:'error',credentials:'omit',referrerPolicy:'no-referrer',signal:controller.signal,headers:{Accept:'application/json','Content-Type':'application/json'},body:JSON.stringify({baseUrl,nonce})});
  const signed=envelope(await routeJSON(response,controller.signal,8192)),v=parseSignedJSON(signed);
  if(v.kind!=='portico.route-proof'||v.version!=='1'||v.baseUrl!==baseUrl||v.nonce!==nonce||typeof v.serverId!=='string'||typeof v.publicKey!=='string'||typeof v.fingerprint!=='string')throw new RouteError('identity_mismatch','The server did not prove this connection.');
  const actual=await serverPin(v.serverId,v.publicKey,v.fingerprint,crypto);if(pin&&!sameServerPin(actual,pin))throw new RouteError('identity_mismatch','This address belongs to a different server. No credentials were sent.');
  times(v,30000,options.now?.()??Date.now(),false);
  if(!await crypto.verify(routeBytes(actual.publicKey,32),routeBytes(signed.signature,64),routeBytes(signed.payload)))throw new RouteError('identity_mismatch','The server could not prove its identity. No credentials were sent.');
  if(controller.signal.aborted)throw new RouteError('cancelled','Connection check cancelled.');return actual;
 }finally{clearTimeout(timer);options.signal?.removeEventListener('abort',cancel);controller.abort();}
}
