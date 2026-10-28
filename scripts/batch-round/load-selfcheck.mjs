/** Offline two-user/four-second request rehearsal. No socket or full-app acceptance. */
import assert from 'node:assert/strict';
import {setTimeout as delay} from 'node:timers/promises';
import {mkdtemp, rm} from 'node:fs/promises';
import {join} from 'node:path';
import {MemoryMutationStore, FileMutationStore, lockMutationDirectory, mutationClient, savedBatch, finalStop} from './load-adapters.ts';
import {SessionsClient} from '../../packages/client-core/src/playback-v1/sessions.ts';
import {ObservationReporter} from '../../packages/client-core/src/playback-v1/observation.ts';
import {personalBatchBody} from '../../packages/client-core/src/personal-saved.ts';
import {emptyUserMutationState, USER_MUTATION_MAX_ENTRIES} from '../../packages/client-core/src/user-mutations.ts';
import {readBrowsePage, readReadingFacet, releaseBrowseSelection} from '../../packages/client-core/src/browse-preparation.ts';
import {options} from './load.ts';

export async function selfCheck(o) {
  assert.equal(o.execute, false);
  const sent = [], wire = [], clients = [], reporters = [];
  const directory = await mkdtemp(join(process.env.TMPDIR ?? '.', 'load-selfcheck-'));
  let unlock;
  try {
    unlock = await lockMutationDirectory(directory);
    await assert.rejects(lockMutationDirectory(directory), {code: 'EEXIST'});
    const disk = new FileMutationStore(join(directory, 'cas.json'));
    const initial = emptyUserMutationState();
    assert.deepEqual(await disk.read(), initial);
    assert.deepEqual(await Promise.all([disk.compareAndSwap(0, {...initial, revision: 1}), disk.compareAndSwap(0, {...initial, revision: 1})]), [true, false]);
    assert.equal((await new FileMutationStore(join(directory, 'cas.json')).read()).revision, 1);
    const memory = new MemoryMutationStore();
    await assert.rejects(memory.compareAndSwap(0, {...initial, revision: 1, entries: Array(USER_MUTATION_MAX_ENTRIES + 1).fill({})}));
    for (let i = 0; i < 2; i++) {
      const store = i === 0 ? new FileMutationStore(join(directory, 'user0.json')) : new MemoryMutationStore(), receipts = new Map();
      const session = {accessToken: 'offline-token-' + i, expiresAt: '2099-01-01T00:00:00Z', authorizationHorizon: '2099-01-02T00:00:00Z',
        tokenGeneration: '1', sessionFamilyId: 'family-' + i, deviceId: 'device-' + i,
        viewer: {serverId: 'offline-server', authority: 'local', accountId: 'account-' + i, profileId: 'profile-' + i, role: 'member'}};
      const transport = async (input, init = {}) => {
        const url = new URL(String(input)), path = url.pathname, method = init.method ?? 'GET', headers = new Headers(init.headers);
        assert.equal(url.origin, 'http://offline.invalid');
        assert.equal(headers.get('Authorization'), 'Bearer ' + session.accessToken);
        const body = init.body ? JSON.parse(init.body) : undefined;
        wire.push({i, path, method, body, key: headers.get('Idempotency-Key')});
        if (path === '/v1/user-mutations/policy') return Response.json({serverTimeMs: Date.now(), retiredBeforeMs: 0});
        if (path.startsWith('/v1/user-mutations/')) {
          const receipt = receipts.get(path.split('/').at(-1));
          return receipt ? Response.json(receipt) : Response.json({error: {code: 'receipt_unknown'}}, {status: 404});
        }
        if (path.endsWith('/observer')) return Response.json({epoch: 'abcdefghijklmnopqrstuv', everyMs: 1000});
        if (path.endsWith('/observation')) return Response.json({acceptedSeq: body.seq});
        if (method === 'DELETE' && path.startsWith('/v1/playback/sessions/')) {
          const entry = (await store.read()).entries.find(entry => entry.key === headers.get('Idempotency-Key'));
          assert.ok(entry, 'custody precedes Stop'); assert.equal(entry.scope.familyId, session.sessionFamilyId);
          assert.deepEqual(body, {generation: 7, positionMs: 3000});
          receipts.set(entry.key, {key: entry.key, operation: entry.operation, status: 'applied', generation: 7, positionMs: 3000,
            positionStatus: 'applied', committedMs: Date.now(), expiresMs: Date.now() + 86400000});
          return new Response(null, {status: 204});
        }
        if (path === '/v1/items/personal-state:batch') {
          const entry = (await store.read()).entries.find(entry => entry.key === headers.get('Idempotency-Key'));
          assert.ok(entry, 'custody precedes Saved'); assert.equal(body.operationId, entry.key);
          assert.equal(entry.scope.profileId, session.viewer.profileId);
          receipts.set(entry.key, {key: entry.key, operation: entry.operation, status: 'applied', committedMs: Date.now(), expiresMs: Date.now() + 86400000});
          // User 1 loses its answer after commit: real coordinator must look up the same key.
          if (i === 1) throw new Error('offline_lost_answer');
          return Response.json({serverId: 'offline-server', viewerFence: 'offline-fence', operationId: body.operationId,
            updated: 1, failed: 0, results: [{itemId: 'known-item', ok: true}]});
        }
        const pending = domain => Response.json({kind: domain + '-preparing', state: 'preparing', queryId: domain + '-' + i, retryAfterMs: 0}, {status: 202});
        if (path.endsWith('/browse') && method === 'POST') return pending('reading-browse');
        if (path.endsWith('/facets')) return pending('reading-facet');
        if (method === 'DELETE') return new Response(null, {status: 204});
        if (path.includes('/reading-browse-queries/')) return Response.json({pivot: 'books', applied: {query: null, sort: [], seek: null}, entries: [],
          pageInfo: {start: 0, total: 0, revision: 'r1', hasMore: false, nextCursor: ''},
          wholeSelectionPreparing: {kind: 'browse-selection-preparing', state: 'preparing', queryId: 'selection-' + i, retryAfterMs: 0}});
        if (path.includes('/reading-facet-queries/')) return Response.json({field: 'author', total: 1, values: [{value: 'Author', label: 'Author', count: 1}]});
        throw new Error('unexpected_offline_request');
      };
      const c = await mutationClient('http://offline.invalid', session, store, transport);
      clients.push({...c, store});
      const sessions = new SessionsClient({...c.http, send: request => {sent.push({i, ...request}); return c.http.send(request);}});
      const reporter = new ObservationReporter({everyMs: 1000,
        claim: body => sessions.claimObserver('session' + i, body), send: body => sessions.observe('session' + i, body)});
      reporter.setGeneration(7); reporter.observe({generation: 7, state: 'playing', positionMs: 0, rate: 1}); reporters.push(reporter);
      c.stop = finalStop(sessions, 'session' + i, 7);
      c.saved = savedBatch(c.api, 'offline-server', 'offline-selfcheck-' + i, [{itemId: 'known-item', watchlisted: true}], AbortSignal.timeout(3000));
      // Handle rejection immediately; joined below after the timed observer window.
      void c.saved.catch(() => {});
      const port = {request: c.api.request.bind(c.api), requestRaw: c.api.requestRaw.bind(c.api)};
      const lifetime = {isCurrent: () => true, signal: AbortSignal.timeout(3000)};
      c.read = (async () => {
        const result = await readBrowsePage(port, 'reading-library', {pivot: 'books', limit: 20}, lifetime);
        assert.equal(result.page.pivot, 'books'); releaseBrowseSelection(port, 'reading-library', result.response);
        assert.equal((await readReadingFacet(port, 'reading-library', '/v1/libraries/reading-library/facets?field=author', 'author', lifetime)).total, 1);
      })();
      void c.read.catch(() => {});
      clients.at(-1).work = c;
    }
    await delay(3200);
    reporters.forEach(reporter => reporter.stop());
    const saved = await Promise.all(clients.map(c => c.work.saved));
    assert.deepEqual(saved.map(result => result.recovered), [false, true]);
    assert.equal(saved[0].updated, 1); assert.equal(saved[1].updated, undefined);
    await Promise.all(clients.map(c => c.work.read));
    await Promise.all(clients.map(c => c.work.stop(3000, AbortSignal.timeout(2000))));
    // A second caller cannot change the first final position or issue a new command.
    await Promise.all(clients.map(c => c.work.stop(9000, AbortSignal.timeout(2000))));
    // The first completion was consumed by SessionsClient; the helper must memoize success.
    const observations = sent.filter(request => request.method === 'PUT');
    assert.equal(sent.filter(request => request.method === 'POST').length, 2);
    assert.equal(wire.filter(request => request.method === 'DELETE' && request.path.startsWith('/v1/playback/sessions/')).length, 2);
    assert.ok(observations.length >= 6);
    for (const request of observations) {
      assert.equal(request.singleAttempt, true); assert.equal(request.deadlineMs, 1500);
      assert.ok(request.body.monotonicMs >= request.body.playClockMs); assert.ok(request.body.seq >= 1);
    }
    for (const c of clients) assert.equal((await c.store.read()).entries.length, 0);
    assert.equal(wire.filter(request => request.path === '/v1/items/personal-state:batch').length, 2, 'lost answer never reauthors Saved');
    assert.equal(wire.filter(request => request.method === 'DELETE' && request.path.includes('-queries/')).length, 6, 'each prepared handle released');
    const body = personalBatchBody('offline-selfcheck', [{itemId: 'known-item', watchlisted: true}]);
    assert.equal(body.items[0].watchlisted, true); assert.throws(() => personalBatchBody('x', []));
    const before = wire.length; await delay(800); assert.equal(wire.length, before, 'no sends after retirement');
    assert.throws(() => options(['--execute', '--self-check']));
    for (const key of ['CI', 'GITHUB_ACTIONS']) {
      const previous = process.env[key]; process.env[key] = 'false';
      try {assert.throws(() => options(['--execute']), {code: 'execute_forbidden_in_ci'});}
      finally {if (previous === undefined) delete process.env[key]; else process.env[key] = previous;}
    }
    return {kind: 'offline-request-boundary-self-check', users: 2, seconds: 4, passed: true, observationRequests: observations.length,
      cleanupCommands: 2, networkRequests: 0, savedReceipts: 2, recoveredSavedReceipts: 1, preparedHandleReleases: 6,
      scope: 'real HttpLocalApi custody/receipt lookup, observer, final Stop, Saved, Browse/Reading preparation; full workload and capacity unrun'};
  } finally {
    reporters.forEach(reporter => reporter.stop()); clients.forEach(c => c.coordinator.dispose());
    await unlock?.(); await rm(directory, {recursive: true, force: true});
  }
}
