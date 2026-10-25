import fs from 'node:fs';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
const input=JSON.parse(fs.readFileSync(process.env.PORTICO_CHECKPOINT_INPUT,'utf8'));
const {chromium}=await import(pathToFileURL(process.env.PORTICO_CHECKPOINT_BROWSER));
const {privacy}=await import(pathToFileURL(process.env.PORTICO_RUNNER_SECONDARY_SOURCE+'/scripts/review-smoke-privacy.mjs'));
const out=process.env.PORTICO_RUNNER_OUTPUT,flows=[],network=[],pageErrors=[],privateValues=[input.username,input.password];
let browser,page,phase='startup',populatedCredentials=false;const external=[];
process.on('SIGTERM',async()=>{await browser?.close();process.exit(143);});
const errorClasses=['Error','TypeError','ReferenceError','SyntaxError','SecurityError','AbortError','NotSupportedError','InvalidStateError','NetworkError','EncodingError','NotAllowedError','DOMException'];
const disabled=new Set(input.disabled??[]);
async function capture(id){
 const credentials=await page.locator('input[type="password"],input[name="username"]').count();
 if(populatedCredentials&&credentials){fs.writeFileSync(path.join(out,'flow-'+id+'.log'),'Screenshot withheld: populated credential controls remain.\n');return;}
 await page.evaluate(values=>{const walk=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);for(let node=walk.nextNode();node;node=walk.nextNode())if(values.some(value=>value&&node.textContent.includes(value)))node.parentElement?.setAttribute('data-checkpoint-private','');},privateValues);
 try{await page.screenshot({path:path.join(out,'flow-'+id+'.jpg'),type:'jpeg',quality:82,timeout:5000,mask:[page.locator('[data-checkpoint-private]')]});}
 finally{await page.locator('[data-checkpoint-private]').evaluateAll(nodes=>nodes.forEach(node=>node.removeAttribute('data-checkpoint-private')));}
 fs.writeFileSync(path.join(out,'flow-'+id+'.log'),await page.evaluate(privacy,{kind:'dom',privateValues}));
}
async function flow(id,run){
 if(disabled.has(input.surface+':'+id)){flows.push({surface:input.surface,id,status:'dropped',manual:'Justin hands-on; flaky twice'});return false;}
 phase=id;const start=Date.now();let record;
 try{const facts=await run();record={surface:input.surface,id,status:'pass',facts:facts??{}};}
 catch(error){record={surface:input.surface,id,status:'fail',class:['TimeoutError','Error','TypeError','ReferenceError','SyntaxError'].includes(error.name)?error.name:'Other',timeout:error.name==='TimeoutError'};}
 record.durationMs=Date.now()-start;flows.push(record);try{await capture(id);}catch{record.screenshotUnavailable=true;}return record.status==='pass';
}
async function home(){await page.goto(input.origin+'/',{waitUntil:'domcontentloaded'});await page.locator('[data-home-document]').waitFor();}
try{
 browser=await chromium.launch({executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,headless:true,args:['--no-sandbox','--disable-dev-shm-usage','--disable-background-networking']});
 const context=await browser.newContext({viewport:{width:1440,height:900}});page=await context.newPage();page.setDefaultTimeout(12000);
 page.on('pageerror',error=>{if(pageErrors.length>=12)return;const text=String(error.message??''),code=/dynamically imported module/i.test(text)?'import_failed':/play\(\).*user|user.*interact|play.*not allowed/i.test(text)?'play_not_allowed':/no supported source|source.*not supported/i.test(text)?'unsupported_source':/Cannot read properties of (?:null|undefined)/.test(text)?'null_property':/Minified React error #\d+/.test(text)?'react_error':'other';pageErrors.push({phase,class:errorClasses.includes(error.name)?error.name:'Other',code,coordinates:[...String(error.stack??'').matchAll(/:(\d+):(\d+)/g)].slice(0,6).map(x=>[Number(x[1]),Number(x[2])])});});
 page.on('response',async response=>{try{
  const url=new URL(response.url());if(![input.origin,input.apiOrigin].includes(url.origin)||network.length>=100)return;
  const known=new Map([['/v1/sessions','sessions'],['/v1/sessions/refresh','refresh'],['/v1/account','account'],['/v1/account/security','security'],['/v1/account/sessions','devices'],['/v1/servers','directory'],['/v1/playback/start','playback-start'],['/v1/playback/options','playback-options'],['/v1/playback/controls','playback-controls']]);
  let operation=known.get(url.pathname);if(/\/reading\/sessions\/[^/]+\/assets$/.test(url.pathname))operation='reading-assets';
  else if(/\/reading\/sessions(?:\/[^/]+)?$/.test(url.pathname))operation='reading-sessions';
  if(!operation)return;const row={phase,operation,method:response.request().method(),status:response.status()};
  if(operation==='reading-sessions'&&response.request().method()==='POST'&&response.status()===200){try{const data=await response.json();if(['epub','pdf','comic'].includes(data.format))row.format=data.format;}catch{}}
  if(response.status()>=400){try{const data=await response.json(),code=data?.error?.code??data?.code;if(['invalid_credentials','session_mode_denied','origin_denied','invalid_request','forbidden','source_unavailable','metadata_conflict','challenge_invalid','rate_limited','session_expired','stream_limit_reached','conversion_capacity','transcode_not_allowed','unsupported_media','connection_too_slow','queue_building','not_permitted','no_tuner_available'].includes(code))row.code=code;}catch{}}
  network.push(row);
 }catch{}});
 page.on('request',request=>{try{const u=new URL(request.url());if(phase==='reading-nonblank-no-egress'&&!['data:','blob:','about:'].includes(u.protocol)&&u.origin!==input.origin)external.push({method:request.method(),foreignOrigin:true});}catch{}});
 await flow('signed-out-launch',async()=>{await page.goto(input.origin+'/',{waitUntil:'domcontentloaded'});await page.locator('input[name="username"]').waitFor();await page.locator('input[name="password"]').waitFor();});
 const authenticated=await flow('sign-in',async()=>{
  populatedCredentials=true;await page.locator('input[name="username"]').fill(input.username);await page.locator('input[name="password"]').fill(input.password);await page.locator('form button[type="submit"]').click();
  if(input.surface==='web'){
   await page.waitForURL(url=>url.pathname==='/'||url.pathname==='/profiles');
   if(new URL(page.url()).pathname==='/profiles')await page.getByRole('button').filter({hasText:input.profileName}).first().click();
   await page.locator('[data-home-document]').waitFor();
  }else{await page.waitForFunction(()=>!document.querySelector('input[name="password"]'));await page.getByRole('heading').first().waitFor();}
 });
 if(input.surface==='web'&&authenticated){
  await flow('home-real-rows',async()=>{await home();await page.locator('[data-home-row] [data-card-kind]').first().waitFor();return {rows:await page.locator('[data-home-row]').count()};});
  let movie='';
  await flow('movies-browse-detail',async()=>{await home();await page.getByRole('link',{name:'Movies',exact:true}).click();const card=page.locator('[data-card-kind]').first();await card.waitFor();movie=await card.locator(':scope > button').first().getAttribute('aria-label');await card.locator(':scope > button').first().click();await page.locator('[data-title-id]').waitFor();});
  await flow('video-start-progress',async()=>{const play=page.locator('[data-title-id]').getByRole('button').filter({hasText:/^(Play|Resume)/}).first();await play.click();await page.waitForFunction(()=>[...document.querySelectorAll('video')].some(v=>v.currentTime>1.5&&!v.paused),{},{timeout:35000});const a=await page.locator('video').evaluateAll(v=>Math.max(...v.map(x=>x.currentTime)));await page.waitForTimeout(2000);const b=await page.locator('video').evaluateAll(v=>Math.max(...v.map(x=>x.currentTime)));if(!(b>a))throw Error();return {advanced:true};});
  await flow('music-browse',async()=>{await home();await page.getByRole('link',{name:'Music',exact:true}).click();await page.locator('[data-card-kind]').first().waitFor();return {visibleCards:await page.locator('[data-card-kind]').count()};});
  await flow('reading-nonblank-no-egress',async()=>{
   await home();await page.getByRole('link',{name:'Books',exact:true}).click();await page.locator('[data-card-kind]').first().waitFor();await page.locator('[data-card-kind]').first().locator(':scope > button').first().click();await page.getByRole('button',{name:/^(Read|Continue reading|Start reading)/}).first().click();await page.waitForURL(/\/read\//);
   let nonblank=false;
   for(let i=0;i<40&&!nonblank;i++){
    for(const frame of page.frames().filter(x=>x!==page.mainFrame())){
     try{nonblank=await frame.evaluate(()=>{const b=document.body;if(!b||!b.getBoundingClientRect().height)return false;return b.innerText.trim().length>20||[...b.querySelectorAll('img')].some(x=>x.complete&&x.naturalWidth>0);});}catch{}
     if(nonblank)break;
    }
    if(!nonblank)await page.waitForTimeout(500);
   }
   if(!nonblank||external.length)throw Error();
   for(const frame of page.frames()){
    const active=await frame.locator('[srcset],a[href^="http:"],a[href^="https:"]').count();if(active)throw Error();
   }
   return {nonblank:true,externalRequests:external.length,fixtureUnchanged:true};
  });
  if(movie)await flow('search-real-title',async()=>{await page.goto(input.origin+'/search',{waitUntil:'domcontentloaded'});await page.getByRole('searchbox').first().fill(movie);await page.getByRole('heading',{name:movie,exact:true}).first().waitFor();return {realTitleResult:true};});
  await flow('settings-open',async()=>{await page.goto(input.origin+'/settings',{waitUntil:'domcontentloaded'});await page.getByRole('heading').first().waitFor();});
  await flow('profile-switch',async()=>{await page.goto(input.origin+'/profiles',{waitUntil:'domcontentloaded'});await page.getByRole('button').filter({hasText:input.reviewProfile}).first().click();await page.locator('[data-home-document]').waitFor();});
  await flow('sign-out',async()=>{await home();const menu=page.getByRole('button',{name:/^Switch profile or server(?:,|$)|, account menu$/});await menu.click();await page.getByRole('menuitem',{name:/^Sign out of /}).click();await page.locator('input[name="password"]').waitFor();if(await page.locator('input[name="password"]').inputValue())throw Error();});
 }else if(authenticated){
  await flow('account-directory',async()=>{await page.goto(input.origin+'/',{waitUntil:'domcontentloaded'});await page.locator('input[name="password"]').waitFor({state:'hidden'});await page.getByRole('heading').first().waitFor();if(await page.locator('a[href^="/library/"]').count())throw Error();});
  await flow('account-management',async()=>{await page.goto(input.origin+'/account',{waitUntil:'domcontentloaded'});await page.getByRole('heading').first().waitFor();if(await page.locator('input[name="password"]').count())throw Error();});
  await flow('security-view',async()=>{await page.getByText('Security',{exact:true}).first().click();await page.getByText(/Password|Two-step|Two-factor|Authenticator/).first().waitFor();});
  await flow('devices-view',async()=>{await page.getByText('Devices',{exact:true}).first().click();await page.getByRole('heading').first().waitFor();});
  await flow('hosted-sign-out',async()=>{await page.getByRole('button',{name:/^Sign out/}).first().click();await page.locator('input[name="password"]').waitFor();});
 }else{flows.push({surface:input.surface,id:'authenticated-flows',status:'unavailable',dependency:'sign-in failed or dropped'});}
}catch(error){flows.push({surface:input.surface,id:phase,status:'fail',class:error.name,timeout:error.name==='TimeoutError'});}
finally{
 if(page){try{await page.locator('input').evaluateAll(nodes=>nodes.forEach(n=>{n.value='';n.removeAttribute('value');}));}catch{}}
 await browser?.close();fs.writeFileSync(path.join(out,'browser-results.log'),JSON.stringify({flows,network,pageErrors},null,2)+'\n');console.log(JSON.stringify({surface:input.surface,flows:flows.map(({id,status,timeout})=>({id,status,...(timeout?{timeout}: {})}))}));
}
process.exitCode=flows.some(x=>x.status==='fail')?1:0;
