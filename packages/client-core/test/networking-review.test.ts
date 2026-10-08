import test from 'node:test';
import assert from 'node:assert/strict';
import {createHash,generateKeyPairSync,sign} from 'node:crypto';
import {ApiError,HttpLocalApi,type LocalSession} from '../src/index.ts';
import {RouteConnection,sessionRoute} from '../src/route-connection.ts';
import {type ServerPin} from '../src/route-identity.ts';
import {parseRememberedServer,rememberNativeSession,restoreNativeSession,rememberRotatedSession,prepareRoutedSession,commitRoutedSession,type RememberedServer,type ServerConnectionStorage} from '../src/server-connections.ts';
import {SetupCodeService,type SetupCodeJournal} from '../src/setup-code.ts';
import {SessionSelectionService} from '../src/session-selection.ts';
const tick=()=>new Promise<void>(r=>setTimeout(r,5));
async function until(check:()=>boolean){for(let i=0;i<120&&!check();i++)await tick();assert.ok(check(),'condition did not settle');}
function deferred<T>(){let resolve!:(v:T)=>void,reject!:(e:unknown)=>void;const promise=new Promise<T>((a,b)=>{resolve=a;reject=b;});return {promise,resolve,reject};}
const keys=generateKeyPairSync('ed25519'),rawKey=keys.publicKey.export({format:'der',type:'spki'}).subarray(-32);
const pin:ServerPin={publicKey:rawKey.toString('base64url'),fingerprint:createHash('sha256').update(rawKey).digest('base64url'),serverId:'srv_'+createHash('sha256').update('portico.server.identity.v1\0').update(rawKey).digest('base64url')};
const lan='http://192.168.1.8:32500',other='http://192.168.1.9:32500';
const signed=(v:unknown)=>{const raw=Buffer.from(JSON.stringify(v));return{payload:raw.toString('base64url'),signature:sign(null,raw,keys.privateKey).toString('base64url')};};
function session(token='token_one',generation='1'):LocalSession {const until=new Date(Date.now()+600000).toISOString();return{accessToken:token,tokenGeneration:generation,sessionFamilyId:'same_family',expiresAt:until,authorizationHorizon:until,serverIdentity:{publicKey:pin.publicKey,fingerprint:pin.fingerprint},viewer:{serverId:pin.serverId,accountId:'owner',profileId:'profile',role:'owner',authority:'local'}};}
function storage(){let value:RememberedServer|undefined;return{read:async()=>structuredClone(value),change:async(update:(v:RememberedServer|undefined)=>RememberedServer|undefined)=>{value=structuredClone(update(structuredClone(value)));}} satisfies ServerConnectionStorage;}
async function ready(fetcher?:typeof fetch){const connection=new RouteConnection({pin,logicalOrigin:lan,memory:{paired:[lan]},probe:async()=>pin,fetcher});await connection.resolve();const api=new HttpLocalApi(lan,'',fetcher);api.useRoutes(connection);return{connection,api};}

test('delayed old mutation failure cannot invalidate a newer proved route or replay the mutation',async()=>{
 const failed=deferred<Response>();let calls=0,proofs=0;const connection=new RouteConnection({pin,logicalOrigin:lan,memory:{paired:[lan]},probe:async()=>{proofs++;return pin;},fetcher:async()=>{calls++;return failed.promise;}});
 try{await connection.resolve();const request=connection.fetch(lan+'/v1/change',{method:'POST'});const rejection=assert.rejects(request);await until(()=>calls===1);connection.networkChanged('wifi-2');await connection.resolve();const revision=connection.getSnapshot().revision,checks=proofs;failed.reject(new Error('old socket failed'));await rejection;assert.equal(calls,1);assert.equal(proofs,checks);assert.equal(connection.getSnapshot().revision,revision);assert.equal(connection.getSnapshot().phase,'ready');}finally{connection.dispose();}
});
test('read failure reuses newer proof without starting a third generation',async()=>{
 const failed=deferred<Response>();let calls=0,proofs=0;const connection=new RouteConnection({pin,logicalOrigin:lan,memory:{paired:[lan]},probe:async()=>{proofs++;return pin;},fetcher:async()=>++calls===1?failed.promise:new Response('ok')});
 try{await connection.resolve();const request=connection.fetch(lan+'/v1/me');await until(()=>calls===1);connection.networkChanged('wifi-2');await connection.resolve();const checks=proofs;failed.reject(new Error('lost read'));assert.equal(await(await request).text(),'ok');assert.equal(calls,2);assert.equal(proofs,checks);}finally{connection.dispose();}
});
test('network change between resolved promise and dispatch fences the credential-bearing request',async()=>{
 let proofGate:ReturnType<typeof deferred<ServerPin>>|undefined;const sent:string[]=[];const connection=new RouteConnection({pin,logicalOrigin:lan,memory:{paired:[lan]},probe:async()=>proofGate?proofGate.promise:pin,fetcher:async input=>{sent.push(String(input));return new Response('ok');}});
 try{await connection.resolve();proofGate=deferred<ServerPin>();const request=connection.fetch(lan+'/v1/me',{headers:{Authorization:'Bearer secret'}});connection.networkChanged('changed-synchronously');await tick();assert.deepEqual(sent,[]);proofGate.resolve(pin);await request;assert.equal(sent.length,1);}finally{connection.dispose();}
});
test('Hosted Retry-After survives recovery attempts without blocking cached LAN proofs',async()=>{
 let fresh=0,available=false,proofs=0;const connection=new RouteConnection({pin,logicalOrigin:lan,memory:{paired:[lan]},probe:async()=>{proofs++;if(!available)throw new Error('offline');return pin;},fresh:async()=>{fresh++;throw new ApiError(429,'rate_limited','wait',true,60);}});
 try{await assert.rejects(connection.resolve());assert.equal(fresh,1);await assert.rejects(connection.recover());assert.equal(fresh,1);available=true;await connection.recover();assert.equal(connection.getSnapshot().phase,'ready');assert.ok(proofs>=3);assert.equal(fresh,1);}finally{connection.dispose();}
});
test('same-family late selection rollback does not replace a newer saved selection',async()=>{
 const base=storage(),ack=deferred<void>();let first=true,committed=false,current=true;const delayed:ServerConnectionStorage={read:base.read,change:async update=>{await base.change(update);if(first){first=false;committed=true;await ack.promise;}}};const a=await ready(),b=await ready();
 try{const old=rememberNativeSession(a.api,session(),{storage:delayed},undefined,()=>current);const rejection=assert.rejects(old);await until(()=>committed);current=false;await rememberNativeSession(b.api,session(),{storage:base});const fresh=await base.read();ack.resolve();await rejection;assert.deepEqual(await base.read(),fresh);assert.equal(sessionRoute('token_one'),b.connection);}finally{a.connection.dispose();b.connection.dispose();}
});
test('old cache owner cannot write over a newer selection of the same family',async()=>{
 const store=storage(),a=await ready(),b=await ready();try{await rememberNativeSession(a.api,session(),{storage:store});await rememberNativeSession(b.api,session(),{storage:store});const fresh=await store.read();a.connection.rememberPaired(other);await a.connection.flush();assert.deepEqual(await store.read(),fresh);}finally{a.connection.dispose();b.connection.dispose();}
});
test('restore rechecks durable selection after proof, before sending a bearer',async()=>{
 const store=storage(),initial=await ready(),proof=deferred<void>();let requested=false,me=0;
 try{await rememberNativeSession(initial.api,session(),{storage:store});const fetcher:typeof fetch=async(input,init)=>{if(String(input).endsWith('/v1/networking/identity-proof')){requested=true;await proof.promise;const q=JSON.parse(String(init?.body));return new Response(JSON.stringify(signed({kind:'portico.route-proof',version:'1',...pin,...q,issuedAt:new Date().toISOString(),expiresAt:new Date(Date.now()+29000).toISOString()})),{headers:{'Content-Type':'application/json'}});}me++;return new Response(JSON.stringify({viewer:session().viewer}),{headers:{'Content-Type':'application/json'}});};const restored=restoreNativeSession({storage:store,fetcher});const rejection=assert.rejects(restored);await until(()=>requested);await store.change(()=>undefined);proof.resolve();await rejection;assert.equal(me,0);}finally{initial.connection.dispose();}
});
test('renewal projection keeps generation monotonic and retains exactly the routed family',async()=>{
 const store=storage(),a=await ready();try{await rememberNativeSession(a.api,session(),{storage:store});await rememberRotatedSession({storage:store},session('token_two','2'));assert.equal(sessionRoute('token_two'),a.connection);await rememberRotatedSession({storage:store},session('token_one','1'));assert.equal((await store.read())?.session.accessToken,'token_two');await assert.rejects(rememberRotatedSession({storage:store},session('conflicting','2')));}finally{a.connection.dispose();}
});
test('prepared approved grant remains retryable after atomic storage failure',async()=>{
 const store=storage(),a=await ready();let fail=true;const flaky:ServerConnectionStorage={read:store.read,change:async update=>{if(fail)throw new Error('keychain unavailable');await store.change(update);}};
 try{const grant=session('prepared');prepareRoutedSession(a.api,grant);await assert.rejects(commitRoutedSession(grant,{storage:flaky}));fail=false;assert.equal(await commitRoutedSession(grant,{storage:flaky}),a.api);assert.equal((await store.read())?.session.accessToken,'prepared');}finally{a.connection.dispose();}
});
const requestId='11111111-1111-4111-8111-111111111111';
const authorization=()=>({deviceCode:'s'.repeat(43),userCode:'ABCD-EFGH',verificationUri:'https://web.getportico.tv/device',verificationUriComplete:'https://web.getportico.tv/device#code=ABCD-EFGH',expiresAt:new Date(Date.now()+600000).toISOString(),expiresIn:600,interval:5});
function journal<T>(initial?:SetupCodeJournal<T>){let value=structuredClone(initial);return{read:()=>structuredClone(value),change:async(update:(v:SetupCodeJournal<T>|undefined)=>SetupCodeJournal<T>)=>{value=structuredClone(update(structuredClone(value)));return structuredClone(value);}};}
const opts={tickMs:5,requestTimeoutMs:50,replaceBeforeMs:60000};
test('pause and resume do not orphan an in-flight idempotent code creation',async()=>{
 const pending=deferred<ReturnType<typeof authorization>>(),store=journal<string>();let calls=0;const service=new SetupCodeService('local:one',{create:async()=>{calls++;return pending.promise;},poll:async()=>'',cancel:async()=>{}},store,async()=>requestId,{...opts,requestTimeoutMs:1000});
 try{await service.start();await until(()=>calls===1);service.pause();await service.start();pending.resolve(authorization());await until(()=>service.getSnapshot().phase==='waiting');assert.equal(calls,1);assert.equal(store.read()?.current?.requestId,requestId);assert.equal(store.read()?.retired.length,0);}finally{service.pause();}
});
test('late poll checkpoint cannot persist a grant after the bounded poll expired',async()=>{
 const store=journal<string>({version:1,authority:'local:one',revision:1,current:{requestId,authorization:authorization(),nextPollAt:0},retired:[]});let checkpoint:((g:string)=>Promise<void>)|undefined,signal:AbortSignal|undefined;
 const service=new SetupCodeService('local:one',{create:async()=>authorization(),poll:async(_entry,s,c)=>{signal=s;checkpoint=c;return new Promise(()=>{});},cancel:async()=>{}},store,async()=>requestId,opts);
 try{await service.start();await until(()=>!!signal?.aborted);await assert.rejects(checkpoint!('late-secret'));assert.equal(store.read()?.current?.grant,undefined);assert.notEqual(service.getSnapshot().phase,'approved');}finally{service.pause();}
});
test('approved adoption survives storage JSON copies and parent unmount acknowledgement',async()=>{
 const grant={family:'one'},store=journal<typeof grant>({version:1,authority:'local:one',revision:1,current:{requestId,authorization:authorization(),nextPollAt:0,grant},retired:[]});
 const service=new SetupCodeService('local:one',{create:async()=>authorization(),poll:async()=>grant,cancel:async()=>{}},store,async()=>requestId,opts);
 try{await service.start();const offered=service.getSnapshot().grant!;assert.ok(service.isCurrent(offered));service.pause();assert.equal(service.isCurrent(offered),false);assert.equal(await service.complete(offered),true);assert.equal(store.read()?.current,undefined);}finally{service.pause();}
});
test('cancelling a lost create reconciles the SAME request before retiring its secret',async()=>{
 const store=journal<string>({version:1,authority:'local:one',revision:1,creating:{requestId,startedAt:Date.now()},retired:[]});const created:string[]=[],cancelled:string[]=[];
 const service=new SetupCodeService('local:one',{create:async id=>{created.push(id);return authorization();},poll:async()=>'',cancel:async entry=>{cancelled.push(entry.requestId);}},store,async()=>{throw new Error('must not allocate another request');},opts);
 // Cancellation loads the protected journal even before a screen is started.
 await service.cancel();assert.deepEqual(created,[requestId]);assert.deepEqual(cancelled,[requestId]);assert.equal(store.read()?.creating,undefined);assert.equal(store.read()?.retired.length,0);
});
test('unobserved authorized server remains in the chooser without fabricated routes',async()=>{
 const selection=new SessionSelectionService({account:{accountId:'account',sessionId:'session'},api:{listServers:async()=>({items:[{id:pin.serverId,name:'Not observed yet',publicKey:pin.publicKey,policyRevision:1,baseUrl:''}]}),serverProfiles:async()=>({}),attach:async()=>session(),revokeLocal:async()=>{}},onSelected:()=>{}});
 try{await selection.loadServers();assert.equal(selection.getSnapshot().phase,'servers');assert.equal(selection.getSnapshot().servers.length,1);assert.equal(selection.getSnapshot().servers[0].routes,undefined);}finally{selection.dispose();}
});

test('route recovery cannot adopt a different tab or device selected profile',async()=>{
 const vault=storage(),{connection,api}=await ready();const saved=session();await rememberNativeSession(api,saved,{storage:vault});
 let probes=0;const env={storage:vault,fetcher:async()=>{probes++;throw new Error('must not send credentials for another selected viewer');}};
 const restored=await restoreNativeSession(env,undefined,{...saved,accessToken:'other_tab_token'});
 assert.equal(restored,undefined);assert.equal(probes,0);connection.dispose();
});

test('identity rejection expires without relaxing the server pin or sending a bearer',async t=>{
 const {RouteError}=await import('../src/route-identity.ts');
 let now=Date.now(),valid=false,proofs=0,requests=0;
 t.mock.method(Date,'now',()=>now);
 const connection=new RouteConnection({pin,logicalOrigin:lan,memory:{paired:[lan]},probe:async(_url,expected)=>{proofs++;assert.deepEqual(expected,pin);if(!valid)throw new RouteError('identity_mismatch','wrong server');return pin;},fetcher:async()=>{requests++;return new Response('ok');}});
 try {
  await assert.rejects(connection.fetch(lan+'/v1/me',{headers:{Authorization:'Bearer secret'}}));
  assert.equal(requests,0);assert.equal(proofs,1);
  valid=true;
  await assert.rejects(connection.recover());assert.equal(proofs,1);assert.equal(connection.getSnapshot().phase,'identity_mismatch');
  now+=5001;
  await connection.recover();await connection.fetch(lan+'/v1/me',{headers:{Authorization:'Bearer secret'}});
  assert.equal(proofs,2);assert.equal(requests,1);
 } finally {connection.dispose();}
});
test('a meaningful network transition allows a fresh pinned proof for rejected Bonjour routes',async()=>{
 const {RouteError}=await import('../src/route-identity.ts');
 let valid=false,proofs=0;
 const connection=new RouteConnection({pin,logicalOrigin:lan,discover:async()=>[{baseUrl:lan,serverId:pin.serverId,fingerprint:pin.fingerprint,port:32500,path:'/',expiresAt:Date.now()+60000}],probe:async(_url,expected)=>{proofs++;assert.deepEqual(expected,pin);if(!valid)throw new RouteError('identity_mismatch','wrong server');return pin;}});
 try {await assert.rejects(connection.resolve());valid=true;connection.networkChanged('wifi-returned');assert.equal(await connection.resolve(),lan);assert.equal(proofs,2);}finally{connection.dispose();}
});

test('restore renews an access token the server no longer accepts instead of losing the sign-in',async t=>{
 const {setInstallationIdentity}=await import('../src/installation.ts');
 setInstallationIdentity({installationId:'inst_restore_'+'x'.repeat(24),name:'Test',platform:'ios',app:'portico-apple',appVersion:'1.0.0'});t.after(()=>setInstallationIdentity(undefined));
 const store=storage(),initial=await ready();
 const saved={...session('token_one','1'),refreshToken:'refresh_'+'a'.repeat(24)};
 const renewed={...session('token_two','2'),refreshToken:'refresh_'+'b'.repeat(24)};
 let refreshes=0;const bearers:string[]=[];
 try{
  await rememberNativeSession(initial.api,saved,{storage:store});
  const json=(status:number,body:unknown)=>new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}});
  const fetcher:typeof fetch=async(input,init)=>{
   const url=String(input);
   if(url.endsWith('/v1/networking/identity-proof')){const q=JSON.parse(String(init?.body));return json(200,signed({kind:'portico.route-proof',version:'1',...pin,...q,issuedAt:new Date().toISOString(),expiresAt:new Date(Date.now()+29000).toISOString()}));}
   if(url.endsWith('/v1/auth/refresh')){refreshes++;return json(200,renewed);}
   const bearer=new Headers(init?.headers).get('authorization')??'';bearers.push(bearer);
   // The server restarted: the old access token is refused although it has not expired.
   return bearer==='Bearer token_two'?json(200,{viewer:saved.viewer}):json(401,{error:{code:'unauthorized'}});
  };
  const restored=await restoreNativeSession({storage:store,fetcher});
  assert.equal(restored?.session.accessToken,'token_two');assert.equal(refreshes,1);
  assert.deepEqual(bearers,['Bearer token_one','Bearer token_two']);
  assert.equal((await store.read())?.session.refreshToken,renewed.refreshToken);
  restored?.api.getRouteConnection()?.dispose();
 }finally{initial.connection.dispose();}
});

test('a malformed renewal (400) keeps the saved credential; only a refusal (401) ends it',async t=>{
 const {setInstallationIdentity}=await import('../src/installation.ts');const {refreshStoredSession}=await import('../src/server-connections.ts');
 setInstallationIdentity({installationId:'inst_renew_'+'x'.repeat(24),name:'Test',platform:'ios',app:'portico-apple',appVersion:'1.0.0'});t.after(()=>setInstallationIdentity(undefined));
 for(const [status,kept] of [[400,true],[401,false]] as const){
  // The saved token is bound to its route, so the server's answer comes through that route.
  const answer:typeof fetch=async()=>new Response(JSON.stringify({error:{code:'x'}}),{status,headers:{'Content-Type':'application/json'}});
  const store=storage(),initial=await ready(answer);
  try{
   await rememberNativeSession(initial.api,{...session('token_one','1'),refreshToken:'refresh_'+'a'.repeat(24)},{storage:store});
   // The access token has expired since, so renewal is due.
   await store.change(v=>v&&{...v,session:{...v.session,expiresAt:new Date(Date.now()-1000).toISOString()}});
   const api=new HttpLocalApi(lan,'token_one',answer);api.useRoutes(initial.connection);
   await assert.rejects(refreshStoredSession({env:{storage:store},api,family:{sessionFamilyId:'same_family',serverId:pin.serverId}}),(e:{status?:number})=>e.status===status);
   assert.equal(!!(await store.read()),kept,`status ${status}`);
  }finally{initial.connection.dispose();}
 }
});

async function restoreAgainst(t:import('node:test').TestContext,meAnswer:(bearer:string)=>number){
 const {setInstallationIdentity}=await import('../src/installation.ts');
 setInstallationIdentity({installationId:'inst_refused_'+'x'.repeat(24),name:'Test',platform:'ios',app:'portico-apple',appVersion:'1.0.0'});t.after(()=>setInstallationIdentity(undefined));
 const store=storage(),initial=await ready();
 const saved={...session('token_one','1'),refreshToken:'refresh_'+'a'.repeat(24)};
 const renewed={...session('token_two','2'),refreshToken:'refresh_'+'b'.repeat(24)};
 const calls={refresh:0,me:0};
 await rememberNativeSession(initial.api,saved,{storage:store});
 const json=(status:number,body:unknown)=>new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}});
 const fetcher:typeof fetch=async(input,init)=>{
  const url=String(input);
  if(url.endsWith('/v1/networking/identity-proof')){const q=JSON.parse(String(init?.body));return json(200,signed({kind:'portico.route-proof',version:'1',...pin,...q,issuedAt:new Date().toISOString(),expiresAt:new Date(Date.now()+29000).toISOString()}));}
  if(url.endsWith('/v1/auth/refresh')){calls.refresh++;return json(200,renewed);}
  calls.me++;const status=meAnswer(new Headers(init?.headers).get('authorization')??'');
  return status===200?json(200,{viewer:saved.viewer}):json(status,{error:{code:'x'}});
 };
 const outcome=await restoreNativeSession({storage:store,fetcher}).then(r=>{r?.api.getRouteConnection()?.dispose();return 'restored';},(e:{code?:string})=>e.code);
 initial.connection.dispose();
 return {outcome,calls,stored:await store.read()};
}
test('a 403 from /me is access refused: no renewal, the credential stays',async t=>{
 const r=await restoreAgainst(t,()=>403);
 assert.equal(r.outcome,'access_refused');assert.equal(r.calls.refresh,0);assert.equal(r.calls.me,1);assert.equal(r.stored?.session.refreshToken,'refresh_'+'a'.repeat(24));
});
test('a second 401 after a successful renewal is access refused, not offline',async t=>{
 const r=await restoreAgainst(t,()=>401);
 assert.equal(r.outcome,'access_refused');assert.equal(r.calls.refresh,1);assert.equal(r.calls.me,2);assert.ok(r.stored?.session.refreshToken);
});

test('restore, Try again and the background refresher at once send one refresh and never reuse a token',async t=>{
 const {setInstallationIdentity}=await import('../src/installation.ts');const {LocalSessionRefresher}=await import('../src/session-refresh.ts');
 setInstallationIdentity({installationId:'inst_race_'+'x'.repeat(24),name:'Test',platform:'ios',app:'portico-apple',appVersion:'1.0.0'});t.after(()=>setInstallationIdentity(undefined));
 // The saved access token expired while the server was down; the refresh token is still good.
 const saved={...session('access_1','1'),refreshToken:'refresh_'+'1'.repeat(24)};
 // A fake server that rotates on the current refresh token and counts any predecessor as reuse.
 let current='refresh_'+'1'.repeat(24),generation=1,refreshes=0,reuse=0;const live=new Set<string>();
 const json=(status:number,body:unknown)=>new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}});
 const fetcher:typeof fetch=async(input,init)=>{
  const url=String(input);
  if(url.endsWith('/v1/networking/identity-proof')){const q=JSON.parse(String(init?.body));return json(200,signed({kind:'portico.route-proof',version:'1',...pin,...q,issuedAt:new Date().toISOString(),expiresAt:new Date(Date.now()+29000).toISOString()}));}
  if(url.endsWith('/v1/auth/refresh')){
   // Slow enough that all three callers are waiting on a renewal at the same time.
   refreshes++;await new Promise(r=>setTimeout(r,150));
   const q=JSON.parse(String(init?.body)) as {refreshToken:string};
   if(q.refreshToken!==current){reuse++;return json(401,{error:{code:'unauthorized'}});}
   generation++;current='refresh_'+String(generation).repeat(24);const access='access_'+generation;live.add(access);
   return json(200,{...session(access,String(generation)),refreshToken:current});
  }
  const bearer=(new Headers(init?.headers).get('authorization')??'').replace('Bearer ','');
  return live.has(bearer)?json(200,{viewer:saved.viewer}):json(401,{error:{code:'unauthorized'}});
 };
 const store=storage(),initial=await ready(fetcher);
 await rememberNativeSession(initial.api,saved,{storage:store});
 await store.change(v=>v&&{...v,session:{...v.session,expiresAt:new Date(Date.now()-1000).toISOString()}});
 const env={storage:store,fetcher};
 // The background refresher on the transport the shell already holds, with the same stored sign-in.
 const shellApi=new HttpLocalApi(lan,'access_1',fetcher);shellApi.useRoutes(initial.connection);
 const {refreshToken:_secret,...visible}=saved;
 const refresher=new LocalSessionRefresher({env,api:shellApi,session:{...visible,expiresAt:new Date(Date.now()-1000).toISOString()},setTimer:()=>0,clearTimer:()=>{}});
 try{
  const [a,b,c]=await Promise.allSettled([restoreNativeSession(env),restoreNativeSession(env),refresher.recover('access_1')]);
  assert.equal(reuse,0,'no refresh token was presented twice');
  assert.equal(refreshes,1,'exactly one renewal');
  const stored=await store.read();
  assert.equal(stored?.session.refreshToken,current);assert.equal(stored?.pendingRefresh,undefined);
  for(const r of [a,b])if(r.status==='fulfilled')assert.equal(r.value?.session.accessToken,'access_2');
assert.equal(c.status==='fulfilled'&&c.value,'access_2');
  for(const r of [a,b])if(r.status==='fulfilled')r.value?.api.getRouteConnection()?.dispose();
 }finally{refresher.stop();initial.connection.dispose();}
});
test('a Portico Account member’s server-local session is remembered with its Portico route source; a legacy hosted viewer must still match',async()=>{
 const store=storage(),a=await ready();
 const hosted={origin:'https://hosted.example',accountId:'acc_portico',key:{} as never};
 try{
  const member={...session(),viewer:{authority:'local',accountId:'acct_server_local',profileId:'profile',serverId:pin.serverId,role:'member'}};
  await rememberNativeSession(a.api,member,{storage:store},hosted);
  const saved=await store.read();assert.equal(saved?.hosted?.accountId,'acc_portico');assert.equal(saved?.session.viewer.authority,'local');
  // …and read back at launch (the e2e restore failed on this before).
  const hostedKey={origin:'https://hosted.example',accountId:'acc_portico',key:{keyId:'k',publicKey:'p'.repeat(43)}};
  assert.equal(parseRememberedServer({...structuredClone(saved),hosted:hostedKey})?.session.viewer.authority,'local');
  assert.throws(()=>parseRememberedServer({...structuredClone(saved),hosted:hostedKey,session:{...structuredClone(saved)!.session,viewer:{...structuredClone(saved)!.session.viewer,authority:'hosted',accountId:'someone_else'}}}));
  const legacy={...session('token_two'),viewer:{authority:'hosted',accountId:'someone_else',profileId:'profile',serverId:pin.serverId,role:'member'}};
  await assert.rejects(rememberNativeSession(a.api,legacy,{storage:store},hosted),/another account/);
 }finally{a.connection.dispose();}
});
