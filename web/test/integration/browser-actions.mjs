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
