import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/auth/HostedChooser.tsx', import.meta.url), {
  react: {default: {}}, '@core/index.ts': {}, '@core/hosted-gate.ts': {}, '../../app/foreground-retry': {}, '../../app/session': {}, '../../app/i18n': {}, '../../ui': {}, './DirectProfiles': {}, './Auth.module.css': {default: {}},
}) as Promise<any>;
const a = {id: 'a'}, b = {id: 'b'};

test('Justin’s rule: one server opens by itself, several open the last one used, otherwise the list', async () => {
  const {autoServer} = await load();
  assert.equal(autoServer([a], {}), a);
  assert.equal(autoServer([a, b], {last: 'b'}), b);
  assert.equal(autoServer([a, b], {}), undefined, 'no last one: the list, in the shell');
  assert.equal(autoServer([a, b], {last: 'gone'}), undefined, 'a last server no longer listed: the list');
  assert.equal(autoServer([a, b], {current: 'a'}), undefined, 'a server already open: nothing opens by itself');
  assert.equal(autoServer([a, b], {current: 'a', preferred: 'b'}), b, 'the switcher’s choice');
});

test('relative() takes an age, not a timestamp, wherever Settings says how long ago', async () => {
  const {readFileSync, readdirSync} = await import('node:fs');
  const dir = new URL('../src/screens/settings/', import.meta.url);
  const sources = readdirSync(dir).filter(f => f.endsWith('.tsx')).map(f => readFileSync(new URL(f, dir), 'utf8'));
  assert.ok(sources.some(text => /relative\(Date\.now\(\) - Date\.parse\(iso\)\)/.test(text)), 'an age is passed');
  for (const text of sources) assert.doesNotMatch(text, /relative\((at|iso)\)/);
});
