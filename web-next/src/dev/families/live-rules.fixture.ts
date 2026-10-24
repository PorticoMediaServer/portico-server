import {defaultI18n} from '@i18n';
import type {SeriesRulesView, SeriesRule, RuleAiring} from '../../screens/live/series-rules-model';
const t = defaultI18n.t;
const atlas = t('next.live.example.atlas'), news = t('next.live.example.northwind');
const airing = (id: string, time: string, season: number, episode: number, title: string, isNew: boolean): RuleAiring => ({id, time, day: t('guide.today'), channelId: 'atlas', channel: atlas, episode: t('downloads.title.episode', {season, episode}), title, isNew});
const clockAirings = [airing('clock-1', '4:15 PM', 5, 9, t('next.live.example.crossed'), false), airing('clock-2', '6:45 PM', 4, 5, t('next.live.example.doors'), false), airing('clock-3', '7:30 PM', 6, 7, t('next.live.example.crossed'), true)];
const base = {enabled: true, episodes: 'all', channel: 'any', keep: 'all', startEarly: 2, endLate: 0} as const;
const rules: readonly SeriesRule[] = [
  {id: 'clockmakers', title: t('next.live.example.clockmakers'), glyph: 'C', tone: 'documentary', channelId: 'atlas', channelName: atlas, recorded: 6, settings: base, airings: clockAirings, next: t('next.live.nextOn', {time: '4:15 PM', channel: atlas})},
  {id: 'world', title: t('next.live.example.world'), glyph: 'W', tone: 'news', channelId: 'news', channelName: news, recorded: 0, settings: {...base, channel: 'this', keep: '1'}, airings: [0, 1].map(index => ({...clockAirings[index]!, id: `news-${index}`, channelId: 'news', channel: news, title: t('next.live.example.world'), time: '4:30 PM'})), next: t('next.live.next', {time: '4:30 PM'})},
  {id: 'football', title: t('next.live.example.football'), glyph: 'S', tone: 'sport', channelId: 'sport', channelName: t('guide.anyChannel'), recorded: 0, settings: base, airings: [{...clockAirings[2]!, id: 'football-1', channelId: 'sport', channel: t('guide.anyChannel'), title: t('next.live.example.football'), time: '8:00 PM'}], next: t('next.live.nextConflict', {time: '8:00 PM'}), conflict: true},
  {id: 'wild', title: t('next.live.example.wild'), glyph: 'W', tone: 'documentary', channelId: 'atlas', channelName: atlas, recorded: 0, settings: {...base, enabled: false, episodes: 'new', channel: 'this', keep: '3'}, airings: [0, 1].map(index => ({...clockAirings[index]!, id: `wild-${index}`, title: t('next.live.example.wild'), isNew: true})), next: ''},
];
export const seriesRulesFixture: SeriesRulesView = {sourceName: t('next.live.example.source'), channelCount: 14, guideThrough: t('next.live.example.saturday'), upcomingCount: 8, recordedCount: 20, conflictCount: 1, storage: {used: 64, free: 412, hoursLeft: 230}, rules, selectedRuleId: 'clockmakers'};
