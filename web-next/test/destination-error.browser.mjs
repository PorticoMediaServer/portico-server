import assert from 'node:assert/strict';

/**
 * FX-156 / BLK-WEB-001 destination-error regression source. Authored against the existing gallery
 * harness and the real browser DOM: the production `DestinationErrorBoundary` is mounted around a
 * destination that throws, with a shell marker as its sibling, so "inside the still-mounted shell"
 * is proved structurally. Authored only — not executed.
 */
export async function checkDestinationError(page, origin) {
  const mount = async () => {
    await page.evaluate(() => {window.disposeDestinationError?.();});
    await page.goto(`${origin}/dev/kit?family=states&case=shortcuts&capture=true`, {waitUntil: 'networkidle'});
    await page.evaluate(async () => {
      const {mountDestinationErrorHarness} = await import('/test/fixtures/destination-error.tsx');
      window.disposeDestinationError = mountDestinationErrorHarness();
    });
    await page.locator('[data-shell-marker]').waitFor();
  };
  const failure = page.getByText("Something didn't load", {exact: true});
  const shell = page.locator('[data-shell-marker]');
  const top = page.locator('[data-shell-top]');

  try {
    // 1. A destination render exception shows the approved page failure inside the mounted shell.
    await mount();
    await failure.waitFor();
    assert.equal(await shell.count(), 1);
    assert.equal(await top.count(), 1);
    assert.match(await page.locator('body').innerText(), /This page ran into a problem/);
    assert.equal(await page.getByRole('button', {name: /Try again/}).count(), 1);
    assert.equal(await page.getByRole('button', {name: /Go to Home/}).count(), 1);

    // 2. Try again resets only the destination and the recovered component renders.
    await page.getByRole('button', {name: /Try again/}).click();
    await page.locator('[data-destination-recovered]').waitFor();
    assert.equal(await failure.count(), 0);
    assert.equal(await shell.count(), 1);

    // 3. A fresh route entry unlatches: fail again, then change the route key without a retry.
    await page.evaluate(() => {window.destinationErrorHarness.reset(); window.destinationErrorHarness.mode('render'); window.destinationErrorHarness.bump();});
    await failure.waitFor();
    await page.evaluate(() => window.destinationErrorHarness.routeKey('entry-b'));
    await page.locator('[data-destination-recovered]').waitFor();
    assert.equal(await failure.count(), 0);

    // 4. A layout-effect exception takes the same bounded path.
    await page.evaluate(() => {window.destinationErrorHarness.reset(); window.destinationErrorHarness.mode('effect'); window.destinationErrorHarness.bump();});
    await failure.waitFor();
    assert.equal(await shell.count(), 1);
    await page.getByRole('button', {name: /Try again/}).click();
    await page.locator('[data-destination-effect]').waitFor();

    // 5. Home uses the real router port and lands on '/'.
    await page.evaluate(() => {window.destinationErrorHarness.reset(); window.destinationErrorHarness.mode('render'); window.destinationErrorHarness.bump();});
    await failure.waitFor();
    await page.getByRole('button', {name: /Go to Home/}).click();
    await page.waitForFunction(() => location.pathname === '/');
  } finally {
    await page.evaluate(() => {window.disposeDestinationError?.();});
  }

  // 6. A deliberate shell-level failure is not caught by the destination boundary.
  const errors = [];
  const onError = error => errors.push(error.message);
  page.on('pageerror', onError);
  try {
    await page.evaluate(async () => {
      const {mountShellFailureHarness} = await import('/test/fixtures/destination-error.tsx');
      window.disposeShellFailure = mountShellFailureHarness();
    });
    await page.waitForTimeout(500);
    assert.ok(errors.some(message => /shell-fault/.test(message)), 'shell failure must surface as an app-level error');
    assert.equal(await page.getByText("Something didn't load", {exact: true}).count(), 0);
  } finally {
    page.off('pageerror', onError);
    await page.evaluate(() => {window.disposeShellFailure?.();});
  }
}
