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
  authority: ReaderMountAuthority; container: HTMLElement;
  reportError: (code: ReaderErrorCode) => void;
  /** Await disposal before reusing a DOM container (including StrictMode reconnect). */
  settled?: Promise<void>;
}>;
export type ReaderSurfaceMount = Readonly<{ready: Promise<void>; close: () => Promise<void>}>;

/** The core owns the document, preferences, location and view; this owns only DOM lifetime. */
export function mountReaderSurface<P extends object>({controller, source, format, initialLocation, metadata, authority, container, reportError, settled = Promise.resolve()}: ReaderSurfaceMountOptions<P>): ReaderSurfaceMount {
  let active = true, opened = false, opening = false, frame: number | undefined, width = 0, height = 0;
  let closeResult: Promise<void> | undefined;
  const current = () => active && authority.isCurrent();
  let offAuthority = () => {}, resolveSize: (() => void) | undefined;
  const waitForSize = new Promise<void>(resolve => { resolveSize = resolve; });
  const observer = new ResizeObserver(() => {
    if (container.clientWidth > 0 && container.clientHeight > 0) resolveSize?.();
    measure();
  });
  const close = (): Promise<void> => {
    if (closeResult) return closeResult;
    active = false; observer.disconnect(); offAuthority(); resolveSize?.();
    if (frame !== undefined) cancelAnimationFrame(frame);
    // A superseded effect that never opened must not close a newer core session.
    const closing = opened || opening ? controller.close() : Promise.resolve();
    // An abandoned reconnect must retain its predecessor's disposal fence.
    closeResult = Promise.all([settled, closing]).then(() => {});
    return closeResult;
  };
  const failed = (error: unknown) => { if (current()) reportError(readerErrorCode(error)); };
  const resize = () => {
    frame = undefined;
    if (!current() || controller.getSnapshot().status !== 'ready') return;
    const nextWidth = container.clientWidth, nextHeight = container.clientHeight;
    if (nextWidth <= 0 || nextHeight <= 0 || nextWidth === width && nextHeight === height) return;
    width = nextWidth; height = nextHeight;
    void controller.resize({width, height}).catch(failed);
  };
  const measure = () => {
    if (current() && opened && frame === undefined) frame = requestAnimationFrame(resize);
  };
  if (current()) offAuthority = authority.subscribe(() => { if (!authority.isCurrent()) void close().catch(() => {}); });
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
    await controller.open({source, format, container, metadata, location: initialLocation, viewport: {width, height}});
    opening = false;
    if (!current()) { await close(); return; }
    opened = true;
    measure();
  })().catch(failed);
  return Object.freeze({ready, close});
}
