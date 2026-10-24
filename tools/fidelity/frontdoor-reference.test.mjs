import test from 'node:test';
import assert from 'node:assert/strict';
import {frontdoorReferences, frontdoorStateAdapters, annotateFrontdoorReference} from './frontdoor-reference.mjs';
import {authoredReferenceScreen} from './reference-screen.mjs';
import {calibrateFrame} from './calibration.mjs';

test('frontdoor public adapters map the observed Button label separately from its focus host', () => {
  for (const [platform, id] of [['phone', 'accounts-welcome-portico'], ['androidTV', 'tv-front-door-portico']]) {
    const adapter = frontdoorStateAdapters[platform];
    const local = id.replace(/portico$/, 'local'), title = platform === 'phone' ? 'accounts-welcome-title' : 'tv-front-door-title';
    assert.deepEqual(adapter.anchors, {[title]: 'title', [id]: 'primary-action', [id + '/label']: 'primary-action-label', [local]: 'secondary-action', [local + '/label']: 'secondary-action-label'});
    assert.deepEqual(adapter.comparison.contentSlots, ['primary', 'secondary'].map(name => ({reference: name + '-action', actual: name + '-action-label'})));
    assert.deepEqual(adapter.comparison.textHandledBySlots, ['primary-action', 'secondary-action']);
    assert.deepEqual(adapter.comparison.actualContentOnly, ['primary-action-label', 'secondary-action-label']);
    assert.equal(frontdoorReferences[platform].viewport.deviceScaleFactor, 1);
    assert.ok(!JSON.stringify(adapter).includes('data-f'));
  }
  assert.equal(frontdoorStateAdapters.phone.comparison.focus, undefined);
  assert.deepEqual(frontdoorStateAdapters.androidTV.comparison.focus, {frame: 'primary-action'});
});

test('cached reference annotations preserve every measured fact and refuse ambiguous targets', () => {
  for (const platform of ['phone', 'androidTV']) {
    const viewport = frontdoorReferences[platform].viewport;
    const node = (id, tag, classes, ownText) => ({id, parent: id === '0' ? null : '0', tag, classes, ownText, normalizedText: ownText,
      leaf: id !== '0', focused: false, effectiveOpacity: 1, box: {x: 16, y: 20, width: 100, height: 30}, style: {fontFamily: 'Example', fontSize: '24px', fontWeight: '800'}});
    const cached = authoredReferenceScreen({version: 1, scope: 'screen', measurement: {renderer: 'dom', coordinateUnit: 'css-px'},
      root: {x: 0, y: 0, width: viewport.width, height: viewport.height}, viewport, fullViewport: true, anchorIssues: [], repeatedLists: [],
      elements: [node('0', 'div', [], ''), node('0.1', 'h1', [], 'Public heading'), node('0.2', 'button', ['btn', 'play'], 'Primary'), node('0.3', 'button', ['btn', 'glass'], 'Secondary')],
      strings: [{id: 'heading', owner: '0.1', kind: 'text', text: 'Public heading'}]}, viewport);
    const original = JSON.stringify(cached), annotated = annotateFrontdoorReference(cached, platform);
    assert.equal(JSON.stringify(cached), original);
    assert.deepEqual(annotated.elements.map(value => value.frame), [undefined, 'title', 'primary-action', 'secondary-action']);
    assert.equal(annotated.strings[0].frame, 'title');
    const strip = screen => {const result = structuredClone(screen); for (const value of [...result.elements, ...result.strings]) delete value.frame; return result;};
    assert.deepEqual(strip(annotated), strip(cached));
    assert.deepEqual(calibrateFrame(annotated).counts, {self: 0, oneFont: 1, oneOpacity: 1});
    assert.throws(() => annotateFrontdoorReference({...cached, elements: [...cached.elements, node('duplicate', 'h1', [], 'Other')]}, platform), /unique/);
    assert.throws(() => annotateFrontdoorReference({...cached, viewport: {...viewport, width: viewport.width + 1}}, platform));
  }
});
