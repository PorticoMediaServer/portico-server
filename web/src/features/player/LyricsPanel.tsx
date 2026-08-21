import { Mic2 } from '#portico-icons';
import { useCallback, useEffect, useRef, useState } from 'react';
import { activeLyricCueIndex, type LyricDocument } from './lyrics';

export function LyricsPanel({ document, currentTime }: { document: LyricDocument; currentTime: number }) {
  const [following, setFollowing] = useState(true);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const cueRefs = useRef(new Map<number, HTMLLIElement>());
  const activeIndex = document.synchronized ? activeLyricCueIndex(document.cues, currentTime) : -1;

  const scrollToActive = useCallback(() => {
    if (activeIndex < 0) return;
    window.requestAnimationFrame(() => {
      const scroller = scrollerRef.current;
      const cue = cueRefs.current.get(activeIndex);
      if (!scroller || !cue) return;
      scroller.scrollTop = Math.max(0, cue.offsetTop - (scroller.clientHeight - cue.offsetHeight) / 2);
    });
  }, [activeIndex]);

  useEffect(() => {
    setFollowing(true);
  }, [document.sourceId]);

  useEffect(() => {
    if (following) scrollToActive();
  }, [following, scrollToActive]);

  if (!document.synchronized) return <div className="lyrics-plain" aria-label="Lyrics"><div className="lyrics-heading"><strong><Mic2 /> Lyrics</strong></div><p>{document.plainText}</p></div>;

  const stopFollowing = () => setFollowing(false);
  return <div className="lyrics-synced">
    <div className="lyrics-heading"><strong><Mic2 /> Lyrics</strong>{!following && <button type="button" aria-label="Follow current lyric" onClick={() => setFollowing(true)}>Follow</button>}</div>
    <div
      ref={scrollerRef}
      className="lyrics-cues"
      role="region"
      aria-label="Synchronized lyrics"
      tabIndex={0}
      onWheel={stopFollowing}
      onTouchStart={stopFollowing}
      onPointerDown={stopFollowing}
      onKeyDown={(event) => {
        if (['ArrowDown', 'ArrowUp', 'End', 'Home', 'PageDown', 'PageUp', ' '].includes(event.key)) stopFollowing();
      }}
    >
      <ol>
        {document.cues.map((cue, index) => <li
          key={cue.id}
          ref={(element) => {
            if (element) cueRefs.current.set(index, element); else cueRefs.current.delete(index);
          }}
          className={index === activeIndex ? 'active' : index < activeIndex ? 'past' : 'future'}
          aria-current={index === activeIndex ? 'true' : undefined}
        >{cue.text || '\u00A0'}</li>)}
      </ol>
    </div>
  </div>;
}
