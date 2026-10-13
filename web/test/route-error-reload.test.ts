import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

type Shown = {props: {children: {props: {action: {onClick: () => void}}}[]}};
const screen = () => componentModule(new URL('../src/screens/NotFound.tsx', import.meta.url), {
  react: {default: {createElement: (type: unknown, props: object, ...children: unknown[]) => ({type, props: {...props, children}})}, useEffect: () => {}},
  '../ui': {StateView: 'state-view'},
  '../app/i18n': {useI18n: () => ({t: (id: string) => id})},
  '@tanstack/react-router': {useNavigate: () => () => {}},
}) as Promise<typeof import('../src/screens/NotFound.tsx')>;
const tryAgain = (shown: unknown) => (shown as Shown).props.children[0]!.props.action.onClick;

// What each browser reports when a split screen's file cannot be loaded, e.g. an open tab asking
// for a file name from the build the server ran before it was upgraded.
const missingChunk = [
  new TypeError('Failed to fetch dynamically imported module: https://media.example/assets/Settings-B1x2y3z4.js'),
  new TypeError('error loading dynamically imported module: https://media.example/assets/Settings-B1x2y3z4.js'),
  new TypeError('Importing a module script failed.'),
  new Error('Unable to preload CSS for /assets/Settings-C5d6e7f8.css'),
];

test('a screen whose file could not be loaded is recognised, and nothing else is', async () => {
  const {screenFileFailed} = await screen();
  for (const error of missingChunk) assert.equal(screenFileFailed(error), true, error.message);
  for (const error of [new Error('Cannot read properties of undefined'), new TypeError('Failed to fetch'), undefined, null, 'Importing a module script failed.', {message: 42}]) assert.equal(screenFileFailed(error), false);
});

test('Try again reloads the page when the screen’s file could not be loaded, and retries in place otherwise', async t => {
  const {RouteErrorScreen} = await screen();
  let reloads = 0, resets = 0;
  const had = Object.getOwnPropertyDescriptor(globalThis, 'location');
  Object.defineProperty(globalThis, 'location', {configurable: true, value: {reload: () => { reloads++; }}});
  t.after(() => { if (had) Object.defineProperty(globalThis, 'location', had); else delete (globalThis as {location?: unknown}).location; });
  for (const error of missingChunk) tryAgain(RouteErrorScreen({error, reset: () => { resets++; }}))();
  assert.deepEqual([reloads, resets], [missingChunk.length, 0], 'retrying in place would only repeat the failed load');
  tryAgain(RouteErrorScreen({error: new Error('render failed'), reset: () => { resets++; }}))();
  assert.deepEqual([reloads, resets], [missingChunk.length, 1]);
  tryAgain(RouteErrorScreen({error: new Error('render failed')}))();
  assert.equal(reloads, missingChunk.length + 1, 'with nothing to reset, reload as before');
});
