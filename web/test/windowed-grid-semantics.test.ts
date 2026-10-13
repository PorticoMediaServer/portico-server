import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

const src = (path: string) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

test('a windowed grid is a list whose cells say where they are in the whole collection', () => {
  const grid = src('ui/WindowedGrid.tsx');
  assert.match(grid, /role=\{semantics === 'list' \? 'list' : undefined\}/);
  assert.match(grid, /role="listitem" aria-setsize=\{setSize\} aria-posinset=\{index \+ 1\}/, 'position within the whole collection, not among the cells drawn');
  assert.match(grid, /setSize=\{semantics === 'list' \? \(snapshot\.total \?\? -1\) : undefined\}/, 'an unknown total is said to be unknown');
  assert.match(src('ui/WindowedGrid.module.css'), /\.item \{ display: contents; \}/, 'the wrapper takes no part in layout');
});

test('focus that would be lost with a card scrolled out of the window goes to the list', () => {
  const grid = src('ui/WindowedGrid.tsx');
  assert.match(grid, /tabIndex=\{-1\} onFocusCapture=/);
  assert.match(grid, /document\.activeElement === document\.body \|\| document\.activeElement === null\)\) el\.focus\(\{preventScroll: true\}\)/);
});

test('the title table numbers its rows in the whole table and opts out of list semantics', () => {
  const table = src('screens/library/LibraryViews.tsx');
  assert.match(table, /role="table"[^>]*aria-rowcount=\{rowCount === undefined \? -1 : rowCount \+ 1\}/);
  assert.match(table, /semantics="none"/);
  assert.match(table, /role="row" aria-rowindex=\{rowIndex\}/);
});
