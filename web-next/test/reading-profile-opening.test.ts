import assert from 'node:assert/strict';
import test from 'node:test';
import {RemoteSourceService, type RemoteSource, type SourceAdvanced, type SourceReadSettings} from '../../packages/client-core/src/remote-sources.ts';
import {sourceChoiceFields, type SourceChoiceDefinition} from '../../packages/client-core/src/remote-source-choice.ts';
import type {LibraryContentApi} from '../../packages/client-core/src/library-content.ts';
import {ReadingProfileOpening} from '../src/screens/settings/remote-sources/reading-profile-opening.ts';

// Authored UNRUN composition vectors: the real SDK owns provenance, receipt
// parsing, mutation exclusion and retained intent. Only HTTP is a held fixture.
const operation = '1788580000000-12345678-1234-1234-1234-123456789012';
const reading = Object.freeze({scanDepth: 'basic', wholeFileReads: false, connections: 4, requestsPerSecond: 4, bytesPerSecond: 0} as const);
const advanced: SourceAdvanced = Object.freeze({scanIntervalSeconds: 21600, watchForChanges: false, scanOnStart: false,
  fullRecheckDays: 0, cacheBytes: 0, cacheFloorBytes: 0, seekPreviews: true, chapterImages: true, waveforms: true,
  loudness: true, checksums: true, introsMode: 'inherit', doubleCheckFolders: false, dirCacheSeconds: 0,
  readBufferBytes: 0, connectTimeoutSeconds: 0, idleTimeoutSeconds: 0, retries: 0, lowLevelRetries: 0});
const definitions: readonly SourceChoiceDefinition[] = Object.freeze((['list', 'once', 'everything', 'ask'] as const)
  .map(name => Object.freeze({name, definitionRevision: 1, reading, advanced})));
const source: RemoteSource = Object.freeze({id: 'remote', kind: 'rclone', name: 'Remote', rootPath: '/remote', generation: 7,
  state: 'ready', rangeSupport: 'conditional', insecureLocal: false, credentialPresent: true});
const wire = {serverId: 'server', viewerFence: 'owner'};
function profile(saved = true): SourceReadSettings {
  return {sourceId: source.id, generation: '7', revision: saved ? 2 : 0, saved, configurationGeneration: '11',
    values: {...reading, wholeFileReads: true, allowAdvancedMediaAnalysis: false}, choiceDefinitions: definitions, choiceApplicableFields: sourceChoiceFields};
}
function parent(): SourceReadSettings {
  return {...profile(), choiceBaseline: {sourceId: source.id, generation: '7', configurationGeneration: '11', name: 'once',
    definitionRevision: 1, readRevision: 2, advancedRevision: 6, reading, advanced, currentAdvanced: advanced,
    applicableFields: sourceChoiceFields, changedFields: []}};
}
function receipt() {return {scope: wire, receipt: {operationId: operation, accepted: true, removed: false, source, readSettings: parent()}};}
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {resolve = yes;reject = no;});
  return {promise, resolve, reject};
}
type Call = {path: string; method: string; body: unknown; signal?: AbortSignal};
function fixture(options: {put?: (call: Call) => Promise<unknown>; recover?: () => Promise<unknown>;
  fresh?: () => Promise<unknown>; unsaved?: boolean; advanced?: () => Promise<unknown>} = {}) {
  const calls: Call[] = [];let submitted = false, live = true;
  const api: LibraryContentApi = {request: async<T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> => {
    const call = {path, method, body, signal};calls.push(call);
    let value: unknown;
    if (method === 'PUT') {submitted = true;value = options.put ? await options.put(call) : receipt();}
    else if (path.includes('/source-operations/')) value = options.recover ? await options.recover() : receipt();
    else if (path.endsWith('/settings/advanced')) value = options.advanced ? await options.advanced() : {scope: wire,
      settings: {sourceId: source.id, generation: '7', revision: options.unsaved ? 0 : 5, saved: !options.unsaved,
        cacheSupported: true, cacheApplied: true, cacheUsedBytes: 0, scanIntervalObserved: 0, values: advanced}};
    else if (path.endsWith('/settings/read-profile')) value = submitted ? (options.fresh ? await options.fresh() : {scope: wire, settings: parent()})
      : {scope: wire, settings: profile(!options.unsaved)};
    else throw new Error('Unexpected fixture request: ' + method + ' ' + path);
    return value as T;
  }};
  const service = new RemoteSourceService(api, {serverId: 'server', viewerId: 'owner'}, async () => operation);
  const opening = new ReadingProfileOpening(service, source, () => live, () => !service.getSnapshot().busy && !service.getSnapshot().ambiguous);
  const draft = () => {const state = opening.getSnapshot();assert.equal(state.kind, 'ready');if (state.kind !== 'ready') throw new Error('No draft');return state.draft;};
  return {opening, service, calls, draft, retireAuthority: () => {live = false;}};
}

test('original service-issued pair and exact offered object reach one atomic PUT, including first apply without baseline', async () => {
  const held = deferred<unknown>(), f = fixture({unsaved: true, put: () => held.promise});await f.opening.load();
  const original = f.draft(), choice = original.profile.choiceDefinitions![1]!;
  f.opening.select(original, {...choice});assert.equal(f.draft(), original, 'a cloned offer is refused');
  f.opening.select(original, choice);const selected = f.draft();
  const save = f.opening.save(selected);void f.opening.save(selected);
  await Promise.resolve();await Promise.resolve();
  assert.equal(f.calls.filter(call => call.method === 'PUT').length, 1);
  const body = f.calls.find(call => call.method === 'PUT')!.body as {values: Record<string, unknown>; startChoice: Record<string, unknown>};
  assert.equal(body.values.allowAdvancedMediaAnalysis, undefined);assert.equal(body.values.wholeFileReads, true);
  assert.equal(body.startChoice.expectedAdvancedRevision, 0);
  held.resolve(receipt());await save;assert.equal(f.opening.getSnapshot().kind, 'saved');
  assert.equal(f.calls.filter(call => call.method === 'PUT').length, 1);
});

test('retained input from draft A cannot edit or submit B; dirty consent is an independent ordinary save', async () => {
  const f = fixture();await f.opening.load();const a = f.draft();
  f.opening.changeConsent(a, true);const b = f.draft();
  f.opening.edit(a, {connections: 9});f.opening.select(b, b.profile.choiceDefinitions![1]!);
  assert.equal(f.draft(), b);assert.equal(b.choice, undefined);
  await f.opening.save(a);assert.equal(f.calls.filter(call => call.method === 'PUT').length, 0);
  await f.opening.save(b);
  const body = f.calls.find(call => call.method === 'PUT')!.body as {values: Record<string, unknown>; startChoice?: unknown};
  assert.equal(body.startChoice, undefined);assert.equal(body.values.allowAdvancedMediaAnalysis, true);
});

test('an unedited consent fact is omitted from an ordinary save', async () => {
  const f = fixture();await f.opening.load();await f.opening.save(f.draft());
  const body = f.calls.find(call => call.method === 'PUT')!.body as {values: Record<string, unknown>};
  assert.equal(Object.hasOwn(body.values, 'allowAdvancedMediaAnalysis'), false);
});

test('unknown retains the original SDK opening after the dispatch flight ends; repeated recovery is GET only', async () => {
  let reads = 0;
  const f = fixture({put: async () => {throw new TypeError('connection lost');}, recover: async () => {
    if (++reads === 1) throw Object.assign(new Error('not found yet'), {code: 'not_found'});return receipt();
  }});
  await f.opening.load();const a = f.draft();f.opening.select(a, a.profile.choiceDefinitions![1]!);await f.opening.save(f.draft());
  const unknown = f.opening.getSnapshot();assert.equal(unknown.kind, 'unknown');if (unknown.kind !== 'unknown') return;
  f.opening.changeConsent(unknown.draft, true);await f.opening.save(unknown.draft);
  await f.opening.recover(unknown);const next = f.opening.getSnapshot();assert.equal(next.kind, 'unknown');
  if (next.kind !== 'unknown') return;assert.equal(next.result.intent, unknown.result.intent);
  await f.opening.recover(unknown);assert.equal(reads, 1, 'an old Check result cannot borrow the newer outcome');
  await f.opening.recover(next);assert.equal(f.opening.getSnapshot().kind, 'saved');
  assert.equal(f.calls.filter(call => call.method === 'PUT').length, 1);assert.equal(reads, 2);
});

test('confirmed acceptance plus failed refresh is terminal accepted-unavailable, never another PUT or recovery', async () => {
  const f = fixture({fresh: async () => {throw new TypeError('refresh offline');}});await f.opening.load();
  const a = f.draft();f.opening.select(a, a.profile.choiceDefinitions![1]!);const selected = f.draft();
  await f.opening.save(selected);const accepted = f.opening.getSnapshot();assert.equal(accepted.kind, 'accepted-unavailable');
  await f.opening.save(selected);assert.equal(f.calls.filter(call => call.method === 'PUT').length, 1);
  if (accepted.kind === 'accepted-unavailable') assert.equal(accepted.result.settings, accepted.result.accepted.settings);
});

test('route/layout retirement aborts only this opening, suppresses late settlement, and never revives its commands', async () => {
  const held = deferred<unknown>(), f = fixture({put: () => held.promise});await f.opening.load();
  const a = f.draft();f.opening.select(a, a.profile.choiceDefinitions![1]!);const selected = f.draft();
  const save = f.opening.save(selected);await Promise.resolve();await Promise.resolve();
  const signal = f.calls.find(call => call.method === 'PUT')!.signal!;
  f.opening.retire();assert.equal(signal.aborted, true);held.resolve(receipt());await save;
  assert.equal(f.opening.getSnapshot().kind, 'retired');await f.opening.save(selected);
  assert.equal(f.calls.filter(call => call.method === 'PUT').length, 1);
});

test('original viewer/source guard retirement before Save makes zero write I/O', async () => {
  const f = fixture();await f.opening.load();const a = f.draft();f.opening.select(a, a.profile.choiceDefinitions![1]!);
  const selected = f.draft();f.retireAuthority();await f.opening.save(selected);
  assert.equal(f.calls.filter(call => call.method === 'PUT').length, 0);
});

test('an unavailable Advanced read supplies no invented pair and cannot offer an atomic choice', async () => {
  const f = fixture({advanced: async () => {throw new Error('unavailable');}});await f.opening.load();
  const draft = f.draft();assert.equal(draft.advanced, undefined);
  f.opening.select(draft, draft.profile.choiceDefinitions![1]!);assert.equal(f.draft(), draft);
});

test('reentrant publication and throwing view listeners cannot duplicate a command or erase its known success', async () => {
  const f = fixture();await f.opening.load();const a = f.draft();f.opening.select(a, a.profile.choiceDefinitions![1]!);
  const selected = f.draft();
  f.opening.subscribe(() => {void f.opening.save(selected);throw new Error('view listener failed');});
  await f.opening.save(selected);
  assert.equal(f.opening.getSnapshot().kind, 'saved');assert.equal(f.calls.filter(call => call.method === 'PUT').length, 1);
});

test('ordinary known save followed by failed refresh also cannot offer a second write', async () => {
  const f = fixture({put: async () => ({scope: wire, receipt: {operationId: operation, accepted: true, removed: false, source}}),
    fresh: async () => {throw new TypeError('refresh offline');}});
  await f.opening.load();const draft = f.draft();await f.opening.save(draft);
  assert.equal(f.opening.getSnapshot().kind, 'saved-unavailable');await f.opening.save(draft);
  assert.equal(f.calls.filter(call => call.method === 'PUT').length, 1);
});
