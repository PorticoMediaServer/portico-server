import type {Services} from '../../data/services';
import type {Item} from '../../data/types';
import {player} from '../../app/player';

/**
 * Starts the right kind of playback for any item: films and episodes open the video player (a show
 * plays its next episode), music and audiobooks play in the mini player and Now Playing.
 */
export async function startPlayback(services: Services, item: Item, opts: {resume?: boolean; shuffle?: boolean; from?: number} = {}) {
  const st = services.user.state(item.id);
  const resumeAt = opts.from ?? (opts.resume !== false && st.progress > 0 && !st.watched ? st.position : 0);
  switch (item.kind) {
    case 'movie': case 'homevideo': case 'episode':
      player.play(item.id, {kind: 'video', from: resumeAt});
      return;
    case 'show': {
      const id = await services.catalog.nextUp(item.id);
      if (id) player.play(id, {kind: 'video', from: services.user.state(id).progress > 0 ? services.user.state(id).position : 0});
      return;
    }
    case 'season': {
      const eps = item.episodeIds;
      const next = eps.find(e => !services.user.state(e).watched) ?? eps[0];
      if (next) player.play(eps, {kind: 'video', index: eps.indexOf(next)});
      return;
    }
    case 'album': {
      const ids = opts.shuffle ? [...item.trackIds].sort(() => Math.random() - 0.5) : item.trackIds;
      player.play(ids, {kind: 'audio', shuffle: opts.shuffle});
      return;
    }
    case 'artist': {
      const albums = await services.catalog.items(item.albumIds);
      const ids = albums.flatMap(a => (a.kind === 'album' ? a.trackIds : []));
      player.play(opts.shuffle ? ids.sort(() => Math.random() - 0.5) : ids, {kind: 'audio', shuffle: opts.shuffle});
      return;
    }
    case 'track': {
      const album = await services.catalog.item(item.albumId);
      const ids = album?.kind === 'album' ? album.trackIds : [item.id];
      player.play(ids, {kind: 'audio', index: Math.max(0, ids.indexOf(item.id))});
      return;
    }
    case 'audiobook':
      player.play(item.id, {kind: 'book', from: resumeAt});
      return;
    default:
      return;
  }
}
