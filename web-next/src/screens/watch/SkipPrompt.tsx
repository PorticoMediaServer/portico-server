import type {CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import type {WebPlaybackRuntimeSnapshot} from '../../platform/playback';
import {Icon} from '../../kit/Icon';
import {currentWatchPolicy, dispatchWatchMarker, type WatchPolicyBinding} from './policy';
import {WatchPromptGlyph} from './PromptGlyph';
import styles from './skip-prompt.module.css';
import motion from './prompt-motion.module.css';
import {promptSnapStyle, usePromptPresence, usePromptReducedMotion} from './prompt-motion';

/** desktop-player f2, phone-player r2 and tablet-play f1. No engine, skip clock or timer. */
export function WatchSkipPrompt({binding, snapshot, expanded, controlsVisible, layout, style}: {
  binding: WatchPolicyBinding; snapshot: WebPlaybackRuntimeSnapshot; expanded: boolean; controlsVisible: boolean;
  layout: 'desktop' | 'tablet' | 'phone'; style?: CSSProperties;
}) {
  const current = currentWatchPolicy(binding, snapshot), policy = current?.shown;
  const open = expanded && !!policy && !policy.attention && !policy.finished && policy.prompt?.kind === 'skip'
    && !current?.live?.attention && !current?.live?.finished && current?.live?.prompt?.kind === 'skip'
    && current.live.prompt.marker.id === policy.prompt.marker.id;
  const reduced = usePromptReducedMotion(), presence = usePromptPresence(open ? {marker: policy!.prompt!.marker} : undefined, open ? policy!.prompt!.marker.id : undefined, reduced);
  const marker = presence.present?.marker;
  if (!marker) return null;
  const act = (kind: 'skip' | 'dismiss') => { void dispatchWatchMarker(binding, snapshot, {kind, markerId: marker.id}).catch(binding.reportError); };
  return <div className={`${styles.corner} ${motion.prompt}`} data-watch-skip data-layout={layout} data-controls-visible={controlsVisible}
    data-prompt-phase={presence.phase} inert={presence.phase === 'out'} aria-hidden={presence.phase === 'out' || undefined}
    style={{...promptSnapStyle, ...style}}>
    {layout !== 'phone' && <button className={styles.dismiss} type="button" aria-label={defaultI18n.t('action.dismiss')} onClick={() => act('dismiss')}><WatchPromptGlyph kind="dismiss" className={styles.glyph} /></button>}
    <button className={styles.skip} type="button" onClick={() => act('skip')}>{defaultI18n.t('player.skip', {kind: marker.kind})}{layout === 'phone' ? <Icon name="i-next" size={18} /> : <WatchPromptGlyph kind="next" className={styles.glyph} />}</button>
  </div>;
}
