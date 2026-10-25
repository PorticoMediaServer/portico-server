import {useCallback, useLayoutEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {ContentEntry} from '@core/library-content.ts';
import {Menu} from '../../kit/overlays';
import type {CardSelectionIntent} from '../../kit/cards';
import type {BrowseItemActions} from './EntryCard';
import type {BrowseController, BrowseMember} from './controller';

const t = defaultI18n.t;

/** One card's menu, bound to the exact opening: the card, its entry, the
 * controller and the applied query that drew it. */
type CardMenu = Readonly<{anchor: HTMLElement; title: string; entry: ContentEntry; controller: BrowseController;
  queryRevision: number; play?: () => void; select?: () => void}>;

export type CardMenuInput = Readonly<{
  controller: BrowseController;
  /** The applied query revision the card was drawn under. */
  queryRevision: number;
  /** The surface's own per-card actions, including its selection descriptor. */
  actions: (member: BrowseMember, index: number) => BrowseItemActions;
}>;

/** The drawn card menu (select.html 17): its ⋯ button or a hold on an unselected
 * card opens it at that exact card, with Play when the entry plays and Select when
 * the card belongs to the surface's explicit selection. Everything here is
 * presentation over the surface's own fenced actions. */
export function useLibraryCardMenu(input: CardMenuInput) {
  const {controller, queryRevision, actions} = input;
  const [cardMenu, setCardMenu] = useState<CardMenu>();
  const cardAnchor = useRef<HTMLElement | null>(null), cardMenuRef = useRef<CardMenu | undefined>(undefined);
  // The ref follows committed openings only, and its cleanup retires the exact
  // opening it held; a later menu is never cleared by an earlier one.
  useLayoutEffect(() => {
    cardMenuRef.current = cardMenu;
    return () => {if (cardMenuRef.current === cardMenu) cardMenuRef.current = undefined;};
  }, [cardMenu]);
  const open = useCallback((anchor: HTMLElement, base: BrowseItemActions, entry: ContentEntry, title: string) => {
    // A retained opener cannot draw a menu for a page the controller no longer serves.
    if (controller.getSnapshot().queryRevision !== queryRevision || !controller.contains(entry)) return;
    cardAnchor.current = anchor;
    setCardMenu({anchor, title, entry, controller, queryRevision,
      ...(base.onPlay ? {play: base.onPlay} : {}), ...(base.selection ? {select: () => base.selection!.onToggle({range: false, source: 'pointer'})} : {})});
  }, [controller, queryRevision]);
  /** The exact opening still owns the menu: same object, same controller, same applied query, same registered page. */
  const current = useCallback((opened: CardMenu) => cardMenuRef.current === opened && opened.controller === controller
    && controller.getSnapshot().queryRevision === opened.queryRevision && controller.contains(opened.entry), [controller]);
  const cardActions = useCallback((member: BrowseMember, index: number): BrowseItemActions => {
    const base = actions(member, index), title = member.entry.displayTitle ?? member.entry.title, descriptor = base.selection;
    // No ⋯ without something it can really do, and an owner that already supplies
    // its own menu keeps it.
    const own = (base.onPlay || descriptor) ? (anchor: HTMLButtonElement) => open(anchor, base, member.entry, title) : undefined;
    const more = base.onMore ?? own;
    return {...base, ...(more ? {onMore: more} : {}),
      ...(descriptor ? {selection: {...descriptor, onToggle: (intent: CardSelectionIntent) => {
        // A hold opens the menu only while this card is not selecting; once the
        // surface is selecting, the card keeps its drawn toggle gesture.
        if (intent.source === 'long-press' && intent.anchor && descriptor.active === false && (base.onMore || own)) {
          if (base.onMore) base.onMore(intent.anchor as HTMLButtonElement);
          else open(intent.anchor, base, member.entry, title);
          return;
        }
        descriptor.onToggle(intent);
      }}} : {})};
  }, [actions, open]);
  // A new applied query retires the menu at once; the stale state never draws again.
  const openMenu = cardMenu && cardMenu.controller === controller && cardMenu.queryRevision === queryRevision ? cardMenu : undefined;
  // An accepted close or action claims the opening synchronously, so a retained
  // same-tick handler finds nothing to dispatch twice.
  const dismiss = (opened: CardMenu) => {if (cardMenuRef.current !== opened) return; cardMenuRef.current = undefined; setCardMenu(undefined);};
  const take = (opened: CardMenu, act: () => void) => {if (!current(opened)) return; cardMenuRef.current = undefined; setCardMenu(undefined); act();};
  const menu = <Menu open={!!openMenu} onOpenChange={value => {if (!value && openMenu) dismiss(openMenu);}} title={openMenu?.title ?? ''} anchorRef={cardAnchor} width={280}
    items={openMenu ? [
      ...(openMenu.play ? [{id: 'play', label: t('next.selection.play'), icon: 'i-play' as const, onSelect: () => take(openMenu, openMenu.play!)}] : []),
      ...(openMenu.select ? [{id: 'select', label: t('next.selection.select'), icon: 'i-select' as const, onSelect: () => take(openMenu, openMenu.select!)}] : []),
    ] : []}/>;
  return {cardActions, menu};
}

