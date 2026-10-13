/**
 * A subtitle choice as a change of the playing session (`PlaybackService.changePlayback`).
 *
 * `rendering` is the original this browser will draw for the track, if any. A styled or image
 * track is then asked for as `embeddedClient`: the server hands the original over and leaves the
 * video alone. An image track this browser says it draws, chosen without a rendering (it failed
 * to draw here), is asked for in the picture; left to itself the server would hand its original
 * over again. Everything else names no delivery and the server decides. A track carries its
 * timing correction with it, so selecting it never resets a correction saved for that file.
 */
import {isBitmapSubtitleFormat} from '@core/presentation/bitmap-subtitles.ts';
import {drawnRenderingFor, type DrawnSubtitleFormat} from '@core/subtitle-originals.ts';

export type SubtitleSessionChange = Readonly<{subtitleTrackId: string | null; subtitleDelivery?: 'embeddedClient' | 'burn'; subtitleOffsetMs?: number}>;

export function subtitleSessionChange(resource: Readonly<{id: string; format: string; offsetUs: string}> | null, rendering: DrawnSubtitleFormat | undefined, draws: readonly string[], offsetUs?: string): SubtitleSessionChange {
  if (!resource) return {subtitleTrackId: null};
  const delivery = rendering ? 'embeddedClient' as const : isBitmapSubtitleFormat(resource.format) && drawnRenderingFor(resource, draws) ? 'burn' as const : undefined;
  const offset = Number(offsetUs ?? resource.offsetUs);
  return {subtitleTrackId: resource.id, ...(delivery ? {subtitleDelivery: delivery} : {}), subtitleOffsetMs: Number.isFinite(offset) ? Math.round(offset / 1000) : 0};
}
