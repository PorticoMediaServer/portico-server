import {routeOrigin,probeRoute,type ServerPin} from '@core/route-identity';
import {HttpLocalApi} from '@core/index.ts';

export type NetworkSystem={id:string;name:string;setupRequired:boolean;version:string;buildId:string};
export type NetworkCheck={system:NetworkSystem;address:string;checkedAt:Date};
export function directServerAddress(value:string):string{
 return routeOrigin(value.trim());
}
export function networkSystem(value:unknown):NetworkSystem{
 if(!value||typeof value!=='object'||Array.isArray(value))throw new Error('This address did not return Portico server information.');
 const v=value as Record<string,unknown>;
 if(typeof v.id!=='string'||!v.id||v.id.length>128||typeof v.name!=='string'||!v.name||v.name.length>256||typeof v.setupRequired!=='boolean')throw new Error('This address did not return valid Portico server information.');
 return {id:v.id,name:v.name,setupRequired:v.setupRequired,version:typeof v.version==='string'?v.version.slice(0,100):'',buildId:typeof v.buildId==='string'?v.buildId.slice(0,128):''};
}
export async function checkCurrentServer(api:HttpLocalApi,signal:AbortSignal):Promise<NetworkCheck>{
 const system=networkSystem(await api.requestBounded<unknown>('/v1/system',16384,signal));
 return {system,address:api.baseUrl,checkedAt:new Date()};
}
export async function checkDirectServer(value:string,expectedID:string,signal:AbortSignal,pin?:ServerPin):Promise<NetworkCheck>{
 const address=directServerAddress(value);
 if(!pin||pin.serverId!==expectedID)throw new Error('Sign in again to establish the current server identity before checking another address.');
 await probeRoute(address,pin,{signal});
 // A candidate receives no current-server token, cookies or redirect authority.
 const api=new HttpLocalApi(address,'',(input,init)=>fetch(input,{...init,credentials:'omit',redirect:'error',cache:'no-store',referrerPolicy:'no-referrer'}));
 const check=await checkCurrentServer(api,signal);
 if(check.system.id!==expectedID)throw new Error(`That address belongs to another Portico server (${check.system.name}). Check the address for this server.`);
 return check;
}
export function directAddressScope(address:string):'device'|'network'|'https'{
 const host=new URL(address).hostname;
 if(['localhost','127.0.0.1','[::1]'].includes(host))return 'device';
 if(host.endsWith('.local')||host.startsWith('192.168.')||host.startsWith('10.')||/^172\.(1[6-9]|2\d|3[01])\./.test(host))return 'network';
 return 'https';
}
