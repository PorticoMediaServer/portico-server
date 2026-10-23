import {useEffect, useRef} from 'react';
import {readerErrorCode} from '@portico/reader-core';
import type {ReaderSurfaceMountOptions} from './mount';
import {mountReaderSurface} from './mount';
import styles from '../../kit/reader/surface.module.css';

export type ReaderSurfaceProps<P extends object> = Omit<ReaderSurfaceMountOptions<P>, 'container' | 'settled'>;

/** Real adapter content only. The surrounding Portico chrome consumes the same controller. */
export function ReaderSurface<P extends object>({controller, source, format, initialLocation, metadata, signal, authority, reportError}: ReaderSurfaceProps<P>) {
  const container = useRef<HTMLDivElement>(null), settled = useRef(Promise.resolve());
  useEffect(() => {
    const element = container.current; if (!element) return;
    const mounted = mountReaderSurface({controller, source, format, initialLocation, metadata, signal, authority, reportError, container: element, settled: settled.current});
    return () => { settled.current = mounted.close().catch(error => { if (authority.isCurrent()) reportError(readerErrorCode(error)); }); };
  }, [controller, source, format, initialLocation, metadata, signal, authority, reportError]);
  return <div ref={container} className={styles.surface} />;
}
