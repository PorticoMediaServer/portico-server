/**
 * A library's metadata source: reading the server's choices, the change it
 * sends, and what a conflict or a bad choice looks like.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
const load = () => import('../src/admin/metadata-agent.ts');

const answer = (over: Record<string, unknown> = {}) => ({
  libraryId: 'lib1', libraryKind: 'movie', revision: 3, agent: 'online',
  agents: [
    {id: 'online', name: 'Portico online metadata', description: 'Matches films with TMDB for titles, artwork, cast and ratings. Your own files and NFO details are used first.', providers: ['tmdb']},
    {id: 'local', name: 'Local metadata only', description: 'Uses only file and folder names, embedded tags, NFO and other sidecar files, and artwork stored next to your media. Portico never looks anything up online for this library.', providers: []},
  ],
  ...over,
});

test('reads the server\'s choices in its order, with its names and descriptions', async () => {
  const {parseMetadataAgent} = await load();
  const a = parseMetadataAgent(answer());
  assert.equal(a.agent, 'online');
  assert.deepEqual(a.agents.map(x => [x.id, x.name]), [['online', 'Portico online metadata'], ['local', 'Local metadata only']]);
  assert.deepEqual(a.agents[0]!.providers, ['tmdb']);
});

test('a reply without a revision, or whose choice isn\'t offered, is refused', async () => {
  const {parseMetadataAgent} = await load();
  assert.throws(() => parseMetadataAgent(answer({revision: 0})));
  assert.throws(() => parseMetadataAgent(answer({agent: 'elsewhere'})));
  assert.throws(() => parseMetadataAgent(null));
});

test('a change sends the revision it was read at, and resolves to the saved state', async () => {
  const {parseMetadataAgent, saveMetadataAgent} = await load();
  const current = parseMetadataAgent(answer());
  const calls: unknown[] = [];
  const saved = await saveMetadataAgent(async (path, method, body) => { calls.push([path, method, body]); return answer({agent: 'local', revision: 4}) as never; }, current, 'lib 1', 'local');
  assert.deepEqual(calls, [['/v1/libraries/lib%201/metadata/agent', 'PUT', {expectedRevision: 3, agent: 'local'}]]);
  assert.equal(saved.agent, 'local');
  assert.equal(saved.revision, 4);
});

test('metadata_conflict means reload; a bad choice is an ordinary error', async () => {
  const {isMetadataConflict} = await load();
  assert.equal(isMetadataConflict({status: 409, code: 'metadata_conflict'}), true);
  assert.equal(isMetadataConflict({status: 400, code: 'invalid_metadata_source'}), false);
  assert.equal(isMetadataConflict({status: 409, code: 'local_metadata_only'}), false);
});

test('listed languages are kept, others filtered, at most 64', async () => {
  const {parseMetadataAgent} = await load();
  const online = (languages: unknown) => parseMetadataAgent(answer({agents: [{id: 'online', name: 'Online', description: 'Online.', providers: ['tmdb'], languages}, {id: 'local', name: 'Local', description: 'Local.', providers: []}]})).agents[0]!;
  assert.deepEqual(online(['en', 'ja']).languages, ['en', 'ja']);
  assert.deepEqual(online(['en', 'EN', 'eng', 'e', '', 'en-US', 3, null, 'ja']).languages, ['en', 'ja']);
  const many = Array.from({length: 70}, (_, i) => String.fromCharCode(97 + (i % 26)) + String.fromCharCode(97 + (Math.floor(i / 26) % 26)));
  assert.equal(online(many).languages.length, 64);
});

test('a missing or malformed languages field reads as empty, never a throw', async () => {
  const {parseMetadataAgent} = await load();
  for (const languages of [undefined, null, 'en', 3, {en: true}]) {
    const agents = parseMetadataAgent(answer({agents: [{id: 'online', name: 'Online', description: 'Online.', providers: [], languages}, {id: 'local', name: 'Local', description: 'Local.', providers: []}]}));
    assert.deepEqual(agents.agents.map(a => a.languages), [[], []]);
  }
  const legacy = parseMetadataAgent(answer());
  assert.deepEqual(legacy.agents.map(a => a.languages), [[], []]);
});

test('defaultLanguage is kept only when it is listed', async () => {
  const {parseMetadataAgent} = await load();
  const option = (languages: unknown, defaultLanguage: unknown) => parseMetadataAgent(answer({agents: [{id: 'online', name: 'Online', description: 'Online.', providers: [], languages, defaultLanguage}, {id: 'local', name: 'Local', description: 'Local.', providers: []}]})).agents[0]!;
  assert.equal(option(['en', 'ja'], 'ja').defaultLanguage, 'ja');
  assert.equal(option(['en', 'ja'], 'en').defaultLanguage, 'en');
  assert.equal(option(['en'], 'ja').defaultLanguage, undefined);
  assert.equal(option([], 'en').defaultLanguage, undefined);
  assert.equal(option(undefined, 'en').defaultLanguage, undefined);
});

test('language passes through as stored; absent stays absent', async () => {
  const {parseMetadataAgent} = await load();
  assert.equal(parseMetadataAgent(answer({language: 'en-US'})).language, 'en-US');
  assert.equal(parseMetadataAgent(answer({language: 'ja'})).language, 'ja');
  assert.ok(!('language' in parseMetadataAgent(answer())));
  assert.ok(!('language' in parseMetadataAgent(answer({language: ''}))));
  assert.ok(!('language' in parseMetadataAgent(answer({language: 'x'.repeat(36)}))));
  assert.ok(!('language' in parseMetadataAgent(answer({language: 3}))));
});

test('parseKindMetadataAgents reads the kind document with the same option parsing', async () => {
  const {parseKindMetadataAgents} = await load();
  const doc = parseKindMetadataAgents({libraryKind: 'movie', agents: [
    {id: 'online', name: 'Portico online metadata', description: 'Online.', providers: ['tmdb'], languages: ['en', 'ja', 'EN'], defaultLanguage: 'en'},
    {id: 'local', name: 'Local metadata only', description: 'Local.', providers: []},
  ]});
  assert.equal(doc.libraryKind, 'movie');
  assert.deepEqual(doc.agents.map(a => a.id), ['online', 'local']);
  assert.deepEqual(doc.agents[0]!.languages, ['en', 'ja']);
  assert.equal(doc.agents[0]!.defaultLanguage, 'en');
  assert.deepEqual(doc.agents[1]!.languages, []);
  assert.equal(doc.agents[1]!.defaultLanguage, undefined);
});

test('parseKindMetadataAgents throws only when agents is missing or empty after parsing', async () => {
  const {parseKindMetadataAgents} = await load();
  assert.throws(() => parseKindMetadataAgents(null));
  assert.throws(() => parseKindMetadataAgents({libraryKind: 'movie'}));
  assert.throws(() => parseKindMetadataAgents({libraryKind: 'movie', agents: 'online'}));
  assert.throws(() => parseKindMetadataAgents({libraryKind: 'movie', agents: []}));
  assert.throws(() => parseKindMetadataAgents({libraryKind: 'movie', agents: [{name: 'No id'}]}));
  const deduped = parseKindMetadataAgents({libraryKind: 'movie', agents: [{id: 'online', name: 'Online', description: 'Online.'}, {id: 'online', name: 'Dupe', description: 'Dupe.'}]});
  assert.deepEqual(deduped.agents.map(a => a.name), ['Online']);
});

test('kindMetadataAgentsPath addresses the kind', async () => {
  const {kindMetadataAgentsPath} = await load();
  assert.equal(kindMetadataAgentsPath('movie'), '/v1/library-kinds/movie/metadata-agents');
  assert.equal(kindMetadataAgentsPath('tv shows'), '/v1/library-kinds/tv%20shows/metadata-agents');
});
