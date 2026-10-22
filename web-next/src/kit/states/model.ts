/** Display-only progress. Unknown work never acquires an invented arc. */
export function knownProgress(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, Math.min(1, value)) : null;
}

export function loadingPhase(elapsedMs: number): 'waiting' | 'shapes' | 'slow' {
  const elapsed = Number.isFinite(elapsedMs) ? Math.max(0, elapsedMs) : 0;
  return elapsed < 300 ? 'waiting' : elapsed < 8000 ? 'shapes' : 'slow';
}

export type SkeletonWindow = Readonly<{
  width: number;
  height: number;
  scrollTop: number;
}>;
export type SkeletonCell = Readonly<{index: number; x: number; y: number; width: number; height: number}>;
export type SkeletonGridGeometry = Readonly<{
  columns: number;
  cellWidth: number;
  rowHeight: number;
  totalHeight: number;
  cells: readonly SkeletonCell[];
}>;

/** Only intersecting rows mount. Library size changes the spacer, never the render work. */
export function skeletonGridWindow(viewport: SkeletonWindow, itemCount: number, options: Readonly<{
  columns?: number; gutter?: number; columnGap?: number; rowGap?: number; ratio?: number; captions?: boolean;
}> = {}): SkeletonGridGeometry {
  const width = Number.isFinite(viewport.width) ? Math.max(0, viewport.width) : 0;
  const height = Number.isFinite(viewport.height) ? Math.max(0, viewport.height) : 0;
  const phone = width < 1024;
  const option = (value: number | undefined, fallback: number) => value !== undefined && Number.isFinite(value) ? Math.max(0,value) : fallback;
  const gutter = option(options.gutter, phone ? 16 : width < 1440 ? 32 + (width - 1024) / 13 : 64);
  const columnGap = option(options.columnGap, phone ? 10 : 20);
  const columns = Math.max(1, Math.floor(option(options.columns, phone ? Math.max(3, Math.floor((width - gutter * 2 + columnGap) / 120)) : Math.max(1, Math.floor((width - gutter * 2 + columnGap) / 190)))));
  const cellWidth = Math.max(0, (width - gutter * 2 - columnGap * (columns - 1)) / columns);
  const ratio = options.ratio && Number.isFinite(options.ratio) && options.ratio > 0 ? options.ratio : 2 / 3;
  const cellHeight = cellWidth / ratio;
  const rowGap = option(options.rowGap, phone ? 16 : 30);
  // Reserve the real card's two text lines, including their margins, as it arrives.
  const captionHeight = options.captions === false ? 0 : phone ? 41 : 50;
  const rowHeight = cellHeight + captionHeight + rowGap;
  const count = Number.isFinite(itemCount) ? Math.max(0, Math.floor(itemCount)) : 0;
  const rows = Math.ceil(count / columns);
  const totalHeight = rows === 0 ? 0 : rows * rowHeight - rowGap;
  const scrollTop = Number.isFinite(viewport.scrollTop) ? Math.max(0, viewport.scrollTop) : 0;
  const firstRow = Math.min(rows, Math.floor(scrollTop / Math.max(1, rowHeight)));
  const lastRow = Math.min(rows, Math.ceil((scrollTop + height) / Math.max(1, rowHeight)));
  const cells: SkeletonCell[] = [];
  if (width > 0 && height > 0) for (let row = firstRow; row < lastRow; row++) {
    for (let column = 0; column < columns; column++) {
      const index = row * columns + column;
      if (index >= count) break;
      cells.push({index, x: gutter + column * (cellWidth + columnGap), y: row * rowHeight, width: cellWidth, height: cellHeight});
    }
  }
  return {columns, cellWidth, rowHeight, totalHeight, cells};
}
