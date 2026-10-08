import {useEffect} from 'react';
import {useLocation} from '@tanstack/react-router';
import {useServices} from '../data/hooks';
import type {Album, Season, Show} from '../data/types';
import {player} from './player';
import {together} from './together';
import {selection} from './selection';
import {actions} from '../screens/shared/actions';

/**
 * Review states on demand: `?demo=<state>` opens a player, a dialog or a problem state over the
 * current page, so every state has a URL (the screen index at /dev/screens links them all).
 */
export const DEMOS: Record<string, string> = {
  'player': 'Video player, mid-film',
  'player-intro': 'Episode with Skip intro',
  'player-options': 'Player options panel',
  'player-audio': 'Audio and subtitles panel',
  'player-chapters': 'Chapters panel',
  'player-queue': 'Up next queue panel',
  'player-playon': 'Play on another screen',
  'player-shortcuts': 'Keyboard shortcuts',
  'player-info': 'Playback info',
  'postplay': 'Up next countdown',
  'still-watching': 'Still watching?',
  'postplay-film': 'Film finished',
  'live': 'Live TV player',
  'live-guide': 'Live player mini guide',
  'owner-stopped': 'Stopped by the server owner',
  'unplayable': 'Title can’t play',
  'dolby-vision': 'Dolby Vision warning',
  'player-reconnecting': 'Player reconnecting',
  'channel-problem': 'Channel problem',
  'group': 'Player with Watch Together pill',
  'nowplaying': 'Now Playing with lyrics',
  'nowplaying-queue': 'Now Playing queue',
  'audiobook': 'Audiobook player',
  'mini': 'Mini player (music)',
  'mini-video': 'Minimized video',
  'select': 'Multi-select with the selection bar',
  'playlist': 'Add to playlist',
  'collection': 'Add to collection',
  'download': 'Download quality',
  'download-show': 'Download a show',
  'rate': 'Rate a title',
  'feedback': 'Report a problem',
  'metadata': 'Metadata editor',
  'fix-match': 'Fix match',
  'delete': 'Delete media',
};

const FILM = 'm-charade';

export function DemoHost() {
  const services = useServices();
  const loc = useLocation();
  const demo = new URLSearchParams(loc.searchStr).get('demo');
  useEffect(() => {
    if (!demo) return;
    let panel: string | null = null;
    const run = async () => {
      const show = (await services.catalog.item('s-signal-bloom')) as Show | undefined;
      const season = show ? ((await services.catalog.item(show.seasonIds[0]!)) as Season) : undefined;
      const eps = season?.episodeIds ?? [];
      switch (demo) {
        case 'player': case 'player-options': case 'player-audio': case 'player-chapters': case 'player-playon': case 'player-shortcuts': case 'player-info':
          player.play(FILM, {kind: 'video', from: 1260});
          panel = demo === 'player' ? null : demo.replace('player-', '');
          if (demo === 'player-audio') setTimeout(() => player.set({subtitle: `${FILM}-s1`}), 500);
          break;
        case 'player-queue': player.play(eps, {kind: 'video', index: 1, from: 300}); panel = 'queue'; break;
        case 'player-intro': player.play(eps, {kind: 'video', index: 2, from: 12}); break;
        case 'postplay': case 'still-watching':
          player.play(eps, {kind: 'video', index: 3, from: 0});
          setTimeout(() => player.set({position: player.get().duration, playing: false, buffering: false, streak: demo === 'still-watching' ? 3 : 0}), 60);
          break;
        case 'postplay-film':
          player.play(FILM, {kind: 'video'});
          setTimeout(() => player.set({position: player.get().duration, playing: false, buffering: false}), 60);
          break;
        case 'live': case 'live-guide': case 'channel-problem':
          player.playLive('ch-northline-news');
          if (demo === 'live-guide') panel = 'guide';
          if (demo === 'channel-problem') setTimeout(() => player.set({problem: 'channel'}), 60);
          else setTimeout(() => player.set({liveBehind: 150}), 60);
          break;
        case 'owner-stopped': player.play(FILM, {kind: 'video', from: 2000}); setTimeout(() => player.set({ownerStopped: 'Restarting the server for an update. Back in five minutes.', playing: false, buffering: false}), 60); break;
        case 'unplayable': player.play(FILM, {kind: 'video'}); setTimeout(() => player.set({problem: 'unplayable', playing: false, buffering: false}), 60); break;
        case 'dolby-vision': player.play(FILM, {kind: 'video'}); setTimeout(() => player.set({problem: 'dolbyVision', playing: false, buffering: false}), 60); break;
        case 'player-reconnecting': player.play(FILM, {kind: 'video', from: 900}); setTimeout(() => player.set({problem: 'reconnecting', buffering: false}), 60); break;
        case 'group': together.join('tg-1'); player.play(FILM, {kind: 'video', from: 600}); break;
        case 'nowplaying': case 'nowplaying-queue': case 'mini': {
          const album = (await services.catalog.item('al-lantern-parade-familiar-roads')) as Album | undefined;
          const tracks = album ? await services.catalog.items(album.trackIds) : [];
          const withLyrics = Math.max(0, tracks.findIndex(t => t.kind === 'track' && t.hasLyrics));
          if (album) player.play(album.trackIds, {kind: 'audio', index: withLyrics, from: 38});
          if (demo !== 'mini') setTimeout(() => player.set({minimized: false}), 30);
          if (demo === 'nowplaying-queue') panel = 'queue';
          break;
        }
        case 'audiobook': player.play('b-pride-and-prejudice', {kind: 'book', from: 5400}); setTimeout(() => player.set({minimized: false}), 30); break;
        case 'mini-video': player.play(FILM, {kind: 'video', from: 1800}); setTimeout(() => player.minimize(), 30); break;
        case 'select': {
          const page = await services.catalog.browse({libraryId: 'lib-movies', filters: {quick: [], fields: {}}, sort: {field: 'title', direction: 'asc'}, pageSize: 3});
          selection.start('lib-movies');
          selection.setAll(page.items.map(i => i.id));
          break;
        }
        case 'playlist': actions.addToPlaylist([FILM]); break;
        case 'collection': actions.addToCollection([FILM]); break;
        case 'download': actions.download([FILM]); break;
        case 'download-show': actions.download(['s-signal-bloom']); break;
        case 'rate': actions.rate(FILM); break;
        case 'feedback': actions.feedback(FILM); break;
        case 'metadata': actions.editMetadata([FILM]); break;
        case 'fix-match': actions.editMetadata([FILM], 'matching'); break;
        case 'delete': actions.remove([FILM]); break;
      }
      if (panel) {
        (window as unknown as {porticoPanel?: string}).porticoPanel = panel;
        setTimeout(() => window.dispatchEvent(new CustomEvent('portico:panel', {detail: panel})), 400);
      }
    };
    void run();
  }, [demo, services]);
  return null;
}
