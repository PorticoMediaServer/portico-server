import test from 'node:test';
import assert from 'node:assert/strict';
import {routeInvitationLink} from '../src/bridge/invitation-location.ts';

test('an invitation link on any page moves to /join with its fragment', () => {
  const urls: string[] = [];
  assert.equal(routeInvitationLink({pathname: '/', hash: '#invite=abc.' + 'x'.repeat(43)}, u => urls.push(u)), true);
  assert.deepEqual(urls, ['/join#invite=abc.' + 'x'.repeat(43)]);
});

test('other fragments and /join itself are left alone', () => {
  const urls: string[] = [];
  assert.equal(routeInvitationLink({pathname: '/join', hash: '#invite=abc.def'}, u => urls.push(u)), false);
  assert.equal(routeInvitationLink({pathname: '/', hash: '#token=abc'}, u => urls.push(u)), false);
  assert.equal(routeInvitationLink({pathname: '/', hash: ''}, u => urls.push(u)), false);
  assert.deepEqual(urls, []);
});
