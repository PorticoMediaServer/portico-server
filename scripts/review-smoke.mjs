import fs from 'node:fs';
import {createHash} from 'node:crypto';
const origin=process.env.PORTICO_REVIEW_ORIGIN,commit=process.env.PORTICO_REVIEW_COMMIT;
const {chromium}=await import(process.env.TMPDIR+'/browser/node_modules/playwright/index.mjs');
let browser,page,step='launch',credentialPhase=false;const consoleEvents=[],requests=[],responses=[];const pendingResponses=[];const out=process.env.PORTICO_RUNNER_OUTPUT;const failures=[];
try {
 browser=await chromium.launch({executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium',headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 const context=await browser.newContext({viewport:{width:1440,height:900}});page=await context.newPage();page.setDefaultTimeout(30000);page.on('console',m=>{if(!credentialPhase)consoleEvents.push({type:m.type(),text:m.text().slice(0,3000)});});page.on('request',r=>{const u=new URL(r.url());requests.push({method:r.method(),origin:u.origin,path:u.pathname.replace(/[0-9a-f]{8}-[0-9a-f-]{27,}/gi,':id'),type:r.resourceType()});});
 page.on('response',r=>{if(!credentialPhase && ['document','script'].includes(r.request().resourceType()))pendingResponses.push((async()=>{const u=new URL(r.url());const body=await r.body();responses.push({origin:u.origin,path:u.pathname.replace(/[0-9a-f]{8}-[0-9a-f-]{27,}/gi,':id'),status:r.status(),contentType:r.headers()['content-type'],sha256:createHash('sha256').update(body).digest('hex')});})().catch(()=>{}));if(r.status()>=400){const u=new URL(r.url());if(u.origin===origin)failures.push({path:u.pathname.replace(/[0-9a-f]{8}-[0-9a-f-]{27,}/gi,':id'),status:r.status()});}});
 page.on('pageerror',()=>failures.push({pageError:true}));
 step='source-cutoff';const stamp=await(await page.request.get(origin+'/portico-build.json')).json();if(stamp.commit!==commit || (stamp.serverCommit && stamp.serverCommit!==commit))throw Error('source-cutoff');
 step='signed-out-root';await page.goto(origin+'/',{waitUntil:'networkidle'});
 await page.locator('input[name="username"]').waitFor();await page.locator('input[name="password"]').waitFor();
 await page.locator('form button[type="submit"]').waitFor();console.log('PASS signed-out / renders sign-in form');
 step='sign-in-document-reload';const response=await page.goto(origin+'/sign-in',{waitUntil:'networkidle'});
 if(response.status()!==200 || !response.headers()['content-type'].includes('text/html'))throw Error('document');
 await page.reload({waitUntil:'networkidle'});await page.locator('input[name="password"]').waitFor();
 await page.screenshot({path:out+'/sign-in.png'});console.log('PASS /sign-in document and reload');console.log('PRIVATE_STDIN_READY');
 let input='';for await(const chunk of process.stdin){input+=chunk;if(input.includes('\n'))break;}
 const credentials=JSON.parse(input);input='';credentialPhase=true;
 step='owner-sign-in';await page.locator('input[name="username"]').fill(credentials.username);await page.locator('input[name="password"]').fill(credentials.password);credentials.password='';
 await page.locator('form button[type="submit"]').click();
 await page.waitForFunction(()=>location.pathname==='/' || location.pathname==='/profiles');
 if(new URL(page.url()).pathname==='/profiles'){await page.reload({waitUntil:'networkidle'});await page.getByRole('button').filter({hasText:/justin/i}).first().click();}
 const trigger=page.getByRole('button',{name:/Switch profile or server/});await trigger.waitFor();
 await page.getByRole('link',{name:'Home',exact:true}).waitFor();await page.locator('[data-home-document]').waitFor();if(new URL(page.url()).pathname!=='/')throw Error('home-route');
 await page.screenshot({path:out+'/owner-home.png'});console.log('PASS owner sign-in -> Home');
 step='second-profile';const profileDocument=await page.goto(origin+'/profiles',{waitUntil:'networkidle'});if(profileDocument.status()!==200 || !profileDocument.headers()['content-type'].includes('text/html'))throw Error('profile-document');await page.reload({waitUntil:'networkidle'});await page.getByRole('button').filter({hasText:/Review/}).first().click();await page.locator('[data-home-document]').waitFor();
 await trigger.waitFor();await trigger.click();await page.getByRole('menuitem',{name:/justin/i}).waitFor();
 console.log('PASS second profile Review reached; owner available to switch back');
 step='sign-out';await page.getByRole('menuitem',{name:'Sign out of Portico demo',exact:true}).click();
 await page.locator('input[name="username"]').waitFor();await page.locator('input[name="password"]').waitFor();
 if(await page.locator('input[name="password"]').inputValue())throw Error('password-retained');
 console.log('PASS sign-out -> sign-in');
 console.log(JSON.stringify({result:'FOUR_SMOKES_PASS',commit,diagnostics:failures}));
} catch(error){console.log(JSON.stringify({result:'SMOKE_FAIL',step,errorType:error.name,diagnostics:failures}));process.exitCode=1;}
finally {
 if(page){
  try {
   await page.locator('input').evaluateAll(nodes=>nodes.forEach(node=>{node.value='';node.removeAttribute('value');}));
   const document=await page.evaluate(()=>{const clone=document.documentElement.cloneNode(true);clone.querySelectorAll('input,textarea').forEach(node=>{node.removeAttribute('value');node.textContent='';});return clone.outerHTML;});
   fs.writeFileSync(out+'/failure-dom.html',document);await page.screenshot({path:out+'/final-page.png'});
   await Promise.allSettled(pendingResponses);console.log(JSON.stringify({diagnosis:{responses,visibility:await page.evaluate(()=>document.visibilityState),url:new URL(page.url()).origin+new URL(page.url()).pathname,title:await page.title(),console:consoleEvents,requests,failures}}));
  }catch{console.log('Diagnostic page unavailable after browser failure');}
 }
 if(browser)await browser.close();
}
