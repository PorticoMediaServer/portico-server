import React, {useCallback, useEffect, useState, useSyncExternalStore} from 'react';
import type {WindowedCollection} from '@core/collections/index.ts';
import {manualMoves, movedOrder, moveRequestAt, moveTarget, placeBeforeMoves, type ManualMove} from '@core/presentation/manual-order.ts';
import {cardCaptionOf} from '../../app/content';
import {iconFor, shapeFor} from '@core/presentation/index.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {Button, Icon, ListRow, Menu, WindowedGrid, cx, type MenuItem} from '../../ui';
import {useI18n} from '../../app/i18n';
import s from './ManualOrder.module.css';

export type OrderedEntry = Readonly<{id: string; media: ContentEntry}>;

/**
 * A personal collection's own order (every entry on hand, read page by page as it scrolls), shown
 * for moving its titles: drag a row by its handle, or use the row's menu (Move to top, up, down, to bottom),
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
  const drop = (index: number) => {
    if (dragging === null || !over) return;
    // Dropped on the lower half of a row: after it; on the upper half: before it.
    const target = over.after ? (dragging < index ? index : index + 1) : (dragging < index ? index - 1 : index);
    move(dragging, Math.max(0, Math.min(shown.length - 1, target)));
  };
  return (
    <div className={s.list} role="list" aria-label={label} aria-busy={busy || undefined}>
      {shown.map((e, i) => (
        <OrderRow key={e.id} media={e.media} index={i} count={shown.length} whole={whole} busy={busy} dragging={dragging} over={over} t={t}
          onDrag={setDragging} onOver={setOver} onDrop={drop} onMove={move} />
      ))}
    </div>
  );
}

/**
 * A library collection's own order, being arranged, however long it is: only the rows in view are
 * drawn and read (the list's own windowed pages), so the 5,000th title moves like the 5th. A move
 * shows at once and stays shown until the list, read again after it, arrives with the server's
 * order. Moves need only the moved title and its new neighbour on hand; "To the bottom" appears
 * once the last title has been read.
 */
export function WindowedOrderList({collection, label, onMove}: {collection: WindowedCollection<ContentEntry>; label: string; onMove: (request: Readonly<{entryId: string; afterEntryId: string}>) => Promise<void>}) {
  const {t} = useI18n();
  const snapshot = useSyncExternalStore(collection.subscribe, collection.getSnapshot);
  const total = snapshot.total ?? 0;
  // Moves made since the list was last read (they lapse when a new reading is shown).
  const [made, setMade] = useState<{generation: number; moves: readonly {from: number; to: number}[]}>({generation: snapshot.generation, moves: []});
  const moves = made.generation === snapshot.generation ? made.moves : noMoves;
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState<number | null>(null);
  const [over, setOver] = useState<{index: number; after: boolean} | null>(null);
  const itemAt = useCallback((index: number) => collection.itemAt(placeBeforeMoves(index, moves)), [collection, moves]);
  const lastKnown = !snapshot.totalIsLowerBound && total > 0 && !!itemAt(total - 1);
  const move = useCallback((from: number, to: number) => {
    if (busy || from === to) return;
    const request = moveRequestAt(i => itemAt(i)?.id, total, from, to);
    if (!request) return;
    const generation = snapshot.generation;
    setMade(m => ({generation, moves: [...(m.generation === generation ? m.moves : []), {from, to}]}));
    setBusy(true);
    void onMove(request).catch(() => setMade({generation, moves: []})).finally(() => setBusy(false));
  }, [busy, itemAt, total, snapshot.generation, onMove]);
  const drop = useCallback((index: number) => {
    if (dragging === null || !over) return;
    const target = over.after ? (dragging < index ? index : index + 1) : (dragging < index ? index - 1 : index);
    move(dragging, Math.max(0, Math.min(total - 1, target)));
  }, [dragging, over, total, move]);
  const renderItem = useCallback((loaded: ContentEntry, index: number) => {
    const media = itemAt(index) ?? loaded;
    return (
      <OrderRow listItem={false} media={media} index={index} count={total} whole={lastKnown} busy={busy} dragging={dragging} over={over} t={t}
        onDrag={setDragging} onOver={setOver} onDrop={drop} onMove={move} />
    );
  }, [itemAt, total, lastKnown, busy, dragging, over, t, move, drop]);
  const placeholder = useCallback(() => <div className={s.placeholder} aria-hidden />, []);
  return (
    <div aria-busy={busy || undefined}>
      <WindowedGrid collection={collection} estimateRowHeight={64} label={label} renderItem={renderItem} renderPlaceholder={placeholder} />
    </div>
  );
}
const noMoves: readonly {from: number; to: number}[] = [];

/** One row of a list being arranged: its handle, and its menu of moves. */
function OrderRow({media, index, count, whole, busy, dragging, over, t, onDrag, onOver, onDrop, onMove, listItem = true}: {/** The row is its list's item (false where the windowed grid's cell already is). */ listItem?: boolean; media: ContentEntry; index: number; count: number; whole: boolean; busy: boolean; dragging: number | null; over: {index: number; after: boolean} | null; t: ReturnType<typeof useI18n>['t']; onDrag: (index: number | null) => void; onOver: React.Dispatch<React.SetStateAction<{index: number; after: boolean} | null>>; onDrop: (index: number) => void; onMove: (from: number, to: number) => void}) {
  const moveLabel: Record<ManualMove, string> = {top: t('order.moveTop'), up: t('web.profile.moveUp'), down: t('web.profile.moveDown'), bottom: t('order.moveBottom')};
  const items: MenuItem[] = manualMoves(index, count, whole).map(m => ({id: m, label: moveLabel[m], icon: m === 'top' || m === 'up' ? 'chevronUp' : 'chevronDown'}));
  return (
    <div role={listItem ? 'listitem' : undefined}
      className={cx(s.item, dragging === index && s.dragging, over?.index === index && dragging !== index && (over.after ? s.after : s.before))}
      onDragOver={ev => { if (dragging === null) return; ev.preventDefault(); const box = ev.currentTarget.getBoundingClientRect(); onOver({index, after: ev.clientY > box.top + box.height / 2}); }}
      onDragLeave={() => onOver(o => (o?.index === index ? null : o))}
      onDrop={ev => { ev.preventDefault(); onDrop(index); onDrag(null); onOver(null); }}>
      <span className={s.handle} draggable={!busy} aria-hidden
        title={t('order.handle', {title: media.title})}
        onDragStart={ev => { ev.dataTransfer.effectAllowed = 'move'; ev.dataTransfer.setData('text/plain', media.id); onDrag(index); }}
        onDragEnd={() => { onDrag(null); onOver(null); }}>
        <Icon name="grip" size={18} />
      </span>
      <ListRow title={media.title} subtitle={cardCaptionOf(media)} art={{path: media.posterUrl, icon: iconFor(media.kind)}} artShape={shapeFor(media.kind)}
        actions={items.length ? <Menu label={t('web.dvr.actionsFor', {title: media.title})} trigger={<Button variant="ghost" icon="more" size="sm" aria-label={t('web.dvr.actionsFor', {title: media.title})} disabled={busy} />} items={items} onSelect={id => onMove(index, moveTarget(id as ManualMove, index, count))} /> : null} />
    </div>
  );
}
