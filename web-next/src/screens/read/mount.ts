import {readerErrorCode, type ReaderController, type ReaderErrorCode, type ReaderFormat, type ReaderLocation, type ReaderMetadata, type ReaderSource} from '@portico/reader-core';

export type ReaderMountAuthority = Readonly<{
  isCurrent: () => boolean;
  /** The existing viewer/origin lifetime supplies its real retirement event. */
  subscribe: (listener: () => void) => (() => void);
}>;
/** Public session methods only: the canonical Reading facade deliberately
 * omits the low-level preference updater and the controller's private fields. */
export type ReaderHostController<P extends object> = Omit<ReaderController<P>, 'updatePreferences'>;
export type ReaderSurfaceMountOptions<P extends object> = Readonly<{
  controller: ReaderHostController<P>; source: ReaderSource; format: ReaderFormat; initialLocation?: ReaderLocation;
  /** Actual manifest identity; an omitted author stays omitted in the engine and chrome. */
  metadata?: ReaderMetadata;
  /** Actual authorized publication session lifetime, separate from viewer scope. */
  signal?: AbortSignal;
  authority: ReaderMountAuthority; container: HTMLElement;
  reportError: (code: ReaderErrorCode) => void;
  /** Await disposal before reusing a DOM container (including StrictMode reconnect). */
  settled?: Promise<void>;
}>;
export type ReaderSurfaceMount = Readonly<{ready: Promise<void>; close: () => Promise<void>}>;

/** The core owns the document, preferences, location and view; this owns only DOM lifetime. */
export function mountReaderSurface<P extends object>({controller, source, format, initialLocation, metadata, signal, authority, container, reportError, settled = Promise.resolve()}: ReaderSurfaceMountOptions<P>): ReaderSurfaceMount {
  let active = true, opened = false, opening = false, frame: number | undefined, width = 0, height = 0;
  let closeResult: Promise<void> | undefined;
  const current = () => active && authority.isCurrent() && !signal?.aborted;
  let offAuthority = () => {}, offSession = () => {}, resolveSize: (() => void) | undefined;
  const waitForSize = new Promise<void>(resolve => { resolveSize = resolve; });
  const observer = new ResizeObserver(() => {
    if (container.clientWidth > 0 && container.clientHeight > 0) resolveSize?.();
    measure();
  });
  const close = (): Promise<void> => {
    if (closeResult) return closeResult;
    active = false; observer.disconnect(); offAuthority(); offSession(); resolveSize?.();
    signal?.removeEventListener('abort', aborted);
    if (frame !== undefined) cancelAnimationFrame(frame);
    // A superseded effect that never opened must not close a newer core session.
    const closing = opened || opening ? controller.close() : Promise.resolve();
    // An abandoned reconnect must retain its predecessor's disposal fence.
    closeResult = Promise.all([settled, closing]).then(() => {});
    return closeResult;
  };
  const aborted = () => {void close().catch(() => {});};
  const failed = (error: unknown) => { if (current()) reportError(readerErrorCode(error)); };
  const resize = () => {
    frame = undefined;
    if (!current() || controller.getSnapshot().status !== 'ready') return;
    const nextWidth = container.clientWidth, nextHeight = container.clientHeight;
    if (nextWidth <= 0 || nextHeight <= 0 || nextWidth === width && nextHeight === height) return;
    width = nextWidth; height = nextHeight;
    void controller.resize({width, height}).catch(error => {
      // A refused resize is not an applied viewport. Let a later observation of
      // the same size retry, without invalidating a newer resize's dimensions.
      if (current() && width === nextWidth && height === nextHeight) {width = 0; height = 0;}
      failed(error);
    });
  };
  const measure = () => {
    if (current() && opened && frame === undefined) frame = requestAnimationFrame(resize);
  };
  if (current()) {
    signal?.addEventListener('abort', aborted, {once: true});
    offAuthority = authority.subscribe(() => { if (!authority.isCurrent()) void close().catch(() => {}); });
  }
  if (!active) offAuthority();
  const ready = (async () => {
    await settled;
    if (!current()) return;
    observer.observe(container);
    if (container.clientWidth <= 0 || container.clientHeight <= 0) {
      await waitForSize;
    }
    if (!current()) return;
    width = container.clientWidth; height = container.clientHeight;
    opening = true;
    await controller.open({source, format, container, metadata, signal, location: initialLocation, viewport: {width, height}});
    opening = false;
    if (!current()) { await close(); return; }
    opened = true;
    // A replacement open retires this mount before its old parent can abort.
    // The controller already owns that disposal; never close the new session.
    offSession = controller.subscribe(() => {
      const snapshot = controller.getSnapshot();
      if (snapshot.status !== 'ready' || snapshot.sourceId !== source.id || snapshot.sourceRevision !== source.revision) {
        opened = false; void close().catch(() => {});
      }
    });
    measure();
  })().catch(failed);
  return Object.freeze({ready, close});
}
