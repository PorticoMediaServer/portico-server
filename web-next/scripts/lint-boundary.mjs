import {existsSync, readdirSync, readFileSync, realpathSync, statSync} from 'node:fs';
import {resolve, dirname, relative, extname, isAbsolute} from 'node:path';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {createRequire, isBuiltin} from 'node:module';
import {parseSync} from 'rolldown/utils';
const extensions = ['', '.ts', '.tsx', '.js', '.mjs', '.css', '.json', '/index.ts', '/index.tsx', '/index.js'];
const within = (root, path) => path === root || (!relative(root, path).startsWith('..') && !isAbsolute(relative(root, path)));
function canonical(path) {
  if (existsSync(path)) return realpathSync(path);
  const parent = dirname(path);
  return parent === path ? path : resolve(canonical(parent), path.slice(parent.length + 1));
}
export function lintBoundary({repo, app = resolve(repo, 'web-next'), entries, paths} = {}) {
  repo = resolve(repo); app = resolve(app);
  const old = resolve(repo, 'web'), oldReal = canonical(old), failures = [], visited = new Set();
  const configPaths = paths ?? JSON.parse(readFileSync(resolve(app, 'tsconfig.json'), 'utf8')).compilerOptions.paths;
  const aliases = Object.entries(configPaths).map(([key, values]) => ({key: key.replace(/\/\*$/, ''), star: key.endsWith('/*'), target: resolve(app, values[0].replace(/\/\*$/, ''))})).sort((a, b) => b.key.length - a.key.length);
  function reject(path, from, specifier) {
    if (within(old, resolve(path)) || within(oldReal, canonical(path))) {
      failures.push(`${relative(repo, from)}: old UI boundary: ${specifier} -> ${relative(repo, path)}`); return true;
    }
    return false;
  }
  function local(specifier, from) {
    let spec = specifier.replace(/[?#].*$/, '');
    if (spec.startsWith('file:')) return fileURLToPath(specifier);
    const alias = aliases.find(a => spec === a.key || a.star && spec.startsWith(a.key + '/'));
    if (alias) return alias.target + spec.slice(alias.key.length);
    if (spec.startsWith('.')) return resolve(dirname(from), spec);
    if (spec.startsWith('/src/')) return resolve(app, '.' + spec);
    if (isAbsolute(spec)) return spec;
    if (spec === 'virtual:portico-kit-families' || isBuiltin(spec)) return;
    // Resolve installed packages too: a symlink must not disguise an old UI module.
    try { return createRequire(pathToFileURL(from)).resolve(spec); }
    catch { failures.push(`${relative(repo, from)}: unresolved import ${specifier}`); }
  }
  function follow(spec, from) {
    if (typeof spec !== 'string') { failures.push(`${relative(repo, from)}: dynamic imports must use a literal path`); return; }
    const target = local(spec, from);
    if (!target || reject(target, from, spec)) return;
    const file = extensions.map(extension => target + extension).find(candidate => existsSync(candidate) && statSync(candidate).isFile());
    if (!file) { failures.push(`${relative(repo, from)}: unresolved local import ${spec}`); return; }
    if (reject(file, from, spec)) return;
    if (!file.includes('/node_modules/')) visit(file);
  }
  function visit(file) {
    if (reject(file, file, file)) return;
    const real = canonical(file);
    if (visited.has(real)) return;
    visited.add(real);
    if (statSync(file).isDirectory()) { for (const child of readdirSync(file)) visit(resolve(file, child)); return; }
    if (!/\.(?:[cm]?[jt]sx?|css)$/.test(file) || file.endsWith('.d.ts')) return;
    const source = readFileSync(file, 'utf8');
    if (extname(file) === '.css') {
      for (const match of source.matchAll(/@import\s+(?:url\(\s*)?['"]([^'"]+)['"]/g)) follow(match[1], file);
      return;
    }
    const parsed = parseSync(file, source);
    if (parsed.errors.length) { failures.push(`${relative(repo, file)}: source cannot be parsed`); return; }
    function walk(node) {
      if (!node || typeof node !== 'object') return;
      if (['ImportDeclaration', 'ExportNamedDeclaration', 'ExportAllDeclaration', 'TSImportType'].includes(node.type) && node.source) follow(node.source.value, file);
      if (node.type === 'TSExternalModuleReference') follow(node.expression?.value, file);
      if (node.type === 'ImportExpression') follow(node.source.type === 'Literal' ? node.source.value : node.source.type === 'TemplateLiteral' && node.source.expressions.length === 0 ? node.source.quasis[0].value.cooked : undefined, file);
      if (node.type === 'CallExpression' && node.callee?.name === 'require') follow(node.arguments[0]?.value, file);
      for (const [key, value] of Object.entries(node)) if (key !== 'parent') {
        if (Array.isArray(value)) value.forEach(walk); else if (value && typeof value === 'object') walk(value);
      }
    }
    walk(parsed.program);
  }
  for (const entry of entries ?? [resolve(app, 'src'), resolve(app, 'vite.config.ts'), resolve(app, 'scripts')]) visit(resolve(entry));
  return failures;
}
if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const repo = fileURLToPath(new URL('../../', import.meta.url));
  const failures = lintBoundary({repo});
  if (failures.length) { console.error(failures.join('\n')); process.exit(1); }
  console.log('old UI boundary clean');
}
