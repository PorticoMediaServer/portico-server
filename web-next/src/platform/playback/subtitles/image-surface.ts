import {ImageSubtitleRenderer} from './image-renderer.ts';
import {Overlay, type OverlayEnvironment, type OverlayVideo} from './overlay.ts';
import type {DrawnSubtitleSource, ReadSubtitleFile} from './types.ts';

/**
 * One image subtitle over the player's own video: the reviewed legacy decoder and overlay,
 * mounted on a canvas this surface owns. Only the image formats this app can draw are admitted.
 */
export type ImageSubtitleSurface = Readonly<{
  present(source: DrawnSubtitleSource): Promise<void>;
  timeline(offsetSeconds: number | null): void;
  setDelay(seconds: number): void;
  setForcedOnly(on: boolean): void;
  /** The track really has some forced lines and some not, so the drawn switch changes something. */
  readonly forcedChoice: boolean;
  clear(): void;
  dispose(): void;
}>;

export function createImageSubtitleSurface(video: HTMLVideoElement, read: ReadSubtitleFile, failed: (reason: unknown) => void = () => {}): ImageSubtitleSurface | undefined {
  if (!Overlay.supported()) return undefined;
  const host = video.parentElement ?? video;
  if (!host) return undefined;
  const canvas = host.ownerDocument.createElement('canvas');
  canvas.setAttribute('data-image-subtitles', '');
  Object.assign(canvas.style, {position: 'absolute', inset: '0', width: '100%', height: '100%', display: 'none', pointerEvents: 'none', zIndex: '1'});
  host.append(canvas);
  const environment: OverlayEnvironment = {
    watchLayout(target, changed) {
      const stops: (() => void)[] = [], observer = typeof ResizeObserver === 'function' ? new ResizeObserver(changed) : undefined;
      observer?.observe(host);
      const events = ['loadedmetadata', 'resize', 'enterpictureinpicture', 'leavepictureinpicture'];
      for (const event of events) { target.addEventListener(event, changed); stops.push(() => target.removeEventListener(event, changed)); }
      stops.push(() => observer?.disconnect());
      return () => {for (const stop of stops) stop();};
    },
    fit: () => 'contain',
    density: () => Math.min(3, Math.max(1, window.devicePixelRatio || 1)),
    origin: element => { const rect = element.getBoundingClientRect(); return {left: rect.left, top: rect.top}; },
    frame: callback => window.requestAnimationFrame(callback),
    cancelFrame: handle => window.cancelAnimationFrame(handle),
  };
  const overlay = new Overlay(video as unknown as OverlayVideo, canvas, {environment,
    renderers: async kind => { if (kind !== 'pgs' && kind !== 'vobsub') throw new Error('unsupported_image_subtitle'); return new ImageSubtitleRenderer(); }});
  return Object.freeze({
    async present(source: DrawnSubtitleSource) {
      if (source.kind !== 'pgs' && source.kind !== 'vobsub') throw new Error('unsupported_image_subtitle');
      await overlay.present(source, read, failed);
    },
    timeline: (offset: number | null) => overlay.timeline(offset),
    setDelay: (seconds: number) => overlay.setDelay(seconds),
    setForcedOnly: (on: boolean) => overlay.setForcedOnly(on),
    get forcedChoice() { return overlay.forcedChoice; },
    clear: () => overlay.clear(),
    dispose: () => { overlay.dispose(); canvas.remove(); },
  });
}
