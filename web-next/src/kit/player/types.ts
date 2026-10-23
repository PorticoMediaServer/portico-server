import type {RefObject} from 'react';
import type {DockPresentation} from '../dock';
import type {PlayOnController, PlayOnSnapshot} from '../../platform/playback/play-on';

/** Display artwork is resolved by the authority-owned shell, never sent as a media grant. */
export type CastPlayerVisuals = Pick<DockPresentation, 'identity' | 'artwork' | 'ambient' | 'palette' | 'skipBackSeconds' | 'skipForwardSeconds'> & Readonly<{
  onMore?: () => void; onMyList?: () => void;
}>;
export type CastPlayerProps = Readonly<{
  controller: PlayOnController; snapshot: PlayOnSnapshot; visuals?: CastPlayerVisuals; phone: boolean;
  returnFocusRef?: RefObject<HTMLElement | null>; reportError: (error: unknown) => void;
}>;
export type PlayOnPanelProps = Pick<CastPlayerProps, 'controller' | 'snapshot' | 'phone' | 'reportError'> & Readonly<{anchorRef: RefObject<HTMLElement | null>}>;
