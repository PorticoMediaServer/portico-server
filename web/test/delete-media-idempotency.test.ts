import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const loadDeleteMedia = () => componentModule(new URL('../src/app/delete-media.tsx', import.meta.url), {
  react: {default: {createElement: () => null}, Suspense: 'suspense', createContext: () => ({Provider: 'provider'}), lazy: () => null, useCallback: (f: unknown) => f, useContext: () => null, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
}) as Promise<any>;

test('Media deletion: a retried delete reuses its idempotency key; an edited one mints a new key', async () => {
  const {deletePayloadKey, createDeleteOperationIds} = await loadDeleteMedia();
  assert.equal(typeof deletePayloadKey, 'function');
  assert.equal(typeof createDeleteOperationIds, 'function');
  const scope = {itemIds: ['m1', 'm2'], revision: 7, deleteFiles: true, confirmation: 'DELETE 2'};
  assert.equal(deletePayloadKey(scope), deletePayloadKey({...scope}), 'the same logical delete builds the same key');
  assert.notEqual(deletePayloadKey(scope), deletePayloadKey({...scope, revision: 8}), 'a new server revision is a new operation');
  assert.notEqual(deletePayloadKey(scope), deletePayloadKey({...scope, deleteFiles: false}), 'changing file handling is a new operation');
  assert.notEqual(deletePayloadKey(scope), deletePayloadKey({...scope, confirmation: 'DELETE 3'}), 'changing the confirmation is a new operation');
  let n = 0;
  const ids = createDeleteOperationIds(() => `del-${++n}`);
  const first = ids.forPayload(deletePayloadKey(scope));
  assert.equal(ids.forPayload(deletePayloadKey(scope)), first, 'a retry after a network error reuses the key');
  const edited = ids.forPayload(deletePayloadKey({...scope, deleteFiles: false}));
  assert.notEqual(edited, first, 'changed input starts a new operation');
  ids.release();
  assert.notEqual(ids.forPayload(deletePayloadKey({...scope, deleteFiles: false})), edited, 'a completed delete releases its key');
});
