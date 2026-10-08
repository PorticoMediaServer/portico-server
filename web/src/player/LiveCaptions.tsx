import {useEffect, useState} from 'react';
import {IconButton} from '../ui';
import {useI18n} from '../app/i18n';

/** The broadcast caption tracks the stream carries (hls.js exposes CEA-608/708 and WebVTT renditions as text tracks). */
export function captionTracks(list: TextTrackList | ArrayLike<TextTrack> | undefined): TextTrack[] {
  const out: TextTrack[] = [];
  if (!list) return out;
  for (let i = 0; i < list.length; i++) { const t = list[i]; if (t && (t.kind === 'captions' || t.kind === 'subtitles')) out.push(t); }
  return out;
}

/**
 * FEAT-07: closed captions while watching a channel. Shown only when the live stream carries a
 * caption track; turns the first one on or all of them off. VOD subtitles keep their own panel.
 */
export function LiveCaptions({video}: {video: HTMLVideoElement | null}) {
  const i18n = useI18n();
  const [tracks, setTracks] = useState<TextTrack[]>([]);
  const [on, setOn] = useState(false);
  useEffect(() => {
    if (!video) return;
    const list = video.textTracks;
    const read = () => { const found = captionTracks(list); setTracks(found); setOn(found.some(t => t.mode === 'showing')); };
    read();
    list.addEventListener('addtrack', read);
    list.addEventListener('removetrack', read);
    list.addEventListener('change', read);
    return () => { list.removeEventListener('addtrack', read); list.removeEventListener('removetrack', read); list.removeEventListener('change', read); };
  }, [video]);
  if (!tracks.length) return null;
  const toggle = () => {
    const next = !on;
    tracks.forEach((t, i) => { t.mode = next && i === 0 ? 'showing' : 'disabled'; });
    setOn(next);
  };
  return <IconButton name="subtitles" variant="ghost" selected={on} label={i18n.t(on ? 'web.player.captionsOff' : 'web.player.captionsOn')} onClick={toggle} />;
}
