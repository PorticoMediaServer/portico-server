import assert from 'node:assert/strict';

/**
 * FX-156 / BLK-WEB-001 destination recovery regression source. The specimen mounts the real router,
 * ShellRoot and Shell chrome with finite fixture data, and installs the destination fault through the
 * actual route's public options update. The caller supplies and owns the browser/origin.
 * Authored only — not executed.
 */
export async function checkDestinationRecovery(page, origin) {
  await page.goto(`${origin}/src/dev/entries/destination-recovery.html`, {waitUntil: 'networkidle'});
  await page.locator('[data-destination-ok]').waitFor();
  const failure = page.getByText("Something didn't load", {exact: true});
  const retry = page.getByRole('button', {name: /Try again/});
  const home = page.getByRole('button', {name: /Go to Home/});
  const shell = () => page.evaluate(() => window.destinationRecovery.shellMarker());
  try {
    // The real shell chrome is mounted around the destination.
    assert.equal(await shell(), true, 'real shell chrome is mounted');

    // A render fault keeps the shell and shows the approved destination failure.
    await page.evaluate(() => {window.destinationRecovery.fault('render'); return window.destinationRecovery.navigate('/search');});
    await failure.waitFor();
    assert.equal(await shell(), true, 'shell chrome survives the destination failure');
    assert.equal(await retry.count(), 1);
    assert.equal(await home.count(), 1);

    // Still-faulted retry stays the bounded failure, not a crash or a latch.
    await retry.click();
    await failure.waitFor();
    assert.equal(await page.locator('[data-destination-ok]').count(), 0);

    // Release, then Try again recovers the destination.
    await page.evaluate(() => window.destinationRecovery.fault('none'));
    await retry.click();
    await page.locator('[data-destination-ok]').waitFor();
    assert.equal(await failure.count(), 0);

    // A layout-effect fault takes the same path.
    await page.evaluate(() => {window.destinationRecovery.fault('effect'); return window.destinationRecovery.navigate('/search');});
    await failure.waitFor();
    assert.equal(await shell(), true);
    await page.evaluate(() => window.destinationRecovery.fault('none'));
    await retry.click();
    await page.locator('[data-destination-ok]').waitFor();

    // A same-href fresh history entry is a new witness: release first, assert still failed, then enter.
    await page.evaluate(() => {window.destinationRecovery.fault('render'); return window.destinationRecovery.navigate('/search');});
    await failure.waitFor();
    await page.evaluate(() => window.destinationRecovery.fault('none'));
    await page.waitForTimeout(150);
    assert.equal(await failure.count(), 1, 'release alone must not unlatch the failed destination');
    await page.evaluate(() => window.destinationRecovery.entry());
    await page.locator('[data-destination-ok]').waitFor();
    assert.equal(await failure.count(), 0);

    // A scope replacement is a new witness too.
    await page.evaluate(() => {window.destinationRecovery.fault('render'); return window.destinationRecovery.navigate('/search');});
    await failure.waitFor();
    await page.evaluate(() => window.destinationRecovery.fault('none'));
    await page.waitForTimeout(150);
    assert.equal(await failure.count(), 1, 'release alone must not unlatch the failed destination');
    await page.evaluate(() => window.destinationRecovery.scope('fixture-scope-2'));
    await page.locator('[data-destination-ok]').waitFor();
    assert.equal(await failure.count(), 0);

    // The real router consumes validateSearch above the outlet: the destination presenter must not
    // appear, and this exact router construction seam is handed back to Web (router.tsx is reserved).
    await page.evaluate(() => {window.destinationRecovery.validate(true); return window.destinationRecovery.navigate('/search?probe=1');});
    await page.waitForTimeout(400);
    assert.equal(await failure.count(), 0, 'validateSearch is consumed above the destination boundary');
    await page.evaluate(() => window.destinationRecovery.validate(false));

    // A ShellRoot throw is a router/app-level failure, never a destination recovery.
    await page.evaluate(() => {window.destinationRecovery.rootFault(true); return window.destinationRecovery.navigate('/search');});
    await page.waitForTimeout(400);
    assert.equal(await failure.count(), 0, 'a ShellRoot throw must not become a destination recovery');
  } finally {
    await page.evaluate(() => {window.destinationRecovery.restore(); window.disposeDestinationRecovery?.();});
  }
}
