import {platformMessages} from './platform.ts';
import {controlsMessages} from './controls.ts';
import {overlaysMessages} from './overlays.ts';
import {artworkMessages} from './artwork.ts';
import {cardsMessages} from './cards.ts';
import {browseMessages} from './browse.ts';
import {layoutMessages} from './layout.ts';
import {statesMessages} from './states.ts';
import {shellMessages} from './shell.ts';
import {dockMessages} from './dock.ts';
import {homeMessages} from './home.ts';
import {controlsNativeMessages} from './controls-native.ts';
import {overlaysNativeMessages} from './overlays-native.ts';
import {artworkNativeMessages} from './artwork-native.ts';
import {cardsNativeMessages} from './cards-native.ts';
import {browseNativeMessages} from './browse-native.ts';
import {layoutNativeMessages} from './layout-native.ts';
import {statesNativeMessages} from './states-native.ts';
import {shellNativeMessages} from './shell-native.ts';
import {dockNativeMessages} from './dock-native.ts';
import {shared} from './shared.ts';

export const nextMessages = {
  ...platformMessages,
  ...controlsMessages,
  ...overlaysMessages,
  ...artworkMessages,
  ...cardsMessages,
  ...browseMessages,
  ...layoutMessages,
  ...statesMessages,
  ...shellMessages,
  ...dockMessages,
  ...homeMessages,
  ...controlsNativeMessages,
  ...overlaysNativeMessages,
  ...artworkNativeMessages,
  ...cardsNativeMessages,
  ...browseNativeMessages,
  ...layoutNativeMessages,
  ...statesNativeMessages,
  ...shellNativeMessages,
  ...dockNativeMessages,
  ...shared,
} as const satisfies Record<`next.${string}`, string>;

export type NextMessageId = keyof typeof nextMessages;
