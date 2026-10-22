import type {QuerySelectionTarget, WholeSelectionSelector} from './types.ts';

export type SelectionQuerySelector =
  | Readonly<{ok: true; selector: WholeSelectionSelector; expected: QuerySelectionTarget['expected']}>
  | Readonly<{ok: false; reason: 'query-refinement-required'}>;

/**
 * Existing JobSelector has no kind/exclusion envelope. A dispatcher must retain
 * the full semantic target; dropping its refinement would mutate extra titles.
 * This guard performs no fetching, narrowing rewrite or membership enumeration.
 */
export function selectionQuerySelector(target: QuerySelectionTarget): SelectionQuerySelector {
  if (target.refinement.kind !== null || target.refinement.excludedItems.length > 0 || target.total !== target.sourceTotal) {
    return Object.freeze({ok: false, reason: 'query-refinement-required'});
  }
  return Object.freeze({ok: true, selector: target.selector, expected: target.expected});
}
