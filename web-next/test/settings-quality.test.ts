import test from 'node:test';
import assert from 'node:assert/strict';
import {settingsRowIndex} from '@core/presentation/settings-spec.ts';
import {settingsBinding} from '@core/presentation/settings-bindings.ts';
import {hasWebPreferenceEffect} from '../src/screens/settings/effects.ts';

const row = (id: string) => {
  const location = settingsRowIndex.get(id);
  assert.ok(location, id);
  return location.row;
};

test('the two saved quality defaults are offered because the server applies them', () => {
  for (const id of ['quality.home', 'quality.remote']) {
    assert.equal(hasWebPreferenceEffect(row(id)), true, id);
    const binding = settingsBinding(id);
    assert.equal(binding?.form, 'preferences');
    assert.equal(binding?.key, id);
  }
});

test('quality rows without a producer stay hidden rather than pretending to work', () => {
  // On mobile data has no registry key or server rule yet (the cellular default is hard-coded).
  assert.equal(hasWebPreferenceEffect(row('quality.mobileData')), false, 'quality.mobileData');
  // Download quality has no preference key or web downloads consumer.
  assert.equal(hasWebPreferenceEffect(row('quality.downloadQuality')), false, 'quality.downloadQuality');
  // TV screen/sound facts and the safe-mode device switches are device settings; the web
  // Settings device source is read-only, so the web player cannot write them yet.
  assert.equal(hasWebPreferenceEffect(row('quality.tvSupports')), false, 'quality.tvSupports');
  assert.equal(hasWebPreferenceEffect(row('quality.safeMode')), false, 'quality.safeMode');
});
