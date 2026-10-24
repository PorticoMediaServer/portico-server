import {defaultI18n} from '@i18n';
import type {BulkReceipt, BulkResult} from '@core/metadata-bulk';
import {validBulkListEntry} from '@core/metadata-bulk';
import type {RepairFieldSpec} from '@core/metadata-repair';
import type {DeletePreview} from '@core/administration';
import type {BulkEditorDraft, BulkFieldValues, BulkListDraft, BulkResultView, PickerResult, PickerFailure, SelectionCount, SelectionMediaKind} from './types';
import {selectionText, type SelectionMessage} from './copy';

const selectionCountKeys = {
  movie: 'next.selection.movie',
  show: 'next.selection.show',
  season: 'next.selection.season',
  episode: 'next.selection.episode',
  artist: 'next.selection.artist',
  album: 'next.selection.album',
  song: 'next.selection.song',
  book: 'next.selection.book',
  ebook: 'next.selection.book',
  comic: 'next.selection.titles',
  audiobook_file: 'next.selection.audiobookPart',
  chapter: 'next.selection.chapter',
  collection: 'next.selection.collection',
  category: 'next.selection.category',
  author: 'next.selection.author',
  book_series: 'next.selection.bookSeries',
  disc: 'next.selection.disc',
  extra: 'next.selection.extra',
  playlist: 'next.selection.playlistsCount',
} satisfies Readonly<Record<Exclude<SelectionMediaKind, 'channel' | 'person'>, SelectionMessage>>;

export function selectionCountText({count, kind}: SelectionCount): string {
  if (kind === 'channel') return defaultI18n.t('next.kit.channels', {count});
  if (kind === 'person') return defaultI18n.t('web.together.people', {count});
  return selectionText(kind ? selectionCountKeys[kind] : 'next.selection.titles', {count});
}

/** Order only the bulk fields in the actual preview registry. */
export function bulkEditorSchema(schema: readonly RepairFieldSpec[], kind: SelectionMediaKind): readonly RepairFieldSpec[] {
  const fields = schema.filter(spec => spec.bulk);
  const order = kind === 'movie' ? ['contentRating', 'studio', 'country', 'year', 'edition', 'metadataLanguage', 'genres', 'tags', 'labels'] : kind === 'episode' ? ['contentRating', 'network', 'year', 'metadataLanguage', 'tags', 'labels'] : kind === 'song' ? ['year', 'metadataLanguage', 'genres', 'tags', 'labels'] : [];
  return [...fields].sort((left, right) => {
    const a = order.indexOf(left.field), b = order.indexOf(right.field);
    return (a < 0 ? order.length : a) - (b < 0 ? order.length : b);
  });
}

export function retryLabel(failures: readonly {title: string}[]): string {
  return failures.length === 1 ? selectionText('next.selection.retryFor', {title: failures[0]!.title}) : selectionText('next.selection.retryCount', {count: failures.length});
}

export function missingPickerIds(failures: readonly PickerFailure[]): readonly string[] {
  return [...new Set(failures.map(value => value.id))];
}

/** List editing is add/remove only; a partially shared value is never converted into replacement. */
export function toggleListRemoval(current: BulkListDraft | undefined, value: string): BulkListDraft {
  const add = current?.add ?? [], remove = current?.remove ?? [];
  if (add.includes(value)) return {add: add.filter(item => item !== value), remove};
  return {add, remove: remove.includes(value) ? remove.filter(item => item !== value) : [...remove, value]};
}

export function addListValue(current: BulkListDraft | undefined, text: string): BulkListDraft | null {
  const value = text.trim();
  if (!validBulkListEntry(value)) return null;
  return {add: [...new Set([...(current?.add ?? []), value])], remove: (current?.remove ?? []).filter(item => item !== value)};
}

export function bulkDraftValid(schema: readonly RepairFieldSpec[], values: Readonly<Record<string, BulkFieldValues>>, draft: BulkEditorDraft): boolean {
  let changes = 0;
  const fields = new Map(schema.filter(spec => spec.bulk).map(spec => [spec.field, spec]));
  for (const [name, value] of Object.entries(draft.fields)) {
    const spec = fields.get(name);
    if (!spec || spec.type === 'list' || !values[name] || spec.maxLength !== undefined && [...value].length > spec.maxLength) return false;
    if (spec.type === 'enum' && value !== '' && !spec.allowed?.includes(value)) return false;
    if (spec.type === 'integer' || spec.type === 'number') {
      if (value !== '') {
        const number = Number(value);
        if (!Number.isFinite(number) || spec.type === 'integer' && !Number.isInteger(number) || spec.min !== undefined && number < spec.min || spec.max !== undefined && number > spec.max) return false;
      }
    }
    changes++;
  }
  for (const [name, list] of Object.entries(draft.lists)) {
    const spec = fields.get(name);
    if (!spec || spec.type !== 'list' || !values[name] || list.add.length > 64 || list.remove.length > 64 || ![...list.add, ...list.remove].every(validBulkListEntry)) return false;
    if (new Set(list.add).size !== list.add.length || new Set(list.remove).size !== list.remove.length || list.add.some(value => list.remove.includes(value))) return false;
    if (list.add.length || list.remove.length) changes++;
  }
  return changes > 0;
}

export type BulkSummaryLine = Readonly<{id: string; text: string; fieldLabel?: string; changedCount?: number; changedTitles?: readonly string[]}>;
/** Display names never enter a metadata write. The draft always keeps the actual language tag. */
export function bulkFieldDisplayValue(field: string, value: string): string {
  return field === 'metadataLanguage' && value ? defaultI18n.languageName(value) : value;
}
export function bulkSummary(schema: readonly RepairFieldSpec[], values: Readonly<Record<string, BulkFieldValues>>, draft: BulkEditorDraft, count: number): readonly BulkSummaryLine[] {
  const rows: BulkSummaryLine[] = [];
  for (const spec of schema.filter(field => field.bulk)) {
    if (Object.hasOwn(draft.fields, spec.field)) {
      const value = draft.fields[spec.field]!, original = values[spec.field];
      const same = spec.type === 'list' || !original ? undefined : original.values.find(entry => entry.value === value)?.count ?? (original.truncated || original.state === 'shared' && original.value === undefined ? undefined : 0);
      rows.push({id: spec.field, fieldLabel: spec.label, text: selectionText('next.selection.scalarChange', {field: spec.label, value: bulkFieldDisplayValue(spec.field, value) || selectionText('next.selection.notSet'), count}), changedCount: same === undefined ? undefined : count - same, changedTitles: original && !original.truncated && original.values.every(entry => entry.titles?.length === entry.count) ? original.values.filter(entry => entry.value !== value).flatMap(entry => entry.titles ?? []) : undefined});
    }
    const list = draft.lists[spec.field];
    if (list?.add.length) rows.push({id: `${spec.field}-add`, fieldLabel: spec.label, text: selectionText('next.selection.listAdd', {field: spec.label, values: defaultI18n.list(list.add), count})});
    if (list?.remove.length) rows.push({id: `${spec.field}-remove`, fieldLabel: spec.label, text: selectionText('next.selection.listRemove', {field: spec.label, values: defaultI18n.list(list.remove), count})});
  }
  return rows;
}

/** The supplied preview remains authoritative. Destructive selections are never chunked. */
export function deletePermission(owner: boolean, count: number, preview: DeletePreview | null, confirmation: string, busy = false): boolean {
  return owner && !busy && Number.isSafeInteger(count) && count > 0 && count <= 200 && preview !== null && preview.targets.length === count && new Set(preview.targets.map(target => target.itemId)).size === count && preview.confirmation.length > 0 && preview.blocked.length === 0 && preview.targets.every(target => target.allowMediaDeletion) && new Set(preview.targets.map(target => target.libraryId)).size === 1 && confirmation === preview.confirmation;
}

/** Receipts remain with the controller; only each target's latest real outcome is displayed. */
export function bulkReceiptView(receipts: readonly BulkReceipt[]): BulkResultView {
  const latest = new Map<string, BulkResult>();
  for (const receipt of receipts) for (const row of receipt.results) latest.set(JSON.stringify([row.kind, row.id]), row);
  const results = [...latest.values()];
  const updated = results.filter(row => row.ok).length;
  return {results, updated, failed: results.length - updated};
}

/** Complete observations may still contain failures; missing counters prove no success. */
export function pickerResultSucceeded(result: PickerResult): boolean {
  return result.complete && result.inserted !== undefined && result.failed === 0 && (result.unknown === undefined || result.unknown === 0) && result.pending !== true && result.failures.length === 0;
}
