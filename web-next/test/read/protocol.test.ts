import assert from 'node:assert/strict';
import {test} from 'node:test';
import {readServerFeatures, readingSeriesDiscoveryEnabled} from '../../src/screens/read/protocol.ts';

const route = (body: unknown) => async (path: string) => {
  assert.equal(path, '/v1/capabilities');
  return {api: {level: 1, major: 1}, limits: {}, features: body};
};

test('the published descriptor decides the series discovery flag', async () => {
  const features = await readServerFeatures(route({reading_discover_series: 'enabled'}), new AbortController().signal);
  assert.equal(readingSeriesDiscoveryEnabled(features), true);
  const older = await readServerFeatures(route({}), new AbortController().signal);
  assert.equal(readingSeriesDiscoveryEnabled(older), false, 'an older server omits the flag');
});

test('a descriptor value other than the enabled word never turns the feature on', async () => {
  for (const value of ['disabled', 'enabled ', 'ENABLED', '', 'true']) {
    const features = await readServerFeatures(route({reading_discover_series: value}), new AbortController().signal);
    assert.equal(readingSeriesDiscoveryEnabled(features), false);
  }
});

test('unusable feature names and values are dropped, not guessed', async () => {
  const features = await readServerFeatures(route({reading_discover_series: 'enabled', 'bad name': 'enabled', good_flag: 'x'.repeat(65), other: 'value'}), new AbortController().signal);
  assert.deepEqual(Object.keys(features), ['reading_discover_series', 'other']);
});
