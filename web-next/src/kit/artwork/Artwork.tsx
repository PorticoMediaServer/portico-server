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
  const value: ArtworkRead = image ? {status: 'ready', image} : current;
  useEffect(() => value.image?.retain?.(), [value.image]);
  return value;
}

export function Artwork({source, alt, fit = 'cover', position, zoom = 1, saturation = 1, brightness = 1, visible = true, fallback, className, style, width, height, onStatusChange, onReady}: ArtworkProps) {
  const read = useArtwork(source, visible);
  const delivered = useRef<{image: typeof read.image; key: string} | undefined>(undefined);
  const [decoded, setDecoded] = useState<{image: typeof read.image; failed: boolean}>({image: undefined, failed: false});
  const failed = !!read.image && decoded.image === read.image && decoded.failed;
  const ready = !!read.image && decoded.image === read.image && !decoded.failed;
  const status = failed ? 'error' : read.image ? ready ? 'ready' : 'loading' : read.status;
  useEffect(() => { onStatusChange?.(status); }, [onStatusChange, status]);
  useEffect(() => { if (ready && source && (delivered.current?.image !== read.image || delivered.current?.key !== source.key)) { delivered.current = {image: read.image, key: source.key}; onReady?.(source); } }, [onReady, ready, source, read.image]);
  return <span className={artClass(styles.artwork, className)} style={artSize({width, height, style})} data-artwork-status={status} role={alt ? 'img' : undefined} aria-label={alt || undefined} aria-hidden={alt ? undefined : true}>
    {(!read.image || failed) && fallback}
    {read.image && !failed && <img key={source?.key} src={read.image.src} alt="" draggable={false} className={artClass(styles.image, ready && styles.decoded)}
      style={{objectFit: fit, objectPosition: position, transform: zoom === 1 ? undefined : `scale(${zoom})`, filter: saturation === 1 && brightness === 1 ? undefined : `saturate(${saturation}) brightness(${brightness})`}}
      onLoad={() => setDecoded({image: read.image, failed: false})} onError={() => setDecoded({image: read.image, failed: true})} />}
  </span>;
}
