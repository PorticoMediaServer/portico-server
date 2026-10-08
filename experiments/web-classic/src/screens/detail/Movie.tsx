import {useState} from 'react';
import {Link} from '@tanstack/react-router';
import {useQuery} from '@/data/DataProvider';
import {usePlayer} from '@/player/PlayerProvider';
import {formatDuration} from '@/app/format';
import {Button} from '@/ui/Button';
import {ErrorState} from '@/ui/Bits';
import {Select} from '@/ui/Form';
import {Shelf} from '@/ui/Shelf';
import {useTitle} from '@/ui/hooks';
import {DetailLoading} from '../common';
import {ActionRow, CastRow, DetailFrame, Facts, MediaInfoDialog, MetaLine, Overview, People, RatingPill, Ratings} from './parts';
import {useDemo} from '@/app/demo';
import s from './Detail.module.css';

/** Movie (and home video) page: play with version, audio and subtitle choices; cast, extras, related. */
export function MovieDetail({id, home}: {id: string; home?: boolean}) {
  const q = useQuery(c => (home ? c.catalog.homeVideo(id) : c.catalog.movie(id)), [id, home], ['personal']);
  const player = usePlayer();
  const [version, setVersion] = useState<string>();
  const [audio, setAudio] = useState<string>();
  const [subs, setSubs] = useState<string>();
  const [info, setInfo] = useState(false);
  useDemo('media-info', () => setInfo(true), !!q.data);
  useTitle(q.data?.entry.title);
  if (q.error && !q.data) return <ErrorState title="This title didn’t load" body={q.error} onRetry={q.refresh} />;
  if (!q.data) return <DetailLoading />;
  const d = q.data;
  const e = d.entry;
  const trailer = d.extras.find(x => x.title.includes('Trailer'));
  const play = (fromStart?: boolean) => {
    player.play([e], 0, {fromStart});
    player.set({audio: audio ?? d.audio.find(t => t.default)?.id ?? d.audio[0]?.id ?? 'a1', subtitles: subs ?? 'off'});
  };
  const v = d.versions.find(x => x.id === version) ?? d.versions[0];
  return (
    <DetailFrame
      entry={e}
      shape={home ? 'wide' : 'poster'}
      eyebrow={<>{home ? <Link to="/library/lib-homevideos">Home Videos</Link> : <Link to={`/library/${e.libraryId}` as never}>{e.libraryId === 'lib-family' ? 'Family' : 'Movies'}</Link>}{d.collections[0] ? <><span aria-hidden>·</span><Link to={`/library/${e.libraryId}/collection/${d.collections[0].id}` as never}>{d.collections[0].name}</Link></> : null}</>}
      sections={<>
        {home ? null : <CastRow people={[...d.directors, ...d.cast, ...d.writers]} />}
        {d.extras.length ? <Shelf title="Extras" entries={d.extras} shape="wide" /> : null}
        {d.related.length ? <Shelf title={home ? 'More home videos' : 'More like this'} entries={d.related} shape={home ? 'wide' : 'poster'} origin="recommendation" /> : null}
      </>}
    >
      <MetaLine wrapLast items={[home ? e.subtitle : e.year, e.rating ? <RatingPill>{e.rating}</RatingPill> : null, formatDuration(e.duration), v?.resolution.startsWith('4K') ? <RatingPill>4K</RatingPill> : null, v?.video.includes('Dolby Vision') ? <RatingPill>Dolby Vision</RatingPill> : null, e.genres?.join(', ')]} />
      <Ratings audience={d.audienceRating} critic={d.criticRating} mine={d.personal.rating} />
      <ActionRow entry={e} personal={d.personal} onPlay={play} watchlist={!home} extra={trailer ? <Button variant="secondary" size="lg" icon="playOutline" label="Trailer" onClick={() => player.play([trailer], 0, {fromStart: true})} /> : null} />
      <Overview text={e.overview} tagline={d.tagline} />
      {home ? null : <Facts rows={[['Directed by', d.directors.length ? <People list={d.directors} /> : undefined], ['Written by', d.writers.length ? <People list={d.writers} /> : undefined], d.studio === 'Public domain' ? ['Rights', 'Public domain'] : ['Studio', d.studio ? <Link to={`/library/${e.libraryId}/studio/${encodeURIComponent(d.studio)}` as never}>{d.studio}</Link> : undefined], ['Starring', d.cast.length ? <People list={d.cast.slice(0, 3)} /> : undefined]]} />}
      <div className={s.selectors}>
        {d.versions.length > 1 ? <Select className={s.selector} label="Version" value={version ?? d.versions[0]!.id} onChange={setVersion} options={d.versions.map(x => ({value: x.id, label: `${x.edition ?? 'Theatrical'} · ${x.label}`}))} /> : null}
        {d.audio.length ? <Select className={s.selector} label="Audio" value={audio ?? d.audio.find(t => t.default)?.id ?? d.audio[0]!.id} onChange={setAudio} options={d.audio.map(t => ({value: t.id, label: `${t.label}${t.channels ? ` · ${t.codec} ${t.channels}` : ''}`}))} /> : null}
        {d.subtitles.length ? <Select className={s.selector} label="Subtitles" value={subs ?? 'off'} onChange={setSubs} options={[{value: 'off', label: 'Off'}, ...d.subtitles.map(t => ({value: t.id, label: `${t.label}${t.forced ? '' : ` · ${t.codec}`}`}))]} /> : null}
      </div>
      <div><Button variant="ghost" size="sm" icon="info" label="Media information" onClick={() => setInfo(true)} /></div>
      {d.license ? <p className="t-caption c-3">Licensed {d.license}{d.studio === "Blender Studio" ? " · Blender Foundation" : ""}</p> : null}
      <MediaInfoDialog open={info} onOpenChange={setInfo} versions={d.versions} title={e.title} />
    </DetailFrame>
  );
}
