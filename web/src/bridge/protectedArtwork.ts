import type {HttpLocalApi} from '@core/index.ts';
// Matches the server normalized-original ceiling; thumbnail reads are smaller.
const maximumArtworkBytes=32*1024*1024;

/** Why an artwork read failed, in the terms the store needs to decide whether to try again. */
export class ArtworkReadError extends Error{
 readonly status:number;
 /** Seconds the server asked us to wait (`Retry-After`), when it said. */
 readonly retryAfter?:number;
 /** A slow or busy server, or artwork the server is still preparing: worth asking again later. */
 readonly retryable:boolean;
 constructor(message:string,status:number,retryable:boolean,retryAfter?:number){super(message);this.name='ArtworkReadError';this.status=status;this.retryable=retryable;this.retryAfter=retryAfter;}
}

/** `Retry-After` as seconds: either delta-seconds or an HTTP date. */
export function retryAfterSeconds(value:string|null,now=Date.now()):number|undefined{
 if(!value)return undefined;
 const trimmed=value.trim();
 if(/^\d+$/.test(trimmed))return Math.min(Number(trimmed),3600);
 const when=Date.parse(trimmed);
 return Number.isFinite(when)?Math.max(0,Math.min(3600,Math.round((when-now)/1000))):undefined;
}

/** A bounded authenticated image read; redirects may not carry credentials elsewhere. */
/** `timeoutMs` 0 = no own timeout (the caller, e.g. the artwork scheduler, owns it). */
export async function protectedArtwork(api:HttpLocalApi,path:string,token:string,signal:AbortSignal,timeoutMs=15000):Promise<Blob>{
 const url=new URL(path,api.baseUrl);
 if(url.origin!==new URL(api.baseUrl).origin)throw new ArtworkReadError('Cross-origin artwork is not allowed.',0,false);
 let response:Response;
 try{
  response=await api.routeFetch(url.href,{headers:{Authorization:`Bearer ${token}`},redirect:'error',signal:timeoutMs>0?AbortSignal.any([signal,AbortSignal.timeout(timeoutMs)]):signal});
 }catch(e){
  // The caller's own abort is final; a timeout or a dropped connection is a slow server.
  if(signal.aborted)throw e;
  throw new ArtworkReadError('Artwork did not arrive in time.',0,true);
 }
 if(!response.ok||!response.body){
  void response.body?.cancel().catch(()=>{});
  const retryAfter=retryAfterSeconds(response.headers.get('Retry-After'));
  // 404 with Retry-After is the server saying "queued, not ready yet"; 429/5xx are load.
  const retryable=response.status===429||response.status>=500||(response.status===404&&retryAfter!==undefined);
  throw new ArtworkReadError('Artwork unavailable.',response.status,retryable,retryAfter);
 }
 const type=response.headers.get('content-type')?.split(';')[0];
 if(type!=='image/jpeg'&&type!=='image/png'&&type!=='image/webp'){void response.body.cancel().catch(()=>{});throw new ArtworkReadError('Unsupported artwork.',response.status,false);}
 const reader=response.body.getReader();const chunks:Uint8Array<ArrayBuffer>[]=[];let size=0;let complete=false;
 try{
  for(;;){const {done,value}=await reader.read();if(done){complete=true;break;}size+=value.byteLength;if(size>maximumArtworkBytes)throw new ArtworkReadError('Artwork is too large.',response.status,false);chunks.push(value);}
  return new Blob(chunks,{type});
 }finally{if(!complete)await reader.cancel().catch(()=>{});reader.releaseLock();}
}
