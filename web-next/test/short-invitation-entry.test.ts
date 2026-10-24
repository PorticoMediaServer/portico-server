import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {transformTsx} from './transform-tsx.mjs';

registerHooks({load(url, context, next) {
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});
const {shortInvitationCodeDisplay, shortInvitationCodePlaceholder, shortInvitationCodeValue, shortInvitationFormErrorKind} = await import('../src/screens/accounts/ShortInvitationEntry.tsx');

test('the typed invitation code is one normal sixteen-symbol value shown in four groups', () => {
  assert.equal(shortInvitationCodePlaceholder, 'XXXX XXXX XXXX XXXX');
  for (const raw of ['008J4CT4ANK7F24S', '008j4ct4ank7f24s', '008J-4CT4-ANK7-F24S', '008J 4CT4 ANK7 F24S', '  008J-4CT4-ANK7-F24S  ']) {
    assert.equal(shortInvitationCodeDisplay(raw), '008J 4CT4 ANK7 F24S');
    assert.equal(shortInvitationCodeValue(shortInvitationCodeDisplay(raw)), '008J4CT4ANK7F24S');
  }
  assert.equal(shortInvitationCodeDisplay('008J4CT4ANK7F24SXYZ'), '008J 4CT4 ANK7 F24S');
  assert.equal(shortInvitationCodeDisplay('008J4CT4ANK'), '008J 4CT4 ANK');
});

test('invalid syntax and unsafe preview refusals use only the approved generic sentences', () => {
  const incomplete = {code: 'invitation_incomplete'};
  assert.equal(shortInvitationFormErrorKind('address', incomplete, undefined, false), 'invalid');
  assert.equal(shortInvitationFormErrorKind('address', {code: 'invalid_address'}, undefined, false), 'typed');
  for (const status of [400, 403, 404, 410, 422, 428]) assert.equal(shortInvitationFormErrorKind('password', {status}, undefined, false), 'unavailable');
  for (const status of [429, 500, 503]) assert.equal(shortInvitationFormErrorKind('portico', {status}, undefined, false), 'typed');
  assert.equal(shortInvitationFormErrorKind('password', {status: 404}, {signInMethod: 'local'}, false), undefined);
  assert.equal(shortInvitationFormErrorKind('password', {status: 404}, undefined, true), undefined);
});
