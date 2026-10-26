#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {pathToFileURL} from 'node:url';
import {ToolError,parseArgs,required,printSummary} from './web/cli.mjs';
import {designRoot,designFile,manropeFont,artifactRoot,writePrivate} from './web/paths.mjs';
import {launch,openContext,openPage,installFont,settle,requireManrope} from './web/browser.mjs';
import {prepareFrame,fitViewport} from './web/frame.mjs';
import {extract} from './web/extract.mjs';

/** Render one exact mock-up frame (or one exact device inside it) at 1:1 design
 * scale and write the shared measurement artifact the web and native comparers
 * cite. */
function pngSize(bytes){
 if(bytes.length<24||bytes.subarray(0,8).toString('hex')!=='89504e470d0a1a0a')throw new ToolError('png-invalid');
 return {width:bytes.readUInt32BE(16),height:bytes.readUInt32BE(20)};
}
function slug(value){return String(value).toLowerCase().replace(/[^a-z0-9._-]+/g,'-');}
function artifactName(file,label,suffix){return path.basename(file).replace(/\.html?$/i,'')+'-'+slug(label)+suffix;}
function parseIdentifiers(values){
 return (values??[]).map(raw=>{
  const at=raw.indexOf('=');
  if(at<1||at>=raw.length-1)throw new ToolError('usage','--identifier needs "selector=identifier"');
  const selector=raw.slice(0,at).trim(),identifier=raw.slice(at+1).trim();
  if(!selector||selector.length>200||!/^[A-Za-z0-9._-]{1,64}$/.test(identifier))throw new ToolError('usage','--identifier invalid');
  return {selector,identifier};
 });
}

async function main(){
 const args=parseArgs(process.argv.slice(2),{values:['file','label','width','height','out','design-root','font','element','units','surface'],multi:['data','identifier'],flags:['no-data']});
 required(args,['file','label','width','height','out']);
 const width=Number(args.width),height=Number(args.height);
 if(!Number.isFinite(width)||width<80||width>4096)throw new ToolError('usage','--width out of range');
 if(!Number.isFinite(height)||height<80||height>6000)throw new ToolError('usage','--height out of range');
 const units=args.units??'pixels';
 if(!['points','pixels'].includes(units))throw new ToolError('usage','--units must be points or pixels');
 const dataSelectors=args.data??[],identifiers=parseIdentifiers(args.identifier);
 if((dataSelectors.length>0)===(args['no-data']===true))throw new ToolError('usage','explicit data mapping required: --data selector or --no-data');
 const root=designRoot(args['design-root']),target=designFile(root,args.file),out=artifactRoot(args.out),font=manropeFont(args.font);
 const browser=await launch();
 try{
  const {context}=await openContext(browser,{width:2048,height:1400});
  const page=await openPage(context);
  try{await page.goto(pathToFileURL(target.absolute).href,{waitUntil:'load'});}
  catch{throw new ToolError('design-navigation');}
  await installFont(page,font);
  await settle(page);
  const info=await prepareFrame(page,args.label);
  let captureSelector='[data-portico-walk-frame="1"]',capture=null;
  if(args.element){
   capture=await page.evaluate(selector=>{
    const frame=document.querySelector('[data-portico-walk-frame="1"]');
    if(!frame)return {error:'frame-missing'};
    let matches;
    try{matches=frame.querySelectorAll(selector);}catch{return {error:'capture-selector-invalid'};}
    if(matches.length!==1)return {error:matches.length?'capture-selector-ambiguous':'capture-selector-missing'};
    const element=matches[0];
    element.setAttribute('data-portico-walk-capture','1');
    const box=element.getBoundingClientRect();
    return {width:Math.round(box.width),height:Math.round(box.height)};
   },args.element);
   if(capture.error)throw new ToolError(capture.error);
   captureSelector='[data-portico-walk-capture="1"]';
  }
  const box=await fitViewport(page,capture??{width:info.width,height:info.height},captureSelector);
  await requireManrope(page);
  const suffix=args.element?'-'+crypto.createHash('sha1').update(args.element).digest('hex').slice(0,8):'';
  const png=path.join(out,artifactName(target.file,args.label,suffix+'.png'));
  const element=await page.$(captureSelector);
  if(!element)throw new ToolError('capture-missing');
  try{await element.screenshot({path:png,type:'png'});}
  catch{throw new ToolError('frame-screenshot');}
  fs.chmodSync(png,0o600);
  const size=pngSize(fs.readFileSync(png));
  if(Math.abs(size.width-box.width)>1||Math.abs(size.height-box.height)>1)throw new ToolError('frame-size-mismatch');
  const tree=await extract(page,{mode:'tree',rootSelector:captureSelector,limit:3000,textLimit:200,dataSelectors,identifiers});
  if(!tree||!Array.isArray(tree.elements))throw new ToolError('frame-extract');
  const artifact={tool:'render-frame',
   ...(args.surface?{surface:args.surface}:{}),
   source:{file:target.file,label:args.label},
   units,
   device:{width:size.width,height:size.height},
   target:{width,height},
   dataMapping:'explicit',
   elements:tree.elements,
   truncated:Boolean(tree.truncated)};
  const json=path.join(out,artifactName(target.file,args.label,suffix+'.json'));
  writePrivate(json,JSON.stringify(artifact,null,2)+'\n');
  printSummary('FRAME',{file:target.file,label:args.label,png:path.basename(png),json:path.basename(json),units,width:size.width,height:size.height,elements:tree.elements.length,dataMapping:artifact.dataMapping});
 } finally {await browser.close().catch(()=>{});}
}

main().catch(error=>{
 const code=error instanceof ToolError?error.code:'tool-failure';
 console.error('render-frame: '+code+(error instanceof ToolError&&error.detail?' ('+error.detail+')':''));
 process.exitCode=2;
});
