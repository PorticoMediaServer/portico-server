import {useRef, type CSSProperties} from 'react';
import {EndControls, Identity, Progress, TerminalActions, Transport} from './parts';
import {remoteTerminal} from './model';
import {dockText} from './text';
import {useDockHandover} from './use-motion';
import type {DockRendererProps} from './types';
import styles from './dock.module.css';

export function DockBar(props: DockRendererProps) {
  const {view, t = dockText, className, style} = props, ref = useRef<HTMLElement | null>(null);
  useDockHandover(ref, `${view.playback.fence.sessionId}:${view.playback.fence.generation}:${view.mode}`);
  return <section ref={ref} className={[styles.bar, className].filter(Boolean).join(' ')} style={{'--dock-glow': view.presentation.palette?.first ?? 'transparent', ...style} as CSSProperties} data-mode={view.mode} data-native-underlay-branch="dock" aria-label={t('next.dock.nowPlaying')}>
    <div data-dock-identity><Identity {...props} /></div>
    {remoteTerminal(view.remote) ? <TerminalActions {...props} /> : <div className={styles.middle}><Transport {...props} /><Progress {...props} /></div>}
    {!remoteTerminal(view.remote) && <EndControls {...props} />}
  </section>;
}
