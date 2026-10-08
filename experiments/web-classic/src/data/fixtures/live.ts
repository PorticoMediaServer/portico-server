import type {Channel, Program, Recording, SeriesRule} from '../types';
import {backdropFor, catalog, channelsRaw, logoFor, stillFor, type RawChannel} from './catalog';
import {hash, pick, rng} from './util';

const MIN = 60 * 1000;
const HOUR = 60 * MIN;

type Pool = {titles: string[]; lengths: number[]; episodic?: boolean; overview: string[]; live?: boolean};
const POOLS: Record<string, Pool> = {
  news: {titles: ['Morning Edition', 'Harbor Report', 'Midday News', 'The Evening Six', 'World Tonight', 'Newsroom Live', 'Late Headlines', 'Business Hour'], lengths: [30, 60], overview: ['Headlines, weather and the stories shaping the day.', 'Live coverage and analysis from the newsroom.', 'Local reports, sport and a look at the week ahead.'], live: true},
  general: {titles: ['Harbor Station', 'The Night Desk', 'North Shore Medical', 'Kessler & Vane', 'Game Show Gold', 'The Talk Table', 'Home Again', 'Evening Magazine', 'Local Heroes'], lengths: [30, 60], episodic: true, overview: ['A new episode of the long-running series.', 'Guests, games and conversation.', 'Stories from around the region.']},
  sports: {titles: ['Premier Football: Live', 'Pro Basketball Tonight', 'College Game Day', 'Sports Desk', 'Motor Racing: Qualifying', 'Tennis: Quarterfinals', 'Baseball Live', 'Highlights Hour'], lengths: [120, 180, 60], overview: ['Live coverage from the stadium.', 'All the action, analysis and replays.', 'The day’s biggest moments.'], live: true},
  drama: {titles: ['Glass City', 'The Keepers', 'Copper County', 'Blackwood Manor', 'Deep Cover', 'Salt & Stone', 'The Quiet Ones'], lengths: [60], episodic: true, overview: ['Secrets surface as the season builds to its finale.', 'An old case is reopened.', 'Loyalties are tested.']},
  kids: {titles: ['Paper Crane Squadron', 'Harbor Witch Delivery', 'The Fox and the Lighthouse', 'Busy Little Builders', 'Story Time Garden', 'Science Kids', 'Cloud Kingdom'], lengths: [30], episodic: true, overview: ['A brand-new adventure.', 'Friends learn something new together.']},
  weather: {titles: ['Weather Now', 'Seven-Day Outlook', 'Storm Watch', 'Coastal Forecast'], lengths: [30], overview: ['Forecasts, radar and conditions across the region.'], live: true},
  music: {titles: ['Live at the Pier', 'Countdown Twenty', 'Unplugged Sessions', 'Late Night Mix', 'Studio Sessions', 'Festival Highlights'], lengths: [60, 30], overview: ['Performances from the region’s best venues.', 'The week’s most played songs.']},
  comedy: {titles: ['Stand-Up Stage', 'The Understudies', 'Brass Monkey', 'Sketch Hour', 'Roommates', 'Garden Party'], lengths: [30], episodic: true, overview: ['Laughs from the country’s best comedians.', 'The gang gets into trouble again.']},
  nature: {titles: ['Wild Coast', 'Ocean Giants', 'Planet Seasons', 'Birds of the North', 'Hidden Rivers', 'Night Hunters'], lengths: [60], episodic: true, overview: ['An intimate look at life in the wild.', 'Stunning footage from the edges of the world.']},
  history: {titles: ['The Archive', 'Lost Cities', 'Engineering Marvels', 'Battlefields', 'Ancient Voyages'], lengths: [60], episodic: true, overview: ['Historians retrace a turning point.', 'The story behind the ruins.']},
  food: {titles: ['Midnight Kitchen', 'Bake Off Club', 'Street Food Stories', 'Fire & Salt', 'Garden to Table'], lengths: [30, 60], episodic: true, overview: ['Chefs race the clock.', 'A culinary road trip.']},
  anime: {titles: ['Skyward Lantern', 'Starfield Academy', 'Mecha Garden', 'Neon Samurai 2099', 'Tidal Hearts', 'Café Constellation'], lengths: [30], episodic: true, overview: ['A new episode.', 'The tournament continues.']},
  scifi: {titles: ['Parallel', 'Signal & Noise', 'The Cartographers', 'Orbital', 'Red Shift'], lengths: [60, 120], episodic: true, overview: ['Strange signals from deep space.', 'A crew fights to get home.']},
  mystery: {titles: ['Mystery Theater', 'The Lodge', 'Hollow Creek', 'Fault Line', 'The Night Visitor'], lengths: [60, 90], episodic: true, overview: ['A detective with a talent for the impossible.', 'Nothing is what it seems.']},
  travel: {titles: ['Coast to Coast', 'Rail Journeys', 'Island Hopping', 'Mountain Towns', 'Tiny Hotels'], lengths: [30, 60], episodic: true, overview: ['A slow journey through remarkable places.']},
  public: {titles: ['Council Meeting', 'Community Voices', 'Arts in Focus', 'Science Now', 'Book Talk'], lengths: [30, 60], overview: ['Conversations with people shaping the community.']},
  talk: {titles: ['Late Show Live', 'Morning Coffee', 'The Couch', 'Tonight with Rosales'], lengths: [60], overview: ['Guests, music and conversation.'], live: true},
  retro: {titles: ['Moonrise Motel', 'Riverside', 'Tidewater', 'Open Water', 'The Weather Station'], lengths: [30, 60], episodic: true, overview: ['A classic episode.']},
  home: {titles: ['Home & Garden', 'Fixer Friday', 'Small Spaces', 'The Potting Shed', 'Dream Kitchens'], lengths: [30], episodic: true, overview: ['Inspiring ideas for every room.']},
  science: {titles: ['Science Hour', 'Deep Field', 'How It Works', 'The Lab', 'Space Now'], lengths: [60], episodic: true, overview: ['The ideas and discoveries changing the world.']},
};

function channelPrograms(ch: RawChannel, from: number, to: number): Program[] {
  const dayStart = new Date(from);
  dayStart.setHours(0, 0, 0, 0);
  let t = dayStart.getTime() - 12 * HOUR;
  const out: Program[] = [];
  const r = rng(hash(ch.id + dayStart.toDateString()));
  const movieFor = (kind: 'noir' | 'silent' | 'family' | 'western' | 'any') => {
    const list = catalog.movies.filter(m => kind === 'any' ? true : kind === 'noir' ? m.genres.includes('Crime') : kind === 'silent' ? m.year < 1930 : kind === 'family' ? m.library === 'lib-family' : m.genres.includes('Western'));
    return pick(r, list.length ? list : catalog.movies);
  };
  let n = 0;
  while (t < to + 2 * HOUR) {
    let title: string, len: number, overview: string, episodeTitle: string | undefined, season: number | undefined, episode: number | undefined, image, category = ch.category;
    const cat = ch.category;
    if (cat === 'movies' || cat === 'westerns' || cat === 'family' || ch.library) {
      const m = ch.id === 'lc-harbor' ? undefined : movieFor(ch.id === 'lc-noir' ? 'noir' : ch.id === 'lc-keaton' ? 'silent' : cat === 'family' || ch.id === 'lc-kids' ? 'family' : cat === 'westerns' ? 'western' : 'any');
      if (m) {
        title = m.title;
        len = Math.max(30, Math.ceil((m.runtime + 5) / 30) * 30);
        overview = m.overview;
        image = backdropFor(m.id);
        category = 'Movie';
      } else {
        const show = catalog.shows.find(s => s.title === 'Harbor Station')!;
        const eps = show.seasons.flatMap(s => s.episodes);
        const e = eps[n % eps.length]!;
        title = show.title; episodeTitle = e.title; season = e.season; episode = e.episode; len = 60; overview = e.overview; image = backdropFor(show.id) ?? stillFor(e.id); category = 'Series';
      }
    } else {
      const pool = POOLS[cat] ?? POOLS.general!;
      title = pick(r, pool.titles);
      len = pick(r, pool.lengths);
      overview = pick(r, pool.overview);
      if (pool.episodic) {
        season = 1 + Math.floor(r() * 5);
        episode = 1 + Math.floor(r() * 12);
        episodeTitle = pick(r, ['Arrival', 'The Offer', 'Crossing', 'Low Water', 'The Letter', 'Night Shift', 'Undertow', 'Signals', 'Homecoming', 'The Storm', 'First Light', 'Echoes', 'Last Call', 'Beacon', 'The Map', 'Salvage', 'Open House', 'The Harvest']);
      }
      image = stillFor(`${ch.id}-${title}`);
      category = cat === 'news' ? 'News' : cat === 'sports' ? 'Sports' : cat === 'kids' ? 'Kids' : cat === 'music' ? 'Music' : pool.episodic ? 'Series' : 'Special';
    }
    const start = t;
    const end = t + len * MIN;
    if (end > from - 13 * HOUR) {
      const pool = POOLS[cat];
      out.push({
        id: `${ch.id}@${start}`,
        channelId: ch.id,
        title,
        episodeTitle,
        start,
        end,
        category,
        overview,
        isNew: !!pool?.episodic && r() < 0.3,
        isLive: !!pool?.live && r() < 0.6,
        premiere: r() < 0.04,
        season,
        episode,
        rating: pick(r, ['TV-G', 'TV-PG', 'TV-14', 'TV-PG']),
        image,
        seriesId: `${ch.id}:${title}`,
      });
    }
    t = end;
    n++;
  }
  return out;
}

const cache = new Map<string, Program[]>();
/** One consistent schedule per channel from yesterday noon to three days ahead. */
const ANCHOR = (() => { const d = new Date(); d.setHours(0, 0, 0, 0); return d.getTime(); })();
export function programsFor(ch: RawChannel, from: number, to: number): Program[] {
  let list = cache.get(ch.id);
  if (!list) {
    list = channelPrograms(ch, ANCHOR, ANCHOR + 72 * HOUR);
    cache.set(ch.id, list);
  }
  return list.filter(p => p.end > from && p.start < to);
}

export function baseChannels(): Channel[] {
  return channelsRaw.map(c => ({id: c.id, number: c.number, name: c.name, logo: logoFor(c.id), group: c.group, source: c.source, hd: c.hd, favorite: ['ch-harbor-one', 'ch-classic-cinema', 'ch-kestrel-sports', 'lc-noir'].includes(c.id), hidden: c.id === 'ch-golf-tennis', library: c.library, description: c.desc}));
}

export function seedRecordings(now: number): {recordings: Recording[]; rules: SeriesRule[]} {
  const chans = channelsRaw;
  const find = (id: string) => chans.find(c => c.id === id)!;
  const rec = (id: string, chId: string, title: string, startOffsetH: number, lenMin: number, state: Recording['state'], extra: Partial<Recording> = {}): Recording => {
    const start = Math.floor((now + startOffsetH * HOUR) / (30 * MIN)) * 30 * MIN;
    return {id, title, channelId: chId, channelName: find(chId).name, start, end: start + lenMin * MIN, state, image: stillFor(id), ...extra};
  };
  const recordings: Recording[] = [
    rec('r1', 'ch-harbor-one', 'The Evening Six', -26, 60, 'completed', {size: '3.1 GB', seriesRuleId: 'rule-1', watched: true}),
    rec('r2', 'ch-harbor-one', 'The Evening Six', -2, 60, 'completed', {size: '3.0 GB', seriesRuleId: 'rule-1'}),
    rec('r3', 'ch-wild-planet', 'Ocean Giants', -50, 60, 'completed', {episodeTitle: 'The Long Migration', size: '4.4 GB', kept: true}),
    rec('r4', 'ch-classic-cinema', 'Charade', -72, 120, 'completed', {size: '7.9 GB', progress: 0.4}),
    rec('r5', 'ch-kestrel-sports', 'Premier Football: Live', -30, 180, 'incomplete', {size: '9.6 GB', reason: 'Only part of the program was recorded.'}),
    rec('r6', 'ch-cookhouse', 'Midnight Kitchen', -98, 60, 'completed', {episodeTitle: 'Fire & Ice', size: '2.8 GB', seriesRuleId: 'rule-2'}),
    rec('r7', 'ch-history-vault', 'Lost Cities', -120, 60, 'failed', {reason: 'The source changed during recording. What was captured is kept.'}),
    rec('r8', 'ch-harbor-one', 'The Evening Six', 0, 60, 'recording', {seriesRuleId: 'rule-1'}),
    rec('r9', 'ch-harbor-one', 'The Evening Six', 22, 60, 'scheduled', {seriesRuleId: 'rule-1'}),
    rec('r10', 'ch-cookhouse', 'Midnight Kitchen', 5, 60, 'scheduled', {episodeTitle: 'Night Market', seriesRuleId: 'rule-2'}),
    rec('r11', 'ch-sci-fi-signal', 'Orbital', 3, 120, 'scheduled'),
    rec('r12', 'ch-kestrel-sports', 'Pro Basketball Tonight', 3, 180, 'conflicted', {reason: 'Other recordings are using every tuner at this time.'}),
    rec('r13', 'ch-motor-nation', 'Motor Racing: Qualifying', 3, 120, 'conflicted', {reason: 'Other recordings are using every tuner at this time.'}),
    rec('r14', 'ch-wild-planet', 'Ocean Giants', 28, 60, 'scheduled', {episodeTitle: 'Kelp Forests', seriesRuleId: 'rule-3'}),
    rec('r15', 'ch-mystery-theater', 'The Lodge', 30, 60, 'scheduled'),
  ];
  const rules: SeriesRule[] = [
    {id: 'rule-1', title: 'The Evening Six', channel: 'this', channelName: 'Harbor One', newOnly: true, keep: 5, startEarly: 0, endLate: 2, paused: false, upcoming: 6, recorded: 2, priority: 3, image: stillFor('rule-1')},
    {id: 'rule-2', title: 'Midnight Kitchen', channel: 'any', newOnly: true, keep: 'all', startEarly: 1, endLate: 5, paused: false, upcoming: 2, recorded: 7, priority: 2, image: stillFor('rule-2')},
    {id: 'rule-3', title: 'Ocean Giants', channel: 'this', channelName: 'Wild Planet', newOnly: false, keep: 10, startEarly: 0, endLate: 0, paused: false, upcoming: 4, recorded: 3, priority: 1, image: stillFor('rule-3')},
    {id: 'rule-4', title: 'Late Show Live', channel: 'this', channelName: 'Late Show Live', newOnly: true, keep: 3, startEarly: 0, endLate: 10, paused: true, upcoming: 0, recorded: 12, priority: 1, image: stillFor('rule-4')},
  ];
  return {recordings, rules};
}
