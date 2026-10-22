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
} as const satisfies Record<`next.${string}`, string>;

export type NextMessageId = keyof typeof nextMessages;
