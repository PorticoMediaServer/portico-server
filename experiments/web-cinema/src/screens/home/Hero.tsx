import {useEffect, useRef, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useDataVersion, useServices} from '../../data/hooks';
import type {Item} from '../../data/types';
import {setTint} from '../../app/tint';
import {reducedMotion} from '../../app/prefs';
import {Art} from '../../ui/Media';
import {Button, IconButton} from '../../ui/Button';
import {Badge} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {cx} from '../../ui/cx';
import {backdropFor, bestResolution, canWatchlist, factsFor, hrefFor, formatRemaining} from '../shared/items';
import {startPlayback} from '../shared/playback';
import {toast} from '../../ui/Feedback';

const SLIDE_MS = 9000;

/**
 * The marquee: full-bleed artwork that runs under the rail, a big editorial title and the two
 * things you most want (play, or find out more). Advances on its own, pauses while you are
 * reading it (hover or focus), and never moves with reduced motion.
 */
export function Hero({items, eyebrow = 'Featured'}: {items: Item[]; eyebrow?: string}) {
  const services = useServices();
  useDataVersion();
  const navigate = useNavigate();
  const [index, setIndex] = useState(0);
  const [paused, setPaused] = useState(false);
  const [tick, setTick] = useState(0);
  const root = useRef<HTMLElement>(null);
  const item = items[index % items.length];
  useEffect(() => { if (item) setTint(item.tint, 0); }, [item]);
  useEffect(() => {
    if (paused || items.length < 2 || reducedMotion()) return;
    const t = setTimeout(() => setIndex(i => (i + 1) % items.length), SLIDE_MS);
    return () => clearTimeout(t);
  }, [index, paused, items.length, tick]);
  if (!item) return null;
  const st = services.user.state(item.id);
  const inProgress = st.progress > 0 && !st.watched;
  const res = bestResolution(item);
  const go = (i: number) => { setIndex((i + items.length) % items.length); setTick(t => t + 1); };
  return (
    <section
      ref={root}
      className={cx('hero', paused && 'is-paused')}
      aria-roledescription="carousel"
      aria-label="Featured titles"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={e => { if (!root.current?.contains(e.relatedTarget as Node)) setPaused(false); }}
      onKeyDown={e => { if (e.key === 'ArrowRight' && e.target === root.current) go(index + 1); if (e.key === 'ArrowLeft' && e.target === root.current) go(index - 1); }}
    >
      <div className="hero-media" aria-hidden>
        {items.map((it, i) => (
          <div key={it.id} className={cx('hero-slide', i === index && 'is-on')}>
            <Art src={backdropFor(it)} tint={it.tint} eager={i === 0 || i === (index + 1) % items.length} position="center 30%" />
          </div>
        ))}
        <div className="hero-scrim" />
      </div>
      <div className="hero-content" key={item.id} aria-roledescription="slide" aria-label={`${index + 1} of ${items.length}: ${item.title}`}>
        <div className="eyebrow hero-eyebrow"><span className="hero-dot" />{eyebrow}{item.kind === 'show' ? ' · Series' : item.kind === 'movie' ? ' · Film' : ''}</div>
        <h2 className="hero-title display">{item.title}</h2>
        <div className="hero-facts">
          {res ? <Badge tone="solid">{res}</Badge> : null}
          {factsFor(item).map((f, i) => <span key={i}>{f}</span>)}
          {item.genres.slice(0, 2).map(g => <span key={g}>{g}</span>)}
        </div>
        {item.synopsis ? <p className="hero-syn clamp-3">{item.synopsis}</p> : null}
        <div className="hero-actions">
          <Button variant="primary" size="lg" icon="play" label={inProgress ? 'Resume' : 'Play'} onClick={() => void startPlayback(services, item, {resume: true})} />
          <Button variant="glass" size="lg" icon="info" label="More info" onClick={() => void navigate({to: hrefFor(item as never) as never})} />
          {canWatchlist(item) ? <IconButton size="lg" icon={st.watchlist ? 'check' : 'plus'} label={st.watchlist ? 'Remove from Watchlist' : 'Add to Watchlist'} active={st.watchlist} onClick={() => void services.user.set([item.id], {watchlist: !st.watchlist}).then(() => toast(st.watchlist ? 'Removed from your Watchlist' : 'Added to your Watchlist'))} /> : null}
          {inProgress ? <span className="hero-left">{formatRemaining(('runtime' in item ? (item as {runtime: number}).runtime * 60 : 0) - st.position)}</span> : null}
        </div>
      </div>
      {items.length > 1 ? (
        <div className="hero-pager">
          <button type="button" className="hero-step" onClick={() => go(index - 1)} aria-label="Previous featured title"><Icon name="chevronLeft" size={20} /></button>
          <div className="hero-dots" role="tablist" aria-label="Choose a featured title">
            {items.map((it, i) => (
              <button key={it.id} type="button" role="tab" aria-selected={i === index} aria-label={it.title} className={cx('hero-pip', i === index && 'is-on')} onClick={() => go(i)}>
                <span style={{animationDuration: `${SLIDE_MS}ms`}} />
              </button>
            ))}
          </div>
          <button type="button" className="hero-step" onClick={() => go(index + 1)} aria-label="Next featured title"><Icon name="chevronRight" size={20} /></button>
        </div>
      ) : null}
    </section>
  );
}
