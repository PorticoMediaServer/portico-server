import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '../src/index.ts';
import {nextMessages} from '../src/catalog/en-US/next/index.ts';
import {savedMessages} from '../src/catalog/en-US/next/saved.ts';

/** @type {import('../src/index.ts').MessageId[]} */
const phoneMessages = ['next.saved.phone.emptyBody', 'next.saved.phone.storageLimitHelp', 'next.saved.phone.free',
  'next.saved.phone.remove', 'next.saved.phone.gone', 'next.saved.phone.otherProfiles', 'next.saved.phone.stoppedHelp',
  'next.saved.phone.downloading', 'next.saved.phone.onThisPhone'];

test('Saved messages reach the canonical catalogue used by every client', () => {
  for (const [id, text] of Object.entries(savedMessages)) {
    assert.equal(nextMessages[id], text, id);
    assert.equal(defaultI18n.has(id), true, id);
  }
  for (const id of phoneMessages) assert.equal(defaultI18n.has(id), true, id);
  assert.equal(defaultI18n.t('next.saved.phone.remove'), savedMessages['next.saved.phone.remove']);
});
