import {useEffect, useMemo, useRef, useState} from 'react';
import {useQuery} from '@/data/DataProvider';
import type {Entry, HomeRow} from '@/data/types';
import {useActions} from '@/app/actions';
import {useSession} from '@/app/session';
import {usePrefs} from '@/app/prefs';
import {hrefFor} from '@/app/links';
import {formatDuration} from '@/app/format';
import {Button, IconButton} from '@/ui/Button';
import {EmptyState, ErrorState} from '@/ui/Bits';
import {Shelf} from '@/ui/Shelf';
import {useGlowTint, useTitle} from '@/ui/hooks';
import {Meta} from '../common';
import {CustomizeHome} from './Customize';
import {useDemo} from '@/app/demo';
import s from './Home.module.css';

/**
 * Home: a rotating featured billboard, then the profile's rows (Continue
 * Watching, Up Next, recently added, recommendations) in the order the
 * profile chose under Customize Home.
 */
export default function Home() {
  useTitle('Home');
  const doc = useQuery(c => c.home.document(), [], ['home', 'personal', 'session']);
  const libs = useQuery(c => c.libraries.list(), [], ['libraries']);
  const {prefs} = usePrefs();
  const session = useSession();
  const [customize, setCustomize] = useState(false);
  useDemo('customize', () => setCustomize(true), !!doc.data);
  if (doc.error && !doc.data) return <ErrorState onRetry={doc.refresh} />;
  if (libs.data && libs.data.length === 0) return <EmptyState icon="folder" title="No libraries yet" body={session.isAdmin ? 'Add a folder of movies, shows or music and Portico organizes it here.' : 'The owner of this server hasn’t shared any libraries with you yet.'} actions={session.isAdmin ? [{label: 'Add a library', icon: 'plus', to: '/server/libraries'}] : undefined} />;
  const rows = doc.data?.rows;
  const showHero = prefs.homeHero && (doc.data?.featured.length ?? 1) > 0;
  return (
    <div>
      {showHero ? <Hero featured={doc.data?.featured} /> : null}
      <div className={`${s.rows} ${showHero ? '' : s.rowsNoHero}`}>
        {rows ? rows.map((row, i) => <HomeShelf key={row.id} row={row} eager={i < 2} />) : [0, 1, 2].map(i => <Shelf key={i} title="" />)}
        {rows && rows.length === 0 ? <EmptyState icon="sparkle" title="Home is empty" body="Every row is hidden. Choose which rows to show and in what order." actions={[{label: 'Customize Home', icon: 'sliders', onClick: () => setCustomize(true)}]} /> : null}
      </div>
      {rows ? <div className={s.foot}><Button variant="ghost" icon="sliders" label="Customize Home" onClick={() => setCustomize(true)} /></div> : null}
      <CustomizeHome open={customize} onOpenChange={setCustomize} />
    </div>
  );
}

function HomeShelf({row, eager}: {row: HomeRow; eager?: boolean}) {
  const href = row.total > row.entries.length || row.kind === 'recent' || row.kind === 'picks' || row.kind === 'watchlist' ? `/home/rows/${encodeURIComponent(row.id)}` : undefined;
  return <Shelf title={row.title} entries={row.entries} shape={row.shape} href={href} total={row.total} reason={row.reason} origin={row.kind === 'continue' ? 'continue' : row.kind === 'picks' || row.kind === 'because' ? 'recommendation' : undefined} eager={eager} />;
}

function Hero({featured}: {featured?: Entry[]}) {
  const [i, setI] = useState(0);
  const [paused, setPaused] = useState(false);
  const actions = useActions();
  const list = useMemo(() => featured?.slice(0, 6) ?? [], [featured]);
  const current = list[i];
  const hero = useRef<HTMLElement>(null);
  useGlowTint(current?.tint);
  useEffect(() => {
    if (paused || list.length < 2) return;
    const t = setTimeout(() => setI(x => (x + 1) % list.length), 9000);
    return () => clearTimeout(t);
  }, [i, paused, list.length]);
  if (!featured) return <section className={s.hero} aria-busy="true"><div className="skeleton" style={{position: 'absolute', inset: 0, borderRadius: 0, opacity: 0.5}} /></section>;
  return (
    <section ref={hero} className={s.hero} aria-roledescription="carousel" aria-label="Featured" onMouseEnter={() => setPaused(true)} onMouseLeave={() => setPaused(false)} onFocus={() => setPaused(true)} onBlur={() => setPaused(false)}>
      {list.map((e, n) => (
        <div key={e.id} className={s.slide} data-on={n === i} aria-hidden={n !== i} aria-roledescription="slide" aria-label={`${n + 1} of ${list.length}: ${e.title}`}>
          <div className={s.backdrop} style={{backgroundImage: e.backdrop ? `url(${e.backdrop})` : undefined}} />
          <div className={s.scrim} />
          <div className={s.body}>
            <span className={s.eyebrow}>{e.kind === 'show' ? 'Series' : 'Film'} · {e.recommendation ? 'Picked for you' : 'In your library'}</span>
            <h2 className={s.title}>{e.title}</h2>
            <Meta items={[e.year, e.rating, e.duration ? formatDuration(e.duration) : e.caption, e.genres?.slice(0, 2).join(', ')]} />
            {e.overview ? <p className={`${s.overview} clamp-3`}>{e.overview}</p> : null}
            <div className={s.buttons}>
              <Button variant="primary" size="lg" icon="play" label={e.progress ? 'Resume' : 'Play'} onClick={() => void actions.play(e)} tabIndex={n === i ? 0 : -1} />
              <Button variant="secondary" size="lg" icon="info" label="More info" to={hrefFor(e) ?? '/'} tabIndex={n === i ? 0 : -1} />
              <IconButton variant="secondary" size="lg" icon={e.watchlisted ? 'savedFilled' : 'saved'} label={e.watchlisted ? 'Remove from Watchlist' : 'Add to Watchlist'} onClick={() => void actions.toggleWatchlist(e)} tabIndex={n === i ? 0 : -1} />
            </div>
          </div>
        </div>
      ))}
      {list.length > 1 ? (
        <div className={s.dots} role="group" aria-label="Choose featured title">
          {list.map((e, n) => <button key={e.id} type="button" className={s.dotBtn} aria-current={n === i} aria-label={`Show ${e.title}`} onClick={() => setI(n)} />)}
        </div>
      ) : null}
    </section>
  );
}
