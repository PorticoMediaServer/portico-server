/** Small CLI helper shared by the walk tools. Keeps failures as closed codes:
 * never a stack, page error, URL or credential value. */
export class ToolError extends Error {
 constructor(code,detail){
  super(detail?code+': '+detail:code);
  this.name='ToolError';
  this.code=code;
  this.detail=detail;
 }
}

/** Parse `--name value`, `--name=value` and boolean flags. Repeats collect. */
export function parseArgs(argv,{values=[],flags=[],multi=[]}={}){
 const takes=new Set(values),booleans=new Set(flags),repeats=new Set(multi),out={};
 for(let index=0;index<argv.length;index++){
  const raw=argv[index];
  if(!raw.startsWith('--'))throw new ToolError('usage','unexpected argument');
  const equal=raw.indexOf('='),name=sliced(raw,equal,2),inline=equal>=0?raw.slice(equal+1):undefined;
  if(booleans.has(name)){
   if(inline!==undefined)throw new ToolError('usage','flag takes no value: '+name);
   out[name]=true;continue;
  }
  if(!takes.has(name)&&!repeats.has(name))throw new ToolError('usage','unknown option: '+name);
  const value=inline!==undefined?inline:argv[++index];
  if(value===undefined||value==='')throw new ToolError('usage','missing value: '+name);
  if(repeats.has(name))(out[name]??=[]).push(value);
  else out[name]=value;
 }
 return out;
}
function sliced(raw,equal,start){return equal<0?raw.slice(start):raw.slice(start,equal);}

export function required(values,names){
 const missing=names.filter(name=>values[name]===undefined||values[name]==='');
 if(missing.length)throw new ToolError('usage','missing '+missing.map(name=>'--'+name).join(', '));
}

/** One-line summary for the console; long detail stays in the JSON report. */
export function printSummary(label,payload){
 console.log(label+' '+JSON.stringify(payload));
}
