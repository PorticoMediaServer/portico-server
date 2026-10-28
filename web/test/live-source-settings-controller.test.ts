import test from 'node:test';
import assert from 'node:assert/strict';
import {SourceSettingsController} from '../src/screens/settings/server-livetv/source-settings-controller.ts';
const id = 'a'.repeat(48);
const filter = {mode: 'include', values: []};
const settings = {enabledGroups: [], enabledChannels: [], kind: 'playlist', streamBufferMs: null, retryWindowMs: null, userAgent: '', guideSourceId: '', importLimits: null,
  filters: {categories: filter, countries: filter, keywords: filter}, logoImport: {enabled: true, overwriteExisting: false}, channelNumbering: {mode: 'source', startAt: 1, step: 1}};
const doc = {sourceId: id, sourceName: 'Stored source', revision: 7, digest: 'digest', tunerCount: 2, effectiveLogoImport: {enabled: true, overwriteExisting: false}, availableKinds: ['playlist', 'xmltv-guide'], settings,
  effective: {importLimits: {bytes: 268435456, channels: 10000, programmes: 1000000}, streamBufferMs: 8000, retryWindowMs: 60000, userAgent: '', guideDays: 7, logoImport: true, discoveryEnabled: false}};
const response = (result = doc) => ({protocolVersion: '1.0', serverId: 'server', result});
test('source configuration uses checked revision, stable retry id and receipt readback', async () => {
  const bodies: any[] = [];let attempt = 0;
  const controller = new SourceSettingsController({async request<T>(_path: string, method?: string, body?: any) {
    if (method === 'PUT') {bodies.push(body);if (++attempt === 1) throw {status: 503};return response({...doc, revision: 8, settings: body.settings}) as T;}return response() as T;
  }}, 'server', id, () => true, () => {}, () => 'b'.repeat(48));
  await controller.load();controller.edit({userAgent: 'Stored player'});
  assert.equal(await controller.save(), false);assert.equal(await controller.save(), true);
  assert.deepEqual(bodies[0], bodies[1]);assert.equal(bodies[0].expectedRevision, 7);assert.equal(controller.getSnapshot().document?.revision, 8);controller.dispose();
});
test('retiring during load/save publication prevents API invocation', async () => {
  let calls = 0;const api = {async request<T>() {calls++;return response() as T;}};
  const loading = new SourceSettingsController(api, 'server', id, () => true, () => {});
  loading.subscribe(() => {if (loading.getSnapshot().loading) loading.dispose();});await loading.load();assert.equal(calls, 0);
  const saving = new SourceSettingsController(api, 'server', id, () => true, () => {});await saving.load();saving.edit({userAgent: 'Changed'});
  saving.subscribe(() => {if (saving.getSnapshot().saving) saving.dispose();});assert.equal(await saving.save(), false);assert.equal(calls, 1);assert.equal(saving.getSnapshot().document, undefined);
});
test('a configuration for another source never populates this source', async () => {
  const controller = new SourceSettingsController({async request<T>() {return response({...doc, sourceId: 'c'.repeat(48)}) as T;}}, 'server', id, () => true, () => {});
  await controller.load();assert.equal(controller.getSnapshot().document, undefined);assert.ok(controller.getSnapshot().error);controller.dispose();
});
