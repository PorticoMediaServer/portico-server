#!/usr/bin/env node
// M0 Hosted smoke: create, verify, sign in and manage a Portico Account on the live Hosted
// deployment, using Security's documented API for bootstrap and the browser for the flows.
//
//   SMOKE_HOSTED_ORIGIN=https://web.getportico.tv \
//   SMOKE_OUT=/path/to/scratch/hosted-<checkpoint> \
//   SMOKE_CHECKPOINT=v2-M0-001 \
//   SMOKE_REGISTRATION_FILE=/path/private/registration-<run>.json \
//   node smoke/hosted/run.mjs
//
// The registration id is written to SMOKE_REGISTRATION_FILE (mode 600) so Security can drop the
// real mail code into the same protected file. Nothing bypasses email verification: if no code
// arrives, the verification step is reported BLOCKED, not passed.
//
// Account creation and verification go through Playwright's context.request so the HttpOnly
// continuation cookie is the browser's own; no token is injected into page storage.
import {mkdirSync, readFileSync, writeFileSync, existsSync, chmodSync} from 'node:fs';
import {join} from 'node:path';
import {randomUUID} from 'node:crypto';
import {createReport, assert, blocked} from '../lib/report.mjs';
import {loadPlaywright, launch, Evidence, settle, clickAny, findAny, bodyText, surfaceState} from '../lib/page.mjs';

const origin = (process.env.SMOKE_HOSTED_ORIGIN || 'https://web.getportico.tv').replace(/\/$/, '');
const out = process.env.SMOKE_OUT || join(process.cwd(), 'smoke-out');
const checkpoint = process.env.SMOKE_CHECKPOINT || '';
// Required: Security's reader writes the issued code into this exact protected file.
// It replaces the file with {code, expiresAt}, so the account record lives beside it and is
// never written by anything but this runner.
const registrationFile = process.env.SMOKE_REGISTRATION_FILE || '';
const credentialsFile = process.env.SMOKE_CREDENTIALS_OUT ||
  (registrationFile ? registrationFile.replace(/registration\.json$/, 'credentials.json') : '');
const waitSeconds = Number(process.env.SMOKE_CODE_WAIT_SECONDS || 300);
const runId = new Date().toISOString().replace(/[-:tTZ.]/g, '').slice(0, 14);
const username = process.env.SMOKE_USERNAME || `smoke${runId}`.slice(0, 24);
const displayName = `Smoke ${runId.slice(-4)}`;
const password = process.env.SMOKE_PASSWORD || `Smoke-${runId}-portico`;
// Security asked for the reserved .invalid domain so no real mail can leave the deployment.
const contactAddress = process.env.SMOKE_CONTACT || `${username}@m0.getportico.invalid`;

mkdirSync(out, {recursive: true});
const playwright = await loadPlaywright();
const report = createReport({surface: 'hosted', origin, checkpoint, out});

function writeProtected(path, value) {
  mkdirSync(join(path, '..'), {recursive: true});
  writeFileSync(path, JSON.stringify(value, null, 2) + '\n', {mode: 0o600});
  try { chmodSync(path, 0o600); } catch {}
}

/** Waits, silently, for Security to write the real mail code into the protected file. */
async function awaitCode() {
  const deadline = Date.now() + waitSeconds * 1000;
  while (Date.now() < deadline) {
    if (existsSync(registrationFile)) {
      try {
        const record = JSON.parse(readFileSync(registrationFile, 'utf8'));
        const code = String(record.code || record.verificationCode || '').trim();
        if (code) return code;
      } catch { /* the writer may be mid-write; try again */ }
    }
    await new Promise(resolve => setTimeout(resolve, 5000));
  }
  return '';
}

async function main() {
  const {browser, context, page, events} = await launch(playwright, {viewport: {width: 1440, height: 900}});
  const evidence = new Evidence(out);
  let verified = false;

  try {
    await report.check('hosted-reachable', async () => {
      const response = await page.goto(origin + '/sign-in', {waitUntil: 'domcontentloaded', timeout: 45000});
      await settle(page, {quiet: 1500});
      const state = await surfaceState(page);
      assert(response && response.status() < 400, `Hosted answered ${response ? response.status() : 'no response'}`);
      assert(state.textLength > 40, `Hosted rendered a blank surface: ${JSON.stringify(state)}`);
      return {detail: `landed on ${new URL(state.url).pathname}`, evidence: {screenshot: await evidence.shot(page, 'hosted-entry')}};
    });

    let registrationId = '';

    await report.check('hosted-create', async () => {
      assert(registrationFile, 'SMOKE_REGISTRATION_FILE is not set; refusing to register an account nobody is reading a code for');
      const response = await context.request.post(origin + '/v1/accounts', {
        headers: {Origin: origin, 'Content-Type': 'application/json'},
        data: {username, displayName, password, contactAddress, sessionMode: 'browser', requestId: randomUUID()},
        timeout: 30000,
      });
      const body = await response.text();
      if (response.status() === 404 || response.status() === 501) {
        throw blocked(`the live deployment has no registration endpoint yet (HTTP ${response.status()}); Release's fresh schema is pending`);
      }
      assert(response.status() === 202 || response.status() === 201,
        `POST /v1/accounts answered ${response.status()}: ${body.slice(0, 300)}`);
      let json = {};
      try { json = JSON.parse(body); } catch { /* an empty body still means accepted */ }
      registrationId = json.registrationId || json.id || json.registration?.id || '';
      assert(registrationId, `the registration response carried no id: ${body.slice(0, 300)}`);
      const record = {registrationId, username, password, contactAddress, origin, createdAt: new Date().toISOString()};
      // The code file may be replaced wholesale by Security's reader; the account record may not.
      writeProtected(credentialsFile, {...record, note: 'account record, never overwritten by the code reader'});
      writeProtected(registrationFile, {...record, note: 'Security writes the real mail code here as {"code":"..."}'});
      return {detail: `registration ${registrationId.slice(0, 8)}… accepted (HTTP ${response.status()}); code file and credential record written`,
        evidence: {registrationFile, credentialsFile}};
    });

    await report.check('hosted-verify', async () => {
      assert(registrationId, 'no registration was created, so there is nothing to verify');
      const status = await context.request.get(origin + '/v1/identity/current', {timeout: 20000}).then(r => r.status()).catch(() => 0);
      assert(status === 200, `the registration cookie did not carry into /v1/identity/current (HTTP ${status})`);
      const code = await awaitCode();
      if (!code) {
        throw blocked(`no verification code appeared in the protected file within ${waitSeconds}s; the real mail code has to be written there`);
      }
      const response = await context.request.post(`${origin}/v1/registrations/${registrationId}/verify`, {
        headers: {Origin: origin, 'Content-Type': 'application/json'},
        data: {code, username, displayName},
        timeout: 30000,
      });
      const body = await response.text();
      assert(response.status() === 200 || response.status() === 201,
        `verification answered ${response.status()}: ${body.slice(0, 300)}`);
      verified = true;
      return {detail: `account verified and signed in (HTTP ${response.status()})`,
        evidence: {screenshot: await evidence.shot(page, 'hosted-verified')}};
    });

    await report.check('hosted-manage', async () => {
      if (!verified) throw blocked('no verified account, so account management is not reachable (see hosted-verify)');
      await page.goto(origin + '/account', {waitUntil: 'domcontentloaded'});
      await settle(page, {quiet: 2500});
      const text = await bodyText(page);
      assert(text.trim().length > 80, `account screen rendered almost no text: ${JSON.stringify(text.slice(0, 160))}`);
      const sections = ['Profile', 'Password', 'Devices', 'Sessions', 'Servers', 'Notifications']
        .filter(name => new RegExp(name, 'i').test(text));
      assert(sections.length >= 3, `account screen shows only ${sections.join(', ') || 'none'}; expected profile, password, devices, sessions and servers`);
      // Change the display name in the drawn control and prove it persisted.
      const rename = process.env.SMOKE_RENAME_TO || `${displayName} B`;
      let renamed = 'name control not exercised';
      const nameField = await findAny(page, [{css: 'input[autocomplete=name]'}, {role: 'textbox', name: /name/i}], {timeout: 3000});
      if (!nameField) {
        const shot = await evidence.shot(page, 'hosted-manage');
        throw blocked(`the account screen draws no display-name field; text was: ${text.replace(/\n/g, ' ').slice(0, 220)}`,
          {screenshot: shot});
      }
      if (nameField) {
        await nameField.fill(rename, {timeout: 8000}).catch(() => {});
        await clickAny(page, [{role: 'button', name: /^Save/}, {css: 'button[type=submit]'}], {label: 'Save name'}).catch(() => {});
        await page.waitForTimeout(2500);
        await page.reload({waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 2000});
        renamed = (await bodyText(page)).includes(rename) ? 'display name saved and persisted' : 'display name did not persist';
        assert(renamed.includes('persisted'), renamed);
      }
      return {detail: `sections: ${sections.join(', ')}; ${renamed}`,
        evidence: {screenshot: await evidence.shot(page, 'hosted-account'), sections}};
    });

    await report.check('hosted-signin', async () => {
      if (!verified) throw blocked('no verified account to sign in with (see hosted-verify)');
      await context.clearCookies();
      await page.goto(origin + '/sign-in', {waitUntil: 'domcontentloaded'});
      await settle(page, {quiet: 1500});
      const user = await findAny(page, [{css: 'input[autocomplete=username]'}, {css: 'input[type=text]'}, {role: 'textbox'}]);
      const pass = await findAny(page, [{css: 'input[type=password]'}]);
      assert(user && pass, 'the sign-in form is missing a username or password field');
      await user.fill(username, {timeout: 10000});
      await pass.fill(password, {timeout: 10000});
      await clickAny(page, [{role: 'button', name: /^(Continue|Sign in)/}, {css: 'button[type=submit]'}], {label: 'Sign in'});
      await page.waitForTimeout(4500);
      await settle(page, {quiet: 1500});
      const state = await surfaceState(page);
      assert(!/\/sign-in/.test(state.url), `sign-in did not leave the form (at ${state.url}): ${state.textHead.slice(0, 140)}`);
      assert(!/\/v3\/signin/.test(state.url), `sign-in bounced to the identity provider at ${new URL(state.url).pathname} instead of opening the account; the just-verified account could not sign in`);
      return {detail: `signed in through the browser form; now at ${new URL(state.url).pathname}`,
        evidence: {screenshot: await evidence.shot(page, 'hosted-signed-in')}};
    });

    await report.blocked('hosted-server-link',
      'BLOCKED-ON Security: complete initial Hosted server setup/link and protected journal are not implemented on main');
  } finally {
    await context.close();
    await browser.close();
  }

  const result = report.write();
  console.log(`\n${result.verdict.toUpperCase()} ${result.counts.pass} pass / ${result.counts.fail} fail / ${result.counts.blocked} blocked`);
  console.log(`page errors ${events.pageErrors.length}, console errors ${events.consoleErrors.length}`);
  process.exit(result.verdict === 'fail' ? 1 : 0);
}

main().catch(error => {
  console.error('hosted smoke crashed: ' + String(error && error.stack ? error.stack : error).slice(0, 1500));
  process.exit(2);
});
