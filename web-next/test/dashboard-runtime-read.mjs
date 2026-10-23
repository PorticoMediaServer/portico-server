/** Manual real-fixture read after the sole writer supplies observed-ready,
 * stable scans and this consumer's explicit cutover slot. No fixture writes.
 * Run from the repository with the normal test/register.mjs TS resolver.
 */
import {spawn} from 'node:child_process';
import {execFileSync} from 'node:child_process';
import {createInterface} from 'node:readline';
import {readFileSync, writeFileSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {resolve, sep} from 'node:path';
import assert from 'node:assert/strict';
import {HttpLocalApi} from '../../packages/client-core/src/index.ts';
import {captureReportContext} from '../../packages/client-core/src/report-context.ts';
import {createDashboardBindings} from '../src/screens/dashboard/bindings.ts';
import {readNowPlaying} from '../src/screens/dashboard/pages.ts';

const configPath = process.argv[2];
if (!configPath) throw new Error('Assigned cutover config is required');
const config = JSON.parse(readFileSync(configPath, 'utf8'));
assert.equal(config.consumer, 'dashboard-web-r29');
assert.equal(config.cutoverAssigned, true);
const repository = fileURLToPath(new URL('../..', import.meta.url));
assert.equal(execFileSync('git', ['rev-parse', 'HEAD'], {cwd: repository, encoding: 'utf8'}).trim(), config.consumerSource);
assert.equal(execFileSync('git', ['status', '--porcelain'], {cwd: repository, encoding: 'utf8'}), '');
const evidenceRoot = '/home/justin/projects/Portico/.worktrees/ui-2026/_program/evidence/orchestrator-r29/dashboard-web-r29';
assert.ok(resolve(config.output).startsWith(evidenceRoot + sep) && config.output.endsWith('.json'));
const worker = spawn('python3', ['-u', fileURLToPath(new URL('./dashboard-runtime-worker.py', import.meta.url)), configPath], {stdio: ['pipe', 'pipe', 'pipe'], env: {...process.env, TMPDIR: config.scratch}});
let resolveReady, rejectReady, cleanup, privateError = false, current = true, sequence = 0;
const ready = new Promise((resolve, reject) => {resolveReady = resolve; rejectReady = reject;});
const pending = new Map(), requests = [], results = [];
worker.stderr.on('data', () => {privateError = true;});
const exited = new Promise(resolve => worker.once('close', code => {
  const error = new Error('Private fixture worker exited');
  rejectReady(error); for (const value of pending.values()) value.reject(error); pending.clear(); resolve(code);
}));
worker.stdin.on('error', () => {for (const value of pending.values()) value.reject(new Error('Private fixture pipe closed')); pending.clear();});
createInterface({input: worker.stdout}).on('line', line => {
  try {
    const value = JSON.parse(line);
    if (value.kind === 'ready') resolveReady(value);
    else if (value.kind === 'cleanup') cleanup = value;
    else if (value.kind === 'failure') {const error = new Error('Private fixture preflight failed'); rejectReady(error); for (const request of pending.values()) request.reject(error); pending.clear();}
    else if (value.kind === 'response') {const request = pending.get(value.id); pending.delete(value.id); if (value.error) request?.reject(new Error('Private fixture read failed')); else request?.resolve(value);}
  } catch {rejectReady(new Error('Invalid private worker message'));}
});
const fetcher = async (input, init) => {
  const url = new URL(String(input)), method = init?.method ?? 'GET', id = ++sequence;
  requests.push({path: url.pathname, method, query: Object.fromEntries(url.searchParams)});
  const raw = await new Promise((resolve, reject) => {pending.set(id, {resolve, reject}); worker.stdin.write(JSON.stringify({id, path: url.pathname + url.search, method}) + '\n');});
  return new Response(JSON.stringify(raw.body), {status: raw.status, headers: {'content-type': 'application/json'}});
};
const context = captureReportContext(Intl.DateTimeFormat().resolvedOptions().timeZone);
let bindings, actor, failure, stage = 'assigned public-system/owner-primary preflight';
try {
  actor = await ready;
  const api = new HttpLocalApi(config.api, '', fetcher);
  bindings = createDashboardBindings({api, context, scope: {serverId: actor.viewer.serverId, viewerId: actor.viewer.profileId}, role: actor.viewer.role, isCurrent: () => current, isMainProfile: () => actor.primary, artwork: () => undefined, effects: {openHome() {}, openSettings() {}, openHistory() {}, reportError() {}}});
  for (const period of ['today', '7d', '30d', '90d', '1y', 'all']) {
    stage = 'common report ' + period;
    const report = await bindings.read.report({period, peoplePeriod: period, topPeriod: period, window: period, topKind: 'movies', peopleLimit: 100});
    const expected = {today: 1, '7d': 7, '30d': 30, '90d': 90, '1y': 52}[period];
    if (expected) assert.equal(report.history.buckets.length, expected);
    for (const part of [report.history, report.people, report.summary, report.topPlayed, report.table, report.load]) {
      assert.equal(part.timeZone, context.timeZone); assert.equal(Date.parse(part.until), Date.parse(context.until)); assert.equal(part.from, report.history.from);
    }
    results.push({period, from: report.history.from, until: report.history.until, buckets: report.history.buckets.length, plays: report.summary.plays, tableTotal: report.table.total, peopleLoaded: report.people.items.length, peopleHasMore: !!report.people.nextCursor});
  }
  for (const period of ['today', '24h']) {
    stage = 'History ' + period;
    const page = await bindings.read.plays({period}); assert.equal(page.buckets.length, period === 'today' ? 1 : 24);
    results.push({history: period, from: page.from, until: page.until, buckets: page.buckets.length, entries: page.entries.length, total: page.total});
  }
  stage = 'moving Live';
  const live = await bindings.read.telemetry('live'); assert.equal(live.timeZone, context.timeZone);
  results.push({live: true, observedAt: live.observedAt, returnedUntil: live.until, metrics: Object.fromEntries(Object.entries(live.status).map(([key, value]) => [key, value.status]))});
  stage = 'Now playing';
  const sessions = await readNowPlaying(bindings, new AbortController().signal);
  results.push({sessionsLoaded: sessions.items.length, totals: sessions.totals});
  stage = 'authority retirement'; current = false;
  await assert.rejects(() => bindings.read.report({period: 'all', peoplePeriod: 'all', topPeriod: 'all', window: 'all', topKind: 'movies'}));
} catch (error) {failure = {stage, name: error?.name ?? 'Error'}; process.exitCode = 1;}
finally {
  bindings?.dispose(); worker.stdin.end();
  let timer; const code = await Promise.race([exited, new Promise(resolve => {timer = setTimeout(() => {worker.kill(); resolve(null);}, 30000);})]); clearTimeout(timer);
  if (code !== 0 || !cleanup || cleanup.failedCount || cleanup.capturedCount < 1) {failure ??= {stage: 'owned family logout', name: 'IncompleteCleanup'}; process.exitCode = 1;}
  writeFileSync(config.output, JSON.stringify({scope: 'Real fresh-runtime Dashboard API consumer read only; no full protected browser/shell, populated parity, immutable snapshot or full MAIN client graph acceptance',consumerSource: config.consumerSource, producerSource: config.producerSource, readyReceipt: config.readyReceipt, context, actor: actor?.viewer, results, requests, cleanup, privateError, failure}, null, 2) + '\n');
  console.log(JSON.stringify({reports: results.length, cleanup: cleanup ? {captured: cleanup.capturedCount, failed: cleanup.failedCount} : null, failure}));
}
