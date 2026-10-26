/** Measured differences between a frame element and the app element the walker
 * pairs with it. Every row carries the frame citation. Text differences are
 * sample-data differences unless text is missing or `strictText` is set. */
const TOLERANCE={fontSize:0.5,lineHeight:1.5,letterSpacing:0.25,opacity:0.02,spacing:1,radius:0.5,box:1};

export function citation({file,label,selector}){return file+' '+label+' '+selector;}

const px=value=>{
 if(typeof value==='number')return value;
 const match=/^(-?\d+(?:\.\d+)?)px$/.exec(String(value).trim());
 return match?Number(match[1]):null;
};
const color=value=>{
 const raw=String(value).trim();
 if(/^transparent$/i.test(raw))return [0,0,0,0];
 const match=/^rgba?\(([^)]+)\)$/i.exec(raw);
 if(!match)return null;
 const parts=match[1].split(/[,\s/]+/).filter(Boolean).map(Number);
 if(parts.length<3||parts.some(part=>!Number.isFinite(part)))return null;
 return [parts[0],parts[1],parts[2],parts.length>3?parts[3]:1];
};
const sameColor=(left,right)=>{
 const a=color(left),b=color(right);
 if(!a||!b)return String(left)===String(right);
 return a.every((value,index)=>Math.abs(value-b[index])<=(index===3?0.01:0.5));
};
const tokens=value=>Array.isArray(value)?value:String(value??'').trim().split(/\s+/).filter(Boolean);

function row(base,kind,field,expected,actual,delta,fidelity){
 return {kind,field,expected,actual,...(delta===undefined?{}: {delta}),fidelity,citation:base.citation,appSelector:base.appSelector};
}

export function comparePair(design,app,{citation:cite,appSelector,strictText=false,data=false,mapped=true}){
 const base={citation:cite,appSelector},rows=[];
 const loose=(field,value)=>field==='letterSpacing'&&String(value).trim()==='normal'?'0px':value;
 const add=(field,expected,actual,delta,fidelity)=>rows.push(row(base,'style',field,expected,actual,delta,fidelity));
 for(const [field,tolerance] of [['fontSize',TOLERANCE.fontSize],['lineHeight',TOLERANCE.lineHeight],['letterSpacing',TOLERANCE.letterSpacing],['opacity',TOLERANCE.opacity]]){
  const expected=loose(field,design.style[field]),actual=loose(field,app.style[field]);
  const left=px(expected),right=px(actual);
  if(left!==null&&right!==null){if(Math.abs(left-right)>tolerance)add(field,left,right,Math.round((right-left)*100)/100,true);}
  else if(String(expected)!==String(actual))add(field,String(expected),String(actual),undefined,true);
 }
 for(const field of ['fontFamily','fontWeight','textAlign']){
  if(String(design.style[field])!==String(app.style[field]))add(field,String(design.style[field]),String(app.style[field]),undefined,true);
 }
 for(const field of ['color','backgroundColor']){
  if(!sameColor(design.style[field],app.style[field]))add(field,design.style[field],app.style[field],undefined,true);
 }
 for(const field of ['padding','margin']){
  const left=tokens(design.style[field]),right=tokens(app.style[field]);
  const length=Math.max(left.length,right.length);
  for(let index=0;index<length;index++){
   const a=px(left[index]),b=px(right[index]);
   if(a!==null&&b!==null){if(Math.abs(a-b)>TOLERANCE.spacing)add(field+'['+index+']',a,b,Math.round((b-a)*100)/100,true);}
   else if(String(left[index]??'')!==String(right[index]??''))add(field+'['+index+']',left[index]??'',right[index]??'',undefined,true);
  }
 }
 const leftGap=tokens(design.style.gap).map(token=>token==='normal'?'0px':token),rightGap=tokens(app.style.gap).map(token=>token==='normal'?'0px':token),gapLength=Math.max(leftGap.length,rightGap.length);
 for(let index=0;index<gapLength;index++){
  const a=px(leftGap[index]),b=px(rightGap[index]);
  if(a!==null&&b!==null){if(Math.abs(a-b)>TOLERANCE.spacing)add('gap['+index+']',a,b,Math.round((b-a)*100)/100,true);}
  else if(String(leftGap[index]??'')!==String(rightGap[index]??''))add('gap['+index+']',leftGap[index]??'',rightGap[index]??'',undefined,true);
 }
 const leftRadius=tokens(String(design.style.borderRadius).replace('/',' ')),rightRadius=tokens(String(app.style.borderRadius).replace('/',' '));
 const radiusLength=Math.max(leftRadius.length,rightRadius.length);
 for(let index=0;index<radiusLength;index++){
  const a=px(leftRadius[index]),b=px(rightRadius[index]);
  if(a!==null&&b!==null){if(Math.abs(a-b)>TOLERANCE.radius)add('borderRadius['+index+']',a,b,Math.round((b-a)*100)/100,true);}
  else if(String(leftRadius[index]??'')!==String(rightRadius[index]??''))add('borderRadius['+index+']',leftRadius[index]??'',rightRadius[index]??'',undefined,true);
 }
 for(const field of ['width','height']){
  const a=design.box?.[field],b=app.box?.[field];
  if(Number.isFinite(a)&&Number.isFinite(b)&&Math.abs(a-b)>TOLERANCE.box)add(field,a,b,b-a,!data);
 }
 for(const [field,name] of [['text','text'],['name','accessible name']]){
  const expected=String(design[field]??''),actual=String(app[field]??'');
  if(expected===actual)continue;
  const fidelity=(data||!mapped)?false:(expected&&!actual)||(strictText&&expected&&actual);
  rows.push(row(base,'text',name,expected,actual,undefined,Boolean(fidelity)));
 }
 return rows;
}

export function compareOrder(designLabels,appLabels,{citation:cite,appSelector,dataFlags=[],mapped=true}){
 const base={citation:cite,appSelector},rows=[];
 const length=Math.max(designLabels.length,appLabels.length);
 if(designLabels.length!==appLabels.length)rows.push(row(base,'order','count',designLabels.length,appLabels.length,appLabels.length-designLabels.length,true));
 for(let index=0;index<length;index++){
  const expected=designLabels[index],actual=appLabels[index];
  if(expected===undefined||actual===undefined)continue;
  if(expected!==actual&&mapped&&!dataFlags[index])rows.push(row(base,'order','index['+index+']',expected,actual,undefined,true));
 }
 return rows;
}

export function formatLine(entry){
 return (entry.fidelity?'DIFF':'DATA')+' '+entry.citation+' '+entry.field+': expected '+JSON.stringify(entry.expected)+' actual '+JSON.stringify(entry.actual)+(entry.delta!==undefined?' delta '+entry.delta:'');
}
