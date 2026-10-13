import {readFileSync} from 'node:fs';
import path from 'node:path';
/** Ordinary password sign-in for generated local fixtures; never accepts a remote origin. */
export function fixtureApi(base, output) {
  if(!['127.0.0.1','localhost','[::1]'].includes(new URL(base).hostname)) throw new Error('Use a disposable loopback server');
  let token;
  const installation=crypto.randomUUID().replaceAll('-','');
  const signIn=async()=>{
    const credentials=JSON.parse(readFileSync(path.join(output,'credentials.json')));
    const response=await fetch(base+'/v1/direct/sign-in',{method:'POST',headers:{'Content-Type':'application/json','X-Portico-Installation-Id':installation},body:JSON.stringify(credentials)});
    const issued=await response.json();
    if(!response.ok||!issued.session?.accessToken)throw new Error('Generated fixture account could not sign in');
    token=issued.session.accessToken;
  };
  return async(route,method='GET',body)=>{
    if(!route.startsWith('/v1/'))throw new Error('Fixture requests must use the local API');
    if(!token)await signIn();
    const request=()=>fetch(base+route,{method,headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});
    let response=await request();
    if(response.status===401){await signIn();response=await request();}
    const text=await response.text();const result=text?JSON.parse(text):null;
    if(!response.ok)throw new Error(`${method} ${route}: ${response.status} ${JSON.stringify(result)}`);
    return result;
  };
}
