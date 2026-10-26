import assert from 'node:assert/strict';

// Focused source only: the caller supplies and owns the browser/origin.
export async function checkBoundCardHooks(page, origin) {
  await page.goto(`${origin}/dev/kit?family=cards&case=reference&capture=true`, {waitUntil: 'networkidle'});
  await page.evaluate(async () => {
    const windowAdd = window.addEventListener, documentAdd = document.addEventListener, captured = [];
    window.addEventListener = function(type, fn, options) {if (['blur', 'scroll', 'keydown', 'keyup'].includes(type) && typeof fn === 'function') captured.push({type, fn}); return windowAdd.call(this, type, fn, options);};
    document.addEventListener = function(type, fn, options) {if (type === 'visibilitychange' && typeof fn === 'function') captured.push({type, fn}); return documentAdd.call(this, type, fn, options);};
    const m = await import('/test/fixtures/card-bound-hooks.tsx'); window.disposeCardBoundHooks = m.mountCardBoundHooks();
    window.cardCapturedListeners = captured; window.restoreCardListeners = () => {window.addEventListener = windowAdd; document.addEventListener = documentAdd;};
  });
  try {
    await page.evaluate(() => {
      const h = window.cardBoundHooks; h.capture();
      const timer = window.setTimeout, raf = window.requestAnimationFrame;
      const callbacks = {timer: [], raf: []};
      window.setTimeout = (fn, delay, ...args) => {if (delay === 500 && typeof fn === 'function') callbacks.timer.push(() => fn(...args)); return timer(fn, delay, ...args);};
      window.requestAnimationFrame = fn => {callbacks.raf.push(fn); return raf(fn);};
      h.current().down(); h.current().move();
      window.setTimeout = timer; window.requestAnimationFrame = raf; window.cardOldScheduled = callbacks;
      h.retire();
      for (const fn of callbacks.timer) fn(); for (const fn of callbacks.raf) fn(performance.now());
    });
    const immediate = await page.evaluate(() => {window.restoreCardListeners(); window.cardOldListeners = [...window.cardCapturedListeners]; return window.cardBoundHooks.observations();});
    assert.equal(immediate.preview.at(-1), false, 'receipt retirement clears owned preview synchronously');
    await page.waitForTimeout(550); assert.equal((await page.evaluate(() => window.cardBoundHooks.observations())).toggles, 0);
    await page.evaluate(() => {const h = window.cardBoundHooks; h.revive(); h.capture(); h.current().down(); h.current().move();
      for (const fn of window.cardOldScheduled.timer) fn(); for (const fn of window.cardOldScheduled.raf) fn(performance.now());
      for (const {type, fn} of window.cardOldListeners) fn(type.startsWith('key') ? new KeyboardEvent(type, {key: 'Shift'}) : new Event(type));});
    const first = await page.evaluate(() => window.cardBoundHooks.observations());
    assert.equal(first.preview.at(-1), true); assert.ok(first.publications > 0);
    await page.waitForTimeout(550);
    const published = await page.evaluate(() => window.cardBoundHooks.observations());
    assert.equal(published.toggles, 1, 'volatile descriptor publication preserves original hold');
    assert.equal(published.preview.at(-1), true, 'publication does not clear live preview');
    assert.equal(await page.evaluate(() => window.cardBoundHooks.current().consume()), true, 'same receipt owns fired hold suppression');
    await page.evaluate(() => window.cardBoundHooks.retire());
    const retired = await page.evaluate(() => {const h = window.cardBoundHooks, before = h.observations(); h.old().move(); h.old().key(); h.old().down(); return {before, after: h.observations(), consumed: h.old().consume()};});
    assert.deepEqual(retired.after, retired.before); assert.equal(retired.consumed, false); assert.equal(retired.before.preview.at(-1), false);
    await page.evaluate(() => {const h = window.cardBoundHooks; h.revive(); h.current().move();});
    await page.evaluate(() => window.dispatchEvent(new KeyboardEvent('keyup', {key: 'Shift'})));
    assert.equal((await page.evaluate(() => window.cardBoundHooks.observations())).preview.at(-1), false);
    await page.evaluate(() => {const h = window.cardBoundHooks; h.missing(true); h.current().down(); h.current().move();});
    const missing = await page.evaluate(() => window.cardBoundHooks.observations());
    await page.waitForTimeout(550); assert.deepEqual(await page.evaluate(() => window.cardBoundHooks.observations()), missing);
  } finally {await page.evaluate(() => {window.restoreCardListeners?.(); window.disposeCardBoundHooks?.();});}
}
