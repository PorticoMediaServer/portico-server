import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from '../transform-tsx.mjs';

registerHooks({load(url, context, next) {
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.svg')) return {format: 'module', source: 'export default "approved-wordmark.svg";', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});
const {LocalInvitationForm, PorticoInvitation, InvitationUnavailable} = await import('../../src/kit/accounts/Invitation.tsx');
const {ServerIdentityFailure} = await import('../../src/kit/accounts/ServerIdentityFailure.tsx');
const noAction = () => {throw new Error('Static rendering cannot dispatch an action');};

test('Local invitation fields keep browser password-manager semantics and no undrawn reveal action', () => {
  const html = renderToStaticMarkup(createElement(LocalInvitationForm, {serverName: 'Actual server', inviter: 'Actual inviter', libraries: ['Real collection'], onJoin: noAction, onBack: noAction}));
  assert.equal((html.match(/type="password"/g) ?? []).length, 2);
  assert.equal((html.match(/autoComplete="new-password"/g) ?? []).length, 2);
  assert.match(html, /name="username"/); assert.match(html, /Actual server/); assert.match(html, /Real collection/);
  assert.doesNotMatch(html, /Show password|Hide password|#i-film|#i-tvlib|#i-category/);
});
test('Portico invitation renders actual account identity and only supplied kind glyphs, never a password form', () => {
  const html = renderToStaticMarkup(createElement(PorticoInvitation, {serverName: 'Actual server', member: true,
    libraries: [{name: 'Ambiguous library name'}, {name: 'Different actual collection', icon: 'i-film'}], account: {username: 'actual_user'}, signedOutContent: null, onJoin: noAction, onDifferentAccount: noAction}));
  assert.match(html, /Join as actual_user/); assert.match(html, /Join Actual server/); assert.match(html, /You’ll join as a member/);
  assert.equal((html.match(/href="#i-film"/g) ?? []).length, 1);
  assert.doesNotMatch(html, /type="password"|#i-tvlib|#i-category|api\//);
});
test('signed-out Portico invitation composes its caller account form without adding another auth model', () => {
  const html = renderToStaticMarkup(createElement(PorticoInvitation, {serverName: 'Actual server', member: false, libraries: [],
    signedOutContent: createElement('form', {'data-real-account-form': true}), onJoin: noAction, onDifferentAccount: noAction}));
  assert.equal((html.match(/<form/g) ?? []).length, 1); assert.match(html, /data-real-account-form/);
  assert.doesNotMatch(html, /Join as|You’ll join as a member|type="password"/);
});
test('expired and account-mismatch states use the exact distinct terminal copy and actions', () => {
  const expired = renderToStaticMarkup(createElement(InvitationUnavailable, {onBack: noAction, onDifferentAccount: noAction}));
  assert.match(expired, /This invitation can’t be used/); assert.match(expired, /Invitations work once and expire after 7 days/); assert.match(expired, /#i-clock/);
  assert.doesNotMatch(expired, /different Portico Account/);
  const other = renderToStaticMarkup(createElement(InvitationUnavailable, {otherAccount: true, onBack: noAction, onDifferentAccount: noAction}));
  assert.match(other, /This invitation is for a different account/); assert.match(other, /Use a different Portico Account/); assert.match(other, /#i-userx/);
  assert.doesNotMatch(other, /Invitations work once|#i-clock/);
});
test('only the changed Local identity has a deliberate Forget action; neither failure offers Trust or a pin confirmation', () => {
  const props = {serverName: 'Actual saved name', onRetry: noAction, onChooseServer: noAction, onForget: async () => noAction()};
  const changed = renderToStaticMarkup(createElement(ServerIdentityFailure, {...props, failure: 'local-changed'}));
  assert.match(changed, /A different server is answering at its address, so nothing was sent/);
  assert.match(changed, /Forget Actual saved name/); assert.match(changed, /#i-shieldx/);
  assert.doesNotMatch(changed, /Trust|Confirm server identity|Identity code|role="alertdialog"/);
  const unconfirmed = renderToStaticMarkup(createElement(ServerIdentityFailure, {...props, failure: 'unconfirmed'}));
  assert.match(unconfirmed, /Nothing was sent to it. Try again, or ask its owner to check the server/);
  assert.match(unconfirmed, /#i-question/); assert.doesNotMatch(unconfirmed, /Forget Actual|Trust|Confirm server identity/);
});
