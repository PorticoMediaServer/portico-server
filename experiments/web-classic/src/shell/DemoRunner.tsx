import {useLocation} from '@tanstack/react-router';
import {useClient} from '@/data/DataProvider';
import {useActions, playableQueue} from '@/app/actions';
import {useSelection} from '@/app/selection';
import {useSession} from '@/app/session';
import {useDemo} from '@/app/demo';
import {usePlayer} from '@/player/PlayerProvider';
import {channelEntry} from '@/app/liveSources';

/** Carries out the screen index's `?demo=` states that belong to the whole app (playback, dialogs, banners). */
export function DemoRunner() {
  const client = useClient();
  const actions = useActions();
  const player = usePlayer();
  const selection = useSelection();
  const session = useSession();
  const path = useLocation().pathname;
  const id = decodeURIComponent(path.split('/')[2] ?? '');
  const entry = () => client.catalog.entry(id);
  useDemo(['rate', 'playlist', 'collection', 'download', 'delete', 'metadata', 'match', 'feedback'], name => {
    const e = entry();
    if (!e) return;
    if (name === 'rate') actions.dialog({kind: 'rate', entry: e});
    else if (name === 'playlist' || name === 'collection') actions.dialog({kind: name, entries: [e]});
    else if (name === 'download') actions.dialog({kind: 'download', entry: e});
    else if (name === 'delete') actions.dialog({kind: 'delete', entries: [e]});
    else if (name === 'metadata' || name === 'match') actions.dialog({kind: 'metadata', entries: [e], tab: name === 'match' ? 'match' : undefined});
    else actions.dialog({kind: 'feedback', entry: e});
  });
  useDemo('select', async () => {
    const page = await client.libraries.browse('lib-movies', {sort: 'title', direction: 'asc', filters: {}}, 0, 6);
    selection.start(page.entries[1]);
    selection.toggle(page.entries[3]!);
    selection.toggle(page.entries[4]!);
  });
  useDemo(['player', 'player-options', 'player-playon'], () => { const e = entry(); if (e) player.play([e], 0, {position: Math.round((e.duration ?? 1800) * 0.36)}); });
  useDemo('player-finished', () => { const e = entry(); if (e?.duration) player.play([e], 0, {position: e.duration - 2}); });
  useDemo(['player-intro', 'player-upnext', 'player-queue'], async name => {
    const show = await client.catalog.show('s-harbor-station');
    const eps = show.seasons.filter(x => x.number > 0).flatMap(x => x.episodes);
    const first = eps[0]!;
    const detail = await client.catalog.episode(first.id);
    const intro = detail.markers.find(m => m.kind === 'intro');
    const position = name === 'player-intro' ? (intro ? intro.start + 3 : 40) : name === 'player-upnext' ? (first.duration ?? 2700) - 3 : 600;
    player.play(eps, 0, {position});
  });
  useDemo(['player-live', 'player-miniguide'], async () => {
    const list = await client.live.channels();
    const ch = list[0]!;
    player.playLive(ch.id, channelEntry(ch, client.live.nowPlaying(ch.id)));
  });
  useDemo('player-problem', async () => {
    const list = await client.live.channels();
    const ch = list.find(c => c.id === 'ch-kestrel-sports') ?? list[0]!;
    player.playLive(ch.id, channelEntry(ch, client.live.nowPlaying(ch.id)));
  });
  useDemo(['now-playing', 'mini-player', 'audiobook-player'], async name => {
    const e = entry();
    if (!e) return;
    const {queue, mode} = await playableQueue(client, e);
    const withLyrics = name === 'now-playing' ? Math.max(0, client.catalog.tracks(queue.map(x => x.id)).findIndex(t => t.hasLyrics)) : 0;
    player.play(queue, withLyrics, {mode, position: name === 'audiobook-player' ? 5400 : 38});
    if (name === 'mini-player') setTimeout(() => player.minimize(), 200);
  });
  useDemo('together-room', async () => {
    await client.together.create('Friday Movie Night', 'host-only');
    await client.together.watch('m-charade-1963');
  });
  useDemo('reconnecting', () => client.session.setConnectivity('reconnecting'));
  useDemo('offline', () => client.session.setConnectivity('offline'));
  useDemo('kids', () => void session.enter('pr-theo'));
  return null;
}
