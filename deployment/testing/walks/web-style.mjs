#!/usr/bin/env node
import path from 'node:path';
import {pathToFileURL} from 'node:url';
import {ToolError,parseArgs,required,printSummary} from './web/cli.mjs';
import {designRoot,designFile,manropeFont,artifactRoot,writePrivate} from './web/paths.mjs';
import {launch,openContext,openPage,installFont,settle,requireManrope,checkStorageState} from './web/browser.mjs';
import {prepareFrame} from './web/frame.mjs';
import {extract} from './web/extract.mjs';
import {comparePair,compareOrder,citation,formatLine} from './web/compare.mjs';

const FRAME='[data-portico-walk-frame="1"]';

function splitPair(raw){
 const at=raw.indexOf('=>');
 if(at<1||at>=raw.length-2)throw new ToolError('usage','--pair needs "frame selector => app selector"');
 const left=raw.slice(0,at).trim(),right=raw.slice(at+2).trim();
 if(!left||!right||left.length>300||right.length>300||/[\r\n]/.test(left+right))throw new ToolError('usage','--pair selector invalid');
 return {frameSelector:left,appSelector:right};
}
function splitOrder(raw){
 const at=raw.indexOf('=>');
 if(at<1||at>=raw.length-2)throw new ToolError('usage','--order needs "frame selectors => app selectors"');
 const list=side=>side.split(',').map(value=>value.trim()).filter(Boolean);
 const frameSelectors=list(raw.slice(0,at)),appSelectors=list(raw.slice(at+2));
 if(!frameSelectors.length||!appSelectors.length||frameSelectors.length>40||appSelectors.length>40)throw new ToolError('usage','--order selector list invalid');
 return {frameSelectors,appSelectors};
}
function appPath(raw){
 const url=new URL(raw);
 if(!['http:','https:','file:'].includes(url.protocol))throw new ToolError('usage','--app must be http(s) or file');
 return url.protocol==='file:'?'/':url.pathname||'/';
}

async function measure(page,rootSelector,selector,dataSelectors){
 const result=await extract(page,{mode:'one',rootSelector,selector,dataSelectors});
 if(result?.error)throw new ToolError('extract-'+result.error,selector);
 return result.element;
}

async function main(){
 const args=parseArgs(process.argv.slice(2),{values:['file','label','app','width','height','app-wait','out','design-root','font','storage-state'],multi:['pair','order','data'],flags:['strict-text','no-data']});
 required(args,['file','label','app','width','height']);
 const width=Number(args.width),height=Number(args.height);
 if(!Number.isFinite(width)||width<80||width>4096)throw new ToolError('usage','--width out of range');
 if(!Number.isFinite(height)||height<80||height>6000)throw new ToolError('usage','--height out of range');
 const pairs=(args.pair??[]).map(splitPair),orders=(args.order??[]).map(splitOrder),dataSelectors=args.data??[];
 if((dataSelectors.length>0)===(args['no-data']===true))throw new ToolError('usage','explicit data mapping required: --data selector or --no-data');
 const mapped=true;
 if(!pairs.length&&!orders.length)throw new ToolError('usage','at least one --pair or --order is required');
 if(pairs.length>60||orders.length>20)throw new ToolError('usage','too many pairs or orders');
 const root=designRoot(args['design-root']),target=designFile(root,args.file),font=manropeFont(args.font);
 const storageState=checkStorageState(args['storage-state']);
 const appUrl=new URL(args.app),appOrigin=appUrl.protocol==='file:'?null:appUrl.origin;
 const out=args.out?artifactRoot(args.out):null;
 const browser=await launch();
 const rows=[];
 let blockedDesign=0,blockedApp=0;
 try{
  const design=await openContext(browser,{width:2048,height:1400});
  const designPage=await openPage(design.context);
  try{await designPage.goto(pathToFileURL(target.absolute).href,{waitUntil:'load'});}
  catch{throw new ToolError('design-navigation');}
  await installFont(designPage,font);await settle(designPage);
  const info=await prepareFrame(designPage,args.label);
  await requireManrope(designPage);
  blockedDesign=design.blocked.count;
  const app=await openContext(browser,{width,height,allowOrigin:appOrigin,storageState});
  const appPage=await openPage(app.context);
  try{await appPage.goto(args.app,{waitUntil:'domcontentloaded'});}
  catch{throw new ToolError('app-navigation');}
  // Measure the app's own loaded fonts/styles without modifying its CSS/CSP.
  await settle(appPage);
  if(args['app-wait']){
   try{await appPage.locator(args['app-wait']).first().waitFor();}
   catch{throw new ToolError('app-wait-timeout');}
  }
  await requireManrope(appPage);
  blockedApp=app.blocked.count;
  for(const pair of pairs){
   const designElement=await measure(designPage,FRAME,pair.frameSelector,dataSelectors);
   const cite=citation({file:target.file,label:args.label,selector:pair.frameSelector});
   const result=await extract(appPage,{mode:'one',rootSelector:null,selector:pair.appSelector});
   if(result?.error==='selector-missing'){
    rows.push({kind:'missing',field:'element',expected:designElement.selector,actual:null,fidelity:designElement.textMode!=='data',citation:cite,appSelector:pair.appSelector});
    continue;
   }
   if(result?.error)throw new ToolError('extract-'+result.error,pair.appSelector);
   rows.push(...comparePair(designElement,result.element,{citation:cite,appSelector:pair.appSelector,strictText:true,data:designElement.textMode==='data',mapped}));
  }
  for(const order of orders){
   const designList=await extract(designPage,{mode:'list',rootSelector:FRAME,selectors:order.frameSelectors,dataSelectors});
   const appList=await extract(appPage,{mode:'list',rootSelector:null,selectors:order.appSelectors});
   if(designList?.error)throw new ToolError('extract-'+designList.error,order.frameSelectors.join(','));
   if(appList?.error)throw new ToolError('extract-'+appList.error,order.appSelectors.join(','));
   const cite=citation({file:target.file,label:args.label,selector:order.frameSelectors.join(',')});
   rows.push(...compareOrder(designList.elements.map(element=>element.label),appList.elements.map(element=>element.label),{citation:cite,appSelector:order.appSelectors.join(','),dataFlags:designList.elements.map(element=>element.textMode==='data'),mapped}));
  }
  const fidelity=rows.filter(entry=>entry.fidelity).length,data=rows.length-fidelity;
  const report={schema:'portico-walk-style/1',frame:{file:target.file,label:args.label,index:info.index},
   device:{width,height},app:{path:appPath(args.app),pairs:pairs.length,orders:orders.length},
   blocked:{design:blockedDesign,app:blockedApp},dataMapping:'explicit',
   summary:{fidelity,data,pairs:pairs.length,orders:orders.length},diffs:rows};
  for(const entry of rows.slice(0,100))console.log(formatLine(entry));
  if(out)writePrivate(path.join(out,path.basename(target.file).replace(/\.html?$/i,'')+'-'+String(args.label).toLowerCase().replace(/[^a-z0-9._-]+/g,'-')+'-style.json'),JSON.stringify(report,null,2)+'\n');
  printSummary('STYLE',{frame:target.file,label:args.label,fidelity,data,pairs:pairs.length,orders:orders.length,blockedApp});
  process.exitCode=fidelity?1:0;
 } finally {await browser.close().catch(()=>{});}
}

main().catch(error=>{
 const code=error instanceof ToolError?error.code:'tool-failure';
 if(process.env.PORTICO_WALKS_DEBUG&&!(error instanceof ToolError))console.error('debug: '+String(error?.name??''));
 console.error('web-style: '+code+(error instanceof ToolError&&error.detail?' ('+error.detail+')':''));
 process.exitCode=2;
});
