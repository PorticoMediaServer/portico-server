/** A bounded reader for server-sent events over `fetch`.
 *
 * The browser's EventSource cannot send an Authorization header, and every Portico stream
 * needs one. This reads the same wire format from a fetch response: `event:`, `data:` (joined
 * across lines), `id:` and `retry:`, frames separated by a blank line, comment lines ignored.
 * One frame is capped so a misbehaving server cannot grow memory without bound. */
export type ServerEvent=Readonly<{event:string;data:string;id:string}>;
const MAX_FRAME=262144;

export async function readEventStream(response:Response,onEvent:(event:ServerEvent)=>void,signal?:AbortSignal):Promise<void>{
 if(!response.ok)throw Object.assign(new Error('The event stream was refused.'),{status:response.status});
 if(!response.body)throw new Error('This client cannot read event streams.');
 // A platform without streaming fetch (React Native) adapts its own transport and may hand over
 // text directly, so the decoder is only made when bytes actually arrive.
 const reader=(response.body as unknown as {getReader():{read():Promise<{value?:Uint8Array|string;done:boolean}>;cancel():Promise<void>;releaseLock():void}}).getReader();let decoder:TextDecoder|undefined;
 let buffer='',event='',data:string[]=[],id='';
 const abort=()=>{void reader.cancel().catch(()=>{});};
 signal?.addEventListener('abort',abort,{once:true});
 const dispatch=()=>{if(data.length||event)onEvent(Object.freeze({event:event||'message',data:data.join('\n'),id}));event='';data=[];};
 try{
  for(;;){
   if(signal?.aborted)return;
   const {value,done}=await reader.read();
   if(done){return;}
   buffer+=typeof value==='string'?value:(decoder??=new TextDecoder()).decode(value,{stream:true});
   if(buffer.length>MAX_FRAME)throw new Error('The event stream sent an oversized frame.');
   let index:number;
   while((index=buffer.search(/\r\n|\n|\r/))>=0){
    const line=buffer.slice(0,index);
    buffer=buffer.slice(index+(buffer.startsWith('\r\n',index)?2:1));
    if(line===''){dispatch();continue;}
    if(line.startsWith(':'))continue;
    const colon=line.indexOf(':'),field=colon<0?line:line.slice(0,colon);
    let value2=colon<0?'':line.slice(colon+1);if(value2.startsWith(' '))value2=value2.slice(1);
    if(field==='event')event=value2;else if(field==='data')data.push(value2);else if(field==='id'&&!value2.includes('\0'))id=value2;
   }
  }
 }finally{signal?.removeEventListener('abort',abort);try{reader.releaseLock();}catch{}}
}
