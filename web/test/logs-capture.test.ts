import test from 'node:test';
import assert from 'node:assert/strict';
import {RETENTION_LIMITS, setDetailWindow} from '../../packages/client-core/src/server-admin/server-forms.ts';

// The message log's rules live in client-core (`server-admin/server-forms.ts`) since the Server
// pages moved under Settings: one control for extra detail, and the retention the server accepts.

test('CD-12: extra detail is one control: 30 minutes on, off again, each with its own operation id', async () => {
  const calls: {path: string; method: string; body: any}[] = [];
  let n = 0;
  const deps = {api: {request: async (path: string, method: string, body: unknown) => { calls.push({path, method, body}); return {}; }}, operationId: () => `op-${++n}`} as any;
  await setDetailWindow(deps, true);
  await setDetailWindow(deps, false);
  assert.deepEqual(calls.map(c => [c.path, c.method, c.body.minutes]), [['/v1/admin/logs/debug-window', 'POST', 30], ['/v1/admin/logs/debug-window', 'POST', 0]]);
  assert.notEqual(calls[0]!.body.operationId, calls[1]!.body.operationId);
});

test('CD-46: retention uses the registry ranges', () => {
  assert.deepEqual(RETENTION_LIMITS, {diagnosticDays: 30, notificationDays: 180, jobDays: 30});
});
