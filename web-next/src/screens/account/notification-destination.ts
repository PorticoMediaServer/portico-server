import type {NotificationAction} from '@core/notifications.ts';

export type NoticeDestination = Readonly<{view: 'home'}> | Readonly<{view: 'settings'}> | Readonly<{view: 'dashboard'}>
  | Readonly<{view: 'library'; id: string}> | Readonly<{view: 'item'; id: string}>;
/** Explicit route targets only. A command is never silently aliased to a page. */
export function notificationDestination(action: NotificationAction, libraries: readonly {id: string}[], canManageServer: boolean): NoticeDestination | undefined {
  if (action.kind !== 'navigate' || !action.target) return;
  const {view, entityId, libraryId} = action.target;
  if (view === 'home' || view === 'settings') return {view};
  if (view === 'server' && canManageServer) return {view: 'dashboard'};
  if (view === 'library' && libraryId && libraries.some(library => library.id === libraryId)) return {view: 'library', id: libraryId};
  if (view === 'item' && entityId) return {view: 'item', id: entityId};
}
