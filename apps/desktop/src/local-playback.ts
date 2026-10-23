import {localPlayerSource,localPlayerLoadReceipt,type LocalPlayerSource,type LocalPlayerLoadContext,type LocalPlayerLoadReceipt} from '../../../packages/client-core/src/playback-v1/local-port.ts';
import {sameSavedDownloadsScope} from '../../../web-next/src/screens/saved/downloads-port.ts';
import type {DesktopDownloadStore,PreparedLocalQueue} from './downloads.ts';

export interface DesktopLocalPlayback {
  load(source:LocalPlayerSource,nativeGeneration:number,context:LocalPlayerLoadContext):Promise<LocalPlayerLoadReceipt>;
  retire(nativeGeneration:number):Promise<void>;
}
const cancelled=()=>new DOMException('Aborted','AbortError');

/** App-private custody of actual native preparations. This is not a second queue:
 * Web owns ordering and controls; native owns receipt validation and its journal. */
export class DesktopLocalPlaybackBroker implements DesktopLocalPlayback {
  private readonly preparations=new Map<string,{store:DesktopDownloadStore;prepared:PreparedLocalQueue}>();
  private readonly loads=new Map<number,{store:DesktopDownloadStore;prepared:PreparedLocalQueue;retired:boolean}>();
  private disposed=false;
  retain(store:DesktopDownloadStore,prepared:PreparedLocalQueue):void {
    store.assertCurrent();
    if(this.disposed||!sameSavedDownloadsScope(store.scope,prepared.scope)||this.preparations.has(prepared.preparationId))throw new Error('downloads_conflict');
    this.preparations.set(prepared.preparationId,{store,prepared});
  }
  async load(input:LocalPlayerSource,nativeGeneration:number,context:LocalPlayerLoadContext):Promise<LocalPlayerLoadReceipt>{
    const source=localPlayerSource(input),owned=this.preparations.get(source.preparationId);
    const member=owned?.prepared.items.find(item=>item.localSourceId===source.localSourceId);
    if(this.disposed||!owned||!member||member.id!==source.entryId||member.itemId!==source.itemId||!sameSavedDownloadsScope(source.scope,owned.prepared.scope)||this.loads.has(nativeGeneration))throw new Error('downloads_conflict');
    const current=()=>!this.disposed&&!context.signal.aborted&&context.isCurrent()&&this.preparations.get(source.preparationId)===owned;
    if(!current())throw cancelled();
    const load={...owned,retired:false};this.loads.set(nativeGeneration,load);
    try{
      const native=await owned.store.loadLocal(owned.prepared,source.localSourceId,nativeGeneration,source.startPositionMs,source.autoplay,context.signal);
      if(!current()||load.retired)throw cancelled();
      // Only the validated ACK for this exact private load can echo public generation.
      return localPlayerLoadReceipt({...native,generation:source.generation},source);
    }catch(error){
      load.retired=true;
      await owned.store.retireLocal(nativeGeneration).catch(()=>{});
      if(this.loads.get(nativeGeneration)===load)this.loads.delete(nativeGeneration);
      throw error;
    }
  }
  async retire(nativeGeneration:number):Promise<void>{
    const load=this.loads.get(nativeGeneration);if(!load)return;
    load.retired=true;this.loads.delete(nativeGeneration);
    await load.store.retireLocal(nativeGeneration);
  }
  async release(prepared:PreparedLocalQueue):Promise<void>{
    const owned=this.preparations.get(prepared.preparationId);
    if(!owned||owned.prepared!==prepared)return;
    this.preparations.delete(prepared.preparationId);
    const generations=[...this.loads].filter(([,load])=>load.prepared===prepared).map(([generation])=>generation);
    await Promise.all(generations.map(generation=>this.retire(generation)));
    await owned.store.releaseLocalQueue(prepared);
  }
  async dispose():Promise<void>{
    if(this.disposed)return;this.disposed=true;
    await Promise.all([...this.preparations.values()].map(({prepared})=>this.release(prepared)));
  }
}
