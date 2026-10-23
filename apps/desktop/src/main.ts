import {installPlaybackEnvironment} from '../../../web-next/src/platform/playback/index.ts';
import {nativeBridge} from './bridge';
import {DesktopMediaPlayer} from './player';

installPlaybackEnvironment({
  createPlayer: (video, options) => new DesktopMediaPlayer(video, options, nativeBridge),
  createRender: null,
  subtitleSurface: false,
});
void import('../../../web-next/src/main.tsx');
