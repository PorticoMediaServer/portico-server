import {useEffect, type CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import {liveRulesFixtures as f} from '../../../../packages/i18n/src/catalog/en-US/next/dev/live-fixtures';
import {referenceDvrTints as tint} from '../../kit/artwork/dev-colors';
import {Icon} from '../../kit/Icon';
import {ProfileAvatarFace} from '../../kit/shell';
import {SeriesRulesScreen, type SeriesRule, type SeriesRulesModel} from '../../screens/live/series-rules';
import shellStyles from '../../shell/shell.module.css';
import activityStyles from '../../kit/shell/activity.module.css';
import wordmark from '../../shell/wordmark.svg';
import {defineGalleryFamily} from '../gallery-registry';

const t = defaultI18n.t;
// The frame's clock is 4:05 PM Atlantic on Wednesday 21 October 2026. A fixed zone keeps the
// reference capture identical on every host; real screens receive already-formatted times too.
const clock = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit', hour12: true, timeZone: 'America/Halifax'});
const at = (hour: number, minute: number) => clock.format(Date.UTC(2026, 9, 21, hour + 3, minute));

const rules: readonly SeriesRule[] = [
  {
    id: 'clockmakers', title: f['next.dev.live.clockmakers'], art: {tint: tint.clockmakers, glyph: 'C'},
    record: 'all', channel: 'any', channelName: f['next.dev.live.atlas'], keep: 'all', startEarlyMinutes: 2, endLateMinutes: 0, status: 'on',
    next: {day: 'today', time: at(16, 15), channel: f['next.dev.live.atlas']}, recordedCount: 6,
    airings: [
      {id: 'clockmakers-1', day: 'today', time: at(16, 15), season: 5, episode: 9, title: f['next.dev.live.crossedWires'], channel: f['next.dev.live.atlas']},
      {id: 'clockmakers-2', day: 'today', time: at(18, 45), season: 4, episode: 5, title: f['next.dev.live.openDoors'], channel: f['next.dev.live.atlas']},
      {id: 'clockmakers-3', day: 'today', time: at(19, 30), season: 6, episode: 7, title: f['next.dev.live.crossedWires'], channel: f['next.dev.live.atlas'], isNew: true},
    ],
  },
  {
    id: 'world-tonight', title: f['next.dev.live.worldTonight'], art: {tint: tint.worldTonight, glyph: 'W'},
    record: 'all', channel: 'this', channelName: f['next.dev.live.northwindNews24'], keep: 1, startEarlyMinutes: 2, endLateMinutes: 0, status: 'on',
    next: {day: 'today', time: at(16, 30)}, recordedCount: 12,
    airings: [
      {id: 'world-tonight-1', day: 'today', time: at(16, 30), title: f['next.dev.live.worldTonight'], channel: f['next.dev.live.northwindNews24']},
      {id: 'world-tonight-2', day: 'tomorrow', time: at(16, 30), title: f['next.dev.live.worldTonight'], channel: f['next.dev.live.northwindNews24']},
    ],
  },
  {
    id: 'summit-football', title: f['next.dev.live.summitFootball'], art: {tint: tint.summitFootball, glyph: 'S'},
    record: 'all', channel: 'any', channelName: f['next.dev.live.summitSports'], keep: 'all', startEarlyMinutes: 2, endLateMinutes: 0, status: 'on',
    next: {day: 'today', time: at(20, 0), conflict: true}, recordedCount: 4,
    airings: [{id: 'summit-football-1', day: 'today', time: at(20, 0), title: f['next.dev.live.summitFootball'], channel: f['next.dev.live.summitSports']}],
  },
  {
    id: 'wild-plains', title: f['next.dev.live.wildPlains'], art: {tint: tint.wildPlains, glyph: 'W'},
    record: 'new', channel: 'this', channelName: f['next.dev.live.atlas'], keep: 3, startEarlyMinutes: 2, endLateMinutes: 0, status: 'paused',
    pausedMatch: 2, recordedCount: 8, airings: [],
  },
];

const model: SeriesRulesModel = {
  rules,
  storage: {usedBytes: 64 * 1024 ** 3, freeBytes: 412 * 1024 ** 3, hoursLeft: 230},
  recordings: {upcomingCount: 8, recordedCount: 20, conflictCount: 1},
  source: {name: f['next.dev.live.source'], channelCount: 14, guideThrough: f['next.dev.live.guideThrough']},
  ambient: tint.ambient,
};

/** The frame's top bar is the real shell's, drawn with its own classes so the capture includes it. */
function FrameChrome() {
  const link = (label: string, current = false) => <li><a href="#" aria-current={current ? 'page' : undefined} onClick={event => event.preventDefault()}>{label}</a></li>;
  return <nav className={shellStyles.topnav} style={{position: 'absolute'}} aria-label={t('next.home.shell.main')}>
    <a className={shellStyles.brand} href="#" aria-label={t('next.platform.appName')} onClick={event => event.preventDefault()}><img src={wordmark} alt=""/></a>
    <ul className={shellStyles.destinations}>
      {link(t('next.home.shell.home'))}
      {link(f['next.dev.live.movies'])}
      {link(f['next.dev.live.tvShows'])}
      {link(f['next.dev.live.anime'])}
      {link(f['next.dev.live.music'])}
      {link(f['next.dev.live.audiobooks'])}
      {link(t('next.home.shell.live'), true)}
    </ul>
    <div className={shellStyles.tools}>
      <a className={shellStyles.icon} href="#" aria-label={t('next.home.shell.search')} onClick={event => event.preventDefault()}><Icon name="i-search" size={22}/></a>
      <a className={shellStyles.icon} href="#" aria-label={t('next.home.shell.saved')} onClick={event => event.preventDefault()}><Icon name="i-bookmark" size={22}/></a>
      <a className={shellStyles.icon} href="#" aria-label={t('next.home.shell.unread', {count: 3})} onClick={event => event.preventDefault()}><Icon name="i-bell" size={22}/><span className={shellStyles.dot}/></a>
      <button type="button" className={activityStyles.trigger} data-busy="true" data-streams="true" aria-label={t('next.shell.activityLabel', {count: 2})} aria-haspopup="dialog" aria-expanded="false">
        <Icon name="i-act" size={22}/><span className={activityStyles.streamCount}>{2}</span>
      </button>
      <span className={shellStyles.avatar} aria-label={t('next.home.shell.profileMenu', {name: f['next.dev.live.profileName']})}><ProfileAvatarFace initial={f['next.dev.live.profileInitial']} art="blue" size={34}/></span>
    </div>
  </nav>;
}

function SeriesRulesFrame() {
  // A capture shows the frame's settled state, never the panel's entry animation.
  const capture = new URLSearchParams(window.location.search).get('capture') === 'true';
  if (capture) document.documentElement.dataset.reduceMotion = 'true';
  useEffect(() => {
    if (!capture) return;
    // The kit's entrance animation leaves the panel on a composited layer, which changes
    // text antialiasing. Cancel it so the captured panel rasters like the flat frame.
    const frame = requestAnimationFrame(() => {
      for (const panel of document.querySelectorAll<HTMLElement>('aside[data-state]')) {
        for (const animation of panel.getAnimations({subtree: true})) animation.cancel();
        panel.style.opacity = '';
        panel.style.transform = '';
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [capture]);
  return <div style={{'--shell-gutter': 'var(--gutter-desktop)'} as CSSProperties}>
    <FrameChrome />
    <SeriesRulesScreen model={model} initialOpenRuleId="clockmakers" />
  </div>;
}

export default defineGalleryFamily({id: 'live', labelKey: 'next.home.shell.live', cases: [
  {id: 'series-rules', labelKey: 'web.dvr.rules', width: 1440, height: 950, render: SeriesRulesFrame},
]});
