import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const sourceRoot = join(root, 'src');
const catalog = JSON.parse(readFileSync(resolve(root, '../api/product-language/en-US.json'), 'utf8'));
const scopes = [
  join(sourceRoot, 'App.tsx'),
  join(sourceRoot, 'runtime', 'RuntimeSurface.tsx'),
  ...['auth', 'settings', 'catalog', 'media', 'detail', 'live-tv', 'watch-with-friends', 'search']
    .map((name) => join(sourceRoot, 'features', name)),
];
const diagnosticAllowlist = new Map([
  ['src/features/detail/DetailPage.tsx', ['test(error.message)']],
  ['src/features/settings/HttpSettingsDataSource.ts', ['reason.message.trim()']],
]);

function filesAt(path) {
  if (statSync(path).isFile()) return [path];
  return readdirSync(path, { withFileTypes: true }).flatMap((entry) => {
    const child = join(path, entry.name);
    if (entry.isDirectory()) return filesAt(child);
    return /\.(?:ts|tsx)$/.test(entry.name) ? [child] : [];
  });
}

const failures = [];
for (const file of scopes.flatMap(filesAt)) {
  const name = relative(root, file);
  const source = readFileSync(file, 'utf8');
  const allowed = diagnosticAllowlist.get(name) ?? [];
  source.split(/\r?\n/).forEach((line, index) => {
    if (!/(?:reason|error|failure)\?*\.message/.test(line)) return;
    if (allowed.some((fragment) => line.includes(fragment))) return;
    failures.push(`${name}:${index + 1}: raw Error.message can reach product copy`);
  });
  for (const match of source.matchAll(/reviewedProductError(?:Text)?\(\s*[^,\n]+,\s*'([^']+)'/g)) {
    const id = match[1];
    const message = catalog.messages[id];
    if (!message) failures.push(`${name}: missing Product Language message ${id}`);
    else if (!message.icon) failures.push(`${name}: Product Language fallback ${id} needs a semantic icon`);
  }
}

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}
console.log('Web Product Language inventory passed.');
