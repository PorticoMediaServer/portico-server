/**
 * Where a compressed windowed list is, given where its box has been scrolled to.
 *
 * Browsers cap an element's height, so a list taller than its box maps the box's scroll range
 * (`0…maxScrolled`) onto the list's own range (`0…maxVirtual`). Two things must both hold:
 *
 *  - a wheel turn, an arrow key or a small drag moves the list by what it moved the box (1:1),
 *    from wherever the list is now, so reading down a list never skips rows;
 *  - the ends line up: the box's top shows the list's first row and its bottom the last row.
 *
 * Moving 1:1 lets the list drift from the proportional position (`scrolled × ratio`). The drift is
 * allowed only as far as the scroll left in each direction can take back at no more than twice the
 * proportional speed, so the list closes on its end smoothly as the box reaches it. It never
 * snaps the way it did when the drift was only corrected at the very end. A large move (a
 * scrollbar drag, Home or End, a jump) goes to the proportional position.
 */
export function compressedPosition({from, scrolled, previousScrolled, maxScrolled, maxVirtual, jump}: {
  /** The list's position before this move (in its own pixels, 0…maxVirtual). */
  from: number;
  /** Where the box is scrolled to now; can run past either end (overscroll, sticky headers). */
  scrolled: number;
  /** Where the box was scrolled to before this move. */
  previousScrolled: number;
  maxScrolled: number;
  maxVirtual: number;
  /** A move at least this large is a jump, not a step. */
  jump: number;
}): {position: number; top: number} {
  const max = Math.max(1, maxScrolled);
  const ratio = Math.max(1, maxVirtual) / max;
  const within = Math.min(max, Math.max(0, scrolled));
  const proportional = within * ratio;
  const delta = scrolled - previousScrolled;
  let top: number;
  if (Math.abs(delta) >= jump) top = proportional;
  else {
    // Bounded on each side by what the scroll left that way can absorb.
    const drift = Math.min(within * ratio, Math.max(-(max - within) * ratio, from + delta - proportional));
    top = proportional + drift;
  }
  top = Math.min(maxVirtual, Math.max(0, top));
  // Past either end the box itself moves (the list's top coming into view, or the end of the page).
  return {position: top + (scrolled - within), top};
}
