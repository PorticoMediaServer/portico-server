/** Ephemeral loopback reference only. Serves the immutable approved design subtree
 * directly from Git; never the working tree, auth, files outside it or a mutable ref. */
import {createServer} from 'node:http';
import {execFileSync} from 'node:child_process';
const revision='812dacacc70e296ac870ec36d1a37a485eaeb8ed';
const repo=process.env.PORTICO_DESIGN_REPO,port=Number(process.env.PORTICO_REFERENCE_PORT);
if(!repo||!Number.isSafeInteger(port)||port<1024||port>65535)throw new Error('explicit reference repository and loopback port required');
const mime={html:'text/html',css:'text/css',js:'text/javascript',svg:'image/svg+xml',png:'image/png',jpg:'image/jpeg',jpeg:'image/jpeg',webp:'image/webp',woff:'font/woff',woff2:'font/woff2'};
const server=createServer((request,response)=>{
  try{
    if(!['GET','HEAD'].includes(request.method))throw new Error('method');
    const pathname=decodeURIComponent(new URL(request.url,'http://127.0.0.1').pathname);
    if(!pathname.startsWith('/812/'))throw new Error('namespace');
    const relative=pathname.slice(5),extension=relative.split('.').at(-1);
    if(!mime[extension]||!relative||!/^[A-Za-z0-9_ .\/-]+$/.test(relative)||relative.split('/').some(part=>part==='.'||part==='..'))throw new Error('path');
    const body=execFileSync('git',['-C',repo,'show',`${revision}:design/ui-v1/${relative}`],{maxBuffer:16*1024*1024,stdio:['ignore','pipe','ignore']});
    response.writeHead(200,{'Content-Type':mime[extension],'Content-Length':body.length,'X-Portico-Design-Revision':revision,'Cache-Control':'no-store'});
    response.end(request.method==='HEAD'?undefined:body);
  }catch{response.writeHead(404);response.end();}
});
const shutdown=()=>server.close(()=>process.exit(0));
process.on('SIGTERM',shutdown);process.on('SIGINT',shutdown);
server.listen(port,'127.0.0.1',()=>console.log(`reference ${revision} on 127.0.0.1:${port}`));
setTimeout(shutdown,10*60*1000).unref();
