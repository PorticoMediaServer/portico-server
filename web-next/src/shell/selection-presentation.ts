import type {BulkEdit, BulkReceipt} from '@core/metadata-bulk.ts';
import type {BulkMetadataPreview, HomeSelectionMember} from '@core/home-selection/index.ts';
import type {BulkEditorDraft, BulkFieldValues, BulkResultView} from '../kit/selection';
import {bulkReceiptView} from '../kit/selection/presentation';

/** A display projection of actual receipts. It is deliberately not a transport receipt. */
export function editorResultView(receipts: readonly BulkReceipt[]): BulkResultView | undefined {
  return receipts.length ? bulkReceiptView(receipts) : undefined;
}
export function editorValues(preview: BulkMetadataPreview): Readonly<Record<string, BulkFieldValues>> {
  return preview.fields;
}
/** Dialog labels come only from the controller's bounded, fenced source members. */
export function selectionMemberDisplay(member: HomeSelectionMember) {
  if (member.entry) return {id: member.entry.id, title: member.entry.title, posterUrl: member.entry.posterUrl};
  if (member.collection) return {id: member.collection.collection.id, title: member.collection.collection.name, posterUrl: member.collection.members[0]?.posterUrl};
  if (member.channel) return {id: JSON.stringify([member.channel.channel.sourceId, member.channel.channel.id]), title: member.channel.channel.name, posterUrl: member.channel.channel.logoPath};
  throw new Error('home-selection-display-required');
}
export function editorUiDraft(draft: Pick<BulkEdit, 'fields' | 'lists' | 'genres' | 'lockEdited'>): BulkEditorDraft {
  const list = (value: BulkEdit['genres']) => ({add: value?.add ?? [], remove: value?.remove ?? []});
  return {fields: Object.fromEntries(Object.entries(draft.fields ?? {}).flatMap(([key, edit]) => edit.value !== undefined ? [[key, edit.value]] : [])),
    lists: {...Object.fromEntries(Object.entries(draft.lists ?? {}).map(([key, edit]) => [key, list(edit)])), ...(draft.genres ? {genres: list(draft.genres)} : {})}, lockEdited: draft.lockEdited ?? true};
}
export function editorCoreDraft(draft: BulkEditorDraft): Pick<BulkEdit, 'fields' | 'lists' | 'genres' | 'lockEdited'> {
  const {genres, ...lists} = draft.lists;
  return {fields: Object.fromEntries(Object.entries(draft.fields).map(([key, value]) => [key, {value}])), lists,
    ...(genres ? {genres} : {}), lockEdited: draft.lockEdited};
}
