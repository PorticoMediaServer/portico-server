import assert from 'node:assert/strict';
import {test} from 'node:test';
import {sectionIntent} from '../src/services/HomePage.tsx';

// FX177: only the two supported, non-secret section anchors become Home intent.
// Every other fragment stays inert so arbitrary hashes can never navigate.

test('the supported Join and Set up anchors become section intent', () => {
  assert.equal(sectionIntent('#join'), 'join');
  assert.equal(sectionIntent('#setup'), 'setup');
});

test('every other fragment is refused', () => {
  for (const hash of ['', '#', '#joi', '#JOIN', '#join/', '#setup?x=1', '#account', '#local', '#join-setup', '#%6Aoin', '#join&setup', '#security']) {
    assert.equal(sectionIntent(hash), undefined, hash);
  }
});
