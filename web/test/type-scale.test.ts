import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync, readdirSync, statSync} from 'node:fs';
import {join} from 'node:path';

const root = new URL('../src/', import.meta.url).pathname;
function cssFiles(dir: string): string[] {
  return readdirSync(dir).flatMap(name => { const path = join(dir, name); return statSync(path).isDirectory() ? cssFiles(path) : path.endsWith('.css') ? [path] : []; });
}

// B-16.17: text follows the browser's font-size preference, so no stylesheet fixes a type size in pixels.
test('type sizes are in rem, so text follows the browser font-size preference', () => {
  const fixed: string[] = [];
  for (const file of cssFiles(root)) {
    const css = readFileSync(file, 'utf8');
    for (const m of css.matchAll(/(?:font-size|line-height):\s*([^;}]*)/g)) if (/\d(?:\.\d+)?px/.test(m[1]!)) fixed.push(`${file.slice(root.length)}: ${m[0]}`);
  }
  assert.deepEqual(fixed, []);
  assert.match(readFileSync(join(root, 'ui/tokens.css'), 'utf8'), /--type-body-size: 0\.9375rem;/);
});

test('primitives keep focus, selection and progress visible in forced colours', () => {
  const css = (name: string) => readFileSync(join(root, 'ui', name), 'utf8');
  for (const name of ['Artwork.module.css', 'Card.module.css', 'Feedback.module.css', 'Field.module.css', 'Tabs.module.css', 'Overlay.module.css', 'TitleHero.module.css', 'Button.module.css']) assert.match(css(name), /@media \(forced-colors: active\)/, name);
  assert.match(css('Card.module.css'), /\.card:has\(:focus-visible\) \.art \{ outline: 2px solid Highlight/, 'the card focus ring is a shadow, which forced colours remove');
});
