import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

/** M7 PERF-07 follow-up: a guide row renders first as a placeholder (its channel not loaded yet),
 * then with the channel. A hook after the placeholder's early return changes the hook order and
 * React throws "Rendered more hooks than during the previous render", taking down the guide. */
test('GuideRow calls every hook before its placeholder return', () => {
  const src = readFileSync(new URL('../src/screens/channels/Channels.tsx', import.meta.url), 'utf8');
  const start = src.indexOf('const GuideRow = React.memo(function GuideRow(');
  const end = src.indexOf('\n});', start);
  const body = src.slice(start, end);
  const early = body.indexOf('if (!channel) {');
  assert.ok(early > 0);
  const hooksAfter = [...body.slice(early).matchAll(/\buse[A-Z]\w*\(/g)].map(m => m[0]);
  assert.deepEqual(hooksAfter, [], 'no hook after the early return');
});

test('ListRowView calls every hook before its placeholder return', () => {
  const src = readFileSync(new URL('../src/screens/channels/Channels.tsx', import.meta.url), 'utf8');
  const start = src.indexOf('const ListRowView = React.memo(function ListRowView(');
  const end = src.indexOf('\n});', start);
  const body = src.slice(start, end);
  const early = body.indexOf('if (!channel)');
  assert.ok(early > 0);
  const hooksAfter = [...body.slice(early).matchAll(/\buse[A-Z]\w*\(/g)].map(m => m[0]);
  assert.deepEqual(hooksAfter, [], 'no hook after the early return');
});
