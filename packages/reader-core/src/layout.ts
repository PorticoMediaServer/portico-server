import {ReaderError} from './errors.ts';
import type {ReaderFit, ReaderViewport} from './ports.ts';
export function validateViewport(viewport: ReaderViewport): void {
  if (!Number.isFinite(viewport.width) || !Number.isFinite(viewport.height) || viewport.width <= 0 || viewport.height <= 0)
    throw new ReaderError('invalid_preferences');
}
export function spreadFits(viewport: ReaderViewport): boolean {
  validateViewport(viewport);
  return viewport.width >= 1024 && viewport.width > viewport.height;
}
export type FixedLayoutInput = Readonly<{
  format: 'pdf' | 'cbz'; viewport: ReaderViewport; page: number; pageCount: number;
  twoPages: boolean; firstPageAlone: boolean; direction: 'ltr' | 'rtl' | 'vertical'; fit: ReaderFit | 'auto';
}>;
/** Logical order never reverses: RTL changes presentation and input direction,
 * so Next still means advancing through the publication's own reading order. */
export function planFixedLayout(input: FixedLayoutInput) {
  const {viewport, page, pageCount, direction, firstPageAlone} = input;
  validateViewport(viewport);
  if (!Number.isSafeInteger(pageCount) || pageCount < 1 || !Number.isSafeInteger(page) || page < 0 || page >= pageCount)
    throw new ReaderError('invalid_location');
  const spread = input.twoPages && direction !== 'vertical' && spreadFits(viewport);
  const start = !spread ? page : firstPageAlone ? page === 0 ? 0 : 1 + 2 * Math.floor((page - 1) / 2) : 2 * Math.floor(page / 2);
  const count = spread && !(firstPageAlone && start === 0) ? Math.min(2, pageCount - start) : 1;
  const pages = Object.freeze(Array.from({length: count}, (_, i) => start + i));
  const fit = input.fit === 'auto' ? viewport.width < 1024 ? 'width' : 'height' : input.fit;
  return Object.freeze({pages, visualPages: Object.freeze(direction === 'rtl' ? [...pages].reverse() : [...pages]),
    spread, fit, direction, nextPage: start + count < pageCount ? start + count : null,
    previousPage: start === 0 ? null : spread ? Math.max(0, start - (firstPageAlone && start === 1 ? 1 : 2)) : start - 1});
}
