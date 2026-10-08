import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

// MAIL-04 is a form flow, not a pure helper: like the gate-3 presenter tests,
// these pin the form's source — the request shape and the absence of the
// token-paste path — while the /reset#token= screen itself is unchanged.
const src = readFileSync(new URL('../src/screens/auth/AccountForm.tsx', import.meta.url), 'utf8');

test('MAIL-04: the recovery request carries the email only, with an empty username', () => {
  assert.match(src, /username: '', contactAddress: contactAddress\.trim\(\)/, 'the server accepts an empty username');
  assert.match(src, /recoveryRequestBody\(address, await ids\.current\.for\('recover', \{address\}\)\)/, 'forgot sends the email-only body');
});

test('MAIL-04: no username field and no recovery-token field remain on the reset path', () => {
  // The link (/reset#token=) carries the token straight to "Choose a new password".
  assert.doesNotMatch(src, /Recovery token/, 'the paste-a-token field is gone from the normal path');
  assert.doesNotMatch(src, /recovery\/complete/, 'this form no longer completes a recovery itself');
  assert.doesNotMatch(src, /mode === 'reset'|go\('reset'\)/, 'the token-paste mode is gone');
  const forgot = src.slice(src.indexOf("if (mode === 'forgot')"), src.indexOf("if (mode === 'sent')"));
  assert.ok(forgot.length > 100, 'the forgot branch exists');
  assert.match(forgot, /label=\{t\('auth\.email'\)\}/, 'forgot asks for the email');
  assert.doesNotMatch(forgot, /label="Username"/, 'forgot asks for email only');
});
