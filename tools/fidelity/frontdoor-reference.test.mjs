import test from 'node:test';
import assert from 'node:assert/strict';
import {frontdoorReferences, frontdoorStateAdapters} from './frontdoor-reference.mjs';

test('frontdoor public adapters map the observed Button label separately from its focus host', () => {
  for (const [platform, id] of [['phone', 'accounts-welcome-portico'], ['androidTV', 'tv-front-door-portico']]) {
    const adapter = frontdoorStateAdapters[platform];
    assert.deepEqual(adapter.anchors, {[id]: 'primary-action', [id + '/label']: 'primary-action-label'});
    assert.deepEqual(adapter.comparison.contentSlots, [{reference: 'primary-action', actual: 'primary-action-label'}]);
    assert.deepEqual(adapter.comparison.textHandledBySlots, ['primary-action']);
    assert.deepEqual(adapter.comparison.actualContentOnly, ['primary-action-label']);
    assert.equal(frontdoorReferences[platform].viewport.deviceScaleFactor, 1);
    assert.ok(!JSON.stringify(adapter).includes('data-f'));
  }
  assert.equal(frontdoorStateAdapters.phone.comparison.focus, undefined);
  assert.deepEqual(frontdoorStateAdapters.androidTV.comparison.focus, {frame: 'primary-action'});
});
