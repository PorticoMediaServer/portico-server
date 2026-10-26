#!/usr/bin/env node
/** Read-only geometry for the CSS-only Hosted workspace glow (artifact
 * 7021853c7898), plus the public create/reset/verify stage geometry.
 *
 * It signs in through the ordinary Hosted UI with a credential file by
 * reference, measures boxes and exact computed gradients at two desktop
 * viewports before and after a document scroll, signs out only this browser's
 * current family through the ordinary workspace control, and writes a private
 * receipt with no account, email, label, URL or page-error text.
 *
 * The shared account is never used by default until the owner has read this
 * source: --skip-account measures only the public stages.
 */
import fs from 'node:fs';
import {ToolError,parseArgs} from '../walks/web/cli.mjs';
import {loadPlaywright,launch} from '../walks/web/browser.mjs';
import {writePrivate} from '../walks/web/paths.mjs';

const ARTIFACT='7021853c7898';
const DEFAULT_CREDENTIALS='/home/justin/projects/Portico/.private/hosted-testing/justin-testing.json';
const DEFAULT_ORIGIN='https://web.getportico.tv';
const DEFAULT_OUT='/home/justin/projects/Portico/.private/hosted-testing/geometry-'+ARTIFACT+'.json';
const DEFAULT_WIDTH=1440,DEFAULT_HEIGHTS='800,520';

/** Owner-only credential file, by reference. Values never reach output. */
function readCredentials(file){
 let stat;
 try{stat=fs.lstatSync(file);}catch{throw new ToolError('credentials-missing');}
 if(stat.isSymbolicLink()||!stat.isFile()||stat.uid!==process.getuid()||(stat.mode&0o077)!==0)throw new ToolError('credentials-permissions');
 let document;
 try{document=JSON.parse(fs.readFileSync(file,'utf8'));}catch{throw new ToolError('credentials-unreadable');}
 if(!document||typeof document.password!=='string'||!document.password)throw new ToolError('credentials-shape');
 const username=typeof document.username==='string'?document.username:'';
 const email=typeof document.email==='string'?document.email:'';
 if(!username&&!email)throw new ToolError('credentials-shape');
 return {username,email,password:document.password};
}

/** Nonpersistent context with the same closed origin guard as the walk tools. */
async function openContext(browser,{width,height,origins}){
 const context=await browser.newContext({viewport:{width,height},deviceScaleFactor:1,locale:'en-US',timezoneId:'UTC',colorScheme:'dark',reducedMotion:'reduce'});
 const blocked={count:0};
 await context.route('**/*',route=>{
  const url=route.request().url();
  if(/^(?:file|data|blob|about):/i.test(url))return route.continue().catch(()=>{});
  try{if(origins.includes(new URL(url).origin))return route.continue().catch(()=>{});}catch{}
  blocked.count++;
  return route.abort().catch(()=>{});
 });
 return {context,blocked};
}

/** Everything measured inside the page, in document coordinates. */
function measureInPage(){
 const round=value=>Math.round(Number(value)*100)/100;
 const rect=element=>{
  if(!element)return null;
  const box=element.getBoundingClientRect();
  return {top:round(box.top+scrollY),bottom:round(box.bottom+scrollY),height:round(box.height),left:round(box.left),right:round(box.right),width:round(box.width)};
 };
 const style=element=>{
  if(!element)return null;
  const computed=getComputedStyle(element);
  return {position:computed.position,inset:[computed.top,computed.right,computed.bottom,computed.left],
   backgroundImage:computed.backgroundImage,backgroundColor:computed.backgroundColor,
   opacity:round(computed.opacity),zIndex:computed.zIndex,pointerEvents:computed.pointerEvents};
 };
 const tokenOf=(element,name)=>[...element.classList].some(token=>token===name||token.startsWith('_'+name+'_'));
 const visible=element=>{
  if(!element)return false;
  const box=element.getBoundingClientRect(),computed=getComputedStyle(element);
  return box.width>1&&box.height>1&&computed.display!=='none'&&computed.visibility==='visible'&&Number(computed.opacity)>0;
 };
 const workspaces=[...document.querySelectorAll('[class*="workspace"]')].filter(element=>tokenOf(element,'workspace'));
 const glowChild=element=>[...element.children].find(child=>child.getAttribute('aria-hidden')==='true'&&tokenOf(child,'glow'))??null;
 const workspace=workspaces.find(element=>glowChild(element)&&visible(element))??workspaces.find(element=>glowChild(element))??null;
 const glow=workspace?glowChild(workspace):null;
 const stage=document.querySelector('section[class*="stage"]');
 const backdrop=stage?[...stage.children].find(element=>element.getAttribute('aria-hidden')==='true')??null:null;
 const snapshot=()=>({viewport:{width:innerWidth,height:innerHeight},scroll:{y:round(scrollY),height:document.documentElement.scrollHeight},
  workspace:rect(workspace),workspaceVisible:visible(workspace),glow:rect(glow),glowVisible:visible(glow),glowStyle:style(glow),stage:rect(stage),stageStyle:style(stage),backdrop:rect(backdrop),backdropStyle:style(backdrop)});
 const frame=()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
 return (async()=>{
  await document.fonts.ready;
  window.scrollTo(0,0);await frame();const before=snapshot();
  window.scrollTo(0,Math.max(0,document.documentElement.scrollHeight-innerHeight));await frame();const after=snapshot();
  window.scrollTo(0,0);await frame();
  return {before,after};
 })();
}

function covers(snapshot,which){
 const box=snapshot[which];
 if(!box)return {document:false,viewport:false};
 const scrollY=Number(snapshot.scroll?.y??0);
 return {document:box.top<=0.5&&box.bottom>=Number(snapshot.scroll?.height??0)-0.5,
  viewport:box.top-scrollY<=0.5&&box.bottom-scrollY>=Number(snapshot.viewport?.height??0)-0.5};
}

function summarizeGroup(snapshots,keys,coverKey){
 return Object.fromEntries(Object.entries(snapshots).map(([height,value])=>{
  const cover=covers(value.before,coverKey),coverAfter=covers(value.after,coverKey);
  return [height,{viewport:value.before.viewport,scrollBefore:value.before.scroll,scrollAfter:value.after.scroll,
   elements:Object.fromEntries(keys.map(key=>[key,{before:value.before[key],after:value.after[key],style:value.before[key+'Style']??null}])),
   visible:{before:{workspace:value.before.workspaceVisible,glow:value.before.glowVisible},after:{workspace:value.after.workspaceVisible,glow:value.after.glowVisible}},
   coversDocument:cover.document&&coverAfter.document,coversViewportBefore:cover.viewport,coversViewportAfter:coverAfter.viewport}];
 }));
}

async function measureAt(page,width,heights){
 const out={};
 for(const height of heights){
  await page.setViewportSize({width,height});
  await page.waitForTimeout(250);
  out[height]=await page.evaluate(measureInPage);
 }
 return out;
}

/** Ordinary Hosted sign-in. Stops before any MFA/verification action. */
async function signIn(page,origin,credentials,state){
 await page.goto(origin+'/',{waitUntil:'domcontentloaded'});
 const identifier=page.locator('input[type="email"],input[name="email"],input[name="username"],input[autocomplete="username"]').first();
 const password=page.locator('input[type="password"]').first();
 try{await identifier.waitFor({timeout:30000});await password.waitFor({timeout:10000});}
 catch{throw new ToolError('sign-in-form-missing');}
 await identifier.fill(credentials.username||credentials.email);
 await password.fill(credentials.password);
 state.submitted=true;
 try{await page.locator('form button[type="submit"]').first().click({timeout:15000});}
 catch{throw new ToolError('sign-in-submit-missing');}
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){
  const path=new URL(page.url()).pathname;
  if(path==='/verify-email'||await page.locator('input[autocomplete="one-time-code"],input[name="code"]').count())return 'mfa-required';
  const password=await page.locator('input[type="password"]').count();
  if(!password){
   const authenticated=await page.evaluate(()=>Boolean(document.querySelector('[class*="workspace"],a[href^="/account"],[class*="header"],[class*="directory"],[class*="start"]')));
   if(authenticated)return 'signed-in';
  }else if(await page.locator('[role="alert"]').count())return 'sign-in-failed';
  await page.waitForTimeout(250);
 }
 return 'sign-in-timeout';
}

/** The real workspace root token with its own direct glow child; never infer it
 * from a glow parent (AuthStage has its own glow-like backdrop). */
async function waitForWorkspace(page){
 await page.waitForFunction(()=>{
  const tokenOf=(element,name)=>[...element.classList].some(token=>token===name||token.startsWith('_'+name+'_'));
  const visible=element=>{
   const box=element.getBoundingClientRect(),computed=getComputedStyle(element);
   return box.width>1&&box.height>1&&computed.display!=='none'&&computed.visibility==='visible'&&Number(computed.opacity)>0;
  };
  return [...document.querySelectorAll('[class*="workspace"]')].some(element=>tokenOf(element,'workspace')&&visible(element)&&[...element.children].some(child=>child.getAttribute('aria-hidden')==='true'&&tokenOf(child,'glow')&&visible(child)));
 },{},{timeout:30000});
}

/** Ordinary UI sign-out for this browser's current family only. */
async function signOut(page){
 const control=page.locator('[class*="workspace"] nav').getByRole('button',{name:'Sign out',exact:true}).first();
 try{
  if(!await control.count())return 'unavailable';
  await control.click({timeout:10000});
  const deadline=Date.now()+15000;
  while(Date.now()<deadline){
   if(await page.locator('input[type="password"],input[type="email"],input[name="email"]').count())return 'done';
   await page.waitForTimeout(250);
  }
  return 'uncertain';
 }catch{return 'uncertain';}
}

async function publicStages(browser,{origin,origins,width,heights}){
 const result={};let blocked=0;
 const {context,blocked:counter}=await openContext(browser,{width,height:heights[0],origins});
 blocked+=counter.count;
 const page=await context.newPage();page.setDefaultTimeout(30000);
 try{
  for(const stage of ['create','reset','verify']){
   let available=false;
   try{
    if(stage==='create'){
     await page.goto(origin+'/',{waitUntil:'domcontentloaded'});
     await page.locator('section[class*="stage"]').first().waitFor({timeout:20000});
     const link=page.getByRole('link',{name:/create.*account/i}).first();
     const alternate=page.getByRole('button',{name:/create.*account/i}).first();
     let opener=null;const deadline=Date.now()+15000;
     while(Date.now()<deadline&&!opener){
      if(await link.count())opener=link;
      else if(await alternate.count())opener=alternate;
      else await page.waitForTimeout(200);
     }
     if(opener){
      await opener.click({timeout:10000});
      try{await page.locator('input[autocomplete="new-password"],input[name="new-password"],input[name="confirmPassword"]').first().waitFor({timeout:8000});available=true;}catch{available=false;}
     }
    }else{
     const path=stage==='reset'?'/reset':'/verify-email';
     await page.goto(origin+path,{waitUntil:'domcontentloaded'});
     const stageElement=page.locator('section[class*="stage"]').first();
     await stageElement.waitFor({timeout:20000});
     available=new URL(page.url()).pathname===path;
    }
   }catch{available=false;}
   if(!available){result[stage]={status:'unavailable'};continue;}
   const snapshots=await measureAt(page,width,heights);
   result[stage]={status:'measured',viewports:summarizeGroup(snapshots,['stage','backdrop'],'backdrop')};
  }
 }finally{await context.close().catch(()=>{});}
 return {stages:result,blocked};
}

async function main(){
 const args=parseArgs(process.argv.slice(2),{values:['credentials','origin','out','width','heights'],flags:['skip-account']});
 const origin=args.origin??DEFAULT_ORIGIN;
 let parsed;
 try{parsed=new URL(origin);}catch{throw new ToolError('usage','--origin must be an http(s) origin');}
 if(!['http:','https:'].includes(parsed.protocol))throw new ToolError('usage','--origin must be an http(s) origin');
 const base=parsed.origin,out=args.out??DEFAULT_OUT,width=Number(args.width??DEFAULT_WIDTH);
 const heights=(args.heights??DEFAULT_HEIGHTS).split(',').map(Number).filter(value=>Number.isFinite(value)&&value>=240&&value<=2000);
 if(!heights.length)throw new ToolError('usage','--heights must be comma-separated pixel heights');
 const receipt={schema:'portico-hosted-geometry/1',artifact:ARTIFACT,status:args['skip-account']?'public-only':'ok',credentials:args['skip-account']?'not-read':'file',
  viewports:{},stages:{},signOut:'not-signed-in',cleanup:'closed'};
 const browser=await launch();
 try{
  const publicRun=await publicStages(browser,{origin:base,origins:[base],width,heights});
  receipt.stages=publicRun.stages;
  if(!args['skip-account']){
   const credentials=readCredentials(args.credentials??DEFAULT_CREDENTIALS);
   const {context,blocked}=await openContext(browser,{width,height:heights[0],origins:[base]});
   const page=await context.newPage();page.setDefaultTimeout(30000);
   let signedIn=false;const state={submitted:false};
   try{
    const outcome=await signIn(page,base,credentials,state);
    receipt.status=outcome;
    if(outcome==='signed-in'){
     signedIn=true;
     if(new URL(page.url()).pathname!=='/account')await page.goto(base+'/account',{waitUntil:'domcontentloaded'});
     await waitForWorkspace(page);
     const snapshots=await measureAt(page,width,heights);
     receipt.viewports=summarizeGroup(snapshots,['workspace','glow'],'glow');
    }
   }catch{
    receipt.status=signedIn?'geometry-failed':'sign-in-error';
   }finally{
    if(signedIn){
     try{receipt.signOut=await signOut(page);}catch{receipt.signOut='uncertain';}
    }else if(state.submitted)receipt.signOut='uncertain';
    else receipt.signOut='not-signed-in';
    await context.close().catch(()=>{receipt.cleanup='uncertain';});
   }
  }
 }finally{await browser.close().catch(()=>{receipt.cleanup='uncertain';});}
 const accountMeasured=Object.keys(receipt.viewports).length>0&&Object.values(receipt.viewports).every(view=>view.visible?.before?.workspace&&view.visible?.before?.glow&&view.visible?.after?.workspace&&view.visible?.after?.glow);
 if(receipt.status==='signed-in'&&!accountMeasured)receipt.status='workspace-missing';
 const totalCovers=accountMeasured&&Object.values(receipt.viewports).every(view=>view.coversDocument&&view.coversViewportBefore&&view.coversViewportAfter);
 receipt.accountMeasured=accountMeasured;
 receipt.coversFullPageAndViewport=Boolean(totalCovers);
 const destination=out;
 try{writePrivate(destination,JSON.stringify(receipt,null,2)+'\n');}catch{throw new ToolError('receipt-write');}
 console.log('GEOMETRY '+JSON.stringify({artifact:ARTIFACT,status:receipt.status,signOut:receipt.signOut,cleanup:receipt.cleanup,heights,account:Object.fromEntries(Object.entries(receipt.viewports).map(([h,v])=>[h,{document:v.coversDocument,viewportBefore:v.coversViewportBefore,viewportAfter:v.coversViewportAfter}])),stages:Object.fromEntries(Object.entries(receipt.stages).map(([k,v])=>[k,v.status]))}));
}

main().catch(error=>{
 const code=error instanceof ToolError?error.code:'tool-failure';
 console.error('hosted-geometry: '+code+(error instanceof ToolError&&error.detail?' ('+error.detail+')':''));
 process.exitCode=2;
});
