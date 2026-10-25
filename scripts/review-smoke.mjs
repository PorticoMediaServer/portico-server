import fs from 'node:fs';
import {createInterface} from 'node:readline';
import {createHash} from 'node:crypto';
const origin=process.env.PORTICO_REVIEW_ORIGIN,commit=process.env.PORTICO_REVIEW_COMMIT;
const {privacy,safePath}=await import('file://'+process.env.PORTICO_RUNNER_SECONDARY_SOURCE+'/scripts/review-smoke-privacy.mjs');
const privateValues=[];const channel=createInterface({input:process.stdin,terminal:false,crlfDelay:Infinity});const lines=channel[Symbol.asyncIterator]();const nextInput=async()=>{const row=await lines.next();if(row.done)throw Error('private-input-closed');return row.value;};
const {chromium}=await import(process.env.TMPDIR+'/browser/node_modules/playwright/index.mjs');
let browser,page,step='launch',credentialPhase=false;const consoleEvents=[],requests=[],responses=[];const pendingResponses=[];const out=process.env.PORTICO_RUNNER_OUTPUT;const failures=[];const authFacts={profileSelectionPOSTOccurred:false,me:[],refresh:[]};const pendingAuth=[];const safeRole=value=>['account','owner','admin','member'].includes(value)?value:'unobserved';
try {
 browser=await chromium.launch({executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium',headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 const context=await browser.newContext({viewport:{width:1440,height:900}});page=await context.newPage();page.setDefaultTimeout(30000);if(!Object.values(await page.evaluate(privacy,{kind:'fixture'})).every(Boolean))throw Error('diagnostic-redaction');page.on('console',m=>{if(!credentialPhase)consoleEvents.push({type:m.type(),text:m.text().slice(0,3000)});});page.on('request',r=>{const u=new URL(r.url());requests.push({method:r.method(),origin:u.origin,path:safePath(u.pathname),type:r.resourceType()});});
 page.on('response',r=>{if(!credentialPhase && ['document','script'].includes(r.request().resourceType()))pendingResponses.push((async()=>{const u=new URL(r.url());const body=await r.body();responses.push({origin:u.origin,path:safePath(u.pathname),status:r.status(),contentType:r.headers()['content-type'],sha256:createHash('sha256').update(body).digest('hex')});})().catch(()=>{}));if(r.status()>=400){const u=new URL(r.url());if(u.origin===origin)failures.push({path:safePath(u.pathname),status:r.status()});}});
 page.on('request',request=>{
  const path=new URL(request.url()).pathname;
  if(request.method()==='POST'&&(/^\/v1\/direct\/profiles\/.+\/select$/.test(path)||path==='/v1/direct/profile-switch'))authFacts.profileSelectionPOSTOccurred=true;
  if(path==='/v1/me'||path==='/v1/auth/refresh')pendingAuth.push((async()=>{const headers=await request.allHeaders();authFacts[path==='/v1/me'?'me':'refresh'].push({utc:new Date().toISOString(),authorizationPresent:!!headers.authorization,cookiePresent:!!headers.cookie});})().catch(()=>{authFacts.requestMetadataUnavailable=true;}));
 });
 page.on('response',response=>{
  const path=new URL(response.url()).pathname;if(!['/v1/direct/sign-in','/v1/me','/v1/auth/refresh'].includes(path))return;
  const entry={utc:new Date().toISOString(),path,status:response.status()};(authFacts.responses??=[]).push(entry);
  if(path!=='/v1/me')pendingAuth.push((async()=>{const body=await response.json();entry.grantRole=safeRole(body.session?.viewer?.role??body.viewer?.role);if(path==='/v1/direct/sign-in'){entry.accountScoped=body.session?.viewer?.role==='account';entry.profilesCount=Array.isArray(body.profiles)?body.profiles.length:null;}})().catch(()=>{authFacts.responseMetadataUnavailable=true;}));
 });
 page.on('pageerror',()=>failures.push({pageError:true}));
 if(process.env.PORTICO_REVIEW_AWAIT_ACTIVATION==='1'){console.log('BROWSER_ADMITTED');if(await nextInput()!=='PUBLIC_ACTIVATED')throw Error('activation-control');}
 step='source-cutoff';const stamp=await(await page.request.get(origin+'/portico-build.json')).json();if(stamp.commit!==commit || (stamp.serverCommit && stamp.serverCommit!==commit))throw Error('source-cutoff');
 step='signed-out-root';await page.goto(origin+'/',{waitUntil:'networkidle'});
 await page.locator('input[name="username"]').waitFor();await page.locator('input[name="password"]').waitFor();
 await page.locator('form button[type="submit"]').waitFor();console.log('PASS signed-out / renders sign-in form');console.log(JSON.stringify({event:'signed-out / renders sign-in form',utc:new Date().toISOString(),origin,commit}));
 step='sign-in-document-reload';const response=await page.goto(origin+'/sign-in',{waitUntil:'networkidle'});
 if(response.status()!==200 || !response.headers()['content-type'].includes('text/html'))throw Error('document');
 await page.reload({waitUntil:'networkidle'});await page.locator('input[name="password"]').waitFor();
 await page.screenshot({path:out+'/sign-in.png'});console.log('PASS /sign-in document and reload');console.log(JSON.stringify({event:'/sign-in document and reload',utc:new Date().toISOString(),origin,commit}));console.log('PRIVATE_STDIN_READY');
 const credentials=JSON.parse(await nextInput());credentialPhase=true;privateValues.push(credentials.username,credentials.password);
 step='owner-sign-in';await page.locator('input[name="username"]').fill(credentials.username);await page.locator('input[name="password"]').fill(credentials.password);credentials.password='';
 await page.locator('form button[type="submit"]').click();
 await page.waitForFunction(()=>location.pathname==='/' || location.pathname==='/profiles');
 if(new URL(page.url()).pathname==='/profiles'){await page.reload({waitUntil:'networkidle'});await page.getByRole('button').filter({hasText:/justin/i}).first().click();}
 step='owner-home-account-control';const trigger=page.getByRole('button',{name:/^Switch profile or server(?:,|$)|, account menu$/});await trigger.waitFor();
 step='owner-home-navigation';await page.getByRole('link',{name:'Home',exact:true}).waitFor();step='owner-home-document';await page.locator('[data-home-document]').waitFor();if(new URL(page.url()).pathname!=='/')throw Error('home-route');
 await page.screenshot({path:out+'/owner-home.png'});console.log('PASS owner sign-in -> Home');console.log(JSON.stringify({event:'owner sign-in -> Home',utc:new Date().toISOString(),origin,commit}));
 step='second-profile';const profileDocument=await page.goto(origin+'/profiles',{waitUntil:'networkidle'});if(profileDocument.status()!==200 || !profileDocument.headers()['content-type'].includes('text/html'))throw Error('profile-document');await page.reload({waitUntil:'networkidle'});await page.getByRole('button').filter({hasText:/Review/}).first().click();await page.locator('[data-home-document]').waitFor();
 await trigger.waitFor();await trigger.click();await page.getByRole('menuitem',{name:/justin/i}).waitFor();
 console.log('PASS second profile Review reached; owner available to switch back');console.log(JSON.stringify({event:'second profile Review reached; owner available to switch back',utc:new Date().toISOString(),origin,commit}));
 step='sign-out';await page.getByRole('menuitem',{name:'Sign out of Portico demo',exact:true}).click();
 await page.locator('input[name="username"]').waitFor();await page.locator('input[name="password"]').waitFor();
 if(await page.locator('input[name="password"]').inputValue())throw Error('password-retained');
 console.log('PASS sign-out -> sign-in');console.log(JSON.stringify({event:'sign-out -> sign-in',utc:new Date().toISOString(),origin,commit}));
 console.log(JSON.stringify({result:'FOUR_SMOKES_PASS',commit,diagnostics:failures}));
} catch(error){console.log(JSON.stringify({result:'SMOKE_FAIL',step,errorType:error.name,diagnostics:failures}));process.exitCode=1;}
finally {
 if(page){
  try {
   await page.locator('input').evaluateAll(nodes=>nodes.forEach(node=>{node.value='';node.removeAttribute('value');}));
   const document=await page.evaluate(privacy,{kind:'dom',privateValues});
   fs.writeFileSync(out+'/failure-dom.log',document);
   await Promise.allSettled([...pendingResponses,...pendingAuth]);const cookies=await browser.contexts()[0].cookies();authFacts.cookies={present:cookies.length>0,flags:cookies.map(c=>({secure:c.secure,httpOnly:c.httpOnly,sameSite:c.sameSite}))};console.log(JSON.stringify({diagnosis:{authFacts,responses,visibility:await page.evaluate(()=>document.visibilityState),url:new URL(page.url()).origin+safePath(new URL(page.url()).pathname),title:await page.title(),console:consoleEvents.map(event=>({...event,text:privacy({kind:'text',value:event.text,privateValues})})),requests,failures}}));try{const credentialsRemain=await page.locator('input[type="password"],input[name="username"],input[name="password"]').count();if(credentialPhase&&credentialsRemain){console.log('Credential-form screenshot withheld; sanitized DOM retained.');}else{await page.screenshot({path:out+'/final-page.png',timeout:5000});}}catch{console.log('Diagnostic screenshot unavailable; DOM and console retained.');}
  }catch{console.log('Diagnostic page unavailable after browser failure');}
 }
 channel.close();if(browser)await browser.close();
}
