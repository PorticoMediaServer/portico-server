/**
 * The interaction renderer's own unmount.
 *
 * React destroys a deleted subtree parent first, then its children in tree order, and within one
 * component in declaration order. react-dom 19.2.8 does that in
 * commitPassiveUnmountEffectsInsideOfDeletedTree_begin (destroy the fiber, then descend to
 * fiber.child); commitDeletionEffectsOnFiber does the same for insertion and layout destroys.
 * dispose() must match, or a screen's cleanups run in an order the real app never sees, and one
 * failing cleanup must not strand the rest.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {createRenderer} from './helpers/interaction-harness.mjs';

test('dispose runs cleanups parent first, then children, in declaration order', () => {
  const h = createRenderer();
  const order: string[] = [];
  const Leaf = () => {
    h.react.useEffect(() => () => order.push('leaf first'), []);
    h.react.useEffect(() => () => order.push('leaf second'), []);
    return h.react.createElement('span', null, 'leaf');
  };
  const Child = () => {
    h.react.useEffect(() => () => order.push('child'), []);
    return h.react.createElement(Leaf);
  };
  const Screen = () => {
    h.react.useEffect(() => () => order.push('screen first'), []);
    h.react.useEffect(() => () => order.push('screen second'), []);
    return h.react.createElement(Child);
  };
  h.render(() => h.react.createElement(Screen));
  h.dispose();
  assert.deepEqual(order, ['screen first', 'screen second', 'child', 'leaf first', 'leaf second']);
});

test('every cleanup runs when one throws, and a falsy thrown value is still the failure', () => {
  for (const thrown of [undefined, 0, '']) {
    const h = createRenderer();
    const order: string[] = [];
    const Child = () => {
      h.react.useEffect(() => () => order.push('child'), []);
      return null;
    };
    const Screen = () => {
      h.react.useEffect(() => () => { order.push('screen'); throw thrown; }, []);
      h.react.useEffect(() => () => order.push('screen second'), []);
      return h.react.createElement(Child);
    };
    h.render(() => h.react.createElement(Screen));
    assert.throws(() => h.dispose(), `dispose raises ${JSON.stringify(thrown)}`);
    assert.deepEqual(order, ['screen', 'screen second', 'child'], `every cleanup ran after ${JSON.stringify(thrown)} was thrown`);
    assert.doesNotThrow(() => h.dispose(), 'the disposed renderer has nothing left to run');
  }
});
