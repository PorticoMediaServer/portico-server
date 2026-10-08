import {useEffect, useState} from 'react';
import {useQuery, useServices} from '../data/hooks';
import type {Episode, Movie} from '../data/types';
import {player, usePlayer} from '../app/player';
import {Art} from '../ui/Media';
import {Icon} from '../ui/Icon';
import {actions} from '../screens/shared/actions';
import {episodeCode, hrefFor} from '../screens/shared/items';

const COUNTDOWN = 10;

/**
 * After a title ends: the next episode counts down (or asks "Still watching?" after several in a
 * row); a film offers Replay, Rate this and what to watch next.
 */
export function PostPlay({item, onClose, onBrowse}: {item: Movie | Episode; onClose: () => void; onBrowse: (to: string) => void}) {
  const services = useServices();
  const s = usePlayer();
  const nextId = s.queue[s.index + 1];
  const next = useQuery(nextId ? `item.${nextId}` : null, () => services.catalog.item(nextId!)).data as Episode | undefined;
  const related = useQuery(!nextId ? `related.${item.id}` : null, () => services.catalog.related(item.id).then(ids => services.catalog.items(ids.slice(0, 4)))).data ?? [];
  const still = s.streak >= 2;
  const [left, setLeft] = useState(COUNTDOWN);
  const [cancelled, setCancelled] = useState(false);
  useEffect(() => {
    if (!next || still || cancelled) return;
    if (left <= 0) { player.next(true); return; }
    const t = setTimeout(() => setLeft(l => l - 1), 1000);
    return () => clearTimeout(t);
  }, [left, next, still, cancelled]);
  if (next) {
    return (
      <div className="postplay is-next" role="dialog" aria-label={still ? 'Still watching?' : 'Up next'}>
        <div className="pp-card">
          <div className="pp-card-art">
            <Art src={next.art.still ?? next.art.backdrop} tint={next.tint} ratio="16/9" />
            {!still && !cancelled ? <svg className="pp-ring" viewBox="0 0 44 44" aria-hidden><circle cx="22" cy="22" r="20" /><circle cx="22" cy="22" r="20" style={{strokeDashoffset: `${(left / COUNTDOWN) * 125.6}`}} /></svg> : null}
          </div>
          <div className="pp-card-text">
            <span className="eyebrow">{still ? 'Still watching?' : 'Up next'}</span>
            <strong className="pp-card-title">{next.title}</strong>
            <span className="faint">{next.kind === 'episode' ? episodeCode(next) : ''}</span>
            <p className="muted clamp-2">{still ? 'Playback paused after several episodes. Confirm to keep going.' : next.synopsis}</p>
            <div className="hstack">
              <button type="button" className="btn btn-primary btn-md" onClick={() => player.next()}><Icon name="play" size={18} fill="currentColor" /><span className="btn-label">{still ? 'I’m still watching' : cancelled ? 'Play now' : `Play now · ${left}s`}</span></button>
              {!still && !cancelled ? <button type="button" className="btn btn-glass btn-md" onClick={() => setCancelled(true)}><span className="btn-label">Cancel</span></button> : <button type="button" className="btn btn-glass btn-md" onClick={onClose}><span className="btn-label">Close</span></button>}
            </div>
          </div>
        </div>
      </div>
    );
  }
  return (
    <div className="postplay" role="dialog" aria-label="Finished">
      <div className="postplay-inner">
        <span className="eyebrow">Finished</span>
        <h2 className="display postplay-title">{item.title}</h2>
        <div className="hstack">
          <button type="button" className="btn btn-primary btn-lg" onClick={() => { player.seek(0); player.resume(); }}><Icon name="rewind" size={20} /><span className="btn-label">Replay</span></button>
          <button type="button" className="btn btn-glass btn-lg" onClick={() => actions.rate(item.id)}><Icon name="star" size={20} /><span className="btn-label">Rate this</span></button>
          <button type="button" className="btn btn-glass btn-lg" onClick={onClose}><span className="btn-label">Close</span></button>
        </div>
        {related.length ? (
          <>
            <h3 className="shelf-title" style={{marginTop: 28}}>More like this</h3>
            <div className="postplay-grid">
              {related.map(r => (
                <button key={r.id} type="button" className="card card-landscape" onClick={() => onBrowse(hrefFor(r as never))}>
                  <span className="card-frame"><Art src={r.art.backdrop ?? r.art.poster} tint={r.tint} /></span>
                  <span className="card-caption"><span className="card-title">{r.title}</span><span className="card-sub">{r.year}</span></span>
                </button>
              ))}
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}
