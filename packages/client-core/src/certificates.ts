import {readBoundedJson} from './bounded-json.ts';

export type CertificateConfig = Readonly<{enabled:boolean; publicAddress:string; publicPort:number; revision:string}>;
export type CertificateStatus = Readonly<{
 configured:boolean; authorityId:string; config:CertificateConfig; state:string; errorCode?:string;
 namespace?:string; dnsName?:string; issuer?:string; environment:'production'|'staging'; orderState?:string;
 notAfter?:string; renewAt?:string; nextAttemptAt?:string; tlsReady:boolean; publiclyTrusted:boolean;
 listenerBound:boolean; listenPort:number; routeHostname?:string; routeUrl?:string; routeErrorCode?:string; reachability:'probe_required';
}>;
export type CertificateAction='status'|'config'|'retry';
export interface CertificateApi {requestCertificate<T>(action:CertificateAction,body:unknown,signal:AbortSignal):Promise<T>}
const record=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const text=(v:unknown,max=256):v is string=>typeof v==='string'&&v.length<=max&&!/[\u0000-\u001f\u007f]/.test(v);
const code=(v:unknown):v is string=>text(v,80)&&/^[a-z0-9_]*$/.test(v);
const port=(v:unknown,min=1):v is number=>typeof v==='number'&&Number.isInteger(v)&&v>=min&&v<=65535;
function fail():never{throw new Error('Invalid certificate status.');}
export function parseCertificateStatus(v:unknown):CertificateStatus {
 if(!record(v)||!record(v.config)||!text(v.authorityId,64)||!/^([a-f0-9]{64})?$/.test(v.authorityId)||!code(v.state)||!['production','staging'].includes(String(v.environment))||v.reachability!=='probe_required')fail();
 const c=v.config;
 if(typeof c.enabled!=='boolean'||!port(c.publicPort)||!text(c.publicAddress,64)||typeof c.revision!=='string'||!/^\d{1,19}$/.test(c.revision))fail();
 for(const key of ['configured','tlsReady','publiclyTrusted','listenerBound'])if(typeof v[key]!=='boolean')fail();
 if(!port(v.listenPort,0)||v.publiclyTrusted&&(!v.tlsReady||v.environment!=='production')||v.tlsReady&&!c.enabled)fail();
 for(const key of ['errorCode','routeErrorCode','orderState'])if(v[key]!==undefined&&!code(v[key]))fail();
 for(const key of ['namespace','dnsName','issuer','routeHostname','routeUrl'])if(v[key]!==undefined&&!text(v[key],512))fail();
 for(const key of ['notAfter','renewAt','nextAttemptAt'])if(v[key]!==undefined&&(!text(v[key],64)||!/^\d{4}-\d\d-\d\dT/.test(v[key])||!Number.isFinite(Date.parse(v[key]))))fail();
 if(v.namespace!==undefined&&!/^ptc-[a-z2-7]{19}[aq]$/.test(v.namespace as string))fail();
 if(v.dnsName!==undefined&&v.dnsName!=='*.'+v.namespace+'.direct.getportico.tv')fail();
 if(v.routeUrl){let u:URL;try{u=new URL(v.routeUrl as string);}catch{fail();}if(u.protocol!=='https:'||u.username||u.password||u.search||u.hash||u.pathname!=='/'||u.hostname!==v.routeHostname||u.hostname!=='current.'+v.namespace+'.direct.getportico.tv'||(u.port||'443')!==String(c.publicPort))fail();}
  // Build a whitelist projection: never retain unexpected keys, CSR, tokens or provider response bodies.
  // CD-10: keep Go wire order (enabled, publicPort, publicAddress, revision) for canonical bytes.
  const out:Record<string,unknown>={configured:v.configured,authorityId:v.authorityId,config:Object.freeze({enabled:c.enabled,publicPort:c.publicPort,publicAddress:c.publicAddress,revision:c.revision}),state:v.state,environment:v.environment,tlsReady:v.tlsReady,publiclyTrusted:v.publiclyTrusted,listenerBound:v.listenerBound,listenPort:v.listenPort,reachability:'probe_required'};
 for(const k of ['errorCode','namespace','dnsName','issuer','orderState','notAfter','renewAt','nextAttemptAt','routeHostname','routeUrl','routeErrorCode'])if(v[k]!==undefined)out[k]=v[k];
 return Object.freeze(out) as CertificateStatus;
}

/** Streaming on web; RN's native fetch buffers before JS, so apply the same
 * bounded projection to its buffered text. No response body enters error text. */
export async function readCertificateResponse<T>(response:Response,signal:AbortSignal):Promise<T>{
 if(response.body&&typeof response.body.getReader==='function')return readBoundedJson<T>(response,16384,signal);
 if(!response.ok)throw Object.assign(new Error('Certificate request failed.'),{status:response.status});
 const length=response.headers.get('Content-Length');
 if(length!==null&&(!/^\d+$/.test(length)||Number(length)>16384))fail();
 if(response.headers.get('Content-Type')?.split(';')[0].trim().toLowerCase()!=='application/json')fail();
 const raw=await response.text();
 if(signal.aborted||raw.length>16384||new TextEncoder().encode(raw).byteLength>16384)fail();
 try{return JSON.parse(raw) as T;}catch{fail();}
}
export async function loadCertificate(api:CertificateApi,signal:AbortSignal):Promise<CertificateStatus>{return parseCertificateStatus(await api.requestCertificate('status',undefined,signal));}
export async function saveCertificate(api:CertificateApi,current:CertificateStatus,config:CertificateConfig,signal:AbortSignal):Promise<CertificateStatus>{
 if(!current.authorityId)throw new Error('Claim this server before configuring HTTPS.');
 // CD-10: explicit Go wire order; never spread the parsed config (wrong insertion order).
 // Outer authorityId then config; inner enabled, publicPort, publicAddress, revision.
 return parseCertificateStatus(await api.requestCertificate('config',{authorityId:current.authorityId,config:{enabled:config.enabled,publicPort:config.publicPort,publicAddress:config.publicAddress,revision:current.config.revision}},signal));
}
export async function retryCertificate(api:CertificateApi,signal:AbortSignal):Promise<CertificateStatus>{return parseCertificateStatus(await api.requestCertificate('retry',{},signal));}
export function certificateError(error:unknown):string{
 const status=record(error)?error.status:undefined;
 if(status===401||status===403)return 'Sign in with this server’s local owner account to manage automatic HTTPS.';
 if(status===404)return 'Automatic HTTPS is not configured on this server. Configure its Hosted connection first.';
 if(status===409)return 'The server claim or configuration changed. Refresh before saving again.';
 if(status===429)return 'A retry is already scheduled. The server will continue automatically.';
 return 'Certificate status could not be updated. Check the server connection and try again.';
}
export function certificateSummary(s:CertificateStatus):string{
 if(s.state==='paused')return 'Automatic HTTPS is paused. Remote TLS connections are not accepted.';
 // Off is said as off: the states below describe a certificate that was asked for.
 if(!s.config.enabled&&!s.tlsReady)return 'No certificate has been requested. Turn on “Request a certificate” to get one.';
 if(s.tlsReady&&s.environment==='staging')return 'A staging certificate is installed for testing. Devices will not publicly trust it.';
 if(s.tlsReady&&!s.listenerBound)return 'A valid certificate is installed, but the TLS listener is not running.';
 if(s.tlsReady&&s.state==='recovering')return 'HTTPS is using the previous valid certificate while replacement is prepared.';
 if(s.errorCode==='acme_account_configuration_or_retry'||s.errorCode==='dns_or_issuer_configuration')return 'The Hosted certificate service needs an operator configuration check or a provider retry. Any valid installed certificate remains available.';
 if(s.tlsReady&&s.state==='renewing')return 'HTTPS is ready. Renewal is in progress using the existing valid certificate.';
 if(s.tlsReady)return 'HTTPS is ready on this server. This does not confirm access from another network.';
 if(s.orderState==='reconciliation_required')return 'The certificate provider may have accepted a request. Operator reconciliation is required; no duplicate order will be created.';
 switch(s.state){
 case 'claim_required':case 'claim_pending':case 'authority_unavailable':return 'Complete this server’s account connection before enabling automatic HTTPS.';
 case 'configuration_required':return 'The server or Hosted certificate service needs operator configuration.';
 case 'expired':return 'The certificate has expired. Remote TLS is unavailable while the server attempts renewal.';
 case 'invalid':return 'Certificate material could not be validated. Remote TLS is unavailable; use local owner access to recover.';
 case 'retrying':return 'A certificate request will be retried automatically. The server keeps any existing valid certificate.';
 default:return 'The server is preparing its certificate. You can leave this page; work continues on the server.';
 }
}
export function certificateDate(value?:string):string{return value?new Date(value).toLocaleString():'Not yet available';}

