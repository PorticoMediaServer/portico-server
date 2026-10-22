import test from 'node:test';
import assert from 'node:assert/strict';
import {AutomaticQuality, fixedQualities, fixedQualityOf, heightClass, isQualityChoice, offeredQualities, qualityChoiceOf, qualityChoiceOrder, qualityRequest} from '../src/choices.ts';

test('the menu order is Original, Automatic, then the fixed qualities, best first', () => {
  assert.deepEqual(qualityChoiceOrder, ['original', 'automatic', '2160', '1440', '1080', '720', '480', '360'], 'the server\u2019s ladder, one for one');
  assert.equal(isQualityChoice('automatic'), true);
  assert.equal(isQualityChoice('auto'), false);
});

test('only qualities really below the source are offered', () => {
  // A 1920x800 film is 1080p: nothing at or above it, unless its bit rate is far above the step's.
  assert.deepEqual(offeredQualities({height: 800, bitrateKbps: 9000}).map(q => q.id), ['720', '480', '360']);
  assert.deepEqual(offeredQualities({height: 1080, bitrateKbps: 30_000}).map(q => q.id), ['1080', '720', '480', '360'], 'a disc copy has a meaningful 1080p below it');
  assert.deepEqual(offeredQualities({height: 2160, bitrateKbps: 25_000}).map(q => q.id), ['1440', '1080', '720', '480', '360']);
  assert.deepEqual(offeredQualities({height: 2160, bitrateKbps: 80_000}).map(q => q.id), ['2160', '1440', '1080', '720', '480', '360']);
  assert.deepEqual(offeredQualities({height: 480}).map(q => q.id), ['360']);
  assert.equal(offeredQualities().length, fixedQualities.length, 'with nothing known, all are offered');
  assert.equal(heightClass(1600), 2160);
  assert.equal(heightClass(1440), 1440);
  assert.equal(heightClass(1036), 1080);
});

test('a choice becomes the session quality request in the server\u2019s own values; Automatic starts at Original', () => {
  assert.deepEqual(qualityRequest('original'), {mode: 'original'});
  assert.deepEqual(qualityRequest('automatic'), {mode: 'automatic', id: 'original'}, 'Automatic on no step, said outright so a stepped session comes back');
  assert.deepEqual(qualityRequest('720'), {mode: 'fixed', id: '720p'});
  assert.deepEqual(qualityRequest('2160'), {mode: 'fixed', id: '2160p'});
  assert.deepEqual(qualityRequest('automatic', fixedQualities.find(q => q.id === '1080')), {mode: 'automatic', id: '1080p'});
  assert.equal(fixedQualityOf(qualityRequest('720'))?.id, '720');
  assert.equal(fixedQualityOf({mode: 'original'}), undefined);
  // The quality in force on a session (what the server started it with) reads back the same way.
  assert.equal(qualityChoiceOf({mode: 'fixed', id: '1440p'}), '1440');
  assert.equal(qualityChoiceOf({mode: 'automatic', id: '720p'}), 'automatic');
  assert.equal(fixedQualityOf({mode: 'automatic', id: '720p'})?.id, '720', 'the step Automatic is on');
  assert.equal(qualityChoiceOf({mode: 'original'}), 'original');
  assert.equal(qualityChoiceOf({mode: 'limit', maxHeight: 720}), '720');
  assert.equal(qualityChoiceOf(undefined), undefined);
});

const steps = fixedQualities.filter(q => q.height <= 1080);
function automatic(options: {originalKbps?: number; startAt?: (typeof steps)[number]} = {}) {
  let now = 0;
  const auto = new AutomaticQuality({steps, originalKbps: options.originalKbps ?? 20_000, now: () => now, ...(options.startAt ? {startAt: options.startAt} : {})});
  return {auto, pass: (ms: number) => { now += ms; }, feed: (kbps: number, seconds: number) => { let last: ReturnType<AutomaticQuality['decide']>; for (let i = 0; i < seconds; i++) { now += 1000; auto.sample(kbps); const d = auto.decide(); if (d !== undefined) last = d; } return last; }};
}

test('Automatic stays at Original while the connection carries it', () => {
  const {auto, feed} = automatic();
  assert.equal(feed(40_000, 120), undefined);
  assert.equal(auto.step, undefined);
});

test('a brief dip changes nothing; a sustained shortfall steps down to what the connection carries with room to spare', () => {
  const {auto, feed} = automatic();
  assert.equal(feed(5000, 5), undefined, 'five seconds is not sustained');
  feed(40_000, 30);
  assert.equal(auto.step, undefined);
  const to = feed(6000, 15);
  assert.equal(to?.id, '720', '6 Mbps carries 720p (4 Mbps) with a fifth to spare, not 1080p (8 Mbps)');
  assert.equal(auto.step?.id, '720');
});

test('two stalls close together step down even with no measurement', () => {
  const {auto, pass} = automatic({originalKbps: undefined});
  auto.stalled();
  assert.equal(auto.decide(), undefined, 'one stall is not a pattern');
  pass(10_000);
  auto.stalled();
  assert.equal(auto.decide()?.id, '1080', 'one step, since nothing says how far');
  // …and stalls far apart are not counted together.
  const other = automatic({originalKbps: undefined});
  other.auto.stalled();
  other.pass(60_000);
  other.auto.stalled();
  assert.equal(other.auto.decide(), undefined);
});

test('it steps back up one step at a time, only after sustained headroom', () => {
  const {auto, feed} = automatic();
  feed(2500, 15);
  assert.equal(auto.step?.id, '480');
  // The estimate rises slowly on purpose, and the headroom must then last thirty seconds.
  assert.equal(feed(7000, 25), undefined, 'not yet');
  assert.equal(feed(7000, 20)?.id, '720', '7 Mbps is half as much again above 720p');
  // 7 Mbps is not half as much again above 1080p (8 Mbps): it stays.
  assert.equal(feed(7000, 120), undefined);
  assert.equal(auto.step?.id, '720');
  feed(40_000, 40);
  assert.equal(auto.step?.id, '1080');
  feed(40_000, 40);
  assert.equal(auto.step, undefined, 'and finally back to Original');
});

test('it does not flap: changes are held apart, and a step up that stalls waits longer before the next try', () => {
  const {auto, feed, pass} = automatic();
  feed(2500, 15);
  assert.equal(auto.step?.id, '480');
  feed(7000, 45);
  assert.equal(auto.step?.id, '720', 'up after thirty seconds of headroom');
  // The step up cannot hold: it stalls twice straight away and comes back down.
  pass(2000); auto.stalled();
  pass(2000); auto.stalled();
  assert.equal(auto.decide(), undefined, 'not within the hold after a change');
  pass(15_000);
  auto.stalled(); pass(1000); auto.stalled();
  assert.equal(auto.decide()?.id, '480');
  // The same headroom as before is no longer enough after thirty seconds: the wait has doubled.
  assert.equal(feed(7000, 45), undefined);
  assert.equal(feed(7000, 30)?.id, '720');
});

test('a title that follows one stepped down starts from that step', () => {
  const {auto, feed} = automatic({startAt: steps[1]});
  assert.equal(auto.step?.id, '720');
  feed(40_000, 40);
  assert.equal(auto.step?.id, '1080');
});
test('Automatic uses an explicit monotonic clock and keeps sampled state bounded',()=>{
 let now=100;const steps=fixedQualities.filter(q=>q.height<=1080),a=new AutomaticQuality({steps,now:()=>now});for(let i=0;i<10000;i++)a.stalled();assert.equal(a.decide()?.id,'1080');now=99;assert.throws(()=>a.sample(4000));assert.throws(()=>new AutomaticQuality({steps:Array(7).fill(steps[0]),now:()=>0}));assert.throws(()=>new AutomaticQuality({steps,now:()=>0,holdMs:Infinity}));
});
