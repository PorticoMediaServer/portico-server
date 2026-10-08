import test from 'node:test';
import assert from 'node:assert/strict';
import {validDirectPassword} from '../src/direct-account.ts';

test('accepts a policy password: 8+ chars, upper, lower and a non-letter', () => {
  assert.equal(validDirectPassword('Abcdef1!'), true);
  assert.equal(validDirectPassword('Correct Horse 9'), true);
  assert.equal(validDirectPassword('Pässwörd1'), true);
});

test('rejects short, single-case, letter-only and oversize values', () => {
  assert.equal(validDirectPassword(''), false);
  assert.equal(validDirectPassword('Ab1!'), false);
  assert.equal(validDirectPassword('abcdefgh1!'), false, 'no uppercase');
  assert.equal(validDirectPassword('ABCDEFGH1!'), false, 'no lowercase');
  assert.equal(validDirectPassword('Abcdefgh'), false, 'letters only');
  // 72-byte UTF-8 cap: 71 ASCII chars fit, 73 do not.
  assert.equal(validDirectPassword('A' + 'a'.repeat(69) + '1'), true);
  assert.equal(validDirectPassword('A' + 'a'.repeat(71) + '1'), false);
  // Counting is by code point, not UTF-16 units: 7 emojis + 'Aa1' is 10 chars.
  assert.equal(validDirectPassword('😀'.repeat(7) + 'Aa1'), true);
  assert.equal(validDirectPassword('😀'.repeat(4) + 'Aa1'), false, '7 code points is too short');
});

test('edge: non-string-shaped and boundary input never throws', () => {
  assert.equal(validDirectPassword('Aa1!Aa1!'), true);
  assert.equal(validDirectPassword('        Aa1'), true, 'spaces count as non-letters');
  assert.equal(validDirectPassword('12345678'), false);
});
