import test from 'node:test';
import assert from 'node:assert/strict';
import {bindShellRouteMotion, finishRouteMotion, routeMotionOwnedByBrowser} from '../src/shell/route-motion.ts';

/** Minimal actual Window shape the binder uses: navigator identity, popstate and
 * the optional Navigation API event target. Nothing here invents UA acceptance. */
function browser(userAgent: string) {
  const page = new EventTarget();
  const navigation = new EventTarget();
  Object.assign(page, {navigator: {userAgent}, navigation});
  return {page: page as unknown as Window, navigation};
}

function router() {
  const updates: boolean[] = [], rendered: Array<() => void> = [];
  return {
    updates,
    fireRendered: () => {for (const listener of [...rendered]) listener();},
    rendering: () => rendered.length,
    router: {
      state: {matches: [] as readonly unknown[]},
      update: (value: {defaultViewTransition: boolean}) => {updates.push(value.defaultViewTransition);},
      subscribe: (_event: 'onRendered', listener: () => void) => {
        rendered.push(listener);
        return () => {const index = rendered.indexOf(listener); if (index >= 0) rendered.splice(index, 1);};
      }
    }
  };
}

const traversal = (hasUAVisualTransition?: boolean, navigationType = 'traverse') =>
  Object.assign(new Event('navigate'), {hasUAVisualTransition, navigationType});

test('UA-owned traversal suppresses until the actual rendered commit releases it', () => {
  const {page, navigation} = browser('iPhone Version/18.0 Mobile Safari/604.1'), fixture = router();
  const off = bindShellRouteMotion(fixture.router, page);
  page.dispatchEvent(new Event('popstate'));
  assert.equal(routeMotionOwnedByBrowser(), true);
  assert.deepEqual(fixture.updates, [false], 'the UA traversal disables Portico motion before any render');
  assert.equal(fixture.rendering(), 1, 'one captured rendered subscription exists');
  navigation.dispatchEvent(traversal(true, 'push'));
  assert.deepEqual(fixture.updates, [false], 'a second owned navigation does not stack another update');
  fixture.fireRendered();
  assert.equal(routeMotionOwnedByBrowser(), false);
  assert.deepEqual(fixture.updates, [false, true], 'the original render commit releases motion once');
  assert.equal(fixture.rendering(), 1, 'the binder keeps its single subscription for later routes');
  off();
  assert.equal(fixture.rendering(), 0, 'cleanup removes the actual rendered subscription');
});

test('ordinary non-traversal history stays source-defined and does not claim ownership', () => {
  const {page} = browser('Desktop Firefox'), fixture = router();
  const off = bindShellRouteMotion(fixture.router, page);
  page.dispatchEvent(new Event('popstate'));
  assert.equal(routeMotionOwnedByBrowser(), false);
  assert.deepEqual(fixture.updates, [], 'a normal push/replace history entry leaves Portico motion alone');
  fixture.fireRendered();
  assert.deepEqual(fixture.updates, [], 'a rendered commit with no UA ownership changes nothing');
  off();
});

test('old cleanup and a late rendered event cannot release a successor owner', () => {
  const {page} = browser('Desktop Safari'), older = router(), newer = router();
  const offOlder = bindShellRouteMotion(older.router, page);
  page.dispatchEvent(new Event('popstate'));
  assert.equal(routeMotionOwnedByBrowser(), false, 'desktop popstate alone is not UA-owned');
  (page as Window & {navigation: EventTarget}).navigation.dispatchEvent(traversal(true));
  assert.equal(routeMotionOwnedByBrowser(), true);
  assert.deepEqual(older.updates, [false]);

  const offNewer = bindShellRouteMotion(newer.router, page);
  (page as Window & {navigation: EventTarget}).navigation.dispatchEvent(traversal(true));
  assert.deepEqual(newer.updates, [false], 'the successor suppresses for its own route');
  assert.equal(routeMotionOwnedByBrowser(), true);
  offOlder();
  assert.deepEqual(newer.updates, [false], 'retiring the older binder never resets the successor');
  assert.equal(routeMotionOwnedByBrowser(), true);
  older.fireRendered();
  assert.deepEqual(newer.updates, [false], 'a captured old rendered callback is not registered and changes nothing');
  assert.equal(routeMotionOwnedByBrowser(), true);

  newer.fireRendered();
  assert.deepEqual(newer.updates, [false, true], 'only the successor render commit releases its own suppression');
  assert.equal(routeMotionOwnedByBrowser(), false);
  offNewer();
  finishRouteMotion();
  assert.deepEqual(newer.updates, [false, true, true], 'unbind re-asserts the default once; finish after retirement is a no-op');
  assert.equal(routeMotionOwnedByBrowser(), false);
});
