import type {PreparedChoice} from '@core/prepared-media.ts';
import type {MenuAnchor} from '../ui/Menu';
import type {ContentEntry} from '@core/library-content.ts';
import type {GuideChannel} from '@core/channel-guide.ts';
import {useContext} from 'react';
import {ActionsCtx, type EntryOrigin, type SequenceContainer} from './engine';

/** Player intents available to every screen. */
export type PlayerActions = {
  /** `prepared`/`quality`: a version picked on the title page before Play. */
  play: (itemId: string, startSeconds: number, entry?: ContentEntry, prepared?: PreparedChoice, quality?: string) => void;
  /** `container`: on Playback v1, a show/season/album/artist/disc/collection/playlist/book plays as one server request. */
  playSequence: (entries: readonly ContentEntry[], index: number, startSeconds: number, container?: SequenceContainer) => void;
  tune: (channel: GuideChannel) => void;
  /** `origin`: where the card sits, when that changes its menu (a recommendation offers Not interested). */
  more: (entry: ContentEntry, anchor?: MenuAnchor, origin?: EntryOrigin) => void;
};
export function usePlayerActions(): PlayerActions {
  const v = useContext(ActionsCtx);
  if (!v) throw new Error('usePlayerActions must be used inside PlayerProvider.');
  return v;
}
