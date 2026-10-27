import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import type {LicenceData} from '../../packages/client-core/src/presentation/licences.ts';
import {retainedOptionalNotice} from '../scripts/licences.ts';
const lock = readFileSync(new URL('../../package-lock.json', import.meta.url), 'utf8');
const data = JSON.parse(readFileSync(new URL('../src/generated/licences.json', import.meta.url), 'utf8')) as LicenceData;
const path = 'node_modules/@napi-rs/canvas-android-arm64';
test('absent optional platform packages retain their exact recorded full notice and copyright', () => {
  const retained = retainedOptionalNotice(data, lock, path);
  assert.ok(retained);
  const original = data.components.find(component => component.id === retained.id)!;
  assert.deepEqual(retained, {...original, text: data.texts[original.text!], copyright: original.copyright ? [...original.copyright] : undefined});
});
test('retained notices cannot cover a changed lock, required package, missing component or missing text', () => {
  assert.equal(retainedOptionalNotice(data, lock + '\n', path), undefined);
  assert.equal(retainedOptionalNotice(data, lock, 'node_modules/react'), undefined);
  assert.equal(retainedOptionalNotice({...data, components: []}, lock, path), undefined);
  assert.equal(retainedOptionalNotice({...data, texts: []}, lock, path), undefined);
});
