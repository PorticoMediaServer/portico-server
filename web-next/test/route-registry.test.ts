import test from 'node:test';
import assert from 'node:assert/strict';
import {createWebScreenRegistry, defineWebScreen, webScreenIdForRoute, webScreenPaths} from '../src/route-registry.ts';
const Screen = () => null;
test('every existing destination has a stable screen id, with no prefix/opaque-id inference', () => {
  assert.equal(Object.keys(webScreenPaths).length, 19);
  for (const [id, path] of Object.entries(webScreenPaths)) assert.equal(webScreenIdForRoute(path), id);
  assert.equal(webScreenIdForRoute('/library/some-id'), undefined);
  assert.equal(webScreenIdForRoute('/settings/extra'), undefined);
});
test('screen modules register independently; missing components remain visible to diagnostics', () => {
  const registry = createWebScreenRegistry({'./screens/settings/settings.route.tsx': {default: defineWebScreen({id: 'settings', component: Screen})}, './screens/accounts/entry.route.tsx': {default: [defineWebScreen({id: 'signIn', component: Screen}), defineWebScreen({id: 'profiles', component: Screen})]}});
  assert.equal(registry.get('settings')?.component, Screen);
  assert.equal(registry.get('player'), undefined);
  assert.equal(registry.chrome('settings'), 'shell');
  assert.equal(registry.chrome('signIn'), 'standalone');
  assert.equal(registry.chrome('player'), 'player');
  assert.ok(registry.missing().includes('player'));
  assert.ok(!registry.missing().includes('profiles'));
});
test('a second owner cannot silently overwrite a route or install a second media host', () => {
  assert.throws(() => createWebScreenRegistry({'a': {default: {id: 'home', component: Screen}}, 'b': {default: {id: 'home', component: Screen}}}), /duplicate-web-screen:home/);
  assert.throws(() => defineWebScreen({id: 'home', component: Screen, playbackHost: Screen}), /playback-host/);
  const registry = createWebScreenRegistry({'player': {default: {id: 'player', component: Screen, playbackHost: Screen}}});
  assert.equal(registry.get('player')?.playbackHost, Screen);
});
