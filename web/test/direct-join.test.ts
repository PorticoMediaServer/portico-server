import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const stub = () => ({});
async function load() {
  return componentModule(new URL('../src/screens/auth/DirectJoin.tsx', import.meta.url), {
    react: {default: {}, useEffect: stub, useState: (v: unknown) => [v, stub]}, '@tanstack/react-router': {useNavigate: stub},
    '@core/server-connections.ts': {}, '@core/known-servers.ts': {knownServerAddress: stub}, '@core/password-strength.ts': {passwordStrength: stub},
    '../../bridge/server-connections': {}, '../../app/session': {}, '../../app/i18n': {}, '../../app/sign-in-errors': {}, '../../ui': {}, './AuthFrame': {}, './AccountForm': {}, './Auth.module.css': {default: {}},
  }) as Promise<typeof import('../src/screens/auth/DirectJoin.tsx')>;
}

test('ONB-08: the accept outcome maps to a next step, never raw text', async () => {
  const {joinErrorId} = await load();
  assert.equal(joinErrorId({status: 410}), 'web.directJoin.error.expired');
  assert.equal(joinErrorId({status: 409}), 'web.directJoin.error.taken');
  assert.equal(joinErrorId({status: 400}), 'web.directJoin.error.invalid');
  assert.equal(joinErrorId({status: 429}), 'web.directJoin.error.tooMany');
  assert.equal(joinErrorId(new TypeError('fetch failed')), undefined);
});

test('ONB-08: code and username follow the server rules', async () => {
  const {validInvitationCode, validMemberUsername} = await load();
  assert.equal(validInvitationCode('x'.repeat(31)), false);
  assert.equal(validInvitationCode('x'.repeat(32)), true);
  assert.equal(validInvitationCode('x'.repeat(129)), false);
  assert.equal(validInvitationCode('abc def'.padEnd(40, 'x')), false);
  assert.equal(validMemberUsername('al'), false);
  assert.equal(validMemberUsername('alex'), true);
  assert.equal(validMemberUsername('alex@home'), false);
  assert.equal(validMemberUsername('alex smith'), false);
});
