import test from 'node:test';
import assert from 'node:assert/strict';
import {liveBehindLabel, liveNeighbour, liveNextLine, liveRail, liveRecordLabel, liveTimeRange} from '../src/screens/live/live-chrome-model';
import type {LiveChromePort, LiveChromeWindow} from '../src/screens/live/live-chrome-types';

const hour = 3_600_000, minute = 60_000;
const window: LiveChromeWindow = {programmeStartMs: 0, programmeEndMs: 3 * hour, seekStartMs: hour + 35 * minute,
  seekEndMs: 2 * hour + 5 * minute, liveEdgeMs: 2 * hour + 5 * minute, positionMs: 2 * hour + 3 * minute, behindSeconds: 120, atLive: false};
const near = (actual: number, expected: number) => assert.ok(Math.abs(actual - expected) < .01, `${actual} != ${expected}`);

test('the live rail spans the whole programme while the played and buffered bands use the real window', () => {
  const rail = liveRail(window);
  near(rail.gone, 52.777); near(rail.buffer, 16.666); near(rail.played, 15.555); near(rail.future, 30.555); near(rail.edge, 69.444); near(rail.knob, 68.333);
  const empty = liveRail({...window, programmeStartMs: 4, programmeEndMs: 4});
  assert.deepEqual(empty, {gone: 0, buffer: 0, played: 0, future: 100, edge: 100, knob: 100});
});

test('one time range omits the repeated meridiem and keeps the locale order', () => {
  const format = (ms: number) => ms === 0 ? '8:00 PM' : '8:15 PM';
  assert.equal(liveTimeRange(0, 1, format), '8:00 to 8:15 PM');
  const pm = (ms: number) => ms === 0 ? '11:45 PM' : '12:15 AM';
  assert.equal(liveTimeRange(0, 1, pm), '11:45 PM to 12:15 AM');
});

test('behind live appears only beyond the live tolerance', () => {
  assert.equal(liveBehindLabel(1), undefined);
  assert.ok(liveBehindLabel(120));
  assert.equal(liveRecordLabel({...fixture, recording: 'recording'}), 'Recording');
  assert.equal(liveRecordLabel(fixture), 'Record');
});

const fixture = {id: 'p', title: 'Cycling: Mountain Stage', startMs: 0, endMs: 1} as const;
const port = {down: {channel: {id: 'a'}}, up: {channel: {id: 'b'}}, nextProgramme: {title: 'Cycling', startMs: 1, endMs: 2}} as unknown as LiveChromePort;

test('channel surf targets the actual down/up neighbours and the next line names the session fact', () => {
  assert.equal(liveNeighbour(port, -1), port.down);
  assert.equal(liveNeighbour(port, 1), port.up);
  assert.equal(liveNextLine(port), 'Then Cycling');
  assert.equal(liveNextLine({...port, nextProgramme: undefined} as LiveChromePort), undefined);
});
