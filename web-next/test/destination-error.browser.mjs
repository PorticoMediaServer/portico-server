import assert from 'node:assert/strict';

/**
 * FX-156 / BLK-WEB-001 destination-error regression source. Authored against the existing gallery
 * harness and the real browser DOM: the production `DestinationErrorBoundary` is mounted around a
 * destination whose fault stays deterministic until the harness releases it, with a shell marker as
 * its sibling. Authored only — not executed.
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
  const retry = page.getByRole('button', {name: /Try again/});
  const home = page.getByRole('button', {name: /Go to Home/});

  try {
    // 1. A deterministic render fault shows the approved failure inside the still-mounted shell.
    await mount();
    await failure.waitFor();
    assert.equal(await page.locator('[data-shell-marker]').count(), 1);
    assert.equal(await page.locator('[data-shell-top]').count(), 1);
    assert.match(await page.locator('body').innerText(), /This page ran into a problem/);
    assert.equal(await retry.count(), 1);
    assert.equal(await home.count(), 1);

    // 2. Try again while the fault persists stays the bounded failure — it neither crashes nor latches.
    await retry.click();
    await failure.waitFor();
    assert.equal(await page.locator('[data-shell-marker]').count(), 1);
    assert.equal(await page.locator('[data-destination-recovered]').count(), 0);

    // 3. Releasing the fault and pressing Try again recovers that destination.
    await page.evaluate(() => window.destinationErrorHarness.release());
    await retry.click();
    await page.locator('[data-destination-recovered]').waitFor();
    assert.equal(await failure.count(), 0);

    // 4. A layout-effect fault takes the same bounded path and recovers after release.
    await page.evaluate(() => {window.destinationErrorHarness.where('effect'); window.destinationErrorHarness.rearm();});
    await failure.waitFor();
    assert.equal(await page.locator('[data-shell-marker]').count(), 1);
    await retry.click();
    await failure.waitFor();
    await page.evaluate(() => window.destinationErrorHarness.release());
    await retry.click();
    await page.locator('[data-destination-recovered]').waitFor();

    // 5. A fresh entry with the identical href unlatches: the fault is released first, the failed
    // destination is asserted still failed before the entry changes, then the new entry recovers.
    await page.evaluate(() => {window.destinationErrorHarness.where('render'); window.destinationErrorHarness.rearm();});
    await failure.waitFor();
    await page.evaluate(() => window.destinationErrorHarness.release());
    await page.waitForTimeout(200);
    assert.equal(await failure.count(), 1, 'release alone must not unlatch the failed destination');
    await page.evaluate(() => window.destinationErrorHarness.entry('entry-b'));
    await page.locator('[data-destination-recovered]').waitFor();
    assert.equal(await failure.count(), 0);

    // 6. A scope replacement with the same entry and href unlatches the same way.
    await page.evaluate(() => window.destinationErrorHarness.rearm());
    await failure.waitFor();
    await page.evaluate(() => window.destinationErrorHarness.release());
    await page.waitForTimeout(200);
    assert.equal(await failure.count(), 1, 'release alone must not unlatch the failed destination');
    await page.evaluate(() => window.destinationErrorHarness.scope('scope-2'));
    await page.locator('[data-destination-recovered]').waitFor();
    assert.equal(await failure.count(), 0);

    // 6b. A new lifetime while the fault continues stays the bounded failure, not a recovery.
    await page.evaluate(() => {window.destinationErrorHarness.rearm(); window.destinationErrorHarness.entry('entry-c');});
    await failure.waitFor();
    assert.equal(await page.locator('[data-destination-recovered]').count(), 0);
    assert.equal(await page.locator('[data-shell-marker]').count(), 1);
    await page.evaluate(() => window.destinationErrorHarness.release());
    await retry.click();
    await page.locator('[data-destination-recovered]').waitFor();

    // 7. Home uses the real router port and lands on '/'.
    await page.evaluate(() => {window.destinationErrorHarness.rearm();});
    await failure.waitFor();
    await home.click();
    await page.waitForFunction(() => location.pathname === '/');
  } finally {
    await page.evaluate(() => {window.disposeDestinationError?.();});
  }

  // 8. Nesting only: a sibling that throws outside the destination boundary is not caught by it.
  // This does NOT exercise the production ShellRoot/router last resort, and registration/validation
  // behavior stays owner UNIMPLEMENTED/UNRUN — no app-level claim is made from this stand-in.
  try {
    await page.evaluate(async () => {
      const {mountShellFailureHarness} = await import('/test/fixtures/destination-error.tsx');
      window.disposeShellSeam = mountShellFailureHarness();
    });
    await page.locator('[data-outer-nesting]').waitFor();
    assert.equal(await page.getByText("Something didn't load", {exact: true}).count(), 0);
    assert.equal(await page.locator('[data-destination-ok]').count(), 0);
  } finally {
    await page.evaluate(() => {window.disposeShellSeam?.();});
  }
}
