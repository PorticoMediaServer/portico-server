import type {CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import type {WebPlaybackRuntimeSnapshot} from '../../platform/playback';
import {Icon} from '../../kit/Icon';
import {dispatchWatchMarker, watchUndoOffer, type WatchPolicyBinding} from './policy';
import corner from './skip-prompt.module.css';
import styles from './undo-prompt.module.css';
import motion from './prompt-motion.module.css';
import {promptSnapStyle, usePromptPresence, usePromptReducedMotion} from './prompt-motion';

/** desktop-player f2 automatic skip / phone-player 2b. The real policy supplies every tick. */
export function WatchUndoPrompt({binding, snapshot, expanded, controlsVisible, layout, style}: {
  binding: WatchPolicyBinding; snapshot: WebPlaybackRuntimeSnapshot; expanded: boolean; controlsVisible: boolean;
  layout: 'desktop' | 'tablet' | 'phone'; style?: CSSProperties;
}) {
  const offer = watchUndoOffer(binding, snapshot), reduced = usePromptReducedMotion();
  const presence = usePromptPresence(expanded && offer ? {offer} : undefined, expanded && offer ? offer.marker.id : undefined, reduced);
  const shown = presence.present?.offer;
  if (!shown) return null;
  return <div className={`${corner.corner} ${motion.prompt}`} data-watch-undo data-layout={layout} data-controls-visible={controlsVisible}
    data-prompt-phase={presence.phase} inert={presence.phase === 'out'} aria-hidden={presence.phase === 'out' || undefined}
    style={{...promptSnapStyle, ...style}} role="status">
    <span className={styles.notice} data-layout={layout}>
      {defaultI18n.t('player.skipped', {kind: shown.marker.kind})}
      <button type="button" onClick={() => { void dispatchWatchMarker(binding, snapshot, {kind: 'undo', markerId: shown.marker.id}).catch(binding.reportError); }}>
        <Icon name="i-back" size={18} />{defaultI18n.t('action.undo')}
      </button>
      <span className={styles.remaining} aria-hidden="true" style={{width: `${shown.fraction * 100}%`}} />
    </span>
  </div>;
}
