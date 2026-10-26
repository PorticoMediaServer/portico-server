import fs from 'node:fs';
import path from 'node:path';
import {ERROR_CLASSES,operation,readingFormat,errorCode,pageError,installCspRecorder,cspSummary} from './receipts.mjs';
import {writeLayoutReceipt} from './reading.mjs';

const MAX_NETWORK=100,MAX_PAGE_ERRORS=12,READING_FLOW='reading-nonblank-no-egress';

/** Owns the browser lifetime, per-flow records, redacted captures and the
 * bounded receipts the runner reads back. Surfaces only add flows. */
export function createSession({input,chromium,privacy,out}){
 const flows=[],network=[],pageErrors=[],external=[];
 const privateValues=[input.username,input.password].filter(Boolean);
 const disabled=new Set(input.disabled??[]);
 let browser,page,phase='startup',populatedCredentials=false,readingVerdict;

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
  phase=id;const started=Date.now();let record;
  try{const facts=await run();record={surface:input.surface,id,status:'pass',facts:facts??{}};}
  catch(error){record={surface:input.surface,id,status:'fail',class:ERROR_CLASSES.has(error?.name)?error.name:'Other',timeout:error?.name==='TimeoutError'};}
  record.durationMs=Date.now()-started;flows.push(record);
  if(id===READING_FLOW){try{await writeLayoutReceipt(page,out,readingVerdict);}catch{readingVerdict={found:false,last:{reason:'layout-receipt-unavailable'}};}}
  try{await capture(id);}catch{record.screenshotUnavailable=true;}
  return record.status==='pass';
 }

 async function start(){
  process.on('SIGTERM',async()=>{try{await browser?.close();}finally{process.exit(143);}});
  browser=await chromium.launch({executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,headless:true,args:['--no-sandbox','--disable-dev-shm-usage','--disable-background-networking']});
  const context=await browser.newContext({viewport:{width:1440,height:900}});
  await context.addInitScript(installCspRecorder);
  page=await context.newPage();page.setDefaultTimeout(12000);
  page.on('pageerror',error=>{if(pageErrors.length<MAX_PAGE_ERRORS)pageErrors.push(pageError(error,phase));});
  page.on('response',async response=>{try{
   const url=new URL(response.url());if(![input.origin,input.apiOrigin].includes(url.origin)||network.length>=MAX_NETWORK)return;
   const name=operation(url.pathname);if(!name)return;
   const row={phase,operation:name,method:response.request().method(),status:response.status()};
   if(name==='reading-sessions'&&row.method==='POST'&&row.status===200){try{row.format=readingFormat((await response.json())?.format);if(!row.format)delete row.format;}catch{}}
   if(row.status>=400){try{row.code=errorCode(await response.json());if(!row.code)delete row.code;}catch{}}
   network.push(row);
  }catch{}});
  page.on('request',request=>{try{
   if(phase!==READING_FLOW||external.length>=32)return;
   const url=new URL(request.url());
   if(!['data:','blob:','about:'].includes(url.protocol)&&url.origin!==input.origin)external.push({method:request.method(),foreignOrigin:true});
  }catch{}});
 }

 function crash(error){flows.push({surface:input.surface,id:phase,status:'fail',class:ERROR_CLASSES.has(error?.name)?error.name:'Other',timeout:error?.name==='TimeoutError'});}

 function unavailable(id,dependency){flows.push({surface:input.surface,id,status:'unavailable',dependency});}

 async function cspReceipt(){
  if(!page)return {total:0,directives:{},blockedClasses:{}};
  const entries=await page.evaluate(()=>Array.isArray(window.__porticoCheckpointCsp)?window.__porticoCheckpointCsp.slice(0,32):[]);
  return cspSummary(entries);
 }

 async function finish(){
  let csp={total:0,directives:{},blockedClasses:{}};
  try{csp=await cspReceipt();}catch{}
  if(page){try{await page.locator('input').evaluateAll(nodes=>nodes.forEach(node=>{node.value='';node.removeAttribute('value');}));}catch{}}
  try{await browser?.close();}catch{}
  const found=readingVerdict?.found===true;
  const reading=readingVerdict?{found,...(found?{chars:readingVerdict.publication.chars,lines:readingVerdict.publication.lines,images:readingVerdict.publication.images}:{reason:readingVerdict.last?.reason??'publication-not-found'})}:undefined;
  fs.writeFileSync(path.join(out,'browser-results.log'),JSON.stringify({flows,network,pageErrors,csp,...(reading?{reading}:{})},null,2)+'\n');
  console.log(JSON.stringify({surface:input.surface,flows:flows.map(({id,status,timeout})=>({id,status,...(timeout?{timeout}:{})}))}));
  process.exitCode=flows.some(record=>record.status==='fail')?1:0;
  return process.exitCode;
 }

 return {
  input,out,
  get page(){return page;},
  get phase(){return phase;},
  flow,start,crash,unavailable,finish,
  markPopulatedCredentials(){populatedCredentials=true;},
  external,
  setReading(result){readingVerdict=result;},
 };
}
