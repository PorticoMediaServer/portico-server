import assert from 'node:assert/strict';

/** Real controlled auth12 inputs; never an API or a production credential fixture. */
export async function checkInvitationForm(page, origin) {
  const requests = [], errors = [];
  page.on('request', request => {if (new URL(request.url()).pathname.startsWith('/v1/')) requests.push(request.url());});
  page.on('pageerror', error => errors.push(error.message));
  await page.setViewportSize({width: 1440, height: 960});
  await page.goto(`${origin}/src/dev/entries/accounts.html?frame=12`, {waitUntil: 'networkidle'});
  await page.evaluate(() => document.fonts.ready);
  const action = () => page.evaluate(() => JSON.parse(document.documentElement.dataset.accountAction));
  assert.equal(await page.getByRole('heading', {name: 'Portico demo', exact: true}).count(), 1);
  assert.equal(await page.getByText('Good', {exact: true}).count(), 1);
  assert.equal(await page.getByRole('button', {name: /Show password|Hide password/}).count(), 0);
  const name = page.getByLabel('Your name Optional', {exact: true}), username = page.getByLabel('Username', {exact: true});
  const password = page.getByLabel('Password', {exact: true}), confirm = page.getByLabel('Confirm password', {exact: true});
  const submit = page.getByRole('button', {name: 'Join and sign in', exact: true});
  assert.equal(await username.getAttribute('autocomplete'), 'username');
  for (const field of [password, confirm]) {assert.equal(await field.getAttribute('autocomplete'), 'new-password'); assert.equal(await field.getAttribute('type'), 'password');}
  await name.fill(''); await username.fill('new_person'); await password.fill('password'); await confirm.fill('password');
  assert.equal(await submit.isEnabled(), true, 'advisory weakness must not block a valid password');
  await confirm.press('Enter'); assert.deepEqual(await action(), {kind: 'join-local', value: {username: 'new_person', password: 'password', name: ''}});
  await password.fill('short'); await confirm.fill('short'); assert.equal(await submit.isDisabled(), true);
  assert.equal(await page.getByText('Too short', {exact: true}).count(), 1);
  await password.fill('long-enough-password'); await confirm.fill('different'); await confirm.blur();
  assert.equal(await submit.isDisabled(), true); assert.equal(await confirm.getAttribute('aria-invalid'), 'true');
  assert.equal(await page.getByText('The passwords don’t match.', {exact: true}).count(), 1);
  assert.equal(await name.inputValue(), ''); assert.equal(await username.inputValue(), 'new_person');
  await password.fill('💠'.repeat(19)); await confirm.fill('💠'.repeat(19)); assert.equal(await submit.isDisabled(), true, 'bcrypt byte limit is enforced without weakening the server rule');
  await page.getByRole('button', {name: 'Back to Portico', exact: true}).click(); assert.deepEqual(await action(), {kind: 'back'});
  await page.goto(`${origin}/src/dev/entries/accounts.html?frame=12&error=taken`, {waitUntil: 'networkidle'});
  await page.getByRole('button', {name: 'Join and sign in', exact: true}).click();
  assert.equal(await page.getByLabel('Username', {exact: true}).inputValue(), 'nana_r');
  assert.equal(await page.getByLabel('Your name Optional', {exact: true}).inputValue(), 'Nana');
  assert.equal(await page.getByLabel('Password', {exact: true}).inputValue(), 'Guest-pass1!');
  assert.equal(await page.getByLabel('Confirm password', {exact: true}).inputValue(), 'Guest-pass1!');
  assert.equal(await page.getByLabel('Username', {exact: true}).getAttribute('aria-invalid'), 'true');
  assert.equal(await page.getByText('That username is taken. Choose another.', {exact: true}).count(), 1);
  await page.getByLabel('Username', {exact: true}).fill('another_name');
  assert.equal(await page.getByText('That username is taken. Choose another.', {exact: true}).count(), 0);
  assert.deepEqual(errors, []); assert.deepEqual(requests, []);
  return {actualForm: true, noReveal: true, passwordManager: true, weakAllowed: true, mismatch: true, byteLimit: true, usernameTaken: true, typedValuesPreserved: true, apiRequests: requests.length, errors};
}
