// Global audio playback: music queue and audiobooks. The <AudioEngine/> component
// owns the actual <audio> element and keeps this store in sync with it.
import { create } from 'zustand';
import type { Audiobook, Palette, Track } from '../data/types';
import { api } from '../services/client';

export interface QueueItem {
  id: string;
  kind: 'track' | 'chapter';
  title: string;
  subtitle: string;
  album?: string;
  albumId?: string;
  artistId?: string;
  image: string;
  duration: number;
  palette?: Palette;
  explicit?: boolean;
  lossless?: string;
  lyrics?: Track['lyrics'];
  /** audiobook: offset of this chapter within the book */
  bookId?: string;
  offset?: number;
}

export type Repeat = 'off' | 'all' | 'one';

interface PlayerState {
  mode: 'music' | 'audiobook' | null;
  queue: QueueItem[];
  original: QueueItem[];
  index: number;
  playing: boolean;
  position: number; // seconds within the current item
  buffering: boolean;
  src: string | null;
  mediaDuration: number;
  shuffle: boolean;
  repeat: Repeat;
  volume: number;
  muted: boolean;
  rate: number;
  sleepAt: number | null; // epoch ms
  sleepEndOfChapter: boolean;
  nowPlaying: boolean;
  panel: 'lyrics' | 'queue' | null;
  castTo: string | null;
  book: Audiobook | null;
  seekNonce: number;

  playTracks: (tracks: Track[], start?: number, opts?: { shuffle?: boolean }) => Promise<void>;
  playBook: (book: Audiobook, position?: number) => Promise<void>;
  toggle: () => void;
  play: () => void;
  pause: () => void;
  next: () => void;
  prev: () => void;
  jump: (i: number) => void;
  seek: (t: number) => void;
  skip: (d: number) => void;
  setPosition: (t: number) => void;
  setShuffle: (v: boolean) => void;
  cycleRepeat: () => void;
  setVolume: (v: number) => void;
  toggleMute: () => void;
  setRate: (r: number) => void;
  setSleep: (minutes: number | null, endOfChapter?: boolean) => void;
  enqueue: (tracks: Track[], next?: boolean) => void;
  removeAt: (i: number) => void;
  move: (from: number, to: number) => void;
  openNowPlaying: (v: boolean) => void;
  setPanel: (p: PlayerState['panel']) => void;
  setCast: (id: string | null) => void;
  stop: () => void;
  setBuffering: (b: boolean) => void;
}

const trackItem = (t: Track): QueueItem => ({
  id: t.id,
  kind: 'track',
  title: t.title,
  subtitle: t.artistName,
  album: t.albumTitle,
  albumId: t.albumId,
  artistId: t.artistId,
  image: t.art.square!,
  duration: t.duration,
  palette: t.palette,
  explicit: t.explicit,
  lossless: t.bitDepth ? `${t.codec} ${t.bitDepth}/${(t.sampleRate ?? 44100) / 1000}` : undefined,
  lyrics: t.lyrics,
});

const shuffleArr = <T,>(a: T[], keepFirst?: T) => {
  const x = a.filter((i) => i !== keepFirst);
  for (let i = x.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [x[i], x[j]] = [x[j], x[i]];
  }
  return keepFirst ? [keepFirst, ...x] : x;
};

async function sourceFor(id: string) {
  const s = await api.getPlayable(id);
  return s;
}

export const usePlayer = create<PlayerState>((set, get) => {
  const loadIndex = async (i: number, position = 0, autoplay = true) => {
    const q = get().queue;
    const item = q[i];
    if (!item) return;
    set({ index: i, position, buffering: true });
    try {
      const s = await sourceFor(item.kind === 'chapter' ? item.bookId! : item.id);
      set({ src: s.url, mediaDuration: s.mediaDuration ?? 0, playing: autoplay, buffering: false, seekNonce: get().seekNonce + 1 });
    } catch {
      set({ buffering: false, playing: false });
    }
  };

  return {
    mode: null,
    queue: [],
    original: [],
    index: 0,
    playing: false,
    position: 0,
    buffering: false,
    src: null,
    mediaDuration: 0,
    shuffle: false,
    repeat: 'off',
    volume: 0.8,
    muted: false,
    rate: 1,
    sleepAt: null,
    sleepEndOfChapter: false,
    nowPlaying: false,
    panel: null,
    castTo: null,
    book: null,
    seekNonce: 0,

    playTracks: async (tracks, start = 0, opts) => {
      const items = tracks.map(trackItem);
      const first = items[start];
      const queue = opts?.shuffle ? shuffleArr(items, first) : items;
      set({ mode: 'music', book: null, queue, original: items, shuffle: !!opts?.shuffle, rate: 1 });
      await loadIndex(opts?.shuffle ? 0 : start);
    },
    playBook: async (book, position = 0) => {
      const chapters: QueueItem[] = book.chapters.map((c, i) => ({
        id: `${book.id}#${i}`,
        kind: 'chapter',
        title: c.title,
        subtitle: book.author,
        album: book.title,
        image: book.art.square!,
        duration: c.duration,
        palette: book.palette,
        bookId: book.id,
        offset: c.start,
      }));
      const idx = Math.max(0, chapters.findIndex((c) => position >= c.offset! && position < c.offset! + c.duration));
      set({ mode: 'audiobook', book, queue: chapters, original: chapters, shuffle: false, repeat: 'off' });
      await loadIndex(idx, position - (chapters[idx]?.offset ?? 0));
    },
    toggle: () => set({ playing: !get().playing }),
    play: () => set({ playing: true }),
    pause: () => set({ playing: false }),
    next: () => {
      const { index, queue, repeat } = get();
      if (repeat === 'one') return get().seek(0);
      if (index + 1 < queue.length) loadIndex(index + 1);
      else if (repeat === 'all' && queue.length) loadIndex(0);
      else set({ playing: false, position: 0 });
    },
    prev: () => {
      const { index, position } = get();
      if (position > 4 || index === 0) get().seek(0);
      else loadIndex(index - 1);
    },
    jump: (i) => loadIndex(i),
    seek: (t) => set({ position: Math.max(0, Math.min(t, get().queue[get().index]?.duration ?? t)), seekNonce: get().seekNonce + 1 }),
    skip: (d) => {
      const { position, queue, index } = get();
      const dur = queue[index]?.duration ?? 0;
      const t = position + d;
      if (t >= dur && index + 1 < queue.length) return loadIndex(index + 1, 0);
      if (t < 0 && index > 0) return loadIndex(index - 1, Math.max(0, (queue[index - 1]?.duration ?? 0) + t));
      get().seek(t);
    },
    setPosition: (t) => set({ position: t }),
    setShuffle: (v) => {
      const { queue, original, index } = get();
      const cur = queue[index];
      if (v) {
        const q = shuffleArr(original, cur);
        set({ shuffle: true, queue: q, index: 0 });
      } else {
        const i = original.findIndex((x) => x.id === cur?.id);
        set({ shuffle: false, queue: original, index: Math.max(0, i) });
      }
    },
    cycleRepeat: () => set({ repeat: get().repeat === 'off' ? 'all' : get().repeat === 'all' ? 'one' : 'off' }),
    setVolume: (v) => set({ volume: v, muted: v === 0 }),
    toggleMute: () => set({ muted: !get().muted }),
    setRate: (r) => set({ rate: r }),
    setSleep: (minutes, endOfChapter) => set({ sleepAt: minutes ? Date.now() + minutes * 60_000 : null, sleepEndOfChapter: !!endOfChapter }),
    enqueue: (tracks, next) => {
      const items = tracks.map(trackItem);
      const { queue, index, mode } = get();
      if (!queue.length || mode !== 'music') {
        get().playTracks(tracks);
        return;
      }
      const q = next ? [...queue.slice(0, index + 1), ...items, ...queue.slice(index + 1)] : [...queue, ...items];
      set({ queue: q, original: [...get().original, ...items] });
    },
    removeAt: (i) => {
      const { queue, index } = get();
      if (i === index) return;
      set({ queue: queue.filter((_, j) => j !== i), index: i < index ? index - 1 : index });
    },
    move: (from, to) => {
      const { queue, index } = get();
      const q = [...queue];
      const [x] = q.splice(from, 1);
      q.splice(to, 0, x);
      const cur = queue[index];
      set({ queue: q, index: q.indexOf(cur) });
    },
    openNowPlaying: (v) => set({ nowPlaying: v }),
    setPanel: (p) => set({ panel: get().panel === p ? null : p }),
    setCast: (id) => set({ castTo: id }),
    stop: () => set({ mode: null, queue: [], original: [], playing: false, src: null, nowPlaying: false, book: null, position: 0 }),
    setBuffering: (b) => set({ buffering: b }),
  };
});

export const currentItem = () => {
  const s = usePlayer.getState();
  return s.queue[s.index];
};
