import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync, readdirSync, statSync} from 'node:fs';
import {join} from 'node:path';

const root = new URL('../src/', import.meta.url).pathname;
const files = (dir: string): string[] => readdirSync(dir).flatMap(name => { const path = join(dir, name); return statSync(path).isDirectory() ? files(path) : /\.tsx?$/.test(name) ? [path] : []; });
const source = (path: string) => readFileSync(join(root, path), 'utf8');

test('a subtitle choice, "off" and a timing correction are one request to the playing session', () => {
  const options = source('player/options.ts');
  // One place makes the change, and it makes it on the session.
  assert.equal((options.match(/playback\.changePlayback\(/g) ?? []).length, 2, 'the change, and one more attempt after the session moved');
  assert.match(options, /subtitleSessionChange\(resource, rendering, draws/);
  // No second way: the subtitle service is read and used to manage files, never asked to select.
  assert.doesNotMatch(options, /service\.choose\(|service\.import\(|byRoute/);
});

test('no web screen asks the subtitle route to select a track', () => {
  for (const path of files(root)) {
    const text = readFileSync(path, 'utf8');
    if (!/SubtitleService|subtitles\.service|usePlayerSubtitles/.test(text)) continue;
    assert.doesNotMatch(text, /subtitles\.service!?\.(choose|import)\(|\bservice\.(choose|import)\((?!.*lyrics)/i, path);
  }
  // The player's rows go through the hook: choose, turn off, add a found track.
  const player = source('player/Player.tsx');
  assert.match(player, /subtitles\.add\(d\)/);
  assert.match(player, /subtitles\.turnOff\(\)/);
  assert.doesNotMatch(player, /\.import\(d\)/);
});

test('a refused change is said to the viewer, never retried another way', () => {
  const player = source('player/Player.tsx');
  assert.match(player, /const subtitleChangeFailed = \(\) => setNotice\(i18n\.t\('player\.subtitles\.changeFailed'\)\)/);
  assert.equal((player.match(/subtitles\.(choose|turnOff|add)\([^)]*\)\.catch\(\(\) => \{\}\)/g) ?? []).length, 0, 'no choice fails in silence');
});
