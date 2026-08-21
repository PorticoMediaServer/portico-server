/**
 * Observability budget for library/search result surfaces. This is not an
 * eviction limit: dropping older cursor pages would currently break stable
 * selection, alphabet seek targets, and browser focus/scroll restoration.
 * A future virtualized window can use this measured boundary without changing
 * the product contract first.
 */
export const RETAINED_RESULT_ITEM_BUDGET = 240;

export type RetainedResultBudgetState = 'within-budget' | 'over-budget';

export function retainedResultBudgetState(itemCount: number): RetainedResultBudgetState {
  return itemCount <= RETAINED_RESULT_ITEM_BUDGET ? 'within-budget' : 'over-budget';
}
