import React, {useEffect, useState, useSyncExternalStore} from 'react';
import type {WindowedCollection} from '@core/collections/index.ts';
import {manualMoves, movedOrder, moveTarget, type ManualMove} from '@core/presentation/manual-order.ts';
import {cardCaptionOf} from '../../app/content';
import {iconFor, shapeFor} from '@core/presentation/index.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {Button, Icon, ListRow, Menu, cx, type MenuItem} from '../../ui';
import {useI18n} from '../../app/i18n';
import s from './ManualOrder.module.css';

export type OrderedEntry = Readonly<{id: string; media: ContentEntry}>;

/**
 * A list that keeps its own order (a personal collection, a library collection), shown for moving
 * its titles: drag a row by its handle, or use the row's menu (Move to top, up, down, to bottom),
 * which a keyboard reaches like any other button. A move is shown at once and sent as the entry
 * and the one it now follows; when the server refuses it the list goes back to the server's.
 */
export function ManualOrderList({entries, whole, label, onMove}: {entries: readonly OrderedEntry[]; whole: boolean; label: string; onMove: (from: number, to: number) => Promise<void>}) {
  const {t} = useI18n();
  const [shown, setShown] = useState(entries);
  const [dragging, setDragging] = useState<number | null>(null);
  const [over, setOver] = useState<{index: number; after: boolean} | null>(null);
  const [busy, setBusy] = useState(false);
  // The server's order wins whenever it arrives (a refresh after a move, a change made elsewhere).
  useEffect(() => setShown(entries), [entries]);
  const move = (from: number, to: number) => {
    if (busy || from === to || to < 0 || to >= shown.length) return;
    const before = shown;
    setShown(movedOrder(shown, from, to));
    setBusy(true);
    void onMove(from, to).catch(() => setShown(before)).finally(() => setBusy(false));
  };
  const moveLabel: Record<ManualMove, string> = {top: t('order.moveTop'), up: t('web.profile.moveUp'), down: t('web.profile.moveDown'), bottom: t('order.moveBottom')};
  const moveIcon: Record<ManualMove, MenuItem['icon']> = {top: 'chevronUp', up: 'chevronUp', down: 'chevronDown', bottom: 'chevronDown'};
  const drop = (index: number) => {
    if (dragging === null || !over) return;
    // Dropped on the lower half of a row: after it; on the upper half: before it.
    const target = over.after ? (dragging < index ? index : index + 1) : (dragging < index ? index - 1 : index);
    move(dragging, Math.max(0, Math.min(shown.length - 1, target)));
  };
  return (
    <div className={s.list} role="list" aria-label={label} aria-busy={busy || undefined}>
      {shown.map((e, i) => {
        const moves = manualMoves(i, shown.length, whole);
        const items: MenuItem[] = moves.map(m => ({id: m, label: moveLabel[m], icon: moveIcon[m]}));
        return (
          <div key={e.id} role="listitem"
            className={cx(s.item, dragging === i && s.dragging, over?.index === i && dragging !== i && (over.after ? s.after : s.before))}
            onDragOver={ev => { if (dragging === null) return; ev.preventDefault(); const box = ev.currentTarget.getBoundingClientRect(); setOver({index: i, after: ev.clientY > box.top + box.height / 2}); }}
            onDragLeave={() => setOver(o => (o?.index === i ? null : o))}
            onDrop={ev => { ev.preventDefault(); drop(i); setDragging(null); setOver(null); }}>
            <span className={s.handle} draggable={!busy} aria-hidden
              title={t('order.handle', {title: e.media.title})}
              onDragStart={ev => { ev.dataTransfer.effectAllowed = 'move'; ev.dataTransfer.setData('text/plain', e.id); setDragging(i); }}
              onDragEnd={() => { setDragging(null); setOver(null); }}>
              <Icon name="grip" size={18} />
            </span>
            <ListRow title={e.media.title} subtitle={cardCaptionOf(e.media)} art={{path: e.media.posterUrl, icon: iconFor(e.media.kind)}} artShape={shapeFor(e.media.kind)}
              actions={items.length ? <Menu label={t('web.dvr.actionsFor', {title: e.media.title})} trigger={<Button variant="ghost" icon="more" size="sm" aria-label={t('web.dvr.actionsFor', {title: e.media.title})} disabled={busy} />} items={items} onSelect={id => move(i, moveTarget(id as ManualMove, i, shown.length))} /> : null} />
          </div>
        );
      })}
    </div>
  );
}

/** The titles a browse list has, all of them up to `max`, for arranging them: it asks for every page
 * and follows them as they arrive. `whole` once the list's last title is on hand. */
export function useWholeList(collection: WindowedCollection<ContentEntry> | undefined, max = 500): Readonly<{entries: readonly OrderedEntry[]; whole: boolean}> {
  const snapshot = useSyncExternalStore(collection?.subscribe ?? noSubscribe, () => collection?.getSnapshot());
  const total = snapshot?.total;
  useEffect(() => { if (collection && total) collection.ensureRange(0, Math.min(total, max) - 1); }, [collection, total, max]);
  const entries: OrderedEntry[] = [];
  for (let i = 0; collection && total !== undefined && i < Math.min(total, max); i++) {
    const item = collection.itemAt(i);
    if (!item) break;
    entries.push({id: item.id, media: item});
  }
  return {entries, whole: total !== undefined && !snapshot?.totalIsLowerBound && entries.length === total};
}
const noSubscribe = () => () => {};
