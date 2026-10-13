import test from 'node:test';
import assert from 'node:assert/strict';
import {checkGeneratedRoutes, pathLiterals} from '../scripts/check-generated-routes.mjs';
import {routePath, encodedRoutePath} from '../../contracts/src/routes.ts';

test('client source admits no new hand-written API path', () => {
 assert.deepEqual(checkGeneratedRoutes(), []);
 assert.equal(pathLiterals(`const path='/v1/new-action';`).length, 1);
 assert.equal(pathLiterals('const path=`/v1/items/${id}/new-action`;').length, 1);
 assert.equal(pathLiterals('const path=`${origin}/v1/new-action`;').length, 1);
 assert.equal(pathLiterals("const path='https://example.test/v1/new-action';").length, 1);
 assert.deepEqual(pathLiterals(`// '/v1/comment'\n/* '/v1/comment' */\nconst label='A library';`), []);
});
test('generated builders encode raw parameters once and preserve existing encodings', () => {
 assert.equal(routePath('get_items_id_detail', {id:'a b'}), '/v1/items/a%20b/detail');
 assert.equal(encodedRoutePath('get_items_id_detail', {id:'a%20b'}), '/v1/items/a%20b/detail');
 assert.throws(() => routePath('get_items_id_detail', {id:''}));
 assert.throws(() => routePath('get_items_id_detail', {id:'a', extra:'b'} as never));
});
