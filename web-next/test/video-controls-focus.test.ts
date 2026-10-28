import test from 'node:test';
import assert from 'node:assert/strict';
import {revealVideoControlsOnTab, videoControlsMayHide} from '../src/screens/watch/video-controls-focus.ts';

test('hidden controls recover forward and backward Tab without letting focus leave first', () => {
  for (const shiftKey of [false, true]) {
    let prevented = false;
    const calls: boolean[] = [];
    assert.equal(revealVideoControlsOnTab({key: 'Tab', shiftKey, defaultPrevented: false, preventDefault() {prevented = true;}}, false,
      backwards => {assert.equal(prevented, true);calls.push(backwards);}), true);
    assert.deepEqual(calls, [shiftKey]);
  }
});
test('visible controls keep native Tab and consumed or unrelated keys do not move focus', () => {
  for (const [key, visible, defaultPrevented] of [['Tab', true, false], ['ArrowRight', false, false], ['Tab', false, true]] as const) {
    assert.equal(revealVideoControlsOnTab({key, shiftKey: false, defaultPrevented, preventDefault() {assert.fail('native key was consumed');}}, visible,
      () => assert.fail('unexpected focus request')), false);
  }
});

test('idle expiry and picture taps cannot hide focused controls regardless of the input modality', () => {
  const inside = {} as Node, outside = {} as Node;
  const chrome = {contains: (node: Node | null) => node === inside};
  assert.equal(videoControlsMayHide(chrome, inside), false);
  assert.equal(videoControlsMayHide(chrome, outside), true);
  assert.equal(videoControlsMayHide(chrome, null), true);
});

test('browser Tab shortcuts do not reveal or consume player focus', () => {
  for (const modifier of ['ctrlKey', 'altKey', 'metaKey']) {
    assert.equal(revealVideoControlsOnTab({key: 'Tab', shiftKey: false, defaultPrevented: false, [modifier]: true,
      preventDefault() {assert.fail('browser shortcut consumed');}}, false, () => assert.fail('focus moved')), false);
  }
});
