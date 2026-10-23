import assert from 'node:assert/strict';

/** Real shell/kit painting; the native frame observation is an explicit synthetic boundary. */
export async function checkNativeMediaSurface(page, origin, onCase = async () => {}) {
  await page.setViewportSize({width: 1440, height: 900});
  await page.goto(`${origin}/test/native-media-surface.html`, {waitUntil: 'networkidle'});
  await page.locator('[data-media-surface]').waitFor({state: 'visible'});
  await page.waitForFunction(() => !document.getAnimations().some(animation => animation.playState === 'running' && animation.effect?.getComputedTiming().endTime !== Infinity));
  await page.evaluate(async () => {await document.fonts.ready; window.nativeSurfaceFixture.video = document.querySelector('video');});
  const inspect = () => page.evaluate(() => {
    const surface = document.querySelector('[data-media-surface]'), picture = document.querySelector('[data-media-picture]'), slot = document.querySelector('[data-media-slot="expanded"]'), video = document.querySelector('video');
    const frame = surface.getBoundingClientRect();
    return {active: surface.dataset.nativeVideoActive === 'true', hidden: getComputedStyle(surface).visibility === 'hidden', finished: surface.hasAttribute('data-finished'),
      picture: getComputedStyle(picture).backgroundColor, videoOpacity: getComputedStyle(video).opacity, slot: getComputedStyle(slot).backgroundColor, slotActive: slot.dataset.nativeVideoActive === 'true', slotHidden: slot.hidden,
      html: getComputedStyle(document.documentElement).backgroundColor, content: getComputedStyle(document.querySelector('main')).backgroundColor,
      frame: {x: frame.x, y: frame.y, width: frame.width, height: frame.height},
      videoCount: document.querySelectorAll('video').length, sameVideo: video === window.nativeSurfaceFixture.video, proof: window.nativeSurfaceFixture.proof};
  });
  const set = async values => {await page.evaluate(values => window.nativeSurfaceFixture.set(values), values); await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));};
  const opaque = value => value !== 'rgba(0, 0, 0, 0)' && value !== 'transparent';
  const baseline = await inspect();
  assert.equal(baseline.active, false); assert.ok(opaque(baseline.picture)); assert.ok(opaque(baseline.slot)); assert.ok(baseline.slotHidden); assert.equal(baseline.videoOpacity, '1');
  await onCase('normal-web', baseline);
  await set({native: true});
  const native = await inspect();
  assert.equal(native.active, true); assert.equal(native.picture, 'rgba(0, 0, 0, 0)'); assert.equal(native.slotActive, false);
  assert.equal(native.videoOpacity, '0', 'empty browser video/UA controls must not cover the native frame');
  assert.deepEqual(native.frame, baseline.frame, 'the native paint flag must not change the media rectangle');
  assert.equal(native.html, baseline.html); assert.equal(native.content, baseline.content);
  await onCase('native-docked', native);
  await set({expanded: true});
  await page.waitForFunction(() => document.querySelector('[data-media-surface]').getAnimations().length === 0);
  const expanded = await inspect();
  assert.equal(expanded.active, true); assert.equal(expanded.slotActive, true); assert.equal(expanded.slotHidden, false); assert.equal(expanded.slot, 'rgba(0, 0, 0, 0)');
  assert.ok(Math.abs(expanded.frame.width - 1440) < 1); assert.ok(Math.abs(expanded.frame.height - 900) < 1);
  assert.ok(expanded.sameVideo); assert.equal(expanded.proof.mounts, 1); assert.equal(expanded.proof.unmounts, 0);
  await onCase('native-expanded', expanded);
  await page.getByRole('button', {name: 'Dialog', exact: true}).click();
  await page.getByRole('dialog', {name: 'Synthetic overlay'}).waitFor();
  assert.equal(await page.locator('[role="dialog"]').evaluate(dialog => !!dialog.closest('[data-native-underlay-branch], [data-shell-navigation], main[data-route-content]')), false, 'the higher dialog is portalled outside lower paint branches');
  await onCase('native-dialog', await inspect());
  await page.getByRole('button', {name: 'Close test dialog'}).click();
  await page.getByRole('dialog', {name: 'Synthetic overlay'}).waitFor({state: 'detached'});
  await set({finished: true});
  const finished = await inspect();
  assert.equal(finished.active, false); assert.equal(finished.slotActive, false); assert.ok(opaque(finished.picture)); assert.ok(opaque(finished.slot));
  assert.equal(finished.videoOpacity, '1');
  await onCase('native-finished', finished);
  await set({finished: false, hidden: true});
  const hidden = await inspect();
  assert.equal(hidden.hidden, true); assert.equal(hidden.active, false); assert.equal(hidden.slotActive, false); assert.equal(hidden.slotHidden, true);
  await onCase('native-hidden', hidden);
  for (const state of [{hidden: false, audio: true, expanded: false}, {audio: false, remote: true}]) {
    await set(state); const rejected = await inspect(); assert.equal(rejected.active, false); assert.equal(rejected.hidden, true); assert.equal(rejected.slotActive, false);
  }
  await set({audio: false, remote: false, expanded: true, native: true});
  await page.waitForFunction(() => document.querySelector('[data-media-surface]').getAnimations().length === 0);
  await set({expanded: false});
  await page.waitForFunction(() => document.querySelector('[data-media-surface]').getAnimations().length > 0);
  const transition = await inspect();
  assert.equal(transition.active, true); assert.ok(transition.sameVideo); assert.equal(transition.videoCount, 1);
  await set({native: false});
  assert.equal((await inspect()).active, false, 'native frame loss during a morph restores web paint immediately');
  await page.waitForFunction(() => document.querySelector('[data-media-surface]').getAnimations().length === 0);
  const restored = await inspect();
  assert.ok(opaque(restored.picture)); assert.deepEqual(restored.frame, baseline.frame); assert.ok(restored.sameVideo); assert.equal(restored.proof.mounts, 1); assert.equal(restored.proof.unmounts, 0);
  assert.equal(restored.html, baseline.html); assert.equal(restored.content, baseline.content); assert.deepEqual(restored.proof.errors, []);
  await onCase('web-restored-after-transition', restored);
  return {cases: 7, transition, limitation: 'Synthetic frame observation. Does not establish native compositor holes, native playback, native controls, or HDR output.'};
}
