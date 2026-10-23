import test from 'node:test';
import assert from 'node:assert/strict';
import {settingsRowIndex} from '../../packages/client-core/src/presentation/settings-spec.ts';
import {exactPresetValue, presetPatch} from '../../packages/client-core/src/presentation/settings-bindings.ts';
import {settingsPresetOptions} from '../src/screens/settings/preset-options.ts';

test('the approved passthrough preset reads and writes only its actual OS variant', () => {
  const original = settingsRowIndex.get('device.passthrough')!.row.control.options!;
  const before = structuredClone(original);
  const windows = settingsPresetOptions(original, {platform: 'desktop', os: 'windows'});
  const mac = settingsPresetOptions(original, {platform: 'desktop', os: 'macos'});
  assert.deepEqual(Object.keys(windows[0].sets!), ['device.passthroughFormats']);
  assert.deepEqual(Object.keys(mac[0].sets!), ['device.passthroughFormatsMac']);
  const selected = windows[0];
  assert.equal(exactPresetValue(windows, id => selected.sets?.[id]), selected.value);
  const patch = presetPatch(mac, mac[0].value);
  assert.equal(Object.hasOwn(patch, 'device.passthroughFormats'), false);
  assert.deepEqual(original, before);
});
test('a platform with no applicable members cannot match an empty set or write a derived choice', () => {
  const original = settingsRowIndex.get('device.passthrough')!.row.control.options!;
  const browser = settingsPresetOptions(original, {platform: 'web', os: 'browser'});
  assert.equal(browser.some(option => option.sets), false);
  assert.equal(exactPresetValue(browser, () => undefined), 'custom');
  assert.throws(() => presetPatch(browser, browser[0].value));
});
