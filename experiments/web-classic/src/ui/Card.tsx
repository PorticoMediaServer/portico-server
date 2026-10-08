import React, {memo, useState} from 'react';
import {Link} from '@tanstack/react-router';
import type {Entry, Shape} from '@/data/types';
import {useActions, type Origin} from '@/app/actions';
import {useSelection} from '@/app/selection';
import {hrefFor} from '@/app/links';
import {remaining} from '@/app/format';
import {Artwork, Progress} from './Bits';
import {Icon} from './Icon';
import {ContextMenu, Menu, type MenuItem} from './Overlay';
import {cx} from './cx';
import s from './Card.module.css';

export function cardLines(e: Entry): [string, string] {
  if (e.kind === 'episode') {
    const code = (e.caption ?? '').split(' · ').filter(p => /^(S\d+|E\d+|Special)/.test(p)).join(' · ');
    const showTitle = (e.caption ?? '').split(' · ').find(p => !/^(S\d+|E\d+|Special)/.test(p));
    return [showTitle ?? e.subtitle ?? e.title, [code.replace(' · ', ' '), e.title].filter(Boolean).join(' · ')];
  }
  if ((e.kind === 'movie' || e.kind === 'homevideo') && e.progress && e.duration) return [e.title, remaining(e.duration, e.progress)];
  return [e.title, e.subtitle ?? ''];
}

type Props = {
  entry: Entry;
  shape?: Shape;
  /** In a grid the card fills its column instead of taking the row width. */
  grid?: boolean;
  origin?: Origin;
  subtitle?: string;
  eager?: boolean;
  /** Part of a roving group (rows and grids): one Tab stop, arrows move. */
  rove?: boolean;
  onOpen?: (e: Entry) => void;
};

/**
 * The media card: poster, wide, square or circle artwork with watched,
 * unwatched-count and progress overlays, title and subtitle underneath.
 * Click opens, the round button plays, the ⋯ button and right-click open the
 * actions menu. Keyboard: Enter opens, P plays, the context-menu key or
 * Shift+F10 opens the menu.
 */
export const MediaCard = memo(function MediaCard({entry, shape, grid, origin, subtitle, eager, rove = true, onOpen}: Props) {
  const actions = useActions();
  const sel = useSelection();
  const [items, setItems] = useState<MenuItem[]>([]);
  const sh = shape ?? entry.shape;
  const href = hrefFor(entry);
  const art = sh === 'poster' && entry.poster ? entry.poster : entry.image;
  const [line1, line2] = cardLines(entry);
  const selected = sel.active && sel.has(entry.id);
  const playable = entry.playable !== false && entry.kind !== 'person' && entry.kind !== 'author' && entry.kind !== 'series' && entry.kind !== 'genre' && entry.kind !== 'studio';
  const label = [line1, subtitle ?? line2, entry.watched ? 'watched' : entry.unwatched ? `${entry.unwatched} unwatched` : entry.progress ? `${Math.round(entry.progress * 100)}% watched` : ''].filter(Boolean).join(', ');
  const onClick = (e: React.MouseEvent) => {
    if (sel.active || e.shiftKey) { e.preventDefault(); sel.toggle(entry); return; }
    if (onOpen) { e.preventDefault(); onOpen(entry); return; }
    if (!href) { e.preventDefault(); void actions.play(entry); }
  };
  const onKeyDown = (e: React.KeyboardEvent) => {
    if ((e.key === 'p' || e.key === 'P') && playable && !e.metaKey && !e.ctrlKey) { e.preventDefault(); void actions.play(entry); }
    if (e.key === 'x' && !e.metaKey && !e.ctrlKey) { e.preventDefault(); sel.toggle(entry); }
    // Shift+F10 and the context-menu key open the actions menu at the card.
    if ((e.key === 'F10' && e.shiftKey) || e.key === 'ContextMenu') {
      e.preventDefault();
      const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
      e.currentTarget.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true, clientX: r.left + r.width / 2, clientY: r.top + Math.min(r.height / 2, 120)}));
    }
  };
  const frame = (
    <div className={s.frame}>
      <Artwork src={art} title={entry.kind === 'episode' ? line1 : entry.title} shape={sh} eager={eager} tint={entry.tint} icon={entry.kind === 'playlist' ? 'playlist' : entry.kind === 'author' ? 'user' : undefined} />
      <div className={s.hover} />
      {entry.badge === 'LIVE' ? <span className={s.live}>LIVE</span> : entry.badge ? <span className={s.corner}>{entry.badge}</span> : null}
      {entry.watched && !entry.progress && entry.kind !== 'person' ? <span className={s.watched} aria-hidden><Icon name="check" size={14} strokeWidth={2.8} /></span> : null}
      {!entry.watched && entry.unwatched && entry.kind === 'show' ? <span className={s.count} aria-hidden>{entry.unwatched > 99 ? '99+' : entry.unwatched}</span> : null}
      {entry.progress && entry.progress > 0 && entry.progress < 1 ? <Progress value={entry.progress} className={s.progress} label={`${Math.round(entry.progress * 100)}% watched`} /> : null}
      {sel.active ? <span className={s.select} aria-hidden>{selected ? <Icon name="check" size={14} strokeWidth={2.8} /> : null}</span> : null}
    </div>
  );
  const hitProps = {
    className: s.hit,
    'aria-label': label,
    'data-rove': rove ? '' : undefined,
    tabIndex: rove ? -1 : 0,
    onClick,
    onKeyDown,
    draggable: false,
  };
  return (
    <ContextMenu items={() => actions.items(entry, origin)}>
      <div className={cx(s.card, grid && s.gridCard, sh === 'circle' && s.circleCard, sel.active && s.selecting, selected && s.selected)} style={grid ? undefined : ({'--card-w': `var(--${sh}-w)`} as React.CSSProperties)} aria-selected={sel.active ? selected : undefined}>
        <div className={s.artBox}>
        {href ? <Link to={href as never} {...hitProps}>{frame}</Link> : <button type="button" {...hitProps} style={{width: '100%', textAlign: 'left'}}>{frame}</button>}
        {playable && !sel.active ? (
          <button type="button" className={s.play} tabIndex={-1} aria-label={`Play ${entry.title}`} onClick={() => void actions.play(entry)} style={sh === 'wide' ? undefined : undefined}>
            <Icon name="play" size={22} />
          </button>
        ) : null}
        {!sel.active ? (
          <Menu label={`More for ${entry.title}`} items={items} trigger={
            <button type="button" className={s.more} tabIndex={-1} aria-label={`More for ${entry.title}`} onPointerDown={() => setItems(actions.items(entry, origin))} onKeyDown={() => setItems(actions.items(entry, origin))}>
              <Icon name="more" size={16} />
            </button>
          } />
        ) : null}
        </div>
        <div className={s.text}>
          {href ? <Link to={href as never} className={s.title} tabIndex={-1} onClick={onClick}>{line1}</Link> : <span className={s.title}>{line1}</span>}
          {subtitle ?? line2 ? <span className={s.sub}>{subtitle ?? line2}</span> : <span className={s.sub} aria-hidden>&nbsp;</span>}
        </div>
      </div>
    </ContextMenu>
  );
});

export function CardSkeleton({shape = 'poster', grid}: {shape?: Shape; grid?: boolean}) {
  return (
    <div className={cx(s.card, grid && s.gridCard)} style={grid ? undefined : ({'--card-w': `var(--${shape}-w)`} as React.CSSProperties)} aria-hidden>
      <div className={cx('skeleton')} style={{aspectRatio: shape === 'poster' ? '2 / 3' : shape === 'wide' ? '16 / 9' : '1', borderRadius: shape === 'circle' ? '50%' : undefined}} />
      <div className={cx('skeleton', s.skelText)} style={shape === 'circle' ? {marginInline: 'auto'} : undefined} />
      <div className={cx('skeleton', s.skelText2)} style={shape === 'circle' ? {marginInline: 'auto'} : undefined} />
    </div>
  );
}
