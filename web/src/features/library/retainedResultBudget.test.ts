import { describe, expect, it } from 'vitest';
import { RETAINED_RESULT_ITEM_BUDGET, retainedResultBudgetState } from './retainedResultBudget';

describe('retained library/search result budget', () => {
  it('marks the measured safe boundary without evicting result pages', () => {
    expect(retainedResultBudgetState(0)).toBe('within-budget');
    expect(retainedResultBudgetState(RETAINED_RESULT_ITEM_BUDGET)).toBe('within-budget');
    expect(retainedResultBudgetState(RETAINED_RESULT_ITEM_BUDGET + 1)).toBe('over-budget');
  });
});
