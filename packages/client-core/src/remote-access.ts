import type {HttpLocalApi} from './index.ts';
import {routeOrigin} from './route-identity.ts';
/**
 * Owner remote-access settings. A save sends the whole object back, so fields a server added
 * (lanSharing, ipv6Open) must round-trip: dropping one would reset it to false. Older servers
 * omit them.
 */
export type RemoteConfiguration={revision:string;enabled:boolean;mapping:boolean;pcp:boolean;natpmp:boolean;upnp:boolean;publicPort:number;gateway:string;lanSharing?:boolean;
 /** The owner confirms inbound IPv6 reaches this server (their router allows it); without it, or a pinhole, the IPv6 address isn't published. */
 ipv6Open?:boolean};
/** Why the server's global IPv6 address is (or isn't) published. */
export type RemoteIPv6State='pinhole'|'firewall_open'|'owner_confirmed'|'unverified';
export type RemoteAccess={authorityId:string;generation:string;config:RemoteConfiguration;state:string;errorCode?:string;topology:{gateway?:string;localAddress?:string;lan:string[];public:string[];bind:string;errorCode?:string};candidates:{baseUrl:string;class:'lan'|'public'|'manual';state:string;verifiedAt?:string}[];mappings:{protocol:string;state:string;errorCode?:string;externalPort:number;expiresAt:string}[];ipv6?:RemoteIPv6State|string;checkedAt?:string;nextAttempt?:string};
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const counter=(v:unknown)=>typeof v==='string'&&/^[1-9][0-9]{0,18}$/.test(v)&&BigInt(v)<=9223372036854775807n;
export function parseRemoteAccess(v:unknown):RemoteAccess{
 if(!object(v)||typeof v.authorityId!=='string'||v.authorityId.length>1024||!counter(v.generation)||typeof v.state!=='string'||!object(v.config)||!object(v.topology)||!Array.isArray(v.candidates)||v.candidates.length>16||!Array.isArray(v.mappings)||v.mappings.length>32)throw new Error('The remote-access response was incomplete.');
 const c=v.config;if(!counter(c.revision)||['enabled','mapping','pcp','natpmp','upnp'].some(k=>typeof c[k]!=='boolean')||['lanSharing','ipv6Open'].some(k=>c[k]!==undefined&&typeof c[k]!=='boolean')||(v.ipv6!==undefined&&(typeof v.ipv6!=='string'||v.ipv6.length>32))||!Number.isInteger(c.publicPort)||Number(c.publicPort)<1||Number(c.publicPort)>65535||typeof c.gateway!=='string'||c.gateway.length>64)throw new Error('The remote-access configuration was incomplete.');
 for(const key of ['lan','public'])if(!Array.isArray(v.topology[key])||(v.topology[key] as unknown[]).length>8||(v.topology[key] as unknown[]).some(x=>typeof x!=='string'||x.length>64))throw new Error('The topology response was incomplete.');
 for(const row of v.candidates){if(!object(row)||typeof row.baseUrl!=='string'||routeOrigin(row.baseUrl)!==row.baseUrl||!['lan','public','manual'].includes(String(row.class))||typeof row.state!=='string')throw new Error('The remote route response was incomplete.');}
 for(const row of v.mappings)if(!object(row)||typeof row.protocol!=='string'||typeof row.state!=='string'||!Number.isInteger(row.externalPort)||typeof row.expiresAt!=='string')throw new Error('The gateway mapping response was incomplete.');
 return v as unknown as RemoteAccess;
}
export async function loadRemoteAccess(api:HttpLocalApi,signal?:AbortSignal){return parseRemoteAccess(await api.request('/v1/networking/remote','GET',undefined,signal));}
export async function saveRemoteAccess(api:HttpLocalApi,status:RemoteAccess,config:RemoteConfiguration,signal?:AbortSignal){
 if(config.revision!==status.config.revision)throw new Error('Reload remote-access settings before saving.');
 return parseRemoteAccess(await api.request('/v1/networking/remote/config','POST',{authorityId:status.authorityId,config},signal));
}
export async function checkRemoteAccess(api:HttpLocalApi,signal?:AbortSignal){return parseRemoteAccess(await api.request('/v1/networking/remote/check','POST',{},signal));}
export function remoteAccessSummary(status:RemoteAccess):string{return ({reachable:'Remote access verified by Portico Account.',disabled:'Remote access is off. Verified local connections remain available.',claim_required:'Connect this server to a Portico Account before publishing remote routes.',certificate_pending:'Waiting for a publicly trusted HTTPS certificate.',manual_only:'Reachable through your own addresses only (a reverse proxy or VPN). Apps test them directly.',hosted_unavailable:'Portico Account is unavailable. Existing verified local connections still work.',remote_unavailable:'No verified public route is available.',checking:'Checking this server and its current network.',lan_only:'Available locally; no public route has been verified.'} as Record<string,string>)[status.state]??'Remote access is being checked.';}
export function remoteAccessHelp(code?:string):string{
 return ({listener_loopback_only:'The listener accepts only this device. Bind the server to its private LAN interface before enabling other devices.',gateway_not_found:'No supported default gateway was found. Configure a private gateway or a manual HTTPS reverse proxy.',gateway_outside_listener:'The gateway-selected interface is outside the server listener.',no_public_route:'The router may be behind carrier-grade NAT or another router. Use a reachable public IPv6 address, configure upstream forwarding, or provide a public HTTPS reverse proxy. Portico does not bypass carrier-grade NAT.',external_proof_or_hosted_unavailable:'A router lease or certificate alone is not proof of reachability. Check upstream NAT, firewall rules, public HTTPS, and the account-service connection.',mapping_not_authorized:'The router did not authorize this mapping.',mapping_unsupported:'This gateway does not support the requested mapping protocol.',mapping_conflict:'This port belongs to another mapping. Choose a different public port; Portico will not overwrite it.'} as Record<string,string>)[code??'']??(code?'The worker will retry where safe. Inspect the diagnostic code and gateway configuration.':'');
}
