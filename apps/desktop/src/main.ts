import {installPlaybackEnvironment} from '../../../web-next/src/platform/playback/index.ts';
import {nativeBridge} from './bridge';
import {DesktopMediaPlayer} from './player';
import {installWindowChromeHost,type WindowChromeInstallation} from '../../../web-next/src/platform/window-chrome.ts';
import {createDesktopWindowChromeHost} from './window-host';

let windowChrome:WindowChromeInstallation|undefined;
const windowHost=createDesktopWindowChromeHost({onEnvironment:environment=>windowChrome?.updateEnvironment(environment)});
windowChrome=installWindowChromeHost(windowHost);
window.addEventListener('pagehide',()=>windowChrome?.dispose(),{once:true});

installPlaybackEnvironment({
  createPlayer: (video, options) => new DesktopMediaPlayer(video, options, nativeBridge),
  createRender: null,
  subtitleSurface: false,
});
void import('../../../web-next/src/main.tsx');
