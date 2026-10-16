/**
 * Moving one entry of a list that keeps its own order (a personal collection, a library
 * collection): what the server is sent is the entry and the one it now follows ("" the front),
 * never the whole order. Pure and shared by every client; the server fences the move by the
 * list's revision.
 */
export type ManualMove = 'top' | 'up' | 'down' | 'bottom';

/** The moves that apply to the entry at `index` of `count` loaded entries. "To the bottom" needs
 * the whole list on hand (`whole`): otherwise the last loaded entry isn't the last one. */
export function manualMoves(index: number, count: number, whole: boolean): ManualMove[] {
  if (index < 0 || index >= count || count < 2) return [];
  const out: ManualMove[] = [];
  if (index > 0) out.push('top', 'up');
  if (index < count - 1) out.push('down');
  if (index < count - 1 && whole) out.push('bottom');
  return out;
}

/** The place `move` takes the entry at `from` to, among `count`. */
export function moveTarget(move: ManualMove, from: number, count: number): number {
  switch (move) {
    case 'top': return 0;
    case 'up': return Math.max(0, from - 1);
    case 'down': return Math.min(count - 1, from + 1);
    case 'bottom': return count - 1;
  }
}

/**
 * The request for moving the entry at `from` to place `to` (a drag's drop place, or a menu move):
 * the entry and the one it follows afterwards ("" for the front). Undefined when nothing moves.
 */
export function moveRequest(ids: readonly string[], from: number, to: number): Readonly<{entryId: string; afterEntryId: string}> | undefined {
  const entryId = ids[from];
  if (entryId === undefined || to < 0 || to >= ids.length || to === from) return undefined;
  const rest = ids.filter((_, i) => i !== from);
  return Object.freeze({entryId, afterEntryId: to === 0 ? '' : rest[to - 1]!});
}

/** The list as it reads once the move is made, for showing it at once while the server agrees. */
export function movedOrder<T>(items: readonly T[], from: number, to: number): T[] {
  if (from < 0 || from >= items.length || to < 0 || to >= items.length || from === to) return [...items];
  const out = items.filter((_, i) => i !== from);
  out.splice(to, 0, items[from]!);
  return out;
}
