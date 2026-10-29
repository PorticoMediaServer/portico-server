import type {BrowseState} from './controller';

/** A Reading library whose image is still being built (P2-59). Its pages carry no letter anchors
 * and no exact total, and the page counts nothing while it fills in (states.html frame 15: never
 * scanning progress, "nothing counting"). The A–Z rail already needs anchors to draw. */
export function browseImporting(state: Pick<BrowseState, 'capabilities' | 'importing'>): boolean {
  return state.capabilities?.importing === true || state.importing === true;
}
/** The toolbar's count: none while importing, a lower bound as "at least", otherwise exact. */
export function browseResultCount(snapshot: Readonly<{total?: number; totalIsLowerBound?: boolean}>, importing: boolean,
  text: (key: 'lib.count' | 'lib.countAtLeast', count: number) => string): string | undefined {
  if (importing || snapshot.total === undefined) return undefined;
  return text(snapshot.totalIsLowerBound ? 'lib.countAtLeast' : 'lib.count', snapshot.total);
}
