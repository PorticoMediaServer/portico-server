import {parseBrowserCredentialRecord} from '@core/credential-format.ts';
import {
  ApiError,
  transportJSON,
  CredentialSessionService,
  type CredentialApi,
  type CredentialContext,
  type CredentialRecord,
  type CredentialStorage,
  type HostedSession,
} from '@core/index.ts';

type BrowserAuthority = {familyId:string};
type BrowserRecord = CredentialRecord<BrowserAuthority>;
type StoragePort = Pick<Storage,'getItem'|'setItem'>;
const maximumBytes = 128 * 1024;

/** Only noncredential coordination state is durable. The browser owns the HttpOnly cookie. */
export class BrowserCredentialStorage implements CredentialStorage<BrowserAuthority> {
  private storage:StoragePort;
  private key:string;
  constructor(storage:StoragePort,key:string) {this.storage=storage;this.key=key;}

  async load():Promise<BrowserRecord|undefined> {
    const raw=this.storage.getItem(this.key);
    if(raw===null)return;
    if(raw.length>maximumBytes)throw new Error('Saved account state is too large.');
    return parseBrowserCredentialRecord(JSON.parse(raw));
  }

  async save(record:BrowserRecord) {
    const raw=JSON.stringify(parseBrowserCredentialRecord(record,false));
    if(raw.length>maximumBytes)throw new Error('Saved account state is too large.');
    this.storage.setItem(this.key,raw);
  }


}

/** A double-submit CSRF token has this shape (Hosted `security.RandomID`). */
const csrfShape=/^[A-Za-z0-9_-]{43}$/;

export class BrowserAccountApi implements CredentialApi<BrowserAuthority> {
  readonly origin:string;
  private pending=new Set<Promise<unknown>>();
  private readonly csrfKey:string;
  constructor(origin:string) {
    const url=new URL(origin);
    if(url.protocol!=='https:'&&!(url.protocol==='http:'&&['127.0.0.1','localhost','[::1]'].includes(url.hostname)))
      throw new Error('Portico Accounts require an HTTPS service.');
    this.origin=url.origin;
    this.csrfKey='portico.account.csrf.v1:'+this.origin;
  }

  /**
   * Hosted protects the browser refresh with a double-submit token: it sets a readable
   * `…_csrf` cookie and answers every sign-in and refresh with `X-CSRF-Token`, and a refresh must
   * send that value back. It is not a credential (it is useless without the HttpOnly refresh
   * cookie). When the web shares Hosted's host (web.getportico.tv, or a loopback dev origin) the
   * cookie itself is read; otherwise the last header value, kept in localStorage across reloads.
   */
  private csrfToken():string|undefined {
    // The cookie is always the current value when this page can read it (same host as Hosted);
    // otherwise the last value Hosted sent in X-CSRF-Token.
    try{
      for(const part of document.cookie.split(';')){
        const [name,...value]=part.trim().split('=');
        if((name.startsWith('__Host-portico_refresh')||name.startsWith('portico_refresh'))&&name.endsWith('_csrf')){const v=value.join('=');if(csrfShape.test(v))return v;}
      }
    }catch{}
    try{const kept=localStorage.getItem(this.csrfKey);if(kept&&csrfShape.test(kept))return kept;}catch{}
    return undefined;
  }
  private keepCsrf(response:Response) {
    const token=response.headers.get('X-CSRF-Token');
    if(token&&csrfShape.test(token)){try{localStorage.setItem(this.csrfKey,token);}catch{}}
  }

  request<T>(path:string,body:unknown,signal?:AbortSignal):Promise<T> {
    return this.send<T>(path,'POST',body,signal);
  }

  send<T>(path:string,method:string,body?:unknown,signal?:AbortSignal,token?:string):Promise<T> {
    const operation=this.perform<T>(path,method,body,signal,token);
    this.pending.add(operation);
    void operation.finally(()=>this.pending.delete(operation)).catch(()=>{});
    return operation;
  }

  async settled() {
    while(this.pending.size)await Promise.allSettled([...this.pending]);
  }

  private async perform<T>(path:string,method:string,body:unknown,signal?:AbortSignal,token?:string):Promise<T> {
    if(!path.startsWith('/v1/'))throw new Error('Invalid account request.');
    const csrf=method==='GET'?undefined:this.csrfToken();
    const response=await fetch(this.origin+path,{
      method,credentials:'include',redirect:'error',
      signal:signal?AbortSignal.any([signal,AbortSignal.timeout(15000)]):AbortSignal.timeout(15000),
      headers:{'Content-Type':'application/json',...(token?{Authorization:'Bearer '+token}:{}),...(csrf?{'X-CSRF-Token':csrf}:{})},...(body===undefined?{}:{body:transportJSON(body)}),
    });
    this.keepCsrf(response);
    if(!response.ok){
      const payload=await response.json().catch(()=>undefined);
      throw new ApiError(response.status,payload?.error?.code??payload?.code??'request_failed',payload?.error?.message??'Could not reach your Portico Account.',payload?.error?.retryable??response.status>=500);
    }
    return response.status===204?undefined as T:response.json();
  }

  authorityFromSession(session:HostedSession):BrowserAuthority {
    if(!session.familyId)throw new Error('Account renewal is not available. Sign in again.');
    if(session.refreshToken)throw new Error('The account service returned a native credential to the browser.');
    return {familyId:session.familyId};
  }
  refresh(authority:BrowserAuthority,requestId:string,signal?:AbortSignal) {
    return this.request<HostedSession>('/v1/sessions/refresh',{familyId:authority.familyId,requestId},signal);
  }
  revoke(authority:BrowserAuthority,signal?:AbortSignal) {
    return this.request<void>('/v1/sessions/revoke',{familyId:authority.familyId},signal);
  }

}

let instance:{api:BrowserAccountApi;service:CredentialSessionService<BrowserAuthority>}|undefined;
export function browserAccount() {
  if(instance)return instance;
  const api=new BrowserAccountApi(import.meta.env.VITE_HOSTED_URL??(import.meta.env.VITE_ACCOUNT_WORKSPACE==='true'||location.hostname==='web.getportico.tv'?location.origin:'http://127.0.0.1:19410'));
  const key='portico.account.v1:'+api.origin;
  const storage=new BrowserCredentialStorage({getItem:name=>localStorage.getItem(name),setItem:(name,value)=>localStorage.setItem(name,value)},key);
  const service=new CredentialSessionService(api,storage,async()=>crypto.randomUUID(),15000,{
    runExclusive:operation=>{
      if(!navigator.locks)throw new Error('This browser cannot safely coordinate account sessions. Use a current browser.');
      return navigator.locks.request(key,{signal:AbortSignal.timeout(20000)},async()=>{
        try{return await operation();}
        finally{await api.settled();}
      });
    },
  });
  const sync=()=>{void service.synchronize().catch(()=>{});};
  addEventListener('storage',event=>{if(event.key===key)sync();});
  addEventListener('online',()=>{void service.retryRevocations().catch(()=>{});});
  // A sign-in is a 90-day window renewed by use of the account service, which a client in daily
  // use against its own server may never need. Renew it about monthly, some minutes after
  // launch at a random moment, so it is never part of a burst.
  setTimeout(()=>{void service.keepAlive().catch(()=>{});},60000+Math.random()*540000);
  instance={api,service};
  return instance;
}
