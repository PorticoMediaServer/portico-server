import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '../../packages/i18n/src/index.ts';
import {
  initialSeriesRulesState, ruleNextLabel, ruleOptionLabels, ruleStatusLabel, seriesRulesReducer, storageFraction, visibleAirings,
  type SeriesRule, type SeriesRulesModel,
} from '../src/screens/live/series-rules/model.ts';

const t = defaultI18n.t;
const at = {early: '4:15 PM', world: '4:30 PM', late: '6:45 PM', newShow: '7:30 PM', football: '8:00 PM'};

const clockmakers: SeriesRule = {
  id: 'clockmakers', title: 'The Clockmakers', art: {tint: '#2f6250', glyph: 'C'},
  record: 'all', channel: 'any', channelName: 'Atlas Documentaries', keep: 'all', startEarlyMinutes: 2, endLateMinutes: 0, status: 'on',
  next: {day: 'today', time: at.early, channel: 'Atlas Documentaries'}, recordedCount: 6,
  airings: [
    {id: 'one', day: 'today', time: at.early, season: 5, episode: 9, title: 'Crossed Wires', channel: 'Atlas Documentaries'},
    {id: 'two', day: 'today', time: at.late, season: 4, episode: 5, title: 'Open Doors', channel: 'Atlas Documentaries'},
    {id: 'three', day: 'today', time: at.newShow, season: 6, episode: 7, title: 'Crossed Wires', channel: 'Atlas Documentaries', isNew: true},
  ],
};
const worldTonight: SeriesRule = {
  id: 'world-tonight', title: 'World Tonight', art: {tint: '#6e3446', glyph: 'W'},
  record: 'all', channel: 'this', channelName: 'Northwind News 24', keep: 1, startEarlyMinutes: 2, endLateMinutes: 0, status: 'on',
  next: {day: 'today', time: at.world}, recordedCount: 12,
  airings: [
    {id: 'world-one', day: 'today', time: at.world, title: 'World Tonight', channel: 'Northwind News 24'},
    {id: 'world-two', day: 'tomorrow', time: at.world, title: 'World Tonight', channel: 'Northwind News 24'},
  ],
};
const summitFootball: SeriesRule = {
  id: 'summit-football', title: 'Summit Football', art: {tint: '#7d5a2a', glyph: 'S'},
  record: 'all', channel: 'any', channelName: 'Summit Sports', keep: 'all', startEarlyMinutes: 2, endLateMinutes: 0, status: 'on',
  next: {day: 'today', time: at.football, conflict: true}, recordedCount: 4,
  airings: [{id: 'summit-one', day: 'today', time: at.football, title: 'Summit Football', channel: 'Summit Sports'}],
};
const wildPlains: SeriesRule = {
  id: 'wild-plains', title: 'Wild Plains', art: {tint: '#2f6250', glyph: 'W'},
  record: 'new', channel: 'this', channelName: 'Atlas Documentaries', keep: 3, startEarlyMinutes: 2, endLateMinutes: 0, status: 'paused',
  pausedMatch: 2, recordedCount: 8, airings: [],
};
const model: SeriesRulesModel = {
  rules: [clockmakers, worldTonight, summitFootball, wildPlains],
  storage: {usedBytes: 64 * 1024 ** 3, freeBytes: 412 * 1024 ** 3, hoursLeft: 230},
  recordings: {upcomingCount: 8, recordedCount: 20, conflictCount: 1},
  source: {name: 'Synthetic Live TV', channelCount: 14, guideThrough: 'Saturday'},
  ambient: ['#2f5f73', '#7a3b3b', '#2f6250'],
};

test('rules state opens only a real requested rule', () => {
  const opened = initialSeriesRulesState(model, 'clockmakers');
  assert.equal(opened.selectedRuleId, 'clockmakers');
  assert.deepEqual(opened.panel && visibleAirings(opened.panel).map(value => value.id), ['one', 'two', 'three']);
  const absent = initialSeriesRulesState(model, 'missing');
  assert.equal(absent.selectedRuleId, undefined);
  assert.equal(absent.panel, undefined);
});

test('choosing another card swaps the editor to that rule and its saved draft', () => {
  const opened = initialSeriesRulesState(model, 'clockmakers');
  const next = seriesRulesReducer(opened, {type: 'open', ruleId: 'summit-football'});
  assert.equal(next.selectedRuleId, 'summit-football');
  assert.equal(next.panel?.title, 'Summit Football');
  assert.equal(next.panel?.draft.record, 'all');
  assert.equal(next.panel?.draft.channel, 'any');
  assert.equal(next.panel?.recordedCount, 4);
});

test('the switch changes only the draft until Save commits it', () => {
  const opened = initialSeriesRulesState(model, 'clockmakers');
  const toggled = seriesRulesReducer(opened, {type: 'set-on', value: false});
  assert.equal(toggled.panel?.draft.on, false);
  assert.equal(toggled.rules.find(rule => rule.id === 'clockmakers')?.status, 'on');
  const saved = seriesRulesReducer(toggled, {type: 'save'});
  assert.equal(saved.selectedRuleId, undefined);
  assert.equal(saved.rules.find(rule => rule.id === 'clockmakers')?.status, 'paused');
  assert.equal(ruleStatusLabel(saved.rules[0]!, t), 'Paused');
});

test('Cancel closes without committing any draft change', () => {
  const opened = initialSeriesRulesState(model, 'clockmakers');
  const changed = seriesRulesReducer(opened, {type: 'set-keep', value: 3});
  const closed = seriesRulesReducer(changed, {type: 'close'});
  assert.equal(closed.selectedRuleId, undefined);
  assert.equal(closed.rules.find(rule => rule.id === 'clockmakers')?.keep, 'all');
});

test('Record narrows Coming up and the match count as the draft changes', () => {
  const opened = initialSeriesRulesState(model, 'clockmakers');
  const all = opened.panel!;
  assert.equal(visibleAirings(all).length, 3);
  assert.equal(t('web.dvr.matches', {count: 3}), '3 upcoming airings match');
  const onlyNew = seriesRulesReducer(opened, {type: 'set-record', value: 'new'});
  assert.deepEqual(visibleAirings(onlyNew.panel!).map(value => value.id), ['three']);
  assert.equal(t('web.dvr.matches', {count: visibleAirings(onlyNew.panel!).length}), '1 upcoming airing matches');
  assert.equal(visibleAirings(seriesRulesReducer(onlyNew, {type: 'set-record', value: 'all'}).panel!).length, 3);
});

test('Channel narrows Coming up to the rule channel', () => {
  const opened = initialSeriesRulesState(model, 'clockmakers');
  const oneChannel = seriesRulesReducer(opened, {type: 'set-channel', value: 'this'}).panel!;
  assert.ok(visibleAirings(oneChannel).every(value => value.channel === 'Atlas Documentaries'));
  const other = {...opened, panel: {...opened.panel!, airings: [
    {id: 'other', day: 'today' as const, time: at.early, title: 'Other', channel: 'Northwind News'},
    ...clockmakers.airings,
  ]}};
  assert.equal(visibleAirings(seriesRulesReducer(other, {type: 'set-channel', value: 'this'}).panel!).length, 3);
  assert.equal(visibleAirings(seriesRulesReducer(other, {type: 'set-channel', value: 'any'}).panel!).length, 4);
});

test('Keep, Start early and End late land in the draft and save', () => {
  let state = initialSeriesRulesState(model, 'clockmakers');
  state = seriesRulesReducer(state, {type: 'set-keep', value: 3});
  state = seriesRulesReducer(state, {type: 'set-start-early', value: 5});
  state = seriesRulesReducer(state, {type: 'set-end-late', value: 10});
  assert.deepEqual([state.panel!.draft.keep, state.panel!.draft.startEarlyMinutes, state.panel!.draft.endLateMinutes], [3, 5, 10]);
  const saved = seriesRulesReducer(state, {type: 'save'}).rules.find(rule => rule.id === 'clockmakers')!;
  assert.deepEqual([saved.keep, saved.startEarlyMinutes, saved.endLateMinutes], [3, 5, 10]);
});

test('Delete removes the rule, closes the editor and keeps the rest', () => {
  const opened = initialSeriesRulesState(model, 'clockmakers');
  const deleted = seriesRulesReducer(opened, {type: 'delete'});
  assert.equal(deleted.selectedRuleId, undefined);
  assert.deepEqual(deleted.rules.map(rule => rule.id), ['world-tonight', 'summit-football', 'wild-plains']);
});

test('card options and next lines say exactly what the frame says', () => {
  assert.deepEqual(ruleOptionLabels(clockmakers, t), ['All episodes', 'Any channel', 'Keep every episode']);
  assert.deepEqual(ruleOptionLabels(worldTonight, t), ['All episodes', 'Northwind News 24', 'Keep the latest episode']);
  assert.deepEqual(ruleOptionLabels(summitFootball, t), ['All episodes', 'Any channel', 'Keep every episode']);
  assert.deepEqual(ruleOptionLabels(wildPlains, t), ['New episodes only', 'Atlas Documentaries', 'Keep the latest 3']);
  assert.equal(ruleNextLabel(clockmakers, t), 'Next: today, 4:15 PM on Atlas Documentaries');
  assert.equal(ruleNextLabel(worldTonight, t), 'Next: today, 4:30 PM');
  assert.equal(ruleNextLabel(summitFootball, t), 'Next: today, 8:00 PM, tuner conflict');
  assert.equal(ruleNextLabel(wildPlains, t), 'Paused. 2 new airings tonight would match');
  assert.equal(ruleStatusLabel(clockmakers, t), 'On');
  assert.equal(ruleStatusLabel(wildPlains, t), 'Paused');
  assert.equal(t('next.live.rules.upcoming', {count: 3}), '3 upcoming');
  assert.equal(t('dvr.state.scheduled'), 'Scheduled');
});

test('the storage bar keeps the drawn one-decimal share', () => {
  assert.equal(storageFraction(64 * 1024 ** 3, 412 * 1024 ** 3), 13.4);
  assert.equal(storageFraction(0, 0), 0);
  assert.equal(storageFraction(1, 3), 25);
});
