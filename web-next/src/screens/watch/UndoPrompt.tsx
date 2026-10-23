import type {CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import type {WebPlaybackRuntimeSnapshot} from '../../platform/playback';
import {Icon} from '../../kit/Icon';
import {dispatchWatchMarker, watchUndoOffer, type WatchPolicyBinding} from './policy';
import corner from './skip-prompt.module.css';
import styles from './undo-prompt.module.css';

/** desktop-player f2 automatic skip / phone-player 2b. The real policy supplies every tick. */
export function WatchUndoPrompt({binding, snapshot, expanded, controlsVisible, layout, style}: {
  binding: WatchPolicyBinding; snapshot: WebPlaybackRuntimeSnapshot; expanded: boolean; controlsVisible: boolean;
  layout: 'desktop' | 'tablet' | 'phone'; style?: CSSProperties;
}) {
  const offer = watchUndoOffer(binding, snapshot);
  if (!expanded || !offer) return null;
  return <div className={corner.corner} data-watch-undo data-layout={layout} data-controls-visible={controlsVisible} style={style} role="status">
    <span className={styles.notice} data-layout={layout}>
      {defaultI18n.t('player.skipped', {kind: offer.marker.kind})}
      <button type="button" onClick={() => { void dispatchWatchMarker(binding, snapshot, {kind: 'undo', markerId: offer.marker.id}).catch(binding.reportError); }}>
        <Icon name="i-back" size={18} />{defaultI18n.t('action.undo')}
      </button>
      <span className={styles.remaining} aria-hidden="true" style={{width: `${offer.fraction * 100}%`}} />
    </span>
  </div>;
}
