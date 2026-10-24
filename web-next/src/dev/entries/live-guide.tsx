import {createRoot} from 'react-dom/client';
import {useCallback, useMemo, useState} from 'react';
import {defaultI18n} from '@i18n';
import {dayStarts, localDayStart, pxPerMsFor} from '@core/guide/index.ts';
import {channelTint, type ChannelMarkModel} from '@portico/client-core/kit-model';
import {Icon, IconSprite} from '../../kit/Icon';
import {Row} from '../../kit/layout';
import {LiveCard} from '../../kit/cards';
import {Button} from '../../kit/controls';
import {ProfileAvatarFace, TopBarActivity} from '../../kit/shell';
import {useShellWidth} from '../../shell/viewport';
import {GuideScreen, type GuideRowData} from '../../screens/live/GuideScreen';
import {onNowProgramme} from '../../screens/live/on-now-programme';
import shell from '../../shell/shell.module.css';
import live from '../../screens/live/live.module.css';
import wordmark from '../../shell/wordmark.svg';
import '../../kit/tokens.css';
import '../../kit/fonts.css';
import '../../bootstrap.css';

// The drawn demo schedule exists only in this DEV specimen. UI copy comes from the catalogue;
// the channel and programme names below are specimen content, never runtime facts.
if (!import.meta.env.DEV) throw new Error('Guide specimens require the development server.');
const t = defaultI18n.t, timezone = 'America/Halifax';
/** Wed 21 Oct 2026, 4:05 PM in the specimen's zone (UTC-3). */
const NOW = Date.UTC(2026, 9, 21, 19, 5);
const at = (hour: number, minutes: number) => Date.UTC(2026, 9, 21, hour + 3, minutes);

type SpecimenProgramme = Readonly<{id: string; title: string; from: [number, number]; to: [number, number]; episode?: string; subtitle?: string;
  flags?: Readonly<{live?: boolean; new?: boolean; premiere?: boolean}>; recording?: 'recording' | 'scheduled'}>;
type SpecimenChannel = Readonly<{id: string; number: string; name: string; group: string; mark: string; favorite?: boolean; guide?: 'none'; programmes: readonly SpecimenProgramme[]}>;

const inherit = 'The Inheritance: A Very Long Title That Should Truncate Gracefully In Every Guide Cell';  // lint-strings-allow
const channels: readonly SpecimenChannel[] = [
  {id: 'nn', number: '101', name: 'Northwind News', group: 'News', mark: 'NN', favorite: true, programmes: [  // lint-strings-allow
    {id: 'nn1', title: t('next.live.example.world'), from: [15, 30], to: [16, 30], flags: {live: true}},
    {id: 'nn2', title: 'Morning Briefing', from: [16, 30], to: [17, 30]}, // lint-strings-allow
    {id: 'nn3', title: 'Midday Report', from: [17, 30], to: [18, 0]}, // lint-strings-allow
    {id: 'nn4', title: 'Market Watch', from: [18, 0], to: [18, 30]}]}, // lint-strings-allow
  {id: 'n24', number: '102', name: t('next.live.example.northwind'), group: 'News', mark: 'N24', programmes: [  // lint-strings-allow
    {id: 'n24.1', title: 'Morning Briefing', from: [15, 30], to: [16, 30]}, // lint-strings-allow
    {id: 'n24.2', title: t('next.live.example.world'), from: [16, 30], to: [17, 30], flags: {live: true}, recording: 'scheduled'},
    {id: 'n24.3', title: 'Morning Briefing', from: [17, 30], to: [18, 30]}]}, // lint-strings-allow
  {id: 'ss', number: '201', name: 'Summit Sports', group: 'Sports', mark: 'SS', programmes: [  // lint-strings-allow
    {id: 'ss1', title: 'Cycling: Mountain Stage', from: [15, 30], to: [18, 30]}]}, // lint-strings-allow
  {id: 'sse', number: '202', name: 'Summit Sports Extra', group: 'Sports', mark: 'SSE', programmes: [  // lint-strings-allow
    {id: 'sse1', title: 'Cycling: Mountain Stage', from: [14, 0], to: [17, 0]}, // lint-strings-allow
    {id: 'sse2', title: 'Cycling: Mountain Stage', from: [17, 0], to: [20, 0]}]}, // lint-strings-allow
  {id: 'lm', number: '301', name: 'Lantern Movies', group: 'Movies', mark: 'LM', programmes: [  // lint-strings-allow
    {id: 'lm1', title: 'Beneath the Glass Sky', from: [13, 57], to: [16, 11]}, // lint-strings-allow
    {id: 'lm2', title: 'A Quiet Harbor', from: [16, 11], to: [17, 53], flags: {premiere: true}}, // lint-strings-allow
    {id: 'lm3', title: 'The Long Road North', from: [17, 53], to: [20, 20], recording: 'scheduled'}]}, // lint-strings-allow
  {id: 'lc', number: '302', name: 'Lantern Classics', group: 'Movies', mark: 'LC', programmes: [  // lint-strings-allow
    {id: 'lc1', title: 'A Quiet Harbor', from: [14, 12], to: [15, 54]}, // lint-strings-allow
    {id: 'lc2', title: "The Lighthouse Keeper's Daughter", from: [15, 54], to: [17, 52]}, // lint-strings-allow
    {id: 'lc3', title: 'Paper Moons', from: [17, 52], to: [19, 28]}]}, // lint-strings-allow
  {id: 'pk', number: '401', name: 'Pebble Kids', group: 'Kids', mark: 'PK', favorite: true, programmes: [  // lint-strings-allow
    {id: 'pk1', title: 'The Tiny Gardeners', from: [15, 15], to: [15, 45], episode: 'S4 E9', subtitle: 'The Long Night'}, // lint-strings-allow
    {id: 'pk2', title: 'Pebble and Friends', from: [15, 45], to: [16, 0], flags: {new: true}, episode: 'S3 E11', subtitle: 'The Long Night'}, // lint-strings-allow
    {id: 'pk3', title: 'Story Time Island', from: [16, 0], to: [16, 30], flags: {new: true}, episode: 'S3 E1', subtitle: 'Homecoming', recording: 'recording'}, // lint-strings-allow
    {id: 'pk4', title: 'Story Time Island', from: [16, 30], to: [17, 0], episode: 'S5 E2', subtitle: 'First Light'}, // lint-strings-allow
    {id: 'pk5', title: 'The Tiny Gardeners', from: [17, 0], to: [17, 30], episode: 'S1 E11', subtitle: 'Borrowed Time'}, // lint-strings-allow
    {id: 'pk6', title: 'Story Time Island', from: [17, 30], to: [18, 0], episode: 'S2 E4', subtitle: 'Open Doors'}, // lint-strings-allow
    {id: 'pk7', title: 'The Tiny Gardeners', from: [18, 0], to: [18, 30], episode: 'S3 E5', subtitle: 'Low Tide'}]}, // lint-strings-allow
  {id: 'ad', number: '501', name: t('next.live.example.atlas'), group: 'Documentary', mark: 'AD', favorite: true, programmes: [  // lint-strings-allow
    {id: 'ad1', title: t('next.live.example.wild'), from: [14, 45], to: [15, 45], episode: 'S5 E1', subtitle: 'Borrowed Time'}, // lint-strings-allow
    {id: 'ad2', title: 'How Bridges Stand', from: [15, 45], to: [16, 15], episode: 'S4 E12', subtitle: 'Fault Lines'}, // lint-strings-allow
    {id: 'ad3', title: t('next.live.example.clockmakers'), from: [16, 15], to: [17, 0], episode: 'S5 E9', subtitle: t('next.live.example.crossed'), recording: 'scheduled'}, // lint-strings-allow
    {id: 'ad4', title: t('next.live.example.wild'), from: [17, 0], to: [18, 0], episode: 'S1 E9', subtitle: 'Borrowed Time'}, // lint-strings-allow
    {id: 'ad5', title: 'Cities That Float', from: [18, 0], to: [18, 45], flags: {new: true}, episode: 'S2 E3', subtitle: 'Borrowed Time'}]}, // lint-strings-allow
  {id: 'tm', number: '601', name: 'Tidal Music TV', group: 'Music', mark: 'TM', programmes: [  // lint-strings-allow
    {id: 'tm1', title: 'Throwback Mix', from: [15, 30], to: [16, 30]}, // lint-strings-allow
    {id: 'tm2', title: 'Throwback Mix', from: [16, 30], to: [17, 30]}, // lint-strings-allow
    {id: 'tm3', title: 'Top 20 Countdown', from: [17, 30], to: [18, 30]}]}, // lint-strings-allow
  {id: 'hc', number: '701', name: 'Harbor City Local', group: 'Local', mark: 'HC', programmes: [  // lint-strings-allow
    {id: 'hc1', title: 'Harbor City This Morning', from: [15, 0], to: [16, 30], episode: 'S6 E7', subtitle: t('next.live.example.crossed')}, // lint-strings-allow
    {id: 'hc2', title: 'Community Kitchen', from: [16, 30], to: [17, 0], episode: 'S3 E6', subtitle: 'Fault Lines'}, // lint-strings-allow
    {id: 'hc3', title: 'Harbor City This Morning', from: [17, 0], to: [18, 30], episode: 'S5 E8', subtitle: 'First Light'}]}, // lint-strings-allow
  {id: 'cc', number: '801', name: 'Comet Comedy', group: 'Entertainment', mark: 'CC', programmes: [  // lint-strings-allow
    {id: 'cc1', title: 'The Night Shift', from: [15, 30], to: [16, 15], episode: 'S2 E9', subtitle: 'The Arrival'}, // lint-strings-allow
    {id: 'cc2', title: 'Half Past Seven', from: [16, 15], to: [16, 30], episode: 'S4 E5', subtitle: 'First Light'}, // lint-strings-allow
    {id: 'cc3', title: inherit, from: [16, 30], to: [17, 15], episode: 'S3 E9', subtitle: t('next.live.example.doors')}, // lint-strings-allow
    {id: 'cc4', title: inherit, from: [17, 15], to: [18, 0], episode: 'S4 E2', subtitle: 'Fault Lines'}, // lint-strings-allow
    {id: 'cc5', title: inherit, from: [18, 0], to: [18, 45], episode: 'S3 E7', subtitle: 'Low Tide'}]}, // lint-strings-allow
  {id: 'ed', number: '802', name: 'Ember Drama', group: 'Entertainment', mark: 'ED', programmes: [  // lint-strings-allow
    {id: 'ed1', title: 'The Night Shift', from: [15, 30], to: [16, 15], episode: 'S2 E10', subtitle: 'Fault Lines'}, // lint-strings-allow
    {id: 'ed2', title: inherit, from: [16, 15], to: [17, 0], episode: 'S5 E2', subtitle: 'Homecoming'}, // lint-strings-allow
    {id: 'ed3', title: inherit, from: [17, 0], to: [17, 45], episode: 'S2 E2', subtitle: 'Fault Lines'}, // lint-strings-allow
    {id: 'ed4', title: inherit, from: [17, 45], to: [18, 30], episode: 'S2 E6', subtitle: 'Fault Lines'}]}, // lint-strings-allow
  {id: 'qc', number: '901', name: 'Quiet Channel', group: 'Other', mark: 'QC', guide: 'none', programmes: []}, // lint-strings-allow
  {id: 'pt', number: '902', name: 'Patchwork TV', group: 'Other', mark: 'PT', programmes: [  // lint-strings-allow
    {id: 'pt1', title: 'Station Nine', from: [15, 15], to: [16, 0], episode: 'S5 E4', subtitle: t('next.live.example.crossed')}, // lint-strings-allow
    {id: 'pt2', title: inherit, from: [16, 15], to: [17, 0], episode: 'S1 E9', subtitle: 'Second Chances'}]}, // lint-strings-allow
];
const groups = ['News', 'Sports', 'Movies', 'Kids', 'Documentary', 'Music', 'Local', 'Entertainment', 'Other'];
const mark = (text: string): ChannelMarkModel => ({sourceId: 'synthetic', id: text, text, seed: text});
const episode = (text?: string) => {
  const [season, number] = text?.match(/\d+/g)?.map(Number) ?? [];
  return season !== undefined && number !== undefined ? {mode: 'seasonal' as const, season, episode: number} : undefined;
};
const program = (channel: SpecimenChannel, item: SpecimenProgramme) => ({id: item.id, channelId: channel.id, title: item.title,
  start: at(...item.from), end: at(...item.to), ...(episode(item.episode) ? {episode: episode(item.episode)} : {}), ...(item.subtitle ? {subtitle: item.subtitle} : {}),
  ...(item.flags ? {flags: item.flags} : {}), ...(item.recording ? {recording: {id: item.id, state: item.recording}} : {})});
const rows: readonly GuideRowData[] = channels.map((channel, index) => ({index,
  channel: {id: channel.id, sourceId: 'synthetic', kind: 'live' as const, number: channel.number, name: channel.name, group: channel.group,
    mark: mark(channel.mark), favorite: channel.favorite === true, tuneAvailable: true, recordAvailable: true, guide: channel.guide ?? ('full' as const)},
  programs: channel.programmes.map(item => program(channel, item)), complete: true}));

function GuideNavigation() {
  return <nav className={shell.topnav} aria-label={t('next.home.shell.main')}>
    <a className={shell.brand} href="#"><img src={wordmark} alt={t('next.platform.appName')}/></a>
    <ul className={shell.destinations}>{(['next.home.shell.home', 'search.chip.movies', 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.1.label', 'web.libraries.kindAnime', 'pref.group.music.title',
      'search.chip.audiobooks', 'next.home.shell.live'] as const).map(key => <li key={key}><a href="#" aria-current={key === 'next.home.shell.live' ? 'page' : undefined}>{t(key)}</a></li>)}</ul>
    <div className={shell.tools}>
      <button className={shell.icon} aria-label={t('next.home.shell.search')}><Icon name="i-search" size={22}/></button>
      <button className={shell.icon} aria-label={t('next.home.shell.saved')}><Icon name="i-bookmark" size={22}/></button>
      <button className={shell.icon} aria-label={t('next.home.shell.unread', {count: 3})}><Icon name="i-bell" size={22}/><span className={shell.dot}/></button>
      <TopBarActivity viewerRole="owner" open={false} onOpenChange={() => {}} view={{serverName: t('next.live.example.source'), streamCount: 2, streams: [], scans: [], tasks: [], phase: 'ready', onRetry() {}, onDashboard() {}}}/>
      <span className={shell.avatar}><ProfileAvatarFace initial="D" size={34}/></span>
    </div>
  </nav>;
}

function Specimen() {
  const width = useShellWidth(), column = width < 600 ? 60 : width < 1024 ? 180 : 284, rowHeight = width < 600 ? 72 : width < 1024 ? 62 : 76;
  const scale = pxPerMsFor(width < 1024 ? 90 : 240), [start, setStart] = useState(at(15, 30));
  const [view, setView] = useState<'guide' | 'list'>('guide');
  const [selected, setSelected] = useState<Readonly<{channelId: string; programmeId?: string}>>();
  const days = dayStarts(NOW, NOW + 2 * 86_400_000, timezone);
  const chips = useMemo(() => [{id: 'all', label: t('channels.filterAll'), count: channels.length, selected: true, onSelect: () => {}},
    {id: 'favorites', label: t('channels.favorites'), favorites: true, count: 3, selected: false, onSelect: () => {}},
    ...groups.map(group => ({id: `group:${group}`, label: group, count: channels.filter(channel => channel.group === group).length, selected: false, onSelect: () => {}}))], []);
  const onVisibleWindow = useCallback(() => {}, []);
  const onNow = <Row className={live.onNow} heading={t('web.live.onNow')} subtitle={t('next.live.favoritesFirst')}>
    <div className={live.nowViewport}>
      <div className={live.nowSpace} style={{width: 14 * 364 - 20}}>
        {[...channels.filter(channel => channel.favorite), ...channels.filter(channel => !channel.favorite)].map((channel, index) => {
          const programs = channel.programmes.map(item => program(channel, item));
          return <div className={live.nowSlot} style={{left: index * 364}} key={channel.id}>
            <LiveCard large channel={{id: channel.id, sourceId: 'synthetic', name: channel.name, number: channel.number, mark: mark(channel.mark), favorite: channel.favorite === true}}
              tint={channelTint(channel.mark)} programme={onNowProgramme(programs, NOW, defaultI18n, timezone)}
              onNavigate={() => setSelected({channelId: channel.id, programmeId: channel.programmes[0]?.id})}/>
          </div>;
        })}
      </div>
      <Button className={live.nowArrow} variant="icon" icon="i-right" iconOnly aria-label={t('web.live.empty.moreTitle')} onClick={() => {}}/>
    </div>
  </Row>;
  return <GuideScreen title={t('next.home.shell.live')}
    switcher={{value: 'all', options: [{value: 'all', label: t('channels.all')}], onChange: () => {}}}
    summary={{name: t('next.live.example.source'), channels: channels.length, through: t('next.live.guideThrough', {day: t('next.live.example.saturday')})}}
    chips={chips} onNow={onNow}
    day={{label: t('guide.day'), value: String(localDayStart(start, timezone)),
      options: days.map(day => ({value: String(day), label: day === localDayStart(NOW, timezone) ? t('guide.today') : defaultI18n.formatDate(day, {weekday: 'long', timeZone: timezone})})),
      onChange: value => {const day = days.find(day => String(day) === value);if (day !== undefined) setStart(day);}}}
    sort={{value: 'number', options: [{value: 'number', label: t('channels.sortNumber')}], onChange: () => {}}}
    view={view} onViewChange={setView}
    time={{earlier: t('guide.earlier'), now: t('guide.now'), later: t('guide.later'), onEarlier: () => setStart(value => value - 7_200_000), onNow: () => setStart(at(15, 30)), onLater: () => setStart(value => value + 7_200_000)}}
    span={{start, end: start + Math.max(1, width - column) / scale, now: NOW, scale, column, rowHeight, timezone}}
    rows={rows} total={rows.length} selected={selected}
    onSelect={(channel, programme) => setSelected({channelId: channel.id, ...(programme ? {programmeId: programme.id} : {})})}
    onVisibleWindow={onVisibleWindow}/>;
}

createRoot(document.getElementById('root')!).render(<><IconSprite /><main data-guide-specimen><GuideNavigation /><Specimen /></main></>);
