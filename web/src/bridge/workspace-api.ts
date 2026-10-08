import {HttpLocalApi} from '@core/index.ts';
import {beginLiveSourceAuthority,denyLiveSourceAuthority,endLiveSourceAuthority,type LiveSourceAuthority} from '@core/live-source-draft.ts';
function denied(error:unknown,path:string){if(error===null||typeof error!=='object'||!('status' in error))return false;return error.status===401||error.status===403&&(path==='/v1/guide'||path.startsWith('/v1/guide?')||path==='/v1/admin/live-sources'||path.startsWith('/v1/admin/live-sources/'));}
/** A denial only clears the still-current authenticated workspace lifetime.
 * Ordinary draft edits remain in that lifetime; unrelated403 is not revocation. */
export class WorkspaceApi extends HttpLocalApi{
 private authority:LiveSourceAuthority|null=null;private scope:string;
 constructor(baseUrl:string,token:string,scope:string,fetcher?:typeof fetch){super(baseUrl,token,fetcher);this.scope=scope;}
 activateSourceAuthority(){const lease=beginLiveSourceAuthority(this.scope);this.authority=lease;return lease;}
 disposeSourceAuthority(expected:LiveSourceAuthority|null=this.authority){if(!expected)return;endLiveSourceAuthority(expected);if(this.authority===expected)this.authority=null;}
 override async request<T>(path:string,method='GET',body?:unknown,signal?:AbortSignal):Promise<T>{const authority=this.authority;try{return await super.request<T>(path,method,body,signal)}catch(error){if(authority&&denied(error,path))denyLiveSourceAuthority(authority);throw error}}
 override async requestBounded<T>(path:string,maxBytes:number,signal:AbortSignal):Promise<T>{const authority=this.authority;try{return await super.requestBounded<T>(path,maxBytes,signal)}catch(error){if(authority&&denied(error,path))denyLiveSourceAuthority(authority);throw error}}
}
