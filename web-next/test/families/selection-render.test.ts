import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from '../transform-tsx.mjs';
import type {ReactElement} from 'react';

// Exercise the actual renderer under Node, including its real Icon and CSS-module imports.
registerHooks({load(url, context, next) {
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});
const {SelectionCircle} = await import('../../src/kit/selection/SelectionCircle.tsx');
const {SelectionHoldFeedback} = await import('../../src/kit/selection/SelectionHoldFeedback.tsx');

test('the interactive circle is a named native button with controlled pressed and disabled states', () => {
  const html = renderToStaticMarkup(createElement(SelectionCircle, {selected: true, selecting: true, disabled: true, accessibilityLabel: 'Select the movie', name: 'selection', value: 'movie-1', size: 24}));
  assert.match(html, /^<button/);
  assert.match(html, /type="button"/);
  assert.match(html, /aria-label="Select the movie"/);
  assert.match(html, /aria-pressed="true"/);
  assert.match(html, /disabled=""/);
  assert.match(html, /name="selection"/);
  assert.match(html, /value="movie-1"/);
  assert.match(html, /--selection-circle-size:24px/);
});

test('a card-owned circle contributes no nested activation or focus target', () => {
  const html = renderToStaticMarkup(createElement('button', {type: 'button'}, createElement(SelectionCircle, {interactive: false, selected: true, selecting: true, accessibilityLabel: 'Select the movie'})));
  assert.equal((html.match(/<button/g) ?? []).length, 1);
  assert.match(html, /<span[^>]+aria-hidden="true"/);
  assert.doesNotMatch(html, /tabindex=|role=|aria-pressed=/);
});

test('inactive feedback paints nothing and unknown progress paints no arc', () => {
  assert.equal(renderToStaticMarkup(createElement(SelectionHoldFeedback, {active: false, progress: .5})), '');
  const html = renderToStaticMarkup(createElement(SelectionHoldFeedback, {active: true, progress: null}));
  assert.match(html, /Keep holding to select/);
  assert.doesNotMatch(html, /<circle|<svg|progressbar/);
});

test('active feedback paints only the supplied fraction and uses the exact catalogue hint', () => {
  const html = renderToStaticMarkup(createElement(SelectionHoldFeedback, {active: true, progress: .25}));
  assert.match(html, /stroke-dashoffset="0.75"/);
  assert.match(html, /Keep holding to select/);
  assert.doesNotMatch(html, /animate|duration/);
});

test('actual circle activation respects cancellation, stops parent activation and reports current controlled state', () => {
  const calls: unknown[] = [];
  const render = (SelectionCircle as unknown as {render: (props: object, ref: null) => ReactElement<{onClick: (event: object) => void}>}).render;
  const button = render({selected: true, selecting: true, accessibilityLabel: 'Select', onSelectedChange: (...args: unknown[]) => calls.push(args)}, null);
  let stopped = false;
  button.props.onClick({defaultPrevented: false, shiftKey: false, stopPropagation() { stopped = true; }});
  assert.equal(stopped, true);
  assert.deepEqual(calls, [[false, {intent: 'toggle', shiftKey: false}]]);
  calls.length = 0;
  button.props.onClick({defaultPrevented: true, shiftKey: false, stopPropagation() { throw new Error('Cancelled click propagated into semantic activation.'); }});
  assert.deepEqual(calls, []);
  button.props.onClick({defaultPrevented: false, shiftKey: true, stopPropagation() {}});
  assert.deepEqual(calls, [[true, {intent: 'range', shiftKey: true}]]);
});
