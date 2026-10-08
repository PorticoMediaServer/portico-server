import React, {useState} from 'react';
import {Link} from '@tanstack/react-router';
import type {Credit, Entry, MediaVersion, Personal, Shape} from '@/data/types';
import {useActions} from '@/app/actions';
import {useClient} from '@/data/DataProvider';
import {useSession} from '@/app/session';
import {Artwork, Progress} from '@/ui/Bits';
import {Button, IconButton} from '@/ui/Button';
import {Dialog, Menu} from '@/ui/Overlay';
import {Icon} from '@/ui/Icon';
import {Shelf} from '@/ui/Shelf';
import {useGlowTint} from '@/ui/hooks';
import {cx} from '@/ui/cx';
import {remaining} from '@/app/format';
import {useDemo} from '@/app/demo';
import s from './Detail.module.css';

/** The shared frame of every title page: backdrop, poster column and the title block. */
export function DetailFrame({entry, backdrop, shape = 'poster', eyebrow, children, sections, posterSlot}: {entry: Entry; backdrop?: string; shape?: Shape; eyebrow?: React.ReactNode; children: React.ReactNode; sections?: React.ReactNode; posterSlot?: React.ReactNode}) {
  useGlowTint(entry.tint);
  return (
    <div className={s.page}>
      <div className={s.backdrop} style={{'--bg': backdrop ?? entry.backdrop ? `url(${backdrop ?? entry.backdrop})` : 'none'} as React.CSSProperties} aria-hidden />
      <div className={cx(s.hero, shape === 'square' && s.heroSquare, shape === 'circle' && s.heroCircle, shape === 'wide' && s.heroWide)}>
        <div className={s.posterCol}>
          {posterSlot ?? <Artwork src={shape === 'poster' ? entry.poster ?? entry.image : entry.image} title={entry.title} shape={shape} className={cx(s.poster, shape === 'circle' && s.posterRound)} eager tint={entry.tint} />}
        </div>
        <div className={s.titleBlock}>
          {eyebrow ? <div className={s.eyebrow}>{eyebrow}</div> : null}
          <h1 className={s.title}>{entry.title}</h1>
          {children}
        </div>
      </div>
      {sections ? <div className={s.sections}>{sections}</div> : null}
    </div>
  );
}

/** Facts separated by dots. `wrapLast` puts the last item (genres) on its own line on phones, so no line starts with a dot. */
export function MetaLine({items, wrapLast}: {items: (React.ReactNode | false | null | undefined | 0)[]; wrapLast?: boolean}) {
  const list = items.filter(x => x !== undefined && x !== null && x !== false && x !== '' && x !== 0);
  return <div className={s.meta}>{list.map((x, i) => <span key={i} className={wrapLast && i === list.length - 1 && i > 0 ? s.metaWrap : undefined}>{x}</span>)}</div>;
}
export const RatingPill = ({children}: {children: React.ReactNode}) => <span className={s.pill}>{children}</span>;

export function Ratings({audience, critic, mine}: {audience?: number; critic?: number; mine?: number}) {
  if (!audience && !critic && !mine) return null;
  return (
    <div className={s.ratings}>
      {audience ? <span className={s.rating} title="Audience rating"><Icon name="starFilled" size={16} />{audience.toFixed(1)}</span> : null}
      {critic ? <span className={cx(s.rating, s.critic)} title="Critics’ score">{critic}% critics</span> : null}
      {mine ? <span className={s.rating} title="Your rating">You: {'★'.repeat(mine)}</span> : null}
    </div>
  );
}

export function Overview({text, tagline}: {text?: string; tagline?: string}) {
  const [open, setOpen] = useState(false);
  if (!text) return null;
  const long = text.length > 320;
  return (
    <>
      {tagline ? <p className={s.tagline}>{tagline}</p> : null}
      <p className={cx(s.overview, long && !open && s.overviewClamp)}>{text}</p>
      {long ? <button type="button" className={s.more} onClick={() => setOpen(!open)} aria-expanded={open}>{open ? 'Less' : 'More'}</button> : null}
    </>
  );
}

export function Facts({rows}: {rows: [string, React.ReactNode][]}) {
  const list = rows.filter(([, v]) => v !== undefined && v !== null && v !== '' && !(Array.isArray(v) && !v.length));
  if (!list.length) return null;
  return <dl className={s.facts}>{list.map(([k, v]) => <div key={k} className={s.fact}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>;
}
export function People({list}: {list: Credit[]}) {
  return <>{list.map((c, i) => <React.Fragment key={c.personId + i}>{i ? ', ' : ''}<Link to={`/person/${c.personId}` as never}>{c.name}</Link></React.Fragment>)}</>;
}

/** Play or resume, then the personal toggles: Watchlist, watched, favorite, rate, and the ⋯ menu. */
export function ActionRow({entry, personal, onPlay, playLabel, extra, watchlist = true, watchedToggle = true}: {entry: Entry; personal: Personal; onPlay: (fromStart?: boolean) => void; playLabel?: string; extra?: React.ReactNode; watchlist?: boolean; watchedToggle?: boolean}) {
  const actions = useActions();
  const client = useClient();
  const p = client.personal.get(entry.id);
  const resume = p.progress > 0 && !p.watched && !!entry.duration;
  const e = {...entry, watched: p.watched, watchlisted: p.watchlisted, favorite: p.favorite};
  void personal;
  const [menu, setMenu] = useState(false);
  useDemo('actions', () => setMenu(true));
  return (
    <>
      <div className={s.buttons}>
        <Button variant="primary" size="lg" icon="play" label={playLabel ?? (resume ? 'Resume' : 'Play')} onClick={() => onPlay(false)} />
        {resume ? <Button variant="secondary" size="lg" icon="refresh" label="From the start" onClick={() => onPlay(true)} /> : null}
        {extra}
        {watchlist ? <IconButton className={s.actBtn} variant="secondary" size="lg" round icon={p.watchlisted ? 'savedFilled' : 'saved'} label={p.watchlisted ? 'On your Watchlist. Remove' : 'Add to Watchlist'} pressed={p.watchlisted} onClick={() => void actions.toggleWatchlist(e)} /> : null}
        {watchedToggle ? <IconButton className={s.actBtn} variant="secondary" size="lg" round icon="watched" label={p.watched ? 'Watched. Mark as unwatched' : 'Mark as watched'} pressed={p.watched} onClick={() => void actions.toggleWatched(e)} /> : null}
        <IconButton className={s.actBtn} variant="secondary" size="lg" round icon={p.favorite ? 'heartFilled' : 'heart'} label={p.favorite ? 'In Favorites. Remove' : 'Add to Favorites'} pressed={p.favorite} onClick={() => void actions.toggleFavorite(e)} />
        <IconButton className={s.actBtn} variant="secondary" size="lg" round icon={p.rating ? 'starFilled' : 'star'} label={p.rating ? `Your rating: ${p.rating} of 5. Change` : 'Rate'} pressed={!!p.rating} onClick={() => actions.dialog({kind: 'rate', entry: e})} />
        <Menu open={menu} onOpenChange={setMenu} label={`More for ${entry.title}`} items={actions.items(e).filter(i => !['play', 'fromStart', 'watched', 'watchlist', 'favorite', 'rate', 'details'].includes(i.id))} trigger={<IconButton className={s.actBtn} variant="secondary" size="lg" round icon="more" label="More actions" />} />
      </div>
      {resume && entry.duration ? <div className={s.progressLine}><Progress value={p.progress / entry.duration} /><span>{remaining(entry.duration, p.progress / entry.duration)}</span></div> : null}
    </>
  );
}

export function CastRow({title = 'Cast and crew', people}: {title?: string; people: Credit[]}) {
  if (!people.length) return null;
  const entries: Entry[] = people.map((c, i) => ({id: c.personId, kind: 'person', title: c.name, subtitle: c.role === 'cast' ? c.character : c.job, image: c.image, shape: 'circle', playable: false, caption: String(i)}));
  const unique = entries.filter((e, i) => entries.findIndex(x => x.id === e.id) === i);
  return <Shelf title={title} entries={unique} shape="circle" />;
}

export function MediaInfoDialog({open, onOpenChange, versions, title}: {open: boolean; onOpenChange: (o: boolean) => void; versions: MediaVersion[]; title: string}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="Media information" description={title} width={560}>
      {versions.map(v => (
        <div key={v.id} className={s.infoGroup}>
          <div className="t-label" style={{marginBottom: 6}}>{v.label}{v.edition ? ` · ${v.edition}` : ''}</div>
          <table className={s.infoTable}><tbody>
            {([['Resolution', v.resolution], ['Video', v.video], ['Audio', v.audio], ['HDR', v.hdr ?? (v.video.includes('Dolby Vision') ? 'Dolby Vision, HDR10' : 'None')], ['Container', v.container], ['Bitrate', v.bitrate], ['File size', v.size], ['File', `/media/Movies/${title.replace(/[^\w ]/g, '')}/${title.replace(/[^\w ]/g, '')}.${v.container.toLowerCase()}`]] as [string, string][]).map(([k, val]) => <tr key={k}><th>{k}</th><td>{val}</td></tr>)}
          </tbody></table>
        </div>
      ))}
    </Dialog>
  );
}

/** Admin shortcut shown on title pages. */
export function useIsAdmin() { return useSession().isAdmin; }
