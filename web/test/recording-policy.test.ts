import test from 'node:test';
import assert from 'node:assert/strict';
import {recordingAllowed, recordingGuideRoute, recordingRulePayload, recordingSeries, parseRecordingGrants, parseRecordingOwners} from '../src/admin/recording-policy.ts';
import {setLibraryAnalysis} from '../src/admin/library-policy.ts';
const owner = {authority: 'local' as const, accountId: 'account', profileId: 'primary'};
const child = {...owner, profileId: 'child'};
const grant = {owner, enabled: true, inheritProfiles: false, revision: 1};

test('recording selection requires an explicit grant and honors child denial before inheritance', () => {
  assert.equal(recordingAllowed(owner, []), false);
  assert.equal(recordingAllowed(child, [grant]), false);
  assert.equal(recordingAllowed(child, [{...grant, inheritProfiles: true}]), true);
  assert.equal(recordingAllowed(child, [{...grant, inheritProfiles: true}, {...grant, owner: child, enabled: false}]), false);
  assert.equal(recordingAllowed({...child, authority: 'hosted'}, [{...grant, inheritProfiles: true}]), false);
});
test('new rule mutation includes actual owner and guide anchor, never a typed title as series identity', () => {
  const anchor = {sourceId: 'source', channelId: 'channel', generation: 'generation', programmeId: 'programme'};
  const draft = {owner, anchor, kind: 'series', name: 'My rule', match: 'provider-series-id', sourceId: 'source', enabled: true, options: {}};
  const payload = recordingRulePayload(draft, [grant], 'operation');
  assert.deepEqual(payload.owner, owner);
  assert.deepEqual(payload.anchor, anchor);
  assert.equal(payload.match, 'provider-series-id');
  assert.equal(payload.expectedRevision, 0);
  assert.throws(() => recordingRulePayload({...draft, anchor: undefined}, [grant], 'operation'), /program/);
  assert.throws(() => recordingRulePayload({...draft, kind: 'keyword'}, [grant], 'operation'), /series/);
  assert.throws(() => recordingRulePayload(draft, [], 'operation'), /Allow/);
  assert.throws(() => recordingRulePayload({...draft, sourceId: 'other'}, [grant], 'operation'), /program/);
});
test('guide choices preserve generation and cannot manufacture a series for titles without IDs', () => {
  const choices = recordingSeries({channels: [{id: 'channel', sourceId: 'source', generation: 'gen', provenance: 'live-source', name: 'Channel', recordAvailable: true, programmes: [{id: 'p1', seriesId: 'stable', title: 'Series', start: '2026-09-22T10:00:00Z', newEvidence: 'new'}, {id: 'p2', seriesId: '', title: 'Unidentified show'}]}]} as any);
  assert.equal(choices.length, 1);
  assert.deepEqual(choices[0]!.anchor, {sourceId: 'source', channelId: 'channel', generation: 'gen', programmeId: 'p1'});
});
test('guide dates reject empty and invalid calendar dates and match whole-second response timestamps', () => {
  for (const day of ['', 'wrong', '2026-02-30']) assert.equal(recordingGuideRoute(day, ''), null);
  const route = recordingGuideRoute('2026-09-22', '')!;
  assert.equal(route.start, '2026-09-22T00:00:00Z');
  assert.equal(route.end, '2026-09-23T00:00:00Z');
});
test('recording directory preserves real profile identities and pagination; malformed grants fail closed', () => {
  assert.deepEqual(parseRecordingOwners({items: [{owner, accountName: 'Account', profileName: 'Primary'}], nextCursor: 'page-two'}).nextCursor, 'page-two');
  assert.throws(() => parseRecordingOwners({items: [{owner: {...owner, profileId: ''}, accountName: 'Account', profileName: 'Missing'}]}));
  assert.throws(() => parseRecordingGrants([{...grant, enabled: 'true'}]));
  assert.deepEqual(parseRecordingGrants([grant]), [grant]);
});
test('library operation enable keeps requested operations, closes dependencies and synchronizes chapters', () => {
  const matrix = {operations: [{id: 'probe', requires: []}, {id: 'local_metadata', requires: ['probe']}, {id: 'chapter_images', requires: ['local_metadata']}]} as any;
  const settings = {analysis: ['probe'], navigation: {chapterThumbnailMode: 'embedded'}} as any;
  const enabled = setLibraryAnalysis(settings, matrix, 'chapter_images', true);
  assert.deepEqual(new Set(enabled.analysis), new Set(['probe', 'local_metadata', 'chapter_images']));
  assert.equal(enabled.navigation.chapterThumbnailMode, 'generated');
  const disabled = setLibraryAnalysis(enabled, matrix, 'probe', false);
  assert.deepEqual(disabled.analysis, []);
  assert.equal(disabled.navigation.chapterThumbnailMode, 'embedded');
});
