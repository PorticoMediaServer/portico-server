import { type CSSProperties, type ImgHTMLAttributes, type ReactNode, useCallback, useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { artworkFailureCacheVersion, forgetArtworkFailure, hasArtworkFailure, hasArtworkSuccess, rememberArtworkFailure, rememberArtworkSuccess, subscribeArtworkFailureCache } from '../../data/artworkFailureCache';

type StableImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, 'src' | 'onError'> & {
  src?: string;
  fallback?: ReactNode;
  /** Authoritative content revision. A change permits one retry of the same URL. */
  retryKey?: string | number;
};

/** Dynamic product image that fails once into stable, geometry-preserving UI. */
export function StableImage({ src, fallback = null, retryKey, ...props }: StableImageProps) {
  const subscribeToSource = useCallback((listener: () => void) => subscribeArtworkFailureCache(src, listener), [src]);
  const sourceVersion = useCallback(() => artworkFailureCacheVersion(src), [src]);
  useSyncExternalStore(subscribeToSource, sourceVersion, sourceVersion);
  const [displayed, setDisplayed] = useState(() => hasArtworkSuccess(src) ? src : undefined);
  const [locallyFailed, setLocallyFailed] = useState<string>();
  const previousRequest = useRef({ src, retryKey });

  useEffect(() => {
    const previous = previousRequest.current;
    previousRequest.current = { src, retryKey };
    if (src && previous.src === src && previous.retryKey !== retryKey) {
      setLocallyFailed((failed) => failed === src ? undefined : failed);
      forgetArtworkFailure(src);
    }
  }, [retryKey, src]);

  useEffect(() => {
    if (!src) {
      setDisplayed(undefined);
    } else if (src !== displayed && hasArtworkSuccess(src)) {
      setDisplayed(src);
    }
  }, [displayed, src]);

  const visible = displayed && locallyFailed !== displayed && !hasArtworkFailure(displayed) ? displayed : undefined;
  const shouldStage = Boolean(src && src !== displayed && locallyFailed !== src && !hasArtworkFailure(src) && !hasArtworkSuccess(src));
  return <>
    {visible ? <img {...props} src={visible} onLoad={() => rememberArtworkSuccess(visible)} onError={() => {
      setLocallyFailed(visible);
      rememberArtworkFailure(visible);
    }} /> : fallback}
    {shouldStage ? <img
      {...props}
      aria-hidden="true"
      alt=""
      data-stable-image-stage=""
      src={src}
      style={{ ...props.style, position: 'absolute', inset: 0, visibility: 'hidden', pointerEvents: 'none' }}
      onLoad={(event) => {
        const candidate = event.currentTarget;
        void (async () => {
          try {
            if (candidate.decode) await candidate.decode();
          } catch {
            // The load event is authoritative when optional decode rejects.
          }
          rememberArtworkSuccess(src!);
          setDisplayed(src);
        })();
      }}
      onError={() => {
        setLocallyFailed(src);
        rememberArtworkFailure(src!);
      }}
    /> : null}
  </>;
}

function cssArtworkUrl(source: string): string {
  const escaped = source.replaceAll('\\', '\\\\').replaceAll('"', '\\"').replace(/[\n\r\f]/g, '');
  return `url("${escaped}")`;
}

/** Keeps the last decoded backdrop until a complete replacement is ready. */
export function useStableBackdrop(source: string | undefined, enabled = true, retryKey?: string | number): CSSProperties['backgroundImage'] {
  const subscribeToSource = useCallback((listener: () => void) => subscribeArtworkFailureCache(source, listener), [source]);
  const sourceVersion = useCallback(() => artworkFailureCacheVersion(source), [source]);
  useSyncExternalStore(subscribeToSource, sourceVersion, sourceVersion);
  const [displayed, setDisplayed] = useState<string | undefined>(() => hasArtworkSuccess(source) ? source : undefined);
  const [locallyFailed, setLocallyFailed] = useState<string>();
  const previousRequest = useRef({ source, retryKey });

  useEffect(() => {
    const previous = previousRequest.current;
    previousRequest.current = { source, retryKey };
    if (source && previous.source === source && previous.retryKey !== retryKey) {
      setLocallyFailed((failed) => failed === source ? undefined : failed);
      forgetArtworkFailure(source);
    }
  }, [retryKey, source]);

  useEffect(() => {
    if (!enabled || !source || source === displayed || locallyFailed === source || hasArtworkFailure(source)) return;
    if (hasArtworkSuccess(source)) {
      setDisplayed(source);
      return;
    }
    let active = true;
    const candidate = new Image();
    const publish = async () => {
      try {
        if (candidate.decode) await candidate.decode();
      } catch {
        // A loaded image remains usable when a browser's optional decode promise rejects.
      }
      if (active) {
        rememberArtworkSuccess(source);
        setDisplayed(source);
      }
    };
    candidate.onload = () => { void publish(); };
    candidate.onerror = () => {
      if (active) setLocallyFailed(source);
      rememberArtworkFailure(source);
    };
    candidate.src = source;
    return () => {
      active = false;
      candidate.onload = null;
      candidate.onerror = null;
    };
  }, [displayed, enabled, locallyFailed, source]);

  if (!enabled) return 'none';
  return displayed ? cssArtworkUrl(displayed) : 'none';
}
