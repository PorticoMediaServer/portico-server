import React, {useEffect, useState} from 'react';
import {Link} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Credit, Extra, Item, MediaVersion, Movie, Track} from '../../data/types';
import {setTint} from '../../app/tint';
import {player} from '../../app/player';
import {Art} from '../../ui/Media';
import {Button, IconButton} from '../../ui/Button';
import {Badge, Skeleton, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Menu, Popover} from '../../ui/Overlay';
import {KeyValue} from '../../ui/Layout';
import {cx} from '../../ui/cx';
import {Shelf} from '../shared/Shelf';
import {MediaCard} from '../shared/Cards';
import {useEntryMenu, actions} from '../shared/actions';
import {canWatchlist, formatClock, formatRemaining, isVideo} from '../shared/items';
import {startPlayback} from '../shared/playback';

/**
 * The stage behind every title page: the backdrop, full-bleed and running under the rail, fading
 * into the canvas so the page scrolls up over it. The glow takes on the title's colour.
 */
export function Stage({art, tint, children, className, blur, position}: {art?: string; tint: string; children: React.ReactNode; className?: string; blur?: boolean; position?: string}) {
  useEffect(() => { setTint(tint, 0); }, [tint]);
  return (
    <section className={cx('stage', blur && 'is-blur', className)}>
      <div className="stage-media" aria-hidden>
        {art ? <Art src={art} tint={tint} eager position={position ?? 'center 25%'} /> : null}
        <div className="stage-scrim" style={{'--c': tint} as React.CSSProperties} />
      </div>
      <div className="stage-content">{children}</div>
    </section>
  );
}

export function Facts({items, badges}: {items: (string | number | null | undefined | false)[]; badges?: React.ReactNode}) {
  const list = items.filter(Boolean) as (string | number)[];
  return (
    <div className="facts">
      {list.map((f, i) => <span key={i} className="fact">{f}</span>)}
      {badges}
    </div>
  );
}

export function ReadMore({text, lines = 3, className}: {text?: string; lines?: number; className?: string}) {
  const [open, setOpen] = useState(false);
  const long = (text?.length ?? 0) > lines * 110;
  if (!text) return null;
  return (
    <div className={cx('readmore', className)}>
      <p className={open || !long ? undefined : `clamp-${lines}`}>{text}</p>
      {long ? <button type="button" className="readmore-btn" onClick={() => setOpen(o => !o)} aria-expanded={open}>{open ? 'Show less' : 'Read more'}</button> : null}
    </div>
  );
}

/** Play or resume, then the quick toggles and everything else in More. */
export function ActionBar({item, playLabel, onPlay, trailer, extra, remaining}: {item: Item; playLabel?: string; onPlay?: () => void; trailer?: boolean; extra?: React.ReactNode; remaining?: number}) {
  const services = useServices();
  useDataVersion();
  const st = services.user.state(item.id);
  const menu = useEntryMenu(item, 'detail');
  const inProgress = st.progress > 0 && !st.watched;
  const set = (patch: Parameters<typeof services.user.set>[1], msg: string) => void services.user.set([item.id], patch).then(() => toast(msg));
  return (
    <div className="action-bar">
      <div className="action-primary">
        <Button variant="primary" size="lg" icon="play" label={playLabel ?? (inProgress ? 'Resume' : 'Play')} onClick={onPlay ?? (() => void startPlayback(services, item, {resume: true}))} />
        {inProgress && !playLabel ? (
          <span className="resume-meta">
            <span className="resume-bar"><span style={{width: `${st.progress * 100}%`}} /></span>
            <span>{formatRemaining(remaining ?? 0)}</span>
          </span>
        ) : null}
      </div>
      <div className="action-secondary">
        {inProgress && isVideo(item) ? <IconButton icon="rewind" label="Play from beginning" onClick={() => void startPlayback(services, item, {resume: false})} /> : null}
        {trailer ? <Button variant="glass" icon="trailer" label="Trailer" onClick={() => player.play(item.id, {kind: 'video', from: 0})} /> : null}
        {canWatchlist(item) ? <IconButton icon={st.watchlist ? 'check' : 'plus'} label={st.watchlist ? 'Remove from Watchlist' : 'Add to Watchlist'} active={st.watchlist} onClick={() => set({watchlist: !st.watchlist}, st.watchlist ? 'Removed from your Watchlist' : 'Added to your Watchlist')} /> : null}
        {isVideo(item) ? <IconButton icon="eye" label={st.watched ? 'Mark unwatched' : 'Mark watched'} active={st.watched} onClick={() => set({watched: !st.watched}, st.watched ? 'Marked unwatched' : 'Marked watched')} /> : null}
        <IconButton icon="heart" label={st.favorite ? 'Remove from Favorites' : 'Add to Favorites'} active={st.favorite} onClick={() => set({favorite: !st.favorite}, st.favorite ? 'Removed from Favorites' : 'Added to Favorites')} />
        <IconButton icon="star" label={st.rating ? `Your rating: ${st.rating} of 5` : 'Rate'} active={!!st.rating} onClick={() => actions.rate(item.id)} />
        {extra}
        <Menu label="More actions" trigger={<button type="button" className="btn btn-glass btn-md btn-icon btn-round" aria-label="More actions"><Icon name="more" size={19} /></button>} items={menu} />
      </div>
    </div>
  );
}

/** Version (edition), audio and subtitles, chosen before pressing Play. */
export function PlaybackChoices({versions, audio, subs, itemId}: {versions: MediaVersion[]; audio: Track[]; subs: Track[]; itemId: string}) {
  const [version, setVersion] = useState<string | undefined>(versions[0]?.id);
  const [a, setA] = useState<string | undefined>(audio[0]?.id);
  const [s, setS] = useState<string | null>(subs.find(x => x.forced)?.id ?? null);
  useEffect(() => { player.set({versionId: version ?? null, audioTrack: a ?? null, subtitle: s}); }, [version, a, s, itemId]);
  const v = versions.find(x => x.id === version);
  const chooser = (label: string, value: string, options: {id: string | null; label: string; hint?: string}[], onPick: (id: string | null) => void, current: string | null | undefined) => (
    <Popover className="choice-pop" trigger={
      <button type="button" className="choice-btn"><span className="choice-k">{label}</span><span className="choice-v">{value}</span><Icon name="chevronDown" size={15} /></button>
    }>
      <div className="choice-list" role="listbox" aria-label={label}>
        {options.map(o => (
          <button key={o.id ?? 'off'} type="button" role="option" aria-selected={o.id === current} className={cx('choice-opt', o.id === current && 'is-on')} onClick={() => onPick(o.id)}>
            <span><strong>{o.label}</strong>{o.hint ? <span>{o.hint}</span> : null}</span>
            {o.id === current ? <Icon name="check" size={16} /> : null}
          </button>
        ))}
      </div>
    </Popover>
  );
  return (
    <div className="choices">
      {versions.length > 1 ? chooser('Version', v?.edition ? `${v.edition}` : v?.label ?? '', versions.map(x => ({id: x.id, label: x.label, hint: `${x.container} · ${x.sizeGB} GB · ${x.audio}`})), id => setVersion(id ?? undefined), version) : v ? <span className="choice-static"><span className="choice-k">Version</span><span className="choice-v">{v.label}</span></span> : null}
      {audio.length ? chooser('Audio', audio.find(x => x.id === a)?.label ?? 'Default', audio.map(x => ({id: x.id, label: x.label, hint: x.codec})), id => setA(id ?? undefined), a) : null}
      {subs.length ? chooser('Subtitles', s ? subs.find(x => x.id === s)?.label ?? 'On' : 'Off', [{id: null, label: 'Off'}, ...subs.map(x => ({id: x.id, label: x.label, hint: x.origin === 'provider' ? 'Downloaded' : x.origin === 'sidecar' ? 'File next to the video' : x.forced ? 'Forced · Embedded' : 'Embedded'}))], setS, s) : null}
    </div>
  );
}

export function Credits({credits}: {credits: Credit[]}) {
  const directors = credits.filter(c => c.job === 'Director' || c.job === 'Creator');
  const writers = credits.filter(c => c.job === 'Writer');
  const cast = credits.filter(c => c.department === 'Acting').slice(0, 3);
  const person = (c: Credit) => <Link key={c.personId + (c.job ?? c.character)} to="/person/$personId" params={{personId: c.personId}} className="credit-link">{c.name}</Link>;
  const join = (list: Credit[]) => list.map((c, i) => <React.Fragment key={i}>{i ? ', ' : ''}{person(c)}</React.Fragment>);
  return (
    <dl className="credit-lines">
      {cast.length ? <div><dt>Starring</dt><dd>{join(cast)}</dd></div> : null}
      {directors.length ? <div><dt>{directors[0]!.job === 'Creator' ? 'Created by' : 'Directed by'}</dt><dd>{join(directors)}</dd></div> : null}
      {writers.length ? <div><dt>Written by</dt><dd>{join(writers)}</dd></div> : null}
    </dl>
  );
}

export function CastShelf({credits, title = 'Cast & Crew'}: {credits: Credit[]; title?: string}) {
  const services = useServices();
  const ids = [...new Set(credits.map(c => c.personId))];
  const people = useQuery(ids.length ? `people.${ids.join()}` : null, async () => Promise.all(ids.map(id => services.catalog.person(id))));
  if (!credits.length) return null;
  const byId = new Map((people.data ?? []).filter(Boolean).map(p => [p!.id, p!]));
  return (
    <Shelf id="cast" title={title}>
      {credits.map((c, i) => {
        const p = byId.get(c.personId);
        return (
          <div key={c.personId + i} className="card-cell cell-person">
            <Link to="/person/$personId" params={{personId: c.personId}} className="card card-person" onFocus={() => p && setTint(p.tint)} onMouseEnter={() => p && setTint(p.tint)}>
              <span className="card-frame">{p ? <Art src={p.art.headshot} tint={p.tint} fallbackIcon="profile" /> : <Skeleton style={{position: 'absolute', inset: 0}} />}</span>
              <span className="card-caption"><span className="card-title">{c.name}</span><span className="card-sub">{c.character ?? c.job}</span></span>
            </Link>
          </div>
        );
      })}
    </Shelf>
  );
}

export function ExtrasShelf({extras, parentId}: {extras: Extra[]; parentId: string}) {
  if (!extras.length) return null;
  return (
    <Shelf id="extras" title="Extras">
      {extras.map(x => (
        <div key={x.id} className="card-cell cell-landscape">
          <button type="button" className="card card-landscape card-extra" onClick={() => player.play(parentId, {kind: 'video', from: 0})} aria-label={`Play ${x.title}, ${x.kind}, ${formatClock(x.duration)}`}>
            <span className="card-frame">
              <Art src={x.art} tint="#1d3848" />
              <span className="extra-play"><Icon name="play" size={22} fill="currentColor" /></span>
              <span className="extra-time">{formatClock(x.duration)}</span>
            </span>
            <span className="card-caption"><span className="card-title">{x.title}</span><span className="card-sub">{x.kind}</span></span>
          </button>
        </div>
      ))}
    </Shelf>
  );
}

export function RelatedShelf({id, title = 'More like this'}: {id: string; title?: string}) {
  const services = useServices();
  const related = useQuery(`related.${id}`, () => services.catalog.related(id).then(ids => services.catalog.items(ids)));
  if (!related.data?.length) return null;
  return (
    <Shelf id="related" title={title}>
      {related.data.map(item => <MediaCard key={item.id} item={item} context="recommendation" />)}
    </Shelf>
  );
}

/** Media information: one column per version, plus what the title is. */
export function DetailsSection({item, about}: {item: Movie | (Item & {versions?: MediaVersion[]}); about: [string, React.ReactNode][]}) {
  const versions = (item as Movie).versions ?? [];
  return (
    <section className="details" aria-labelledby="details-h">
      <h2 className="shelf-title" id="details-h">Details</h2>
      <div className="details-grid">
        <div className="details-col">
          <h3 className="eyebrow">About</h3>
          <KeyValue items={about} />
        </div>
        {versions.map(v => (
          <div className="details-col" key={v.id}>
            <h3 className="eyebrow">{versions.length > 1 ? v.label : 'Media information'}</h3>
            <KeyValue items={[
              ['Resolution', <>{v.resolution}{v.hdr ? <Badge tone="accent" className="ml">{v.hdr}</Badge> : null}</>],
              ['Video', v.video],
              ['Audio', v.audio],
              ['Container', v.container],
              ['Size', `${v.sizeGB} GB · ${v.bitrateMbps} Mb/s`],
              ...(v.edition ? [['Edition', v.edition] as [string, string]] : []),
            ]} />
          </div>
        ))}
      </div>
    </section>
  );
}

export function TitleSkeleton() {
  return (
    <div className="stage stage-skeleton" aria-busy="true" aria-label="Loading">
      <div className="stage-content">
        <Skeleton w={140} h={12} />
        <Skeleton w="min(560px, 80%)" h={72} style={{marginTop: 18}} />
        <Skeleton w="min(340px, 60%)" h={14} style={{marginTop: 18}} />
        <Skeleton w="min(520px, 80%)" h={14} style={{marginTop: 14}} />
        <Skeleton w="min(480px, 76%)" h={14} style={{marginTop: 8}} />
        <div className="action-bar"><Skeleton w={150} h={52} r={26} /><Skeleton w={44} h={44} r={22} /><Skeleton w={44} h={44} r={22} /><Skeleton w={44} h={44} r={22} /></div>
      </div>
    </div>
  );
}

