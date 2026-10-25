import {parseSelectedViewerRecord,selectedViewerRecord,type SelectedViewerRecord,type SelectedViewerStorage} from '@core/viewer-selection';
import {BrowserProtectedStore,browserAppSessions} from './server-connections';
import type {AppSessionStore} from '@core/app-sessions.ts';
import {forgetAllShells, forgetShell} from '../shell/last-known';


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
   forgetAllShells();
   await store.change(()=>undefined);
   if((await store.read())!==undefined)throw new Error('The old server credential could not be removed from this browser.');
  }
 },
 async clearIfFamily(sessionFamilyId){
  forgetShell(sessionFamilyId);
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
/** Selection is a projection of the same protected multi-server record as refresh and routes.
 * No second credential vault can retain a signed-out family. Existing originals are untouched. */
export function createAppViewerStorage(store:AppSessionStore):SelectedViewerStorage {
 return {
  async read(){const entry=await store.selected();return entry?selectedViewerRecord(entry.connection.logicalOrigin,entry.connection.session,true):null;},
  async write(record){
   if(!record){const entry=await store.selected();if(entry)await store.signOutServer(entry.connection.pin.serverId,entry.connection.session.sessionFamilyId);return;}
   const state=await store.read(),entry=state.servers.find(value=>value.connection.pin.serverId===record.session.viewer.serverId);
   if(!entry||entry.connection.session.sessionFamilyId!==record.session.sessionFamilyId||entry.connection.session.viewer.profileId!==record.session.viewer.profileId||entry.connection.logicalOrigin!==record.serverUrl)throw new Error('Save the verified server connection before selecting it.');
   await store.select(entry.connection.pin.serverId);
  },
  async clearIfFamily(family){const state=await store.read();const entry=state.servers.find(value=>value.connection.pin.serverId===state.selectedServerId&&value.connection.session.sessionFamilyId===family);if(entry)await store.signOutServer(entry.connection.pin.serverId,family);},
 };
}
export const browserViewerStorage:SelectedViewerStorage = {
 read:()=>createAppViewerStorage(browserAppSessions()).read(),
 write:record=>createAppViewerStorage(browserAppSessions()).write(record),
 clearIfFamily:family=>createAppViewerStorage(browserAppSessions()).clearIfFamily!(family),
};
