import React, {useEffect, useRef, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useQuery} from '@/data/DataProvider';
import {useActions} from '@/app/actions';
import {formatTime} from '@/app/format';
import {Artwork} from '@/ui/Bits';
import {Button, IconButton} from '@/ui/Button';
import {Segmented} from '@/ui/Form';
import {Icon} from '@/ui/Icon';
import {Menu} from '@/ui/Overlay';
import {usePhone} from '@/ui/hooks';
import {cx} from '@/ui/cx';
import {usePlayer, usePlayerState} from './PlayerProvider';
import {Timeline} from './Timeline';
import {OptionsPanel, PlayOnDialog, QueuePanel} from './Panels';
import s from './Player.module.css';

function useBookChapters(id: string | undefined, book: boolean) {
  return useQuery(c => (book && id ? c.catalog.audiobook(id) : Promise.resolve(undefined)), [id, book]);
}

/** The music and audiobook bar docked at the bottom of every page. */
export function MiniPlayer() {
  const st = usePlayerState();
  const player = usePlayer();
  const phone = usePhone();
  const [playOn, setPlayOn] = useState(false);
  const e = st.queue[st.index];
  if (!e) return null;
  const book = st.mode === 'book';
  return (
    <div className={s.mini} role="region" aria-label="Now playing">
      <div className={s.miniProgress}><span style={{width: `${st.duration ? (st.position / st.duration) * 100 : 0}%`}} /></div>
      <button type="button" className={s.miniInfo} onClick={() => player.expand()} aria-label={`Open now playing: ${e.title}`}>
        <Artwork src={e.image} title={e.title} shape={book ? 'poster' : 'square'} className={s.miniArt} style={book ? {width: 34} : undefined} />
        <span className={s.miniText}><b>{e.title}</b><span>{st.cast ? `Playing on ${st.cast.name}` : book ? `${e.subtitle} · ${formatTime(st.position)} of ${formatTime(st.duration)}` : e.subtitle}</span></span>
      </button>
      <div className={s.miniCenter}>
        <div className={s.miniButtons}>
          {!phone ? <IconButton variant="ghost" icon={book ? 'skipBack' : 'previous'} label={book ? 'Back 15 seconds' : 'Previous'} onClick={() => (book ? player.skip(-15) : player.previous())} /> : null}
          <button type="button" className={s.playBtn} onClick={() => player.toggle()} aria-label={st.playing ? 'Pause' : 'Play'}><Icon name={st.playing ? 'pause' : 'play'} size={20} /></button>
          <IconButton variant="ghost" icon={book ? 'skipForward' : 'next'} label={book ? 'Forward 30 seconds' : 'Next'} onClick={() => (book ? player.skip(30) : player.next())} />
        </div>
      </div>
      <div className={s.miniRight}>
        {!book ? <IconButton variant="ghost" icon="shuffle" label={st.shuffle ? 'Shuffle is on' : 'Shuffle'} pressed={st.shuffle} onClick={() => player.set({shuffle: !st.shuffle})} /> : null}
        {!book ? <IconButton variant="ghost" icon={st.repeat === 'one' ? 'repeatOne' : 'repeat'} label={st.repeat === 'off' ? 'Repeat' : st.repeat === 'all' ? 'Repeat all' : 'Repeat one'} pressed={st.repeat !== 'off'} onClick={() => player.set({repeat: st.repeat === 'off' ? 'all' : st.repeat === 'all' ? 'one' : 'off'})} /> : null}
        <IconButton variant="ghost" icon="cast" label="Play on another device" pressed={!!st.cast} onClick={() => setPlayOn(true)} />
        <IconButton variant="ghost" icon="expand" label="Open now playing" onClick={() => player.expand()} />
        <IconButton variant="ghost" icon="close" label="Stop and close" onClick={() => player.close()} />
      </div>
      <PlayOnDialog open={playOn} onOpenChange={setPlayOn} />
    </div>
  );
}

/** Now playing: artwork, transport, and beside it lyrics that follow along, the queue, or an audiobook's chapters. */
export function NowPlaying() {
  const st = usePlayerState();
  const player = usePlayer();
  const actions = useActions();
  const navigate = useNavigate();
  const e = st.queue[st.index];
  const book = st.mode === 'book';
  const [side, setSide] = useState<'lyrics' | 'queue' | 'chapters'>(book ? 'chapters' : 'lyrics');
  const [sideOpen, setSideOpen] = useState(false);
  const [options, setOptions] = useState(false);
  const [playOn, setPlayOn] = useState(false);
  const bookDetail = useBookChapters(e?.id, book);
  const track = useQuery(c => (!book && e ? Promise.resolve(c.catalog.tracks([e.id])[0]) : Promise.resolve(undefined)), [e?.id, book]);
  const lyricRef = useRef<HTMLButtonElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  useEffect(() => { rootRef.current?.focus({preventScroll: true}); }, []);
  useEffect(() => { setSide(book ? 'chapters' : 'lyrics'); }, [book]);
  useEffect(() => {
    const on = (ev: KeyboardEvent) => {
      if (options || playOn) return;
      if (ev.key === 'Escape') player.minimize();
      else if (ev.key === ' ' && !(ev.target as HTMLElement).closest('button, input')) { ev.preventDefault(); player.toggle(); }
    };
    window.addEventListener('keydown', on);
    return () => window.removeEventListener('keydown', on);
  }, [player, options, playOn]);
  const lyrics = track.data?.lyrics;
  const lyricIndex = lyrics ? lyrics.reduce((idx, l, i) => (st.position >= l.at ? i : idx), -1) : -1;
  useEffect(() => { lyricRef.current?.scrollIntoView({block: 'center', behavior: 'smooth'}); }, [lyricIndex]);
  if (!e) return null;
  const chapters = bookDetail.data?.chapters ?? [];
  const ci = Math.max(0, chapters.findIndex((c, i) => st.position >= c.start && st.position < (chapters[i + 1]?.start ?? Infinity)));
  const chapter = chapters[ci];
  return (
    <div ref={rootRef} tabIndex={-1} className={cx(s.now, sideOpen && s.nowSideOpen)} role="dialog" aria-modal="true" aria-label={`Now playing: ${e.title}`} style={{'--tint': e.tint} as React.CSSProperties}>
      <div className={s.nowBg} />
      <div className={s.nowMain}>
        <div className={s.nowTop}>
          <Button variant="ghost" icon="chevronDown" label="Minimize" onClick={() => player.minimize()} />
          <Menu label="More" items={[
            ...(e.parentId && !book ? [{id: 'album', label: 'Go to album', icon: 'music' as const, onSelect: () => { player.minimize(); void navigate({to: `/media/${e.parentId}` as never}); }}] : []),
            ...(book ? [{id: 'book', label: 'Go to audiobook', icon: 'audiobook' as const, onSelect: () => { player.minimize(); void navigate({to: `/media/${e.id}` as never}); }}] : []),
            {id: 'playlist', label: 'Add to playlist…', icon: 'playlist', onSelect: () => actions.dialog({kind: 'playlist', entries: [e]})},
            {id: 'cast', label: 'Play on…', icon: 'cast', onSelect: () => setPlayOn(true)},
            {id: 'report', label: 'Report a problem…', icon: 'flag', onSelect: () => actions.dialog({kind: 'feedback', entry: e})},
            {id: 'close', label: 'Stop and close', icon: 'close', separatorBefore: true, onSelect: () => player.close()},
          ]} trigger={<IconButton variant="ghost" icon="more" label="More" />} />
        </div>
        <Artwork src={e.image} title={e.title} shape={book ? 'poster' : 'square'} className={s.nowArt} eager tint={e.tint} style={book ? {width: 'min(300px, 36vh)'} : undefined} />
        <div className={s.nowTitle}>
          <h2>{e.title}</h2>
          <p>{e.subtitle}{book && chapter ? ` · ${chapter.title} (${ci + 1} of ${chapters.length})` : ''}</p>
          {st.cast ? <p style={{color: 'var(--accent-strong)'}}>Playing on {st.cast.name}</p> : null}
        </div>
        <div className={s.nowTimeline}><span>{formatTime(st.position)}</span><Timeline position={st.position} duration={st.duration} onSeek={t => player.seek(t)} chapters={book ? chapters.map(c => ({title: c.title, start: c.start})) : []} /><span>-{formatTime(Math.max(0, st.duration - st.position))}</span></div>
        <div className={s.nowControls}>
          {!book ? <IconButton variant="ghost" icon="shuffle" label="Shuffle" pressed={st.shuffle} onClick={() => player.set({shuffle: !st.shuffle})} /> : <IconButton variant="ghost" icon="skipBack" label="Back 15 seconds" onClick={() => player.skip(-15)} />}
          <IconButton variant="ghost" size="lg" icon="previous" label={book ? 'Previous chapter' : 'Previous'} onClick={() => (book && chapter ? player.seek(chapters[Math.max(0, ci - (st.position - chapter.start < 5 ? 1 : 0))]!.start) : player.previous())} />
          <button type="button" className={s.nowPlay} onClick={() => player.toggle()} aria-label={st.playing ? 'Pause' : 'Play'}><Icon name={st.playing ? 'pause' : 'play'} size={30} /></button>
          <IconButton variant="ghost" size="lg" icon="next" label={book ? 'Next chapter' : 'Next'} onClick={() => (book && chapters[ci + 1] ? player.seek(chapters[ci + 1]!.start) : player.next())} />
          {!book ? <IconButton variant="ghost" icon={st.repeat === 'one' ? 'repeatOne' : 'repeat'} label={st.repeat === 'off' ? 'Repeat' : st.repeat === 'all' ? 'Repeat all' : 'Repeat one'} pressed={st.repeat !== 'off'} onClick={() => player.set({repeat: st.repeat === 'off' ? 'all' : st.repeat === 'all' ? 'one' : 'off'})} /> : <IconButton variant="ghost" icon="skipForward" label="Forward 30 seconds" onClick={() => player.skip(30)} />}
        </div>
        <div className={s.nowExtras}>
          {book ? <Button size="sm" variant="secondary" icon="speed" label={st.speed === 1 ? 'Speed' : `${st.speed}×`} onClick={() => setOptions(true)} /> : null}
          <Button size="sm" variant="secondary" icon="sleep" label={st.sleepLabel ? `Sleep · ${st.sleepLabel}` : 'Sleep timer'} onClick={() => setOptions(true)} />
          <Button size="sm" variant="secondary" icon="cast" label={st.cast ? st.cast.name : 'Play on'} onClick={() => setPlayOn(true)} />
          <Button size="sm" variant="secondary" icon={book ? 'chapters' : 'lyrics'} label={book ? 'Chapters' : 'Lyrics and queue'} onClick={() => setSideOpen(true)} className={s.sideToggle} />
        </div>
      </div>
      <aside className={s.nowSide} aria-label={book ? 'Chapters and queue' : 'Lyrics and queue'}>
        <div className={s.nowSideHead} style={{display: 'flex', gap: 8, alignItems: 'center'}}>
          <Segmented label="Show" value={side} onChange={setSide} options={book ? [{value: 'chapters', label: 'Chapters'}, {value: 'queue', label: 'Up next'}] : [{value: 'lyrics', label: 'Lyrics'}, {value: 'queue', label: 'Up next'}]} />
          {sideOpen ? <IconButton variant="ghost" icon="close" label="Close" onClick={() => setSideOpen(false)} /> : null}
        </div>
        <div className={s.nowSideBody}>
          {side === 'lyrics' ? (lyrics?.length ? lyrics.map((l, i) => (
            <button key={i} ref={i === lyricIndex ? lyricRef : undefined} type="button" className={cx(s.lyric, i === lyricIndex && s.lyricNow, i < lyricIndex && s.lyricPast)} onClick={() => player.seek(l.at)}>{l.line}</button>
          )) : <p className="c-3" style={{padding: 10}}>{track.loading ? 'Loading lyrics…' : 'No lyrics for this song.'}</p>) : null}
          {side === 'queue' ? <QueuePanel embedded /> : null}
          {side === 'chapters' ? chapters.map((c, i) => (
            <button key={c.number} type="button" className={s.opt} aria-checked={i === ci} role="menuitemradio" onClick={() => player.seek(c.start)}>
              <span className={s.optCheck}>{i === ci ? <Icon name="audio" size={16} /> : null}</span>
              <span className={s.optText}>{c.title}<span className={s.optSub}>{c.title === `Chapter ${c.number}` ? formatTime(c.duration) : `Chapter ${c.number} · ${formatTime(c.duration)}`}</span></span>
            </button>
          )) : null}
        </div>
      </aside>
      {options ? <OptionsPanel onClose={() => setOptions(false)} audio={[]} subtitles={[]} captionStyle={{size: 'medium', background: 'box'}} onCaptionStyle={() => {}} /> : null}
      <PlayOnDialog open={playOn} onOpenChange={setPlayOn} />
    </div>
  );
}
