import {parseSelectedViewerRecord,type SelectedViewerRecord,type SelectedViewerStorage} from '@core/viewer-selection';
import {BrowserProtectedStore} from './server-connections';

/** Signing in lasts: the selected viewer is kept for this browser, not for one tab, so closing
 * the window or restarting the computer does not sign anyone out. The server decides how long a
 * sign-in is good for (ninety days from its last use) and can end it at any moment; this only
 * stops the browser from forgetting it first. A tab keeps the viewer it started with until it
 * is reloaded.
 *
 * SEC-12: the record carries the viewer's access token, so it is never script-readable
 * plaintext. It lives in IndexedDB encrypted with AES-GCM under a non-extractable key made for
 * this installation (`BrowserProtectedStore`, the same vault as the refresh credential): a dump
 * of localStorage or IndexedDB yields ciphertext and a key reference that can't be exported.
 * At run time the token is only in memory. The old plaintext localStorage entry is moved once
 * and deleted. */
const legacyKey='portico.selected-viewer.v1',legacyQuarantine='portico.selected-viewer.quarantined.v1';
const store=new BrowserProtectedStore<SelectedViewerRecord>('portico.selected-viewer.v2');

let migrated:Promise<void>|undefined;
/** Moves a plaintext record left by an older version into the vault, then deletes it. */
function migrate():Promise<void>{
 migrated??=(async()=>{
  let raw:string|null=null,quarantined=false;
  try{raw=localStorage.getItem(legacyKey);quarantined=localStorage.getItem(legacyQuarantine)==='true';}catch{return;}
  if(raw===null&&!quarantined)return;
  try{
   const record=!quarantined&&raw?parseSelectedViewerRecord(JSON.parse(raw)):null;
   if(record)await store.change(current=>current??record);
  }catch{/* An unreadable old record is dropped, as before. */}
  finally{try{localStorage.removeItem(legacyKey);localStorage.removeItem(legacyQuarantine);}catch{}}
 })();
 return migrated;
}

export const browserViewerStorage:SelectedViewerStorage={
 async read(){
  await migrate();
  const value=await store.read();
  if(!value)return null;
  try{return parseSelectedViewerRecord(value);}catch{await store.change(()=>undefined);return null;}
 },
 async write(record){
  await migrate();
  if(record){
   const json=JSON.stringify(record);
   await store.change(()=>JSON.parse(json) as SelectedViewerRecord);
   if(JSON.stringify(await store.read())!==json)throw new Error('Server credentials could not be read back from this browser.');
  }else{
   await store.change(()=>undefined);
   if((await store.read())!==undefined)throw new Error('The old server credential could not be removed from this browser.');
  }
 },
};
