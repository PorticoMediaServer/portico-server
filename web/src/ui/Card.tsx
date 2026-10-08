import React, {useRef} from 'react';
import {uiI18n} from './i18n';
import {cx} from './cx';
import {Artwork, type ArtworkShape} from './Artwork';
import {Icon, type IconName} from './Icon';
import {anchorAt, anchorOf, type MenuAnchor} from './Menu';
import s from './Card.module.css';

export type CardProps = {
  title: string;
  caption?: string;
  path?: string;
  shape?: ArtworkShape;
  icon?: IconName;
  progress?: number;
  watched?: boolean;
  unavailable?: boolean;
  onOpen?: () => void;
  onPlay?: () => void;
  /** Opens the actions menu from the anchor the gesture supplies (control box, or pointer for right-click and long press). */
  onMore?: (anchor: MenuAnchor) => void;
  /** The card's page: the art becomes a link, so middle-click, a new tab and Copy link work; a plain click still opens in place. */
  href?: string;
  className?: string;
  hideCopy?: boolean;
  /** `contain` fits square art (albums) inside a poster frame so a mixed row keeps one geometry (WEB-HOME-01). */
  fit?: 'cover' | 'contain';
  /** Selection mode: the art toggles selection instead of opening. */
  selectable?: boolean;
  selected?: boolean;
  onSelect?: (modifiers: {range: boolean}) => void;
};

/**
 * Media card: reserved artwork geometry, title and up to two lines of
 * decision-useful copy. Hover or keyboard focus reveals Play and More; both
 * remain reachable through the context menu on touch.
 */
/** Memoised; the section passes stable handlers now that player intents are a stable context. */
export const Card = React.memo(function Card({title, caption, path, shape = 'poster', icon, progress, watched, unavailable, onOpen, onPlay, onMore, href, className, hideCopy, selectable, selected, onSelect, fit}: CardProps) {
  const stop = (fn?: () => void) => (e: React.MouseEvent) => {
    e.stopPropagation();
    e.preventDefault();
    fn?.();
  };
  // Touch has no hover: a long press opens More; right-click does the same on desktop.
  const press = useRef<{timer?: ReturnType<typeof setTimeout>; fired: boolean}>({fired: false});
  const pressStart = (e: React.PointerEvent) => {
    if (!onMore || e.pointerType === 'mouse') return;
    press.current.fired = false;
    const at = anchorAt(e.clientX, e.clientY);
    press.current.timer = setTimeout(() => { press.current.fired = true; onMore(at); }, 500);
  };
  const pressEnd = () => { clearTimeout(press.current.timer); };
  // WEB-SYS-04: More and Play stay out of the tab order, so the focused card answers keys:
  // M, the context-menu key or Shift+F10 open More; P plays.
  const onKey = (e: React.KeyboardEvent<HTMLElement>) => {
    if (selectable) return;
    if (onMore && (e.key === 'm' || e.key === 'M' || e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10'))) { e.preventDefault(); onMore(anchorOf(e.currentTarget)); }
    else if (onPlay && (e.key === 'p' || e.key === 'P')) { e.preventDefault(); onPlay(); }
    // A link answers Enter by itself; Space opens it too, as it did when the card was a button.
    else if (href && e.key === ' ') { e.preventDefault(); onOpen?.(); }
  };
  // In selection mode the art toggles the selection, so it is a button, not a link.
  const link = !!href && !selectable;
  const activate = (e: React.MouseEvent) => {
    if (press.current.fired) { press.current.fired = false; e.preventDefault(); return; }
    if (selectable || (onSelect && e.shiftKey)) { e.preventDefault(); onSelect?.({range: e.shiftKey}); return; }
    // A link's Cmd/Ctrl-click and middle-click are the browser's: a new tab.
    if (link && (e.metaKey || e.ctrlKey || e.altKey || e.button !== 0)) return;
    if (!link && onSelect && (e.metaKey || e.ctrlKey)) { e.preventDefault(); onSelect({range: false}); return; }
    if (link) e.preventDefault();
    onOpen?.();
  };
  return (
    <div className={cx(s.card, unavailable && s.unavailable, shape === 'circle' && s.person, selectable && s.selecting, selected && s.selected, className)} role="group">
      <div className={s.art}>
        {React.createElement(
          link ? 'a' : 'button',
          {
            ...(link ? {href} : {type: 'button'}),
            className: s.artButton,
            onClick: activate,
            onContextMenu: onMore && !selectable ? (e: React.MouseEvent) => { e.preventDefault(); e.stopPropagation(); onMore(anchorAt(e.clientX, e.clientY)); } : undefined,
            onPointerDown: selectable ? undefined : pressStart,
            onPointerUp: pressEnd,
            onPointerLeave: pressEnd,
            onPointerCancel: pressEnd,
            // A link is dragged by the browser; the card is not a drag source.
            draggable: link ? false : undefined,
            'aria-label': title,
            'aria-pressed': selectable ? !!selected : undefined,
            onKeyDown: onKey,
            'aria-describedby': onMore || onPlay ? 'card-keyboard-hint' : undefined,
          },
          <Artwork path={path} shape={shape} icon={icon} progress={progress} alt="" fit={fit} initial={shape === 'circle' ? title.slice(0, 1) : undefined} />,
        )}
        {/* One hidden hint per card is read on every card by a screen reader (sixty times on Home);
            the shortcut description lives once in the shell instead (`#card-keyboard-hint`). */}
        <div className={s.badges}>{onSelect ? <button type="button" className={cx(s.checkbox, selected && s.checked, selectable && s.checkboxLive)} role="checkbox" tabIndex={selectable ? 0 : -1} aria-checked={!!selected} aria-label={uiI18n().t('card.select', {title})} onClick={e => { e.stopPropagation(); onSelect({range: e.shiftKey}); }}>{selected ? <Icon name="check" size={14} strokeWidth={2.4} /> : null}</button> : <span />}{watched ? <span className={s.watched} title={uiI18n().t('status.watched')}><Icon name="check" size={14} strokeWidth={2.2} /></span> : null}</div>
        {(onPlay || onMore) && !selectable ? (
          <div className={s.overlay}>
            {onPlay ? <button type="button" className={s.playButton} tabIndex={-1} onClick={stop(onPlay)} aria-label={uiI18n().t('card.play', {title})}><Icon name="play" size={18} /></button> : <span />}
            {onMore ? <button type="button" className={s.moreButton} tabIndex={-1} onClick={e => { e.stopPropagation(); e.preventDefault(); onMore(anchorOf(e.currentTarget)); }} aria-label={uiI18n().t('card.more', {title})} aria-haspopup="menu"><Icon name="more" size={16} /></button> : null}
          </div>
        ) : null}
      </div>
      {!hideCopy ? (
        <div className={s.copy}>
          <span className={s.title} title={title}>{title}</span>
          {caption ? <span className={s.caption}>{caption}</span> : null}
        </div>
      ) : null}
    </div>
  );
});

export function BannerCard({title, caption, path, onOpen, className}: {title: string; caption?: string; path?: string; onOpen?: () => void; className?: string}) {
  return (
    <button type="button" className={cx(s.card, s.banner, className)} onClick={onOpen} style={{padding: 0}}>
      <Artwork path={path} shape="landscape" icon="collection" alt="" />
      <div className={s.bannerCopy}>
        <span className={s.title} title={title}>{title}</span>
        {caption ? <span className={s.caption}> · {caption}</span> : null}
      </div>
    </button>
  );
}
