import {installPlaybackEnvironment} from '../../../web-next/src/platform/playback/index.ts';
import {installSavedDownloadsBinder} from '../../../web-next/src/screens/saved/native-binding.ts';
import {nativeBridge} from './bridge';
import {DesktopMediaPlayer} from './player';
import {installWindowChromeHost,type WindowChromeInstallation} from '../../../web-next/src/platform/window-chrome.ts';
import {createDesktopNativeWindowHost} from './window-native-host.ts';
import {nativeWindowBridge} from './window';
import {installSettingsPlatform} from '../../../web-next/src/platform/settings-platform.ts';
import {DesktopSettingsEqualizer,desktopEqualizerPort} from './SettingsEqualizer';
import {createDesktopDevicePort,nativeDeviceFacts} from './device';
import {nativeCapabilities} from './capabilities';
import {installPlatformInstallationProvider} from '../../../web-next/src/platform/installation-provider.ts';
import {nativeInstallation} from './installation';
import {createDesktopDownloadsHost} from './downloads-host.ts';
import {createDesktopAppLifetime} from './app-lifetime.ts';
import {installDesktopCloseHost} from './close-host.ts';
import {browserViewerStorage} from '../../../web-next/src/platform/viewer-selection-storage.ts';
import {installOfflineDownloads} from '../../../web-next/src/shell/offline-downloads.ts';
import {offlineDownloadScope} from './offline-scope.ts';

const lifetime=createDesktopAppLifetime();
const close=()=>{void lifetime.dispose().catch(()=>console.warn('desktop_shutdown_failed'));};
const closeHost=installDesktopCloseHost({events:window,disposeApp:()=>lifetime.dispose(),acknowledge:()=>nativeWindowBridge.action('close'),onError:()=>console.warn('desktop_shutdown_failed')});
lifetime.retain(()=>closeHost.dispose());
async function startDesktop(){
  const native=await nativeWindowBridge.read();lifetime.assertCurrent();
  lifetime.retain(installPlatformInstallationProvider(nativeInstallation));
  const facts=await nativeDeviceFacts().catch(()=>undefined);lifetime.assertCurrent();
  const device=facts?createDesktopDevicePort(facts):undefined;if(device)lifetime.retain(()=>device.dispose());
  lifetime.retain(installSettingsPlatform({environment:{platform:'desktop',os:native.os},...(device?{device}:{}),Equalizer:DesktopSettingsEqualizer,equalizerPort:desktopEqualizerPort}));
  let windowChrome:WindowChromeInstallation|undefined;
  const windowHost=createDesktopNativeWindowHost({onEnvironment:value=>windowChrome?.updateEnvironment(value),prepareToClose:()=>lifetime.dispose()});
  windowChrome=installWindowChromeHost(windowHost);windowChrome.updateEnvironment({os:native.os,fullscreen:native.fullscreen});
  lifetime.retain(()=>{windowChrome?.dispose();return windowHost.dispose();});
  const downloads=createDesktopDownloadsHost({reportFailure:()=>console.warn('downloads_unavailable')});
  lifetime.retain(installSavedDownloadsBinder(downloads.bindSaved));
  lifetime.retain(installPlaybackEnvironment({
    capabilities:()=>facts?.appVersion?nativeCapabilities(native.os,facts.appVersion):Promise.reject(new Error('engine_unavailable')),
    createPlayer:(video,options)=>new DesktopMediaPlayer(video,options,nativeBridge,true,downloads.broker),
    attachLocalQueue:downloads.attachLocalQueue,
    createRender:null,subtitleSurface:false,equalizerPort:desktopEqualizerPort,
  }));
  lifetime.retain(()=>downloads.dispose());
  // The launch page offers this device's completed downloads from the persisted,
  // validated scope; no server call, no player and no second downloads owner.
  const selected=await browserViewerStorage.read().catch(()=>null);lifetime.assertCurrent();
  const offlineScope=offlineDownloadScope(selected?.session,Date.now());
  const deadline=selected?Date.parse(selected.session.authorizationHorizon||selected.session.expiresAt):NaN;
  // The exact retained selection lease, not only its horizon: a different viewer, profile,
  // server or family must never inherit this list.
  const lease=selected&&offlineScope?JSON.stringify([offlineScope.authority,offlineScope.serverId,offlineScope.accountId,offlineScope.profileId,selected.session.sessionFamilyId,selected.session.tokenGeneration]):'';
  if(offlineScope&&lease){
    const port=await downloads.bindOfflineDownloads(offlineScope,lifetime.signal).catch(()=>undefined);
    if(port){
      lifetime.retain(()=>port.dispose());
      const listeners=new Set<()=>void>();
      let retired=false,timer:ReturnType<typeof setTimeout>|undefined;
      let snapshot:{phase:'loading'|'ready'|'unavailable';items:readonly {id:string;itemId:string;revision:number;quality:string;bytes:number;title:string;artist:string;kind:string;onServer:boolean}[]}={phase:'loading',items:[]};
      const unavailable=Object.freeze({phase:'unavailable' as const,items:Object.freeze([]) as readonly never[]});
      const publish=(next:typeof snapshot)=>{if(retired)return;snapshot=next;for(const listener of [...listeners])listener();};
      const same=(record:Awaited<ReturnType<typeof browserViewerStorage.read>>)=>{const next=record&&offlineDownloadScope(record.session,Date.now());return !!next&&JSON.stringify([next.authority,next.serverId,next.accountId,next.profileId,record!.session.sessionFamilyId,record!.session.tokenGeneration])===lease;};
      const live=()=>!retired&&!lifetime.signal.aborted&&Number.isFinite(deadline)&&deadline>Date.now();
      const retire=()=>{if(retired)return;retired=true;if(timer){clearTimeout(timer);timer=undefined;}snapshot=unavailable;for(const listener of [...listeners])listener();port.dispose();};
      // One captured deadline clears the visible list even if nothing else renders or reads.
      if(Number.isFinite(deadline))timer=setTimeout(retire,Math.max(0,deadline-Date.now()));
      const leased=async()=>live()&&same(await browserViewerStorage.read().catch(()=>null));
      const refresh=async()=>{
        if(!live()){retire();return;}
        const items=await port.list(new AbortController().signal).catch(()=>undefined);
        if(items===undefined||!live()){retire();return;}
        // Re-check the exact lease after the list returns, before anything is published.
        if(!await leased()){retire();return;}
        publish(items.length?{phase:'ready',items}:{phase:'unavailable',items:[]});
      };
      void refresh();
      lifetime.retain(installOfflineDownloads({
        subscribe:listener=>{listeners.add(listener);return()=>{listeners.delete(listener);};},
        snapshot:()=>{if(!live()){void Promise.resolve().then(retire);return unavailable;}return snapshot;},
        openFolder:async()=>{
          if(!await leased()){retire();throw new DOMException('Aborted','AbortError');}
          await port.openFolder(new AbortController().signal);
          if(!await leased())retire();else void refresh();
        },
        retire,
      }));
    }
  }
  lifetime.retain(()=>downloads.dispose());
  // The launch page offers this device's completed downloads from the persisted,
  // validated scope; no server call, no player and no second downloads owner.
  const selected=await browserViewerStorage.read().catch(()=>null);lifetime.assertCurrent();
  const offlineScope=offlineDownloadScope(selected?.session,Date.now());
  const horizon=selected?Date.parse(selected.session.authorizationHorizon||selected.session.expiresAt):NaN;
  // The persisted authority, not the launch, decides for how long the list may be shown.
  const alive=()=>!lifetime.signal.aborted&&Number.isFinite(horizon)&&horizon>Date.now();
  if(offlineScope){
    const port=await downloads.bindOfflineDownloads(offlineScope,lifetime.signal).catch(()=>undefined);
    if(port){
      lifetime.retain(()=>port.dispose());
      const listeners=new Set<()=>void>();
      let snapshot:{phase:'loading'|'ready'|'unavailable';items:readonly {id:string;itemId:string;revision:number;quality:string;bytes:number;title:string;artist:string;kind:string;onServer:boolean}[]}={phase:'loading',items:[]};
      const publish=(next:typeof snapshot)=>{snapshot=next;for(const listener of [...listeners])listener();};
      const retire=()=>{snapshot={phase:'unavailable',items:[]};for(const listener of [...listeners])listener();port.dispose();};
      const refresh=()=>alive()
        ?port.list(new AbortController().signal).then(items=>{if(alive())publish(items.length?{phase:'ready',items}:{phase:'unavailable',items:[]});}).catch(()=>{if(alive())publish({phase:'unavailable',items:[]});})
        :Promise.resolve(retire());
      void refresh();
      lifetime.retain(installOfflineDownloads({
        subscribe:listener=>{listeners.add(listener);return()=>{listeners.delete(listener);};},
        snapshot:()=>{if(!alive())retire();return snapshot;},
        openFolder:async()=>{if(!alive()){retire();throw new DOMException('Aborted','AbortError');}await port.openFolder(new AbortController().signal);void refresh();},
        retire,
      }));
    }
  }
  // The original app owns all screen/router/React assembly and captured viewers.
  await lifetime.importWebApp(()=>import('../../../web-next/src/main.tsx'));
  if(import.meta.env.DEV)void import('./dev-measurement.ts').then(module=>module.installDesktopMeasurement(lifetime.signal)).catch(()=>console.warn('desktop_measurement_unavailable'));
}
void startDesktop().catch(error=>{
  if(!lifetime.signal.aborted&&!(error instanceof DOMException&&error.name==='AbortError'))console.error('desktop_start_failed');
  close();
});
