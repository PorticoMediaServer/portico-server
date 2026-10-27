import {useEffect, useRef, useLayoutEffect} from 'react';
import {readerErrorCode} from '@portico/reader-core';
import type {ReaderSurfaceMountOptions} from './mount';
import {mountReaderSurface, type ReaderSurfaceMount} from './mount';
import styles from '../../kit/reader/surface.module.css';

export type ReaderSurfaceProps<P extends object> = Omit<ReaderSurfaceMountOptions<P>, 'container' | 'settled'>;

/** Real adapter content only. The surrounding Portico chrome consumes the same controller. */
export function ReaderSurface<P extends object>({controller, source, format, initialLocation, metadata, signal, authority, reportError, crease}: ReaderSurfaceProps<P>) {
  const container = useRef<HTMLDivElement>(null), settled = useRef(Promise.resolve()), mount = useRef<ReaderSurfaceMount | undefined>(undefined);
  // A crease change is applied to the existing mount below, never a remount.
  const admitted = useRef(crease);
  useLayoutEffect(() => {admitted.current = crease;});
  useEffect(() => {
    const element = container.current, crease = admitted.current; if (!element) return;
    const mounted = mountReaderSurface({controller, source, format, initialLocation, metadata, signal, authority, reportError, container: element, settled: settled.current, crease});
    mount.current = mounted;
    return () => { mount.current = undefined; settled.current = mounted.close().catch(error => { if (authority.isCurrent()) reportError(readerErrorCode(error)); }); };
  }, [controller, source, format, initialLocation, metadata, signal, authority, reportError]);
  useEffect(() => { mount.current?.setCrease(crease); }, [crease]);
  return <div ref={container} className={styles.surface} />;
}
