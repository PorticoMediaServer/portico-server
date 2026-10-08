import React, {useEffect, useRef, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useQuery, useServices} from '../data/hooks';
import type {Audiobook, Item, TrackItem} from '../data/types';
import {player, usePlayer} from '../app/player';
import {usePrefs} from '../app/prefs';
import {Art} from '../ui/Media';
import {Icon, type IconName} from '../ui/Icon';
import {Slider} from '../ui/Controls';
import {Tooltip} from '../ui/Overlay';
import {toast} from '../ui/Feedback';
import {cx} from '../ui/cx';
import {formatClock} from '../screens/shared/items';
import {actions} from '../screens/shared/actions';
import {QueueList, Sleep} from './Panels';
import {takePending} from './VideoPlayer';

type Side = 'lyrics' | 'queue' | 'chapters' | 'sleep' | null;

/**
 * Now Playing for music and audiobooks: the artwork large over its own blurred colour, the
 * scrubber and transport, and a side panel for synced lyrics, the queue, or a book's chapters,
 * speed and sleep timer.
 */
export function NowPlaying() {
  const services = useServices();
  const navigate = useNavigate();
  const s = usePlayer();
  const {prefs, update} = usePrefs();
  const item = useQuery(s.itemId ? `item.${s.itemId}` : null, () => services.catalog.item(s.itemId!)).data;
  const queueItems = useQuery(`queue.${s.queue.join(',')}`, () => services.catalog.items(s.queue)).data ?? [];
  const byId = new Map(queueItems.map(i => [i.id, i]));
  const book = s.kind === 'book' && item?.kind === 'audiobook' ? (item as Audiobook) : null;
  const track = item?.kind === 'track' ? (item as TrackItem) : null;
  const lyrics = useQuery(track?.hasLyrics ? `lyrics.${track.id}` : null, () => services.catalog.lyrics(track!.id)).data;
  const [side, setSide] = useState<Side>(() => (takePending() as Side) ?? 'lyrics');
  useEffect(() => { if (side === 'lyrics' && track && !track.hasLyrics) setSide('queue'); if ((side === 'lyrics' || side === 'queue') && book) setSide('chapters'); }, [track, side, book]);
  useEffect(() => {
    const on = (e: Event) => setSide((e as CustomEvent<Side>).detail);
    window.addEventListener('portico:panel', on);
    return () => window.removeEventListener('portico:panel', on);
  }, []);
  useEffect(() => {
    document.body.style.overflow = 'hidden';
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('input, textarea, [role=slider]')) return;
      if (e.key === 'Escape') player.minimize();
      if (e.key === ' ') { e.preventDefault(); player.toggle(); }
    };
    window.addEventListener('keydown', onKey);
    return () => { document.body.style.overflow = ''; window.removeEventListener('keydown', onKey); };
  }, []);
  if (!item) return <div className="nowplaying" role="dialog" aria-label="Now playing" />;
  const chapter = book ? [...book.chapters].reverse().find(c => c.start <= s.position) ?? book.chapters[0] : null;
  const chapterIndex = book && chapter ? book.chapters.indexOf(chapter) : -1;
  const art = book ? book.art.poster : item.art.square;
  const title = book ? chapter?.title ?? book.title : item.title;
  const sub = book ? `${book.title} · ${book.author}` : track ? `${track.artistName} · ${track.albumTitle}` : '';
  const btn = (icon: IconName, label: string, onClick: () => void, opts: {on?: boolean; big?: boolean; disabled?: boolean} = {}) => (
    <Tooltip label={label}>
      <button type="button" className={cx('np-btn', opts.big && 'is-big', opts.on && 'is-on')} onClick={onClick} aria-label={label} aria-pressed={opts.on} disabled={opts.disabled}>
        <Icon name={icon} size={opts.big ? 34 : 22} fill={icon === 'play' || icon === 'pause' ? 'currentColor' : undefined} />
      </button>
    </Tooltip>
  );
  return (
    <div className={cx('nowplaying', side && 'has-side')} role="dialog" aria-modal="true" aria-label="Now playing" style={{'--tint-c': item.tint} as React.CSSProperties}>
      <div className="np-bg" aria-hidden><Art src={art} tint={item.tint} eager /></div>
      <div className="np-top">
        <Tooltip label="Minimize (Esc)"><button type="button" className="np-btn" onClick={() => player.minimize()} aria-label="Minimize"><Icon name="chevronDown" size={26} /></button></Tooltip>
        <span className="eyebrow">{book ? 'Listening to' : 'Now playing'}</span>
        <div className="np-top-right">
          {btn('cast', s.playOn ? `Playing on ${s.playOn.device}` : 'Play on another speaker', () => { player.set({playOn: s.playOn ? null : {device: 'Bedroom HomePod', kind: 'airplay', state: 'playing'}}); toast(s.playOn ? 'Playing on this browser' : 'Playing on Bedroom HomePod', {icon: 'cast'}); }, {on: !!s.playOn})}
          {btn('close', 'Stop and close', () => player.stop())}
        </div>
      </div>
      <div className="np-main">
        <div className="np-art-wrap">
          <Art src={art} tint={item.tint} className={cx('np-art', book && 'is-book', s.playing && 'is-playing')} ratio={book ? '2/3' : '1/1'} eager fallbackIcon={book ? 'bookOpen' : 'music'} />
        </div>
        <div className="np-info">
          <div className="np-titles">
            <h2 className="display np-title">{title}</h2>
            <button type="button" className="np-sub" onClick={() => { player.minimize(); void navigate({to: '/media/$itemId', params: {itemId: book ? book.id : track?.albumId ?? item.id}}); }}>{sub}</button>
            {book ? <span className="faint small">Chapter {chapterIndex + 1} of {book.chapters.length}</span> : null}
          </div>
          <div className="np-scrub">
            <Slider label="Position" value={Math.floor(s.position)} max={Math.max(1, Math.floor(s.duration))} onChange={v => player.seek(v)} />
            <div className="np-times tabular"><span>{formatClock(s.position)}</span><span>−{formatClock(Math.max(0, s.duration - s.position))}</span></div>
          </div>
          <div className="np-transport">
            {book ? btn('rewind', 'Back 15 seconds', () => player.skip(-15)) : btn('shuffle', s.shuffle ? 'Shuffle on' : 'Shuffle off', () => player.set({shuffle: !s.shuffle}), {on: s.shuffle})}
            {btn('previous', book ? 'Previous chapter' : 'Previous', () => (book && chapterIndex > 0 ? player.seek(book.chapters[chapterIndex - (s.position - chapter!.start > 5 ? 0 : 1)]!.start) : player.previous()))}
            {btn(s.playing ? 'pause' : 'play', s.playing ? 'Pause' : 'Play', () => player.toggle(), {big: true})}
            {btn('next', book ? 'Next chapter' : 'Next', () => (book ? book.chapters[chapterIndex + 1] && player.seek(book.chapters[chapterIndex + 1]!.start) : player.next()), {disabled: book ? chapterIndex >= book.chapters.length - 1 : s.index >= s.queue.length - 1 && s.repeat === 'off'})}
            {book ? btn('fastForward', 'Forward 30 seconds', () => player.skip(30)) : btn(s.repeat === 'one' ? 'repeatOne' : 'repeat', s.repeat === 'off' ? 'Repeat off' : s.repeat === 'all' ? 'Repeat all' : 'Repeat this song', () => player.set({repeat: s.repeat === 'off' ? 'all' : s.repeat === 'all' ? 'one' : 'off'}), {on: s.repeat !== 'off'})}
          </div>
          <div className="np-extras">
            <div className="np-volume">
              <button type="button" className="np-btn is-sm" onClick={() => update({muted: !prefs.muted})} aria-label={prefs.muted ? 'Unmute' : 'Mute'}><Icon name={prefs.muted ? 'mute' : 'volume'} size={18} /></button>
              <Slider label="Volume" value={prefs.muted ? 0 : Math.round(prefs.volume * 100)} onChange={v => update({volume: v / 100, muted: false})} />
            </div>
            {book ? (
              <div className="np-speed" role="radiogroup" aria-label="Speed">
                {[0.8, 1, 1.2, 1.5, 2].map(r => <button key={r} type="button" role="radio" aria-checked={s.rate === r} className={cx('pp-speed', s.rate === r && 'is-on')} onClick={() => player.set({rate: r})}>{r}×</button>)}
              </div>
            ) : null}
          </div>
          <div className="np-tabs" role="tablist" aria-label="Now playing panels">
            {book ? <NpTab id="chapters" icon="list" label="Chapters" side={side} setSide={setSide} /> : <NpTab id="lyrics" icon="mic" label="Lyrics" side={side} setSide={setSide} disabled={!track?.hasLyrics} />}
            {!book ? <NpTab id="queue" icon="playlist" label="Queue" side={side} setSide={setSide} /> : null}
            <NpTab id="sleep" icon="timer" label={s.sleep.mode === 'off' ? 'Sleep timer' : `Sleep · ${s.sleep.label}`} side={side} setSide={setSide} />
            <Tooltip label="Add to playlist"><button type="button" className="np-btn is-sm" onClick={() => actions.addToPlaylist([item.id])} aria-label="Add to playlist"><Icon name="listPlus" size={18} /></button></Tooltip>
          </div>
        </div>
        {side ? (
          <aside className="np-side" aria-label={side}>
            {side === 'lyrics' ? <Lyrics lines={lyrics ?? null} position={s.position} /> : null}
            {side === 'queue' ? <><div className="np-side-head"><h3 className="pp-title">Up next</h3><button type="button" className="btn btn-ghost btn-sm" onClick={() => void services.lists.createPlaylist(`Queue from ${new Date().toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}`, '', false, s.queue).then(p => toast(`Saved the queue as “${p.title}”`, {icon: 'playlist'}))}><Icon name="save" size={15} /><span className="btn-label">Save as playlist</span></button></div><QueueList queue={s.queue} index={s.index} byId={byId as Map<string, Item>} compact /></> : null}
            {side === 'chapters' && book ? (
              <ol className="np-chapters">
                {book.chapters.map((c, i) => (
                  <li key={i}><button type="button" className={cx('chapter-row', i === chapterIndex && 'is-current')} onClick={() => player.seek(c.start)}><span className="chapter-n tabular">{i === chapterIndex ? <Icon name="play" size={13} fill="currentColor" /> : i + 1}</span><span className="chapter-t">{c.title}</span><span className="chapter-d tabular">{formatClock(c.duration)}</span></button></li>
                ))}
              </ol>
            ) : null}
            {side === 'sleep' ? <><h3 className="pp-title" style={{marginBottom: 10}}>Sleep timer</h3><Sleep onDone={() => setSide(book ? 'chapters' : 'queue')} /></> : null}
          </aside>
        ) : null}
      </div>
    </div>
  );
}

function NpTab({id, icon, label, side, setSide, disabled}: {id: Side; icon: IconName; label: string; side: Side; setSide: (s: Side) => void; disabled?: boolean}) {
  return (
    <button type="button" role="tab" aria-selected={side === id} className={cx('np-tab', side === id && 'is-on')} onClick={() => setSide(side === id ? null : id)} disabled={disabled}>
      <Icon name={icon} size={16} />{label}
    </button>
  );
}

/** Synced lyrics: the current line bright and centred, earlier lines dim; click a line to jump. */
function Lyrics({lines, position}: {lines: {t: number; line: string}[] | null; position: number}) {
  const box = useRef<HTMLDivElement>(null);
  const current = lines ? Math.max(0, lines.findLastIndex(l => l.t <= position)) : -1;
  useEffect(() => {
    const el = box.current?.querySelector<HTMLElement>('.is-now');
    if (el && box.current) box.current.scrollTo({top: el.offsetTop - box.current.clientHeight / 2 + el.clientHeight / 2, behavior: 'smooth'});
  }, [current]);
  if (!lines) return <p className="pp-note">No lyrics for this song.</p>;
  return (
    <div className="lyrics" ref={box}>
      {lines.map((l, i) => <button key={i} type="button" className={cx('lyric', i === current && 'is-now', i < current && 'is-past')} onClick={() => player.seek(l.t)}>{l.line || '♪'}</button>)}
    </div>
  );
}
