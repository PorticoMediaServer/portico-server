/** Operator-only, UNRUN load driver. See load-README.md. Node 24.21.0, no install/build. */
import {hostname, networkInterfaces} from 'node:os';
import {open, constants} from 'node:fs/promises';
import {pathToFileURL} from 'node:url';
import {setTimeout as delay} from 'node:timers/promises';
import {randomUUID, createHash} from 'node:crypto';
import {type LocalSession} from '../../packages/client-core/src/index.ts';
import {join} from 'node:path';
import {FileMutationStore, lockMutationDirectory, mutationClient, savedBatch, finalStop} from './load-adapters.ts';
import {readBrowsePage, releaseBrowseSelection, readReadingFacet} from '../../packages/client-core/src/browse-preparation.ts';
import {validInstallationId} from '../../packages/client-core/src/installation.ts';
import {capabilityDocumentProblem, type CapabilityDocument} from '../../packages/client-core/src/client-profile.ts';
import {BrowseQueryBuilder, browseFacetsPath, browseCapabilitiesPath, parseBrowseCapabilities} from '../../packages/client-core/src/browse.ts';
import {SearchService, searchPath} from '../../packages/client-core/src/search.ts';
import {HomeService} from '../../packages/client-core/src/home-service.ts';
import {LibraryManagementService} from '../../packages/client-core/src/library-management.ts';
import {ObservationReporter} from '../../packages/client-core/src/playback-v1/observation.ts';
import {CapabilityPublisher} from '../../packages/client-core/src/playback-v1/options.ts';
import {EventsClient} from '../../packages/client-core/src/playback-v1/events.ts';
import {readEventStream} from '../../packages/client-core/src/sse.ts';
import {routePath} from '../../packages/contracts/src/route-paths.ts';

type Identity = {accessToken: string; expiresAt: string; sessionFamilyId: string; tokenGeneration: string; authorizationHorizon: string; authority: 'local' | 'hosted'; accountId: string;
  profileId: string; deviceId: string; installationId: string; role: 'owner' | 'member'; capabilities: CapabilityDocument};
type User = Identity & {libraryId: string; pivot: string; search: string; itemIds: string[];
  bytesPerSecond: number; directVerifiedAt: string; reading: {libraryId: string; pivot: string; facet: string}};
export type Manifest = {version: 1; target: string; serverId: string; conversionMode: 'never';
  stateAdapterReceipt: string; owner: Identity & {libraryId: string}; quietMember: Identity; users: User[]};
export type Options = {execute: boolean; selfCheck: boolean; users: number; seconds: number; ramp: number;
  target?: string; manifest?: string; out?: string; mutationStore?: string; label: string; window: string; hardwareUsers: number;
  readP99: number; writeP99: number; observationGap: number};
function fault(code: string): never {throw Object.assign(new Error(code), {code});}
const id = (v: unknown): v is string => typeof v === 'string' && /^[A-Za-z0-9_.:-]{1,256}$/.test(v);
const now = () => performance.now();
const sleep = async (ms: number, signal: AbortSignal) => {await delay(Math.max(1, ms), undefined, {signal});};
const aborted = (e: unknown) => ['AbortError', 'TimeoutError'].includes(String((e as any)?.name));

export function options(args: string[]): Options {
  const flags = new Map<string, string>();
  const boolean = new Set(['execute', 'dry-run', 'self-check']);
  const names = new Set(['users', 'seconds', 'ramp-seconds', 'target', 'manifest', 'out', 'mutation-store', 'label', 'operator-window',
    'hardware-users', 'read-p99-ms', 'write-p99-ms', 'observation-gap-ms', ...boolean]);
  for (let i = 0; i < args.length; i++) {
    const key = args[i].replace(/^--/, '');
    if (!args[i].startsWith('--') || !names.has(key) || flags.has(key)) fault('invalid_arguments');
    const value = boolean.has(key) ? 'true' : args[++i];
    if (!value || value.startsWith('--')) fault('missing_argument');
    flags.set(key, value);
  }
  const num = (key: string, fallback: number, min: number, max: number) => {
    const value = Number(flags.get(key) ?? fallback);
    if (!Number.isSafeInteger(value) || value < min || value > max) fault('invalid_numeric_argument');
    return value;
  };
  const selfCheck = flags.has('self-check'), execute = flags.has('execute');
  // Presence, including CI=false, is deliberately sufficient. No override exists.
  if (execute && ('CI' in process.env || 'GITHUB_ACTIONS' in process.env)) fault('execute_forbidden_in_ci');
  if (execute && (selfCheck || flags.has('dry-run'))) fault('conflicting_modes');
  if (selfCheck && (flags.has('manifest') || flags.has('target') || flags.has('mutation-store'))) fault('self_check_has_no_private_input');
  const users = num('users', selfCheck ? 2 : 80, 2, 100), seconds = num('seconds', selfCheck ? 4 : 600, 4, 7200);
  if (![2, 80, 100].includes(users) || users === 2 && seconds !== 4 || selfCheck && (users !== 2 || seconds !== 4)) fault('unsupported_workload');
  const label = flags.get('label') ?? 'baseline', window = flags.get('operator-window') ?? 'none';
  if (!['baseline', 'scheduled-backup', 'manual-backup'].includes(label) || !/^[A-Za-z0-9_.:-]{1,80}$/.test(window)) fault('invalid_receipt_label');
  if (label !== 'baseline' && window === 'none') fault('backup_requires_operator_window');
  return {execute, selfCheck, users, seconds, ramp: num('ramp-seconds', users === 2 ? 1 : 30, 1, 600),
    target: flags.get('target'), manifest: flags.get('manifest'), out: flags.get('out'), mutationStore: flags.get('mutation-store'), label, window,
    hardwareUsers: num('hardware-users', 2, 1, 3), readP99: num('read-p99-ms', 2000, 10, 60000),
    writeP99: num('write-p99-ms', 2000, 10, 60000), observationGap: num('observation-gap-ms', 2500, 1500, 10000)};
}

export function requireDriver(selfCheck: boolean) {
  const host = hostname().toLowerCase();
  const addresses = Object.values(networkInterfaces()).flat().map(x => x?.address);
  if (/mini[-_]?1|ehler-personal/.test(host) || addresses.includes('100.120.50.24')) fault('driver_forbidden_on_mini1');
  if (!addresses.includes('100.113.197.98') && !(host === 'portico-runner' && !selfCheck)) fault('driver_requires_mini2_or_runner');
}
export function targetOrigin(value: string) {
  let u: URL; try {u = new URL(value);} catch {fault('invalid_target');}
  if (!['http:', 'https:'].includes(u!.protocol) || u!.hostname !== '100.120.50.24' || u!.username || u!.password
    || u!.pathname !== '/' || u!.search || u!.hash) fault('target_not_allowlisted');
  return u!.origin;
}
export function validateManifest(m: Manifest, o: Options) {
  if (m?.version !== 1 || m.conversionMode !== 'never' || !id(m.serverId) || !id(m.stateAdapterReceipt)
    || !Array.isArray(m.users) || m.users.length < o.users || m.users.length > 100) fault('manifest_state_or_user_gap');
  if (targetOrigin(m.target) !== targetOrigin(o.target ?? '')) fault('manifest_target_mismatch');
  const all = [m.owner, m.quietMember, ...m.users];
  for (const p of all) {
    if (!p || !['local', 'hosted'].includes(p.authority) || !['owner', 'member'].includes(p.role)
      || ![p.accountId, p.profileId, p.deviceId, p.sessionFamilyId, p.tokenGeneration].every(id) || !validInstallationId(p.installationId)
      || typeof p.accessToken !== 'string' || !p.accessToken || /\s/.test(p.accessToken)
      || !Number.isFinite(Date.parse(p.expiresAt)) || Date.parse(p.expiresAt) < Date.now() + (o.ramp + o.seconds + 90) * 1000
      || !Number.isFinite(Date.parse(p.authorizationHorizon)) || Date.parse(p.authorizationHorizon) < Date.parse(p.expiresAt)
      || capabilityDocumentProblem(p.capabilities)) fault('manifest_identity_or_capability_gap');
  }
  for (const key of ['accessToken', 'accountId', 'profileId', 'deviceId', 'installationId', 'sessionFamilyId'] as const)
    if (new Set(all.map(p => p[key])).size !== all.length) fault('manifest_identities_must_be_distinct');
  if (m.owner.role !== 'owner' || m.quietMember.role !== 'member' || !id(m.owner.libraryId)) fault('manifest_owner_or_quiet_gap');
  for (const u of m.users) if (!u.reading || ![u.reading.libraryId, u.reading.pivot, u.reading.facet].every(id) || u.role !== 'member' || !id(u.libraryId) || !id(u.pivot) || typeof u.search !== 'string'
    || !u.search.trim() || u.search.length > 80 || !Array.isArray(u.itemIds) || !u.itemIds.length || u.itemIds.length > 20
    || !u.itemIds.every(id) || new Set(u.itemIds).size !== u.itemIds.length || !Number.isSafeInteger(u.bytesPerSecond)
    || u.bytesPerSecond < 1024 || u.bytesPerSecond > 16 * 1024 * 1024 || !Number.isFinite(Date.parse(u.directVerifiedAt))) fault('manifest_playable_item_gap');
}
async function privateManifest(path: string): Promise<Manifest> {
  const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  try {
    const s = await file.stat();
    if (!s.isFile() || (s.mode & 0o777) !== 0o600 || s.uid !== process.getuid?.() || s.size > 4 * 1024 * 1024) fault('manifest_requires_owned_0600_file');
    try {return JSON.parse(await file.readFile('utf8'));} catch {fault('manifest_invalid_json');}
  } finally {await file.close();}
}

// Fixed logarithmic buckets, including an overflow bucket. Quantiles are conservative upper bounds.
const bounds = Array.from({length: 48}, (_, i) => Math.ceil(1.3 ** i));
class Histogram {
  bins = new Array(bounds.length + 1).fill(0); count = 0; maxMs = 0;
  add(ms: number) {this.count++; this.maxMs = Math.max(this.maxMs, ms); const i = bounds.findIndex(b => ms <= b); this.bins[i < 0 ? bounds.length : i]++;}
  percentile(fraction: number) {let n = 0; for (let i = 0; i < this.bins.length; i++) if ((n += this.bins[i]) >= Math.ceil(this.count * fraction)) return bounds[i] ?? this.maxMs; return 0;}
  p99() {return this.percentile(.99);}
  receipt() {return {count: this.count, p95UpperMs: this.percentile(.95), p99UpperMs: this.p99(), maxMs: Math.ceil(this.maxMs), bins: this.bins};}
}
class Metrics {
  ops: Record<string, {attempts: number; ok: number; errors: number; refusals: number; cancelled: number; latency: Histogram; codes: Record<string, number>}> = {};
  counters: Record<string, number> = {};
  inc(k: string, n = 1) {this.counters[k] = (this.counters[k] ?? 0) + n;}
  async measure<T>(name: string, fn: () => Promise<T>): Promise<T> {
    const s = this.ops[name] ??= {attempts: 0, ok: 0, errors: 0, refusals: 0, cancelled: 0, latency: new Histogram(), codes: {}};
    s.attempts++; const at = now();
    try {const value = await fn(); s.ok++; return value;}
    catch (e) {
      if (aborted(e)) s.cancelled++; else {
        s.errors++; const status = Number((e as any)?.status);
        if ([401, 403, 409, 412, 429, 503].includes(status)) s.refusals++;
        // Never echo server messages, arbitrary codes, paths, query text, grants or bearer values.
        const key = [400, 401, 403, 404, 409, 410, 412, 429, 500, 502, 503, 504].includes(status) ? String(status) : 'other';
        s.codes[key] = (s.codes[key] ?? 0) + 1;
      } throw e;
    } finally {s.latency.add(now() - at);}
  }
  receipt() {return Object.fromEntries(Object.entries(this.ops).map(([k, v]) => [k, {...v, latency: v.latency.receipt()}]));}
}
type UserReceipt = {index: number; bytes: number; observations: number; maxObservationGapMs: number;
  mediaStarted: boolean; mediaActive: boolean; startedAt: number; lastAck: number; expectedBytes: number; stopped: boolean};

export async function run(m: Manifest, o: Options, transport: typeof fetch = globalThis.fetch) {
  const metrics = new Metrics(), controller = new AbortController(), signal = controller.signal;
  const started = now(), wallStarted = new Date().toISOString(), end = started + (o.ramp + o.seconds) * 1000;
  let interrupted = false, plateauUsers = 0;
  const interrupt = () => {interrupted = true; controller.abort();};
  process.once('SIGINT', interrupt); process.once('SIGTERM', interrupt);
  const deadline = setTimeout(() => controller.abort(), end - now());
  const receipts: UserReceipt[] = [], retired: (() => void)[] = [], streams = new Set<Promise<unknown>>();
  const scopedFetch = (p: Identity): typeof fetch => async (input, init = {}) => {
    const url = new URL(String(input));
    if (url.origin !== m.target || url.username || url.password) fault('cross_origin_request_refused');
    const method = init.method ?? 'GET';
    const headers = new Headers(init.headers);
    // Installation globals are app-wide: inject the captured installation per transport, never swap globals.
    if (method === 'POST' && /\/v1\/(?:auth|direct)\//.test(url.pathname)) headers.set('X-Portico-Installation-Id', p.installationId);
    const response = await transport(input, {...init, headers, redirect: 'error', credentials: 'omit', referrerPolicy: 'no-referrer'});
    metrics.inc('wireRequests');
    if (!response.ok) metrics.inc('wireHTTP' + (response.status >= 400 && response.status <= 599 ? response.status : 'Other'));
    return response;
  };
  const clients: (Awaited<ReturnType<typeof mutationClient>> & {store: FileMutationStore})[] = [];
  const client = async (p: Identity) => {
    if (!o.mutationStore) fault('persistent_mutation_store_required');
    const session: LocalSession = {accessToken: p.accessToken, expiresAt: p.expiresAt, sessionFamilyId: p.sessionFamilyId,
      tokenGeneration: p.tokenGeneration, authorizationHorizon: p.authorizationHorizon, deviceId: p.deviceId, installationId: p.installationId,
      viewer: {serverId: m.serverId, authority: p.authority, accountId: p.accountId, profileId: p.profileId, role: p.role}};
    const name = createHash('sha256').update(JSON.stringify([m.target, m.serverId, p.authority, p.accountId, p.profileId, p.sessionFamilyId, p.deviceId])).digest('hex');
    const store = new FileMutationStore(join(o.mutationStore, name + '.json'));
    const c = await mutationClient(m.target, session, store, scopedFetch(p));
    const bound = {request: <T>(path: string, method = 'GET', body?: unknown, other?: AbortSignal) =>
      c.api.request<T>(path, method, body, AbortSignal.any([signal, ...(other ? [other] : []), AbortSignal.timeout(5000)])),
      requestRaw: c.api.requestRaw.bind(c.api), acknowledgeUserMutation: c.api.acknowledgeUserMutation.bind(c.api)};
    const result = {...c, store, bound, fetch: scopedFetch(p)}; clients.push(result); return result;
  };
  const scope = (p: Identity) => ({serverId: m.serverId, viewerId: JSON.stringify([p.authority, p.accountId, p.profileId])});
  const verify = async (p: Identity, c: Awaited<ReturnType<typeof client>>) => metrics.measure('identity', async () => {
    const {viewer} = await c.api.me(AbortSignal.any([signal, AbortSignal.timeout(5000)]));
    if (!viewer || viewer.serverId !== m.serverId || viewer.accountId !== p.accountId || viewer.profileId !== p.profileId
      || viewer.authority !== p.authority || viewer.role !== p.role) fault('actual_identity_mismatch');
    const devices: any = await c.bound.request(routePath('get_devices'));
    const current = devices?.items?.filter((d: any) => d.current);
    if (current?.length !== 1 || current[0].id !== p.deviceId || current[0].installationId !== p.installationId) fault('actual_device_mismatch');
  });
  async function user(p: User, index: number) {
    const c = await client(p), home = new HomeService(c.bound, {timeoutMs: 5000, initialLimit: 20});
    const search = new SearchService({api: c.bound, scope: scope(p), timeoutMs: 5000, pageSize: 20});
    const life = new AbortController(), local = AbortSignal.any([signal, life.signal]);
    let stop: ReturnType<typeof finalStop> | undefined, reporter: ObservationReporter | undefined, sampleTimer: ReturnType<typeof setInterval> | undefined;
    let draining: Promise<unknown> | undefined;
    const r: UserReceipt = {index, bytes: 0, observations: 0, maxObservationGapMs: 0, mediaStarted: false, mediaActive: false,
      startedAt: 0, lastAck: 0, expectedBytes: 0, stopped: false}; receipts.push(r);
    try {
      await sleep(index * o.ramp * 1000 / o.users, local);
      await verify(p, c);
      await metrics.measure('capabilities', async () => {
        if (await new CapabilityPublisher(c.http, () => p.deviceId).publish(p.capabilities, local) !== 'accepted') fault('capability_publication_failed');
      });
      const capabilities = await metrics.measure('browseCapabilities', async () => parseBrowseCapabilities(await c.bound.request(browseCapabilitiesPath(p.libraryId, p.pivot))));
      if (capabilities.library.id !== p.libraryId) fault('browse_library_mismatch');
      const body = new BrowseQueryBuilder(capabilities, p.pivot).limit(20).build();
      const itemId = p.itemIds[index % p.itemIds.length];
      const session = await metrics.measure('start', () => c.sessions.start({itemId}, {quality: {mode: 'original'}, subtitles: {trackId: null}, startFrom: 'beginning'}, randomUUID(), {
        signal: local, capturePreparing: s => {stop = finalStop(c.sessions, s.id, s.presentation.generation); return () => stop!(undefined, AbortSignal.timeout(5000));}}));
      stop = finalStop(c.sessions, session.id, session.presentation.generation);
      const presentation = session.presentation, decisions = Object.values(presentation.decision).filter(Boolean);
      if (session.itemId !== itemId || session.state !== 'playing' || presentation.mode !== 'direct'
        || !decisions.length || decisions.some(d => d!.action !== 'copy')) fault('direct_only_plan_required');
      const mediaURL = new URL(presentation.url, m.target);
      if (mediaURL.origin !== m.target || mediaURL.username || mediaURL.password || !mediaURL.pathname.startsWith('/v1/media/')) fault('media_origin_or_path_refused');
      reporter = new ObservationReporter({everyMs: 1000,
        claim: b => metrics.measure('claim', () => c.sessions.claimObserver(session.id, b, local)),
        send: sample => metrics.measure('observation', async () => {
          const ack = await c.sessions.observe(session.id, sample, local);
          if (ack.acceptedSeq !== sample.seq) fault('observation_sequence_mismatch');
          const at = now(); r.maxObservationGapMs = Math.max(r.maxObservationGapMs, at - r.lastAck);
          r.lastAck = at; r.observations++; metrics.inc('observationAcks'); return ack;
        })});
      reporter.setGeneration(presentation.generation);
      let lastByte = 0;
      draining = metrics.measure('media', async () => {
        const response = await c.fetch(mediaURL.href, {headers: {Authorization: 'Bearer ' + p.accessToken}, signal: local});
        if (response.status !== 200 || !response.body || /(?:json|mpegurl|text\/)/i.test(response.headers.get('content-type') ?? '')) {
          await response.body?.cancel(); throw Object.assign(new Error('media_body_refused'), {status: response.status});
        }
        const reader = response.body.getReader(); const cancel = () => {void reader.cancel().catch(() => {});};
        local.addEventListener('abort', cancel, {once: true});
        r.startedAt = r.lastAck = now(); r.expectedBytes = p.bytesPerSecond * Math.max(0, end - r.startedAt) / 1000;
        r.mediaStarted = r.mediaActive = true; metrics.inc('mediaBodies');
        try {
          while (!local.aborted) {
            const readAt = now();
            const stall = setTimeout(() => life.abort(), 5000);
            let chunk: ReadableStreamReadResult<Uint8Array>;
            try {chunk = await reader.read();} finally {clearTimeout(stall);}
            if (local.aborted) break;
            if (chunk.done) fault('media_ended_before_window');
            metrics.inc('mediaReadMs', Math.ceil(now() - readAt));
            for (let offset = 0; offset < chunk.value.length && !local.aborted; offset += 16384) {
              const size = Math.min(16384, chunk.value.length - offset);
              await sleep(r.startedAt + (r.bytes + size) / p.bytesPerSecond * 1000 - now(), local);
              r.bytes += size; lastByte = now(); metrics.inc('mediaBytes', size);
            }
          }
        } finally {r.mediaActive = false; local.removeEventListener('abort', cancel); await reader.cancel().catch(() => {}); reader.releaseLock();}
      });
      // Observe the simulated byte consumer, never assert decoder/renderer facts.
      sampleTimer = setInterval(() => reporter?.observe({generation: presentation.generation,
        state: lastByte && now() - lastByte < 2000 ? 'playing' : 'buffering',
        positionMs: Math.floor(r.bytes / p.bytesPerSecond * 1000), rate: 1}), 250);
      // A failed drain retires this user; keep its rejection handled until joined below.
      void draining.catch(() => life.abort());
      let cycle = 0;
      let pendingSaved: {operationId: string; items: {itemId: string; watchlisted: boolean}[]} | undefined;
      while (!local.aborted) {
        for (const [name, action] of [
          ['browse', async () => {
            const result = await readBrowsePage(c.bound, p.libraryId, body, {signal: local, isCurrent: () => !local.aborted});
            releaseBrowseSelection(c.bound, p.libraryId, result.response);
          }],
          ['reading', async () => {
            const capabilities = parseBrowseCapabilities(await c.bound.request(browseCapabilitiesPath(p.reading.libraryId, p.reading.pivot)));
            if (capabilities.library.id !== p.reading.libraryId) fault('reading_library_mismatch');
            const query = new BrowseQueryBuilder(capabilities, p.reading.pivot).limit(20).build();
            const lifetime = {signal: local, isCurrent: () => !local.aborted};
            const result = await readBrowsePage(c.bound, p.reading.libraryId, query, lifetime);
            releaseBrowseSelection(c.bound, p.reading.libraryId, result.response);
            const path = browseFacetsPath(p.reading.libraryId) + '?' + new URLSearchParams({field: p.reading.facet, pivot: p.reading.pivot, limit: '20'});
            await readReadingFacet(c.bound, p.reading.libraryId, path, p.reading.facet, lifetime);
          }],
          ['search', async () => {
            // SearchService builds searchPath and validates the echoed query, groups and scope.
            searchPath({q: p.search, record: false}, 20);
            await search.select(p.search, {record: false}); const s = search.getSnapshot();
            if (s.phase !== 'ready') throw s.error ?? new Error('search_not_ready');
            if (s.groups.some(g => g.status !== 'success')) fault('search_group_failed');
          }],
          ['home', async () => {await home.load(); const s = home.getSnapshot(); if (s.phase !== 'ready') throw s.error ?? new Error('home_not_ready');}],
        ] as const) {
          if (local.aborted) break;
          await metrics.measure(name, action).catch(e => {if (aborted(e)) throw e;});
          await sleep(o.selfCheck ? 80 : 700 + (index * 97 + cycle * 31) % 500, local);
        }
        if (!local.aborted && cycle++ % 3 === 0) await metrics.measure('saved', async () => {
          pendingSaved ??= {operationId: randomUUID(), items: p.itemIds.slice(0, 3).map(itemId => ({itemId, watchlisted: cycle % 2 === 1}))};
          const receipt = await savedBatch(c.api, m.serverId, pendingSaved.operationId, pendingSaved.items, local);
          pendingSaved = undefined;
          if (receipt.updated !== undefined) metrics.inc('savedItems', receipt.updated);
          if (receipt.recovered) metrics.inc('savedRecoveredReceipts');
        }).catch(e => {if (aborted(e)) throw e;});
        await sleep(o.selfCheck ? 100 : 1000, local);
      }
    } catch (e) {if (!aborted(e) && !signal.aborted) metrics.inc('userFailures');}
    finally {
      if (sampleTimer) clearInterval(sampleTimer);
      reporter?.stop(); home.dispose(); search.dispose(); life.abort();
      await draining?.catch(() => {});
      if (r.startedAt) r.maxObservationGapMs = Math.max(r.maxObservationGapMs, Math.min(end, now()) - r.lastAck);
      if (stop) await metrics.measure('stop', async () => {
        await stop!(Math.floor(r.bytes / p.bytesPerSecond * 1000), AbortSignal.timeout(5000)); r.stopped = true;
      }).catch(() => {});
    }
  }
  async function scanOwner() {
    const c = await client(m.owner), service = new LibraryManagementService({api: c.bound, scope: scope(m.owner), readRetries: 0, timeoutMs: 5000});
    retired.push(() => service.dispose()); await verify(m.owner, c);
    await service.selectLibrary(m.owner.libraryId);
    while (!signal.aborted) {
      await sleep(o.selfCheck ? 600 : 60000, signal);
      await metrics.measure('scan', async () => {
        await service.scan(); const s = service.getSnapshot();
        if (s.mutation.phase !== 'complete') throw s.mutation.error ?? new Error('scan_receipt_failed');
        metrics.inc('scanReceipts');
      }).catch(e => {if (aborted(e)) throw e;});
    }
  }
  async function quietMember() {
    const p = m.quietMember, c = await client(p); await verify(p, c);
    const events = new EventsClient({http: c.http, openStream: async (after, receive, s) => {
      const local = AbortSignal.any([s, signal]);
      const task = metrics.measure('sse', async () => {
        // Headers have a deadline; the quiet stream deliberately survives without events.
        const timeout = setTimeout(() => controller.abort(), 5000);
        let response: Response;
        try {response = await c.fetch(m.target + routePath('get_events'), {signal: local,
          headers: {Authorization: 'Bearer ' + p.accessToken, Accept: 'text/event-stream', ...(after ? {'Last-Event-ID': after} : {})}});} finally {clearTimeout(timeout);}
        if (response.status !== 200 || !response.headers.get('content-type')?.includes('text/event-stream')) {
          await response.body?.cancel(); throw Object.assign(new Error('sse_refused'), {status: response.status});
        }
        metrics.inc('sseOpened');
        await readEventStream(response, frame => {metrics.inc('sseFrames'); receive(frame);}, local);
      });
      streams.add(task); try {await task;} finally {streams.delete(task);}
    }});
    retired.push(() => events.stop()); events.onAny(() => metrics.inc('sseEvents')); events.start();
  }
  try {
    // There is no hardware-only session launch contract in this pin. No conversion starts.
    metrics.inc('hardwareOnlyRequested', o.hardwareUsers); metrics.inc('hardwareOnlyLocallyRefused', o.hardwareUsers);
    const plateau = setTimeout(() => {plateauUsers = receipts.filter(r => r.mediaActive && r.bytes > 0).length;}, o.ramp * 1000 + 1000);
    retired.push(() => clearTimeout(plateau));
    await Promise.allSettled([scanOwner().catch(e => {if (!aborted(e)) metrics.inc('ownerFailures');}),
      quietMember().catch(e => {if (!aborted(e)) metrics.inc('quietMemberFailures');}), ...m.users.slice(0, o.users).map(user)]);
  } finally {
    controller.abort(); clearTimeout(deadline); retired.forEach(f => f());
    await Promise.allSettled([...streams]);
    clients.forEach(c => c.coordinator.dispose());
    process.removeListener('SIGINT', interrupt); process.removeListener('SIGTERM', interrupt);
  }
  const achieved = receipts.filter(r => r.bytes > 0 && r.observations > 0).length;
  const enough = (key: string) => (metrics.ops[key]?.ok ?? 0) >= o.users;
  const gates = {
    uninterrupted: !interrupted, users: achieved === o.users, plateauUsers: plateauUsers === o.users,
    mediaBytes: receipts.length === o.users && receipts.every(r => r.mediaStarted && r.bytes >= r.expectedBytes * .8),
    observations: receipts.every(r => r.observations >= Math.floor(o.seconds * .7) && r.maxObservationGapMs <= o.observationGap),
    reads: ['browse', 'reading', 'search', 'home'].every(enough), savedReceipts: enough('saved'),
    scanReceipts: (metrics.counters.scanReceipts ?? 0) >= (o.selfCheck ? 1 : Math.floor((o.ramp + o.seconds) / 60)),
    quietSSE: (metrics.counters.sseOpened ?? 0) > 0,
    cleanup: receipts.every(r => !r.mediaStarted || r.stopped),
    mutationCustody: (await Promise.all(clients.map(c => c.store.read()))).every(state => state.entries.length === 0),
    noErrorsOrRefusals: !Object.values(metrics.ops).some(s => s.errors) && !Object.keys(metrics.counters).some(k => /Failures$|^wireHTTP/.test(k)),
    readLatency: ['browse', 'reading', 'search', 'home'].every(k => !!metrics.ops[k]?.latency.count && metrics.ops[k].latency.p99() <= o.readP99),
    writeLatency: ['saved', 'scan', 'claim', 'stop', 'observation'].every(k => !!metrics.ops[k]?.latency.count && metrics.ops[k].latency.p99() <= o.writeP99),
  };
  return {kind: o.selfCheck ? 'mock-transport-only' : 'simulated-client-load', label: o.label, operatorWindow: o.window,
    startedAt: wallStarted, elapsedSeconds: (now() - started) / 1000, requestedUsers: o.users, achievedUsers: achieved, plateauUsers,
    decoderAcceptance: false, deviceAcceptance: false, hardwareAcceptance: false, completeCampaignAcceptance: false,
    gaps: ['hardware_only_launch_contract_unavailable', 'refresh_storage_adapter_not_supplied', 'conversion_mode_operator_precondition', '102_identity_provisioning_unproved', '5m_full_appstate_adapter_unproved'],
    thresholds: {readP99Ms: o.readP99, writeP99Ms: o.writeP99, observationGapMs: o.observationGap, minimumByteFraction: .8},
    gates, passed: Object.values(gates).every(Boolean), counters: metrics.counters, operations: metrics.receipt(),
    histogramUpperBoundsMs: bounds, users: receipts.map(({startedAt, lastAck, ...r}) => r)};
}

export async function main(args = process.argv.slice(2)) {
  const o = options(args);
  if (!o.execute && !o.selfCheck) {
    console.log(JSON.stringify({state: 'UNRUN', mode: 'dry-run', users: o.users, seconds: o.seconds, rampSeconds: o.ramp,
      targetHost: '100.120.50.24', driver: 'mini2-or-portico-runner', label: o.label, operatorWindow: o.window,
      requirements: ['explicit --execute', 'owned 0600 manifest', 'owned 0700 persistent mutation-store directory', 'full appstate adapter', 'conversionMode=never', 'separate host observer'],
      hardwareOnly: 'unavailable: locally refused; never software fallback'})); return;
  }
  if (process.versions.node !== '24.21.0') fault('node_24_21_0_required');
  requireDriver(o.selfCheck);
  let receipt: {passed: boolean};
  if (o.selfCheck) {
    const {selfCheck} = await import('./load-selfcheck.mjs'); receipt = await selfCheck(o);
  } else {
    if (!o.manifest || !o.target || !o.out || !o.mutationStore) fault('execute_requires_manifest_target_out_mutation_store');
    o.target = targetOrigin(o.target); const m = await privateManifest(o.manifest); m.target = targetOrigin(m.target); validateManifest(m, o);
    // Reserve evidence before starting any request. Never overwrite a previous run.
    const out = await open(o.out, constants.O_CREAT | constants.O_EXCL | constants.O_WRONLY | constants.O_NOFOLLOW, 0o600);
    let unlock: (() => Promise<void>) | undefined;
    try {unlock = await lockMutationDirectory(o.mutationStore); receipt = await run(m, o); await out.writeFile(JSON.stringify(receipt, null, 2) + '\n');} finally {await out.close(); await unlock?.();}
  }
  console.log(JSON.stringify(receipt)); if (!receipt.passed) process.exitCode = 1;
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(() => {console.error('load harness refused or failed; no private details emitted. See load-README.md prerequisites.'); process.exitCode = 1;});
}
