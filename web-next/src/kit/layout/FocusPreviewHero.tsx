import {useEffect, useRef, useState, type ReactNode} from 'react';
import {Hero, type HeroCandidate, type HeroProps} from './Hero';
import {useLayoutMotion} from './motion';
import styles from './layout.module.css';

export function usePreviewDebounce<T>(value: T, key: string | null, hold = false): T {
  const [settled, setSettled] = useState(value);
  const latest = useRef(value); latest.current = value;
  useEffect(() => {
    if (hold) return;
    const timer = window.setTimeout(() => setSettled(latest.current), 250);
    return () => window.clearTimeout(timer);
  }, [key, hold]);
  return settled;
}

export function PreviewDebounce<T>({value, getKey, hold = false, children}: {value: T; getKey: (value: T) => string | null; hold?: boolean; children: (settled: T) => ReactNode}) {
  return children(usePreviewDebounce(value, getKey(value), hold));
}

export type FocusPreviewHeroProps<T extends HeroCandidate> = Omit<HeroProps<T>, 'candidates' | 'activeIndex' | 'onIndexChange' | 'renderActions' | 'platform'> & {
  entry: T | null; holdingDirection?: boolean;
  /** Resolve when the actual scheduler-backed foreground is decoded. */
  prepareArtwork: (entry: T) => Promise<void>;
  onPreviewError?: () => void;
};

/** Only an actual bounded focus summary is accepted; artwork remains owned by its renderer. */
export function FocusPreviewHero<T extends HeroCandidate>({entry, holdingDirection = false, reducedMotion = false, renderArtwork, prepareArtwork, onPreviewError, className, ...props}: FocusPreviewHeroProps<T>) {
  const settled = usePreviewDebounce(entry, entry?.id ?? null, holdingDirection), [layers, setLayers] = useState<readonly T[]>([]);
  const motion = useLayoutMotion(reducedMotion), prepare = useRef(prepareArtwork), fail = useRef(onPreviewError);
  prepare.current = prepareArtwork; fail.current = onPreviewError;
  useEffect(() => {
    if (!settled) { setLayers([]); return; }
    let current = true;
    void prepare.current(settled).then(() => {
      if (current) setLayers(previous => previous.at(-1)?.id === settled.id ? [...previous.slice(0, -1), settled] : [...previous.slice(-1), settled]);
    }, () => { if (current) fail.current?.(); });
    return () => { current = false; };
  }, [settled]);
  return <div className={[styles.preview, className].filter(Boolean).join(' ')}>
    <Hero {...props} reducedMotion={motion.reduced} candidates={layers} activeIndex={layers.length - 1} showNavigation={false} renderArtwork={renderArtwork} active={false} platform="tv" />
  </div>;
}
