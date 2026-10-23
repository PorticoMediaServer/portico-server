import {parseSelectedViewerRecord,type SelectedViewerRecord,type SelectedViewerStorage} from '@core/viewer-selection';
import {BrowserProtectedStore} from './server-connections';

/** Signing in lasts: the selected viewer is kept for this browser, not for one tab, so closing
 * the window or restarting the computer does not sign anyone out. The server decides how long a
 * sign-in is good for (ninety days from its last use) and can end it at any moment; this only
 * stops the browser from forgetting it first. A tab keeps the viewer it started with until it
 * is reloaded or the server ends that sign-in; a tab whose own sign-in has ended never removes a
 * record another tab saved since (`clearIfFamily`).
 *
 * SEC-12: the record carries the viewer's access token, so it is never script-readable
 * plaintext. It lives in IndexedDB encrypted with AES-GCM under a non-extractable key made for
 * this installation (`BrowserProtectedStore`, the same vault as the refresh credential): a dump
 * of localStorage or IndexedDB yields ciphertext and a key reference that can't be exported.
 * At run time the token is only in memory. No plaintext localStorage record is read. */
export function createBrowserViewerStorage(store: Pick<BrowserProtectedStore<SelectedViewerRecord>, 'read' | 'change'>): SelectedViewerStorage {
return {
 async read(){
  const value=await store.read();
  if(!value)return null;
  try{return parseSelectedViewerRecord(value);}catch{await store.change(()=>undefined);return null;}
 },
 async write(record){
  if(record){
   const json=JSON.stringify(record);
   await store.change(()=>JSON.parse(json) as SelectedViewerRecord);
   if(JSON.stringify(await store.read())!==json)throw new Error('Server credentials could not be read back from this browser.');
  }else{
   await store.change(()=>undefined);
   if((await store.read())!==undefined)throw new Error('The old server credential could not be removed from this browser.');
  }
 },
 async clearIfFamily(sessionFamilyId){
  // One step in the store, so a record another tab saves in the meantime is never the one removed.
  await store.change(current=>{
   if(!current)return undefined;
   let family:string|undefined;
   try{family=parseSelectedViewerRecord(current)?.session.sessionFamilyId;}catch{return undefined;}
   return family===sessionFamilyId?undefined:current;
  });
 },
};
}
export const browserViewerStorage = createBrowserViewerStorage(new BrowserProtectedStore<SelectedViewerRecord>('portico.selected-viewer.v2'));
