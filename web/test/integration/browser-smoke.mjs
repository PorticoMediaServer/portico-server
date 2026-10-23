#!/usr/bin/env node
/** Production browser integration with fresh disposable state and generated media.
 * PORTICO_SERVER_BIN=/absolute/prebuilt/server PORTICO_BROWSER_OUTPUT=/absolute/empty/scratch
 * node web/test/integration/browser-smoke.mjs
 * Requires the built web/dist, Python, FFmpeg and Chrome; no package installation.
 */
import {spawn} from 'node:child_process';
import {mkdirSync, writeFileSync, existsSync, openSync, readdirSync} from 'node:fs';
import path from 'node:path';
import net from 'node:net';
import http from 'node:http';
import {fileURLToPath} from 'node:url';
import {randomBytes} from 'node:crypto';
import {ChromeDriver} from './chrome-driver.mjs';
import {fixtureApi} from './local-fixture-api.mjs';
const root=fileURLToPath(new URL('../../../',import.meta.url));
const output=process.env.PORTICO_BROWSER_OUTPUT;
const binary=process.env.PORTICO_SERVER_BIN;
if(!output||!path.isAbsolute(output)||!binary||!path.isAbsolute(binary))throw new Error('Set absolute PORTICO_BROWSER_OUTPUT and PORTICO_SERVER_BIN');
if(existsSync(path.join(output,'state'))||existsSync(path.join(output,'credentials.json')))throw new Error('Choose fresh scratch; existing state is never reused or removed');
mkdirSync(output,{recursive:true});
if(readdirSync(path.join(root,'web/dist/assets')).some(name=>/^(?:Gallery|Features|WindowedLab|BrowseLab|ChannelsLab)-/.test(name)))throw new Error('Production bundle still contains developer screens');
const pause=ms=>new Promise(resolve=>setTimeout(resolve,ms));
async function freePort(){const server=net.createServer();await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));const port=server.address().port;await new Promise(resolve=>server.close(resolve));return port;}
async function command(executable,args,env=process.env,log){
 const fd=log?openSync(path.join(output,log),'w',0o600):undefined;
 const child=spawn(executable,args,{cwd:root,env,stdio:fd===undefined?'inherit':['ignore',fd,fd]});
 return await new Promise((resolve,reject)=>{child.on('error',reject);child.on('exit',code=>resolve(code));});
}
const serverPort=await freePort(),chromePort=await freePort();
const base=`http://127.0.0.1:${serverPort}`;
const env={...process.env,PORTICO_BROWSER_BASE:base,PORTICO_CHROME_PORT:String(chromePort),PORTICO_BROWSER_OUTPUT:output};
const credentials={username:'owner-'+randomBytes(3).toString('hex'),password:'Test-'+randomBytes(12).toString('base64url')+'9x'};
writeFileSync(path.join(output,'credentials.json'),JSON.stringify(credentials),{mode:0o600});
let server,chrome,driver,sink;
const summary=[];
try{
 if(await command('python3',['web/test/integration/generate-browser-media.py',path.join(output,'media')],env,'media.log'))throw new Error('Media generation failed');
 if(await command(process.execPath,['web/test/integration/generate-browser-subtitles.mjs',path.join(output,'media')],env,'image-fixture.log'))throw new Error('Image generation failed');
 const serverLog=openSync(path.join(output,'server.log'),'w',0o600);
 server=spawn(binary,[],{cwd:root,env:{...env,PORTICO_DATA_DIR:path.join(output,'state'),PORTICO_WEB_DIR:path.join(root,'web/dist'),PORTICO_BIND:`127.0.0.1:${serverPort}`,PORTICO_DISCOVERY:'0',PORTICO_FFMPEG:process.env.PORTICO_FFMPEG||'/usr/bin/ffmpeg',PORTICO_FFPROBE:process.env.PORTICO_FFPROBE||'/usr/bin/ffprobe',HTTP_PROXY:'http://127.0.0.1:9',HTTPS_PROXY:'http://127.0.0.1:9',ALL_PROXY:'http://127.0.0.1:9',NO_PROXY:'127.0.0.1,localhost'},stdio:['ignore',serverLog,serverLog]});
 for(let i=0;i<120;i++){try{const system=await(await fetch(base+'/v1/system')).json();if(system.hostedConfigured)throw new Error('Use an unconfigured local development server');if(system.setupRequired)break;}catch(e){if(e.message.includes('unconfigured'))throw e;}await pause(250);}
 const chromeLog=openSync(path.join(output,'chrome.log'),'w',0o600);
 chrome=spawn(process.env.PORTICO_CHROME_BIN||'/usr/bin/google-chrome',['--headless=new','--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--disable-background-networking','--disable-component-update','--disable-sync','--disable-default-apps','--no-first-run','--no-default-browser-check','--autoplay-policy=no-user-gesture-required','--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE localhost, EXCLUDE 127.0.0.1','--remote-debugging-address=127.0.0.1',`--remote-debugging-port=${chromePort}`,`--user-data-dir=${path.join(output,'chrome')}`,'about:blank'],{stdio:['ignore',chromeLog,chromeLog]});
 for(let i=0;i<80;i++){try{driver=await ChromeDriver.connect(chromePort,output);break;}catch{}await pause(250);}
 if(!driver)throw new Error('Chrome did not start');
 await driver.viewport(1440,1000);await driver.navigate(base);await driver.waitFor(`document.querySelector('input[autocomplete="username"]')`);
 await driver.screenshot('desktop-first-run');await driver.viewport(390,844);await driver.screenshot('phone-first-run');await driver.viewport(1440,1000);
 await driver.clickSelector('[role="radio"]',1);
 await driver.fill('input[autocomplete="username"]',credentials.username);
 const passwords=await driver.evaluate(`[...document.querySelectorAll('input[type="password"]')].map(i=>i.id)`);
 for(const id of passwords)await driver.fill('#'+id,credentials.password);
 await driver.waitFor(`!document.querySelector('button[type="submit"]').disabled`);await driver.clickText('Create owner');await driver.waitFor(`document.body.innerText.includes('Add library')`);
 await driver.screenshot('desktop-home-empty');driver.close();driver=undefined;
 summary.push({name:'first-run',status:'pass'});
 const api=fixtureApi(base,output),libraries=[];
 for(const [kind,dir,name] of [['movie','movies','Films'],['tv','tv','Television'],['anime','anime','Anime'],['music','music','Music'],['audiobook','audiobook','Audiobooks']]){
  const library=await api('/v1/libraries','POST',{kind,name,path:path.join(output,'media',dir),metadataAgent:'local'});libraries.push(library);await api(`/v1/libraries/${library.id}/scans`,'POST');
 }
 writeFileSync(path.join(output,'libraries.json'),JSON.stringify(libraries));
 if(await command('python3',['web/test/integration/generate-browser-media.py',path.join(output,'media'),'--add-version'],env,'version-fixture.log'))throw new Error('Version generation failed');
 await api(`/v1/libraries/${libraries.find(l=>l.kind==='movie').id}/scans`,'POST');
 for(let i=0;i<120;i++){const jobs=await api('/v1/admin/jobs?limit=40');if(jobs.items?.length>=5&&jobs.items.every(j=>['complete','complete_with_warnings'].includes(j.status)))break;await pause(250);}
 const webhookPort=await freePort();env.PORTICO_WEBHOOK_URL=`http://127.0.0.1:${webhookPort}/events`;
 sink=http.createServer((req,res)=>{let body='';req.on('data',d=>body+=d);req.on('end',()=>{writeFileSync(path.join(output,'webhook-received.json'),JSON.stringify({received:true,body:JSON.parse(body),signed:!!req.headers['x-portico-signature']}));res.end('ok');});});
 await new Promise(resolve=>sink.listen(webhookPort,'127.0.0.1',resolve));
 for(const mode of ['subtitles','browse','groups','freshness','console','media','profiles','tokens','downloads','csp']){
  const code=await command(process.execPath,['web/test/integration/browser-actions.mjs',mode],env,`actions-${mode}.log`);summary.push({name:mode,status:code===0?'pass':'fail',exitCode:code});writeFileSync(path.join(output,'summary.json'),JSON.stringify(summary,null,2));console.log(mode,code===0?'pass':'fail');
 }
 const walked=await command(process.execPath,['web/test/integration/browser-walk.mjs'],env,'walk.log');summary.push({name:'screen-walk',status:walked===0?'pass':'fail',exitCode:walked});
 driver=await ChromeDriver.connect(chromePort,output);await driver.viewport(1440,1000);await driver.navigate(base);await driver.waitFor(`document.querySelector('button[class*="avatarItem"]')`);await driver.clickSelector('button[class*="avatarItem"]');await driver.clickText('Sign out','[role="menuitem"]');await driver.waitFor(`document.querySelector('input[autocomplete="username"]')`);await driver.screenshot('desktop-signed-out');summary.push({name:'sign-out',status:'pass'});
 writeFileSync(path.join(output,'summary.json'),JSON.stringify(summary,null,2));
 if(summary.some(s=>s.status==='fail'))process.exitCode=1;
}finally{
 driver?.close();if(sink)await new Promise(resolve=>sink.close(resolve));chrome?.kill('SIGTERM');server?.kill('SIGTERM');
}
