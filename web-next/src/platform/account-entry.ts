import {HttpLocalApi, type LocalSession} from '@core/index.ts';
import {ServerSignInProbe, parseServerSignIn, type ServerSignIn} from '@core/sign-in.ts';
import {directSignIn, completeDirectSignInChallenge, changeRequiredPassword, parseDirectSnapshot, type DirectSignIn, type DirectSnapshot} from '@core/profile-management.ts';
import {inspectDirectServer, connectPairedServer, connectPairedPorticoServer, rememberNativeSession, type ConnectionEnvironment, type RememberedServer} from '@core/server-connections.ts';
import {assertFirstContactIdentity, RouteError, type ServerPin} from '@core/route-identity.ts';
import {knownServerRoutes, type AppSessionStore} from '@core/app-sessions.ts';
import {acceptPorticoInvitation, type HostedRequestApi} from '@core/portico-servers.ts';
import type {JoinLink} from '@core/join-link.ts';
import {passwordStrength} from '@core/password-strength.ts';
import {requestOwnerResetFile, verifyOwnerResetFile, verifyMemberResetCode, changeResetPassword, type ResetFileLocation} from '@core/account-reset.ts';
import {MemberAccountMoveClient, type MoveCodePreview} from '@core/account-move.ts';
import {CodedError} from '@core/server-messages.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {decodePreviewInvitationResponse, decodePostAccessInvitationsAcceptResponse} from '../../../packages/contracts/src/decoders.ts';
import type {PreviewInvitationResponse} from '../../../packages/contracts/src/types.ts';
import {browserAppSessions, browserConnectionEnvironment, restoreBrowserConnection} from './server-connections';
import {browserAppSessionActions, type BrowserAppSessionActions} from './app-sessions';
import {browserServerDisplay} from './server-display';

export type AccountEntryState = Readonly<{
 phase:'welcome'|'address'|'checking'|'password'|'portico'|'challenge'|'password-change'|'profiles'|'ready'|'joined'|'move-code'|'move-local'|'move-portico';
 server?:ServerSignIn; origin?:string; pin?:ServerPin; snapshot?:DirectSnapshot;
 invitation?:PreviewInvitationResponse; joined?:Readonly<{origin:string;signIn:'local'|'portico'}>;
 movePreview?:MoveCodePreview;
 busy?:boolean; error?:unknown;
}>;
type EntryPorts = Readonly<{
 store:AppSessionStore; actions:BrowserAppSessionActions;
 environment:(id?:string)=>ConnectionEnvironment;
 restore:typeof restoreBrowserConnection;
 inspect:typeof inspectDirectServer; connect:typeof connectPairedServer; remember:typeof rememberNativeSession;
 connectPortico?:typeof connectPairedPorticoServer;
 retire?:(api:HttpLocalApi,session:LocalSession)=>Promise<void>;
 temporary?:(source:HttpLocalApi,session:LocalSession)=>HttpLocalApi;
 invitationDestination?:'adopt'|'server-origin';
 rememberName?:(api:HttpLocalApi,name:string)=>void;
}>;

/** Web account entry keeps credential entry behind an explicit kind and private-key proof.
 * Each operation captures its destination. Abandoning it cannot publish a late credential. */
export class BrowserAccountEntry {
 private state:AccountEntryState=Object.freeze({phase:'welcome'});
 private readonly listeners=new Set<()=>void>();
 private epoch=0;private controller?:AbortController;private api?:HttpLocalApi;private session?:LocalSession;
 private challenge?:Extract<DirectSignIn,{kind:'challenge'}>;private challengeRevision?:number;private disposed=false;
 private invitationCode?:string;private joinedAccountId?:string;private joinedUsername?:string;
 private moveProof?:Readonly<{code:string;revision:number;preview:MoveCodePreview}>;
 readonly probe:ServerSignInProbe;
 constructor(private readonly ports:EntryPorts,probe=new ServerSignInProbe()){this.probe=probe;}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(value:AccountEntryState){if(this.disposed)return;this.state=Object.freeze(value);for(const fn of this.listeners)fn();}
 private operation(){this.controller?.abort();const epoch=++this.epoch,controller=this.controller=new AbortController();return {signal:controller.signal,current:()=>!this.disposed&&epoch===this.epoch&&!controller.signal.aborted};}
 private release(){this.api?.getRouteConnection()?.dispose();this.api=undefined;this.session=undefined;this.challenge=undefined;this.challengeRevision=undefined;this.moveProof=undefined;}
 cancel(){++this.epoch;this.controller?.abort();this.probe.cancel();this.release();this.invitationCode=undefined;this.joinedAccountId=undefined;this.joinedUsername=undefined;this.publish({phase:'welcome'});}
 address(){this.cancel();this.publish({phase:'address'});}
 async inspect(address:string,linkId?:string){
  this.cancel();const op=this.operation();this.publish({phase:'checking'});
  try{
   const server=await this.probe.inspect(address);if(!op.current())return;
   if(!server)throw this.probe.getSnapshot().error??new Error('The server did not answer.');
   const origin=this.probe.getSnapshot().origin!;
   const pin=await this.ports.inspect(origin,this.ports.environment(server.id),op.signal);
   if(!op.current())return;if(pin.serverId!==server.id)throw new RouteError('identity_mismatch','The server identity changed.');
   assertFirstContactIdentity(origin,pin,{linkId,knownRoutes:knownServerRoutes(await this.ports.store.read())});
   if(!op.current())return;if(server.setupRequired)throw new RouteError('setup_required','This server needs setup.');
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
  if(result.kind==='challenge'){if(op.current()){this.challenge=result;this.challengeRevision=revision;this.publish({...this.state,phase:'challenge',busy:false});}return;}
  try{
   if(!op.current())throw new Error('Sign-in was cancelled.');
   if(!this.state.server||result.snapshot.serverId!==this.state.server.id)throw new Error('The server identity changed.');
   if(this.joinedAccountId&&result.snapshot.account.id!==this.joinedAccountId)throw new CodedError('identity_mismatch','The invitation account changed.');
   if(this.state.invitation&&this.ports.invitationDestination==='server-origin'){
    await this.retire(api,result.session);
    if(op.current())this.publish({...this.state,phase:'joined',joined:{origin:this.state.origin!,signIn:this.state.server.signIn},busy:false,error:undefined});
    this.invitationCode=undefined;return;
   }
   api.setAccessToken(result.session.accessToken);
   const env={...this.ports.environment(result.snapshot.serverId),storage:this.ports.store.connectionStorage(result.snapshot.serverId,{signIn:this.state.server.signIn,hostedAccount:hosted,expectedRevision:revision})};
   await this.ports.remember(api,result.session,env,hosted,op.current);
   if(!op.current())return;
   this.session=result.session;
   this.publish({...this.state,snapshot:result.snapshot,busy:false,error:undefined,phase:result.passwordChangeRequired?'password-change':result.accountScoped?'profiles':'ready'});
  }catch(error){await this.retire(api,result.session);api.setAccessToken('');throw error;}
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
 async previewMoveCode(code:string){
  const state=this.state,api=this.api;
  if(!api||!state.server||state.invitation||state.busy||!['password','portico','move-code','move-local','move-portico'].includes(state.phase))return;
  const op=this.operation();this.moveProof=undefined;this.publish({...state,phase:'move-code',movePreview:undefined,busy:true,error:undefined});
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
 /** Code proof issues only a restricted transient family. Set its password,
  * retire it, then use ordinary sign-in and the existing durable admission. */
 async resetLocalPassword(proof:Readonly<{kind:'owner';code:string;turnOffTwoStep?:boolean}|{kind:'member';username:string;code:string}>,newPassword:string,secondFactorCode=''){
  const state=this.state,api=this.api;
  if(state.phase!=='password'||state.server?.signIn!=='local'||state.invitation||!api||state.busy||!passwordStrength(newPassword).acceptable)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  let restricted:Extract<DirectSignIn,{kind:'signed-in'}>|undefined;
  try{
   const revision=(await this.ports.store.read()).revision;if(!op.current())return;
   const result=proof.kind==='owner'?await verifyOwnerResetFile(api,proof.code,proof.turnOffTwoStep===true,op.signal):await verifyMemberResetCode(api,proof.username,proof.code,op.signal);
   if(result.kind!=='signed-in')throw new CodedError('invalid_response','The reset did not return a password-change session.');
   restricted=result;
   if(!op.current())return;
   if(result.snapshot.serverId!==state.server.id||result.resetFileOwnerSession||!result.passwordChangeRequired||result.session.refreshToken||
     proof.kind==='owner'&&result.snapshot.account.role!=='owner'||proof.kind==='member'&&result.snapshot.account.role==='owner')throw new CodedError('invalid_response','The reset did not match this account.');
   await changeResetPassword(this.temporary(api,result.session),newPassword,secondFactorCode,op.signal);
   await this.retire(api,result.session);restricted=undefined;
   if(!op.current())return;
   await this.accept(await directSignIn(api,result.snapshot.account.username,newPassword,op.signal),api,revision,op);
  }catch(error){if(op.current())this.publish({...state,busy:false,error});}
  finally{if(restricted)await this.retire(api,restricted.session);}
 }
 async completeChallenge(code:string){
  const state=this.state,api=this.api,challenge=this.challenge,revision=this.challengeRevision;if(state.phase!=='challenge'||!api||!challenge||revision===undefined)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{if((await this.ports.store.read()).revision!==revision)throw new CodedError('cancelled','The server selection changed. Start this sign-in again.');if(!op.current())return;await this.accept(await completeDirectSignInChallenge(api,challenge.token,code,op.signal),api,revision,op);}
  catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }
 async changePassword(currentPassword:string,newPassword:string){
  const state=this.state,api=this.api,session=this.session;if(state.phase!=='password-change'||!api||!session)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{await changeRequiredPassword(api,currentPassword,newPassword,op.signal);await this.ports.actions.signOutServer(session);if(op.current())this.publish({...state,phase:'password',snapshot:undefined,busy:false,error:undefined});}
  catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }
 async loadProfiles(){
  this.cancel();const op=this.operation();this.publish({phase:'profiles',busy:true});
  try{
   const entry=await this.ports.store.selected();if(!entry)throw new Error('Choose a server first.');
   const restored=await this.ports.restore(entry.connection.session);
   if(!op.current()){restored?.api.getRouteConnection()?.dispose();return;}if(!restored)throw new Error('This server sign-in has ended.');
   this.api=restored.api;this.session=restored.session;
   const [server,snapshot]=await Promise.all([
    restored.api.request(routePath('get_system'),'GET',undefined,op.signal).then(parseServerSignIn),
    restored.api.request(routePath('get_direct_account'),'GET',undefined,op.signal).then(raw=>parseDirectSnapshot(raw,{authority:'local',accountId:restored.session.viewer.accountId,serverId:entry.connection.pin.serverId})),
   ]);
   if(!op.current())return;if(server.id!==entry.connection.pin.serverId||server.signIn!==entry.signIn)throw new Error('The server sign-in changed.');
   this.publish({phase:'profiles',server,snapshot,busy:false});
  }catch(error){if(op.current()){this.release();this.publish({phase:'profiles',error});}}
 }
 async chooseProfile(profileId:string,pin=''){
  const state=this.state,api=this.api,session=this.session;if(state.phase!=='profiles'||!state.snapshot||!api||!session)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{await this.ports.actions.switchProfile(api,session,profileId,pin,op.signal,op.current);if(op.current())this.publish({...state,phase:'ready',busy:false});}
  catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }

 dispose(){this.cancel();this.disposed=true;this.probe.dispose();this.listeners.clear();}
}
export function createBrowserAccountEntry(options:Pick<EntryPorts,'invitationDestination'>={}){return new BrowserAccountEntry({store:browserAppSessions(),actions:browserAppSessionActions(),environment:browserConnectionEnvironment,restore:restoreBrowserConnection,inspect:inspectDirectServer,connect:connectPairedServer,remember:rememberNativeSession,rememberName:(api,name)=>browserServerDisplay().remember(api,name),...options});}
