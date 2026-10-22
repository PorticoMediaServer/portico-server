import test from 'node:test';
import assert from 'node:assert/strict';
import {bindShellRouteMotion, finishRouteMotion, routeMotionOwnedByBrowser} from '../src/shell/route-motion.ts';

function browser(userAgent: string) {
  const events = new EventTarget();
  Object.assign(events, {navigator: {userAgent}, navigation: new EventTarget()});
  return events as unknown as Window;
}
test('an iPhone traversal suppresses both router and fallback until the content commit', () => {
  const updates: boolean[] = [], page = browser('iPhone Version/18.0 Mobile Safari/604.1');
  const off = bindShellRouteMotion({update: value => { updates.push(value.defaultViewTransition); }}, page);
  page.dispatchEvent(new Event('popstate'));
  assert.equal(routeMotionOwnedByBrowser(), true);
  assert.deepEqual(updates, [false]);
  finishRouteMotion();
  assert.equal(routeMotionOwnedByBrowser(), false);
  assert.deepEqual(updates, [false, true]);
  off();
});
test('explicit browser visual ownership suppresses motion on any supported platform', () => {
  const updates: boolean[] = [], page = browser('Desktop Firefox');
  const off = bindShellRouteMotion({update: value => { updates.push(value.defaultViewTransition); }}, page);
  (page as Window & {navigation: EventTarget}).navigation.dispatchEvent(Object.assign(new Event('navigate'), {hasUAVisualTransition: true, navigationType: 'traverse'}));
  assert.equal(routeMotionOwnedByBrowser(), true);
  finishRouteMotion();
  assert.deepEqual(updates, [false, true]);
  off();
});
test('normal history on desktop retains View Transitions and cleanup removes all listeners', () => {
  const updates: boolean[] = [], page = browser('Desktop Safari');
  const off = bindShellRouteMotion({update: value => { updates.push(value.defaultViewTransition); }}, page);
  page.dispatchEvent(new Event('popstate'));
  assert.equal(routeMotionOwnedByBrowser(), false);
  assert.deepEqual(updates, []);
  off();
  const after = updates.length;
  page.dispatchEvent(Object.assign(new Event('popstate'), {hasUAVisualTransition: true}));
  assert.equal(updates.length, after);
  assert.equal(routeMotionOwnedByBrowser(), false);
});
