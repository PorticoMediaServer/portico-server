#!/usr/bin/env node
import assert from 'node:assert/strict';
import {readFileSync, writeFileSync, copyFileSync} from 'node:fs';
import path from 'node:path';
import {ChromeDriver} from './chrome-driver.mjs';
import {fixtureApi} from './local-fixture-api.mjs';
const output=process.env.PORTICO_BROWSER_OUTPUT;
if(!output)throw new Error('Set PORTICO_BROWSER_OUTPUT');
const base=process.env.PORTICO_BROWSER_BASE||'http://127.0.0.1:32761';
const api=fixtureApi(base,output);
const driver=await ChromeDriver.connect(Number(process.env.PORTICO_CHROME_PORT||32762),output);
const libraries=JSON.parse(readFileSync(path.join(output,'libraries.json')));
const results=[];
const mode=process.argv[2]||'subtitles';
const pause=ms=>new Promise(resolve=>setTimeout(resolve,ms));
async function go(route){await driver.evaluate(`history.pushState({},'',${JSON.stringify(route)});dispatchEvent(new PopStateEvent('popstate'));`);await pause(500);}
async function step(name,run){
 const before=driver.events.length;
 try{const evidence=await run();const screenshot=await driver.screenshot(name);results.push({name,status:'pass',evidence,screenshot,events:driver.events.slice(before)});console.log('pass',name);}
 catch(error){results.push({name,status:'fail',error:String(error),screenshot:await driver.screenshot(name+'-failed'),screen:await driver.inspect(),events:driver.events.slice(before)});console.error('fail',name,String(error));}
 writeFileSync(path.join(output,'actions-'+mode+'.json'),JSON.stringify(results,null,2));
}
async function startFilm(container='MP4'){
 const lib=libraries.find(l=>l.kind==='movie');
 await go(`/library/${lib.id}?view=browse`);
 await driver.waitFor(`location.pathname.startsWith('/library/') && [...document.querySelectorAll('a')].some(a=>a.getAttribute('aria-label')?.startsWith('Browser Film'))`);
 const filmLabel=await driver.evaluate(`[...document.querySelectorAll('a')].find(a=>a.getAttribute('aria-label')?.startsWith('Browser Film')).getAttribute('aria-label')`);
 await driver.clickText(filmLabel,'a');
 await driver.waitFor(`location.pathname.startsWith('/media/')`);
 await driver.waitFor(`document.querySelector('select')`);
 const version=await driver.evaluate(`[...document.querySelector('select').options].find(o=>o.label.includes(${JSON.stringify(container)})).value`);
 await driver.fill('select',version);await pause(150);
 const label=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.innerText==='Play'||b.innerText.startsWith('Resume (')).innerText`);
 await driver.clickText(label,'button');
 await driver.waitFor(`document.querySelector('video')?.currentTime>0&&!document.querySelector('video').paused`,30000);
 await driver.clickText('Pause');await driver.waitFor(`document.querySelector('video').paused`);
 // Use the actual player's seek control to put every generated cue on screen.
 await driver.fill('input[aria-label="Playback position"]','3');await pause(250);
}
async function chooseSubtitle(title){
 const label=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.getAttribute('aria-label')?.startsWith('Subtitles:'))?.getAttribute('aria-label')`);
 if(label) {await driver.clickText(label);}
 else {
  if(!await driver.evaluate(`!!document.querySelector('[role="dialog"]')`))await driver.clickText('Playback options');
  await driver.waitFor(`document.querySelector('[role="dialog"]')`);
  if(!await driver.evaluate(`document.querySelector('[role="dialog"] details:nth-child(2)').open`))await driver.clickSelector('[role="dialog"] details:nth-child(2) summary');
 }
 await pause(150);
 const option=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.getClientRects().length && b.innerText.startsWith(${JSON.stringify('English · '+title)})).innerText`);
 assert(!option.includes('Converts'),'Device-rendered subtitle must not claim video conversion');
 await driver.clickText(option,'button');
 await driver.evaluate(`document.dispatchEvent(new Event('visibilitychange'))`);
 await pause(900);
 if(await driver.evaluate(`!!document.querySelector('[role="dialog"]')`))await driver.clickText('Close','[role="dialog"] button');
}
const canvasPixels=`[...document.querySelectorAll('canvas')].some(c=>c.getContext('2d')?.getImageData(0,0,c.width,c.height).data.some((v,i)=>i%4===3&&v>0))`;
try{
 if(mode==='subtitles'){
  await driver.viewport(1440,1000);
  await driver.navigate(base);await driver.waitFor(`document.body.innerText.includes('Recently added')`);
  await startFilm();
  for(const [viewport,width,height] of [['desktop',1440,1000],['phone',390,844]]){
   await driver.viewport(width,height);
   await step(viewport+'-text-subtitle',async()=>{await chooseSubtitle('text');await driver.waitFor(`[...document.querySelector('video').textTracks].some(t=>[...t.activeCues||[]].some(c=>c.text.includes('Browser text subtitle')))`);return {activeTextCue:true};});
   await step(viewport+'-styled-subtitle',async()=>{await chooseSubtitle('styled');await driver.waitFor(canvasPixels,30000);return {painted:true,workerLoaded:await driver.evaluate(`!!window.SubtitlesOctopus`)};});
   await step(viewport+'-image-subtitle',async()=>{await chooseSubtitle('image');await driver.waitFor(canvasPixels,30000);const plan=[...driver.responses.values()].find(p=>p.renderer==='client'&&p.selected?.title==='image');assert(plan,'Server must hand the image to the browser renderer');return {painted:true,renderer:plan.renderer,format:plan.selected.format};});
  }
  if(await driver.evaluate(`!!document.querySelector('[role="dialog"]')`))await driver.clickText('Close','[role="dialog"] button');
  await driver.clickText('Close player');await driver.waitFor(`!document.querySelector('video')`);
  await driver.viewport(1440,1000);
  await step('desktop-fixed-quality',async()=>{
   await startFilm('MKV');
   const label=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.getAttribute('aria-label')?.startsWith('Quality:')).getAttribute('aria-label')`);
   await driver.clickText(label);await pause(100);
   const choice=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.innerText.startsWith('480p')).innerText`);
   await driver.clickText(choice,'button');
   await driver.waitFor(`document.querySelector('button[aria-label="Quality: 480p"]')`);
   await driver.clickText('Play','button[aria-label="Play"]');
   await driver.waitFor(`document.querySelector('video').videoHeight===480&&document.querySelector('video').currentTime>4`,30000);
   await driver.clickText('Pause');await driver.waitFor(`document.querySelector('video').paused`);
   return await driver.evaluate(`({height:document.querySelector('video').videoHeight,time:document.querySelector('video').currentTime,ready:document.querySelector('video').readyState})`);
  });
  await driver.clickText('Close player');
 }
 if(mode==='groups'){
  await driver.navigate(base);await driver.waitFor(`document.querySelector('button[class*="avatarItem"]')`);
  for(const [viewport,width,height] of [['desktop',1440,1000],['phone',390,844]]){
   await driver.viewport(width,height);
   for(const [kind,view,title,expected] of [['music','artists','Browser Artist','Browser Song'],['music','releases','Browser Album','Browser Song'],['audiobook','authors','Browser Author','Browser Book']]){
    await step(viewport+'-'+view+'-group',async()=>{
     const lib=libraries.find(l=>l.kind===kind);await go(`/library/${lib.id}?view=${view}`);
     await driver.waitFor(`[...document.querySelectorAll('a')].some(a=>a.getAttribute('aria-label')?.startsWith(${JSON.stringify(title)}))`);
     const label=await driver.evaluate(`[...document.querySelectorAll('a')].find(a=>a.getAttribute('aria-label')?.startsWith(${JSON.stringify(title)})).getAttribute('aria-label')`);
     await driver.clickText(label,'a');await driver.waitFor(`document.body.innerText.includes(${JSON.stringify(expected)})`,15000);
     return {route:await driver.evaluate('location.pathname'),member:expected};
    });
   }
   await step(viewport+'-book-author-context',async()=>{
    const lib=libraries.find(l=>l.kind==='audiobook');await go(`/library/${lib.id}?view=browse`);
    await driver.waitFor(`[...document.querySelectorAll('a')].some(a=>a.getAttribute('aria-label')?.startsWith('Browser Book'))`);
    const label=await driver.evaluate(`[...document.querySelectorAll('a')].find(a=>a.getAttribute('aria-label')?.startsWith('Browser Book')).getAttribute('aria-label')`);
    await driver.clickText(label,'a');await driver.waitFor(`location.pathname.includes('/book/') && document.body.innerText.includes('Author & series')`);
    await driver.clickText('Browser Author','button');await driver.waitFor(`location.pathname.includes('/author/') && document.body.innerText.includes('Browser Book')`);
    return {authorContextOpensBook:true};
   });
  }
 }
 if(mode==='tokens'){
  await driver.viewport(1440,1000);await driver.navigate(base);await driver.waitFor(`document.querySelector('button[class*="avatarItem"]')`);
  const member={username:'member-'+crypto.randomUUID().slice(0,8),password:'Test-'+crypto.randomUUID()+'9x'};
  writeFileSync(path.join(output,'member-credentials.json'),JSON.stringify(member),{mode:0o600});
  const owner=JSON.parse(readFileSync(path.join(output,'credentials.json')));
  async function signIn(credentials){
   await driver.clickSelector('button[class*="avatarItem"]');await driver.clickText('Sign out','[role="menuitem"]');await driver.waitFor(`document.querySelector('input[autocomplete="username"]')`);
   await driver.fill('input[autocomplete="username"]',credentials.username);await driver.fill('input[autocomplete="current-password"]',credentials.password);await driver.clickText('Sign in','button[type="submit"]');
   await driver.waitFor(`document.querySelector('button[class*="avatarItem"]')||document.body.innerText.includes('Who’s watching')`);
   if(await driver.evaluate(`document.body.innerText.includes('Who’s watching')`)){
    const option=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.innerText.includes(${JSON.stringify(credentials.username)})).innerText.trim()`);await driver.clickText(option,'button');
   }
   await driver.waitFor(`document.querySelector('button[class*="avatarItem"]')`);
  }
  await step('desktop-member-account-created',async()=>{
   await go('/settings/server-people');await driver.waitFor(`document.body.innerText.includes('Add account')`);await driver.clickText('Add account');await driver.waitFor(`document.querySelector('[role="dialog"] input')`);
   const fields=await driver.evaluate(`[...document.querySelectorAll('[role="dialog"] input')].map(i=>i.id)`);
   await driver.fill('#'+fields[0],member.username);await driver.fill('#'+fields[1],'Browser Member');await driver.fill('#'+fields[2],member.password);await pause(120);await driver.clickText('Add account','[role="dialog"] button');await driver.waitFor(`!document.querySelector('[role="dialog"]')`);return {created:true};
  });
  await step('desktop-member-token-issued',async()=>{
   await signIn(member);await go('/settings/profile');await driver.waitFor(`document.body.innerText.includes('Personal access tokens')`);await driver.clickText('Personal access tokens','summary');await driver.waitFor(`document.body.innerText.includes('Create a token')`);
   await driver.clickText('Create a token');await driver.waitFor(`document.querySelector('[role="dialog"] input')`);await driver.fill('[role="dialog"] input','Browser member token');await pause(120);await driver.clickText('Create','[role="dialog"] button');await driver.waitFor(`document.querySelector('[role="dialog"] textarea')`);
   const secret=await driver.evaluate(`document.querySelector('[role="dialog"] textarea').value`);
   const read=await fetch(base+'/v1/libraries',{headers:{Authorization:'Bearer '+secret}});assert.equal(read.status,200);await driver.screenshot('desktop-member-token-secret');await driver.clickText('I’ve copied it');await driver.waitFor(`!document.querySelector('[role="dialog"]')`);return {created:true,readScopeWorks:true};
  });
  await step('desktop-owner-member-token-revoked',async()=>{
   await signIn(owner);await go('/settings/server-integrations?tab=tokens');await driver.waitFor(`document.body.innerText.includes('Browser member token')`);await driver.screenshot('desktop-owner-member-token');await driver.clickText('Revoke');await driver.waitFor(`document.querySelector('[role="dialog"]')`);await driver.clickText('Revoke','[role="dialog"] button');await driver.waitFor(`!document.querySelector('[role="dialog"]')`);return {revoked:true};
  });
 }
 if(mode==='downloads'){
  await driver.viewport(1440,1000);await driver.navigate(base);await driver.waitFor(`document.body.innerText.includes('Recently added')`);
  await step('desktop-download-prepared',async()=>{
   const library=libraries.find(l=>l.kind==='movie');await go(`/library/${library.id}?view=browse`);await driver.waitFor(`document.querySelector('a[aria-label^="Browser Film"]')`);const label=await driver.evaluate(`document.querySelector('a[aria-label^="Browser Film"]').getAttribute('aria-label')`);await driver.clickText(label,'a');await driver.waitFor(`document.querySelector('button[aria-label="More actions"]')`);
   await driver.clickText('More actions');await driver.waitFor(`document.querySelector('[role="menuitem"]')`);await driver.clickText('Download…','[role="menuitem"]');await driver.waitFor(`document.querySelector('[role="dialog"]')`);
   await driver.waitFor(`[...document.querySelectorAll('[role="dialog"] button')].some(b=>b.innerText.startsWith('Original'))`);const original=await driver.evaluate(`[...document.querySelectorAll('[role="dialog"] button')].find(b=>b.innerText.startsWith('Original')).innerText.trim()`);await driver.clickText(original,'[role="dialog"] button');await driver.waitFor(`document.querySelector('a[href="/downloads"]')`);await driver.clickText('View downloads','a');await driver.waitFor(`document.body.innerText.includes('Ready to save')&&!document.querySelector('[role="dialog"]')`);return {readyToSave:true};
  });
  await step('desktop-download-saved',async()=>{
   const directory=path.join(output,'saved-downloads');await driver.send('Browser.setDownloadBehavior',{behavior:'allow',downloadPath:directory,eventsEnabled:true});await driver.clickText('Save file');
   const {readdirSync,statSync}=await import('node:fs');for(let i=0;i<80;i++){let files=[];try{files=readdirSync(directory).filter(f=>!f.endsWith('.crdownload'));}catch{}if(files.length){const bytes=statSync(path.join(directory,files[0])).size;assert(bytes>1000);return {file:files[0],bytes};}await pause(150);}throw new Error('The original download did not finish');
  });
  await driver.viewport(390,844);await driver.screenshot('phone-download-prepared');
 }
 if(mode==='csp'){
  await driver.viewport(1440,1000);await driver.navigate(base+'/settings/server-integrations?tab=keys');await driver.waitFor(`document.body.innerText.includes('Create a key')`);
  await step('desktop-dialog-csp',async()=>{
   await driver.clickText('Create a key');await driver.waitFor(`document.querySelector('[role="dialog"]')`);
   const state=await driver.evaluate(`({nonce:document.querySelector('meta[name="portico-style-nonce"]').content,global:window.__webpack_nonce__,overflow:getComputedStyle(document.body).overflow,accepted:[...document.querySelectorAll('style')].some(s=>s.nonce&&s.sheet)})`);
   assert.equal(state.nonce,state.global);assert.equal(state.overflow,'hidden');assert(state.accepted);await driver.clickText('Cancel','[role="dialog"] button');return state;
  });
  let keySecret;
  await step('desktop-api-key-issued',async()=>{
   await driver.clickText('Create a key');await driver.waitFor(`document.querySelector('[role="dialog"] input')`);await driver.fill('[role="dialog"] input','Browser integration key');await pause(120);await driver.clickText('Create','[role="dialog"] button');await driver.waitFor(`document.querySelector('[role="dialog"] textarea')`);
   keySecret=await driver.evaluate(`document.querySelector('[role="dialog"] textarea').value`);
   const read=await fetch(base+'/v1/libraries',{headers:{Authorization:'Bearer '+keySecret}});assert.equal(read.status,200);
   await driver.screenshot('desktop-api-key-secret');await driver.clickText('I’ve copied it');await driver.waitFor(`!document.querySelector('[role="dialog"]')`);return {created:true,readScopeWorks:true};
  });
  await step('desktop-api-key-revoked',async()=>{
   await driver.waitFor(`document.body.innerText.includes('Browser integration key')`);await driver.clickText('Revoke');await driver.waitFor(`document.querySelector('[role="dialog"]')`);await driver.clickText('Revoke','[role="dialog"] button');await driver.waitFor(`!document.querySelector('[role="dialog"]')`);
   if(keySecret){const denied=await fetch(base+'/v1/libraries',{headers:{Authorization:'Bearer '+keySecret}});assert.equal(denied.status,401);}
   return {revoked:true};
  });
  await step('desktop-untrusted-style-refused',async()=>{
   const refused=await driver.evaluate(`(()=>{const style=document.createElement('style');style.textContent='.portico-csp-probe {color:rgb(1, 2, 3)}';document.head.append(style);const denied=!style.sheet;style.remove();return denied;})()`);
   assert(refused);return {arbitraryInlineStyleBlocked:true,expectedPolicyViolation:true};
  });
 }
 if(mode==='profiles'){
  const credentials=JSON.parse(readFileSync(path.join(output,'credentials.json')));
  await driver.viewport(1440,1000);await driver.navigate(base);await driver.waitFor(`document.body.innerText.includes('Recently added')`);
  await step('desktop-profile-created',async()=>{
   await go('/settings/profile');await driver.waitFor(`document.body.innerText.includes('Add profile')`);
   if(await driver.evaluate(`document.body.innerText.includes('Browser Viewer')`))return {alreadyCreated:true};
   await driver.clickText('Add profile');await driver.waitFor(`document.querySelector('[role="dialog"] input')`);
   await driver.fill('[role="dialog"] input','Browser Viewer');await driver.clickText('Save','[role="dialog"] button');await driver.waitFor(`!document.querySelector('[role="dialog"]')&&document.body.innerText.includes('Browser Viewer')`);
   return {created:true};
  });
  for(const [viewport,width,height] of [['desktop',1440,1000],['phone',390,844]]){
   await driver.viewport(width,height);
   await step(viewport+'-profile-switched',async()=>{
    const eventStart=driver.events.length;
    await driver.clickSelector(viewport==='desktop'?'button[class*="avatarItem"]':'button[class*="topAvatar"]');await driver.waitFor(`document.querySelector('[role="menuitem"]')`);await driver.clickText('Switch profile…','[role="menuitem"]');
    await driver.waitFor(`document.querySelector('[role="dialog"]')?.innerText.includes('Browser Viewer')`);
    const option=await driver.evaluate(`[...document.querySelectorAll('[role="dialog"] button')].find(b=>b.innerText.includes('Browser Viewer')).innerText.trim()`);await driver.clickText(option,'[role="dialog"] button');
    await driver.waitFor(`!document.querySelector('[role="dialog"]')`);await pause(450);
    await go('/settings/profile');await driver.waitFor(`document.body.innerText.includes('Browser Viewer')`);
    assert(!await driver.evaluate(`document.body.innerText.includes('Two-step verification')||document.body.innerText.includes('Change password')`),'Secondary profiles must not receive account-management controls');
    assert(!driver.events.slice(eventStart).some(e=>e.kind==='http'&&e.status===403),'Profile switch must not send forbidden account requests');
    await driver.screenshot(viewport+'-viewer-profile');
    await driver.clickSelector(viewport==='desktop'?'button[class*="avatarItem"]':'button[class*="topAvatar"]');await driver.clickText('Switch profile…','[role="menuitem"]');await driver.waitFor(`document.querySelector('[role="dialog"]')?.innerText.includes('Browser Viewer')`);
    const owner=await driver.evaluate(`[...document.querySelectorAll('[role="dialog"] button')].find(b=>b.innerText.includes(${JSON.stringify(credentials.username)})).innerText.trim()`);await driver.clickText(owner,'[role="dialog"] button');await driver.waitFor(`!document.querySelector('[role="dialog"]')`);
    await pause(450);
    assert(!driver.events.slice(eventStart).some(e=>e.kind==='http'&&e.status===403),'Returning from another profile must respect the server account grant');
    return {switchedToViewerAndBack:true};
   });
  }
  const previous=await api('/v1/admin/connectivity/policy');
  await api('/v1/admin/connectivity/policy','PATCH',{expectedRevision:previous.revision,operationId:crypto.randomUUID(),lanNetworks:['10.0.0.0/8'],lanDiscoveryEnabled:false});
  try{
   await step('phone-plain-http-sign-in',async()=>{
    await driver.clickSelector('button[class*="topAvatar"]');await driver.clickText('Sign out','[role="menuitem"]');await driver.waitFor(`document.querySelector('input[autocomplete="username"]')`);
    await driver.navigate(base);await driver.waitFor(`document.querySelector('input[autocomplete="username"]')`);
    await driver.waitFor(`document.body.innerText.includes('This connection uses HTTP outside your home networks.')`);
    await driver.screenshot('phone-plain-http-note');
    await driver.fill('input[autocomplete="username"]',credentials.username);await driver.fill('input[autocomplete="current-password"]',credentials.password);
    await driver.clickText('Sign in','button[type="submit"]');await driver.waitFor(`document.body.innerText.includes('Who’s watching')`);
    const option=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.innerText.includes(${JSON.stringify(credentials.username)})).innerText.trim()`);await driver.clickText(option,'button');await driver.waitFor(`document.body.innerText.includes('Recently added')`);
    return {httpNoteShown:true,signedIn:true};
   });
  }finally{const current=await api('/v1/admin/connectivity/policy');await api('/v1/admin/connectivity/policy','PATCH',{expectedRevision:current.revision,operationId:crypto.randomUUID(),lanNetworks:previous.lanNetworks,lanDiscoveryEnabled:false});}
 }
 if(mode==='media'){
  await driver.viewport(1440,1000);await driver.navigate(base);await driver.waitFor(`document.body.innerText.includes('Recently added')`);
  await step('desktop-next-episode',async()=>{
   const tv=libraries.find(l=>l.kind==='tv');await go(`/library/${tv.id}?view=browse`);await driver.waitFor(`document.querySelector('a[aria-label="Browser Show"]')`);await driver.clickText('Browser Show','a');
   await driver.waitFor(`location.pathname.startsWith('/show/')&&[...document.querySelectorAll('button')].some(b=>b.innerText.startsWith('Play S')||b.innerText==='Play')`);
   const label=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.innerText.startsWith('Play S')||b.innerText==='Play').innerText`);await driver.clickText(label,'button');
   await driver.waitFor(`document.querySelector('video')?.currentTime>1`,30000);
   const first=await driver.evaluate(`document.querySelector('video').currentSrc`);await driver.screenshot('desktop-first-episode');
   await driver.clickText('Next');await driver.waitFor(`document.querySelector('video').currentSrc!==${JSON.stringify(first)}&&document.querySelector('video').currentTime>1`,30000);
   await driver.clickText('Pause');await driver.waitFor(`document.querySelector('video').paused`);
   const text=await driver.evaluate('document.body.innerText');assert(/Episode 2|Second Episode/.test(text));
   return {nextEpisodeAdvanced:true};
  });
  if(await driver.evaluate(`!!document.querySelector('video')`))await driver.clickText('Close player');
  await step('desktop-music-playing',async()=>{
   const music=libraries.find(l=>l.kind==='music');await go(`/library/${music.id}?view=songs`);await driver.waitFor(`document.querySelector('button[aria-label="Play Browser Song"]')`);await driver.clickText('Play Browser Song');
   await driver.waitFor(`document.body.innerText.includes('Browser Song')&&document.querySelector('button[aria-label="Pause"]')`);
   const before=await driver.evaluate(`[...document.querySelectorAll('input[type="range"]')].map(e=>({label:e.getAttribute('aria-label'),value:e.value}))`);
   await pause(1600);const after=await driver.evaluate(`[...document.querySelectorAll('input[type="range"]')].map(e=>({label:e.getAttribute('aria-label'),value:e.value}))`);
   assert(after.some((e,i)=>Number(e.value)>Number(before[i]?.value)),'Music position must advance');
   await driver.screenshot('desktop-music-mini');
   const open=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.getAttribute('aria-label')?.includes('Now Playing')||b.getAttribute('aria-label')?.includes('Now playing'))?.getAttribute('aria-label')`);
   if(open)await driver.clickText(open);
   await driver.viewport(390,844);await driver.screenshot('phone-music-playing');
   await driver.clickText('Pause');return {positionAdvanced:true,before,after};
  });
 }
 if(mode==='console'){
  await driver.viewport(1440,1000);await driver.navigate(base);await driver.waitFor(`document.body.innerText.includes('Recently added')`);
  await step('desktop-webhook-created',async()=>{
   await go('/settings/server-integrations?tab=webhooks');await driver.waitFor(`document.body.innerText.includes('Add a webhook')`);if(await driver.evaluate(`document.body.innerText.includes('Browser webhook')`))return {alreadyCreated:true};await driver.clickText('Add a webhook');
   await driver.waitFor(`document.querySelector('[role="dialog"]')`);await driver.fill('[role="dialog"] input:not([type="password"])','Browser webhook');
   const inputs=await driver.evaluate(`[...document.querySelectorAll('[role="dialog"] input')].filter(i=>i.type==='text').map(i=>i.id)`);
   await driver.fill('#'+inputs[1],process.env.PORTICO_WEBHOOK_URL||'http://127.0.0.1:32763/events');
   const checkbox=await driver.evaluate(`[...document.querySelectorAll('[role="dialog"] label')].find(l=>l.innerText==='A library scan finishes').htmlFor`);
   await driver.clickSelector('#'+checkbox);await pause(100);
   await driver.clickText('Add a webhook','[role="dialog"] button');await driver.waitFor(`document.querySelector('[role="dialog"] textarea')`);
   await driver.screenshot('desktop-webhook-secret');await driver.clickText('I’ve copied it');await driver.waitFor(`!document.querySelector('[role="dialog"]')`);
   return {created:true};
  });
  await step('desktop-webhook-test',async()=>{
   const toggle=await driver.evaluate(`[...document.querySelectorAll('button[role="switch"]')].find(b=>b.getAttribute('aria-label')?.includes('Browser webhook')&&b.getAttribute('aria-checked')==='false')?.getAttribute('aria-label')`);if(toggle){await driver.clickText(toggle);await pause(250);}
   const label=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.getAttribute('aria-haspopup')==='menu'&&b.getAttribute('aria-label')?.includes('Browser webhook')).getAttribute('aria-label')`);
   await driver.clickText(label);await driver.waitFor(`document.querySelector('[role="menuitem"]')`);
   const testLabel=await driver.evaluate(`[...document.querySelectorAll('[role="menuitem"]')].find(e=>e.innerText.toLowerCase().includes('test')).innerText`);
   await driver.clickText(testLabel,'[role="menuitem"]');await driver.waitFor(`document.querySelector('[role="dialog"]')`);
   await driver.waitFor(`document.querySelector('[role="dialog"]').innerText.includes('Delivered')`,20000);
   await driver.clickText('Done','[role="dialog"] button');return {deliveredToLoopback:true};
  });
  await step('desktop-backup-created',async()=>{
   await go('/settings/server-storage');await driver.waitFor(`document.body.innerText.includes('Back up now')`);await driver.clickText('Back up now');
   await driver.waitFor(`document.body.innerText.includes('Restore')&&!document.body.innerText.includes('No backups yet')`,30000);
   return {backupListed:true};
  });
  await step('desktop-support-export',async()=>{
   await go('/settings/server-troubleshooting');await driver.waitFor(`document.body.innerText.includes('Prepare export')`);await driver.clickText('Prepare','button');
   await driver.waitFor(`document.body.innerText.includes('Download JSON')`,20000);return {exportPrepared:true};
  });
  for(const [viewport,width,height] of [['desktop',1440,1000],['phone',390,844]]){
   await driver.viewport(width,height);
   for(const tab of ['general','metadata','attention','scanning','advanced']){
    await step(viewport+'-library-'+tab,async()=>{
     const lib=libraries.find(l=>l.kind==='movie');await go(`/settings/server-libraries?id=${lib.id}&tab=${tab}`);await pause(650);
     const text=await driver.evaluate('document.body.innerText');assert(text.includes('Films'));assert(!text.includes('Try again'));return {loaded:true};
    });
   }
  }
 }
 if(mode==='browse'){
  await driver.navigate(base);await driver.waitFor(`document.body.innerText.includes('Recently added')`);
  for(const [viewport,width,height] of [['desktop',1440,1000],['phone',390,844]]){
   await driver.viewport(width,height);
   for(const library of libraries){
    await step(viewport+'-'+library.kind+'-sort-filter',async()=>{
     await go(`/library/${library.id}?view=browse`);await driver.waitFor(`document.querySelector('button[aria-label="Sort by"]')`);
     const sorts=[];
     for(const index of [1,2]){
      await driver.clickText('Sort by');await driver.waitFor(`document.querySelector('[role="menuitem"]')`);
      const options=await driver.evaluate(`[...document.querySelectorAll('[role="menuitem"]')].map(e=>e.innerText.trim())`);
      const label=options[Math.min(index,options.length-1)];sorts.push(label);
      await driver.clickText(label,'[role="menuitem"]');await pause(450);
      assert(await driver.evaluate(`!!new URL(location.href).searchParams.get('sort')`));
     }
     if(viewport==='phone'){await driver.clickText('Filter');await pause(150);}
     await driver.clickText('Favorites','button');await pause(350);
     if(viewport==='phone')await driver.clickText('Done','button');
     const selected=await driver.evaluate(`location.href`);assert(selected.includes('filters='));
     
     await pause(400);
     await driver.screenshot(viewport+'-'+library.kind+'-filtered');
     if(viewport==='phone'){const filterLabel=await driver.evaluate(`[...document.querySelectorAll('button')].find(b=>b.innerText.startsWith('Filter')).innerText`);await driver.clickText(filterLabel);await pause(150);}
     await driver.clickText('Favorites','button');if(viewport==='phone')await driver.clickText('Done','button');await pause(450);
     assert(!await driver.evaluate(`new URL(location.href).searchParams.has('filters')`));
     return {sorts,filter:'Favorites',cleared:true};
    });
   }
   const movie=libraries.find(l=>l.kind==='movie');
   await step(viewport+'-genre-group',async()=>{
    await go(`/library/${movie.id}?view=browse`);await driver.waitFor(`document.querySelector('a[aria-label^="Browser Film"]')`);
    const label=await driver.evaluate(`document.querySelector('a[aria-label^="Browser Film"]').getAttribute('aria-label')`);await driver.clickText(label,'a');
    await driver.waitFor(`location.pathname.startsWith('/media/')&&[...document.querySelectorAll('a')].some(a=>a.innerText==='Testing')`);await driver.clickText('Testing','a');
    await driver.waitFor(`location.pathname.startsWith('/library/')&&document.querySelector('button[aria-label="Sort by"]')`);
    assert(await driver.evaluate(`location.search.includes('filters=')`));return {sortAndFilterAvailable:true};
   });
  }
 }
 if(mode==='freshness'){
  await driver.viewport(1440,1000);
  await startFilm();if(await driver.evaluate(`!!document.querySelector('[role="dialog"]')`))await driver.clickText('Close','[role="dialog"] button');
  await driver.clickText('Close player');await driver.waitFor(`!document.querySelector('video')`);
  const route=await driver.evaluate('location.href');
  const before=await driver.evaluate(`document.querySelector('select').options.length`);
  await driver.evaluate(`window.__blankMain=0;window.__loadedAt=performance.timeOrigin;window.__watchMain=new MutationObserver(()=>{if(!document.querySelector('main')?.innerText.includes('Browser Film'))window.__blankMain++;});window.__watchMain.observe(document.querySelector('main'),{subtree:true,childList:true});`);
  await step('desktop-version-freshness',async()=>{
   const source=path.join(output,'media/movies/Browser Film (2024)/Browser Film (2024).mp4');
   copyFileSync(source,source.replace('.mp4',' 480p.mp4'));
   const started=Date.now();const lib=libraries.find(l=>l.kind==='movie');await api(`/v1/libraries/${lib.id}/scans`,'POST');
   await driver.waitFor(`document.querySelector('select').options.length>${before}`,30000);
   const state=await driver.evaluate(`({blankMain:window.__blankMain,loadedAt:window.__loadedAt,currentLoadedAt:performance.timeOrigin,url:location.href,versions:document.querySelector('select').options.length})`);
   assert.equal(state.blankMain,0);assert.equal(state.loadedAt,state.currentLoadedAt);assert.equal(state.url,route);
   return {...state,elapsedMs:Date.now()-started};
  });
  await driver.evaluate(`window.__watchMain.disconnect()`);
 }
}finally{driver.close();}
if(results.some(r=>r.status==='fail'))process.exitCode=1;
