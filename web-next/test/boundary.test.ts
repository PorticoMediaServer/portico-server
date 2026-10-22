import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync, mkdirSync, writeFileSync, symlinkSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {lintBoundary} from '../scripts/lint-boundary.mjs';
function fixture(t: import('node:test').TestContext) {
  const repo = mkdtempSync(join(tmpdir(), 'portico-boundary-'));
  const app = join(repo, 'web-next'), src = join(app, 'src');
  mkdirSync(src, {recursive: true}); mkdirSync(join(repo, 'web/src'), {recursive: true});
  writeFileSync(join(repo, 'web/src/Forbidden.tsx'), 'export const Forbidden = () => null;');
  t.after(() => rmSync(repo, {recursive: true, force: true}));
  const write = (name: string, code: string) => { const p = join(src, name); mkdirSync(join(p, '..'), {recursive: true}); writeFileSync(p, code); };
  const lint = (paths = {'@/*': ['./src/*']}) => lintBoundary({repo, app, paths, entries: [src]});
  return {repo, app, src, write, lint};
}
test('new modules, aliases, type imports, re-exports and explicit dynamic imports pass', t => {
  const f = fixture(t); f.write('allowed.ts', 'export type Allowed = string;');
  f.write('index.ts', "import type {Allowed} from '@/allowed'; export * from './allowed'; const load = () => import('./allowed');");
  assert.deepEqual(f.lint(), []);
});
for (const source of [
  "import {Forbidden} from '../../web/src/Forbidden';",
  "export * from '../../web/src/Forbidden';",
  "export {Forbidden} from '../../web/src/Forbidden';",
  "const load = () => import('../../web/src/Forbidden');",
  "const load = () => import(`../../web/src/Forbidden`);",
  "const load = require('../../web/src/Forbidden');",
  "type X = import('../../web/src/Forbidden').Forbidden;",
  "import x = require('../../web/src/Forbidden');",
  "import type {Forbidden} from '../../web/src/Forbidden';",
  "import '../../web/src/../src/Forbidden.tsx?raw';",
]) test('rejects forbidden import syntax: ' + source, t => {
  const f = fixture(t); f.write('bad.ts', source); assert.match(f.lint().join('\n'), /old UI boundary/);
});
test('an alias cannot traverse or point into the old tree', t => {
  const f = fixture(t); f.write('bad.ts', "import '@/../../web/src/Forbidden';"); assert.match(f.lint().join('\n'), /old UI boundary/);
  f.write('bad.ts', "import '@old/Forbidden';"); assert.match(f.lint({'@old/*': ['../web/src/*']}).join('\n'), /old UI boundary/);
});
test('symlink files and directories cannot conceal the old UI', t => {
  const f = fixture(t); symlinkSync(join(f.repo, 'web/src'), join(f.src, 'linked'));
  assert.match(f.lint().join('\n'), /old UI boundary/);
});
test('transitive re-exports outside app source still reach the boundary', t => {
  const f = fixture(t); mkdirSync(join(f.repo, 'shared'));
  writeFileSync(join(f.repo, 'shared/index.ts'), "export * from '../web/src/Forbidden';");
  f.write('index.ts', "import '../../shared';"); assert.match(f.lint().join('\n'), /old UI boundary/);
});
test('CSS imports are checked and unresolved/variable imports fail closed', t => {
  const f = fixture(t); f.write('bad.css', "@import '../../web/src/Forbidden.tsx';"); assert.match(f.lint().join('\n'), /old UI boundary/);
  f.write('bad.css', ''); f.write('bad.ts', "const load = (path: string) => import(path);"); assert.match(f.lint().join('\n'), /literal path/);
  f.write('bad.ts', "import './missing';"); assert.match(f.lint().join('\n'), /unresolved/);
});
