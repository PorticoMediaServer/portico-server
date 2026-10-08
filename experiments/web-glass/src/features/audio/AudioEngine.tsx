import { useEffect, useRef } from 'react';
import { usePlayer } from '../../state/player';
import { api } from '../../services/client';

/**
 * Owns the single <audio> element. Fixture streams are short loops, so position is a
 * virtual clock advanced by the element's own playback (respecting rate, buffering and
 * pauses) and mapped back onto the stream with a modulo on seek.
 */
export function AudioEngine() {
  const ref = useRef<HTMLAudioElement>(null);
  const src = usePlayer((s) => s.src);
  const playing = usePlayer((s) => s.playing);
  const seekNonce = usePlayer((s) => s.seekNonce);
  const volume = usePlayer((s) => s.volume);
  const muted = usePlayer((s) => s.muted);
  const rate = usePlayer((s) => s.rate);
  const castTo = usePlayer((s) => s.castTo);
  const last = useRef(0);
  const lastSave = useRef(0);

  useEffect(() => {
    const a = ref.current;
    if (!a || !src) return;
    if (!a.src.endsWith(src)) {
      a.src = src;
      a.load();
    }
  }, [src]);

  useEffect(() => {
    const a = ref.current;
    if (!a) return;
    if (playing && src) {
      a.play().catch(() => {
        /* autoplay blocked until a gesture: keep the virtual clock paused */
        usePlayer.setState({ playing: false });
      });
    } else a.pause();
  }, [playing, src]);

  useEffect(() => {
    const a = ref.current;
    if (!a) return;
    const apply = () => {
      const { position, mediaDuration } = usePlayer.getState();
      const d = a.duration || mediaDuration;
      if (d && Number.isFinite(d)) a.currentTime = position % d;
      last.current = a.currentTime;
    };
    if (a.readyState >= 1) apply();
    else a.addEventListener('loadedmetadata', apply, { once: true });
  }, [seekNonce]);

  useEffect(() => {
    const a = ref.current;
    if (!a) return;
    a.volume = volume;
    a.muted = muted || !!castTo;
  }, [volume, muted, castTo]);

  useEffect(() => {
    if (ref.current) ref.current.playbackRate = rate;
  }, [rate]);

  // Media Session: lock screen / hardware keys
  useEffect(() => {
    if (!('mediaSession' in navigator)) return;
    const ms = navigator.mediaSession;
    const unsub = usePlayer.subscribe((s, p) => {
      const cur = s.queue[s.index];
      const prev = p.queue[p.index];
      if (cur && cur !== prev) {
        ms.metadata = new MediaMetadata({ title: cur.title, artist: cur.subtitle, album: cur.album, artwork: [{ src: cur.image, sizes: '600x600', type: 'image/webp' }] });
      }
      if (s.playing !== p.playing) ms.playbackState = s.playing ? 'playing' : 'paused';
    });
    const st = usePlayer.getState;
    ms.setActionHandler('play', () => st().play());
    ms.setActionHandler('pause', () => st().pause());
    ms.setActionHandler('nexttrack', () => st().next());
    ms.setActionHandler('previoustrack', () => st().prev());
    ms.setActionHandler('seekforward', () => st().skip(30));
    ms.setActionHandler('seekbackward', () => st().skip(-15));
    return unsub;
  }, []);

  const onTime = () => {
    const a = ref.current;
    if (!a) return;
    const s = usePlayer.getState();
    const item = s.queue[s.index];
    if (!item || !s.playing) {
      last.current = a.currentTime;
      return;
    }
    let d = a.currentTime - last.current;
    if (d < -0.5 && a.duration) d += a.duration; // looped
    last.current = a.currentTime;
    if (d <= 0 || d > 3) return;
    const pos = s.position + d;
    if (pos >= item.duration - 0.2) {
      if (s.sleepEndOfChapter && s.mode === 'audiobook') {
        usePlayer.setState({ playing: false, sleepEndOfChapter: false, sleepAt: null });
        return;
      }
      s.next();
      return;
    }
    s.setPosition(pos);
    if (s.sleepAt && Date.now() >= s.sleepAt) usePlayer.setState({ playing: false, sleepAt: null });
    if (s.mode === 'audiobook' && s.book && Date.now() - lastSave.current > 10_000) {
      lastSave.current = Date.now();
      api.saveProgress(s.book.id, (item.offset ?? 0) + pos, s.book.duration);
    }
  };

  return (
    <audio
      ref={ref}
      loop
      preload="auto"
      onTimeUpdate={onTime}
      onWaiting={() => usePlayer.getState().setBuffering(true)}
      onPlaying={() => usePlayer.getState().setBuffering(false)}
      style={{ display: 'none' }}
    />
  );
}
