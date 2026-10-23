import type {CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import type {WebPlaybackRuntimeSnapshot} from '../../platform/playback';
import {Icon} from '../../kit/Icon';
import {currentWatchPolicy, dispatchWatchMarker, type WatchPolicyBinding} from './policy';
import styles from './skip-prompt.module.css';

/** Desktop/tablet draw different prompt glyphs from the phone's shared catalogue. */
function PromptGlyph({kind}: {kind: 'dismiss' | 'next'}) {
  return <svg className={styles.glyph} width={20} height={20} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
    {kind === 'dismiss'
      ? <path fill="none" stroke="currentColor" strokeWidth={2.2} strokeLinecap="round" d="M6.5 6.5l11 11M17.5 6.5l-11 11" />
      : <path fill="currentColor" d="M18.5 5.2a1.1 1.1 0 0 0-2.2 0v13.6a1.1 1.1 0 0 0 2.2 0zm-3.9 7.6a1 1 0 0 0 0-1.6L5.9 5.1A1 1 0 0 0 4.3 6v12a1 1 0 0 0 1.6.9z" />}
  </svg>;
}

/** desktop-player f2, phone-player r2 and tablet-play f1. No engine, skip clock or timer. */
export function WatchSkipPrompt({binding, snapshot, expanded, controlsVisible, layout, style}: {
  binding: WatchPolicyBinding; snapshot: WebPlaybackRuntimeSnapshot; expanded: boolean; controlsVisible: boolean;
  layout: 'desktop' | 'tablet' | 'phone'; style?: CSSProperties;
}) {
  const current = currentWatchPolicy(binding, snapshot), policy = current?.shown;
  if (!expanded || !policy || policy.attention || policy.finished || policy.prompt?.kind !== 'skip'
    || current?.live?.attention || current?.live?.finished || current?.live?.prompt?.kind !== 'skip'
    || current.live.prompt.marker.id !== policy.prompt.marker.id) return null;
  const marker = policy.prompt.marker;
  const act = (kind: 'skip' | 'dismiss') => { void dispatchWatchMarker(binding, snapshot, {kind, markerId: marker.id}).catch(binding.reportError); };
  return <div className={styles.corner} data-watch-skip data-layout={layout} data-controls-visible={controlsVisible} style={style}>
    {layout !== 'phone' && <button className={styles.dismiss} type="button" aria-label={defaultI18n.t('action.dismiss')} onClick={() => act('dismiss')}><PromptGlyph kind="dismiss" /></button>}
    <button className={styles.skip} type="button" onClick={() => act('skip')}>{defaultI18n.t('player.skip', {kind: marker.kind})}{layout === 'phone' ? <Icon name="i-next" size={18} /> : <PromptGlyph kind="next" />}</button>
  </div>;
}
