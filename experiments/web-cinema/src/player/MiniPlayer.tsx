import {useQuery, useServices} from '../data/hooks';
import type {Audiobook, Episode, TrackItem} from '../data/types';
import {player, usePlayer} from '../app/player';
import {Art} from '../ui/Media';
import {Icon} from '../ui/Icon';
import {Tooltip} from '../ui/Overlay';
import {cx} from '../ui/cx';
import {episodeCode} from '../screens/shared/items';

/**
 * The mini player docked at the bottom (above the tab bar on phones): what's playing, the basic
 * controls and a thin progress line. Opens Now Playing, or brings a minimized video back.
 */
export function MiniPlayer() {
  const services = useServices();
  const s = usePlayer();
  const item = useQuery(s.itemId ? `item.${s.itemId}` : null, () => services.catalog.item(s.itemId!)).data;
  const channels = useQuery(s.kind === 'live' ? 'live.channels' : null, () => services.live.channels()).data;
  const channel = channels?.find(c => c.id === s.channelId);
  const video = s.kind === 'video' || s.kind === 'live';
  const book = item?.kind === 'audiobook' ? (item as Audiobook) : null;
  const chapter = book ? [...book.chapters].reverse().find(c => c.start <= s.position) : null;
  const title = s.kind === 'live' ? channel?.name ?? 'Live TV' : book ? book.title : item?.title ?? '';
  const sub = s.kind === 'live' ? 'Live' : book ? chapter?.title ?? book.author : item?.kind === 'track' ? (item as TrackItem).artistName : item?.kind === 'episode' ? episodeCode(item as Episode) : item?.year ? String(item.year) : '';
  const art = s.kind === 'live' ? channel?.logo : video ? item?.art.backdrop ?? (item as Episode | undefined)?.art.still : book ? book.art.poster : item?.art.square;
  const open = () => player.set({minimized: false, pip: false});
  return (
    <div className={cx('mini', video && 'is-video')} role="region" aria-label="Now playing">
      <span className="mini-progress" aria-hidden><span style={{width: s.duration ? `${(s.position / s.duration) * 100}%` : s.kind === 'live' ? '100%' : '0%'}} /></span>
      <button type="button" className="mini-open" onClick={open} aria-label={video ? `Back to the player: ${title}` : `Open Now Playing: ${title}`}>
        <Art src={art} tint={item?.tint ?? channel?.tint} className={cx('mini-art', video ? 'is-wide' : book ? 'is-book' : '')} ratio={video ? '16/9' : book ? '2/3' : '1/1'} eager />
        <span className="mini-text"><strong>{title}</strong><span>{sub}</span></span>
      </button>
      <div className="mini-controls">
        {!video && !book ? <Tooltip label="Previous"><button type="button" className="np-btn is-sm hide-phone" onClick={() => player.previous()} aria-label="Previous"><Icon name="previous" size={18} /></button></Tooltip> : null}
        {book || video ? <Tooltip label={book ? 'Back 15 seconds' : 'Back 10 seconds'}><button type="button" className="np-btn is-sm hide-phone" onClick={() => player.skip(book ? -15 : -10)} aria-label="Back"><Icon name="rewind" size={18} /></button></Tooltip> : null}
        <Tooltip label={s.playing ? 'Pause' : 'Play'}><button type="button" className="np-btn mini-play" onClick={() => player.toggle()} aria-label={s.playing ? 'Pause' : 'Play'}><Icon name={s.playing ? 'pause' : 'play'} size={20} fill="currentColor" /></button></Tooltip>
        {!video ? <Tooltip label={book ? 'Forward 30 seconds' : 'Next'}><button type="button" className="np-btn is-sm" onClick={() => (book ? player.skip(30) : player.next())} aria-label={book ? 'Forward 30 seconds' : 'Next'}><Icon name={book ? 'fastForward' : 'next'} size={18} /></button></Tooltip> : null}
        <Tooltip label={video ? 'Expand' : 'Open Now Playing'}><button type="button" className="np-btn is-sm hide-phone" onClick={open} aria-label="Expand"><Icon name={video ? 'fullscreen' : 'chevronUp'} size={18} /></button></Tooltip>
        <Tooltip label="Stop"><button type="button" className="np-btn is-sm" onClick={() => player.stop()} aria-label="Stop and close"><Icon name="close" size={18} /></button></Tooltip>
      </div>
    </div>
  );
}
