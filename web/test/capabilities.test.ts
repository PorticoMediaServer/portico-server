import test from 'node:test';
import assert from 'node:assert/strict';
import {CAPABILITY_BEHIND_PRESENCE, CAPABILITY_ROWS, capabilityReasonId, parseCapabilities} from '../src/admin/capabilities.ts';

test('capabilities parse defensively and keep code and detail', () => {
  const items = parseCapabilities({items: [
    {capability: 'live_tv', available: false, code: 'decoder_confinement_unavailable', detail: 'bwrap is not on the server’s PATH', checkedAt: '2026-09-23T05:00:00Z'},
    {capability: 'recording', available: true, checkedAt: '2026-09-23T05:00:00Z'},
    {capability: 7, available: true, checkedAt: 'x'},
    null,
  ]});
  assert.equal(items.length, 2);
  assert.deepEqual(items[0], {capability: 'live_tv', available: false, checkedAt: '2026-09-23T05:00:00Z', code: 'decoder_confinement_unavailable', detail: 'bwrap is not on the server’s PATH'});
  assert.equal(items[1]!.code, undefined);
  assert.throws(() => parseCapabilities({}));
});

test('every reason code has plain words; unknown codes fall back, never to the code', () => {
  assert.equal(capabilityReasonId('decoder_confinement_unavailable'), 'web.capabilities.reason.platform');
  assert.equal(capabilityReasonId('ffmpeg_not_configured'), 'web.capabilities.reason.tools');
  assert.equal(capabilityReasonId('decoder_dependencies_unavailable'), 'web.capabilities.reason.dependencies');
  assert.equal(capabilityReasonId('hardware_unavailable'), 'web.capabilities.reason.hardware');
  assert.equal(capabilityReasonId('decoder_sandbox_unavailable'), 'web.capabilities.reason.decoderSandboxUnavailable');
  assert.equal(capabilityReasonId('decoder_sandbox_off'), 'web.capabilities.reason.decoderSandboxOff');
  assert.equal(capabilityReasonId('dolby_vision_approximate'), 'web.capabilities.reason.dolbyVisionApproximate');
  assert.equal(capabilityReasonId('something_new'), 'web.capabilities.reason.other');
});

test('new diagnostics rows exist and stay behind presence until sent', () => {
  assert.ok((CAPABILITY_ROWS as readonly string[]).includes('decoder_sandbox'));
  assert.ok((CAPABILITY_ROWS as readonly string[]).includes('dolby_vision_conversion'));
  assert.ok((CAPABILITY_BEHIND_PRESENCE as readonly string[]).includes('decoder_sandbox'));
  assert.ok((CAPABILITY_BEHIND_PRESENCE as readonly string[]).includes('dolby_vision_conversion'));
});

test('detail keeps up to 1000 characters', () => {
  const long = 'x'.repeat(1200);
  const items = parseCapabilities({items: [{capability: 'live_tv', available: false, code: 'decoder_sandbox_unavailable', detail: long, checkedAt: '2026-09-23T05:00:00Z'}]});
  assert.equal(items[0]!.detail?.length, 1000);
});
