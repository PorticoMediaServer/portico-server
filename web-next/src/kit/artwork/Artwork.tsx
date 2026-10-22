import {useEffect, useRef, useState, type CSSProperties} from 'react';
import {observeArtwork, type ArtworkRead} from './source';
import type {ArtworkProps, ArtworkSource} from './types';
import styles from './artwork.module.css';

export const artClass = (...names: (string | undefined | false)[]) => names.filter(Boolean).join(' ');
export function artSize({width, height, style}: {width?: number | string; height?: number | string; style?: CSSProperties}): CSSProperties {
  return {...(width === undefined ? {} : {width}), ...(height === undefined ? {} : {height}), ...style};
}

function useArtwork(source: ArtworkSource | null | undefined, visible: boolean): ArtworkRead {
  const scheduler = source?.scheduler, key = source?.key, retain = source?.retain;
  const order = source?.request?.order, priority = source?.request?.priority, group = source?.request?.group, large = source?.request?.large;
  const [read, setRead] = useState<ArtworkRead & {scheduler?: typeof scheduler; key?: string}>({status: 'empty'});
  useEffect(() => {
    if (!scheduler || !key || !visible) return;
    return observeArtwork({scheduler, key, retain, request: {order, priority, group, large}}, value => setRead({...value, scheduler, key}));
  }, [scheduler, key, retain, visible, order, priority, group, large]);
  const image = visible && scheduler && key ? scheduler.peek(key) : undefined;
  const current = visible && scheduler && key ? read.scheduler === scheduler && read.key === key ? read : {status: 'loading' as const} : {status: 'empty' as const};
  const value: ArtworkRead = image ? {status: 'ready', image, cached: read.scheduler === scheduler && read.key === key ? read.cached === true : true} : current;
  useEffect(() => value.image?.retain?.(), [value.image]);
  return value;
}

export function Artwork({source, alt, fit = 'cover', position, zoom = 1, saturation = 1, brightness = 1, visible = true, arrivalDurationMs, fallback, className, style, width, height, onStatusChange, onReady, onError}: ArtworkProps) {
  const read = useArtwork(source, visible);
  const delivered = useRef<{image: typeof read.image; key: string; scheduler: ArtworkSource['scheduler']} | undefined>(undefined);
  const reportedError = useRef<{key: string; scheduler: ArtworkSource['scheduler']} | undefined>(undefined);
  const [decoded, setDecoded] = useState<{image: typeof read.image; scheduler?: ArtworkSource['scheduler']; key?: string; failed: boolean}>({image: undefined, failed: false});
  const currentDecode = !!read.image && decoded.image === read.image && decoded.scheduler === source?.scheduler && decoded.key === source?.key;
  const failed = currentDecode && decoded.failed;
  const ready = currentDecode && !decoded.failed;
  const status = failed ? 'error' : read.image ? ready ? 'ready' : 'loading' : read.status;
  const scheduler = source?.scheduler, key = source?.key;
  useEffect(() => {
    delivered.current = undefined; reportedError.current = undefined;
    if (!visible || !scheduler || !key) setDecoded({image: undefined, failed: false});
  }, [visible, scheduler, key]);
  useEffect(() => { onStatusChange?.(status); }, [onStatusChange, status]);
  useEffect(() => {
    if (ready && source) {
      reportedError.current = undefined;
      if (delivered.current?.image !== read.image || delivered.current?.key !== source.key || delivered.current?.scheduler !== source.scheduler) {
        delivered.current = {image: read.image, key: source.key, scheduler: source.scheduler}; onReady?.(source);
      }
    }
    if (status === 'error' && source && (reportedError.current?.key !== source.key || reportedError.current?.scheduler !== source.scheduler)) {
      reportedError.current = {key: source.key, scheduler: source.scheduler}; onError?.(source);
    }
  }, [onReady, onError, ready, status, source, read.image]);
  const arrivalStyle = typeof arrivalDurationMs === 'number' && Number.isFinite(arrivalDurationMs) && arrivalDurationMs >= 0 ? {'--art-arrival-duration': `${arrivalDurationMs}ms`} as CSSProperties : undefined;
  return <span className={artClass(styles.artwork, className)} style={artSize({width, height, style: {...arrivalStyle, ...style}})} data-artwork-status={status} data-artwork-cached={read.cached || undefined} role={alt ? 'img' : undefined} aria-label={alt || undefined} aria-hidden={alt ? undefined : true}>
    {fallback != null && <span className={styles.artworkFallback}>{fallback}</span>}
    {read.image && !failed && <img key={source?.key} src={read.image.src} alt="" draggable={false} className={artClass(styles.image, ready && styles.decoded)}
      style={{objectFit: fit, objectPosition: position, transform: zoom === 1 ? undefined : `scale(${zoom})`, filter: saturation === 1 && brightness === 1 ? undefined : `saturate(${saturation}) brightness(${brightness})`}}
      onLoad={() => setDecoded({image: read.image, scheduler: source?.scheduler, key: source?.key, failed: false})} onError={() => setDecoded({image: read.image, scheduler: source?.scheduler, key: source?.key, failed: true})} />}
  </span>;
}
