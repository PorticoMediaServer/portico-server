import {randomId} from './random-id.ts';
import {ApiError,HttpLocalApi,type LocalSession} from './index.ts';
import {parseLocalSession} from './local-session.ts';
export type SetupDraft={version:1;serverId:string;requestId:string;setupToken:string;name:string;username:string;authMode:'hosted'|'local';recoverySaved:boolean;interactive?:boolean};
export interface SetupDraftStore {read():Promise<SetupDraft|undefined>;change(update:(old:SetupDraft|undefined)=>SetupDraft|undefined):Promise<void>}
export type SetupInput=Omit<SetupDraft,'version'|'serverId'|'requestId'>&{password:string};
function serverId(api:HttpLocalApi){const id=api.getRouteConnection()?.pin.serverId;if(!id)throw new Error('Verify this server identity before initialization.');return id;}
/** Passwords are never placed in the draft. The high-entropy setup secret and
 * operation ID live only in protected credential storage until durable adoption. */
export async function initializeServer(api:HttpLocalApi,store:SetupDraftStore,input:SetupInput,signal:AbortSignal,requestId:()=>Promise<string>=async()=>randomId()):Promise<LocalSession>{
 const id=serverId(api),newId=await requestId();let draft:SetupDraft|undefined;
 await store.change(old=>{if(old&&(old.serverId!==id||old.setupToken!==input.setupToken))throw new Error('A different setup is saved. Resume it or sign in with your recovery owner.');draft=old??{version:1,serverId:id,requestId:newId,setupToken:input.setupToken,name:input.name,username:input.username,authMode:input.authMode,recoverySaved:input.recoverySaved,interactive:input.interactive};return draft;});
 try{return await resumeServerSetup(api,store,signal);}catch(e){if(!(e instanceof ApiError)||e.code!=='not_found')throw e;}
 return parseLocalSession(await api.request('/v1/setup','POST',{requestId:draft!.requestId,setupToken:draft!.setupToken,username:draft!.username,password:input.password,name:draft!.name,authMode:draft!.authMode,recoverySaved:draft!.recoverySaved,interactive:draft!.interactive},signal),{serverId:id,authority:'local'});
}
export async function resumeServerSetup(api:HttpLocalApi,store:SetupDraftStore,signal:AbortSignal):Promise<LocalSession>{
 const id=serverId(api),draft=await store.read();if(!draft||draft.version!==1||draft.serverId!==id)throw new Error('No interrupted setup is saved for this server.');
 return parseLocalSession(await api.request('/v1/setup/resume','POST',{requestId:draft.requestId,setupToken:draft.setupToken},signal),{serverId:id,authority:'local'});
}
export async function completeServerSetupDraft(api:HttpLocalApi,store:SetupDraftStore){const id=serverId(api);await store.change(old=>old?.serverId===id?undefined:old);}
