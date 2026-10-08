/**
 * The claim-code flow against the server's shapes
 * (server/internal/networking/claim_handler.go + claim_web.go) and Hosted's
 * (portico-internal: hosted-services/hosted/internal/httpapi/claim_web.go). Hosted isn't deployable from
 * here, so these pin the request/response contract; the end-to-end check runs
 * at the next Hosted release.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {awaitWebClaim, beginWebClaim, ClaimError, claimErrorMessage, safeApprovalUrl} from '../src/app/claim-connect.ts';

const pin = {serverId: 'srv_abc', publicKey: 'pk', fingerprint: 'fp'};
const identity = {serverId: 'srv_abc', localGeneration: '0'};
const operation = {operationId: 'op_1', localGeneration: '0', revision: '1'};
const code = 'C'.repeat(49);
const approvalUrl = `https://web.getportico.tv/claim#code=${code}&server=Den&return=https%3A%2F%2Fden.example`;

function server(script: Record<string, (body: any) => unknown>) {
  const calls: {path: string; method: string; body: any}[] = [];
  return {calls, api: {request: async (path: string, method = 'GET', body?: unknown) => {
    calls.push({path, method, body});
    const handler = script[`${method} ${path}`];
    if (!handler) throw new Error(`unexpected ${method} ${path}`);
    return handler(body) as never;
  }}};
}

test('claim: prepare with no account (BE-hosted d37e441), then web-approval returns the approval URL and code', async () => {
  const {calls, api} = server({
    'GET /v1/networking/claim': () => ({state: 'unclaimed', identity, actions: ['prepare'], installationAcknowledged: false, approvalRequired: false}),
    'POST /v1/networking/claim/prepare': () => ({state: 'prepared', identity, operation, actions: ['cancel', 'approve'], installationAcknowledged: false, approvalRequired: true}),
    'POST /v1/networking/claim/web-approval': () => ({state: 'prepared', identity, operation, actions: ['cancel', 'approve'], installationAcknowledged: false, approvalRequired: true, approvalUrl, claimCode: code}),
  });
  const {status, expected} = await beginWebClaim(api, pin);
  assert.deepEqual(calls.map(c => `${c.method} ${c.path}`), ['GET /v1/networking/claim', 'POST /v1/networking/claim/prepare', 'POST /v1/networking/claim/web-approval']);
  assert.deepEqual(calls[1]!.body, {expected: identity});
  assert.deepEqual(calls[2]!.body, {expected: operation});
  assert.deepEqual(calls[2]!.body, {expected: operation});
  assert.equal(status.approvalUrl, approvalUrl);
  assert.equal(status.claimCode, code);
  assert.deepEqual(expected, operation);
  assert.equal(safeApprovalUrl(status.approvalUrl, 'https://web.getportico.tv'), approvalUrl);
});

test('claim: an existing prepared claim is reused (no second prepare)', async () => {
  const {api, calls} = server({
    'GET /v1/networking/claim': () => ({state: 'prepared', identity, operation, actions: ['cancel', 'approve'], installationAcknowledged: false, approvalRequired: true}),
    'POST /v1/networking/claim/web-approval': () => ({state: 'prepared', identity, operation, actions: ['cancel', 'approve'], installationAcknowledged: false, approvalRequired: true, approvalUrl, claimCode: code}),
  });
  await beginWebClaim(api, pin);
  assert.deepEqual(calls.map(c => c.path), ['/v1/networking/claim', '/v1/networking/claim/web-approval']);
});

test('claim: errors map to catalogue copy (400 invalid_request, 429 too many, 409 claim_stale, closed)', () => {
  assert.equal(claimErrorMessage({status: 400, code: 'invalid_request'}), 'web.claim.errorInvalid');
  assert.equal(claimErrorMessage({status: 429, code: 'rate_limited'}), 'web.claim.errorTooMany');
  assert.equal(claimErrorMessage({status: 409, code: 'claim_stale'}), 'web.claim.errorStale');
  assert.equal(claimErrorMessage(new ClaimError('approval_closed')), 'web.claim.closed');
  assert.equal(claimErrorMessage(new ClaimError('approval_changed')), 'web.claim.closed');
  assert.equal(claimErrorMessage({status: 503}), undefined);
});

test('claim: await-approval finishes the claim (continue when installed but unacknowledged)', async () => {
  const {calls, api} = server({
    'POST /v1/networking/claim/await-approval': () => ({state: 'installed', identity, operation, accountId: 'acc_1', actions: ['cancel', 'continue'], installationAcknowledged: false, approvalRequired: false}),
    'POST /v1/networking/claim/continue': () => ({state: 'installed', identity, operation, accountId: 'acc_1', actions: ['cancel'], installationAcknowledged: true, approvalRequired: false}),
  });
  const final = await awaitWebClaim(api, pin, operation);
  assert.equal(final.installationAcknowledged, true);
  assert.deepEqual(calls.map(c => c.path), ['/v1/networking/claim/await-approval', '/v1/networking/claim/continue']);
  assert.deepEqual(calls[0]!.body, {expected: operation});
});

test('claim: a denied or expired code ends as approval_closed', async () => {
  const {api} = server({'POST /v1/networking/claim/await-approval': () => ({state: 'approval_closed'})});
  await assert.rejects(awaitWebClaim(api, pin, operation), (e: unknown) => e instanceof ClaimError && e.code === 'approval_closed');
});

test('claim: only the Portico Account /claim page with a code may be opened', () => {
  assert.equal(safeApprovalUrl('https://evil.example/claim#code=x', 'https://web.getportico.tv'), undefined);
  assert.equal(safeApprovalUrl('https://web.getportico.tv/other#code=x', 'https://web.getportico.tv'), undefined);
  assert.equal(safeApprovalUrl('https://web.getportico.tv/claim', 'https://web.getportico.tv'), undefined);
});
