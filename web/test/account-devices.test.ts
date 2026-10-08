import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const noop = () => ({});
const stub = () =>
  new Proxy(
    {
      react: {default: {}, useCallback: (f: unknown) => f, useEffect: noop, useMemo: noop, useState: (v: unknown) => [v, noop]},
      '@i18n': {defaultI18n: {t: (k: string) => k}},
    },
    {get: (t: any, k: string) => t[k] ?? new Proxy({}, {get: () => noop})},
  );
const accountModule = () => componentModule(new URL('../src/screens/account/Account.tsx', import.meta.url), stub()) as Promise<typeof import('../src/screens/account/Account.tsx')>;

test('WEB-AUTH-02: sign-out-all targets every active family except this browser', async () => {
  const {otherActiveIds} = await accountModule();
  assert.deepEqual(otherActiveIds([]), []);
  assert.deepEqual(
    otherActiveIds([
      {id: 'here', status: 'active', current: true},
      {id: 'phone', status: 'active', current: false},
      {id: 'old', status: 'revoked', current: false},
      {id: 'stale', status: 'expired', current: false},
    ]),
    ['phone'],
  );
});

test('WEB-AUTH-02: sign-out-all walks every page and stops on failure', async () => {
  const {revokeAllOtherSessions} = await accountModule();
  const revoked: string[] = [];
  let err: unknown = null;
  let page = 0;
  const pages = [
    [{id: 'here', status: 'active', current: true}, {id: 'a', status: 'active', current: false}, {id: 'old', status: 'revoked', current: false}],
    [{id: 'b', status: 'active', current: false}],
  ];
  const service = {
    getSnapshot: () => ({phase: 'ready' as const, items: pages[page]!, nextCursor: page < pages.length - 1 ? `c${page}` : '', mutationError: err}),
    revoke: async (id: string) => {
      revoked.push(id);
      for (const rows of pages) {
        const row = rows.find(x => x.id === id);
        if (row) row.status = 'revoked';
      }
    },
    next: async () => { page++; },
  };
  assert.equal(await revokeAllOtherSessions(service), true);
  assert.deepEqual(revoked, ['a', 'b']);

  // A pending mutation error stops the run and reports false.
  err = {code: 'request_failed'};
  assert.equal(await revokeAllOtherSessions(service), false);
});

test('WEB-AUTH-02: sign-out-all reports a failed revoke instead of looping', async () => {
  const {revokeAllOtherSessions} = await accountModule();
  const stuck = {
    getSnapshot: () => ({phase: 'ready' as const, items: [{id: 'x', status: 'active', current: false}], nextCursor: '', mutationError: null}),
    revoke: async () => { throw new Error('stale state'); },
    next: async () => {},
  };
  assert.equal(await revokeAllOtherSessions(stuck), false);
});
