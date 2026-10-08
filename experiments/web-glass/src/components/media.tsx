import { memo, useEffect, useRef, useState, type ReactNode } from 'react';
import { Link } from 'react-router';
import { Check, ChevronLeft, ChevronRight, Play, Plus, Radio } from 'lucide-react';
import type { LibraryCard, Palette, Shelf as ShelfT, ShelfItem } from '../data/types';
import { cls, Progress } from './ui';

// ---------- Artwork ----------

export const Artwork = memo(function Artwork({
  src,
  alt = '',
  ratio,
  palette,
  className,
  eager,
  fit = 'cover',
  children,
  sizes,
}: {
  src?: string;
  alt?: string;
  ratio?: string;
  palette?: Palette;
  className?: string;
  eager?: boolean;
  fit?: 'cover' | 'contain';
  children?: ReactNode;
  sizes?: string;
}) {
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState(false);
  const ref = useRef<HTMLImageElement>(null);
  useEffect(() => {
    if (ref.current?.complete && ref.current.naturalWidth) setLoaded(true);
  }, [src]);
  return (
    <div
      className={cls('art', loaded && 'loaded', className)}
      style={{ aspectRatio: ratio, ['--art-base' as string]: palette?.base, ['--art-accent' as string]: palette?.accent }}
    >
      {src && !failed && (
        <img
          ref={ref}
          className="art-img"
          src={src}
          alt={alt}
          loading={eager ? 'eager' : 'lazy'}
          decoding="async"
          sizes={sizes}
          draggable={false}
          style={{ objectFit: fit }}
          onLoad={() => setLoaded(true)}
          onError={() => setFailed(true)}
        />
      )}
      {(failed || !src) && <div className="art-fallback">{alt?.slice(0, 1)}</div>}
      {children}
    </div>
  );
});

// ---------- Cards ----------

type Shape = 'poster' | 'landscape' | 'square' | 'circle';

export function MediaCard({
  item,
  shape,
  showText = true,
  eager,
  size,
  overlay,
}: {
  item: ShelfItem | LibraryCard;
  shape: Shape;
  showText?: boolean;
  eager?: boolean;
  size?: 'hero';
  overlay?: ReactNode;
}) {
  const ratio = shape === 'poster' ? '2 / 3' : shape === 'landscape' ? '16 / 9' : '1 / 1';
  const card = item as LibraryCard;
  return (
    <Link to={item.href} className={cls('card', `card-${shape}`, size && `card-${size}`)} viewTransition draggable={false}>
      <Artwork src={item.image} alt={item.title} ratio={ratio} palette={item.palette} eager={eager} className="card-art">
        {item.progress !== undefined && item.progress > 0 && <Progress value={item.progress} className="card-progress" />}
        {card.watched && !item.progress && shape === 'poster' && (
          <span className="card-watched" aria-label="Watched">
            <Check />
          </span>
        )}
        {!!card.unwatchedCount && !card.watched && shape === 'poster' && <span className="card-count">{card.unwatchedCount}</span>}
        {item.badge && <span className={cls('card-badge', item.badge === 'LIVE' && 'live', item.badge === 'New' && 'new')}>{item.badge}</span>}
        {overlay}
      </Artwork>
      {showText && (
        <div className="card-text">
          <div className="card-title ellipsis">{item.title}</div>
          {item.subtitle && <div className="card-sub ellipsis">{item.subtitle}</div>}
        </div>
      )}
    </Link>
  );
}

function ContinueCard({ item, eager }: { item: ShelfItem; eager?: boolean }) {
  return (
    <div className="card card-continue">
      <Link to={item.href} className="continue-hit" viewTransition aria-label={`Play ${item.title}`}>
        <Artwork src={item.image} alt="" ratio="16 / 9" palette={item.palette} eager={eager} className="card-art">
          <div className="continue-shade" />
          {item.logo ? <img className="continue-logo" src={item.logo} alt={item.title} loading="lazy" /> : <div className="continue-name">{item.title}</div>}
          <span className="continue-play glass glass-clear" aria-hidden="true">
            <Play fill="currentColor" />
          </span>
          {item.badge && <span className="card-badge new">{item.badge}</span>}
          {item.progress !== undefined && <Progress value={item.progress} className="card-progress" />}
        </Artwork>
      </Link>
      <Link to={item.meta ?? item.href} className="card-text" viewTransition>
        <div className="card-title ellipsis">{item.kind === 'episode' ? item.title : item.title}</div>
        <div className="card-sub ellipsis">{item.subtitle}</div>
      </Link>
    </div>
  );
}

function LiveCard({ item }: { item: ShelfItem }) {
  return (
    <Link to={item.href} className="card card-live" viewTransition>
      <Artwork src={item.image} alt="" ratio="16 / 9" className="card-art">
        <div className="continue-shade" />
        <span className="live-chip">
          <Radio />
          {item.badge === 'LIVE' ? 'LIVE' : 'ON NOW'}
        </span>
        {item.progress !== undefined && <Progress value={item.progress} accent className="card-progress" />}
      </Artwork>
      <div className="card-text">
        <div className="card-title ellipsis">{item.title}</div>
        <div className="card-sub ellipsis">{item.subtitle}</div>
      </div>
    </Link>
  );
}

function Top10Card({ item, rank }: { item: ShelfItem; rank: number }) {
  return (
    <Link to={item.href} className="card card-top10" viewTransition>
      <span className="top10-num" aria-hidden="true">
        {rank}
      </span>
      <Artwork src={item.image} alt={item.title} ratio="2 / 3" palette={item.palette} className="card-art" />
    </Link>
  );
}

function HeroCard({ item }: { item: ShelfItem }) {
  return (
    <Link to={item.href} className="card card-herocard" viewTransition>
      <Artwork src={item.image} alt="" ratio="16 / 8" palette={item.palette} className="card-art">
        <div className="herocard-shade" style={{ ['--tint' as string]: item.palette?.base }} />
        <div className="herocard-text">
          <div className="herocard-title">{item.title}</div>
          <div className="herocard-sub">{item.subtitle}</div>
        </div>
      </Artwork>
    </Link>
  );
}

// ---------- Shelf ----------

export function Shelf({ shelf, eager }: { shelf: ShelfT; eager?: boolean }) {
  const track = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({ start: true, end: false });
  const update = () => {
    const el = track.current;
    if (!el) return;
    setEdges({ start: el.scrollLeft < 8, end: el.scrollLeft + el.clientWidth >= el.scrollWidth - 8 });
  };
  useEffect(() => {
    update();
    const el = track.current;
    if (!el) return;
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, [shelf.items.length]);
  const page = (dir: 1 | -1) => {
    const el = track.current;
    if (!el) return;
    el.scrollBy({ left: dir * (el.clientWidth - 120), behavior: 'smooth' });
  };
  const style = shelf.style;
  return (
    <section className={cls('shelf', `shelf-${style}`)} aria-label={shelf.title}>
      <header className="shelf-head">
        {shelf.href ? (
          <Link to={shelf.href} className="shelf-title linkish" viewTransition>
            {shelf.title}
            <ChevronRight />
          </Link>
        ) : (
          <h2 className="shelf-title">{shelf.title}</h2>
        )}
        {shelf.subtitle && <span className="shelf-sub">{shelf.subtitle}</span>}
      </header>
      <div className="shelf-viewport">
        <div className="shelf-track no-scrollbar" ref={track} onScroll={update}>
          {shelf.items.map((it, i) => {
            const e = eager && i < 8;
            switch (style) {
              case 'continue':
                return <ContinueCard key={it.id} item={it} eager={e} />;
              case 'live':
                return <LiveCard key={it.id} item={it} />;
              case 'top10':
                return <Top10Card key={it.id} item={it} rank={i + 1} />;
              case 'hero-cards':
                return <HeroCard key={it.id} item={it} />;
              case 'landscape':
                return <MediaCard key={it.id} item={it} shape="landscape" eager={e} />;
              case 'square':
                return <MediaCard key={it.id} item={it} shape="square" eager={e} />;
              case 'circle':
                return <MediaCard key={it.id} item={it} shape="circle" eager={e} />;
              default:
                return <MediaCard key={it.id} item={it} shape="poster" eager={e} />;
            }
          })}
        </div>
        <button className={cls('shelf-nav prev glass', edges.start && 'hidden')} onClick={() => page(-1)} aria-label="Scroll back" tabIndex={-1}>
          <ChevronLeft />
        </button>
        <button className={cls('shelf-nav next glass', edges.end && 'hidden')} onClick={() => page(1)} aria-label="Scroll forward" tabIndex={-1}>
          <ChevronRight />
        </button>
      </div>
    </section>
  );
}

export function ShelfSkeleton({ style = 'poster', title = true }: { style?: 'poster' | 'landscape' | 'square'; title?: boolean }) {
  const w = style === 'poster' ? 'var(--poster-w)' : style === 'landscape' ? 'var(--landscape-w)' : 'var(--square-w)';
  const ratio = style === 'poster' ? '2 / 3' : style === 'landscape' ? '16 / 9' : '1 / 1';
  return (
    <div className="shelf" aria-hidden="true">
      {title && (
        <div className="shelf-head">
          <div className="skel" style={{ width: 180, height: 22 }} />
        </div>
      )}
      <div className="shelf-track no-scrollbar" style={{ overflow: 'hidden' }}>
        {Array.from({ length: 9 }, (_, i) => (
          <div key={i} style={{ width: w, flex: 'none' }}>
            <div className="skel" style={{ aspectRatio: ratio, borderRadius: 12 }} />
            <div className="skel" style={{ height: 12, width: '70%', marginTop: 10 }} />
          </div>
        ))}
      </div>
    </div>
  );
}

// ---------- Rating / meta helpers ----------

export function MetaLine({ items }: { items: (ReactNode | false | undefined | null)[] }) {
  const xs = items.filter(Boolean);
  return (
    <div className="meta-line">
      {xs.map((x, i) => (
        <span key={i}>{x}</span>
      ))}
    </div>
  );
}

export function Score({ critic, audience }: { critic?: number; audience?: number }) {
  return (
    <span className="score">
      {critic !== undefined && (
        <span title="Critics score" className={cls('score-item', critic >= 60 ? 'fresh' : 'rotten')}>
          <span className="score-dot" />
          {critic}%
        </span>
      )}
      {audience !== undefined && (
        <span title="Audience score" className="score-item aud">
          <span className="score-dot" />
          {audience}%
        </span>
      )}
    </span>
  );
}

export function WatchlistButton({ on, onToggle, size = 'lg' }: { on: boolean; onToggle: () => void; size?: 'md' | 'lg' }) {
  return (
    <button className={cls('icon-btn glass', size === 'lg' && 'lg', on && 'active')} onClick={onToggle} aria-label={on ? 'Remove from Watchlist' : 'Add to Watchlist'} title={on ? 'In your Watchlist' : 'Add to Watchlist'} aria-pressed={on}>
      {on ? <Check /> : <Plus />}
    </button>
  );
}
