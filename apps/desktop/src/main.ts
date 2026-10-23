import {installPlaybackEnvironment} from '../../../web-next/src/platform/playback/index.ts';
import {nativeBridge} from './bridge';
import {DesktopMediaPlayer} from './player';
import {installWindowChromeHost,type WindowChromeInstallation} from '../../../web-next/src/platform/window-chrome.ts';
import {createDesktopWindowChromeHost} from './window-host';
import {nativeWindowBridge} from './window';
import {installSettingsPlatform} from '../../../web-next/src/platform/settings-platform.ts';
import {DesktopSettingsEqualizer,desktopEqualizerPort} from './SettingsEqualizer';
import {createDesktopDevicePort,nativeDeviceFacts} from './device';

async function startDesktop() {
  // The Settings shell captures its platform before restoration. Only the native
  // window can identify this OS; viewport width and the WebView UA are not authority.
  const native=await nativeWindowBridge.read();
  const facts=await nativeDeviceFacts().catch(()=>undefined);
  const device=facts?createDesktopDevicePort(facts):undefined;
  const stopSettings=installSettingsPlatform({environment:{platform:'desktop',os:native.os},...(device?{device}:{}),Equalizer:DesktopSettingsEqualizer});
  let windowChrome:WindowChromeInstallation|undefined;
  const windowHost=createDesktopWindowChromeHost({onEnvironment:environment=>windowChrome?.updateEnvironment(environment)});
  windowChrome=installWindowChromeHost(windowHost);
  windowChrome.updateEnvironment({os:native.os,fullscreen:native.fullscreen});
  window.addEventListener('pagehide',()=>{stopSettings();device?.dispose();windowChrome?.dispose();},{once:true});
  installPlaybackEnvironment({
    createPlayer: (video, options) => new DesktopMediaPlayer(video, options, nativeBridge),
    createRender: null,
    subtitleSurface: false,
    equalizerPort: desktopEqualizerPort,
  });
  await import('../../../web-next/src/main.tsx');
}
void startDesktop().catch(()=>console.error('desktop_start_failed'));
