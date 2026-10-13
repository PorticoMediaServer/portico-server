'use strict';
(() => {
 const request=JSON.parse(document.getElementById('request').textContent);
 // Remove the return/state/challenge from history before the account tab opens.
 history.replaceState(null,'','/auth/apps/authorize');
 const status=document.getElementById('status');
 const buttons=[...document.querySelectorAll('button')];
 let reviewed=null;
 let notBefore=0;
 const text=(id,value)=>{document.getElementById(id).textContent=value;};
 async function call(path,body){
  if(Date.now()<notBefore)throw new Error('Wait a moment before trying again.');
  const name=location.protocol==='https:'?'__Host-portico_refresh_csrf':'portico_refresh_dev_csrf';
  const csrf=document.cookie.split('; ').find(c=>c.startsWith(name+'='))?.slice(name.length+1)??'';
  const response=await fetch(path,{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify(body),signal:AbortSignal.timeout(15000),redirect:'error'});
  const value=await response.json();
  if(!response.ok){
   const retry=response.headers.get('Retry-After');
   const seconds=retry&&/^\d+$/.test(retry)?Number(retry):retry?Math.max(0,(Date.parse(retry)-Date.now())/1000):0;
   if(response.status===429||response.status>=500)notBefore=Date.now()+Math.max(5000,seconds*1000);
   if(response.status===401)throw new Error('Sign in to your Portico Account, then return here to continue.');
   throw new Error(value.error?.message??'Sign-in could not complete. Try again.');
  }
  return value;
 }
 async function work(action){
  buttons.forEach(b=>b.disabled=true);status.textContent='';
  try{await action();}catch(e){status.textContent=e.name==='TimeoutError'?'Sign-in could not complete. Try again.':e.message;}
  finally{buttons.forEach(b=>b.disabled=false);}
 }
 document.getElementById('review').addEventListener('click',()=>work(async()=>{
  reviewed=await call('/v1/app-authorizations',request);
  text('consent-line',`Allow ${reviewed.appName} (an unverified app name) to sign you in to ${reviewed.serverName}?`);
  text('account-name',reviewed.accountName);
  text('return-address',reviewed.redirectUri);
  document.getElementById('signin').hidden=true;document.getElementById('consent').hidden=false;
  document.getElementById('approve').focus();
 }));
 const decide=approve=>work(async()=>{
  const result=await call('/v1/app-authorizations/'+encodeURIComponent(reviewed.id)+'/decision',{accountId:reviewed.accountId,approve});
  location.assign(result.location);
 });
 document.getElementById('approve').addEventListener('click',()=>decide(true));
 document.getElementById('deny').addEventListener('click',()=>decide(false));
})();
