import test from 'node:test';
import assert from 'node:assert/strict';
import {audiobookRates, listeningRates, parseListeningPreferences} from '../src/listening.ts';

test('audiobooks may be played at up to three times speed; music stays within two', () => {
  assert.deepEqual([...audiobookRates], [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3]);
  assert.equal(Math.max(...listeningRates), 2);
  const saved = (musicRate: number, bookRate: number) => ({revision: 3, musicRate, bookRate, autoplayNext: true, passoutMinutes: 0});
  const cases: {name: string; music: number; book: number; ok: boolean}[] = [
    {name: 'a book at three times', music: 1, book: 3, ok: true},
    {name: 'a book at two and a half', music: 1, book: 2.5, ok: true},
    {name: 'music at three times', music: 3, book: 1, ok: false},
    {name: 'a book at four times', music: 1, book: 4, ok: false},
    {name: 'a speed between the steps', music: 1, book: 2.2, ok: false},
  ];
  for (const c of cases) {
    if (c.ok) assert.equal(parseListeningPreferences(saved(c.music, c.book)).bookRate, c.book, c.name);
    else assert.throws(() => parseListeningPreferences(saved(c.music, c.book)), undefined, c.name);
  }
});
