import test from 'node:test';
import assert from 'node:assert/strict';
import {compareScreens} from './compare.mjs';

const node = (frame, patch = {}) => ({id: frame, frame, tag: 'h1', ownText: 'Public heading', box: {x: 10, y: 20, width: 100, height: 40}, style: {fontFamily: 'Example', fontSize: '32px', fontWeight: '800', opacity: '.7', color: 'rgb(46, 63, 73)'}, ...patch});
const spec = elements => ({scope: 'screen', fullViewport: true, root: {width: 200, height: 100}, viewport: {deviceScaleFactor: 1}, anchorIssues: [], elements});

test('identical semantic screen passes, diagnostic DOM classes never change measured typography', () => {
  const reference = spec([node('title')]), actual = spec([node('title', {classes: ['different-implementation-wrapper']})]);
  const result = compareScreens(reference, actual);
  assert.equal(result.verdict, 'PASS'); assert.equal(result.differences.length, 0);
  assert.ok(result.diagnostics.some(value => value.property === 'classes' && value.scored === false));
});
test('measured negative fixture reports type, opacity, geometry, missing and extra drawn content', () => {
  const reference = spec([node('title'), node('all-genres', {ownText: 'Public genres'})]);
  const actual = spec([node('title', {box: {x: 14, y: 20, width: 100, height: 40}, style: {fontFamily: 'Example', fontSize: '34px', fontWeight: '800', opacity: '.6', color: 'rgb(255, 0, 0)'}}), node('added', {ownText: 'Added content'})]);
  const result = compareScreens(reference, actual);
  assert.equal(result.verdict, 'FAIL');
  for (const property of ['fontSize', 'opacity', 'box.x', 'color']) assert.ok(result.differences.some(value => value.property === property));
  assert.ok(result.differences.some(value => value.kind === 'missing-element' && value.label === 'all-genres'));
  assert.ok(result.differences.some(value => value.kind === 'extra-element' && value.label === 'added'));
});
test('two pixel box boundary passes; larger geometry, viewport mismatch and ambiguous matching cannot pass', () => {
  assert.equal(compareScreens(spec([node('title')]), spec([node('title', {box: {x: 12, y: 20, width: 100, height: 40}})])).verdict, 'PASS');
  assert.equal(compareScreens(spec([node('title')]), spec([node('title', {box: {x: 12.01, y: 20, width: 100, height: 40}})])).verdict, 'FAIL');
  assert.equal(compareScreens(spec([node('title')]), {...spec([node('title')]), fullViewport: false}).verdict, 'FAIL');
  const plain = node(null, {id: 'expected', tag: 'button'});
  const ambiguous = compareScreens(spec([plain]), spec([{...plain, id: 'one'}, {...plain, id: 'two'}]));
  assert.equal(ambiguous.verdict, 'FAIL'); assert.ok(ambiguous.differences.some(value => value.kind === 'ambiguous-match'));
});
test('image slot geometry and fit are checked without artwork contents', () => {
  const image = node('art', {tag: 'img', ownText: '', image: {kind: 'image', loaded: true}, style: {objectFit: 'contain'}});
  assert.equal(compareScreens(spec([image]), spec([{...image}])).verdict, 'PASS');
  assert.ok(compareScreens(spec([image]), spec([{...image, style: {objectFit: 'cover'}}])).differences.some(value => value.property === 'objectFit'));
});

test('wrapped control label is a separate measured text part without descendant text hoisting', () => {
  const button = node('submit', {tag: 'button', ownText: 'Public action'});
  const shell = {...button, ownText: ''}, label = {...button, id: 'submit-label', frame: 'submit-label', tag: 'span'};
  const adapter = {textHandledBySlots: ['submit'], actualContentOnly: ['submit-label'], contentSlots: [{reference: 'submit', actual: 'submit-label'}]};
  const result = compareScreens(spec([button]), spec([shell, label]), adapter);
  assert.equal(result.verdict, 'PASS');
  const changed = {...label, style: {...label.style, fontSize: '34px'}};
  assert.ok(compareScreens(spec([button]), spec([shell, changed]), adapter).differences.some(value => value.property === 'fontSize'));
});
