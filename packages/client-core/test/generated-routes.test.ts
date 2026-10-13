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

test('development and dormant protocols have separate contracts and retain encoded paths',async()=>{
 const optional=await import('../../contracts/src/optional/routes.ts');
 const development=await import('../../contracts/src/development/routes.ts');
 const production=await import('../../contracts/src/routes.ts');
 assert.equal(optional.routes.length,12);
 assert.equal(development.routes.length,2);
 assert.equal(optional.routePath('get_account_profile_deletion',{id:'a/b'}),'/v1/account/profile-deletions/a%2Fb');
 assert.equal(optional.routePath('send_device_command',{id:'a b'}),'/v1/devices/a%20b/commands');
 assert.equal(development.routePath('post_dev_e2e_sign_in'),'/v1/dev/e2e/sign-in');
 for(const route of [...optional.routes,...development.routes]) assert.equal(production.routes.some(p=>p.method===route.method&&p.path===route.path),false);
});
