import {homeViewResourceId, homeViewRowId, isPersonalRowId, type HomeLayoutView} from '@core/home.ts';
import type {PersonalResource} from '@core/personal-saved.ts';

/** A dialog draft keeps the exact published revision until an explicit reload. */
export type HomeLayoutDraft = Readonly<{
  view: HomeLayoutView;
  order: readonly string[];
  hidden: ReadonlySet<string>;
  added: ReadonlyMap<string, PersonalResource>;
}>;
export function homeLayoutDraft(view: HomeLayoutView): HomeLayoutDraft {
  return {view, order: view.rows.filter(row => !isPersonalRowId(row.id)).map(row => row.id), hidden: new Set(view.hiddenRowIds), added: new Map()};
}
export function homeDraftRow(draft: HomeLayoutDraft, id: string) {
  return draft.view.rows.find(row => row.id === id);
}
export function homeDraftCanMove(draft: HomeLayoutDraft, id: string): boolean {
  return draft.added.has(id) || homeDraftRow(draft, id)?.reorderable === true;
}
export function homeDraftCanHide(draft: HomeLayoutDraft, id: string): boolean {
  const row = homeDraftRow(draft, id);
  return draft.added.has(id) || !!row && row.hideable && !row.required;
}
/** Fixed rows keep their slots; only server-reorderable rows exchange places. */
export function moveHomeDraftRow(draft: HomeLayoutDraft, id: string, target: number): HomeLayoutDraft {
  if (!homeDraftCanMove(draft, id) || target < 0 || target >= draft.order.length) return draft;
  const targetId = draft.order[target];
  if (!homeDraftCanMove(draft, targetId)) return draft;
  const movable = draft.order.filter(value => homeDraftCanMove(draft, value)), from = movable.indexOf(id), to = movable.indexOf(targetId);
  if (from < 0 || from === to) return draft;
  movable.splice(to, 0, ...movable.splice(from, 1));
  let at = 0;
  return {...draft, order: draft.order.map(value => homeDraftCanMove(draft, value) ? movable[at++] : value)};
}
export function showHomeDraftRow(draft: HomeLayoutDraft, id: string, shown: boolean): HomeLayoutDraft {
  if (!homeDraftCanHide(draft, id)) return draft;
  const hidden = new Set(draft.hidden);
  if (shown) hidden.delete(id); else hidden.add(id);
  return {...draft, hidden};
}
export function addHomeDraftView(draft: HomeLayoutDraft, resource: PersonalResource): HomeLayoutDraft {
  if (resource.kind !== 'view' || resource.status !== 'ready' || !resource.definition) return draft;
  const id = homeViewRowId(resource.id);
  if (draft.order.includes(id)) return draft;
  return {...draft, order: [...draft.order, id], added: new Map(draft.added).set(id, resource)};
}
export function removeHomeDraftView(draft: HomeLayoutDraft, id: string): HomeLayoutDraft {
  if (!homeViewResourceId(id)) return draft;
  const hidden = new Set(draft.hidden), added = new Map(draft.added);
  hidden.delete(id);added.delete(id);
  return {...draft, order: draft.order.filter(value => value !== id), hidden, added};
}
export function homeDraftIntent(draft: HomeLayoutDraft) {
  return {expectedRevision: draft.view.revision,
    rowOrder: draft.order.filter(id => homeDraftCanMove(draft, id)),
    hiddenRowIds: [...draft.hidden].filter(id => {
      const row = homeDraftRow(draft, id);
      if (row && !homeDraftCanHide(draft, id)) return false;
      return !homeViewResourceId(id) || draft.order.includes(id);
    })};
}
