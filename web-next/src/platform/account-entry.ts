import {HttpLocalApi, type LocalSession} from '@core/index.ts';
import {ServerSignInProbe, parseServerSignIn, type ServerSignIn} from '@core/sign-in.ts';
import {directSignIn, completeDirectSignInChallenge, changeRequiredPassword, parseDirectSnapshot, type DirectSignIn, type DirectSnapshot} from '@core/profile-management.ts';
import {inspectDirectServer, connectPairedServer, rememberNativeSession, type ConnectionEnvironment, type RememberedServer} from '@core/server-connections.ts';
import {assertFirstContactIdentity, RouteError, type ServerPin} from '@core/route-identity.ts';
import {knownServerRoutes, type AppSessionStore} from '@core/app-sessions.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {browserAppSessions, browserConnectionEnvironment, restoreBrowserConnection} from './server-connections';
import {browserAppSessionActions, type BrowserAppSessionActions} from './app-sessions';
import {browserServerDisplay} from './server-display';
import {BrowserProfileRecovery} from './profile-recovery';

export type AccountEntryState = Readonly<{
 phase:'welcome'|'address'|'checking'|'password'|'portico'|'challenge'|'password-change'|'profiles'|'ready';
 server?:ServerSignIn; origin?:string; pin?:ServerPin; snapshot?:DirectSnapshot;
 busy?:boolean; error?:unknown;
}>;
type EntryPorts = Readonly<{
 store:AppSessionStore; actions:BrowserAppSessionActions;
 environment:(id?:string)=>ConnectionEnvironment;
 restore:typeof restoreBrowserConnection;
 inspect:typeof inspectDirectServer; connect:typeof connectPairedServer; remember:typeof rememberNativeSession;
 rememberName?:(api:HttpLocalApi,name:string)=>void;
}>;

/** Web account entry keeps credential entry behind an explicit kind and private-key proof.
 * Each operation captures its destination. Abandoning it cannot publish a late credential. */
export class BrowserAccountEntry {
 private state:AccountEntryState=Object.freeze({phase:'welcome'});
 private readonly listeners=new Set<()=>void>();
 private epoch=0;private controller?:AbortController;private api?:HttpLocalApi;private session?:LocalSession;
 private challenge?:Extract<DirectSignIn,{kind:'challenge'}>;private disposed=false;
 readonly probe:ServerSignInProbe;
 constructor(private readonly ports:EntryPorts,probe=new ServerSignInProbe()){this.probe=probe;}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(value:AccountEntryState){if(this.disposed)return;this.state=Object.freeze(value);for(const fn of this.listeners)fn();}
 private operation(){this.controller?.abort();const epoch=++this.epoch,controller=this.controller=new AbortController();return {signal:controller.signal,current:()=>!this.disposed&&epoch===this.epoch&&!controller.signal.aborted};}
 private release(){this.api?.getRouteConnection()?.dispose();this.api=undefined;this.session=undefined;this.challenge=undefined;}
 cancel(){++this.epoch;this.controller?.abort();this.probe.cancel();this.release();this.publish({phase:'welcome'});}
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
  }catch(error){if(op.current()){this.release();this.publish({phase:'address',error});}}
 }
 private async retire(api:HttpLocalApi,session:LocalSession){
  const cleanup=new HttpLocalApi(api.baseUrl,session.accessToken);const route=api.getRouteConnection();if(route)cleanup.useRoutes(route);
  await cleanup.logout().catch(()=>{});
 }
 private async accept(result:DirectSignIn,api:HttpLocalApi,revision:number,op:ReturnType<BrowserAccountEntry['operation']>,hosted?:RememberedServer['hosted']){
  if(result.kind==='challenge'){if(op.current()){this.challenge=result;this.publish({...this.state,phase:'challenge',busy:false});}return;}
  try{
   if(!op.current())throw new Error('Sign-in was cancelled.');
   if(!this.state.server||result.snapshot.serverId!==this.state.server.id)throw new Error('The server identity changed.');
   api.setAccessToken(result.session.accessToken);
   const env={...this.ports.environment(result.snapshot.serverId),storage:this.ports.store.connectionStorage(result.snapshot.serverId,{signIn:this.state.server.signIn,hostedAccount:hosted,expectedRevision:revision})};
   await this.ports.remember(api,result.session,env,hosted,op.current);
   if(!op.current())return;
   this.session=result.session;
   this.publish({...this.state,snapshot:result.snapshot,busy:false,error:undefined,phase:result.passwordChangeRequired?'password-change':result.accountScoped?'profiles':'ready'});
  }catch(error){await this.retire(api,result.session);throw error;}
 }
 async signIn(username:string,password:string){
  const state=this.state,api=this.api;
  if(state.phase!=='password'||state.server?.signIn!=='local'||!api||!username||!password)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{const revision=(await this.ports.store.read()).revision;if(!op.current())return;await this.accept(await directSignIn(api,username,password,op.signal),api,revision,op);}
  catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }
 async completeChallenge(code:string){
  const state=this.state,api=this.api,challenge=this.challenge;if(state.phase!=='challenge'||!api||!challenge)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{const revision=(await this.ports.store.read()).revision;if(!op.current())return;await this.accept(await completeDirectSignInChallenge(api,challenge.token,code,op.signal),api,revision,op);}
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
 async chooseProfile(profileId:string,pin='',remember=false){
  const state=this.state,api=this.api,session=this.session;if(state.phase!=='profiles'||!state.snapshot||!api||!session)return;
  const op=this.operation();this.publish({...state,busy:true,error:undefined});
  try{await this.ports.actions.switchProfile(api,session,profileId,pin,op.signal,op.current,remember);if(op.current())this.publish({...state,phase:'ready',busy:false});}
  catch(error){if(op.current())this.publish({...state,busy:false,error});}
 }
 profileRecovery(profileId:string){
  const api=this.api,state=this.state,snapshot=state.snapshot,epoch=this.epoch;
  if(state.phase!=='profiles'||!api||!snapshot||!state.server)throw new Error('Choose a current server profile first.');
  return new BrowserProfileRecovery(api,snapshot,profileId,state.server.signIn,()=>!this.disposed&&this.epoch===epoch&&this.api===api&&this.state.snapshot===snapshot);
 }
 dispose(){this.cancel();this.disposed=true;this.probe.dispose();this.listeners.clear();}
}
export function createBrowserAccountEntry(){return new BrowserAccountEntry({store:browserAppSessions(),actions:browserAppSessionActions(),environment:browserConnectionEnvironment,restore:restoreBrowserConnection,inspect:inspectDirectServer,connect:connectPairedServer,remember:rememberNativeSession,rememberName:(api,name)=>browserServerDisplay().remember(api,name)});}
