import fs from 'node:fs';
import {pathToFileURL} from 'node:url';
import {ToolError} from './cli.mjs';

/** Use an existing Playwright install by reference; never add one to a repo. */
export async function loadPlaywright(){
 const explicit=process.env.PORTICO_WALKS_PLAYWRIGHT;
 if(explicit){
  if(!fs.existsSync(explicit))throw new ToolError('playwright-unavailable','PORTICO_WALKS_PLAYWRIGHT is missing');
  return import(pathToFileURL(explicit).href);
 }
 try{return await import('playwright');}catch{throw new ToolError('playwright-unavailable','set PORTICO_WALKS_PLAYWRIGHT');}
}

export async function launch(){
 const {chromium}=await loadPlaywright();
 const executablePath=process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||process.env.PORTICO_WALKS_CHROMIUM||undefined;
 try{
  return await chromium.launch({headless:true,executablePath,args:['--no-sandbox','--disable-dev-shm-usage','--disable-background-networking','--force-color-profile=srgb']});
 }catch{throw new ToolError('browser-launch');}
}

/** One isolated context. The design side allows only local files; the app side
 * filters page requests by origin without recording URLs. When normal app service
 * workers are enabled, their requests are outside this page-route observation;
 * the style tool never presents it as an egress qualification. */
export async function openContext(browser,{width,height,allowOrigin=null,storageState,serviceWorkers='block'}){
 let context;
 try{
  context=await browser.newContext({viewport:{width,height},deviceScaleFactor:1,locale:'en-US',timezoneId:'UTC',colorScheme:'dark',reducedMotion:'reduce',serviceWorkers,...(storageState?{storageState}:{})});
 }catch{throw new ToolError('context-create');}
 const blocked={count:0};
 await context.route('**/*',route=>{
  const url=route.request().url();
  if(/^(?:file|data|blob|about):/i.test(url))return route.continue().catch(()=>{});
  try{if(allowOrigin&&new URL(url).origin===allowOrigin)return route.continue().catch(()=>{});}catch{}
  blocked.count++;
  return route.abort().catch(()=>{});
 });
 return {context,blocked,serviceWorkers};
}

export async function openPage(context){
 const page=await context.newPage();
 page.setDefaultTimeout(20000);
 return page;
}

/** Add the app's own Manrope face locally, so type measurements never depend on
 * a remote font service and no request leaves the machine. */
export async function installFont(page,font){
 if(!font)return;
 const css='@font-face{font-family:Manrope;font-style:normal;font-weight:100 900;font-display:block;src:url(data:font/woff2;base64,'+font.bytes.toString('base64')+') format("woff2")}';
 await page.addStyleTag({content:css});
}

export async function settle(page){
 await page.evaluate(async()=>{
  await document.fonts.ready;
  await Promise.all([...document.images].map(image=>image.complete?undefined:new Promise(resolve=>{
   image.addEventListener('load',resolve,{once:true});
   image.addEventListener('error',resolve,{once:true});
  })));
  await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
 });
}

export async function requireManrope(page){
 const loaded=await page.evaluate(()=>[...document.fonts].some(face=>face.family.replace(/["']/g,'')==='Manrope'&&face.status==='loaded')&&['400','600','800'].every(weight=>document.fonts.check(weight+' 16px Manrope')));
 if(!loaded)throw new ToolError('manrope-unavailable');
}

/** Owner-only storage state for a signed-in walk account. The path is used by
 * reference only and is never printed or copied. */
export function checkStorageState(file){
 if(!file)return undefined;
 let stat;
 try{stat=fs.lstatSync(file);}catch{throw new ToolError('storage-state-missing');}
 if(stat.isSymbolicLink()||!stat.isFile()||stat.uid!==process.getuid()||(stat.mode&0o077)!==0)throw new ToolError('storage-state-permissions');
 return file;
}
