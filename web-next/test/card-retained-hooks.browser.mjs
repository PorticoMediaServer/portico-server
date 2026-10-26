import assert from 'node:assert/strict';

// Source regression only. The caller owns a browser, origin and cleanup; never self-starts a job.
export async function checkCardRetainedHooks(page, origin) {
  await page.goto(`${origin}/dev/kit?family=cards&case=reference&capture=true`, {waitUntil: 'networkidle'});
  await page.evaluate(async () => {const {mountCardRetainedHarness} = await import('/test/fixtures/card-retained-hooks.tsx'); window.disposeCardRetained = mountCardRetainedHarness();});
  try {
    await page.evaluate(() => {const h = window.cardRetainedHarness; h.capture(); h.old().enter(); h.old().down();});
    await page.evaluate(() => window.cardRetainedHarness.setRetained(true));
    const withdrawn = await page.evaluate(() => {const h = window.cardRetainedHarness; const before = h.observations(); h.old().move(); h.old().key(); h.old().leave(); h.old().down(); return {before, after: h.observations(), consumed: h.old().consume()};});
    assert.deepEqual(withdrawn.after, withdrawn.before); assert.equal(withdrawn.consumed, false);
    await page.waitForTimeout(550);
    assert.equal((await page.evaluate(() => window.cardRetainedHarness.observations())).toggles, 0);
    await page.evaluate(() => {const h = window.cardRetainedHarness; h.setRetained(false); h.current().move();});
    await page.evaluate(() => window.dispatchEvent(new KeyboardEvent('keyup', {key: 'Shift'})));
    assert.equal((await page.evaluate(() => window.cardRetainedHarness.observations())).preview.at(-1), false, 'fresh mouse move re-admits the window Shift listener');
    await page.evaluate(() => {const h = window.cardRetainedHarness; h.current().down(); h.old().leave(); h.old().down();});
    await page.waitForTimeout(550);
    const revived = await page.evaluate(() => {const h = window.cardRetainedHarness; return {state: h.observations(), oldConsumed: h.old().consume(), currentConsumed: h.current().consume()};});
    assert.equal(revived.state.toggles, 1); assert.equal(revived.oldConsumed, false); assert.equal(revived.currentConsumed, true);
    await page.evaluate(() => {const h = window.cardRetainedHarness; h.capture(); h.setDisabled(true); h.old().key(); h.old().down();});
    const disabled = await page.evaluate(() => window.cardRetainedHarness.observations());
    await page.waitForTimeout(550); assert.deepEqual(await page.evaluate(() => window.cardRetainedHarness.observations()), disabled);
  } finally {await page.evaluate(() => window.disposeCardRetained?.());}
}
