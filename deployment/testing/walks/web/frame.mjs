import {ToolError} from './cli.mjs';

/** Resolve and isolate one exact frame, then pin it to 1:1 design scale.
 *
 * Label conventions, taken from the design pages themselves: a frame's
 * `data-f` gallery value, else its own or its `.screen` id (with or without a
 * short letter prefix such as `f`/`g`), else its 1-based document position.
 * Ambiguity is a failure, never a guess. */
export async function prepareFrame(page,label){
 const info=await page.evaluate(async label=>{
  const frames=[...document.querySelectorAll('.frame')];
  const screenOf=frame=>frame.querySelector('.screen');
  const candidates=frames.map((frame,index)=>({index:index+1,dataF:frame.getAttribute('data-f')||'',id:frame.id||screenOf(frame)?.id||''}));
  const available=[...new Set(candidates.flatMap(candidate=>[candidate.dataF,candidate.id,String(candidate.index)]).filter(Boolean))].slice(0,80);
  if(!/^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$/.test(label))return {error:'label-invalid'};
  const unique=list=>list.length===1?list[0]:null;
  let chosen=unique(candidates.filter(candidate=>candidate.dataF&&candidate.dataF===label));
  if(!chosen){
   const byId=candidates.filter(candidate=>candidate.id===label||candidate.id.replace(/^[a-z]+/,'')===label);
   if(byId.length>1)return {error:'label-ambiguous',available};
   chosen=byId[0];
  }
  if(!chosen){
   const byIndex=candidates.filter(candidate=>String(candidate.index)===label);
   if(byIndex.length>1)return {error:'label-ambiguous',available};
   chosen=byIndex[0];
  }
  if(!chosen)return {error:'label-unknown',available};
  const frame=frames[chosen.index-1],screen=screenOf(frame);
  if(!frame||!screen)return {error:'frame-without-screen'};
  for(const other of frames)if(other!==frame)other.style.display='none';
  screen.style.zoom='1';
  frame.style.display='block';
  frame.style.width='max-content';
  frame.style.maxWidth='none';
  frame.style.overflow='visible';
  frame.style.borderRadius='0';
  const width=Math.round(screen.getBoundingClientRect().width);
  frame.style.width=width+'px';
  frame.setAttribute('data-portico-walk-frame','1');
  await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
  const screenBox=screen.getBoundingClientRect(),frameBox=frame.getBoundingClientRect();
  return {index:chosen.index,dataF:chosen.dataF,id:chosen.id,label,width:Math.round(frameBox.width),height:Math.round(frameBox.height),
   screen:{width:Math.round(screenBox.width),height:Math.round(screenBox.height)}};
 },label);
 if(info.error)throw new ToolError(info.error,info.available?info.available.join(','):undefined);
 if(!(info.width>0&&info.height>0))throw new ToolError('frame-empty');
 return info;
}

/** Resize for the element capture, bounded, and re-measure once. */
export async function fitViewport(page,info,selector='[data-portico-walk-frame="1"]'){
 const width=Math.min(4096,info.width+80),height=Math.min(6000,info.height+80);
 await page.setViewportSize({width,height});
 await page.evaluate(async()=>{await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});
 const box=await page.evaluate(selector=>{
  const frame=document.querySelector(selector);
  if(!frame)return null;
  const rect=frame.getBoundingClientRect();
  return {width:Math.round(rect.width),height:Math.round(rect.height)};
 },selector);
 if(!box||box.width<1||box.height<1)throw new ToolError('frame-empty');
 return box;
}
