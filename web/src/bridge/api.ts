import {ApiError} from '@core/index.ts';
export type AccountSession={accessToken:string;expiresAt:string;account:{id:string;username:string;displayName:string};profiles:{id:string;name:string}[]};
export type ServerEntry=import('@core/index.ts').HostedServer;
export async function hosted<T>(origin:string,path:string,body?:unknown,token?:string):Promise<T>{
 const url=new URL(origin);if(url.protocol!=='https:'&&!(url.protocol==='http:'&&['127.0.0.1','localhost'].includes(url.hostname)))throw new Error('Use an HTTPS address for Hosted Services.');
 const response=await fetch(url.origin+path,{method:body===undefined?'GET':'POST',signal:AbortSignal.timeout(15000),redirect:'error',credentials:'omit',referrerPolicy:'no-referrer',headers:{'Content-Type':'application/json',...(token?{Authorization:`Bearer ${token}`}:{})},...(body===undefined?{}:{body:JSON.stringify(body)})});
 if(!response.ok){const value=await response.json().catch(()=>null);throw new ApiError(response.status,value?.error?.code??'request_failed',value?.error?.message??'Could not reach your account.',response.status>=500);}
 return response.status===204?undefined as T:response.json();
}
export const message=(error:unknown)=>error instanceof Error?error.message:'Something went wrong. Try again.';
export function duration(seconds:number){if(!seconds)return '';const hours=Math.floor(seconds/3600);const minutes=Math.floor(seconds%3600/60);return hours?`${hours}h ${minutes}m`:`${minutes} min`;}
