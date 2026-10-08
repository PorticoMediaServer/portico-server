import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/operation-ids.ts', import.meta.url), {
  react: {default: {}},
}) as Promise<any>;

test('BE-API-10: a retried submission reuses its operation id; an edited one mints a new id', async () => {
  const {createOperationIds} = await load();
  let n = 0;
  const ids = createOperationIds(() => `op-${++n}`);
  const first = ids.forPayload(JSON.stringify({revision: 3, name: 'Night'}));
  assert.equal(ids.forPayload(JSON.stringify({revision: 3, name: 'Night'})), first, 'a retry after a network error reuses the id');
  const edited = ids.forPayload(JSON.stringify({revision: 3, name: 'Morning'}));
  assert.notEqual(edited, first, 'changed input starts a new operation');
  ids.release();
  const afterRelease = ids.forPayload(JSON.stringify({revision: 3, name: 'Morning'}));
  assert.notEqual(afterRelease, edited, 'a completed operation releases its id');
});

test('the default id is 48 lowercase hex, the shape the Live TV routes require (a UUID was refused)', async () => {
  const {createOperationIds, operationId} = await load();
  for (const id of [operationId(), createOperationIds().forPayload('{}')]) {
    assert.match(id, /^[0-9a-f]{48}$/);
  }
  assert.notEqual(operationId(), operationId());
});
