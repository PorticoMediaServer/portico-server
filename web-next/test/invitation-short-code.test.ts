import test from 'node:test';
import assert from 'node:assert/strict';
import {ShortCodeIssuer} from '../src/kit/accounts/invitation-short-code.ts';

const serverId = 'srv_' + 's'.repeat(43), expiresAt = '2099-01-01T00:00:00Z';

test('a New code is a fresh operation; retrying the same request reuses it', async () => {
  let next = 0;
  const issuer = new ShortCodeIssuer(() => ['operation-one', 'operation-two'][next++]!);
  const calls: string[] = [];
  const run = async (operationId: string) => {calls.push(operationId);return {serverId, expiresAt, shortCode: '008J4CT4ANK7F24S'};};
  try {
    await issuer.issue(true, run);
    assert.deepEqual(issuer.getSnapshot(), {phase: 'ready', operationId: 'operation-one', code: '008J-4CT4-ANK7-F24S', expiresAt});
    await issuer.retry(run);
    assert.equal(calls[1], 'operation-one');
    await issuer.issue(true, run);
    assert.equal(calls[2], 'operation-two');
    assert.deepEqual(calls, ['operation-one', 'operation-one', 'operation-two']);
  } finally {issuer.dispose();}
});

test('a receipt without a code is an absence and never rotates by itself', async () => {
  const issuer = new ShortCodeIssuer(() => 'replay-operation');
  let calls = 0;
  try {
    await issuer.issue(true, async () => {calls++;return {serverId, expiresAt};});
    assert.deepEqual(issuer.getSnapshot(), {phase: 'unavailable', operationId: 'replay-operation', expiresAt});
    await new Promise(resolve => setTimeout(resolve, 5));
    assert.equal(calls, 1);
    await issuer.issue(true, async () => {calls++;return {serverId, expiresAt, shortCode: '008J4CT4ANK7F24S'};});
    assert.equal(calls, 2);
    assert.equal(issuer.getSnapshot().phase, 'ready');
  } finally {issuer.dispose();}
});

test('a failed request shows the error without an automatic retry, and Try again keeps the operation', async () => {
  const issuer = new ShortCodeIssuer(() => 'captured-operation');
  const calls: string[] = [];
  try {
    const failure = Object.assign(new Error('refused'), {status: 409});
    await issuer.issue(true, async operationId => {calls.push(operationId);throw failure;});
    assert.deepEqual(issuer.getSnapshot(), {phase: 'failed', operationId: 'captured-operation', error: failure});
    assert.deepEqual(calls, ['captured-operation']);
    await issuer.retry(async operationId => {calls.push(operationId);return {serverId, expiresAt, shortCode: '008J4CT4ANK7F24S'};});
    assert.deepEqual(calls, ['captured-operation', 'captured-operation']);
    assert.equal(issuer.getSnapshot().phase, 'ready');
  } finally {issuer.dispose();}
});

test('closing or disposing aborts the held request and ignores its late code', async () => {
  let release!: (value: {serverId: string; expiresAt: string; shortCode: string}) => void;
  const issuer = new ShortCodeIssuer(() => 'held-operation');
  try {
    const pending = issuer.issue(true, (_operationId, signal) => new Promise((resolve, reject) => {
      release = resolve;
      signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), {name: 'AbortError'})), {once: true});
    }));
    assert.equal(issuer.getSnapshot().phase, 'issuing');
    issuer.reset();
    assert.deepEqual(issuer.getSnapshot(), {phase: 'idle'});
    release({serverId, expiresAt, shortCode: '008J4CT4ANK7F24S'});
    await pending;
    assert.deepEqual(issuer.getSnapshot(), {phase: 'idle'});
  } finally {issuer.dispose();}
});

test('a missing cryptographic id source fails closed without a request or a fabricated id', async () => {
  const issuer = new ShortCodeIssuer(() => {throw new Error('crypto unavailable');});
  let calls = 0;
  try {
    await issuer.issue(true, async () => {calls++;return {serverId, expiresAt, shortCode: '008J4CT4ANK7F24S'};});
    assert.equal(calls, 0);
    assert.equal(issuer.getSnapshot().phase, 'failed');
  } finally {issuer.dispose();}
});
