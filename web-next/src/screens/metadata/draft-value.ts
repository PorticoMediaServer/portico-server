import type {RepairData} from '@core/metadata-repair.ts';

/** Lock-only edits must retain a list's values, just as the visible Tags field does. */
export function metadataDraftValue(data: RepairData | null | undefined, field: string): string {
  const original = data?.snapshot.fields[field];
  return data?.schema.find(spec => spec.field === field)?.type === 'list'
    ? (original?.values ?? []).join(', ')
    : original?.value ?? '';
}
