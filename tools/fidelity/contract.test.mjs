import test from 'node:test';
import assert from 'node:assert/strict';
import {compareMeasurements} from './compare-files.mjs';
import {validateMeasurement} from './contract.mjs';

const fixture = () => ({version: 1, scope: 'screen', fullViewport: true,
  root: {x: 0, y: 0, width: 390, height: 844}, viewport: {width: 390, height: 844, deviceScaleFactor: 1}, anchorIssues: [], repeatedLists: [], strings: [],
  elements: ['title', 'caption'].map((frame, index) => ({id: String(index), parent: null, frame, tag: 'Text', role: 'text', leaf: true, ownText: frame, normalizedText: frame,
    box: {x: 16, y: 20 + index * 40, width: 200, height: 30}, textRects: [{x: 16, y: 20 + index * 40, width: 100, height: 25}], focused: index === 0,
    effectiveOpacity: 1, effectiveBackgroundColor: 'rgb(7, 16, 24)', style: {fontFamily: 'Example', fontSize: '24px', fontWeight: '700', color: 'rgb(237, 245, 250)'}}))});

test('one shared contract compares native logical geometry without falsifying physical pixel ratio', () => {
  const reference = fixture(), actual = fixture(); actual.measurement = {renderer: 'react-native', platform: 'android', coordinateUnit: 'dp'}; actual.viewport.deviceScaleFactor = 3;
  assert.deepEqual(validateMeasurement(actual), []);
  const result = compareMeasurements(reference, actual, {focus: {frame: 'title'}});
  assert.equal(result.verdict, 'PASS'); assert.deepEqual(result.calibration.counts, {self: 0, oneFont: 1, oneOpacity: 1});
  delete actual.measurement;
  assert.ok(compareMeasurements(reference, actual).differences.some(value => value.kind === 'screen-state'));
});
test('actual unique focused target is scored only for an explicitly drawn focus state', () => {
  const reference = fixture(), actual = fixture(); actual.elements[0].focused = false;
  assert.equal(compareMeasurements(reference, actual).verdict, 'PASS');
  assert.equal(compareMeasurements(reference, actual, {focus: {frame: 'title'}}).differences.filter(value => value.kind === 'focus-state').length, 1);
  actual.elements.forEach(node => node.focused = true);
  assert.equal(compareMeasurements(reference, actual, {focus: {frame: 'title'}}).differences.filter(value => value.kind === 'focus-state').length, 1);
});
test('native unavailable text geometry stays explicit and prevents a zero-difference pass', () => {
  const reference = fixture(), actual = fixture(); actual.measurement = {renderer: 'react-native', platform: 'tvos', coordinateUnit: 'pt'};
  delete actual.elements[1].textRects;
  assert.ok(validateMeasurement(actual).includes('elements.1.textRects'));
  actual.unavailable = [{element: '1', property: 'textRects', reason: 'Actual native line callback unavailable'}];
  assert.deepEqual(validateMeasurement(actual), []);
  assert.equal(compareMeasurements(reference, actual).differences.filter(value => value.kind === 'unavailable-measurement').length, 1);
});
test('invalid measured root or duplicate identities fails before comparison', () => {
  const reference = fixture(), actual = fixture(); actual.root.x = 4; actual.elements[1].id = '0';
  const result = compareMeasurements(reference, actual);
  assert.equal(result.verdict, 'FAIL'); assert.ok(result.differences.some(value => value.label === 'actual.root.viewport'));
  assert.ok(result.differences.some(value => value.label === 'actual.elements.1.id'));
});
