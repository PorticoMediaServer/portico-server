/** Component-only VPS Chrome comparison against an immutable reference bridge. */
import {build} from 'vite';
import react from '@vitejs/plugin-react';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {readFile,mkdir,writeFile} from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const repo=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../..');
const tmp=path.join(process.env.TMPDIR,'caption-capture'),out=process.env.PORTICO_RUNNER_OUTPUT;
const reference=process.env.PORTICO_REFERENCE_URL,revision='812dacacc70e296ac870ec36d1a37a485eaeb8ed';
if(!reference||!out)throw new Error('explicit reference and output required');
const check=await fetch(reference);if(!check.ok||check.headers.get('x-portico-design-revision')!==revision)throw new Error('immutable reference mismatch');
await mkdir(tmp,{recursive:true});await mkdir(out,{recursive:true});
await build({configFile:false,root:repo,plugins:[react()],define:{'process.env.NODE_ENV':JSON.stringify('production')},build:{outDir:path.join(tmp,'bundle'),emptyOutDir:true,minify:false,lib:{entry:path.join(repo,'apps/desktop/test/window-chrome.fixture.tsx'),formats:['es'],fileName:()=> 'caption.js',cssFileName:'caption'}}});
const server=createServer(async(req,res)=>{
 const files={'/caption.js':['caption.js','text/javascript'],'/caption.css':['caption.css','text/css']};const file=files[req.url];
 if(!file){res.writeHead(404);res.end();return;}
 try{res.writeHead(200,{'Content-Type':file[1],'Access-Control-Allow-Origin':'*'});res.end(await readFile(path.join(tmp,'bundle',file[0])));}catch{res.writeHead(500);res.end();}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
const origin=`http://127.0.0.1:${server.address().port}`;
const browser=spawn(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',['--headless=new','--no-first-run','--no-default-browser-check',`--user-data-dir=${path.join(tmp,'profile')}`,'--remote-debugging-address=127.0.0.1','--remote-debugging-port=0','about:blank'],{stdio:['ignore','ignore','pipe']});
let socket;
try{
 const endpoint=await new Promise((resolve,reject)=>{
  let text='';const timeout=setTimeout(()=>reject(new Error('Chrome startup timeout')),20000);
  browser.once('exit',code=>{clearTimeout(timeout);reject(new Error(`Chrome startup exit ${code}: ${text.slice(-2000)}`));});
  browser.stderr.on('data',chunk=>{text+=chunk;const found=text.match(/DevTools listening on (ws:\/\/[^\s]+)/);if(found){clearTimeout(timeout);resolve(found[1]);}});
 });
 const port=new URL(endpoint).port;
 const targets=await fetch(`http://127.0.0.1:${port}/json/list`).then(r=>r.json());
 socket=new WebSocket(targets.find(t=>t.type==='page').webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.addEventListener('open',resolve,{once:true});socket.addEventListener('error',reject,{once:true});});
 let next=0;const requests=new Map(),events=new Map();
 socket.addEventListener('message',event=>{
  const value=JSON.parse(String(event.data));
  if(value.id){const pending=requests.get(value.id);if(!pending)return;requests.delete(value.id);clearTimeout(pending.timer);value.error?pending.reject(new Error(JSON.stringify(value.error))):pending.resolve(value.result);}
  else if(events.has(value.method)){const done=events.get(value.method);events.delete(value.method);done(value.params);}
 });
 const send=(method,params={})=>new Promise((resolve,reject)=>{const id=++next;const timer=setTimeout(()=>{requests.delete(id);reject(new Error(`CDP timeout ${method}`));},25000);requests.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}));});
 const evaluate=async expression=>{const value=await send('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(value.exceptionDetails)throw new Error(JSON.stringify(value.exceptionDetails));return value.result.value;};
 const capture=async clip=>{
  let previous;
  // Establish stability independently on each side. Never choose a capture by
  // whether it resembles the other side, and never relax the pixel comparison.
  for(let attempt=0;attempt<4;attempt++){
   await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');
   const current=(await send('Page.captureScreenshot',{format:'png',clip,captureBeyondViewport:true})).data;
   if(current===previous)return current;previous=current;
  }
  throw new Error('caption_capture_unstable');
 };
 await send('Page.enable');await send('Runtime.enable');
 await send('Emulation.setDeviceMetricsOverride',{width:1520,height:1100,deviceScaleFactor:1,mobile:false});
 const cases=[['f2',0,'normal'],['f2s',0,'close-hover'],['f2s',1,'maximize-hover'],['f2s',2,'maximized']];
 const results=[],pairs=[];
 for(const [frame,index,name] of cases){
  const loaded=new Promise((resolve,reject)=>{const timer=setTimeout(()=>{events.delete('Page.loadEventFired');reject(new Error('reference load timeout'));},25000);events.set('Page.loadEventFired',()=>{clearTimeout(timer);resolve();});});
  const url=new URL(reference);url.searchParams.set('f',frame);await send('Page.navigate',{url:url.href});await loaded;
  await evaluate(`document.fonts.ready.then(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))))`);
  const target=await evaluate(`(()=>{const cap=document.querySelectorAll('#${frame} .capw')[${index}];if(!cap)throw Error('reference caption absent');const box=cap.getBoundingClientRect();if(Math.abs(box.width-138)>0.01||Math.abs(box.height-32)>0.01)throw Error('scaled reference');window.__referenceCaption=cap;return {clip:{x:box.x,y:box.y,width:box.width,height:box.height,scale:1},native:{os:'windows',focused:!cap.classList.contains('off'),fullscreen:false,maximized:cap.closest('.win').classList.contains('max'),captionHover:[...cap.children].findIndex(s=>s.classList.contains('hov'))+1,captions:{minimize:'Minimize',maximize:'Maximize',restore:'Restore',close:'Close'}}};})()`);
  const expected=await capture(target.clip);
  await evaluate(`(async()=>{const link=document.createElement('link');link.rel='stylesheet';link.href=${JSON.stringify(origin+'/caption.css')};const ready=new Promise((r,j)=>{link.onload=r;link.onerror=j;});document.head.append(link);await ready;const {mountCaption}=await import(${JSON.stringify(origin+'/caption.js')});window.__referenceCaption.style.visibility='hidden';window.__captionFixture=mountCaption(window.__referenceCaption.closest('.app'),${JSON.stringify(target.native)});await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));})()`);
  const geometry=await evaluate(`window.__captionFixture.geometry()`);
  const styles=await evaluate(`(()=>{const original=window.__referenceCaption.children[1],actual=document.querySelector('[data-window-no-drag] button:nth-child(2)');const differences=(x,y)=>{const a=getComputedStyle(x),b=getComputedStyle(y),changed={};for(const property of a){if(a.getPropertyValue(property)!==b.getPropertyValue(property))changed[property]=[a.getPropertyValue(property),b.getPropertyValue(property)];}return changed;};const rect=element=>{const r=element.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}};const a=original.querySelector('use'),b=actual.querySelector('use');return {differences:differences(original,actual),useDifferences:differences(a,b),symbolDifferences:differences(document.getElementById(a.href.baseVal.slice(1)).firstElementChild,document.getElementById(b.href.baseVal.slice(1)).firstElementChild),referenceSvg:rect(original.querySelector('svg')),componentSvg:rect(actual.querySelector('svg'))};})()`);
  if(!geometry||Object.values(geometry).some(r=>r.width!==46||r.height!==32))throw new Error('production caption hit area mismatch');
  const actual=await capture(target.clip);
  const blank=new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(new Error('blank navigation timeout')),10000);events.set('Page.loadEventFired',()=>{clearTimeout(timer);resolve();});});
  await send('Page.navigate',{url:'about:blank'});await blank;
  const difference=await evaluate(`(async()=>{const load=src=>new Promise((r,j)=>{const i=new Image();i.onload=()=>r(i);i.onerror=j;i.src=src;});const [a,b]=await Promise.all([load('data:image/png;base64,${expected}'),load('data:image/png;base64,${actual}')]);document.documentElement.style.margin='0';document.body.replaceChildren();document.body.style.cssText='margin:0;display:flex;gap:0;line-height:0';document.body.append(a,b);const c=document.createElement('canvas');c.width=138;c.height=32;const ctx=c.getContext('2d',{willReadFrequently:true});ctx.drawImage(a,0,0);const x=ctx.getImageData(0,0,138,32).data;ctx.clearRect(0,0,138,32);ctx.drawImage(b,0,0);const y=ctx.getImageData(0,0,138,32).data;let pixels=0,max=0,referenceBright=0,componentBright=0;const locations=[];for(let p=0;p<x.length;p+=4){let changed=false;for(let k=0;k<4;k++){const d=Math.abs(x[p+k]-y[p+k]);max=Math.max(max,d);changed ||= d!==0;}if(changed){pixels++;if(locations.length<24)locations.push({x:p/4%138,y:Math.floor(p/4/138),reference:[...x.slice(p,p+4)],component:[...y.slice(p,p+4)]});}if(x[p]>180&&x[p+1]>180&&x[p+2]>180)referenceBright++;if(y[p]>180&&y[p+1]>180&&y[p+2]>180)componentBright++;}return {pixels,max,referenceBright,componentBright,locations};})()`);
  pairs.push([expected,actual]);
  results.push({state:name,reference:frame,index,componentOnly:true,referenceLeft:true,dimensionsEach:[138,32],geometry,styles,difference});
 }
 const pair=await evaluate(`(async()=>{document.body.replaceChildren();const canvas=document.createElement('canvas');canvas.width=276;canvas.height=128;document.body.append(canvas);const ctx=canvas.getContext('2d');const rows=${JSON.stringify(pairs)};for(let row=0;row<rows.length;row++){for(let side=0;side<2;side++){const image=new Image();await new Promise((resolve,reject)=>{image.onload=resolve;image.onerror=reject;image.src='data:image/png;base64,'+rows[row][side];});ctx.drawImage(image,side*138,row*32);}}return canvas.toDataURL('image/jpeg',1).split(',')[1];})()`);
 await writeFile(path.join(out,'caption-side-by-side.jpg'),Buffer.from(pair,'base64'));
 const report={reference,revision,scope:'COMPONENT ONLY; surrounding Home is immutable reference scaffold, not product UI. No Windows/native/Snap/full-screen acceptance.',fullScreensAccepted:0,results};
 await writeFile(path.join(out,'caption-checklist.log'),JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report));
 if(results.some(r=>r.difference.pixels!==0||r.difference.referenceBright<10||r.difference.componentBright<10))process.exitCode=1;
 try{await send('Browser.close');}catch{}
}finally{
 socket?.close();if(browser.exitCode===null){browser.kill('SIGTERM');await new Promise(resolve=>{browser.once('exit',resolve);setTimeout(()=>{if(browser.exitCode===null)browser.kill('SIGKILL');resolve();},3000);});}
 await new Promise(resolve=>server.close(resolve));
}
