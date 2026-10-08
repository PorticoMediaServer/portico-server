import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { Info, Play } from 'lucide-react';
import type { HeroItem } from '../data/types';
import { useToggleWatchlist, useWatchlist } from '../services/queries';
import { useIsMobile } from '../lib/hooks';
import { Button, cls, Progress } from './ui';
import { WatchlistButton } from './media';
import { toast } from './overlay';

const ADVANCE_MS = 9000;

export function Hero({ items, compact }: { items: HeroItem[]; compact?: boolean }) {
  const [i, setI] = useState(0);
  const [paused, setPaused] = useState(false);
  const mobile = useIsMobile();
  const navigate = useNavigate();
  const watchlist = useWatchlist();
  const toggle = useToggleWatchlist();
  const track = useRef<HTMLDivElement>(null);
  const n = items.length;

  const go = useCallback((k: number) => setI(((k % n) + n) % n), [n]);

  useEffect(() => {
    if (paused || n < 2 || mobile) return;
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (reduce) return;
    const t = setTimeout(() => go(i + 1), ADVANCE_MS);
    return () => clearTimeout(t);
  }, [i, paused, n, go, mobile]);

  // phones: swipeable pages via scroll-snap; keep the dot in sync
  const onScroll = () => {
    const el = track.current;
    if (!el) return;
    setI(Math.round(el.scrollLeft / el.clientWidth));
  };

  if (!n) return null;
  const inList = (id: string) => (watchlist.data ?? []).some((w) => w.id === id);
  const toggleList = (h: HeroItem) =>
    toggle.mutate(h.id, {
      onSuccess: (on) => toast(on ? `Added ${h.title} to your Watchlist` : `Removed ${h.title} from your Watchlist`),
    });

  if (mobile)
    return (
      <section className="hero-m" aria-roledescription="carousel" aria-label="Featured">
        <div className="hero-m-track no-scrollbar" ref={track} onScroll={onScroll}>
          {items.map((h, k) => (
            <article key={h.id} className="hero-m-card" aria-roledescription="slide" aria-label={`${k + 1} of ${n}: ${h.title}`}>
              <Link to={h.href} className="hero-m-art" viewTransition>
                <img src={h.backdrop} alt="" loading={k === 0 ? 'eager' : 'lazy'} />
                <div className="hero-m-shade" style={{ ['--tint' as string]: h.palette.base }} />
              </Link>
              <div className="hero-m-content">
                <div className="hero-eyebrow">{h.eyebrow}</div>
                {h.logo ? <img className="hero-m-logo" src={h.logo} alt={h.title} /> : <h2 className="hero-m-title">{h.title}</h2>}
                <div className="hero-m-meta">{h.meta.slice(0, 3).join(' · ')}</div>
                <div className="hero-m-actions">
                  <Button variant="primary" size="lg" icon={<Play fill="currentColor" />} onClick={() => navigate(h.playHref)}>
                    {h.progress ? 'Resume' : 'Play'}
                  </Button>
                  <WatchlistButton on={inList(h.id)} onToggle={() => toggleList(h)} />
                </div>
              </div>
            </article>
          ))}
        </div>
        <div className="hero-m-dots" aria-hidden="true">
          {items.map((h, k) => (
            <i key={h.id} className={cls(k === i && 'on')} />
          ))}
        </div>
      </section>
    );

  return (
    <section
      className={cls('hero', compact && 'compact')}
      aria-roledescription="carousel"
      aria-label="Featured"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      {items.map((h, k) => (
        <div key={h.id} className={cls('hero-slide', k === i && 'on')} aria-hidden={k !== i} style={{ ['--tint' as string]: h.palette.base }}>
          <div className="hero-art">
            <img src={h.backdrop} alt="" loading={k === 0 ? 'eager' : 'lazy'} fetchPriority={k === 0 ? 'high' : 'auto'} />
          </div>
          <div className="hero-shade" />
        </div>
      ))}
      {items.map((h, k) =>
        k !== i ? null : (
          <div key={h.id} className="hero-content" aria-live="polite">
            <div className="hero-eyebrow">{h.eyebrow}</div>
            {h.logo ? <img className="hero-logo" src={h.logo} alt={h.title} /> : <h2 className="hero-title">{h.title}</h2>}
            <div className="hero-meta">
              {h.meta.map((m) => (
                <span key={m}>{m}</span>
              ))}
            </div>
            <div className="hero-badges">
              {h.badges.map((b) => (
                <span key={b} className="badge">
                  {b}
                </span>
              ))}
            </div>
            <p className="hero-summary clamp-3">{h.summary}</p>
            <div className="hero-actions">
              <Button variant="primary" size="lg" icon={<Play fill="currentColor" />} onClick={() => navigate(h.playHref)}>
                {h.progress ? 'Resume' : h.kind === 'show' && h.resumeLabel ? h.resumeLabel.replace('Play ', 'Play ') : 'Play'}
              </Button>
              <WatchlistButton on={inList(h.id)} onToggle={() => toggleList(h)} />
              <Button variant="glass" size="lg" icon={<Info />} to={h.href}>
                Details
              </Button>
            </div>
            {h.progress !== undefined && (
              <div className="hero-progress">
                <Progress value={h.progress} />
                <span>{h.resumeLabel}</span>
              </div>
            )}
          </div>
        ),
      )}
      {n > 1 && (
        <div className="hero-pager glass" role="tablist" aria-label="Featured titles">
          {items.map((h, k) => (
            <button key={h.id} role="tab" aria-selected={k === i} aria-label={h.title} className={cls('hero-dot', k === i && 'on', k === i && !paused && 'running')} onClick={() => go(k)}>
              <i style={{ animationDuration: `${ADVANCE_MS}ms` }} />
            </button>
          ))}
        </div>
      )}
    </section>
  );
}

export function HeroSkeleton() {
  const mobile = useIsMobile();
  if (mobile)
    return (
      <div className="hero-m" aria-hidden="true">
        <div className="hero-m-track">
          <div className="hero-m-card">
            <div className="skel" style={{ position: 'absolute', inset: 0, borderRadius: 28 }} />
          </div>
        </div>
      </div>
    );
  return (
    <div className="hero" aria-hidden="true">
      <div className="hero-content">
        <div className="skel" style={{ width: 140, height: 12 }} />
        <div className="skel" style={{ width: 380, height: 88, marginTop: 18, borderRadius: 14 }} />
        <div className="skel" style={{ width: 300, height: 14, marginTop: 20 }} />
        <div className="skel" style={{ width: 460, height: 14, marginTop: 14 }} />
        <div className="skel" style={{ width: 420, height: 14, marginTop: 8 }} />
        <div style={{ display: 'flex', gap: 12, marginTop: 26 }}>
          <div className="skel" style={{ width: 140, height: 50, borderRadius: 25 }} />
          <div className="skel" style={{ width: 50, height: 50, borderRadius: 25 }} />
        </div>
      </div>
    </div>
  );
}
