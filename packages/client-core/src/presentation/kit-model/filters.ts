import {part, type FactPart, type KitMessageId, type TextEnvironment} from './text.ts';

export type FilterOperator = 'equals' | 'not-equals' | 'in' | 'not-in' | 'is-present' | 'is-missing' | 'at-least' | 'greater-than' | 'at-most' | 'less-than' | 'between' | 'starts-with';
export type FilterValue = string | number | boolean | null | undefined;
export type AppliedFilter = Readonly<{field: string; operator: FilterOperator; value?: FilterValue | readonly FilterValue[]}>;
export type AppliedFilterModel = Readonly<{
  fieldId: string; field: FactPart; operator: FilterOperator; operatorPart: FactPart;
  values: readonly FactPart[]; negated: boolean; presence: 'present' | 'missing' | undefined;
}>;
const operators = {
  equals: 'next.kit.filter.equals', 'not-equals': 'next.kit.filter.not', in: 'next.kit.filter.in', 'not-in': 'next.kit.filter.notIn',
  'is-present': 'next.kit.filter.present', 'is-missing': 'next.kit.filter.missing', 'at-least': 'next.kit.filter.atLeast',
  'greater-than': 'next.kit.filter.greaterThan', 'at-most': 'next.kit.filter.atMost', 'less-than': 'next.kit.filter.lessThan',
  between: 'next.kit.filter.between', 'starts-with': 'next.kit.filter.startsWith',
} as const satisfies Record<FilterOperator, KitMessageId>;

/** Value labels come from the capability owner; arbitrary metadata punctuation is preserved. */
export function appliedFilterParts(predicate: AppliedFilter, fieldName: (id: string) => string, env: TextEnvironment, valueName: (value: string | number) => string = String): AppliedFilterModel {
  const {t} = env, fieldId = predicate.field === 'decade' ? 'year' : predicate.field;
  const presence = predicate.operator === 'is-present' ? 'present' : predicate.operator === 'is-missing' ? 'missing' : undefined;
  const raw = Array.isArray(predicate.value) ? predicate.value : [predicate.value];
  const values = presence ? [] : raw.filter((v): v is string | number | boolean => v !== null && v !== undefined && (typeof v !== 'number' || Number.isFinite(v))).map((v, i) => part(`value-${i}`, typeof v === 'boolean' ? t(v ? 'next.kit.yes' : 'next.kit.no') : predicate.field === 'decade' ? t('next.kit.filter.decade', {year: String(v)}) : valueName(v), 'value'));
  return Object.freeze({fieldId, field: part('field', fieldName(fieldId), 'context'), operator: predicate.operator,
    operatorPart: part('operator', t(operators[predicate.operator]), 'context'), values: Object.freeze(values),
    negated: predicate.operator === 'not-equals' || predicate.operator === 'not-in' || predicate.operator === 'is-missing', presence,
  });
}
