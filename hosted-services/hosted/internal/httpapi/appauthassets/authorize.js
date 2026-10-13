'use strict';
(() => {
 const strings=JSON.parse(document.getElementById('strings').textContent);
 const request=JSON.parse(document.getElementById('request').textContent);
 // Remove the return/state/challenge from history before the account tab opens.
 history.replaceState(null,'','/auth/apps/authorize');
 const status=document.getElementById('status');
 const buttons=[...document.querySelectorAll('button')];
 const decisions=[document.getElementById('approve'),document.getElementById('deny')];
 let reviewed=null;
 let notBefore=0;
 // Continue is pressed before anything about the app is on screen. The answer
 // is accepted only once the consent has been shown for a moment, so a held
 // key, a second press or a double-click cannot carry on into a decision.
 let decideAfter=0;
 const text=(id,value)=>{document.getElementById(id).textContent=value;};
 // Each name is its own element, filled in one pass: words supplied by the app
 // or the server's owner never run into the page's own or take another's place.
 const sentence=(id,template,names)=>document.getElementById(id).replaceChildren(...template.split(/(\{[A-Za-z]+\})/).filter(Boolean).map(part=>{
  const key=part.slice(1,-1);
  if(!part.startsWith('{')||!Object.hasOwn(names,key))return document.createTextNode(part);
  const name=document.createElement('strong');name.textContent=names[key];return name;
 }));
 async function call(path,body){
  if(Date.now()<notBefore)throw new Error(strings.wait);
  const name=location.protocol==='https:'?'__Host-portico_refresh_csrf':'portico_refresh_dev_csrf';
  const csrf=document.cookie.split('; ').find(c=>c.startsWith(name+'='))?.slice(name.length+1)??'';
  const response=await fetch(path,{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify(body),signal:AbortSignal.timeout(15000),redirect:'error'});
  const value=await response.json();
  if(!response.ok){
   const retry=response.headers.get('Retry-After');
   const seconds=retry&&/^\d+$/.test(retry)?Number(retry):retry?Math.max(0,(Date.parse(retry)-Date.now())/1000):0;
   if(response.status===429||response.status>=500)notBefore=Date.now()+Math.max(5000,seconds*1000);
   if(response.status===401)throw new Error(strings.signIn);
   throw new Error(value.error?.message??strings.failed);
  }
  return value;
 }
 async function work(action){
  buttons.forEach(b=>b.disabled=true);status.textContent='';
  try{await action();}catch(e){status.textContent=e.name==='TimeoutError'?strings.failed:e.message;}
  finally{
   buttons.forEach(b=>b.disabled=false);
   const wait=decideAfter-Date.now();
   if(wait>0){decisions.forEach(b=>b.disabled=true);setTimeout(()=>decisions.forEach(b=>b.disabled=false),wait);}
  }
 }
 document.getElementById('review').addEventListener('click',()=>work(async()=>{
  reviewed=await call('/v1/app-authorizations',request);
  sentence('consent-line',strings.consent,{appName:reviewed.appName,serverName:reviewed.serverName});
  text('account-name',reviewed.accountName);
  text('return-address',reviewed.redirectUri);
  document.getElementById('signin').hidden=true;document.getElementById('consent').hidden=false;
  decideAfter=Date.now()+1000;
  // Focus goes to the question itself, never to an answer.
  document.getElementById('consent-line').focus();
 }));
 const decide=approve=>Date.now()<decideAfter?undefined:work(async()=>{
  const result=await call('/v1/app-authorizations/'+encodeURIComponent(reviewed.id)+'/decision',{accountId:reviewed.accountId,approve});
  location.assign(result.location);
 });
 document.getElementById('approve').addEventListener('click',()=>decide(true));
 document.getElementById('deny').addEventListener('click',()=>decide(false));
})();
