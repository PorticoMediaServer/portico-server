import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import {
  Cast,
  ChevronDown,
  Disc3,
  Ellipsis,
  FastForward,
  Gauge,
  GripVertical,
  Heart,
  ListMusic,
  MicVocal,
  Moon,
  Pause,
  Play,
  Repeat,
  Repeat1,
  Rewind,
  Shuffle,
  SkipBack,
  SkipForward,
  User,
  Volume1,
  Volume2,
  X,
} from 'lucide-react';
import { usePlayer, type QueueItem } from '../../state/player';
import { useUi } from '../../state/ui';
import { useIsMobile, useLockBody, fmtClock, fmtDuration, plural } from '../../lib/hooks';
import { Scrubber } from '../../components/Scrubber';
import { IconButton, cls } from '../../components/ui';
import { Menu, toast, useMenu } from '../../components/overlay';
import { castLabel } from './MiniPlayer';

export function NowPlaying() {
  const open = usePlayer((s) => s.nowPlaying);
  const mode = usePlayer((s) => s.mode);
  const [closing, setClosing] = useState(false);
  const setOpen = usePlayer((s) => s.openNowPlaying);
  useLockBody(open);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !document.querySelector('.sheet-root, .popover')) close();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
  const close = () => {
    setClosing(true);
    setTimeout(() => {
      setOpen(false);
      setClosing(false);
    }, 320);
  };
  if (!open || !mode) return null;
  return createPortal(<Body closing={closing} onClose={close} />, document.body);
}

function Body({ closing, onClose }: { closing: boolean; onClose: () => void }) {
  const s = usePlayer();
  const item = s.queue[s.index];
  const mobile = useIsMobile();
  const navigate = useNavigate();
  const setCast = useUi((x) => x.setCast);
  const [loved, setLoved] = useState<Record<string, boolean>>({});
  const speed = useMenu();
  const sleep = useMenu();
  const more = useMenu();
  const drag = useRef<{ y: number; dy: number } | null>(null);
  const sheet = useRef<HTMLDivElement>(null);
  if (!item) return null;
  const book = s.mode === 'audiobook';
  const panel = s.panel;
  const PlayIcon = s.playing ? Pause : Play;
  const bookPos = book ? (item.offset ?? 0) + s.position : 0;
  const bookLeft = book && s.book ? s.book.duration - bookPos : 0;

  const go = (to: string) => {
    onClose();
    navigate(to, { viewTransition: true });
  };

  return (
    <div
      className={cls('np-root', closing && 'closing', mobile && 'is-mobile', panel && 'with-panel')}
      role="dialog"
      aria-label="Now Playing"
      style={{ ['--np-base' as string]: item.palette?.base, ['--np-accent' as string]: item.palette?.accent }}
    >
      <div className="np-bg" aria-hidden="true">
        <img src={item.image} alt="" />
      </div>
      <div
        ref={sheet}
        className="np-sheet"
        onPointerDown={(e) => {
          if (mobile && (e.target as HTMLElement).closest('.np-grab-zone')) drag.current = { y: e.clientY, dy: 0 };
        }}
        onPointerMove={(e) => {
          if (!drag.current || !sheet.current) return;
          drag.current.dy = Math.max(0, e.clientY - drag.current.y);
          sheet.current.style.transform = `translateY(${drag.current.dy}px)`;
        }}
        onPointerUp={() => {
          if (!drag.current || !sheet.current) return;
          if (drag.current.dy > 110) onClose();
          else sheet.current.style.transform = '';
          drag.current = null;
        }}
      >
        <header className="np-head np-grab-zone">
          {mobile ? <div className="sheet-grabber" /> : null}
          <IconButton label="Close Now Playing" variant="plain" onClick={onClose} className="np-close">
            <ChevronDown />
          </IconButton>
          <div className="np-context">
            <span className="t-eyebrow">{book ? 'Audiobook' : s.shuffle ? 'Shuffling' : 'Playing from'}</span>
            <span className="np-context-title ellipsis">{book ? s.book?.title : item.album}</span>
          </div>
          <IconButton label="More" variant="plain" onClick={more.open}>
            <Ellipsis />
          </IconButton>
          <Menu
            anchor={more.anchor}
            open={more.isOpen}
            onClose={more.close}
            title={item.title}
            items={
              book
                ? [
                    { label: 'Go to Audiobook', icon: <Disc3 />, onSelect: () => go(`/audiobook/${s.book!.id}`) },
                    { label: 'Stop Playback', icon: <X />, danger: true, onSelect: () => s.stop() },
                  ]
                : [
                    { label: 'Go to Album', icon: <Disc3 />, onSelect: () => go(`/album/${item.albumId}`) },
                    { label: 'Go to Artist', icon: <User />, onSelect: () => go(`/artist/${item.artistId}`) },
                    { label: 'Start Station', icon: <Disc3 />, onSelect: () => toast(`Station based on “${item.title}” is up next`, <Disc3 />) },
                    { separator: true, label: '' },
                    { label: 'Stop Playback', icon: <X />, danger: true, onSelect: () => s.stop() },
                  ]
            }
          />
        </header>

        <div className="np-body">
          <div className={cls('np-main', panel && mobile && 'compact')}>
            <div className={cls('np-art-wrap', !s.playing && 'paused')}>
              <img className={cls('np-art', book && 'book')} src={item.image} alt="" />
            </div>
            <div className="np-meta">
              <div className="np-titles">
                <div className="np-title ellipsis">
                  {item.title}
                  {item.explicit && <span className="explicit">E</span>}
                </div>
                <button className="np-artist ellipsis" onClick={() => (book ? go(`/audiobook/${s.book!.id}`) : go(`/artist/${item.artistId}`))}>
                  {book ? `${s.book?.title} · ${item.subtitle}` : item.subtitle}
                </button>
              </div>
              {!book && (
                <IconButton
                  label={loved[item.id] ? 'Loved' : 'Love'}
                  variant="plain"
                  active={loved[item.id]}
                  onClick={() => {
                    setLoved((l) => ({ ...l, [item.id]: !l[item.id] }));
                    if (!loved[item.id]) toast('Added to Loved Tracks', <Heart />);
                  }}
                >
                  <Heart fill={loved[item.id] ? 'currentColor' : 'none'} />
                </IconButton>
              )}
            </div>
            <Scrubber value={s.position} max={item.duration} onSeek={(t) => s.seek(t)} className="np-scrub" label="Playback position" />
            {book && s.book && (
              <div className="np-booktime t-sub tnum">
                Chapter {s.index + 1} of {s.queue.length} · {fmtDuration(bookLeft)} left in book
              </div>
            )}
            {!book && item.lossless && <div className="np-quality">{item.lossless.replace('FLAC', 'Lossless FLAC').replace('ALAC', 'Lossless ALAC')} kHz</div>}
            <div className="np-controls">
              {book ? (
                <button className="np-small" onClick={speed.open} aria-label="Playback speed">
                  {s.rate}×
                </button>
              ) : (
                <IconButton label="Shuffle" variant="plain" active={s.shuffle} onClick={() => s.setShuffle(!s.shuffle)}>
                  <Shuffle />
                </IconButton>
              )}
              {book ? (
                <IconButton label="Back 15 seconds" variant="plain" size="lg" onClick={() => s.skip(-15)}>
                  <Rewind fill="currentColor" />
                </IconButton>
              ) : (
                <IconButton label="Previous" variant="plain" size="lg" onClick={() => s.prev()}>
                  <SkipBack fill="currentColor" />
                </IconButton>
              )}
              <button className="np-play" onClick={() => s.toggle()} aria-label={s.playing ? 'Pause' : 'Play'}>
                <PlayIcon fill="currentColor" />
              </button>
              {book ? (
                <IconButton label="Forward 30 seconds" variant="plain" size="lg" onClick={() => s.skip(30)}>
                  <FastForward fill="currentColor" />
                </IconButton>
              ) : (
                <IconButton label="Next" variant="plain" size="lg" onClick={() => s.next()}>
                  <SkipForward fill="currentColor" />
                </IconButton>
              )}
              {book ? (
                <IconButton label="Sleep timer" variant="plain" active={!!s.sleepAt || s.sleepEndOfChapter} onClick={sleep.open}>
                  <Moon />
                </IconButton>
              ) : (
                <IconButton label={`Repeat: ${s.repeat}`} variant="plain" active={s.repeat !== 'off'} onClick={() => s.cycleRepeat()}>
                  {s.repeat === 'one' ? <Repeat1 /> : <Repeat />}
                </IconButton>
              )}
            </div>
            <Menu
              anchor={speed.anchor}
              open={speed.isOpen}
              onClose={speed.close}
              title="Playback Speed"
              align="start"
              items={[0.75, 1, 1.1, 1.25, 1.5, 1.75, 2].map((r) => ({ label: `${r}×${r === 1 ? ' (Normal)' : ''}`, checked: s.rate === r, onSelect: () => s.setRate(r) }))}
            />
            <Menu
              anchor={sleep.anchor}
              open={sleep.isOpen}
              onClose={sleep.close}
              title="Sleep Timer"
              items={[
                { label: 'Off', checked: !s.sleepAt && !s.sleepEndOfChapter, onSelect: () => s.setSleep(null) },
                { label: 'End of chapter', checked: s.sleepEndOfChapter, onSelect: () => s.setSleep(null, true) },
                ...[5, 15, 30, 45, 60].map((m) => ({ label: `${m} minutes`, onSelect: () => (s.setSleep(m), toast(`Sleep timer set for ${m} minutes`, <Moon />)) })),
              ]}
            />
            <div className="np-volume">
              <Volume1 />
              <input
                type="range"
                className="range"
                min={0}
                max={1}
                step={0.01}
                value={s.muted ? 0 : s.volume}
                style={{ ['--pct' as string]: `${(s.muted ? 0 : s.volume) * 100}%` }}
                onChange={(e) => s.setVolume(Number(e.target.value))}
                aria-label="Volume"
              />
              <Volume2 />
            </div>
            <div className="np-tools">
              {!book && (
                <IconButton label="Lyrics" variant="plain" active={panel === 'lyrics'} disabled={!item.lyrics} onClick={() => s.setPanel('lyrics')}>
                  <MicVocal />
                </IconButton>
              )}
              <IconButton label="Play on another device" variant="plain" active={!!s.castTo} onClick={() => setCast(true)}>
                <Cast />
              </IconButton>
              <IconButton label={book ? 'Chapters' : 'Up Next'} variant="plain" active={panel === 'queue'} onClick={() => s.setPanel('queue')}>
                <ListMusic />
              </IconButton>
            </div>
            {s.castTo && (
              <div className="np-casting">
                <Cast /> Playing on {castLabel(s.castTo)}
              </div>
            )}
          </div>

          {panel && (
            <aside className="np-panel" aria-label={panel === 'lyrics' ? 'Lyrics' : book ? 'Chapters' : 'Up Next'}>
              {panel === 'lyrics' ? <Lyrics item={item} position={s.position} onSeek={s.seek} /> : <QueuePanel book={book} />}
            </aside>
          )}
        </div>
      </div>
    </div>
  );
}

function Lyrics({ item, position, onSeek }: { item: QueueItem; position: number; onSeek: (t: number) => void }) {
  const box = useRef<HTMLDivElement>(null);
  const lines = item.lyrics ?? [];
  const idx = lines.reduce((acc, l, i) => (position >= l.t ? i : acc), -1);
  useEffect(() => {
    const el = box.current?.querySelector<HTMLElement>(`[data-l="${idx}"]`);
    if (el && box.current) box.current.scrollTo({ top: el.offsetTop - box.current.clientHeight * 0.32, behavior: 'smooth' });
  }, [idx]);
  if (!lines.length) return <div className="np-empty">Lyrics aren’t available for this song.</div>;
  return (
    <div className="lyrics" ref={box}>
      {lines.map((l, i) => (
        <button key={i} data-l={i} className={cls('lyric', i === idx && 'now', i < idx && 'past')} onClick={() => onSeek(l.t)}>
          {l.text === '♪' ? <span className="lyric-dots">• • •</span> : l.text}
        </button>
      ))}
      <div className="lyrics-credit">Lyrics synced by LRCLIB · Written by the artist</div>
    </div>
  );
}

function QueuePanel({ book }: { book: boolean }) {
  const s = usePlayer();
  const [dragFrom, setDragFrom] = useState<number | null>(null);
  if (book)
    return (
      <div className="queue">
        <div className="queue-head">
          <h3>Chapters</h3>
          <span className="t-sub">{s.queue.length} chapters</span>
        </div>
        {s.queue.map((c, i) => (
          <button key={c.id} className={cls('queue-row', i === s.index && 'now', i < s.index && 'done')} onClick={() => s.jump(i)}>
            <span className="queue-num tnum">{i + 1}</span>
            <span className="queue-main">
              <span className="ellipsis">{c.title}</span>
            </span>
            <span className="queue-dur tnum">{fmtClock(c.duration)}</span>
          </button>
        ))}
      </div>
    );
  const upNext = s.queue.slice(s.index + 1);
  return (
    <div className="queue">
      <div className="queue-head">
        <h3>Playing Next</h3>
        <span className="t-sub">{plural(upNext.length, 'song')}</span>
      </div>
      {upNext.length === 0 && <div className="np-empty">Nothing queued. Add songs with “Play Next” or “Add to Queue”.</div>}
      {upNext.map((t, j) => {
        const i = s.index + 1 + j;
        return (
          <div
            key={`${t.id}-${i}`}
            className={cls('queue-row', dragFrom === i && 'dragging')}
            draggable
            onDragStart={() => setDragFrom(i)}
            onDragOver={(e) => e.preventDefault()}
            onDrop={() => {
              if (dragFrom !== null && dragFrom !== i) s.move(dragFrom, i);
              setDragFrom(null);
            }}
            onDragEnd={() => setDragFrom(null)}
          >
            <img src={t.image} alt="" className="queue-art" />
            <button className="queue-main" onClick={() => s.jump(i)}>
              <span className="ellipsis">{t.title}</span>
              <span className="t-sub ellipsis">{t.subtitle}</span>
            </button>
            <button className="icon-btn sm plain" aria-label={`Remove ${t.title}`} onClick={() => s.removeAt(i)}>
              <X />
            </button>
            <GripVertical className="queue-grip" aria-hidden="true" />
          </div>
        );
      })}
      <div className="queue-foot">
        <Gauge className="inline-ic" /> Autoplay keeps similar music going when the queue ends.
      </div>
    </div>
  );
}
