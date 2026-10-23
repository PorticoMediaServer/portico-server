import {useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {legacyChannel, type GuideChannelRow, type GuideProgram} from '@core/guide/index.ts';
import {defaultI18n} from '@i18n';
import {Row} from '../../kit/layout';
import {Button} from '../../kit/controls';
import {LiveCard} from '../../kit/cards';
import {Skeleton, StateView} from '../../kit/states';
import type {ShellArtwork} from '../../shell/artwork';
import type {LiveBindings} from './bindings';
import {onNowProgramme} from './on-now-programme';
import styles from './live.module.css';

const cardWidth = 344, step = 364, t = defaultI18n.t;
/** O(visible) horizontally, including distant positions. The source supplies
 * the complete favorites-first order and total; no lineup is copied or ranked. */
export function LiveOnNow({owner, now, timezone, artwork, reducedMotion, onOpen, onPlay}: Readonly<{
  owner: LiveBindings; now: number; timezone: string; artwork: ShellArtwork; reducedMotion: boolean;
  onOpen: (channel: GuideChannelRow, programme?: GuideProgram) => void; onPlay?: (channel: GuideChannelRow) => void;
}>) {
  const current = useMemo(() => owner.onNow(), [owner]);
  useEffect(() => () => current.dispose(), [current]);
  const state = useSyncExternalStore(current.store.subscribe, current.store.getSnapshot, current.store.getSnapshot);
  const track = useRef<HTMLDivElement>(null), [window, setWindow] = useState({left: 0, width: 1});
  useLayoutEffect(() => {
    const element = track.current;if (!element) return;
    const measure = () => setWindow({left: element.scrollLeft, width: element.clientWidth});
    const observer = new ResizeObserver(measure);observer.observe(element);measure();return () => observer.disconnect();
  }, []);
  const first = Math.max(0, Math.floor(window.left / step) - 1), count = Math.ceil(window.width / step) + 2;
  const last = Math.max(first, Math.min(first + count - 1, (state.total ?? count) - 1));
  useEffect(() => current.store.setViewport({firstRow: first, lastRow: last, start: now, end: now + 1}), [current, first, last, now]);
  const shift = (direction: 1 | -1) => {
    const element = track.current;if (!element || !owner.life.current()) return;
    element.scrollBy({left: direction * Math.max(step, element.clientWidth - 128), behavior: reducedMotion ? 'instant' : 'smooth'});
  };
  if (state.total === 0) return null;
  let pending = false;
  const slots = Array.from({length: Math.max(0, last - first + 1)}, (_, offset) => {
    const index = first + offset, channel = current.store.channelAt(index), page = current.store.programsFor(index, now, now + 1);
    if (!channel || !page.complete) {
      if (pending) return null;pending = true;
      return <div className={styles.nowSlot} style={{left: index * step}} key={index}>{page.failed ? <StateView title={t('guide.tileFailed')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => current.store.retry()}}/> : <Skeleton width={cardWidth} height={204}/>}</div>;
    }
    const native = legacyChannel(channel);
    if (!native) return null;
    return <div className={styles.nowSlot} style={{left: index * step}} key={index} data-on-now-channel-id={channel.id}>
      <LiveCard large channel={{id: channel.id, sourceId: native.sourceId, name: channel.name, number: channel.number, favorite: channel.favorite,
        ...(channel.mark ? {mark: {...channel.mark, sourceId: native.sourceId, id: native.id}} : {})}}
        logo={artwork.foreground(channel.logoUrl, 68, 1)} programme={onNowProgramme(page.programs, now, defaultI18n, timezone)}
        onNavigate={() => {if (!owner.life.current()) return;if (channel.tuneAvailable && onPlay) onPlay(channel);else onOpen(channel, page.programs.find(programme => programme.start <= now && now < programme.end));}} onPlay={channel.tuneAvailable && onPlay ? () => onPlay(channel) : undefined}/>
    </div>;
  });
  return <Row className={styles.onNow} data-reduced-motion={reducedMotion || undefined} heading={t('web.live.onNow')} subtitle={t('next.live.favoritesFirst')}>
    {state.channelsError ? <StateView title={t('guide.tileFailed')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => current.store.retry()}}/>
      : <div className={styles.nowViewport} ref={track} onScroll={event => setWindow({left: event.currentTarget.scrollLeft, width: event.currentTarget.clientWidth})}>
        <div className={styles.nowSpace} style={{width: Math.max(1, state.total ?? count) * step - 20}}>{slots}</div>
      </div>}
    {window.left > 1 && <Button className={`${styles.nowArrow} ${styles.nowPrevious}`} variant="icon" icon="i-left" iconOnly aria-label={t('next.layout.previousItems')} onClick={() => shift(-1)}/>}
    {state.total !== undefined && window.left + window.width < state.total * step - 21 && <Button className={styles.nowArrow} variant="icon" icon="i-right" iconOnly aria-label={t('web.live.empty.moreTitle')} onClick={() => shift(1)}/>}
  </Row>;
}
