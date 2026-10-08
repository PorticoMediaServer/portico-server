import React, {useState} from 'react';
import {useSelection} from '../../app/selection';
import {useSession} from '../../app/session';
import {repairTargetFor, useMetadataEditor} from '../../app/metadata-editor';
import {Dialog, Icon, Menu, Spinner, useCompact} from '../../ui';
import {currentI18n} from '../../app/i18n';
import {CollectionPicker, PlaylistPicker} from './EntryActions';
import {runBulkSelection, type BulkProgress} from './bulk-job';
import type {JobArgs, JobCommand, JobFailure} from '@core/bulk-jobs.ts';
import s from './SelectionBar.module.css';

/**
 * Bulk actions for the current selection. Every action is one bulk job
 * (`POST /v1/jobs`) with progress and a failure summary — never one request
 * per item. PERF-S09: Select-all with nothing deselected sends the screen's
 * whole-set target (one query or container job, "Select all N"); a moved set
 * falls back to one job per 200 loaded items.
 */
export function SelectionBar() {
  const selection = useSelection();
  const {api, owner, session} = useSession();
  const editor = useMetadataEditor();
  const compact = useCompact();
  const [busy, setBusy] = useState<string>();
  const [progress, setProgress] = useState<BulkProgress>();
  const [result, setResult] = useState<string>();
  const [playlist, setPlaylist] = useState(false);
  const [collection, setCollection] = useState(false);
  if (!selection?.active) return null;
  const t = currentI18n().t;
  const count = selection.entries.length;
  const ids = () => [...new Set(selection.entries.map(e => e.playback?.itemId ?? e.id))];

  /**
   * One bulk action. PERF-S09: Select-all with nothing deselected sends the
   * screen's whole-set target (one query or container job fenced with the
   * catalog revision); a moved set falls back to the chunked items path, and
   * anything else submits the explicit items — never a library iteration.
   */
  const submit = async (label: string, command: JobCommand, args: JobArgs, onDone?: (ok: number, failed: readonly JobFailure[]) => void) => {
    const targets = ids();
    if (!targets.length) return;
    setBusy(label);
    setResult(undefined);
    setProgress(undefined);
    try {
      const outcome = await runBulkSelection(api, command, args, {visibleIds: selection.visible.map(e => e.id), selectedIds: selection.ids, target: selection.target, wholeSet: selection.wholeSet}, () => crypto.randomUUID(), setProgress);
      setProgress(undefined);
      setBusy(undefined);
      if (onDone) onDone(outcome.ok, outcome.failed);
      else if (outcome.failed.length) setResult(t('web.selection.partial', {ok: outcome.ok, failed: outcome.failed.length}));
      else if (command === 'trash') setResult(t('web.selection.trashed', {count: outcome.ok}));
      else if (command === 'refresh') setResult(t('web.selection.refreshed', {count: outcome.ok}));
      else setResult(t('web.selection.updated', {count: outcome.ok}));
      if (outcome.ok && typeof window !== 'undefined' && (command === 'personal-state')) {
        const patch = args as {watched?: boolean; favorite?: boolean; watchlist?: boolean};
        for (const [action, value] of Object.entries(patch)) {
          if (value !== undefined) window.dispatchEvent(new CustomEvent('portico:personal-change', {detail: {itemIds: targets, action: action === 'watchlisted' ? 'watchlist' : action, value}}));
        }
      }
      selection.refresh();
    } catch (e) {
      setProgress(undefined);
      setBusy(undefined);
      const code = (e as {code?: string} | null)?.code;
      // selection_changed fails before any mutation: the scope moved, so a
      // refresh and a new submission are the recovery, not a blind retry.
      setResult(code === 'selection_changed' ? t('web.selection.refreshChanged') : t('web.selection.jobFailed'));
    }
  };
  const personal = (label: string, patch: {watched?: boolean; favorite?: boolean; watchlisted?: boolean}) => void submit(label, 'personal-state', patch);
  const status = busy ? (progress && progress.totalKnown ? t('web.selection.working', {done: progress.done, total: progress.total}) : progress ? t('web.selection.preparing') : undefined) : undefined;
  const all = selection.visible;
  const allSelected = !!all.length && all.every(e => selection.isSelected(e.id));
  // The whole-set size when the screen named its set ("Select all 1,240"), else what's on screen.
  const selectAllCount = selection.target && selection.target.total > 0 ? selection.target.total : all.length;
  return (
    <div className={s.bar} role="region" aria-label={t('web.selection.region')}>
      <div className={s.count}>
        <span className={s.countNumber}>{selection.wholeSet && selection.target ? selection.target.total : count}</span>
        <span className={s.countCopy}>{result ?? status ?? t('web.selection.selected')}</span>
        {all.length && !allSelected ? <button type="button" className={s.link} onClick={() => (selection.target && selection.target.total > all.length ? selection.selectWholeSet(all) : selection.selectMany(all))}>{t('web.selection.selectAll', {count: selectAllCount})}</button> : null}
      </div>
      <span className={s.divider} aria-hidden />
      <div className={s.actions}>
        <Action icon="check" label={t('web.selection.watched')} busy={busy === 'watched'} disabled={!count || !!busy} onClick={() => personal('watched', {watched: true})} compact={compact} />
        <Action icon="eyeOff" label={t('web.selection.unwatched')} busy={busy === 'unwatched'} disabled={!count || !!busy} onClick={() => personal('unwatched', {watched: false})} compact={compact} />
        {/* WEB-LIB-08: Watchlist and Favorites can be removed in bulk too. */}
        <Menu label={t('web.selection.watchlist')} align="center" trigger={<Action icon="bookmark" label={t('web.selection.watchlist')} busy={busy === 'watchlist'} disabled={!count || !!busy} compact={compact} />} items={[{id: 'add', label: t('web.selection.watchlistAdd'), icon: 'bookmark'}, {id: 'remove', label: t('web.selection.watchlistRemove'), icon: 'close'}]} onSelect={id => personal('watchlist', {watchlisted: id === 'add'})} />
        <Menu label={t('web.selection.favorite')} align="center" trigger={<Action icon="heart" label={t('web.selection.favorite')} busy={busy === 'favorite'} disabled={!count || !!busy} compact={compact} />} items={[{id: 'add', label: t('web.selection.favoriteAdd'), icon: 'heart'}, {id: 'remove', label: t('web.selection.favoriteRemove'), icon: 'close'}]} onSelect={id => personal('favorite', {favorite: id === 'add'})} />
        <Action icon="queue" label={t('web.selection.playlist')} disabled={!count || !!busy} onClick={() => setPlaylist(true)} compact={compact} />
        {owner ? <Action icon="collection" label={t('web.selection.collection')} disabled={!count || !!busy} onClick={() => setCollection(true)} compact={compact} /> : null}
        {owner && session?.viewer.authority === 'local' ? <Action icon="edit" label={t('web.selection.edit')} disabled={!count || !!busy || !selection.entries.every(e => repairTargetFor(e))} onClick={() => editor?.open({targets: selection.entries.map(e => repairTargetFor(e)!), titles: selection.entries.map(e => e.title), onSaved: selection.refresh})} compact={compact} /> : null}
        {owner && session?.viewer.authority === 'local' ? <Action icon="refresh" label={t('web.selection.refresh')} busy={busy === 'refresh'} disabled={!count || !!busy} onClick={() => void submit('refresh', 'refresh', {})} compact={compact} /> : null}
        {/* Justin's decision: trash is single-step with no preview or confirmation. Empty trash keeps its confirmation. */}
        {owner ? <Action icon="trash" label={t('web.selection.delete')} busy={busy === 'delete'} disabled={!count || !!busy || !selection.entries.every(e => e.playback)} onClick={() => void submit('delete', 'trash', {})} compact={compact} /> : null}
      </div>
      <span className={s.divider} aria-hidden />
      <button type="button" className={s.done} onClick={selection.exit}>{t('web.selection.done')}</button>
      <Dialog open={playlist} onOpenChange={setPlaylist} title={t('web.selection.addToPlaylist', {count})} width={480}>
        <PlaylistPicker bulk itemIds={selection.entries.map(e => e.playback?.itemId ?? e.id)} itemTitles={selection.entries.map(e => e.title)} onBack={() => setPlaylist(false)} onDone={message => { setPlaylist(false); setResult(message); }} />
      </Dialog>
      <Dialog open={collection} onOpenChange={setCollection} title={t('web.selection.addToCollection', {count})} width={480}>
        <CollectionPicker bulk itemIds={selection.entries.map(e => e.playback?.itemId ?? e.id)} onBack={() => setCollection(false)} onDone={message => { setCollection(false); setResult(message); }} />
      </Dialog>
    </div>
  );
}

/** A bar button; forwards props and ref so it can also be a menu trigger. */
type ActionProps = {icon: 'check' | 'eyeOff' | 'bookmark' | 'heart' | 'queue' | 'collection' | 'refresh' | 'edit' | 'trash'; label: string; busy?: boolean; compact: boolean} & Omit<React.ComponentPropsWithoutRef<'button'>, 'children'>;
const Action = React.forwardRef<HTMLButtonElement, ActionProps>(function Action({icon, label, busy, compact, ...rest}, ref) {
  return (
    <button ref={ref} type="button" className={s.action} aria-label={label} aria-busy={busy || undefined} title={label} {...rest}>
      {busy ? <Spinner className={s.actionSpinner} /> : <Icon name={icon} size={18} />}
      {!compact ? <span>{label}</span> : null}
    </button>
  );
});
