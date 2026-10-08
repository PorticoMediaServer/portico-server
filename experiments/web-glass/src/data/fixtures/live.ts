import type { Channel, Program, Recording, ScheduledRecording, SeriesRule } from '../types.ts';
import { NOW, art, rng } from './util.ts';

type Cat = Program['category'];
interface Slot {
  title: string;
  cat: Cat;
  mins: number;
  eps?: string[];
  summary: string;
  seed: number;
  rating?: Program['rating'];
  live?: boolean;
}

export const channels: Channel[] = [
  { id: 'ch-2', number: '2.1', name: 'Harbor 2', callSign: 'WHBR', logoColor: '#2f86bb', logoText: 'H2', source: 'HDHomeRun', hd: true, favorite: true, category: 'Local', streamId: 'coffee-run' },
  { id: 'ch-4', number: '4.1', name: 'Channel 4', callSign: 'WPOR', logoColor: '#d9434e', logoText: '4', source: 'HDHomeRun', hd: true, category: 'Local', streamId: 'coffee-run' },
  { id: 'ch-5', number: '5.1', name: 'PBN', callSign: 'WPBN', logoColor: '#4d7cf0', logoText: 'PBN', source: 'HDHomeRun', hd: true, favorite: true, category: 'Documentary', streamId: 'coffee-run' },
  { id: 'ch-7', number: '7.1', name: 'Summit 7', callSign: 'WSMT', logoColor: '#f0a02a', logoText: 'S7', source: 'HDHomeRun', hd: true, category: 'Sports', streamId: 'coffee-run' },
  { id: 'ch-9', number: '9.1', name: 'NineNet', callSign: 'WNIN', logoColor: '#8a5cf0', logoText: '9', source: 'HDHomeRun', hd: true, category: 'Entertainment', streamId: 'coffee-run' },
  { id: 'ch-11', number: '11.1', name: 'KidsTown', callSign: 'WKTN', logoColor: '#35c28a', logoText: 'KT', source: 'HDHomeRun', hd: true, favorite: true, category: 'Kids', streamId: 'coffee-run' },
  { id: 'ch-13', number: '13.1', name: 'Weather Now', callSign: 'WWXN', logoColor: '#5cb3e2', logoText: 'WX', source: 'Antenna', hd: false, category: 'News', streamId: 'coffee-run' },
  { id: 'ch-101', number: '101', name: 'Portico Classics', callSign: 'PCLS', logoColor: '#c9a86a', logoText: 'PC', source: 'IPTV', hd: true, favorite: true, category: 'Movies', streamId: 'sintel-trailer' },
  { id: 'ch-102', number: '102', name: 'Nature+', callSign: 'NATP', logoColor: '#3fae6b', logoText: 'N+', source: 'IPTV', hd: true, category: 'Documentary', streamId: 'coffee-run' },
  { id: 'ch-103', number: '103', name: 'World News 24', callSign: 'WN24', logoColor: '#e0513f', logoText: '24', source: 'IPTV', hd: true, category: 'News', streamId: 'coffee-run' },
  { id: 'ch-104', number: '104', name: 'Cook’s Table', callSign: 'COOK', logoColor: '#ef7d45', logoText: 'CT', source: 'IPTV', hd: true, category: 'Food', streamId: 'coffee-run' },
  { id: 'ch-105', number: '105', name: 'Arena Sports 2', callSign: 'ARN2', logoColor: '#1f9ad6', logoText: 'A2', source: 'IPTV', hd: true, category: 'Sports', streamId: 'coffee-run' },
  { id: 'ch-106', number: '106', name: 'Retro TV', callSign: 'RTRO', logoColor: '#e25aa8', logoText: 'RT', source: 'IPTV', hd: false, category: 'Entertainment', streamId: 'sintel-trailer' },
  { id: 'ch-107', number: '107', name: 'Chill FM', callSign: 'CHFM', logoColor: '#6f7bf7', logoText: 'FM', source: 'IPTV', hd: false, category: 'Music', streamId: 'coffee-run' },
];

const S = (title: string, cat: Cat, mins: number, summary: string, seed: number, extra: Partial<Slot> = {}): Slot => ({ title, cat, mins, summary, seed, ...extra });

const LINEUPS: Record<string, Slot[]> = {
  'ch-2': [
    S('Harbor Newsroom', 'News', 120, 'Local news, traffic and weather for greater Portland.', 97),
    S('The Harbor Hour', 'Talk', 60, 'Conversations with the people shaping the coast.', 169),
    S('News Update', 'News', 30, 'The latest headlines from the Harbor 2 newsroom.', 460),
    S('Harborview General', 'Series', 60, 'Drama at the busiest hospital in New England.', 328, { eps: ['Code Blue', 'Night Rounds', 'Second Opinion', 'Discharge', 'Triage'], rating: 'TV-14' }),
    S('Harbor 2 News', 'News', 60, 'Local and national news.', 116),
    S('Quiz Night', 'Talk', 30, 'Three contestants, one buzzer.', 220, { rating: 'TV-G' }),
    S('The Precinct', 'Series', 60, 'Detectives in Boston’s busiest district.', 340, { eps: ['Chain of Evidence', 'The Informant', 'Cold Open', 'Stakeout'], rating: 'TV-14' }),
    S('The Nightcap', 'News', 30, 'The day in review, in thirty minutes.', 98),
  ],
  'ch-4': [
    S('Maine Today', 'Talk', 120, 'Magazine show with news, cooking and guests.', 200),
    S('Court TV Live', 'Talk', 60, 'Real cases, real rulings.', 44),
    S('Channel 4 News', 'News', 30, 'Local news at the top of the hour.', 257),
    S('Blue Collar Blues', 'Series', 30, 'Sitcom about a family-run hardware store.', 411, { eps: ['Nuts and Bolts', 'Grand Reopening', 'Inventory', 'The Big Sale'], rating: 'TV-PG' }),
    S('College Football: Maine vs. New Hampshire', 'Sports', 210, 'The Battle for the Brice-Cowell Musket, live from Orono.', 204, { live: true }),
    S('Late Show with Danny Price', 'Talk', 60, 'Comedy, music and celebrity guests.', 406),
  ],
  'ch-5': [
    S('Planet Shoreline', 'Documentary', 60, 'The world’s coastlines, from tidal pools to fjords.', 35, { eps: ['Fjords', 'Mangroves', 'Coral Coasts', 'Ice Shelves'] }),
    S('Secrets of the Deep', 'Documentary', 60, 'Exploring the least-known habitats on Earth.', 184, { eps: ['The Midnight Zone', 'Trenches', 'Vents'] }),
    S('Nova Frontier', 'Documentary', 60, 'The frontier of science, explained.', 280, { eps: ['Dark Matter', 'Quantum Biology', 'Exoplanets'] }),
    S('Railway Journeys', 'Documentary', 30, 'Slow travel across Europe by train.', 24),
    S('Antiques Hunt', 'Talk', 60, 'Treasures from attics across New England.', 183),
    S('PBN NewsHour', 'News', 60, 'In-depth reporting and analysis.', 250),
  ],
  'ch-7': [
    S('SportsDesk', 'Sports', 60, 'Highlights and analysis.', 245),
    S('NHL: Boston at Montreal', 'Sports', 180, 'Original Six rivalry night at the Bell Centre.', 400, { live: true }),
    S('Tennis: Tokyo Open Final', 'Sports', 150, 'Women’s singles final.', 463, { live: true }),
    S('Premier League Review', 'Sports', 60, 'Every goal from the weekend.', 259),
    S('Marathon Live', 'Sports', 120, 'Live coverage from the Portland Marathon.', 34, { live: true }),
  ],
  'ch-9': [
    S('Movie: Northbound', 'Movie', 120, 'Estranged sisters drive a 1978 camper from Texas to the Arctic Circle.', 4, { rating: 'PG-13' }),
    S('The Chase', 'Series', 60, 'A bounty hunter who never loses a mark.', 451, { eps: ['Pursuit', 'Crossroads', 'Dead End'], rating: 'TV-14' }),
    S('Love on the Line', 'Series', 60, 'Reality dating show on a cross-country train.', 21, { rating: 'TV-14' }),
    S('Movie: Everything Is Weather', 'Movie', 105, 'A never-wrong meteorologist is wrong about everything else.', 36, { rating: 'PG-13' }),
    S('Dispatch', 'Series', 60, 'Emergency dispatchers in Chicago.', 352, { eps: ['Mayday', 'Signal 30', 'Rollover'], rating: 'TV-14' }),
  ],
  'ch-11': [
    S('Little Explorers', 'Kids', 30, 'Pip and Juniper go looking for the smallest adventures.', 424, { eps: ['Snail Mail', 'Cloud Spotting', 'Rock Pools'], rating: 'TV-Y' }),
    S('Rocket & Roo', 'Kids', 30, 'A kangaroo and a robot build things that fly.', 589, { rating: 'TV-Y7' }),
    S('Tiny Chefs', 'Kids', 30, 'Kids cook for their families.', 115, { rating: 'TV-G' }),
    S('The Bubble Bunch', 'Kids', 30, 'Underwater friends solve problems together.', 150, { rating: 'TV-Y' }),
    S('Story Time Live', 'Kids', 30, 'Picture books read aloud.', 383, { rating: 'TV-Y' }),
  ],
  'ch-13': [S('Weather Now', 'News', 60, 'Continuous forecast, radar and marine conditions.', 377)],
  'ch-101': [
    S('The General (1926)', 'Movie', 90, 'Buster Keaton chases his stolen locomotive across enemy lines.', 32, { rating: 'NR' }),
    S('His Girl Friday (1940)', 'Movie', 95, 'A newspaper editor schemes to win back his ace reporter.', 44, { rating: 'NR' }),
    S('Charade (1963)', 'Movie', 115, 'Grant and Hepburn in Paris, pursued for a stolen fortune.', 553, { rating: 'NR' }),
    S('Detour (1945)', 'Movie', 70, 'A hitchhiking pianist’s luck runs out.', 223, { rating: 'NR' }),
    S('Sherlock Jr. (1924)', 'Movie', 50, 'A projectionist dreams himself into the movie.', 21, { rating: 'NR' }),
  ],
  'ch-102': [
    S('Wild Country', 'Documentary', 90, 'A red deer herd through the seasons in the Scottish Highlands.', 356, { rating: 'TV-G' }),
    S('Small Wonders', 'Documentary', 60, 'Macro cameras and the lives of tiny creatures.', 105, { eps: ['Drifters', 'Underfoot', 'After Dark'], rating: 'TV-G' }),
    S('Big Cats Week', 'Documentary', 60, 'Tigers, jaguars and snow leopards.', 158, { rating: 'TV-PG' }),
    S('Wings', 'Documentary', 30, 'Migration season across the Americas.', 443, { rating: 'TV-G' }),
  ],
  'ch-103': [
    S('World News 24', 'News', 60, 'Live international news.', 250),
    S('Markets Now', 'News', 30, 'Global markets update.', 460),
    S('The Briefing', 'News', 30, 'Top stories and analysis.', 201),
  ],
  'ch-104': [
    S('Open Flame', 'Food', 60, 'Home cooks compete over open fire.', 196, { eps: ['Wet Wood', 'Highland Game', 'Night Market'], rating: 'TV-PG' }),
    S('Weeknight Kitchen', 'Food', 30, 'Thirty-minute dinners.', 409),
    S('Bake Along', 'Food', 30, 'Bread, pastry and patience.', 329),
    S('Street Food Atlas', 'Food', 60, 'The world’s best street food, stall by stall.', 258, { eps: ['Bangkok', 'Mexico City', 'Istanbul'] }),
  ],
  'ch-105': [
    S('Champions League Tonight', 'Sports', 60, 'Preview and analysis.', 254),
    S('Football: Porto vs. Ajax', 'Sports', 120, 'Group stage, live from the Estádio do Dragão.', 259, { live: true }),
    S('Cycling: Vuelta Stage 18', 'Sports', 180, 'Mountain stage to the Alto de l’Angliru.', 93, { live: true }),
    S('Golf Classics', 'Sports', 60, 'Great rounds revisited.', 84),
  ],
  'ch-106': [
    S('Space Rangers', 'Series', 30, 'Sixties sci-fi adventures aboard the SS Horizon.', 373, { eps: ['The Moon Menace', 'Signal from Saturn', 'Crater City'], rating: 'TV-G' }),
    S('Diner Days', 'Series', 30, 'Seventies sitcom set in a New Jersey diner.', 169, { eps: ['Short Order', 'The Health Inspector', 'Blue Plate Special'], rating: 'TV-G' }),
    S('Mystery Theater', 'Series', 60, 'Anthology whodunits from 1958.', 283, { rating: 'TV-PG' }),
  ],
  'ch-107': [S('Chill FM', 'Music', 60, 'Lo-fi, ambient and downtempo — around the clock.', 291)],
};

const startOfWindow = (() => {
  const d = new Date(NOW);
  d.setMinutes(d.getMinutes() < 30 ? 0 : 30, 0, 0);
  return d.getTime() - 6 * 3_600_000;
})();

export const GUIDE_START = startOfWindow;
export const GUIDE_HOURS = 30;

export const programs: Program[] = channels.flatMap((ch) => {
  const lineup = LINEUPS[ch.id];
  const r = rng(ch.id);
  const out: Program[] = [];
  let t = GUIDE_START - Math.floor(r() * 3) * 30 * 60_000;
  let i = Math.floor(r() * lineup.length);
  let n = 0;
  while (t < GUIDE_START + GUIDE_HOURS * 3_600_000) {
    const slot = lineup[i % lineup.length];
    const epTitle = slot.eps ? slot.eps[(n + i) % slot.eps.length] : undefined;
    const start = t;
    const end = t + slot.mins * 60_000;
    out.push({
      id: `${ch.id}-${n}`,
      channelId: ch.id,
      title: slot.title,
      episodeTitle: epTitle,
      summary: slot.summary,
      start: new Date(start).toISOString(),
      end: new Date(end).toISOString(),
      category: slot.cat,
      isNew: !!slot.eps && r() > 0.5,
      isLive: slot.live,
      rating: slot.rating,
      image: art.program(`s${slot.seed}`),
      seriesId: slot.eps ? `series-${slot.title.toLowerCase().replace(/[^a-z0-9]+/g, '-')}` : undefined,
    });
    t = end;
    i++;
    n++;
  }
  return out;
});

export const programSeeds = Array.from(new Set(Object.values(LINEUPS).flat().map((s) => s.seed)));

const ago = (h: number) => new Date(NOW - h * 3_600_000).toISOString();

export const recordings: Recording[] = [
  { id: 'rec-1', title: 'Planet Shoreline', episodeTitle: 'Fjords', channelName: 'PBN', recordedAt: ago(20), duration: 3600, sizeBytes: 4.1e9, image: art.program('s35'), status: 'completed', progress: 0.35 },
  { id: 'rec-2', title: 'Harborview General', episodeTitle: 'Night Rounds', channelName: 'Harbor 2', recordedAt: ago(28), duration: 3600, sizeBytes: 5.6e9, image: art.program('s328'), status: 'completed' },
  { id: 'rec-3', title: 'NHL: Boston at Montreal', channelName: 'Summit 7', recordedAt: ago(48), duration: 10800, sizeBytes: 14.2e9, image: art.program('s400'), status: 'completed', watched: true },
  { id: 'rec-4', title: 'Open Flame', episodeTitle: 'Highland Game', channelName: 'Cook’s Table', recordedAt: ago(52), duration: 3600, sizeBytes: 3.2e9, image: art.program('s196'), status: 'completed' },
  { id: 'rec-5', title: 'Little Explorers', episodeTitle: 'Rock Pools', channelName: 'KidsTown', recordedAt: ago(60), duration: 1800, sizeBytes: 1.4e9, image: art.program('s424'), status: 'completed', watched: true },
  { id: 'rec-6', title: 'Secrets of the Deep', episodeTitle: 'The Midnight Zone', channelName: 'PBN', recordedAt: ago(75), duration: 3600, sizeBytes: 4.4e9, image: art.program('s184'), status: 'completed' },
  { id: 'rec-7', title: 'Charade (1963)', channelName: 'Portico Classics', recordedAt: ago(96), duration: 6900, sizeBytes: 6.1e9, image: art.program('s553'), status: 'completed', progress: 0.62 },
  { id: 'rec-8', title: 'The Precinct', episodeTitle: 'Chain of Evidence', channelName: 'Harbor 2', recordedAt: ago(120), duration: 3600, sizeBytes: 5.2e9, image: art.program('s340'), status: 'partial', failureReason: 'Signal lost for 3 min 12 s (tuner 2, weak signal on 2.1)' },
  { id: 'rec-9', title: 'Nova Frontier', episodeTitle: 'Exoplanets', channelName: 'PBN', recordedAt: ago(140), duration: 3600, sizeBytes: 4.0e9, image: art.program('s280'), status: 'completed' },
  { id: 'rec-10', title: 'Street Food Atlas', episodeTitle: 'Mexico City', channelName: 'Cook’s Table', recordedAt: ago(160), duration: 3600, sizeBytes: 3.3e9, image: art.program('s258'), status: 'completed', watched: true },
  { id: 'rec-11', title: 'Premier League Review', channelName: 'Summit 7', recordedAt: ago(170), duration: 3600, sizeBytes: 5.0e9, image: art.program('s259'), status: 'failed', failureReason: 'All tuners busy — conflicted with 2 higher-priority recordings' },
  { id: 'rec-12', title: 'Wild Country', channelName: 'Nature+', recordedAt: ago(200), duration: 5400, sizeBytes: 6.8e9, image: art.program('s356'), status: 'completed' },
];

export const seriesRules: SeriesRule[] = [
  { id: 'rule-1', title: 'Planet Shoreline', channels: ['ch-5'], newOnly: true, keep: 'all', paddingBefore: 1, paddingAfter: 3, priority: 1, image: art.program('s35'), upcoming: 3 },
  { id: 'rule-2', title: 'Harborview General', channels: 'any', newOnly: true, keep: 5, paddingBefore: 0, paddingAfter: 2, priority: 2, image: art.program('s328'), upcoming: 2 },
  { id: 'rule-3', title: 'Open Flame', channels: ['ch-104'], newOnly: false, keep: 10, paddingBefore: 0, paddingAfter: 0, priority: 3, image: art.program('s196'), upcoming: 4 },
  { id: 'rule-4', title: 'Little Explorers', channels: 'any', newOnly: false, keep: 'all', paddingBefore: 0, paddingAfter: 0, priority: 4, image: art.program('s424'), upcoming: 6 },
  { id: 'rule-5', title: 'The Precinct', channels: ['ch-2'], newOnly: true, keep: 3, paddingBefore: 2, paddingAfter: 5, priority: 5, image: art.program('s340'), upcoming: 1 },
];

export const initialScheduled = (): ScheduledRecording[] => {
  const wanted = new Set(['Planet Shoreline', 'Harborview General', 'Open Flame', 'Little Explorers', 'The Precinct', 'NHL: Boston at Montreal']);
  const future = programs.filter((p) => Date.parse(p.start) > NOW && wanted.has(p.title)).slice(0, 12);
  return future.map((p, i) => {
    const ch = channels.find((c) => c.id === p.channelId)!;
    const rule = seriesRules.find((r) => r.title === p.title);
    return {
      id: `sch-${p.id}`,
      programId: p.id,
      title: p.title,
      episodeTitle: p.episodeTitle,
      channelId: ch.id,
      channelName: ch.name,
      start: p.start,
      end: p.end,
      ruleId: rule?.id,
      conflict: i === 4 ? 'Overlaps 2 other recordings — 3 of 4 tuners in use' : undefined,
    };
  });
};
