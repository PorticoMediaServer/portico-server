import React, {useEffect, useState} from 'react';
import type {HomeLayoutView} from '@core/home.ts';
import {Button, Dialog, Icon, Loading, Notice, Switch, Text, cx} from '../../ui';
import s from './Customise.module.css';
import {errorText} from '../../app/errors';
import {homeRowTitle} from '../../app/home';
import {homeViewResourceId, homeViewRowId, isPersonalRowId} from '@core/home.ts';
import {PersonalSavedService, type PersonalSavedSnapshot} from '@core/personal-saved.ts';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {useService} from '../../app/content';
import {requestId} from '../../app/detail';
import {useLibrariesContext} from '../../app/libraries';
import {useI18n} from '../../app/i18n';

/**
 * Row order and visibility. The rows come from GET /v1/home/layout (every row
 * this viewer could have, in the viewer's order, whether or not it has entries
 * today); the server says which rows may move or hide (required rows never
 * hide); this dialog only arranges the rest and saves one fenced layout.
 */

/** Lists for the dialog: the viewer's order, and the ids the view hides. Empty rows stay listed. */
export function customiseLists(view: HomeLayoutView): {order: string[]; hidden: string[]} {
  const rows = view.rows.filter(r => !isPersonalRowId(r.id));
  return {order: rows.map(r => r.id), hidden: rows.filter(r => r.hidden).map(r => r.id)};
}

/** Save payload: reorderable rows in the dialog order, with the view's revision as the fence. */
export function customiseSavePayload(view: HomeLayoutView, order: readonly string[], hidden: readonly string[]): {rowOrder: string[]; hiddenRowIds: string[]; expectedRevision: number} {
  return {rowOrder: order.filter(id => view.rows.find(r => r.id === id)?.reorderable !== false), hiddenRowIds: hidden.filter(id => order.includes(id) || !homeViewResourceId(id)), expectedRevision: view.revision};
}

/** P6: a saved view added to Home goes last; one already there isn't added twice. */
export function addViewRow(order: readonly string[], resourceId: string): string[] {
  const id = homeViewRowId(resourceId);
  return order.includes(id) ? [...order] : [...order, id];
}

/** Removing a saved view from Home drops its id from the order (and from the hidden set). */
export function removeViewRow(order: readonly string[], hidden: ReadonlySet<string>, id: string): {order: string[]; hidden: Set<string>} {
  const next = new Set(hidden);
  next.delete(id);
  return {order: order.filter(x => x !== id), hidden: next};
}

export function CustomiseHome({open, onClose, onSave, onReset, loadLayout}: {open: boolean; onClose: () => void; onSave: (order: readonly string[], hidden: readonly string[], expectedRevision?: number) => Promise<unknown>; onReset: () => Promise<unknown>; loadLayout: (signal?: AbortSignal) => Promise<HomeLayoutView>}) {
  const i18n = useI18n();
  const libraries = useLibrariesContext();
  const libraryName = (id: string) => libraries.items.find(l => l.id === id)?.name;
  const [order, setOrder] = useState<string[]>([]);
  const [hidden, setHidden] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [view, setView] = useState<HomeLayoutView | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<unknown>(undefined);
  const [attempt, setAttempt] = useState(0);
  // P6: saved views added in this dialog, by row id, for their names until the layout lists them.
  const [addedNames, setAddedNames] = useState<ReadonlyMap<string, string>>(new Map());
  const [picking, setPicking] = useState(false);
  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    let active = true;
    setLoading(true);
    setLoadError(undefined);
    setView(undefined);
    setError(undefined);
    loadLayout(controller.signal).then(v => {
      if (!active || controller.signal.aborted) return;
      setView(v);
      const lists = customiseLists(v);
      setOrder(lists.order);
      setHidden(new Set(lists.hidden));
      setLoading(false);
    }).catch(e => {
      if (!active || controller.signal.aborted) return;
      setLoadError(e);
      setLoading(false);
    });
    return () => { active = false; controller.abort(); };
  }, [open, loadLayout, attempt]);
  const rowOf = (id: string) => view?.rows.find(r => r.id === id);
  const addView = (resourceId: string, name: string) => {
    setOrder(prev => addViewRow(prev, resourceId));
    setAddedNames(prev => new Map(prev).set(homeViewRowId(resourceId), name));
    setPicking(false);
  };
  const removeView = (id: string) => {
    const next = removeViewRow(order, hidden, id);
    setOrder(next.order);
    setHidden(next.hidden);
  };
  const move = (id: string, delta: number) => setOrder(prev => { const i = prev.indexOf(id); const j = i + delta; if (i < 0 || j < 0 || j >= prev.length) return prev; const next = [...prev]; [next[i], next[j]] = [next[j], next[i]]; return next; });
  // Drag to reorder: the dragged row takes the place of the row it is over as it passes that
  // row's middle, so the list shows the result while the pointer is still down. The arrows stay
  // for the keyboard.
  const [dragging, setDragging] = useState<string>();
  const dragOver = (event: React.DragEvent<HTMLLIElement>, id: string) => {
    if (!dragging) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
    if (dragging === id || rowOf(id)?.reorderable === false) return;
    const box = event.currentTarget.getBoundingClientRect();
    const after = event.clientY > box.top + box.height / 2;
    setOrder(prev => {
      const from = prev.indexOf(dragging);
      const over = prev.indexOf(id);
      if (from < 0 || over < 0) return prev;
      // Only when the pointer has crossed the middle towards the dragged row's far side.
      if (from < over ? !after : after) return prev;
      const next = prev.filter(x => x !== dragging);
      next.splice(over, 0, dragging);
      return next;
    });
  };
  const save = async () => {
    if (!view) return;
    setBusy(true); setError(undefined);
    try {
      const payload = customiseSavePayload(view, order, [...hidden]);
      await onSave(payload.rowOrder, payload.hiddenRowIds, payload.expectedRevision);
      onClose();
    } catch (e) { setError(errorText(e, 'home', 'save')); } finally { setBusy(false); }
  };
  const reset = async () => { setBusy(true); try { await onReset(); onClose(); } catch (e) { setError(errorText(e, 'home', 'save')); } finally { setBusy(false); } };
  const retry = () => setAttempt(a => a + 1);
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={i18n.t('home.customize.action')} description={i18n.t('home.customize.subtitle')} width={520} actions={<><Button variant="ghost" label={i18n.t('action.resetToDefault')} onClick={() => void reset()} disabled={busy || loading || !view} /><span style={{flex: 1}} /><Button variant="ghost" label={i18n.t('action.cancel')} onClick={onClose} disabled={busy} /><Button variant="primary" label={i18n.t('action.save')} loading={busy} disabled={loading || !view} onClick={() => void save()} /></>} spread>
      {error ? <Notice tone="error">{error}</Notice> : null}
      {loading ? <Loading /> : null}
      {loadError && !loading ? <Notice tone="error" action={{label: i18n.t('action.tryAgain'), onClick: retry}}>{errorText(loadError, 'home', 'load')}</Notice> : null}
      {view && !loading && !loadError ? (
      <ul className={s.list}>
        {order.map((id, i) => {
          const row = rowOf(id);
          const title = addedNames.get(id) ?? homeRowTitle(id, row?.title, libraryName, row?.titleText);
          const savedView = !!homeViewResourceId(id);
          const isHidden = hidden.has(id);
          return (
            <li
              key={id}
              className={cx(s.row, isHidden && s.hidden, dragging === id && s.dragging)}
              draggable={row?.reorderable !== false && !busy}
              onDragStart={e => { e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', title); setDragging(id); }}
              onDragOver={e => dragOver(e, id)}
              onDrop={e => e.preventDefault()}
              onDragEnd={() => setDragging(undefined)}
            >
              <div className={s.handle}>
                <button type="button" className={s.arrow} aria-label={i18n.t('home.customize.moveUp', {title})} disabled={i === 0 || row?.reorderable === false || busy} onClick={() => move(id, -1)}><Icon name="chevronUp" size={14} /></button>
                <button type="button" className={s.arrow} aria-label={i18n.t('home.customize.moveDown', {title})} disabled={i === order.length - 1 || row?.reorderable === false || busy} onClick={() => move(id, 1)}><Icon name="chevronDown" size={14} /></button>
              </div>
              <div className={s.copy}>
                <Text variant="body">{title}</Text>
              </div>
              {savedView ? <Button variant="ghost" size="sm" icon="close" aria-label={i18n.t('home.customize.removeRow', {title})} title={i18n.t('home.customize.removeRow', {title})} disabled={busy} onClick={() => removeView(id)} /> : null}
              {/* CON-23: a row that can't be hidden says so instead of showing a locked switch; the switch's name never flips with its state. */}
              {row?.required || row?.hideable === false
                ? <span className={s.always}><Icon name="lock" size={14} />{i18n.t('home.customize.alwaysOn')}</span>
                : <Switch label={i18n.t('home.customize.showRow', {title})} checked={!isHidden} disabled={busy} onCheckedChange={v => setHidden(prev => { const n = new Set(prev); if (v) n.delete(id); else n.add(id); return n; })} />}
            </li>
          );
        })}
      </ul>
      ) : null}
      {view && !loading && !loadError ? (picking ? <SavedViewPicker exclude={order} onPick={addView} onCancel={() => setPicking(false)} /> : <div><Button variant="secondary" size="sm" icon="plus" label={i18n.t('home.customize.addView')} disabled={busy} onClick={() => setPicking(true)} /></div>) : null}
    </Dialog>
  );
}

/** P6: the viewer's saved views (the saved-resources API), to put one on Home. */
function SavedViewPicker({exclude, onPick, onCancel}: {exclude: readonly string[]; onPick: (resourceId: string, name: string) => void; onCancel: () => void}) {
  const i18n = useI18n();
  const {api} = useSession();
  const scope = useViewerScope();
  const {service, snapshot} = useService<PersonalSavedService, PersonalSavedSnapshot>(() => new PersonalSavedService(api, scope, requestId), [api, scope]);
  useEffect(() => { void service.select({view: 'views'}).catch(() => {}); }, [service]);
  const views = snapshot.resources.filter(r => r.kind === 'view' && r.status === 'ready' && !exclude.includes(homeViewRowId(r.id)));
  return (
    <div className={s.picker}>
      <Text variant="caption" tone="secondary">{i18n.t('home.customize.chooseView')}</Text>
      {snapshot.loading ? <Loading label={i18n.t('home.customize.loadingViews')} /> : null}
      {snapshot.error ? <Notice tone="error" action={{label: i18n.t('action.tryAgain'), onClick: () => void service.refresh()}}>{errorText(snapshot.error, 'home', 'load')}</Notice> : null}
      {views.map(v => <Button key={v.id} variant="ghost" icon="filter" label={v.name} onClick={() => onPick(v.id, v.name)} />)}
      {!snapshot.loading && !snapshot.error && !views.length ? <Text variant="caption" tone="tertiary">{i18n.t('home.customize.noViews')}</Text> : null}
      <div style={{display: 'flex', gap: 8}}>
        {snapshot.nextCursor ? <Button variant="ghost" size="sm" label={i18n.t('action.showMore')} disabled={snapshot.loading} onClick={() => void service.next()} /> : null}
        <Button variant="ghost" size="sm" label={i18n.t('action.cancel')} onClick={onCancel} />
      </div>
    </div>
  );
}
