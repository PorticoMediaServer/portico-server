import {parsePreferenceSnapshot,type DeviceClass,type PreferencePatch,type PreferenceScope,type PreferenceSnapshot} from './preferences.ts';

/**
 * Minimal viewer-preferences reader (PERF-25): `GET`/`PATCH /v1/preferences`
 * without the owner-console client (`console.ts`) and its telemetry,
 * attention and playback-history parsers, which every viewer would otherwise
 * download at startup to read a few settings.
 *
 * Contract mirror of `ConsoleClient.preferences`/`applyPreferences`: the
 * server wraps data in `{scope: {serverId, viewerFence}, data}`, the fence
 * pins the selected viewer (a changed fence ends this scope), and a PATCH
 * carries an idempotency key. No intent journal: callers surface failures
 * directly (the web settings UI retries by hand).
 */
export type PreferencesApi={request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>;requestConsole?<T>(path:string,method:string,body:unknown,signal:AbortSignal):Promise<T>};
export type PreferencesScope={serverId:string;viewerId:string};

const object=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);

/** Operation keys are uniqueness tokens, not credentials or authorization. */
export function preferencesOperationId():string{return globalThis.crypto?.randomUUID?.()??`op-${Date.now().toString(36)}-${Math.random().toString(36).slice(2,16)}`;}

export class PreferencesReader {
  private disposed=false;private fence?:string;private requests=new Set<AbortController>();
  private api:PreferencesApi;readonly scope:PreferencesScope;
  constructor(api:PreferencesApi,scope:PreferencesScope){if(!scope.serverId||!scope.viewerId)throw new Error('A selected viewer is required.');this.api=api;this.scope={...scope};}
  dispose(){this.disposed=true;for(const c of this.requests)c.abort();this.requests.clear();}
  private async call<T>(path:string,method:string,body:unknown,parse:(v:unknown)=>T):Promise<T>{
    if(this.disposed)throw new Error('This preferences scope is closed.');
    const c=new AbortController();this.requests.add(c);const timer=setTimeout(()=>c.abort(),10000);
    try{
      const raw=await (this.api.requestConsole?this.api.requestConsole<unknown>(path,method,body,c.signal):this.api.request<unknown>(path,method,body,c.signal));
      if(this.disposed||c.signal.aborted)throw new Error('Preferences request ended.');
      if(!object(raw)||!object(raw.scope)||raw.scope.serverId!==this.scope.serverId||typeof raw.scope.viewerFence!=='string'||!/^[a-f0-9]{64}$/.test(raw.scope.viewerFence)||this.fence!==undefined&&this.fence!==raw.scope.viewerFence)throw new Error('The selected viewer changed. Reopen this server.');
      const out=parse(raw.data);this.fence=raw.scope.viewerFence;return out;
    }finally{clearTimeout(timer);this.requests.delete(c);}
  }
  preferences(deviceClass:DeviceClass){return this.call('/v1/preferences?deviceClass='+deviceClass,'GET',undefined,parsePreferenceSnapshot);}
  /** `values` carries only the keys that change; `null` clears that override. */
  applyPreferences(scope:PreferenceScope,deviceClass:DeviceClass,expectedRevision:number,values:PreferencePatch){return this.call('/v1/preferences','PATCH',{scope,deviceClass,expectedRevision,values,idempotencyKey:preferencesOperationId()},parsePreferenceSnapshot);}
}
