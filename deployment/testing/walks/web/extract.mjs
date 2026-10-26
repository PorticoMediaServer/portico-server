/** One bounded page function for the shared frame measurement and for the app
 * side. It records exact computed CSS values and document order, and never
 * reads input values, URLs, src/href or page errors. */
export function inPageExtract(request){
 const skip=new Set(['SCRIPT','STYLE','NOSCRIPT','TEMPLATE','LINK','META','HEAD','TITLE']);
 const compact=value=>String(value??'').replace(/\s+/g,' ').trim();
 const number=value=>{const parsed=Number.parseFloat(String(value));return Number.isFinite(parsed)?parsed:value;};
 const rgb=value=>{
  const raw=String(value??'').trim();
  if(/^transparent$/i.test(raw))return [0,0,0,0];
  const match=/^rgba?\(([^)]+)\)$/i.exec(raw);
  if(!match)return null;
  const parts=match[1].split(/[,\s/]+/).filter(Boolean).map(Number);
  return parts.length>=3&&parts.every(part=>Number.isFinite(part))?[parts[0],parts[1],parts[2],parts.length>3?parts[3]:1]:null;
 };
 const background=value=>{
  const raw=String(value??'');
  if(!raw||raw==='none')return 'none';
  if(/gradient\(/i.test(raw))return 'gradient';
  if(/url\(/i.test(raw))return 'image';
  return 'other';
 };
 const actionClasses=new Set(['btn','button','chip','tab','icon-btn','ghost','act','qt','ft','vchip','rchip','seg','card','thumb']);
  const implicitRole=element=>{
  const explicit=element.getAttribute('role');
  if(explicit&&/^[a-z][a-z-]{0,23}$/i.test(explicit))return explicit.toLowerCase();
  if([...element.classList].some(name=>actionClasses.has(name)))return 'button';
  const tag=element.tagName.toLowerCase();
  if(tag==='button')return 'button';
  if(tag==='a')return 'link';
  if(tag==='img'||tag==='svg')return 'img';
  if(/^h[1-6]$/.test(tag))return 'heading';
  if(tag==='input'){
   const type=(element.getAttribute('type')||'text').toLowerCase();
   if(type==='checkbox')return 'checkbox';
   if(type==='radio')return 'radio';
   if(type==='range')return 'slider';
   if(type==='search')return 'searchbox';
   return 'textbox';
  }
  if(tag==='textarea')return 'textbox';
  if(tag==='select')return 'combobox';
  if(tag==='nav')return 'navigation';
  if(tag==='main')return 'main';
  if(tag==='ul'||tag==='ol')return 'list';
  if(tag==='li')return 'listitem';
  if(tag==='table')return 'table';
  if(tag==='tr')return 'row';
  if(tag==='td'||tag==='th')return 'cell';
  return '';
 };
 const styleOf=element=>{
  const style=getComputedStyle(element);
  return {fontFamily:compact(String(style.fontFamily).split(',')[0]).replace(/^['"]|['"]$/g,''),fontSize:style.fontSize,fontWeight:style.fontWeight,lineHeight:style.lineHeight,
   letterSpacing:style.letterSpacing,color:style.color,backgroundColor:style.backgroundColor,textAlign:style.textAlign,borderRadius:style.borderRadius,
   padding:[style.paddingTop,style.paddingRight,style.paddingBottom,style.paddingLeft].map(number),margin:[style.marginTop,style.marginRight,style.marginBottom,style.marginLeft].map(number),
   gap:style.rowGap===style.columnGap?style.rowGap:[style.rowGap,style.columnGap].join(' '),opacity:number(style.opacity),display:style.display,
   normalizedRGB:{color:rgb(style.color),backgroundColor:rgb(style.backgroundColor)},backgroundImage:background(style.backgroundImage)};
 };
 const ownText=element=>{
  let text='';
  for(const node of element.childNodes)if(node.nodeType===3)text+=' '+node.nodeValue;
  return compact(text);
 };
 const pathOf=(element,root)=>{
  const parts=[];
  for(let node=element;node&&node!==root;){
   const parent=node.parentElement;
   if(!parent)break;
   parts.unshift(node.tagName.toLowerCase()+':nth-child('+([...parent.children].indexOf(node)+1)+')');
   node=parent;
  }
  return parts.join(' > ');
 };
 const shortOf=element=>{
  const tag=element.tagName.toLowerCase(),id=element.id?'#'+CSS.escape(element.id):'';
  return tag+id+[...element.classList].slice(0,4).map(name=>'.'+CSS.escape(name)).join('');
 };
 const selectorOf=(element,root)=>{
  const short=shortOf(element);
  try{if(root.querySelectorAll(short).length===1)return short;}catch{}
  return pathOf(element,root);
 };
 const isData=(element,selectors)=>{
  for(const selector of selectors){
   try{if(element.matches(selector)||element.closest(selector))return true;}catch{}
  }
  return false;
 };
 const roleOf=(element,text)=>implicitRole(element)||(text?'text':'other');
 const rootOf=selector=>selector?document.querySelector(selector):document.body;
 const dataSelectors=Array.isArray(request.dataSelectors)?request.dataSelectors:[];
 const identifiers=Array.isArray(request.identifiers)?request.identifiers:[];
 const identifierOf=element=>{for(const entry of identifiers){try{if(element.matches(entry.selector))return entry.identifier;}catch{}}return undefined;};
 const mode=request.mode,textLimit=request.textLimit??200;
 if(mode==='one'){
  const root=rootOf(request.rootSelector);
  if(!root)return {error:'root-missing'};
  const matches=root.querySelectorAll(request.selector);
  if(matches.length!==1)return {count:matches.length,error:matches.length?'selector-ambiguous':'selector-missing'};
  const element=matches[0],box=element.getBoundingClientRect(),rootBox=root.getBoundingClientRect();
  return {count:1,element:{tag:element.tagName.toLowerCase(),role:roleOf(element,compact(element.innerText)),selector:selectorOf(element,root),path:pathOf(element,root),
   name:compact(element.getAttribute('aria-label')??element.getAttribute('alt')??element.getAttribute('title')??'').slice(0,textLimit),
   text:compact(element.innerText).slice(0,textLimit),
   box:{x:Math.round(box.x-rootBox.x),y:Math.round(box.y-rootBox.y),width:Math.round(box.width),height:Math.round(box.height)},
   style:styleOf(element),textMode:isData(element,dataSelectors)?'data':'static'}};
 }
 if(mode==='list'){
  const root=rootOf(request.rootSelector);
  if(!root)return {error:'root-missing'};
  const elements=[];
  for(const selector of request.selectors){
   const matches=root.querySelectorAll(selector);
   if(matches.length!==1)return {error:matches.length?'selector-ambiguous':'selector-missing',selector};
   const element=matches[0];
   elements.push({selector,label:compact(element.innerText||element.getAttribute('aria-label')||'').slice(0,textLimit),
    textMode:isData(element,dataSelectors)?'data':'static'});
  }
  return {elements};
 }
 const root=rootOf(request.rootSelector);
 if(!root)return {error:'root-missing'};
 const limit=request.limit??3000,elements=[],stack=[...root.children].reverse();
 while(stack.length&&elements.length<limit){
  const element=stack.pop();
  if(skip.has(element.tagName))continue;
  const box=element.getBoundingClientRect(),rootBox=root.getBoundingClientRect();
  const text=ownText(element),name=compact(element.getAttribute('aria-label')??element.getAttribute('alt')??element.getAttribute('title')??'').slice(0,textLimit);
  const visible=box.width>0&&box.height>0;
  if(visible||text||name){
   const style=styleOf(element),identifier=identifierOf(element);
   const flat=style.backgroundImage==='none'&&style.normalizedRGB.backgroundColor&&style.normalizedRGB.backgroundColor[3]===1;
   elements.push({selector:selectorOf(element,root),label:(text||name).slice(0,textLimit),role:roleOf(element,text),
    box:[Math.round(box.x-rootBox.x),Math.round(box.y-rootBox.y),Math.round(box.width),Math.round(box.height)],
    style,textMode:isData(element,dataSelectors)?'data':'static',...(identifier?{identifier}:{}),...(flat?{normalizedRGB:style.normalizedRGB.backgroundColor.slice(0,3)}:{})});
  }
  if(element.tagName==='svg')continue;
  const children=[...element.children];
  for(let cursor=children.length-1;cursor>=0;cursor--)stack.push(children[cursor]);
 }
 return {root:{selector:request.rootSelector,tag:root.tagName.toLowerCase(),box:(()=>{const box=root.getBoundingClientRect();return [Math.round(box.width),Math.round(box.height)];})()},elements,truncated:elements.length>=limit};
}

export async function extract(page,request){
 return page.evaluate(inPageExtract,request);
}
