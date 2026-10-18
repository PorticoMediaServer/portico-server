import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {PersonalSavedService, UNCONFIRMED, savedChange, savedViewBrowse, type PersonalResource, type SavedDefinition} from '@core/personal-saved.ts';
import {viewerScope} from '@core/presentation/index.ts';
import {Button, Chip, Dialog, Input, Menu, Notice, Text, type MenuItem} from '../../ui';
import {errorText} from '../../app/errors';
import {useI18n} from '../../app/i18n';
import {useSession} from '../../app/session';
import {useFreshDocument} from '../../app/freshness';
import {titlesWithPersonalState} from '@core/freshness-hub.ts';

/** A saved view saved, renamed or deleted on another device moves this profile's personal state, as on Saved. */
const presetDependencies = Object.freeze([...titlesWithPersonalState()]);

/**
 * A library's presets: the saved views this profile keeps for the library (and its pivot), read
 * from the server's scoped list so they follow the profile to every device. One list per library,
 * pivot and viewer: another profile, server or library reads its own (a service of its own), and
 * a late answer for the one before never shows. Listing reads no view's content, a page at a time.
 * A change with no answer is unconfirmed (`unconfirmed`): it is offered again as it was sent, never
 * replaced by another, until it is confirmed or set aside (`discard`).
 */
export function useLibraryPresets(libraryId: string, pivot: string | undefined, enabled: boolean) {
  const {api, session} = useSession();
  const scope = useMemo(() => viewerScope(session, api), [session, api]);
  // One service per viewer, library and pivot: the effect below selects it and disposes of it.
  const service = useMemo(() => (enabled && pivot && libraryId ? new PersonalSavedService(api, scope, () => Promise.resolve(crypto.randomUUID())) : null), [api, scope, enabled, pivot, libraryId]);
  useEffect(() => {
    if (!service || !pivot) return;
    void service.select({view: 'views', libraryId, pivot});
    return () => service.dispose();
  }, [service, libraryId, pivot]);
  const snapshot = useSyncExternalStore(service?.subscribe ?? noSubscribe, () => service?.getSnapshot() ?? null);
  // Kept current while the library is in front (another device's save shows here), read quietly
  // in place, the pages already shown included; never over a change still on its way.
  useFreshDocument(presetDependencies, context => service?.revalidate(context) ?? Promise.resolve(), () => !!service?.getSnapshot().viewerFence);
  const unconfirmed = snapshot && service ? service.unconfirmed() : undefined;
  return {
    service, snapshot, unconfirmed,
    /** Saves the current filters and sort as a new view: a refusal (a taken name, a conflict) is thrown, never taken for success. */
    save: (name: string, definition: SavedDefinition) => (service ? savedChange(service, {action: 'create', kind: 'view', name, definition}) : Promise.reject(new Error(''))),
    rename: (row: PersonalResource, name: string) => (service ? savedChange(service, {action: 'update', name}, row) : Promise.resolve()),
    remove: (row: PersonalResource) => (service ? savedChange(service, {action: 'delete'}, row) : Promise.resolve()),
    /** Reads the list again quietly (the picker opened): what another device saved since shows. */
    refresh: () => { void service?.revalidate().catch(() => undefined); },
    /** Sets an unconfirmed change aside (it may still have been made; the list is read again). */
    discard: () => { if (!service) return; service.dismiss(); void service.refresh(); },
  };
}
const noSubscribe = () => () => {};
/** The words for a change that failed: an earlier unconfirmed one in the way, or the server's answer. */
export const changeError = (t: ReturnType<typeof useI18n>['t'], e: unknown) => ((e as {code?: string})?.code === UNCONFIRMED ? t('lib.savedViewUnconfirmed') : errorText(e, 'library', 'save'));

export type LibraryPresets = ReturnType<typeof useLibraryPresets>;

/** The toolbar's Saved views menu: apply one, or manage them. One that needs review is shown, explained, and never applied. */
export function PresetsMenu({presets, onApply, onManage, compact}: {presets: LibraryPresets; onApply: (row: PersonalResource) => void; onManage: () => void; compact?: boolean}) {
  const {t} = useI18n();
  const snap = presets.snapshot;
  const rows = snap?.resources ?? [];
  const items: MenuItem[] = snap?.loading && !rows.length ? [{id: ':loading', label: t('lib.savedViewsLoading'), disabled: true}]
    : snap?.error && !rows.length ? [{id: ':retry', label: t('lib.savedViewsFailed'), meta: t('action.tryAgain')}]
    : !rows.length ? [{id: ':empty', label: t('lib.savedViewsEmpty'), disabled: true}]
    : rows.map(r => (savedViewBrowse(r) ? {id: r.id, label: r.name, icon: 'bookmark'} : {id: r.id, label: r.name, meta: t('lib.savedViewReview'), disabled: true}));
  // A page at a time: the ones before and after, and a page that could not be read says so.
  if (rows.length && snap?.error) items.push({id: ':retry-page', label: t('lib.savedViewsFailed'), meta: t('action.tryAgain'), separatorBefore: true});
  if (snap?.history.length) items.push({id: ':previous', label: t('lib.savedViewsPrevious'), icon: 'chevronUp', separatorBefore: true});
  if (snap?.nextCursor) items.push({id: ':next', label: t('lib.savedViewsMore'), icon: 'chevronDown', separatorBefore: !snap?.history.length});
  if (rows.length) items.push({id: ':manage', label: t('lib.savedViewsManage'), icon: 'settings', separatorBefore: true});
  const choose = (id: string) => {
    if (id === ':manage') onManage();
    else if (id === ':retry') void presets.service?.refresh();
    else if (id === ':retry-page') void presets.service?.retryRead();
    else if (id === ':next') void presets.service?.next();
    else if (id === ':previous') void presets.service?.previous();
    else { const row = rows.find(r => r.id === id); if (row && savedViewBrowse(row)) onApply(row); }
  };
  // Opening it reads the list again quietly, so a view saved on another device is there.
  return <Menu label={t('lib.savedViews')} align="start" trigger={<Chip label={compact ? '' : t('lib.savedViews')} icon="bookmark" aria-label={t('lib.savedViews')} onPointerDown={presets.refresh} onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowDown') presets.refresh(); }} />} items={items} onSelect={choose} />;
}

/** Rename and delete a library's saved views from their rows; one that needs review says why. */
export function ManagePresetsDialog({open, onClose, presets}: {open: boolean; onClose: () => void; presets: LibraryPresets}) {
  const {t} = useI18n();
  const rows = presets.snapshot?.resources ?? [];
  const [editing, setEditing] = useState<{id: string; name: string} | null>(null);
  const [confirm, setConfirm] = useState<PersonalResource | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  useEffect(() => { if (!open) { setEditing(null); setConfirm(null); setError(undefined); } }, [open]);
  const run = async (work: () => Promise<void>) => {
    if (busy) return;
    setBusy(true); setError(undefined);
    try { await work(); setEditing(null); setConfirm(null); } catch (e) { setError(changeError(t, e)); } finally { setBusy(false); }
  };
  // A rename with no answer is offered again as it was sent (its name, not an edited one).
  const held = presets.unconfirmed?.intent.action === 'update' ? {id: presets.unconfirmed.resourceId, name: presets.unconfirmed.intent.name ?? ''} : undefined;
  const snap = presets.snapshot;
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={t('lib.savedViewsManage')} description={t('lib.savedViewsManageLede')} width={480} actions={<Button variant="primary" label={t('lib.done')} onClick={onClose} />}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error">{error}</Notice> : null}
        {rows.map(row => {
          const ready = !!savedViewBrowse(row);
          if (editing?.id === row.id || held?.id === row.id) {
            const name = held?.id === row.id ? held.name : editing!.name.trim();
            return (
              <div key={row.id} style={{display: 'flex', gap: 8, alignItems: 'flex-end'}}>
                <div style={{flex: 1}}><Input label={t('lib.saveViewName')} value={held?.id === row.id ? held.name : editing!.name} readOnly={held?.id === row.id} maxLength={200} autoFocus onChange={e => setEditing({id: row.id, name: e.target.value})} onKeyDown={e => { if (e.key === 'Enter' && name) void run(() => presets.rename(row, name)); }} /></div>
                <Button variant="ghost" size="sm" label={t('action.cancel')} disabled={busy} onClick={() => { if (held?.id === row.id) presets.discard(); setEditing(null); }} />
                <Button variant="primary" size="sm" label={t('action.save')} loading={busy} disabled={!name} onClick={() => void run(() => presets.rename(row, name))} />
              </div>
            );
          }
          if (confirm?.id === row.id) return (
            <div key={row.id} style={{display: 'flex', gap: 8, alignItems: 'center'}}>
              <Text style={{flex: 1}}>{t('lib.deleteSavedView', {name: row.name})}</Text>
              <Button variant="ghost" size="sm" label={t('action.cancel')} disabled={busy} onClick={() => setConfirm(null)} />
              <Button variant="danger" size="sm" label={t('lib.delete')} loading={busy} onClick={() => void run(() => presets.remove(row))} />
            </div>
          );
          return (
            <div key={row.id} style={{display: 'flex', gap: 8, alignItems: 'center'}}>
              <div style={{flex: 1, minWidth: 0}}>
                <Text>{row.name}</Text>
                {ready ? null : <Text variant="caption" tone="tertiary">{t('lib.savedViewReviewHelp')}</Text>}
              </div>
              {row.actions.includes('update') ? <Button variant="ghost" size="sm" label={t('lib.rename')} disabled={busy} onClick={() => setEditing({id: row.id, name: row.name})} /> : null}
              {row.actions.includes('delete') ? <Button variant="ghost" size="sm" label={t('lib.delete')} disabled={busy} onClick={() => setConfirm(row)} /> : null}
            </div>
          );
        })}
        {snap?.history.length || snap?.nextCursor ? (
          <div style={{display: 'flex', gap: 8, justifyContent: 'flex-end'}}>
            {snap.history.length ? <Button variant="ghost" size="sm" icon="chevronUp" label={t('lib.savedViewsPrevious')} disabled={busy || snap.loading} onClick={() => void presets.service?.previous()} /> : null}
            {snap.nextCursor ? <Button variant="ghost" size="sm" icon="chevronDown" label={t('lib.savedViewsMore')} disabled={busy || snap.loading} onClick={() => void presets.service?.next()} /> : null}
          </div>
        ) : null}
        {snap?.error ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: () => void presets.service?.retryRead()}}>{t('lib.savedViewsFailed')}</Notice> : null}
      </div>
    </Dialog>
  );
}
