import {readFileSync} from 'node:fs';
import {defaultI18n} from '../../packages/i18n/src/index.ts';

const t = defaultI18n.t;
const sourceFiles = [
  'src/dev/families/live-rules.tsx', 'src/dev/families/live-rules.fixture.ts',
  'src/screens/live/SeriesRules.tsx', 'src/kit/dvr/RecordingsLayout.tsx',
  'src/kit/dvr/RuleCard.tsx', 'src/kit/overlays/SidePanel.tsx',
];
const preferredKeys = [...new Set(sourceFiles.flatMap(path => [...readFileSync(new URL('../../web-next/' + path, import.meta.url), 'utf8').matchAll(/\bt\(\s*['"]([^'"]+)['"]/g)].map(value => value[1])).concat([
  'next.home.shell.home', 'search.chip.movies', 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.1.label',
  'web.libraries.kindAnime', 'pref.group.music.title', 'search.chip.audiobooks', 'next.home.shell.live', 'next.overlays.close',
]))];
const message = (name, key, values) => ({name, key, values});
const titles = ['clockmakers', 'world', 'football', 'wild'].map(name => t('next.live.example.' + name));

/** Genuine existing DEV case, public fixture values and semantic selectors only. */
export const liveRules = {
  id: 'live-series-rules', state: 'clockmakers-editor-all-any-keep-all-early2-late-none',
  viewport: {width: 1440, height: 950, deviceScaleFactor: 1},
  referenceSelector: '#live-rules', actualSelector: 'body',
  readySelector: 'body:has([data-gallery-ready="true"]):has([data-live-rules]):has(aside[role="region"][data-state="open"])',
  entry: '/dev/kit?family=live-rules&case=editor&capture=true', motion: 'settled', preferredKeys,
  reference: {screen: '$root', navigation: ':scope > nav', heading: '.lv-top h1', header: '.lv-top', rules: '.rules', panel: 'aside.sheet', 'panel-title': 'aside h2', 'coming-up': 'aside h3', footer: '.sh-foot'},
  actual: {screen: '$root', navigation: '[data-live-rules] > nav', heading: '[data-live-rules] > header h1', header: '[data-live-rules] > header',
    rules: '[data-live-rules] div[class*="_rules_"]', panel: 'aside[role="region"][data-state="open"]', 'panel-title': 'aside h2', 'coming-up': 'aside h3', footer: 'aside > footer'},
  repeats: {
    reference: [{container: 'rules', items: ':scope > .rule', role: 'rule-card', parts: {copy: '.r-copy', state: '.r-state', more: '.r-act'}}],
    actual: [{container: 'rules', items: ':scope > article', role: 'rule-card', parts: {copy: ':scope > button[class*="_copy_"]', state: ':scope > div[class*="_state_"]', more: ':scope > button[class*="_more_"]'}}],
  },
  messageSlots: [
    message('header.channels', 'player.info.channels', {count: 14}),
    message('header.guideThrough', 'next.live.guideThrough', {day: t('next.live.example.saturday')}),
    message('storage.used', 'next.live.storageUsed', {amount: 64}), message('storage.free', 'next.live.storageFree', {amount: 412}),
    message('storage.left', 'next.live.storageLeft', {hours: 230}), message('storage.label', 'next.live.storageLabel', {used: 64, free: 412}),
    ...[1, 2, 3].map(count => message('rules.upcoming.' + count, 'next.live.upcomingCount', {count})),
    message('selected.matches', 'next.live.matchCount', {count: 3}), message('selected.recorded', 'next.live.recordedCount', {count: 6}),
    message('clock.next', 'next.live.nextOn', {time: '4:15 PM', channel: t('next.live.example.atlas')}),
    message('world.next', 'next.live.next', {time: '4:30 PM'}), message('football.next', 'next.live.nextConflict', {time: '8:00 PM'}),
    message('wild.paused', 'next.live.pausedMatch', {count: 2}), message('wild.keep', 'next.live.keepLatestCount', {count: 3}),
    message('early.minutes', 'next.live.minutes', {count: 2}), message('selected.label', 'next.live.editRule', {title: titles[0]}),
    ...titles.map((title, index) => message('rules.actions.' + index, 'next.live.actionsFor', {title})),
    ...[[5, 9], [4, 5], [6, 7]].map(([season, episode]) => message(`airing.${season}.${episode}`, 'downloads.title.episode', {season, episode})),
  ],
  dataValues: [
    ...['C', 'W', 'S'].map(value => ({name: 'rules.monogram.' + value, value})), {name: 'shell.viewer.initial', value: 'D'},
    ...['1', '2', '3', '4', '6', '8', '20'].map(value => ({name: 'fixture.count.' + value, value})),
    ...['4:15 PM', '6:45 PM', '7:30 PM'].map((value, index) => ({name: 'airing.time.' + index, value})),
  ],
};
