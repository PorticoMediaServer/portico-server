import test from 'node:test';
import assert from 'node:assert/strict';
import {readdirSync, readFileSync, statSync} from 'node:fs';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';

// C60 / C45: there is no step-up. Account management runs on the signed-in session; only
// email, password and two-step changes ask for the current password (plus a code), in that
// flow's own fields. Nothing in the web app may call a step-up route or method.
const src = fileURLToPath(new URL('../src/', import.meta.url));
function files(dir: string): string[] {
  return readdirSync(dir).flatMap(name => {
    const p = join(dir, name);
    return statSync(p).isDirectory() ? files(p) : /\.(tsx?|mjs)$/.test(name) ? [p] : [];
  });
}

test('no web source calls a step-up (method, route or fixture)', () => {
  const hits: string[] = [];
  for (const file of files(src)) {
    readFileSync(file, 'utf8').split('\n').forEach((line, i) => {
      if (/\bstepUp\b|\/step-up\b|\bstep_up\b|\breauthenticateFor\b/.test(line)) hits.push(`${file.slice(src.length)}:${i + 1}`);
    });
  }
  assert.deepEqual(hits, []);
});

test('credential changes collect the current password in their own dialogs', () => {
  const direct = readFileSync(join(src, 'screens/settings/Profiles.tsx'), 'utf8');
  assert.match(direct, /identity\.changePassword\(current, next/, 'direct password change sends the current password itself');
  const twoStep = readFileSync(join(src, 'screens/settings/Security.tsx'), 'utf8');
  assert.match(twoStep, /step === 'password'/, 'turning on two-step asks for the password in its own step');
  assert.doesNotMatch(twoStep, /saveRestrictions\([^)]*(token|proof)/, 'profile limits save on the session alone');
  const hosted = readFileSync(join(src, 'screens/account/Security.tsx'), 'utf8');
  const proofTasks = hosted.slice(hosted.indexOf('const tasks = {'), hosted.indexOf('} satisfies Record<string, Task>'));
  const purposes = [...proofTasks.matchAll(/purpose: '([a-z_]+)'/g)].map(m => m[1]).sort();
  assert.deepEqual(purposes, ['email_change', 'mfa_disable', 'mfa_enroll', 'password_set_or_change', 'recovery_codes'], 'only email, password and two-step tasks take a password proof');
  // NEW-13: connecting and disconnecting Google or Apple run on the session alone.
  assert.match(hosted, /client\.start\(provider, \{mode: 'link', requestId: [^}]+intent: \{purpose: 'provider_link', target: provider, operationId: [^}]+\}\}, undefined, await api\.token\(\)\)/, 'Connect sends no proof');
  assert.match(hosted, /client\.action\(\{purpose: 'provider_unlink', target: disconnecting\.id, operationId: [^}]+\}, undefined, '', scope/, 'Disconnect sends no proof and no password');
});
