/**
 * A windowed list's row pitch after a measurement at the same width. It never shrinks, so a window
 * of mixed-height rows converges instead of moving the window, re-measuring and moving it back.
 */
export function settledPitch(previous: number | undefined, measured: number): number {
  return previous === undefined ? measured : Math.max(previous, measured);
}
