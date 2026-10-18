import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {PersonalSavedService, savedViewBrowse, type PersonalResource, type SavedDefinition} from '@core/personal-saved.ts';
import {viewerScope} from '@core/presentation/index.ts';
import {Button, Chip, Dialog, Input, Menu, Notice, Text, type MenuItem} from '../../ui';
import {errorText} from '../../app/errors';
import {useI18n} from '../../app/i18n';
import {useSession} from '../../app/session';

/**
 * A library's presets: the saved views this profile keeps for the library (and its pivot), read
 * from the server's scoped list so they follow the profile to every device. One list per library,
 * pivot and viewer: another profile, server or library reads its own, and a late answer for the
 * one before never shows. Listing reads no view's content.
 */
export function useLibraryPresets(libraryId: string, pivot: string | undefined, enabled: boolean) {
  const {api, session} = useSession();
  const scope = useMemo(() => viewerScope(session, api), [session, api]);
  const service = useMemo(() => (enabled && pivot ? new PersonalSavedService(api, scope, () => Promise.resolve(crypto.randomUUID())) : null), [api, scope, enabled, pivot]);
  useEffect(() => {
    if (!service || !pivot) return;
    void service.select({view: 'views', libraryId, pivot});
    return () => service.dispose();
  }, [service, libraryId, pivot]);
  const snapshot = useSyncExternalStore(service?.subscribe ?? noSubscribe, () => service?.getSnapshot() ?? null);
  /** Saves the current filters and sort as a new view: a refusal (a taken name, a conflict) is thrown, never taken for success. */
  const save = async (name: string, definition: SavedDefinition) => {
    if (!service) throw new Error('unavailable');
    if (service.getSnapshot().mutationError) service.dismiss();
    if (!(await service.mutate({action: 'create', kind: 'view', name, definition}))) throw failure(service);
  };
  const rename = async (row: PersonalResource, name: string) => { if (!service) return; service.dismiss(); if (!(await service.mutate({action: 'update', name}, false, row))) throw failure(service); };
  const remove = async (row: PersonalResource) => { if (!service) return; service.dismiss(); if (!(await service.mutate({action: 'delete'}, false, row))) throw failure(service); };
  return {service, snapshot, save, rename, remove};
}
const noSubscribe = () => () => {};
const failure = (service: PersonalSavedService) => { const e = service.getSnapshot().mutationError; return Object.assign(new Error(e?.message ?? 'failed'), {code: e?.code}); };

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
  if (rows.length) items.push({id: ':manage', label: t('lib.savedViewsManage'), icon: 'settings', separatorBefore: true});
  const choose = (id: string) => {
    if (id === ':manage') onManage();
    else if (id === ':retry') void presets.service?.refresh();
    else { const row = rows.find(r => r.id === id); if (row && savedViewBrowse(row)) onApply(row); }
  };
  return <Menu label={t('lib.savedViews')} align="start" trigger={<Chip label={compact ? '' : t('lib.savedViews')} icon="bookmark" aria-label={t('lib.savedViews')} />} items={items} onSelect={choose} />;
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
    try { await work(); setEditing(null); setConfirm(null); } catch (e) { setError(errorText(e, 'library', 'save')); } finally { setBusy(false); }
  };
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={t('lib.savedViewsManage')} description={t('lib.savedViewsManageLede')} width={480} actions={<Button variant="primary" label={t('lib.done')} onClick={onClose} />}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error">{error}</Notice> : null}
        {rows.map(row => {
          const ready = !!savedViewBrowse(row);
          if (editing?.id === row.id) return (
            <div key={row.id} style={{display: 'flex', gap: 8, alignItems: 'flex-end'}}>
              <div style={{flex: 1}}><Input label={t('lib.saveViewName')} value={editing.name} maxLength={200} autoFocus onChange={e => setEditing({id: row.id, name: e.target.value})} onKeyDown={e => { if (e.key === 'Enter' && editing.name.trim()) void run(() => presets.rename(row, editing.name.trim())); }} /></div>
              <Button variant="ghost" size="sm" label={t('action.cancel')} disabled={busy} onClick={() => setEditing(null)} />
              <Button variant="primary" size="sm" label={t('action.save')} loading={busy} disabled={!editing.name.trim()} onClick={() => void run(() => presets.rename(row, editing.name.trim()))} />
            </div>
          );
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
      </div>
    </Dialog>
  );
}
