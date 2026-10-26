import fs from 'node:fs';
import path from 'node:path';

/** Publication documents are the sanitized XHTML sections rendered by the
 * reader's paginator iframe. They contain no scripts, frames or forms. Text
 * must be visible with real Range geometry on at least two lines and at least
 * 80 characters, and no ancestor may hide it. Images are recorded for the
 * receipt but are not a substitute for the unchanged fixture text. This probe
 * runs inside a candidate frame; callers only pass paginator-owned frames. */
export function probePublication(){
 const visible=rect=>rect.width>1&&rect.height>1&&rect.right>0&&rect.bottom>0&&rect.left<innerWidth&&rect.top<innerHeight;
 const rendered=element=>{
  for(let node=element;node&&node.nodeType===1;node=node.parentElement){
   const style=getComputedStyle(node);
   if(style.display==='none'||style.visibility!=='visible'||Number(style.opacity)===0)return false;
   if(node===document.documentElement)break;
  }
  return true;
 };
 if(!document.body)return {qualified:false,reason:'no-body'};
 if(document.querySelector('script,iframe,frame,form'))return {qualified:false,reason:'non-publication-document'};
 const body=document.body.getBoundingClientRect();
 if(!visible(body)||!rendered(document.body))return {qualified:false,reason:'invisible-body'};
 let chars=0,textRects=0;const lines=new Set();
 const walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);
 for(let node=walker.nextNode();node;node=walker.nextNode()){
  if(!node.textContent.trim())continue;
  const parent=node.parentElement;
  if(!parent||['SCRIPT','STYLE','TITLE'].includes(parent.tagName))continue;
  if(!rendered(parent))continue;
  const range=document.createRange();range.selectNodeContents(node);
  let counted=false;
  for(const rect of range.getClientRects()){
   if(!visible(rect)||rect.height<4||rect.width<2)continue;
   textRects++;counted=true;if(lines.size<4000)lines.add(Math.round(rect.top));
  }
  if(counted)chars+=node.textContent.replace(/\s+/g,' ').trim().length;
  if(textRects>20000)break;
 }
 let images=0,imagePixels=0;
 for(const image of document.images){
  if(!image.complete||image.naturalWidth<128)continue;
  const rect=image.getBoundingClientRect();
  if(!visible(rect)||!rendered(image))continue;
  images++;imagePixels+=Math.round(rect.width*rect.height);
 }
 const qualified=chars>=80&&lines.size>=2;
 return {qualified,reason:qualified?'publication-text':chars<80?'insufficient-text':'insufficient-text-lines',
  chars,textRects,lines:lines.size,images,imagePixels,body:{width:Math.round(body.width),height:Math.round(body.height)}};
}

/** Runs in the parent document against the iframe element itself. The iframe
 * and its shadow host must be rendered through their whole ancestor chain, so a
 * hidden or transparent parent cannot keep qualifying on raw box geometry. */
export function probeFrameOwner(element){
 const rect=element.getBoundingClientRect(),root=element.getRootNode();
 const host=root instanceof ShadowRoot?root.host:null,box=node=>node?{width:Math.round(node.getBoundingClientRect().width),height:Math.round(node.getBoundingClientRect().height)}:null;
 const rendered=node=>{
  let current=node;
  while(current&&current.nodeType===1){
   const style=getComputedStyle(current);
   if(style.display==='none'||style.visibility!=='visible'||Number(style.opacity)===0)return false;
   const parent=current.parentNode;
   current=parent instanceof ShadowRoot?parent.host:parent?.nodeType===1?parent:null;
  }
  return true;
 };
 return {part:element.getAttribute('part')??'',sandbox:(element.getAttribute('sandbox')??'').split(' ').filter(Boolean),
  visible:rect.width>1&&rect.height>1&&rect.right>0&&rect.bottom>0&&rect.left<innerWidth&&rect.top<innerHeight,
  rendered:rendered(element),
  frame:{width:Math.round(rect.width),height:Math.round(rect.height)},
  host:host?{tag:host.tagName.toLowerCase(),...box(host),flow:host.getAttribute('flow')??'',rendered:rendered(host)}:null};
}

/** The publication iframe is the paginator's own `part="filter"` child, and it
 * must be rendered through its ancestor chain. The Reading screen's surrounding
 * app chrome is never a publication, so its shell text cannot pass. */
function ownerIsPaginator(owner){
 return owner.part==='filter'||owner.host?.tag==='foliate-paginator';
}

/** Runs inside a publication frame: sanitized sections may not reference
 * remote or responsive asset sources. */
export function probePublicationLinks(){
 return document.querySelectorAll('[srcset],a[href^="http:"],a[href^="https:"]').length;
}

export async function publication(page,{attempts=40,delay=500}={}){
 let inspected=0,last=null;
 for(let attempt=0;attempt<attempts;attempt++){
  for(const frame of page.frames().slice(0,16)){
   if(frame===page.mainFrame())continue;
   inspected++;
   let owner,probe;
   try{
    const element=await frame.frameElement();
    owner=await element.evaluate(probeFrameOwner);
    if(!ownerIsPaginator(owner))continue;
    if(!owner.rendered||!owner.visible||owner.frame.width<200||owner.frame.height<160)continue;
    if(owner.host&&(!owner.host.rendered||owner.host.width<240||owner.host.height<200))continue;
    probe=await frame.evaluate(probePublication);
   }catch{continue;}
   last={owner,probe};
   if(probe?.qualified)return {found:true,inspected,frame:owner.frame,host:owner.host,publication:probe,frameRef:frame};
  }
  if(attempt<attempts-1)await page.waitForTimeout(delay);
 }
 return {found:false,inspected,last:last?{part:last.owner.part,host:last.owner.host,reason:last.probe?.reason??'unreadable'}:null};
}

/** Runs inside one frame: bounded layout structure, styles and CSP classes. */
function probeLayout(){
 const rect=element=>{const r=element.getBoundingClientRect();return ['x','y','width','height'].map(key=>Math.round(Math.max(-100000,Math.min(100000,r[key]))));};
 const measure=(element,kind)=>{const style=getComputedStyle(element);return {kind,rect:rect(element),display:['none','block','inline','inline-block','flex','inline-flex','grid','contents'].includes(style.display)?style.display:'other',visibility:['visible','hidden','collapse'].includes(style.visibility)?style.visibility:'other',opacity:Math.max(0,Math.min(1,Number(style.opacity)))};};
 const elements=[],pending=[document];let visited=0,styleCount=0,nonceStyleCount=0,textNodes=0;
 if(document.body){elements.push(measure(document.body,'body'));const walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(walker.nextNode()&&textNodes<100000)textNodes++;}
 while(pending.length&&visited<5000){
  const root=pending.shift();
  for(const element of root.querySelectorAll('*')){
   if(++visited>5000)break;
   if(element.shadowRoot)pending.push(element.shadowRoot);
   if(element.tagName==='STYLE'){styleCount++;if(element.nonce)nonceStyleCount++;}
   if(elements.length<32){
    if(element.tagName==='IFRAME')elements.push(measure(element,'iframe'));
    else if(element.tagName==='FOLIATE-PAGINATOR')elements.push(measure(element,'paginator'));
    else if(element.id==='container')elements.push(measure(element,'container'));
   }
  }
 }
 return {documentClass:location.protocol==='blob:'?'publication-blob':location.protocol==='about:'?'inert-document':'application',textNodes,elements,styleCount,nonceStyleCount,csp:window.__porticoCheckpointCsp??[]};
}

/** Bounded layout receipt for the Reading checkpoint, including the exact
 * publication verdict that qualified. Structure and geometry only. */
export async function writeLayoutReceipt(page,out,result){
 const frames=[];
 for(const frame of page.frames().slice(0,16)){
  try{frames.push(await frame.evaluate(probeLayout));}catch{frames.push({unavailable:true});}
 }
 const publication=result?.found?{found:true,frame:result.frame,host:result.host,document:{chars:result.publication.chars,textRects:result.publication.textRects,lines:result.publication.lines,images:result.publication.images,imagePixels:result.publication.imagePixels,body:result.publication.body}}:{found:false,reason:result?.last?.reason??'publication-not-found'};
 const receipt={frames,publication};
 fs.writeFileSync(path.join(out,'reading-layout.log'),JSON.stringify(receipt,null,2)+'\n');
 return receipt;
}
