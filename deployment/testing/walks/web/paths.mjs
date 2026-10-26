import fs from 'node:fs';
import path from 'node:path';
import {ToolError} from './cli.mjs';

/** The approved mock-ups. WALKS.md pins this folder; allow an explicit override
 * for another checkout of the same design repository. */
export const DESIGN_ROOT='/home/justin/projects/Portico/.worktrees/design-ui-v1/design/ui-v1';

export function designRoot(option){
 const root=path.resolve(option??process.env.PORTICO_WALKS_DESIGN??DESIGN_ROOT);
 let real;
 try{real=fs.realpathSync(root);}catch{throw new ToolError('design-root-missing');}
 if(!fs.statSync(real).isDirectory())throw new ToolError('design-root-missing');
 return real;
}

/** One exact mock-up file inside the design root, named by a relative path or an
 * absolute path that resolves inside it. */
export function designFile(root,argument){
 if(!argument)throw new ToolError('usage','missing --file');
 let real;
 try{real=fs.realpathSync(path.resolve(root,argument));}catch{throw new ToolError('design-file-missing');}
 if(!real.startsWith(root+path.sep))throw new ToolError('design-file-outside-root');
 if(!/\.html?$/i.test(real))throw new ToolError('design-file-not-html');
 return {absolute:real,file:path.relative(root,real)};
}

/** Manrope, for full local type without any remote font request. The product
 * bundle already carries the variable face; the tool never copies it. */
const FONT_CANDIDATES=[
 ()=>process.env.PORTICO_WALKS_MANROPE,
 ()=>process.env.PORTICO_SERVER_REPO&&path.join(process.env.PORTICO_SERVER_REPO,'web-next/src/kit/fonts/manrope-var-latin.woff2'),
 ()=>'/home/justin/projects/Portico/apps/portico-server/web-next/src/kit/fonts/manrope-var-latin.woff2',
];
export function manropeFont(explicit){
 for(const candidate of [()=>explicit,...FONT_CANDIDATES]){
  const name=candidate();
  if(!name)continue;
  try{
   const real=fs.realpathSync(name),bytes=fs.readFileSync(real);
   if(bytes.subarray(0,4).toString('latin1')==='wOF2')return {path:real,bytes};
  }catch{}
 }
 return null;
}

/** Artifacts live outside every repository. The render-frame wrapper supplies
 * the owned campaign folder; the tool itself only needs somewhere to write. */
export function artifactRoot(option){
 const root=path.resolve(option??process.env.PORTICO_WALKS_ARTIFACTS??'');
 if(!option&&!process.env.PORTICO_WALKS_ARTIFACTS)throw new ToolError('usage','missing --out');
 fs.mkdirSync(root,{recursive:true,mode:0o700});
 const stat=fs.statSync(root);
 if(!stat.isDirectory())throw new ToolError('output-invalid');
 return root;
}

export function writePrivate(file,data){
 const handle=fs.openSync(file,'w',0o600);
 try{fs.writeFileSync(handle,data);}finally{fs.closeSync(handle);}
}
