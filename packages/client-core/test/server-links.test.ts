import test from 'node:test';
import assert from 'node:assert/strict';
import {isScopedScreenPath, parseServerLink, parseServerLinkPath, scopedPathname, serverLink} from '../src/server-links.ts';

test('O-162 frozen grammar: a server link names its server and a scoped screen, and nothing else', () => {
  assert.deepEqual(parseServerLinkPath('/s/srv_1/item/abc'), {serverId: 'srv_1', pathname: '/item/abc'});
  assert.deepEqual(parseServerLinkPath('/s/srv_1'), {serverId: 'srv_1', pathname: '/'});
  for (const path of ['/s/srv/sign-in', '/s/srv/account', '/s/srv/servers', '/s/srv/setup', '/s/srv/settings', '/s/../item/x', '/item/x']) assert.equal(parseServerLinkPath(path), undefined, path);
  assert.equal(isScopedScreenPath('/read/b'), true);assert.equal(isScopedScreenPath('/device'), false);
  assert.equal(scopedPathname('/library/l', 'srv_1'), '/s/srv_1/library/l');
});

test('a whole shared link: https on the Portico link hosts only, never credentials or a port', () => {
  assert.deepEqual(parseServerLink('https://web.getportico.tv/s/srv_1/item/abc?tab=details#x'), {serverId: 'srv_1', pathname: '/item/abc', search: '?tab=details', hash: '#x'});
  assert.deepEqual(parseServerLink('https://getportico.tv/s/srv_1/live')?.pathname, '/live');
  for (const href of ['http://web.getportico.tv/s/srv_1/item/a', 'https://evil.test/s/srv_1/item/a', 'https://user:pw@web.getportico.tv/s/srv_1/item/a',
    'https://web.getportico.tv:8443/s/srv_1/item/a', 'https://web.getportico.tv/item/a', 'https://web.getportico.tv/s/srv_1/account', 'not a url'])
    assert.equal(parseServerLink(href), undefined, href);
});

test('the builder writes the one canonical link, and refuses what the parser would refuse', () => {
  assert.equal(serverLink('srv_1', '/item/abc', '?tab=details'), 'https://web.getportico.tv/s/srv_1/item/abc?tab=details');
  assert.equal(serverLink('srv_1', '/'), 'https://web.getportico.tv/s/srv_1');
  assert.equal(serverLink('srv_1', '/settings'), undefined);assert.equal(serverLink('../x', '/item/a'), undefined);
  const built = serverLink('srv_1', '/person/p', 'a=1', 'b')!;
  assert.deepEqual(parseServerLink(built), {serverId: 'srv_1', pathname: '/person/p', search: '?a=1', hash: '#b'}, 'round trip');
});
