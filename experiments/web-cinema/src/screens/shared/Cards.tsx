import React, {useCallback, useEffect, useLayoutEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import {Link, useNavigate} from '@tanstack/react-router';
import {useDataVersion, useServices} from '../../data/hooks';
import type {Episode, Item, Movie, Shape} from '../../data/types';
import {setTint} from '../../app/tint';
import {selection, useSelection} from '../../app/selection';
import {useSession} from '../../app/session';
import {Art} from '../../ui/Media';
import {Icon} from '../../ui/Icon';
import {Menu, Tooltip} from '../../ui/Overlay';
import {Badge, Progress, toast} from '../../ui/Feedback';
import {cx} from '../../ui/cx';
import {useHover} from '../../ui/useMediaQuery';
import {reducedMotion} from '../../app/prefs';
import {artFor, backdropFor, bestResolution, canWatchlist, episodeCode, factsFor, hrefFor, isVideo, linkFor, shapeFor, subtitleFor} from './items';
import {entryMenu} from './actions';
import {startPlayback} from './playback';

export type CardContext = 'continue' | 'recommendation' | 'card';

/**
 * A title in a row or grid. Hover (with a fine pointer) opens a preview that grows out of the card
 * with the backdrop, the key facts and quick actions, like the streaming services do. Focus lifts
 * the card with a bright ring. Either way the room's glow picks up the artwork's colour.
 */
export function MediaCard({item, shape: forced, context = 'card', showTitle, rank, fill, subtitle, caption = true, onOpen, eager}: {
  item: Item;
  shape?: Shape;
  context?: CardContext;
  showTitle?: string;
  rank?: number;
  fill?: boolean;
  subtitle?: string;
  caption?: boolean;
  onOpen?: () => void;
  eager?: boolean;
}) {
  const services = useServices();
  useDataVersion();
  const st = services.user.state(item.id);
  const shape = forced ?? shapeFor(item);
  const sel = useSelection();
  const selecting = sel.active;
  const selected = sel.ids.includes(item.id);
  const hoverCapable = useHover();
  const ref = useRef<HTMLAnchorElement>(null);
  const [preview, setPreview] = useState<DOMRect | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const link = linkFor(item as never);
  const episode = item.kind === 'episode' ? (item as Episode) : null;
  const sub = subtitle ?? (episode ? episodeCode(episode) + (showTitle ? '' : '') : subtitleFor(item, showTitle));
  const title = episode && showTitle ? showTitle : item.title;
  const inProgress = st.progress > 0 && !st.watched;

  const enter = () => {
    setTint(item.tint);
    if (!hoverCapable || selecting || shape === 'circle' || reducedMotion()) return;
    clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      if (ref.current && ref.current.matches(':hover')) setPreview(ref.current.getBoundingClientRect());
    }, 520);
  };
  const leave = () => clearTimeout(timer.current);
  useEffect(() => () => clearTimeout(timer.current), []);

  const inner = (
    <>
      <span className="card-frame">
        <Art src={artFor(item, shape)} alt="" tint={item.tint} fallback={item.title} fallbackIcon={shape === 'square' ? 'music' : shape === 'circle' ? 'profile' : 'film'} eager={eager} />
        {st.watched && isVideo(item) && item.kind !== 'show' ? <span className="card-watched" aria-label="Watched"><Icon name="check" size={13} stroke={3} /></span> : null}
        {inProgress ? <span className="card-progress"><Progress value={st.progress} thin tone="white" label="Progress" /></span> : null}
        {episode ? <span className="card-tag">{episodeCode(episode)}</span> : null}
        {selecting ? <span className={cx('card-check', selected && 'is-on')} aria-hidden><Icon name="check" size={15} stroke={3} /></span> : null}
      </span>
      {caption ? (
        <span className="card-caption">
          <span className="card-title">{title}</span>
          {sub ? <span className="card-sub">{episode && showTitle ? item.title : sub}</span> : null}
        </span>
      ) : null}
    </>
  );

  const props = {
    ref,
    className: cx('card', `card-${shape}`, fill && 'is-fill', selected && 'is-selected', rank !== undefined && 'has-rank'),
    onMouseEnter: enter,
    onMouseLeave: leave,
    onFocus: () => setTint(item.tint),
    'aria-label': `${title}${sub ? `, ${episode && showTitle ? item.title + ', ' + episodeCode(episode) : sub}` : ''}${st.watched ? ', watched' : inProgress ? ', in progress' : ''}${selecting ? (selected ? ', selected' : ', not selected') : ''}`,
    onContextMenu: undefined,
  };

  return (
    <div className={cx('card-cell', `cell-${shape}`, fill && 'is-fill')} data-rank={rank}>
      {rank !== undefined ? <span className="card-rank" aria-hidden>{rank}</span> : null}
      {selecting ? (
        <button type="button" {...props} ref={ref as unknown as React.Ref<HTMLButtonElement>} aria-pressed={selected} onClick={() => selection.toggle(item.id)}>{inner}</button>
      ) : (
        <Link to={link.to as never} params={link.params as never} search={(link.search ?? {}) as never} {...props} onClick={onOpen}>{inner}</Link>
      )}
      {preview ? <PreviewCard item={item} anchor={preview} context={context} onClose={() => setPreview(null)} showTitle={showTitle} /> : null}
    </div>
  );
}

function PreviewCard({item, anchor, context, onClose, showTitle}: {item: Item; anchor: DOMRect; context: CardContext; onClose: () => void; showTitle?: string}) {
  const services = useServices();
  useDataVersion();
  const navigate = useNavigate();
  const session = useSession();
  const st = services.user.state(item.id);
  const ref = useRef<HTMLDivElement>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const width = Math.min(Math.max(anchor.width * 1.9, 340), 420, window.innerWidth - 24);
  const left = Math.max(12, Math.min(window.innerWidth - width - 12, anchor.left + anchor.width / 2 - width / 2));
  const top = Math.max(12, anchor.top - 24);
  const [pos, setPos] = useState({top, left});
  useLayoutEffect(() => {
    const h = ref.current?.offsetHeight ?? 0;
    const maxTop = window.innerHeight - h - 12;
    setPos({top: Math.max(12, Math.min(top, maxTop)), left});
  }, [top, left]);
  const close = useCallback(() => { if (!menuOpen) onClose(); }, [menuOpen, onClose]);
  useEffect(() => {
    const onScroll = () => onClose();
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('scroll', onScroll, {capture: true, passive: true});
    window.addEventListener('keydown', onKey);
    return () => { window.removeEventListener('scroll', onScroll, {capture: true}); window.removeEventListener('keydown', onKey); };
  }, [onClose]);
  const episode = item.kind === 'episode' ? (item as Episode) : null;
  const facts = episode ? [episodeCode(episode), `${episode.runtime} min`] : factsFor(item);
  const res = bestResolution(item);
  const inProgress = st.progress > 0 && !st.watched;
  const genres = item.genres.slice(0, 3);
  const link = hrefFor(item as never);
  const origin = `${anchor.left + anchor.width / 2 - pos.left}px ${anchor.top + anchor.height / 2 - pos.top}px`;
  const sx = anchor.width / width;
  const toggle = (patch: Partial<typeof st>, msg: string) => void services.user.set([item.id], patch).then(() => toast(msg));
  return createPortal(
    <div
      ref={ref}
      className="preview"
      style={{top: pos.top, left: pos.left, width, transformOrigin: origin, '--from-scale': sx, '--tint-c': item.tint} as React.CSSProperties}
      onMouseLeave={close}
      role="dialog"
      aria-label={`${item.title} preview`}
    >
      <button type="button" className="preview-art" onClick={() => { onClose(); void navigate({to: link as never}); }} aria-label={`Open ${item.title}`} tabIndex={-1}>
        <Art src={backdropFor(item)} tint={item.tint} ratio="16/9" eager />
        <span className="preview-art-scrim" />
        <span className="preview-title display">{episode && showTitle ? showTitle : item.kind === 'episode' ? item.title : item.title}</span>
        {inProgress ? <span className="preview-progress"><Progress value={st.progress} thin tone="white" label="Progress" /></span> : null}
      </button>
      <div className="preview-body">
        <div className="preview-actions">
          <Tooltip label={inProgress ? 'Resume' : 'Play'}>
            <button type="button" className="pv-btn pv-play" aria-label={inProgress ? `Resume ${item.title}` : `Play ${item.title}`} onClick={() => { onClose(); void startPlayback(services, item, {resume: true}); }}><Icon name="play" size={20} fill="currentColor" /></button>
          </Tooltip>
          {canWatchlist(item) ? (
            <Tooltip label={st.watchlist ? 'Remove from Watchlist' : 'Add to Watchlist'}>
              <button type="button" className={cx('pv-btn', st.watchlist && 'is-on')} aria-pressed={st.watchlist} aria-label="Watchlist" onClick={() => toggle({watchlist: !st.watchlist}, st.watchlist ? 'Removed from your Watchlist' : 'Added to your Watchlist')}><Icon name={st.watchlist ? 'check' : 'plus'} size={19} /></button>
            </Tooltip>
          ) : null}
          {isVideo(item) ? (
            <Tooltip label={st.watched ? 'Mark unwatched' : 'Mark watched'}>
              <button type="button" className={cx('pv-btn', st.watched && 'is-on')} aria-pressed={st.watched} aria-label="Watched" onClick={() => toggle({watched: !st.watched}, st.watched ? 'Marked unwatched' : 'Marked watched')}><Icon name="eye" size={18} /></button>
            </Tooltip>
          ) : null}
          {item.kind === 'movie' || item.kind === 'show' ? (
            <Tooltip label={st.liked === 'like' ? 'Liked' : 'Like'}>
              <button type="button" className={cx('pv-btn', st.liked === 'like' && 'is-on')} aria-pressed={st.liked === 'like'} aria-label="Like" onClick={() => toggle({liked: st.liked === 'like' ? null : 'like'}, st.liked === 'like' ? 'Rating removed' : 'Thanks. You’ll see more like this.')}><Icon name="like" size={18} fill={st.liked === 'like' ? 'currentColor' : 'none'} /></button>
            </Tooltip>
          ) : null}
          <span className="preview-spacer" />
          <Menu label="More actions" onOpenChange={setMenuOpen} trigger={<button type="button" className="pv-btn" aria-label={`More actions for ${item.title}`}><Icon name="more" size={19} /></button>} items={entryMenu(item, services, {navigate: to => { onClose(); void navigate({to: to as never}); }, manager: session.manager, context})} onSelect={() => { setMenuOpen(false); onClose(); }} />
          <Tooltip label="Details">
            <button type="button" className="pv-btn" aria-label={`Details for ${item.title}`} onClick={() => { onClose(); void navigate({to: link as never}); }}><Icon name="chevronDown" size={20} /></button>
          </Tooltip>
        </div>
        <div className="preview-meta">
          {res ? <Badge tone="accent">{res}</Badge> : null}
          {facts.map((f, i) => <span key={i}>{f}</span>)}
        </div>
        {episode ? <p className="preview-syn clamp-2">{item.synopsis}</p> : genres.length ? <div className="preview-genres">{genres.map((g, i) => <span key={g}>{i ? <i aria-hidden>•</i> : null}{g}</span>)}</div> : null}
        {inProgress ? <p className="preview-left">{formatLeft(item, st.position)}</p> : null}
      </div>
    </div>,
    document.body,
  );
}

function formatLeft(item: Item, position: number) {
  const total = 'runtime' in item ? (item as Movie).runtime * 60 : 0;
  if (!total) return '';
  const m = Math.max(1, Math.round((total - position) / 60));
  return m >= 60 ? `${Math.floor(m / 60)} hr ${m % 60} min left` : `${m} min left`;
}

/** Skeleton cards while a row or grid loads. */
export function CardSkeleton({shape = 'poster', fill}: {shape?: Shape; fill?: boolean}) {
  return (
    <div className={cx('card-cell', `cell-${shape}`, fill && 'is-fill')} aria-hidden>
      <span className={`card card-${shape}`}>
        <span className="card-frame"><span className="skeleton" style={{position: 'absolute', inset: 0, borderRadius: 'inherit'}} /></span>
        <span className="card-caption"><span className="skeleton" style={{height: 12, width: '80%', borderRadius: 4}} /><span className="skeleton" style={{height: 10, width: '45%', borderRadius: 4, marginTop: 6}} /></span>
      </span>
    </div>
  );
}

