import {HttpLocalApi, type HostedServer, type LocalSession} from '@core/index.ts';
import {ServerSignInProbe, parseServerSignIn, type ServerSignIn} from '@core/sign-in.ts';
import {directSignIn, directSignInContinuation, shouldRetireAccountSession, completeDirectSignInChallenge, changeRequiredPassword, parseDirectSnapshot, type DirectSignIn, type DirectSnapshot} from '@core/profile-management.ts';
import {inspectDirectServer, connectPairedServer, connectPairedPorticoServer, connectHostedServer, rememberNativeSession, type ConnectionEnvironment, type RememberedServer} from '@core/server-connections.ts';
import {assertFirstContactIdentity, RouteError, sameServerPin, type ServerPin} from '@core/route-identity.ts';
import {knownServerRoutes, type AppSessionStore} from '@core/app-sessions.ts';
import {acceptPorticoInvitation, porticoSignIn, type HostedRequestApi} from '@core/portico-servers.ts';
import type {JoinLink} from '@core/join-link.ts';
import {passwordStrength} from '@core/password-strength.ts';
import {requestOwnerResetFile, verifyOwnerResetFile, verifyMemberResetCode, changeResetPassword, ownerResetSecurity, finishOwnerResetPassword, type ResetFileLocation} from '@core/account-reset.ts';
import {MemberAccountMoveClient, type MoveCodePreview, type MoveCodeLink} from '@core/account-move.ts';
import {IdentityClient, type TwoFactorEnrolment} from '@core/identity-client.ts';
import {CodedError, unreadableServerResponse} from '@core/server-messages.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {decodePreviewInvitationResponse, decodePostAccessInvitationsAcceptResponse} from '../../../packages/contracts/src/decoders.ts';
import type {PreviewInvitationResponse} from '../../../packages/contracts/src/types.ts';
import {browserAppSessions, browserConnectionEnvironment, restoreBrowserConnection, observeBrowserSessionChanges} from './server-connections';
import {browserAppSessionActions, type BrowserAppSessionActions} from './app-sessions';
import {browserServerDisplay} from './server-display';
import {BrowserProfileRecovery} from './profile-recovery';
import {createBrowserRegisteredSignIn, type BrowserRegisteredSignIn} from './registered-sign-in';
import {dispatchPorticoReturn, type CapturedPorticoReturn} from './portico-return-dispatch';
import {createBrowserOwnerClaim, type BrowserOwnerClaim} from './owner-claim';
import {createBrowserSetupClaim, type BrowserSetupClaim} from './setup-claim';
import {buildHostedSignInStartURL} from '@core/hosted-sign-in-start.ts';
import type {AccountServerAccess} from './account-server-access';

type RegisteredController=Pick<BrowserRegisteredSignIn,'prepare'|'finish'|'completeChallenge'|'cancel'>;
type RegisteredCompletion=Awaited<ReturnType<RegisteredController['finish']>>;
type OwnerReturnController=Pick<BrowserOwnerClaim,'pendingReturn'|'finish'|'cancel'>;
type SetupReturnController=Pick<BrowserSetupClaim,'pendingReturn'|'finish'|'cancel'>;
type ContinuationReturnController=OwnerReturnController|SetupReturnController;
type AdmittedCompletion=Readonly<{api:HttpLocalApi;result:Extract<DirectSignIn,{kind:'signed-in'}>}>;
export type OwnerResetDetails=Readonly<{username:string;twoStepEnabled:boolean}>;
export type HostedDirectoryServer=Readonly<Pick<HostedServer,'id'|'name'|'publicKey'|'policyRevision'|'baseUrl'|'routes'>>;
type OwnerResetCapture=Readonly<{source:HttpLocalApi;api:HttpLocalApi;grant:Extract<DirectSignIn,{kind:'signed-in'}>;pin:ServerPin;revision:number}>;

export type AccountEntryState = Readonly<{
 phase:'welcome'|'address'|'checking'|'password'|'portico'|'challenge'|'password-change'|'profiles'|'ready'|'joined'|'move-code'|'move-local'|'move-portico'|'two-step-setup'|'owner-reset-password';
 server?:ServerSignIn; origin?:string; pin?:ServerPin; snapshot?:DirectSnapshot;
 invitation?:PreviewInvitationResponse; joined?:Readonly<{origin:string;signIn:'local'|'portico'}>;
 movePreview?:MoveCodePreview; ownerReset?:OwnerResetDetails;
 busy?:boolean; error?:unknown;
}>;
type EntryPorts = Readonly<{
 store:AppSessionStore; actions:BrowserAppSessionActions;
 environment:(id?:string)=>ConnectionEnvironment;
 restore:typeof restoreBrowserConnection;
 inspect:typeof inspectDirectServer; connect:typeof connectPairedServer; remember:typeof rememberNativeSession;
 connectPortico?:typeof connectPairedPorticoServer;
 connectHosted?:typeof connectHostedServer; signInHosted?:typeof porticoSignIn;
 retire?:(api:HttpLocalApi,session:LocalSession)=>Promise<void>;
 temporary?:(source:HttpLocalApi,session:LocalSession)=>HttpLocalApi;
 invitationDestination?:'adopt'|'server-origin';
 observeSelection?:(changed:()=>void)=>()=>void;
 registered?:()=>Readonly<{controller:RegisteredController;installationId:()=>string}>;
 ownerReturn?:()=>OwnerReturnController;
 setupReturn?:()=>SetupReturnController;
 setupRequired?:(origin:string,pin:ServerPin)=>void;
 rememberName?:(api:HttpLocalApi,name:string)=>void;
}>;

/** Web account entry keeps credential entry behind an explicit kind and private-key proof.
 * Each operation captures its destination. Abandoning it cannot publish a late credential. */
export class BrowserAccountEntry {
 private state:AccountEntryState=Object.freeze({phase:'welcome'});
 private readonly listeners=new Set<()=>void>();
 private readonly retiredListeners=new Set<()=>void>();private offSelection?:()=>void;
 private epoch=0;private controller?:AbortController;private api?:HttpLocalApi;private session?:LocalSession;
 private challenge?:Extract<DirectSignIn,{kind:'challenge'}>;private challengeRevision?:number;private disposed=false;
 private registered?:RegisteredController;private registeredChallenge?:RegisteredController;private ownerReturn?:ContinuationReturnController;
 private invitationCode?:string;private joinedAccountId?:string;private joinedUsername?:string;
 private ownerReset?:OwnerResetCapture;private ownerResetTimer?:ReturnType<typeof setTimeout>;
 private moveProof?:Readonly<{code:string;revision:number;preview:MoveCodePreview}>;
 private moveTicket?:Readonly<{ticket:string;revision:number;expiresAt:string}>;
 private hostedChallenge?:Readonly<{account:AccountServerAccess;hosted:NonNullable<RememberedServer['hosted']>}>;
 private offHostedChallenge?:()=>void;
 readonly probe:ServerSignInProbe;
 constructor(private readonly ports:EntryPorts,probe=new ServerSignInProbe()){this.probe=probe;}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(value:AccountEntryState){if(this.disposed)return;this.state=Object.freeze(value);for(const fn of this.listeners){try{fn();}catch{ /* One view cannot undo a committed credential or block other subscribers. */ }}}
 subscribeAuthorityRetired=(listener:()=>void)=>{this.retiredListeners.add(listener);return()=>{this.retiredListeners.delete(listener);};};
 private retired(){for(const listener of this.retiredListeners){try{listener();}catch{ /* Clear every other secret-bearing view too. */ }}}
 private stopAuthorityWatch(){const off=this.offSelection;this.offSelection=undefined;try{off?.();}catch{ /* Retirement must still clear the captured authority. */ }}
 private operation(){this.controller?.abort();const epoch=++this.epoch,controller=this.controller=new AbortController();return {signal:controller.signal,current:()=>!this.disposed&&epoch===this.epoch&&!controller.signal.aborted};}
 private release(){
  this.clearHostedChallenge();
  this.stopAuthorityWatch();const api=this.api,registered=this.registered,ownerReturn=this.ownerReturn,reset=this.takeOwnerReset(),hadAuthority=!!this.session||!!this.challenge||!!registered||!!ownerReturn||!!reset;
  this.api=undefined;this.session=undefined;this.challenge=undefined;this.challengeRevision=undefined;this.moveProof=undefined;this.moveTicket=undefined;this.registered=undefined;this.registeredChallenge=undefined;this.ownerReturn=undefined;
  if(ownerReturn)void ownerReturn.cancel().catch(()=>{});
  if(registered)void registered.cancel().catch(()=>{});
  if(hadAuthority)this.retired();
  if(reset)void this.retire(reset.source,reset.grant.session).catch(()=>{}).finally(()=>api?.getRouteConnection()?.dispose());
  else api?.getRouteConnection()?.dispose();
 }
 /** Read canonical selection after credential-free change hints. The private
  * captured grant defines this lifetime; no family id is added to public state. */
 private watchAuthority(){
  this.stopAuthorityWatch();
  const observe=this.ports.observeSelection,api=this.api,session=this.session,challenge=this.challenge??this.registeredChallenge,revision=this.challengeRevision,pin=api?.getRouteConnection()?.pin,kind=this.state.server?.signIn;
  if(!observe||!api||!pin||!session&&!challenge)return;
  let stopped=false,reading=false,changes=0;
  const current=()=>!stopped&&!this.disposed&&this.api===api&&(session?!!this.session: (this.challenge??this.registeredChallenge)===challenge);
  const check=async()=>{
   ++changes;if(reading)return;reading=true;
   try{while(current()){
    const expected=changes;
    if(session){
     const selected=await this.ports.store.selected();if(!current())return;
     const saved=selected?.connection.session;
     if(!selected||selected.signIn!==kind||!sameServerPin(selected.connection.pin,pin)||!saved||saved.sessionFamilyId!==session.sessionFamilyId||(['serverId','authority','accountId','profileId','role'] as const).some(key=>saved.viewer[key]!==session.viewer[key])){this.cancel();return;}
     // A rotation is the same private lifetime, with its actual saved bearer.
     if(this.session?.accessToken!==saved.accessToken){this.session=saved;api.setAccessToken(saved.accessToken);}
    }else{
     const selected=await this.ports.store.read();if(!current())return;
     if(selected.revision!==revision){this.cancel();return;}
    }
    if(expected===changes)return;
   }}catch(error){if(current()){this.cancel();this.publish({phase:'welcome',error});}}
   finally{reading=false;}
  };
  let off:()=>void;
  try{off=observe(()=>{void check();});}catch(error){this.cancel();this.publish({phase:'welcome',error});return;}
  if(!current()){stopped=true;off();return;}
  this.offSelection=()=>{stopped=true;off();};void check();
 }
 cancel(){++this.epoch;this.controller?.abort();this.probe.cancel();this.release();this.invitationCode=undefined;this.joinedAccountId=undefined;this.joinedUsername=undefined;this.publish({phase:'welcome'});}
 address(){this.cancel();this.publish({phase:'address'});}
 private clearHostedChallenge(){const off=this.offHostedChallenge;this.hostedChallenge=undefined;this.offHostedChallenge=undefined;off?.();}
 private watchHostedChallenge(account:AccountServerAccess,hosted:NonNullable<RememberedServer['hosted']>){
  this.clearHostedChallenge();const capture=this.hostedChallenge={account,hosted};
  const off=account.observe?.(()=>{if(this.hostedChallenge===capture&&!account.isCurrent())this.cancel();});
  if(this.hostedChallenge===capture)this.offHostedChallenge=off;else off?.();
  if(!account.isCurrent())this.cancel();
 }
 /** A directory fact is only a hint until Hosted-signed routes and the Server's
  * private key prove this exact identity. The Server still decides membership. */
 async openHostedServer(input:HostedDirectoryServer,source:AccountServerAccess):Promise<boolean>{
  const account:AccountServerAccess=Object.freeze({origin:source.origin,accountId:source.accountId,request:source.request.bind(source),isCurrent:source.isCurrent.bind(source),...(source.observe?{observe:source.observe.bind(source)}:{})});
  const server:HostedServer=Object.freeze({id:input.id,name:input.name,publicKey:input.publicKey,policyRevision:input.policyRevision,baseUrl:input.baseUrl,...(input.routes?{routes:structuredClone(input.routes)}:{})});
  this.cancel();const op=this.operation();this.publish({phase:'checking',busy:true});
  const guard={...op,current:()=>op.current()&&account.isCurrent()};
  let connected:Awaited<ReturnType<typeof connectHostedServer>>|undefined;
  try{
   if(!guard.current())throw new CodedError('cancelled','The Portico Account changed.');
   const revision=(await this.ports.store.read()).revision;if(!guard.current())return false;
   const request:HostedRequestApi['request']=async<T,>(path:string,method?:string,body?:unknown,signal?:AbortSignal)=>{
    if(!guard.current())throw new CodedError('cancelled','The Portico Account changed.');
    const result=await account.request<T>(path,method,body,signal);
    if(!guard.current())throw new CodedError('cancelled','The Portico Account changed.');return result;
   };
   connected=await(this.ports.connectHosted??connectHostedServer)(server,{origin:account.origin,accountId:account.accountId,request:(path,signal)=>request(path,'GET',undefined,signal)},this.ports.environment(server.id),op.signal);
   if(!guard.current())return false;
   const api=connected.api,pin=connected.connection.pin,origin=api.baseUrl;
   const verified=parseServerSignIn(await api.request(routePath('get_system'),'GET',undefined,op.signal));
   if(!guard.current())return false;
   if(pin.serverId!==server.id||pin.publicKey!==server.publicKey||connected.hosted.origin!==account.origin||connected.hosted.accountId!==account.accountId||verified.id!==server.id||verified.signIn!=='portico'||verified.setupRequired)throw new RouteError('identity_mismatch','The server identity or sign-in changed.');
   const saved=await this.ports.store.read();if(!guard.current())return false;
   if(saved.revision!==revision)throw new CodedError('cancelled','The server selection changed.');
   assertFirstContactIdentity(origin,pin,{linkId:server.id,knownRoutes:knownServerRoutes(saved)});
   this.api=api;this.ports.rememberName?.(api,verified.name);
   this.publish({phase:'portico',busy:true,server:verified,origin,pin});
   if(!guard.current())return false;
   const result=await(this.ports.signInHosted??porticoSignIn)({request},account.origin,{request:(path,method,body,signal)=>{
    if(!guard.current())throw new CodedError('cancelled','The Portico Account changed.');
    return api.request(path,method,body,signal);
   }},server.id,{kind:'interactive',signal:op.signal});
   if(!guard.current()||result.kind==='signed-in'&&result.snapshot.account.hostedAccountId!==account.accountId){
    if(result.kind==='signed-in')await this.retire(api,result.session);
    if(op.current())throw new CodedError('cancelled','The Portico Account changed.');return false;
   }
   if(result.kind==='challenge')this.watchHostedChallenge(account,connected.hosted);
   if(!guard.current())return false;
   await this.accept(result,api,revision,guard,connected.hosted);
   return guard.current();
  }catch(error){if(op.current()){this.release();this.publish({phase:'address',error,busy:false});}return false;}
  finally{if(op.current()&&!account.isCurrent())this.cancel();if(connected&&this.api!==connected.api)connected.connection.dispose();}
 }
 async inspect(address:string,linkId?:string){
  this.cancel();const op=this.operation();this.publish({phase:'checking'});
  try{
   const server=await this.probe.inspect(address);if(!op.current())return;
   if(!server)throw this.probe.getSnapshot().error??new Error('The server did not answer.');
   const origin=this.probe.getSnapshot().origin!;
   const pin=await this.ports.inspect(origin,this.ports.environment(server.id),op.signal);
   if(!op.current())return;if(pin.serverId!==server.id)throw new RouteError('identity_mismatch','The server identity changed.');
   assertFirstContactIdentity(origin,pin,{linkId,knownRoutes:knownServerRoutes(await this.ports.store.read())});
   if(!op.current())return;if(server.setupRequired){
    if(this.ports.setupRequired){this.ports.setupRequired(origin,pin);return;}
    throw new RouteError('setup_required','This server needs setup.');
   }
   const connected=await this.ports.connect(origin,pin,this.ports.environment(server.id),op.signal);
   if(!op.current()){connected.connection.dispose();return;}
   // A second kind read on the verified connection closes a change between discovery and entry.
   this.api=connected.api;
   const verified=parseServerSignIn(await connected.api.request(routePath('get_system'),'GET',undefined,op.signal));
   if(!op.current())return;if(verified.id!==server.id||verified.signIn!==server.signIn)throw new RouteError('identity_mismatch','The server sign-in changed.');
   assertFirstContactIdentity(origin,pin,{linkId,knownRoutes:knownServerRoutes(await this.ports.store.read())});
   if(!op.current())return;
   this.ports.rememberName?.(connected.api,verified.name);
   this.publish({server:verified,origin,pin,phase:verified.signIn==='local'?'password':'portico',busy:false});
   return {api:connected.api,current:op.current};
  }catch(error){if(op.current()){this.release();this.publish({phase:'address',error});}}
 }
 private temporary(api:HttpLocalApi,session:LocalSession){
  if(this.ports.temporary)return this.ports.temporary(api,session);
  const cleanup=new HttpLocalApi(api.baseUrl,session.accessToken);const route=api.getRouteConnection();if(route)cleanup.useRoutes(route);return cleanup;
 }
 private async retire(api:HttpLocalApi,session:LocalSession){
  if(this.ports.retire)return this.ports.retire(api,session);
  const cleanup=this.temporary(api,session);
  await cleanup.request(routePath('delete_sessions_current'),'DELETE',undefined,AbortSignal.timeout(5000)).catch(()=>{});
 }
 private async accept(result:DirectSignIn,api:HttpLocalApi,revision:number,op:ReturnType<BrowserAccountEntry['operation']>,hosted?:RememberedServer['hosted']){
  if(result.kind==='challenge'){if(op.current()){this.challenge=result;this.challengeRevision=revision;this.watchAuthority();this.publish({...this.state,phase:'challenge',busy:false});}return;}
  this.stopAuthorityWatch();
  try{
   if(!op.current())throw new Error('Sign-in was cancelled.');
   if(!this.state.server||result.snapshot.serverId!==this.state.server.id)throw new Error('The server identity changed.');
   if(this.joinedAccountId&&result.snapshot.account.id!==this.joinedAccountId)throw new CodedError('identity_mismatch','The invitation account changed.');
   if(this.state.invitation&&this.ports.invitationDestination==='server-origin'){
    await this.retire(api,result.session);
    if(op.current())this.publish({...this.state,phase:'joined',joined:{origin:this.state.origin!,signIn:this.state.server.signIn},busy:false,error:undefined});
    this.invitationCode=undefined;return;
   }
   const previous=await this.ports.store.selected(),pin=api.getRouteConnection()?.pin;
   if(!op.current())throw new CodedError('cancelled','The server selection changed.');
   api.setAccessToken(result.session.accessToken);
   const env={...this.ports.environment(result.snapshot.serverId),storage:this.ports.store.connectionStorage(result.snapshot.serverId,{signIn:this.state.server.signIn,hostedAccount:hosted,expectedRevision:revision})};
   await this.ports.remember(api,result.session,env,hosted,op.current);
   if(!op.current())return;
   const installed=await this.ports.store.selected();
   if(!op.current())return;
   if(!pin||!installed||!sameServerPin(installed.connection.pin,pin)||installed.connection.session.sessionFamilyId!==result.session.sessionFamilyId||installed.connection.session.viewer.accountId!==result.snapshot.account.id)throw new CodedError('cancelled','The server selection changed.');
   this.session=result.session;
   // Cleanup is for this exact superseded account grant only. It never runs
   // before durable admission, and never revokes the current or a peer family.
   if(previous?.signIn===installed.signIn&&previous.connection.session.viewer.role==='account'&&previous.connection.session.viewer.authority===result.session.viewer.authority&&previous.connection.session.viewer.accountId===result.snapshot.account.id&&sameServerPin(previous.connection.pin,pin)&&shouldRetireAccountSession(previous.connection.session,result.session))void this.retire(api,previous.connection.session).catch(()=>{});
   const continuation=directSignInContinuation(result);
   this.publish({...this.state,snapshot:result.snapshot,busy:false,error:undefined,phase:continuation==='viewer'?'ready':continuation==='profile-selection'?'profiles':continuation});
   this.watchAuthority();
  }catch(error){await this.retire(api,result.session);if(op.current()){api.setAccessToken('');this.watchAuthority();}throw error;}
 }
 /** The link code is not posted until the kind, private-key proof, link id and
  * all remembered ownership of this origin have passed the normal entry guard. */
 async inspectInvitation(link:JoinLink){
  const inspected=await this.inspect(link.server,link.id);if(!inspected?.current())return;
  const state=this.state,api=inspected.api;
  if(!api||!state.server||!['password','portico'].includes(state.phase))return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{
   const preview=decodePreviewInvitationResponse(await api.request(routePath('preview_invitation'),'POST',{code:link.code},op.signal));
   if(!op.current())return;
   if(preview.signInMethod!==state.server.signIn||!['member','admin'].includes(preview.role))throw new CodedError('invalid_response','The invitation sign-in method changed.');
   this.invitationCode=link.code;this.publish({...state,invitation:preview,busy:false});
  }catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }
 async joinLocal(username:string,password:string,name=''){
  const state=this.state,api=this.api,code=this.invitationCode,trimmed=username.trim();
  if(state.phase!=='password'||state.server?.signIn!=='local'||!state.invitation||!api||!code||state.busy)return;
  if(trimmed.length<3||trimmed.length>64||/[\s@]/.test(trimmed)||!passwordStrength(password).acceptable)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{
   const revision=(await this.ports.store.read()).revision;if(!op.current())return;
   if(this.joinedUsername!==undefined&&this.joinedUsername!==trimmed)throw new CodedError('invitation_used','Continue with the account already created.');
   if(this.joinedUsername===undefined){
    const member=decodePostAccessInvitationsAcceptResponse(await api.request(routePath('post_access_invitations_accept'),'POST',{code,username:trimmed,password,...(name.trim()?{name:name.trim()}:{})},op.signal));
    if(!op.current())return;
    if(!('accountId' in member)||!member.accountId||member.username!==trimmed)throw new CodedError('invalid_response','The invitation account could not be read.');
    this.joinedAccountId=member.accountId;this.joinedUsername=trimmed;
   }
   if(!op.current())return;
   await this.accept(await directSignIn(api,trimmed,password,op.signal),api,revision,op);
  }catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }
 async joinPortico(account:Readonly<{origin:string;accountId:string;request:HostedRequestApi['request'];isCurrent:()=>boolean}>){
  const state=this.state,code=this.invitationCode;
  if(state.phase!=='portico'||state.server?.signIn!=='portico'||!state.invitation||!state.pin||!state.origin||!code||state.busy||!account.isCurrent())return;
  const op=this.operation(),guard={...op,current:()=>op.current()&&account.isCurrent()};this.publish({...state,busy:true,error:undefined});
  let paired:Awaited<ReturnType<typeof connectPairedPorticoServer>>|undefined;
  try{
   const revision=(await this.ports.store.read()).revision;if(!guard.current())return;
   const check=()=>{if(!guard.current())throw new CodedError('cancelled','The Portico Account changed.');};
   const request:HostedRequestApi['request']=async<T,>(path:string,method?:string,body?:unknown,signal?:AbortSignal)=>{check();const result=await account.request<T>(path,method,body,signal);check();return result;};
   paired=await (this.ports.connectPortico??connectPairedPorticoServer)(state.origin,state.pin,
    {origin:account.origin,accountId:account.accountId,request:(path,signal)=>request(path,'GET',undefined,signal)},this.ports.environment(state.server.id),op.signal);
   if(!guard.current()){paired.connection.dispose();return;}
   const server=paired.api;
   const result=await acceptPorticoInvitation({request},account.origin,{request:(path,method,body,signal)=>{check();return server.request(path,method,body,signal);}},state.server.id,code,op.signal);
   if(result.kind!=='signed-in')throw new CodedError('invalid_response','The Portico invitation returned an unexpected challenge.');
   if(!guard.current()||result.snapshot.account.hostedAccountId!==account.accountId){await this.retire(paired.api,result.session);throw new CodedError('cancelled','The Portico Account changed.');}
   const previous=this.api;this.api=paired.api;previous?.getRouteConnection()?.dispose();
   await this.accept(result,paired.api,revision,guard,paired.hosted);
  }catch(error){if(guard.current())this.publish({...state,busy:false,error});}
  finally{
   if(paired&&this.api!==paired.api)paired.connection.dispose();
   // A family change ends this attempt, not the invitation. Its old guard may
   // never admit a grant, but the current page must let the new account act.
   if(op.current()&&!account.isCurrent())this.publish({...state,busy:false,error:undefined});
  }
 }
 async signIn(username:string,password:string){
  const state=this.state,api=this.api;
  if(state.phase!=='password'||state.server?.signIn!=='local'||state.invitation||!api||!username||!password)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{const revision=(await this.ports.store.read()).revision;if(!op.current())return;await this.accept(await directSignIn(api,username,password,op.signal),api,revision,op);}
  catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }
 /** The owner's code is previewed only at the already-proved server. It never
  * enters public state, browser storage, a URL or a Hosted request. */
 /** The synchronous fragment capture owns the code. Preserve its public
  * expected identity through proof before asking this server to preview it. */
 async inspectMoveLink(origin:string,link:MoveCodeLink){
  const {code,id}=link;
  const inspected=await this.inspect(origin,id);if(!inspected?.current())return;
  await this.previewMoveCode(code);
 }
 async previewMoveCode(code:string){
  const state=this.state,api=this.api;
  if(!api||!state.server||state.invitation||state.busy||!['password','portico','move-code','move-local','move-portico'].includes(state.phase))return;
  const op=this.operation();this.moveProof=undefined;this.moveTicket=undefined;this.publish({...state,phase:'move-code',movePreview:undefined,busy:true,error:undefined});
  const client=new MemberAccountMoveClient(api,{serverId:state.server.id,isCurrent:op.current,retireLateGrant:session=>this.retire(api,session)});
  try{
   const revision=(await this.ports.store.read()).revision;if(!op.current())return;
   const preview=await client.previewCode(code,op.signal);
   if(!op.current())return;
   if((await this.ports.store.read()).revision!==revision)throw new CodedError('cancelled','The server selection changed.');
   if(!op.current())return;
   if(preview.signIn!==state.server.signIn)throw new CodedError('identity_mismatch','The server sign-in changed.');
   this.moveProof={code,revision,preview};
   this.publish({...state,phase:preview.signIn==='local'?'move-local':'move-portico',movePreview:preview,busy:false,error:undefined});
  }catch(error){if(op.current())this.publish({...state,phase:'move-code',movePreview:undefined,busy:false,error});}
  finally{client.dispose();}
 }
 /** During the actual guided move the old password proves only this Waiting
  * account. The one-use ticket stays private until the protected return owns it. */
 async proveLocalMovePassword(username:string,password:string):Promise<boolean>{
  const state=this.state,api=this.api;
  if(state.phase!=='portico'||state.server?.signIn!=='portico'||state.server.move!=='to_portico'||state.invitation||state.busy||!api)return false;
  const op=this.operation();this.moveProof=undefined;this.moveTicket=undefined;this.publish({...state,busy:true,error:undefined});
  const client=new MemberAccountMoveClient(api,{serverId:state.server.id,isCurrent:op.current,retireLateGrant:session=>this.retire(api,session)});
  try{
   const revision=(await this.ports.store.read()).revision;if(!op.current())return false;
   const proof=await client.password(username,password,op.signal);
   if(!op.current())return false;
   if((await this.ports.store.read()).revision!==revision)throw new CodedError('cancelled','The server selection changed.');
   if(!op.current())return false;
   if(Date.parse(proof.expiresAt)<=Date.now())throw new CodedError('move_code_invalid','The move proof expired.');
   this.moveTicket={ticket:proof.ticket,expiresAt:proof.expiresAt,revision};
   this.publish({...state,phase:'move-portico',busy:false,error:undefined});return true;
  }catch(error){if(op.current())this.publish({...state,busy:false,error});return false;}
  finally{client.dispose();}
 }
 async redeemLocalMove(username:string,password:string){
  const state=this.state,api=this.api,proof=this.moveProof;
  if(state.phase!=='move-local'||state.server?.signIn!=='local'||state.busy||!api||!proof||!passwordStrength(password).acceptable)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  const client=new MemberAccountMoveClient(api,{serverId:state.server.id,isCurrent:op.current,retireLateGrant:session=>this.retire(api,session)});
  try{
   if(Date.parse(proof.preview.expiresAt)<=Date.now())throw new CodedError('move_code_invalid','The move code expired.');
   if((await this.ports.store.read()).revision!==proof.revision)throw new CodedError('cancelled','The server selection changed.');
   if(!op.current())return;
   const result=await client.redeemLocal(proof.code,username,password,op.signal);
   await this.accept(result,api,proof.revision,op);this.moveProof=undefined;
  }catch(error){if(op.current())this.publish({...state,busy:false,error});}
  finally{client.dispose();}
 }
 /** Auth20: the answer is a public location, never the file's one-use code.
  * This Local path cannot become a Portico recovery-owner continuation. */
 async requestLocalResetFile(username:string):Promise<ResetFileLocation|undefined>{
  const state=this.state,api=this.api;
  if(state.phase!=='password'||state.server?.signIn!=='local'||state.invitation||!api||state.busy)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{
   const location=await requestOwnerResetFile(api,username,op.signal);
   if(!op.current())return;
   this.publish({...state,busy:false});return location;
  }catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }
 /** Owner file proof is spent once. Only public security facts leave this
  * controller; the original password-only grant never enters the saved selection. */
 private takeOwnerReset(){const reset=this.ownerReset;this.ownerReset=undefined;clearTimeout(this.ownerResetTimer);this.ownerResetTimer=undefined;return reset;}
 private async assertOwnerReset(reset:OwnerResetCapture,current:()=>boolean){
  const captured=()=>{
   const pin=reset.source.getRouteConnection()?.pin;
   return current()&&this.ownerReset===reset&&this.api===reset.source&&!!pin&&sameServerPin(pin,reset.pin)&&Date.parse(reset.grant.session.expiresAt)>Date.now();
  };
  if(!captured())throw new CodedError('cancelled','The reset proof is no longer current.');
  const saved=await this.ports.store.read();
  if(!captured()||saved.revision!==reset.revision)throw new CodedError('cancelled','The server selection changed.');
 }
 private watchOwnerReset(reset:OwnerResetCapture){
  this.stopAuthorityWatch();
  const current=()=>!this.disposed&&this.ownerReset===reset;
  const check=()=>{void this.assertOwnerReset(reset,current).catch(()=>{if(current())this.cancelLocalOwnerReset();});};
  this.ownerResetTimer=setTimeout(()=>{if(current())this.cancelLocalOwnerReset();},Math.min(2147483647,Math.max(0,Date.parse(reset.grant.session.expiresAt)-Date.now())));
  try{this.offSelection=this.ports.observeSelection?.(check);}catch{this.cancelLocalOwnerReset();return;}
  check();
 }
 async verifyLocalOwnerReset(code:string):Promise<OwnerResetDetails|undefined>{
  const state=this.state,source=this.api;
  if(state.phase!=='password'||state.server?.signIn!=='local'||state.invitation||!source||state.busy||!code.trim())return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  let grant:Extract<DirectSignIn,{kind:'signed-in'}>|undefined,reset:OwnerResetCapture|undefined;
  try{
   const revision=(await this.ports.store.read()).revision;if(!op.current())return;
   const result=await verifyOwnerResetFile(source,code,false,op.signal);
   if(result.kind!=='signed-in')throw new CodedError('invalid_response','The reset did not return a password-change session.');
   grant=result;if(!op.current())return;
   if(result.snapshot.serverId!==state.server.id||result.snapshot.account.disabled||result.session.viewer.role!=='account'||result.session.viewer.profileId!==result.snapshot.account.primaryProfileId)throw new CodedError('invalid_response','The reset did not match this account.');
   const pin=source.getRouteConnection()?.pin;if(!pin||!state.pin||!sameServerPin(pin,state.pin))throw new CodedError('identity_mismatch','The server identity changed.');
   reset={source,api:this.temporary(source,result.session),grant:result,pin:Object.freeze({...pin}),revision};this.ownerReset=reset;
   await this.assertOwnerReset(reset,op.current);
   const details=await ownerResetSecurity(reset.api,result,op.signal);
   await this.assertOwnerReset(reset,op.current);
   this.publish({...state,phase:'owner-reset-password',ownerReset:details,busy:false,error:undefined});this.watchOwnerReset(reset);
   return this.ownerReset===reset?details:undefined;
  }catch(error){if(op.current())this.publish({...state,busy:false,error});}
  finally{
   if(grant&&!reset)await this.retire(source,grant.session).catch(()=>{});
   else if(reset&&this.ownerReset===reset&&this.state.phase!=='owner-reset-password'){
    this.takeOwnerReset();await this.retire(source,reset.grant.session).catch(()=>{});
   }
  }
 }
 cancelLocalOwnerReset(){
  const state=this.state,reset=this.takeOwnerReset();
  if(!reset&&!(state.phase==='password'&&state.busy))return;
  this.operation();this.stopAuthorityWatch();this.retired();
  if(reset)void this.retire(reset.source,reset.grant.session).catch(()=>{});
  this.publish({...state,phase:'password',ownerReset:undefined,busy:false,error:undefined});
 }
 async finishLocalOwnerResetPassword(newPassword:string,turnOffTwoStep:boolean):Promise<boolean>{
  const state=this.state,reset=this.ownerReset;
  if(state.phase!=='owner-reset-password'||!reset||state.busy||!passwordStrength(newPassword).acceptable||typeof turnOffTwoStep!=='boolean')return false;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{
   await this.assertOwnerReset(reset,op.current);
   await finishOwnerResetPassword(reset.api,reset.grant,newPassword,turnOffTwoStep,op.signal);
   await this.assertOwnerReset(reset,op.current);
   this.takeOwnerReset();this.stopAuthorityWatch();this.retired();await this.retire(reset.source,reset.grant.session).catch(()=>{});
   if(!op.current())return false;
   this.publish({...state,phase:'password',ownerReset:undefined,busy:false,error:undefined});return true;
  }catch(error){
   if(this.ownerReset===reset){this.takeOwnerReset();this.stopAuthorityWatch();this.retired();await this.retire(reset.source,reset.grant.session).catch(()=>{});}
   if(op.current())this.publish({...state,phase:'password',ownerReset:undefined,busy:false,error});return false;
  }
 }
 /** Code proof issues only a restricted transient family. Set its password,
  * retire it, then use ordinary sign-in and the existing durable admission. */
 async resetLocalPassword(proof:Readonly<{kind:'owner';code:string;turnOffTwoStep?:boolean}|{kind:'member';username:string;code:string}>,newPassword:string,secondFactorCode=''){
  if(!passwordStrength(newPassword).acceptable)return;
  if(proof.kind==='owner'){const details=await this.verifyLocalOwnerReset(proof.code);if(details&&await this.finishLocalOwnerResetPassword(newPassword,proof.turnOffTwoStep===true))await this.signIn(details.username,newPassword);return;}
  const state=this.state,api=this.api;
  if(state.phase!=='password'||state.server?.signIn!=='local'||state.invitation||!api||state.busy||!passwordStrength(newPassword).acceptable)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  let restricted:Extract<DirectSignIn,{kind:'signed-in'}>|undefined;
  try{
   const revision=(await this.ports.store.read()).revision;if(!op.current())return;
   const result=await verifyMemberResetCode(api,proof.username,proof.code,op.signal);
   if(result.kind!=='signed-in')throw new CodedError('invalid_response','The reset did not return a password-change session.');
   restricted=result;
   if(!op.current())return;
   if(result.snapshot.serverId!==state.server.id||result.resetFileOwnerSession||!result.passwordChangeRequired||result.session.refreshToken||
     result.snapshot.account.role==='owner')throw new CodedError('invalid_response','The reset did not match this account.');
   await changeResetPassword(this.temporary(api,result.session),newPassword,secondFactorCode,op.signal);
   await this.retire(api,result.session);restricted=undefined;
   if(!op.current())return;
   await this.accept(await directSignIn(api,result.snapshot.account.username,newPassword,op.signal),api,revision,op);
  }catch(error){if(op.current())this.publish({...state,busy:false,error});}
  finally{if(restricted)await this.retire(api,restricted.session);}
 }
 /** Factor setup keeps the existing restricted account family. Passwords,
  * secrets and recovery codes stay in the caller's ephemeral form, never state/storage. */
 private requiredFactorAttempt(){
  const state=this.state,api=this.api,session=this.session;
  if(state.phase!=='two-step-setup'||state.server?.signIn!=='local'||!state.snapshot?.twoStepSetupRequired||state.busy||!api||!session||session.viewer.role!=='account')return;
  const op=this.operation(),snapshot=state.snapshot;
  const assertCurrent=async()=>{
   const selected=await this.ports.store.selected(),saved=selected?.connection.session;
   if(!op.current()||this.api!==api||this.session!==session||this.state.snapshot!==snapshot||selected?.connection.pin.serverId!==snapshot.serverId||saved?.sessionFamilyId!==session.sessionFamilyId||saved.viewer.accountId!==snapshot.account.id||saved.viewer.profileId!==session.viewer.profileId||saved.viewer.role!=='account')throw new CodedError('cancelled','The server selection changed.');
  };
  const guarded={request:async<T,>(path:string,method?:string,body?:unknown,signal?:AbortSignal)=>{
   await assertCurrent();op.signal.throwIfAborted();
   const result=await api.request<T>(path,method,body,signal?AbortSignal.any([op.signal,signal]):op.signal);
   await assertCurrent();return result;
  }};
  const client=new IdentityClient(guarded);
  this.publish({...state,busy:true,error:undefined});
  return {state,op,client,readSnapshot:async()=>parseDirectSnapshot(await guarded.request(routePath('get_direct_account'),'GET'),{authority:'local',serverId:snapshot.serverId,accountId:snapshot.account.id})};
 }
 async beginRequiredTwoStep(password:string):Promise<TwoFactorEnrolment|undefined>{
  if(!password)return;
  const attempt=this.requiredFactorAttempt();if(!attempt)return;
  const {state,op,client}=attempt;
  try{const enrolment=await client.enrolTwoFactor(password);if(!op.current())return;this.publish({...state,busy:false,error:undefined});return enrolment;}
  catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }
 async confirmRequiredTwoStep(password:string,code:string):Promise<boolean>{
  if(!password||!code.trim())return false;
  const attempt=this.requiredFactorAttempt();if(!attempt)return false;
  const {state,op,client,readSnapshot}=attempt;
  try{
   const factor=await client.verifyTwoFactor(password,code.trim());
   if(!factor.enabled||factor.pendingEnrolment)throw new CodedError('invalid_response',unreadableServerResponse);
   const snapshot=await readSnapshot();
   if(snapshot.twoStepSetupRequired)throw new CodedError('invalid_response',unreadableServerResponse);
   this.publish({...state,snapshot,phase:'profiles',busy:false,error:undefined});return true;
  }catch(error){if(op.current())this.publish({...state,busy:false,error});return false;}
 }
 /** Canonical return controllers already remembered a signed-in result. Only
  * reread its exact admitted selection; never repeat accept/remember here. */
 private async discardReturn(api:HttpLocalApi,session:LocalSession){
  const pin=api.getRouteConnection()?.pin;
  try{await this.ports.store.connectionStorage(session.viewer.serverId).change(current=>
   current&&pin&&sameServerPin(current.pin,pin)&&current.session.sessionFamilyId===session.sessionFamilyId&&
   current.session.viewer.serverId===session.viewer.serverId&&current.session.viewer.authority===session.viewer.authority&&current.session.viewer.accountId===session.viewer.accountId?undefined:current);}
  finally{try{await this.retire(api,session);}finally{api.getRouteConnection()?.dispose();}}
 }
 private async admittedCompletion({api,result}:AdmittedCompletion,op:ReturnType<BrowserAccountEntry['operation']>){
  if(!op.current()){await this.discardReturn(api,result.session);return;}
  const pin=api.getRouteConnection()?.pin;this.api=api;
  if(!pin)throw new CodedError('invalid_return',unreadableServerResponse);
  const selected=await this.ports.store.selected(),saved=selected?.connection.session;
  if(!op.current()){await this.discardReturn(api,result.session);return;}
  if(selected?.signIn!=='portico'||!sameServerPin(selected.connection.pin,pin)||!saved||saved.sessionFamilyId!==result.session.sessionFamilyId||
    result.snapshot.serverId!==pin.serverId||result.snapshot.account.id!==saved.viewer.accountId||(['serverId','authority','accountId','profileId','role'] as const).some(key=>saved.viewer[key]!==result.session.viewer[key])){await this.discardReturn(api,result.session);throw new CodedError('cancelled','The server selection changed.');}
  this.api=api;this.session=result.session;
  await this.restoreProfiles({pin,session:result.session});
 }
 private async registeredCompletion(completion:RegisteredCompletion,controller:RegisteredController,op:ReturnType<BrowserAccountEntry['operation']>){
  const {api,result}=completion;
  if(result.kind==='signed-in')return this.admittedCompletion({api,result},op);
  if(!op.current()){api.getRouteConnection()?.dispose();return;}
  this.api=api;const pin=api.getRouteConnection()?.pin;
  if(!pin)throw new CodedError('invalid_return',unreadableServerResponse);
  const [server,selection]=await Promise.all([api.request(routePath('get_system'),'GET',undefined,op.signal).then(parseServerSignIn),this.ports.store.read()]);
  if(!op.current()){api.getRouteConnection()?.dispose();return;}
  if(server.id!==pin.serverId||server.signIn!=='portico')throw new CodedError('invalid_return',unreadableServerResponse);
  this.api=api;this.registeredChallenge=controller;this.challengeRevision=selection.revision;
  this.publish({phase:'challenge',server,origin:api.baseUrl,pin,busy:false});this.watchAuthority();
 }
 async preparePorticoSignIn(hostedOrigin:string):Promise<string|undefined>{
  const state=this.state,api=this.api,proof=this.moveProof,ticket=this.moveTicket,moving=state.phase==='move-portico';
  if(!['portico','move-portico'].includes(state.phase)||state.server?.signIn!=='portico'||state.invitation||state.busy||!api||!state.origin||!this.ports.registered)return;
  // A move is never silently retried as ordinary sign-in. Its actual old-
  // password ticket or owner-issued code must accompany the private prepare.
  if(moving&&(state.server.move!=='to_portico'||(!proof&&!ticket)))return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  let preparing:RegisteredController|undefined;
  const checkMove=async()=>{
   if(!moving)return;
   const revision=proof?.revision??ticket!.revision,expiresAt=proof?.preview.expiresAt??ticket!.expiresAt;
   if(!op.current()||Date.parse(expiresAt)<=Date.now())throw new CodedError('move_code_invalid','The move proof expired.');
   if((await this.ports.store.read()).revision!==revision)throw new CodedError('cancelled','The server selection changed.');
   if(!op.current()||Date.parse(expiresAt)<=Date.now())throw new CodedError('cancelled','The account move request ended.');
  };
  try{
   await checkMove();if(!op.current())return;
   const previous=this.registered;this.registered=undefined;if(previous)await previous.cancel();if(!op.current())return;
   await checkMove();if(!op.current())return;
   const flow=this.ports.registered();preparing=this.registered=flow.controller;
   const move=moving?(proof?{moveCode:proof.code}:{moveTicket:ticket!.ticket}):{};
   const url=await flow.controller.prepare(api,state.origin,hostedOrigin,flow.installationId(),move);
   await checkMove();if(!op.current())return;
   this.publish({...state,busy:false,error:undefined});return url;
  }catch(error){
   if(preparing&&this.registered===preparing){this.registered=undefined;await preparing.cancel().catch(()=>{});}
   if(op.current())this.publish({...state,busy:false,error});
  }
 }
 async finishPorticoReturn(captured:CapturedPorticoReturn){
  // Cancel prior entry work BEFORE selecting an owner journal. Calling cancel
  // between pendingReturn and finish would delete that very captured attempt.
  this.cancel();const op=this.operation();this.publish({phase:'checking',busy:true});
  let owner:ContinuationReturnController|undefined;
  try{
   // Only the protected journal selects a continuation. An ambiguous pair of
   // active journals refuses without consuming either one; URL purpose cannot
   // choose setup or confer owner-switch authority.
   const choices:{controller:ContinuationReturnController;pending:NonNullable<Awaited<ReturnType<ContinuationReturnController['pendingReturn']>>>}[]=[];
   for(const create of [this.ports.setupReturn,this.ports.ownerReturn]){
    if(!create)continue;
    const controller=create(),pending=await controller.pendingReturn();
    if(!op.current()){await controller.cancel();return;}
    if(pending)choices.push({controller,pending});
   }
   if(choices.length>1)throw new CodedError('invalid_return',unreadableServerResponse);
   if(choices.length===1){
    const choice=choices[0]!;owner=choice.controller;this.ownerReturn=owner;
    dispatchPorticoReturn(captured,choice.pending);
    this.stopAuthorityWatch();
    await this.admittedCompletion(await owner.finish(captured),op);return;
   }
   if(!this.ports.registered)throw new CodedError('invalid_return',unreadableServerResponse);
   const {controller}=this.ports.registered();this.registered=controller;
   await this.registeredCompletion(await controller.finish(captured),controller,op);
  }catch(error){if(op.current()){
   // The domain preserves a legitimate journal for malformed/mixed callbacks;
   // a valid owner's one-shot exchange is terminal on failure. Do not turn a
   // rejected URL into cancellation of a protected pending attempt.
   if(this.ownerReturn===owner)this.ownerReturn=undefined;
   this.release();this.publish({phase:'welcome',error});
  }}
 }
 async completeChallenge(code:string){
  const registered=this.registeredChallenge;
  if(registered&&this.state.phase==='challenge'&&!this.state.busy){
   const state=this.state,op=this.operation();this.stopAuthorityWatch();this.publish({...state,busy:true,error:undefined});
   try{await this.registeredCompletion(await registered.completeChallenge(code),registered,op);}
   catch(error){if(op.current()){this.publish({...state,busy:false,error});this.watchAuthority();}}return;
  }
  const state=this.state,api=this.api,challenge=this.challenge,revision=this.challengeRevision;if(state.phase!=='challenge'||!api||!challenge||revision===undefined)return;
  const op=this.operation(),hosted=this.hostedChallenge,guard={...op,current:()=>op.current()&&(!hosted||hosted.account.isCurrent())};this.publish({...state,busy:true,error:undefined});
  try{
   if((await this.ports.store.read()).revision!==revision)throw new CodedError('cancelled','The server selection changed. Start this sign-in again.');if(!guard.current())return;
   const result=await completeDirectSignInChallenge(api,challenge.token,code,op.signal);
   if(!guard.current()||hosted&&result.kind==='signed-in'&&result.snapshot.account.hostedAccountId!==hosted.account.accountId){
    if(result.kind==='signed-in')await this.retire(api,result.session);
    if(op.current())throw new CodedError('cancelled','The Portico Account changed.');return;
   }
   await this.accept(result,api,revision,guard,hosted?.hosted);
   if(result.kind==='signed-in'&&this.hostedChallenge===hosted)this.clearHostedChallenge();
  }catch(error){if(op.current()){if(hosted&&!hosted.account.isCurrent())this.cancel();else this.publish({...state,busy:false,error});}}
 }
 async changePassword(currentPassword:string,newPassword:string){
  const state=this.state,api=this.api,session=this.session;if(state.phase!=='password-change'||!api||!session)return;
  const op=this.operation();this.stopAuthorityWatch();this.publish({...state,busy:true,error:undefined});
  try{await changeRequiredPassword(api,currentPassword,newPassword,op.signal);await this.ports.actions.signOutServer(session);if(op.current()){this.session=undefined;this.challenge=undefined;this.challengeRevision=undefined;this.retired();api.setAccessToken('');this.publish({...state,phase:'password',snapshot:undefined,busy:false,error:undefined});}}
  catch(error){if(op.current()){this.publish({...state,busy:false,error});this.watchAuthority();}}
 }
 async loadProfiles(){return this.restoreProfiles();}
 private async restoreProfiles(expected?:Readonly<{pin:ServerPin;session:LocalSession}>){
  this.cancel();const op=this.operation();this.publish({phase:'profiles',busy:true});
  try{
   const entry=await this.ports.store.selected();if(!entry)throw new Error('Choose a server first.');
   const matches=(saved:typeof entry|undefined)=>!expected||!!saved&&saved.signIn==='portico'&&sameServerPin(saved.connection.pin,expected.pin)&&saved.connection.session.sessionFamilyId===expected.session.sessionFamilyId&&(['serverId','authority','accountId','profileId','role'] as const).every(key=>saved.connection.session.viewer[key]===expected.session.viewer[key]);
   if(!matches(entry))throw new CodedError('cancelled','The server selection changed.');
   const restored=await this.ports.restore(entry.connection.session);
   if(!op.current()){restored?.api.getRouteConnection()?.dispose();return;}if(!restored)throw new Error('This server sign-in has ended.');
   this.api=restored.api;this.session=restored.session;
   const [server,snapshot]=await Promise.all([
    restored.api.request(routePath('get_system'),'GET',undefined,op.signal).then(parseServerSignIn),
    restored.api.request(routePath('get_direct_account'),'GET',undefined,op.signal).then(raw=>parseDirectSnapshot(raw,{authority:'local',accountId:restored.session.viewer.accountId,serverId:entry.connection.pin.serverId})),
   ]);
   if(!op.current())return;if(server.id!==entry.connection.pin.serverId||server.signIn!==entry.signIn)throw new Error('The server sign-in changed.');
   if(expected&&!matches(await this.ports.store.selected()))throw new CodedError('cancelled','The server selection changed.');
   if(!op.current())return;
   this.publish({phase:snapshot.twoStepSetupRequired?'two-step-setup':'profiles',server,origin:restored.api.baseUrl,pin:entry.connection.pin,snapshot,busy:false});
   this.watchAuthority();
  }catch(error){if(op.current()){this.release();this.publish({phase:'profiles',error});}}
 }
 async chooseProfile(profileId:string,pin='',remember=false){
  const state=this.state,api=this.api,session=this.session;if(state.phase!=='profiles'||!state.snapshot||state.snapshot.twoStepSetupRequired||!api||!session)return;
  const op=this.operation();this.stopAuthorityWatch();this.publish({...state,busy:true,error:undefined});
  try{const chosen=await this.ports.actions.switchProfile(api,session,profileId,pin,op.signal,op.current,remember);if(op.current()){this.session=chosen;this.challenge=undefined;this.challengeRevision=undefined;this.retired();if(op.current()){this.publish({...state,phase:'ready',busy:false});this.watchAuthority();}}}
  catch(error){if(op.current()){this.publish({...state,busy:false,error});this.watchAuthority();}}
 }
 profileRecovery(profileId:string){
  const api=this.api,state=this.state,snapshot=state.snapshot,session=this.session,epoch=this.epoch;
  if(state.phase!=='profiles'||state.busy||!api||!snapshot||snapshot.twoStepSetupRequired||!state.server||!session)throw new Error('Choose a current server profile first.');
  return new BrowserProfileRecovery(api,snapshot,profileId,state.server.signIn,
   ()=>!this.disposed&&this.epoch===epoch&&this.api===api&&this.state.phase==='profiles'&&this.state.snapshot===snapshot&&this.session?.sessionFamilyId===session.sessionFamilyId,
   grant=>this.temporary(api,grant));
 }

 dispose(){this.cancel();this.disposed=true;this.probe.dispose();this.listeners.clear();this.retiredListeners.clear();}
}
export function createBrowserAccountEntry(options:Pick<EntryPorts,'invitationDestination'>={}){return new BrowserAccountEntry({store:browserAppSessions(),actions:browserAppSessionActions(),environment:browserConnectionEnvironment,restore:restoreBrowserConnection,inspect:inspectDirectServer,connect:connectPairedServer,remember:rememberNativeSession,observeSelection:observeBrowserSessionChanges,registered:()=>createBrowserRegisteredSignIn(buildHostedSignInStartURL),ownerReturn:createBrowserOwnerClaim,setupReturn:createBrowserSetupClaim,setupRequired:(origin)=>location.assign(new URL('/setup',origin).href),rememberName:(api,name)=>browserServerDisplay().remember(api,name),...options});}
