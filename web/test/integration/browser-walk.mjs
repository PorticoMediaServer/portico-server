#!/usr/bin/env node
/** Walk a signed-in disposable server. Launch Chrome with chrome-driver.mjs instructions,
 * set PORTICO_BROWSER_OUTPUT to scratch, and PORTICO_BROWSER_BASE to the loopback server.
 * Uses real routes, browser rendering, and original API reads; never contacts external services.
 */
import {ChromeDriver} from './chrome-driver.mjs';
import {writeFileSync, readFileSync} from 'node:fs';
import path from 'node:path';
const output=process.env.PORTICO_BROWSER_OUTPUT;
if(!output) throw new Error('Set PORTICO_BROWSER_OUTPUT to a scratch directory');
const base=process.env.PORTICO_BROWSER_BASE || 'http://127.0.0.1:32761';
if(!['127.0.0.1','localhost','[::1]'].includes(new URL(base).hostname))throw new Error('Use a disposable loopback server');
const driver=await ChromeDriver.connect(Number(process.env.PORTICO_CHROME_PORT || 32762),output);
const libraries=JSON.parse(readFileSync(path.join(output,'libraries.json')));
const results=[];
const pause=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const routes=[
 ['home','/'],['search','/search?q=Browser'],['saved','/saved'],['downloads','/downloads'],['settings','/settings'],
 ...['profile','playback','appearance','device','privacy'].map(p=>['settings-'+p,'/settings/'+p]),
 ...['dashboard','history','libraries','metadata','streaming','remote','storage','schedule','troubleshooting','general'].map(p=>['server-'+p,'/settings/server-'+p]),
 ...['keys','tokens','webhooks','assistants'].map(tab=>['integrations-'+tab,'/settings/server-integrations?tab='+tab]),
 ...['accounts','invitations','devices'].map(tab=>['people-'+tab,'/settings/server-people?tab='+tab]),
 ...['sources','recording','channels'].map(tab=>['live-'+tab,'/settings/server-live?tab='+tab]),
 ...libraries.flatMap(l=>(l.kind==='music'?['discover','browse','releases','songs','genres','playlists']:l.kind==='audiobook'?['discover','browse','authors','series','collections']:l.kind==='movie'?['discover','browse','collections','unmatched']:['discover','browse','aired','collections','unmatched']).map(tab=>[l.kind+'-'+tab,`/library/${l.id}?view=${tab}`])),
];
try {
 for(const [viewport,width,height] of [['desktop',1440,1000],['phone',390,844]]) {
  await driver.viewport(width,height);
  for(const [name,route] of routes) {
   const before=driver.events.length;
   await driver.evaluate(`history.pushState({}, '', ${JSON.stringify(route)}); dispatchEvent(new PopStateEvent('popstate'));`);
   await pause(900);
   const screen=await driver.inspect();
   const screenshot=await driver.screenshot(viewport+'-'+name);
   const failed=/Something went wrong|couldn.t open|Unable to|Try again|Loading…/.test(screen.text) || !screen.text.trim();
   const result={viewport,name,route,screenshot,status:failed?'review':'rendered',screen,events:driver.events.slice(before)};
   results.push(result);
   writeFileSync(path.join(output,'walk-results.json'),JSON.stringify(results,null,2));
   console.log(viewport,name,result.status,result.events.filter(e=>['exception','http'].includes(e.kind)).map(e=>e.kind+' '+(e.status||'')+' '+(e.url||'')).join('; '));
  }
 }
} finally { driver.close(); }
