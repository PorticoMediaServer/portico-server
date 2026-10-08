import {parseTrustedProfile,trustScopeKey,type ProfileScope,type TrustedProfile} from '@core/profile-management';
import {BrowserProtectedStore} from './server-connections';

/** Only the non-secret installation identifier survives in localStorage. */
export function browserInstallation():string{
 try{const key='portico.profile-installation.v1';let id=localStorage.getItem(key);if(!id){id=crypto.randomUUID();localStorage.setItem(key,id);}return id;}catch{return '';}
}

/**
 * "Remember this profile" handles (a trust token that skips the profile PIN on this installation)
 * are device credentials. SEC-12: they live in IndexedDB encrypted under a non-extractable
 * AES-GCM key (`BrowserProtectedStore`), never as script-readable plaintext; old plaintext
 * localStorage entries are moved once and deleted. Reads are synchronous from a copy held in
 * memory, loaded at start (`browserTrustLoaded` before relying on one); until it has loaded, a
 * profile simply asks for its PIN.
 */
type TrustMap=Record<string,TrustedProfile>;
type Scope=ProfileScope&{serverId:string;profileId:string;installationId:string};
const MAX=64,legacyPrefix='["portico-profile-trust-v1"';
let cache:TrustMap={},store:BrowserProtectedStore<TrustMap>|undefined,loaded:Promise<void>|undefined;
const vault=()=>store??=new BrowserProtectedStore<TrustMap>('portico.profile-trust.v2');

function legacyEntries():[string,string][]{
 const out:[string,string][]=[];
 try{for(let i=0;i<localStorage.length;i++){const k=localStorage.key(i);if(k?.startsWith(legacyPrefix)){const v=localStorage.getItem(k);if(v)out.push([k,v]);}}}catch{}
 return out;
}

/** Loads the saved handles (moving any plaintext ones into the vault first). */
export function browserTrustLoaded():Promise<void>{
 loaded??=(async()=>{
  const legacy=legacyEntries();
  try{
   if(legacy.length){
    const moved:TrustMap={};for(const [k,v] of legacy){try{moved[k]=JSON.parse(v) as TrustedProfile;}catch{}}
    await vault().change(current=>({...moved,...current}));
   }
   cache={...(await vault().read())??{},...cache};
  }catch{/* No vault (private window, old browser): handles last for this page only. */}
  finally{for(const [k] of legacy){try{localStorage.removeItem(k);}catch{}}}
 })();
 return loaded;
}
if(typeof indexedDB!=='undefined'&&typeof navigator!=='undefined'&&navigator.locks)void browserTrustLoaded();

function persist(update:(current:TrustMap)=>TrustMap){
 void browserTrustLoaded().then(()=>vault().change(current=>{const next=update({...current});const keys=Object.keys(next);for(const k of keys.slice(0,Math.max(0,keys.length-MAX)))delete next[k];return keys.length?next:undefined;})).catch(()=>{});
}

export function readBrowserTrust(scope:Scope):TrustedProfile|undefined{
 const key=trustScopeKey(scope),raw=cache[key];if(!raw)return undefined;
 try{return parseTrustedProfile(raw,scope);}catch{forgetBrowserTrust(scope);return undefined;}
}
export function saveBrowserTrust(proof:TrustedProfile){const key=trustScopeKey(proof);cache={...cache,[key]:proof};persist(current=>({...current,[key]:proof}));}
export function forgetBrowserTrust(scope:Scope){try{const key=trustScopeKey(scope);const {[key]:_gone,...rest}=cache;cache=rest;persist(current=>{delete current[key];return current;});}catch{}}
