import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const noop = () => ({});
test('A80: deletion is confirmed by the account email (username without one), case-insensitively', async () => {
  const {deletionConfirmationTarget, confirmationMatches} = await componentModule(new URL('../src/screens/account/Security.tsx', import.meta.url), new Proxy({react: {default: {}, useCallback: (f: unknown) => f, useEffect: noop, useMemo: noop, useState: (v: unknown) => [v, noop]}, '@i18n': {defaultI18n: {t: (k: string) => k}}}, {get: (t: any, k: string) => t[k] ?? new Proxy({}, {get: () => noop})})) as typeof import('../src/screens/account/Security.tsx');
  assert.equal(deletionConfirmationTarget('sam@example.com', 'sam'), 'sam@example.com');
  assert.equal(deletionConfirmationTarget('', 'sam'), 'sam');
  assert.equal(confirmationMatches('  SAM@example.COM ', 'sam@example.com'), true);
  assert.equal(confirmationMatches('sam@example.org', 'sam@example.com'), false);
  assert.equal(confirmationMatches('', ''), false);
});
