#!/usr/bin/env node
/**
 * ONB-01 integration check: drives a throwaway server's first-run endpoints the
 * way the web setup screen does, with generated throwaway credentials, for both
 * branches. Not part of `npm test` (it needs a server binary):
 *
 *   PORTICO_SERVER_BIN=/path/to/server node test/integration/setup-flow.mjs
 *
 * Each branch starts its own server on a fresh temporary state directory and
 * removes it afterwards. Nothing touches a real server.
 */
import {spawn} from 'node:child_process';
import {mkdtempSync, realpathSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {randomBytes, randomUUID} from 'node:crypto';
import assert from 'node:assert/strict';

const bin = process.env.PORTICO_SERVER_BIN;
if (!bin) { console.error('Set PORTICO_SERVER_BIN to a Portico server binary.'); process.exit(2); }

async function withServer(port, run) {
  // The server insists on canonical paths (macOS tmp is behind a symlink).
  const state = realpathSync(mkdtempSync(path.join(tmpdir(), 'portico-setup-')));
  const child = spawn(bin, [], {env: {...process.env, PORTICO_STATE_DIR: state, PORTICO_BIND: `127.0.0.1:${port}`}, stdio: ['ignore', 'pipe', 'pipe']});
  let log = '';
  child.stderr.on('data', d => { log += d; });
  child.stdout.on('data', d => { log += d; });
  const exited = new Promise(done => child.once('exit', code => done(code)));
  const base = `http://127.0.0.1:${port}`;
  try {
    for (let i = 0; i < 60; i++) {
      try { const r = await fetch(base + '/v1/system'); if (r.ok) break; } catch {}
      await new Promise(done => setTimeout(done, 250));
    }
    await run(base);
  } catch (e) {
    console.error(log.split('\n').slice(-8).join('\n'));
    throw e;
  } finally {
    if (child.exitCode === null) child.kill('SIGTERM');
    await exited;
    rmSync(state, {recursive: true, force: true});
  }
}

const json = async (base, route, {method = 'GET', body, token, origin} = {}) => {
  const headers = {Accept: 'application/json'};
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (token) headers.Authorization = 'Bearer ' + token;
  if (origin) headers.Origin = origin;
  const response = await fetch(base + route, {method, headers, body: body === undefined ? undefined : JSON.stringify(body)});
  const text = await response.text();
  return {status: response.status, body: text ? JSON.parse(text) : null};
};

/** Throwaway credentials that satisfy the server's recovery-password rule. */
const throwaway = () => ({username: 'owner-' + randomBytes(3).toString('hex'), password: 'Test-' + randomBytes(12).toString('base64url') + '9x'});

async function setup(base, authMode) {
  const system = await json(base, '/v1/system');
  assert.equal(system.body.setupRequired, true, 'a fresh server reports setupRequired');
  const token = await json(base, '/v1/setup/browser', {method: 'POST', body: {}, origin: base});
  assert.equal(token.status, 200, 'the local browser gets a setup token: ' + JSON.stringify(token.body));
  const creds = throwaway();
  const requestId = randomUUID();
  const created = await json(base, '/v1/setup', {method: 'POST', body: {requestId, setupToken: token.body.setupToken, username: creds.username, password: creds.password, name: 'Setup test', authMode, recoverySaved: true, interactive: true}});
  assert.equal(created.status, 201, 'owner created: ' + JSON.stringify(created.body));
  assert.equal(created.body.viewer.role, 'owner');
  assert.equal(created.body.viewer.authority, 'local');
  const resumed = await json(base, '/v1/setup/resume', {method: 'POST', body: {requestId, setupToken: token.body.setupToken}});
  assert.equal(resumed.status, 200, 'a reload resumes the same setup: ' + JSON.stringify(resumed.body));
  const after = await json(base, '/v1/system');
  assert.equal(after.body.setupRequired, false, 'setup is no longer required once the owner exists');
  assert.equal(after.body.name, 'Setup test', 'the chosen server name is kept');
  const stateRead = await json(base, '/v1/setup/state', {token: created.body.accessToken});
  assert.equal(stateRead.status, 200, 'the owner can read setup state: ' + JSON.stringify(stateRead.body));
  return {access: created.body.accessToken, state: stateRead.body};
}

await withServer(32651, async base => {
  const {access, state} = await setup(base, 'local');
  assert.equal(state.authMode, 'local');
  assert.equal(state.ready, false, 'the checklist shows until setup is finished');
  const finished = await json(base, '/v1/setup/finish', {method: 'POST', body: {revision: state.revision}, token: access});
  assert.equal(finished.status, 200, 'Finish setup succeeds: ' + JSON.stringify(finished.body));
  assert.equal(finished.body.ready, true);
  console.log('ok - "Keep accounts on this server": owner created, resumable, finished');
});

await withServer(32652, async base => {
  const {access, state} = await setup(base, 'hosted');
  assert.equal(state.authMode, 'hosted');
  assert.equal(state.recoveryOwner, true, 'the recovery owner exists');
  assert.equal(state.claimInstalled, false, 'the Portico Account claim is still pending');
  const early = await json(base, '/v1/setup/finish', {method: 'POST', body: {revision: state.revision}, token: access});
  assert.notEqual(early.status, 200, 'setup cannot finish before the claim lands');
  console.log('ok - "Use a Portico Account": recovery owner created, claim pending, finish refused until claimed');
});
